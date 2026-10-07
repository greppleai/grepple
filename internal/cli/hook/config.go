// Package hook runs repository-owned, read-only GritQL checks.
package hook

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/gritql"
	"go.yaml.in/yaml/v3"
)

const maxHookConfigBytes = 64 << 10

var hookID = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type rule struct {
	Version        int               `yaml:"version"`
	ID             string            `yaml:"id"`
	Enabled        yaml.Node         `yaml:"enabled" json:"-"`
	Engine         string            `yaml:"engine"`
	Include        []string          `yaml:"include"`
	Exclude        []string          `yaml:"exclude"`
	Severity       string            `yaml:"severity"`
	Message        string            `yaml:"message"`
	Query          string            `yaml:"query"`
	Relation       *relationConfig   `yaml:"relation"`
	Annotation     *annotationConfig `yaml:"annotation"`
	Assert         *assertionConfig  `yaml:"assert"`
	Unsuppressible bool              `yaml:"unsuppressible"`
}

type relationKeyConfig struct {
	Binding        string `yaml:"binding"`
	DescendantKind string `yaml:"descendant_kind"`
	Projection     string `yaml:"projection"`
}

type relationConfig struct {
	LeftQuery         string            `yaml:"left_query"`
	RightQuery        string            `yaml:"right_query"`
	PartitionQuery    string            `yaml:"partition_query"`
	LeftKey           relationKeyConfig `yaml:"left_key"`
	RightKey          relationKeyConfig `yaml:"right_key"`
	PartitionKey      relationKeyConfig `yaml:"partition_key"`
	Scope             string            `yaml:"scope"`
	GoModule          string            `yaml:"go_module"`
	Mode              string            `yaml:"mode"`
	LeftInclude       []string          `yaml:"left_include"`
	MaxFindings       int               `yaml:"max_findings"`
	ReportChangedOnly bool              `yaml:"report_changed_only"`
	UniqueLeft        bool              `yaml:"unique_left"`
}

type compiledRule struct {
	rule
	program  *gritql.Program
	relation *gritql.RelationSpec
	metric   *gritql.MetricQuery
}

// hookRoot selects the nearest ancestor with a hook directory, including from
// a nested package. It does not read config outside that repository scope.
func hookRoot(start string) (string, error) {
	root, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		info, err := os.Stat(filepath.Join(root, ".grepple", "hooks"))
		if err == nil && info.IsDir() {
			return root, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("no .grepple/hooks directory found from %s", start)
		}
		root = parent
	}
}

func loadRules(root string, ids []string) ([]compiledRule, error) {
	selection := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !hookID.MatchString(id) {
			return nil, fmt.Errorf("invalid hook --id %q", id)
		}
		selection[id] = true
	}
	directory := filepath.Join(root, ".grepple", "hooks")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	if len(entries) > 256 {
		return nil, fmt.Errorf("too many hook directory entries (maximum 256)")
	}
	rules := make([]compiledRule, 0, len(entries))
	seen := make(map[string]bool)
	enabled := make(map[string]bool)
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") || !entry.Type().IsRegular() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		if len(selection) > 0 && !selection[name] {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxHookConfigBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(content) > maxHookConfigBytes {
			return nil, fmt.Errorf("hook %s exceeds %d bytes", path, maxHookConfigBytes)
		}
		var config rule
		decoder := yaml.NewDecoder(bytes.NewReader(content))
		decoder.KnownFields(true)
		if err := decoder.Decode(&config); err != nil {
			return nil, fmt.Errorf("hook %s: %w", path, err)
		}
		var remainder any
		if err := decoder.Decode(&remainder); err != io.EOF {
			return nil, fmt.Errorf("hook %s: expected exactly one YAML document", path)
		}
		if !hookID.MatchString(config.ID) || config.ID != name || seen[config.ID] {
			return nil, fmt.Errorf("hook %s: id must match unique filename", path)
		}
		seen[config.ID] = true
		isEnabled := true
		if config.Enabled.Kind != 0 {
			if config.Enabled.Kind != yaml.ScalarNode || config.Enabled.Tag != "!!bool" {
				return nil, fmt.Errorf("hook %s: enabled must be true or false", path)
			}
			if err := config.Enabled.Decode(&isEnabled); err != nil {
				return nil, fmt.Errorf("hook %s: %w", path, err)
			}
		}
		enabled[config.ID] = isEnabled
		if config.Version != 1 || config.Engine != "gritql-v1" && config.Engine != "gritql-relational-v1" && config.Engine != "gritql-metric-v1" {
			return nil, fmt.Errorf("hook %s: requires version 1 and a supported GritQL engine", path)
		}
		if config.Severity != "error" && config.Severity != "warning" {
			return nil, fmt.Errorf("hook %s: severity must be error or warning", path)
		}
		if strings.TrimSpace(config.Message) == "" || len(config.Message) > 1024 || len(config.Include) == 0 || len(config.Include) > 32 || len(config.Exclude) > 32 {
			return nil, fmt.Errorf("hook %s: requires a short message and 1-32 include globs", path)
		}
		for _, glob := range append(append([]string(nil), config.Include...), config.Exclude...) {
			if strings.TrimSpace(glob) == "" || filepath.IsAbs(glob) || strings.ContainsRune(glob, 0) {
				return nil, fmt.Errorf("hook %s: include globs must be repository-relative", path)
			}
			for _, segment := range strings.Split(filepath.ToSlash(glob), "/") {
				if segment == ".." {
					return nil, fmt.Errorf("hook %s: include globs must not escape the repository", path)
				}
			}
		}
		if err := validateRuleSourceConstraints(config); err != nil {
			return nil, fmt.Errorf("hook %s: %w", path, err)
		}
		if config.Engine == "gritql-relational-v1" {
			if config.Query != "" || config.Relation == nil {
				return nil, fmt.Errorf("hook %s: relational engine requires relation and no file-local query", path)
			}
			spec, err := compileRelationConfig(config.Relation)
			if err != nil {
				return nil, fmt.Errorf("hook %s: %w", path, err)
			}
			if err := validateRelationMessage(config.Message, spec); err != nil {
				return nil, fmt.Errorf("hook %s: %w", path, err)
			}
			rules = append(rules, compiledRule{rule: config, relation: &spec})
			continue
		}
		if config.Engine == "gritql-metric-v1" {
			if config.Relation != nil || strings.TrimSpace(config.Query) == "" {
				return nil, fmt.Errorf("hook %s: metric engine requires query and no relation", path)
			}
			metric, err := gritql.CompileMetric([]byte(config.Query), gritql.CompileOptions{})
			if err != nil {
				return nil, fmt.Errorf("hook %s: %w", path, err)
			}
			if err := validateMetricMessage(config.Message, metric); err != nil {
				return nil, fmt.Errorf("hook %s: %w", path, err)
			}
			rules = append(rules, compiledRule{rule: config, metric: metric})
			continue
		}
		if config.Relation != nil || strings.TrimSpace(config.Query) == "" {
			return nil, fmt.Errorf("hook %s: file-local engine requires query and no relation", path)
		}
		program, err := compileFileProgram(config)
		if err != nil {
			return nil, fmt.Errorf("hook %s: %w", path, err)
		}
		rules = append(rules, compiledRule{rule: config, program: program})
	}
	for id := range selection {
		if !seen[id] {
			return nil, fmt.Errorf("unknown hook id %q", id)
		}
	}
	// A selected, disabled rule is still parsed and validated, but does not
	// enter the execution plan or appear among reported hooks.
	active := rules[:0]
	for _, item := range rules {
		if enabled[item.ID] {
			active = append(active, item)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	return active, nil
}
