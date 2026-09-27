package hook

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/greppleai/grepple/gritql"
	"github.com/greppleai/grepple/internal/cliruntime"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

var relationPlaceholder = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func compileRelationConfig(config *relationConfig) (gritql.RelationSpec, error) {
	if config.MaxFindings < 0 || config.MaxFindings > 100_000 {
		return gritql.RelationSpec{}, fmt.Errorf("relation max_findings must be between 1 and 100000 when set")
	}
	compile := func(source string) (*gritql.Program, error) {
		if strings.TrimSpace(source) == "" {
			return nil, fmt.Errorf("relation query cannot be empty")
		}
		return gritql.Compile([]byte(source), gritql.CompileOptions{})
	}
	left, err := compile(config.LeftQuery)
	if err != nil {
		return gritql.RelationSpec{}, fmt.Errorf("left query: %w", err)
	}
	right := left
	if config.RightQuery != "" && config.RightQuery != config.LeftQuery {
		right, err = compile(config.RightQuery)
		if err != nil {
			return gritql.RelationSpec{}, fmt.Errorf("right query: %w", err)
		}
	}
	var partition *gritql.Program
	if config.PartitionQuery != "" {
		partition, err = compile(config.PartitionQuery)
		if err != nil {
			return gritql.RelationSpec{}, fmt.Errorf("partition query: %w", err)
		}
	} else if config.PartitionKey.Binding != "" || config.PartitionKey.DescendantKind != "" {
		return gritql.RelationSpec{}, fmt.Errorf("partition key requires partition query")
	}
	spec := gritql.RelationSpec{
		Left: left, Right: right, Partition: partition,
		LeftKey:      gritql.RelationKey{Binding: config.LeftKey.Binding, DescendantKind: config.LeftKey.DescendantKind},
		RightKey:     gritql.RelationKey{Binding: config.RightKey.Binding, DescendantKind: config.RightKey.DescendantKind},
		PartitionKey: gritql.RelationKey{Binding: config.PartitionKey.Binding, DescendantKind: config.PartitionKey.DescendantKind},
		Scope:        config.Scope, Mode: config.Mode, LeftInclude: config.LeftInclude, UniqueLeft: config.UniqueLeft,
	}
	if err := spec.Validate(); err != nil {
		return gritql.RelationSpec{}, err
	}
	return spec, nil
}

