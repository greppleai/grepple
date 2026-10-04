package hook

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/gritql"
)

// Args runs configured read-only checks against changed files, or the full repository.
type Args struct {
	All     bool     `arg:"--all" help:"scan all selected repository sources instead of Git-changed files"`
	IDs     []string `arg:"--id,separate" placeholder:"ID" help:"run only this hook ID; repeatable"`
	JSON    bool     `arg:"--json" help:"emit a complete structured report"`
	Workers int      `arg:"--workers" default:"4" placeholder:"N" help:"scan up to N files concurrently (1-4; default 4)"`
}

func (Args) Description() string {
	return "Run local GritQL hooks in .grepple/hooks; defaults to staged, unstaged, and untracked Git files."
}

type Finding struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Report struct {
	Schema   string    `json:"schema"`
	Mode     string    `json:"mode"`
	Files    int       `json:"files"`
	Hooks    []string  `json:"hooks"`
	Findings []Finding `json:"findings"`
}

type command struct{ context cliruntime.Context }

func New(application cliruntime.Context) cliruntime.Command { return &command{context: application} }

func (command *command) Run(args []string) error {
	values := Args{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple hook"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(command.context.Stdout())
			return nil
		}
		return err
	}
	return Execute(command.context, &values)
}

// Execute runs hooks from already parsed application arguments.
func Execute(application cliruntime.Context, values *Args) error {
	if values.Workers < 1 || values.Workers > maxHookWorkers {
		return fmt.Errorf("hook --workers must be between 1 and %d", maxHookWorkers)
	}
	root, err := hookRoot(application.Repository().WorkingDirectory())
	if err != nil {
		return err
	}
	rules, err := loadRules(root, values.IDs)
	if err != nil {
		return err
	}
	policy, err := application.Repository().ScopeOptions()
	if err != nil {
		return err
	}
	localRules := make([]compiledRule, 0, len(rules))
	var relationalRules []compiledRule
	var metricRules []compiledRule
	for _, item := range rules {
		if item.relation != nil {
			relationalRules = append(relationalRules, item)
		} else if item.metric != nil {
			metricRules = append(metricRules, item)
		} else {
			localRules = append(localRules, item)
		}
	}
	// A cross-file result depends on declarations in unchanged files. Never
	// treat Git-changed paths as a complete relational source universe.
	full := values.All || len(relationalRules) != 0
	mode := "changed"
	if full {
		mode = "all"
	}
	report := Report{Schema: "grepple-hook-v1", Mode: mode, Hooks: make([]string, 0, len(rules)), Findings: []Finding{}}
	for _, rule := range rules {
		report.Hooks = append(report.Hooks, rule.ID)
	}
	if len(rules) > 0 {
		ctx := context.Background()
		files, err := selectedFiles(ctx, root, full, policy)
		if err != nil {
			return err
		}
		report.Files = len(files)
		if len(files) > 0 {
			if len(localRules) != 0 {
				found, err := scanRules(ctx, root, files, localRules, full, values.Workers)
				if err != nil {
					return err
				}
				report.Findings = append(report.Findings, found...)
			}
			if len(metricRules) != 0 {
				found, err := scanMetricRulesCached(ctx, root, files, metricRules, full)
				if err != nil {
					return err
				}
				report.Findings = append(report.Findings, found...)
			}
			if len(relationalRules) != 0 {
				found, err := scanRelationalRulesCached(ctx, root, files, relationalRules, values.Workers)
				if err != nil {
					return err
				}
				if !values.All {
					changedOnly := make(map[string]bool)
					for _, item := range relationalRules {
						if item.Relation != nil && item.Relation.ReportChangedOnly {
							changedOnly[item.ID] = true
						}
					}
					if len(changedOnly) > 0 {
						changedFiles, err := selectedFiles(ctx, root, false, policy)
						if err != nil {
							return err
						}
						changed := make(map[string]bool, len(changedFiles))
						for _, path := range changedFiles {
							changed[path] = true
						}
						filtered := found[:0]
						for _, finding := range found {
							if !changedOnly[finding.ID] || changed[finding.Path] {
								filtered = append(filtered, finding)
							}
						}
						found = filtered
					}
				}
				report.Findings = append(report.Findings, found...)
			}
			// Apply source-authored exceptions only after every engine has produced
			// complete raw findings. Caches must retain unsuppressed results.
			report.Findings, err = filterSuppressedFindings(root, report.Findings, rules...)
			if err != nil {
				return err
			}
			sort.Slice(report.Findings, func(i, j int) bool {
				a, b := report.Findings[i], report.Findings[j]
				if a.Path != b.Path {
					return a.Path < b.Path
				}
				if a.Line != b.Line {
					return a.Line < b.Line
				}
				if a.Column != b.Column {
					return a.Column < b.Column
				}
				return a.ID < b.ID
			})
		}
	}
	if values.JSON {
		if err := cliruntime.NewOutput(application.Stdout()).WriteJSON(report); err != nil {
			return err
		}
	} else {
		for _, finding := range report.Findings {
			if _, err := fmt.Fprintf(application.Stdout(), "%s:%d:%d [%s] %s: %s\n", finding.Path, finding.Line, finding.Column, finding.ID, finding.Severity, finding.Message); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(application.Stdout(), "hook mode=%s files=%d rules=%d findings=%d\n", mode, report.Files, len(rules), len(report.Findings)); err != nil {
			return err
		}
	}
	if len(report.Findings) > 0 {
		application.RequestExit(1)
	}
	return nil
}

func scanRulesUncached(ctx context.Context, root string, paths []string, rules []compiledRule, all bool, workers int) ([]Finding, error) {
	candidates := make([]gritql.ScanCandidate, 0, len(paths))
	for _, path := range paths {
		candidates = append(candidates, gritql.ScanCandidate{ReadPath: path, Path: path})
	}
	groups := make(map[string][]compiledRule)
	keys := make([]string, 0)
	for _, rule := range rules {
		key := strings.Join(rule.Include, "\x00") + "\x00\x00" + strings.Join(rule.Exclude, "\x00")
		if _, exists := groups[key]; !exists {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], rule)
	}
	sort.Strings(keys)
	found := make([]Finding, 0)
	var assertionBytes int64
	for _, key := range keys {
		group := groups[key]
		programs := make([]gritql.ProgramScan, 0, len(group))
		byID := make(map[string]rule, len(group))
		for _, item := range group {
			programs = append(programs, gritql.ProgramScan{Program: item.program, PatternID: item.ID, Message: item.Message})
			byID[item.ID] = item.rule
		}
		options := gritql.ScanOptions{IncludeGlobs: group[0].Include, ExcludeGlobs: group[0].Exclude}
		if all {
			options.EvaluateOptions = gritql.EvaluateOptions{MaxElapsed: 10 * time.Second, MaxBatchElapsed: 300 * time.Second}
		}
		rows, err := scanProgramFiles(ctx, os.DirFS(root), programs, candidates, options, workers, all)
		if err != nil {
			return nil, err
		}
		for programIndex, item := range programs {
			for _, row := range rows {
				program := row[programIndex]
				if truncations := program.Result.Truncations(); len(truncations) > 0 {
					return nil, fmt.Errorf("hook %s: incomplete scan: %v", program.PatternID, truncations)
				}
				for _, diagnostic := range program.Result.Diagnostics() {
					location := ""
					if path, ok := diagnostic.Path(); ok {
						location = path + ": "
					}
					return nil, fmt.Errorf("hook %s: incomplete scan: %s%s: %s", program.PatternID, location, diagnostic.Code(), diagnostic.Message())
				}
				config := byID[item.PatternID]
				found, err = appendStructuralMatches(found, config, program.Result.Findings(), &assertionBytes)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	var annotationErr error
	found, annotationErr = filterAnnotatedFindings(root, found, rules)
	if annotationErr != nil {
		return nil, annotationErr
	}
	sort.Slice(found, func(i, j int) bool {
		left, right := found[i], found[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		return left.ID < right.ID
	})
	return found, nil
}
