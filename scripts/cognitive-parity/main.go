// Command cognitive-parity compares the authored GritQL cognitive rule with
// Revive 1.12.0 on the same Grepple-selected Go source universe.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/gritql"
	"github.com/greppleai/grepple/internal/parser"
	"go.yaml.in/yaml/v3"
)

type reviveFinding struct {
	Failure  string
	Position struct {
		Start struct {
			Filename string
			Offset   int
			Line     int
		}
	}
}

type score struct {
	path, name          string
	offset, line, value int
	parts               []gritql.MetricContribution
}

type difference struct {
	key, kind    string
	revive, grit score
}

var scoreExpression = regexp.MustCompile(`has cognitive complexity ([0-9]+) \(`)

func main() {
	root := flag.String("root", ".", "repository root")
	includeGenerated := flag.Bool("include-generated", false, "have Revive score generated Go sources too")
	show := flag.Int("show", 20, "number of sorted differences to print (0 for counts only)")
	flag.Parse()
	if *show < 0 {
		fail(errors.New("-show must be nonnegative"))
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		fail(err)
	}
	if err := compare(absolute, *includeGenerated, *show); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}

func compare(root string, includeGenerated bool, show int) error {
	version, err := exec.Command("revive", "-version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("identify Revive version: %w: %s", err, version)
	}
	if strings.TrimSpace(string(version)) != "version 1.12.0" {
		return fmt.Errorf("cognitive parity requires Revive 1.12.0, got %q", strings.TrimSpace(string(version)))
	}
	paths, err := listGoSources(root)
	if err != nil {
		return err
	}
	program, err := readCognitiveRule(root)
	if err != nil {
		return err
	}
	revive, err := reviveScores(root, paths, includeGenerated)
	if err != nil {
		return err
	}
	grit, err := gritScores(root, paths, program)
	if err != nil {
		return err
	}
	fmt.Printf("go_files=%d revive_functions=%d gritql_functions=%d\n", len(paths), len(revive), len(grit))
	reportDifferences(revive, grit, show)
	return nil
}

func listGoSources(root string) ([]string, error) {
	cmd := exec.Command("grepple", "--files", "**/*.go", "--limit", "0", "--max-output-bytes", "0", "--no-spill")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("grepple file discovery: %w: %s", err, output)
	}
	paths := strings.Fields(string(output))
	if len(paths) == 0 {
		return nil, errors.New("grepple selected no Go sources")
	}
	for _, path := range paths {
		if !strings.HasSuffix(path, ".go") {
			return nil, fmt.Errorf("unexpected file-list entry: %s", path)
		}
	}
	return paths, nil
}

func readCognitiveRule(root string) (*gritql.MetricQuery, error) {
	content, err := os.ReadFile(filepath.Join(root, "examples", "go-cognitive.yaml"))
	if err != nil {
		return nil, err
	}
	var config struct{ Query string }
	if err := yaml.Unmarshal(content, &config); err != nil {
		return nil, err
	}
	return gritql.CompileMetric([]byte(config.Query), gritql.CompileOptions{})
}

func reviveScores(root string, paths []string, includeGenerated bool) (map[string]score, error) {
	config, err := os.CreateTemp("", "grepple-revive-cognitive-*.toml")
	if err != nil {
		return nil, err
	}
	defer os.Remove(config.Name())
	_, err = fmt.Fprintf(config, "ignoreGeneratedHeader = %t\n[rule.cognitive-complexity]\narguments = [-1]\n", includeGenerated)
	if err != nil {
		config.Close()
		return nil, err
	}
	if err := config.Close(); err != nil {
		return nil, err
	}
	args := append([]string{"-config", config.Name(), "-formatter", "json"}, paths...)
	cmd := exec.Command("revive", args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("revive: %w: %s", err, output[:min(len(output), 500)])
	}
	var findings []reviveFinding
	if err := json.Unmarshal(output, &findings); err != nil {
		return nil, fmt.Errorf("revive JSON: %w", err)
	}
	results := make(map[string]score, len(findings))
	for _, finding := range findings {
		parts := scoreExpression.FindStringSubmatch(finding.Failure)
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected Revive message: %s", finding.Failure)
		}
		value, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		path := finding.Position.Start.Filename
		if filepath.IsAbs(path) {
			path, err = filepath.Rel(root, path)
			if err != nil {
				return nil, err
			}
		}
		path = filepath.ToSlash(path)
		key := scoreKey(path, finding.Position.Start.Offset)
		if _, exists := results[key]; exists {
			return nil, fmt.Errorf("duplicate Revive function %s", key)
		}
		results[key] = score{path: path, offset: finding.Position.Start.Offset, line: finding.Position.Start.Line, value: value}
	}
	return results, nil
}