func validateRelationMessage(message string, spec gritql.RelationSpec) error {
	for _, match := range relationPlaceholder.FindAllStringSubmatch(message, -1) {
		key := match[1]
		if spec.Mode == "unmatched_left" && strings.HasPrefix(key, "right.") {
			return fmt.Errorf("relation message placeholder %q requires a matched right finding", key)
		}
		if key == "key" || key == "left.path" || key == "left.basename" || key == "right.path" || key == "right.basename" {
			continue
		}
		var program *gritql.Program
		var variable string
		switch {
		case strings.HasPrefix(key, "left."):
			program, variable = spec.Left, strings.TrimPrefix(key, "left.")
		case strings.HasPrefix(key, "right."):
			program, variable = spec.Right, strings.TrimPrefix(key, "right.")
		default:
			return fmt.Errorf("unknown relation message placeholder %q", key)
		}
		found := false
		for _, item := range program.Variables() {
			if item.Name == "$"+variable {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("relation message placeholder %q is not captured", key)
		}
	}
	unmatched := relationPlaceholder.ReplaceAllString(message, "")
	if strings.Contains(unmatched, "{{") || strings.Contains(unmatched, "}}") {
		return fmt.Errorf("invalid relation message placeholder")
	}
	return nil
}

func renderRelationMessage(message string, hit gritql.RelationHit) (string, error) {
	var err error
	out := relationPlaceholder.ReplaceAllStringFunc(message, func(token string) string {
		key := token[2 : len(token)-2]
		switch key {
		case "key":
			return hit.KeyText
		case "left.path":
			return hit.Left.Path()
		case "right.path":
			return hit.Right.Path()
		case "left.basename":
			return path.Base(hit.Left.Path())
		case "right.basename":
			return path.Base(hit.Right.Path())
		}
		finding := hit.Left
		name := strings.TrimPrefix(key, "left.")
		if strings.HasPrefix(key, "right.") {
			finding = hit.Right
			name = strings.TrimPrefix(key, "right.")
		}
		for _, binding := range finding.Bindings() {
			if binding.Name() != name {
				continue
			}
			if node, ok := binding.Node(); ok && node.Lexeme() != "" {
				return node.Lexeme()
			}
		}
		err = fmt.Errorf("%s: relation message capture %q is absent or not a leaf", finding.Path(), key)
		return ""
	})
	if err != nil {
		return "", err
	}
	return out, nil
}

func scanRelationalRules(ctx context.Context, root string, paths []string, rules []compiledRule, workers int) ([]Finding, error) {
	candidates := make([]gritql.ScanCandidate, len(paths))
	for i, source := range paths {
		candidates[i] = gritql.ScanCandidate{ReadPath: source, Path: source}
	}
	var found []Finding
	for _, item := range rules {
		if item.relation == nil {
			continue
		}
		options := gritql.ScanOptions{IncludeGlobs: item.Include, ExcludeGlobs: item.Exclude, EvaluateOptions: gritql.EvaluateOptions{MaxElapsed: 10 * time.Second, MaxBatchElapsed: 300 * time.Second}}
		if item.Relation != nil {
			options.EvaluateOptions.MaxFindings = item.Relation.MaxFindings
		}
		rows, err := scanProgramFiles(ctx, os.DirFS(root), item.relation.Programs(), candidates, options, workers, true)
		if err != nil {
			return nil, fmt.Errorf("hook %s: %w", item.ID, err)
		}
		hits, err := item.relation.JoinRows(rows)
		if err != nil {
			return nil, fmt.Errorf("hook %s: %w", item.ID, err)
		}
		for _, hit := range hits {
			message, err := renderRelationMessage(item.Message, hit)
			if err != nil {
				return nil, fmt.Errorf("hook %s: %w", item.ID, err)
			}
			reported := hit.Right
			if item.relation.Mode == "unmatched_left" {
				reported = hit.Left
			}
			start := reported.Start()
			found = append(found, Finding{ID: item.ID, Path: reported.Path(), Line: start.Line, Column: start.Column, Severity: item.Severity, Message: message})
		}
	}
	return found, nil
}

// RepositoryRoot resolves the hook configuration ancestor for source paths.
func RepositoryRoot(start string) (string, error) { return hookRoot(start) }

// CheckRepositoryRelation is the read-only entry point for integrations that
// cannot import internal CLI packages (notably the separately built Pi hook).
// It always checks the full scoped repository, not just Git-changed sources.
func CheckRepositoryRelation(ctx context.Context, start, id string) ([]Finding, error) {
	root, err := hookRoot(start)
	if err != nil {
		return nil, err
	}
	rules, err := loadRules(root, []string{id})
	if err != nil {
		return nil, err
	}
	if len(rules) != 1 || rules[0].relation == nil {
		return nil, fmt.Errorf("hook %s is not relational", id)
	}
	config, configPath, err := cliruntime.LoadRepositoryConfig(root)
	if err != nil {
		return nil, err
	}
	policy := sourcedomain.Options{WorkingDirectory: root, IgnoreRoot: root}
	if configPath != "" {
		policy.IgnoreRoot = filepath.Dir(configPath)
		policy.IgnorePaths = config.Ignore.Paths
	}
	paths, err := selectedFiles(ctx, root, true, policy)
	if err != nil {
		return nil, err
	}
	found, err := scanRelationalRulesCached(ctx, root, paths, rules, maxHookWorkers)
	if err != nil {
		return nil, err
	}
	return found, nil
}