func gritScores(root string, paths []string, metric *gritql.MetricQuery) (map[string]score, error) {
	results := map[string]score{}
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		document, err := parser.ParseDocument("go", string(content))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		scores, err := gritql.AnalyzeMetrics(context.Background(), metric.Spec, document, gritql.EvaluateOptions{})
		document.Close()
		if err != nil {
			return nil, fmt.Errorf("score %s: %w", path, err)
		}
		for _, result := range scores {
			key := scoreKey(path, result.Range.StartByte)
			if _, exists := results[key]; exists {
				return nil, fmt.Errorf("duplicate GritQL function %s", key)
			}
			results[key] = score{path: path, name: result.Name, offset: result.Range.StartByte, line: result.Range.Start.Line, value: result.Score, parts: result.Contributions}
		}
	}
	return results, nil
}

func scoreKey(path string, offset int) string { return fmt.Sprintf("%s:%d", path, offset) }

func reportDifferences(revive, grit map[string]score, show int) {
	var differences []difference
	paired, scoreDiff, reviveOnly, gritOnly := 0, 0, 0, 0
	reviveAbove, gritAbove, bothAbove, reviveThresholdOnly, gritThresholdOnly := 0, 0, 0, 0, 0
	deltas := map[int]int{}
	for key, left := range revive {
		if left.value > 15 {
			reviveAbove++
		}
		right, found := grit[key]
		if !found {
			reviveOnly++
			if left.value > 15 {
				reviveThresholdOnly++
			}
			differences = append(differences, difference{key: key, kind: "revive-only", revive: left})
			continue
		}
		paired++
		if right.value > 15 && left.value > 15 {
			bothAbove++
		} else if left.value > 15 {
			reviveThresholdOnly++
		} else if right.value > 15 {
			gritThresholdOnly++
		}
		if left.value != right.value {
			scoreDiff++
			deltas[right.value-left.value]++
			differences = append(differences, difference{key: key, kind: "score", revive: left, grit: right})
		}
	}
	for key, right := range grit {
		if right.value > 15 {
			gritAbove++
		}
		if _, found := revive[key]; !found {
			gritOnly++
			if right.value > 15 {
				gritThresholdOnly++
			}
			differences = append(differences, difference{key: key, kind: "gritql-only", grit: right})
		}
	}
	fmt.Printf("paired=%d different_scores=%d revive_only=%d gritql_only=%d delta_grit_minus_revive=%v\n", paired, scoreDiff, reviveOnly, gritOnly, deltas)
	fmt.Printf("above_15 revive=%d gritql=%d both=%d revive_only=%d gritql_only=%d\n", reviveAbove, gritAbove, bothAbove, reviveThresholdOnly, gritThresholdOnly)
	sort.Slice(differences, func(i, j int) bool { return differences[i].key < differences[j].key })
	for i, diff := range differences {
		if i >= show {
			break
		}
		fmt.Printf("%s %s Revive=%d GritQL=%d %s", diff.key, diff.kind, diff.revive.value, diff.grit.value, diff.grit.name)
		for k, part := range diff.grit.parts {
			if k == 8 {
				fmt.Printf(" ...(%d more)", len(diff.grit.parts)-k)
				break
			}
			fmt.Printf(" %s:%d@%d", part.Rule, part.Points, part.Range.Start.Line)
		}
		fmt.Println()
	}
}
