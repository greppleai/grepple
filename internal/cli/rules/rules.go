// Package rules implements predefined remote search commands.
package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/greppleai/grepple/api"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/rulespec"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/alexflint/go-arg"
)

// Run dispatches the rules command family. Deprecated: construct the command with New.
func Run(args []string, dependencies Dependencies) error { return New(dependencies).Run(args) }

// Run dispatches the predefined-search command family.
func (command *command) Run(args []string) error {
	if len(args) == 0 {
		return rulesUsage()
	}
	switch args[0] {
	case "add", "create":
		return command.runAdd(args[1:])
	case "list", "ls":
		return command.runList(args[1:])
	case "get", "show":
		return command.runGet(args[1:])
	case "rm", "remove", "delete":
		return command.runDelete(args[1:])
	case "results":
		return command.runResults(args[1:])
	case "-h", "--help", "help":
		return rulesUsage()
	default:
		return fmt.Errorf("unknown `grepple rules` subcommand %q (want add|list|get|rm|results)", args[0])
	}
}

func (command *command) runAdd(args []string) error  { return runRulesAdd(args, command.dependencies) }
func (command *command) runList(args []string) error { return runRulesList(args, command.dependencies) }
func (command *command) runGet(args []string) error  { return runRulesGet(args, command.dependencies) }
func (command *command) runDelete(args []string) error {
	return runRulesDelete(args, command.dependencies)
}
func (command *command) runResults(args []string) error {
	return runRulesResults(args, command.dependencies)
}

func rulesUsage() error {
	return cliruntime.NewOutput(os.Stdout).WriteString(strings.Join([]string{
		"Predefined searches: saved searches whose results are materialized per repo",
		"and refreshed whenever a repository is reindexed.",
		"",
		"Usage:",
		"  grepple rules add [flags] PATTERN [GLOB ...]   create/replace a rule",
		"  grepple rules list                             list rule definitions",
		"  grepple rules get ID                           show one rule",
		"  grepple rules rm ID                            delete a rule",
		"  grepple rules results ID [--json]              fetch materialized results",
		"",
		"add flags: --id --name --mode(count|files) --engine(text|gritql) --grit",
		"           -f/--query-file (structural) -l/--files --regex -i/--ignore-case",
		"           --repo PATTERN --exclude-repo PATTERN",
		"",
	}, "\n"))
}

type rulesAddArgs struct {
	commonArgs
	ID          string   `arg:"--id" placeholder:"ID" help:"stable rule id (default: slug of --name)"`
	Name        string   `arg:"--name" placeholder:"NAME" help:"human-readable name"`
	Mode        string   `arg:"--mode" placeholder:"MODE" help:"count | files"`
	Engine      string   `arg:"--engine" placeholder:"ENGINE" help:"text | gritql"`
	Grit        bool     `arg:"--grit" help:"create a gritql structural rule"`
	QueryFile   string   `arg:"-f,--query-file" placeholder:"PATH" help:"read structural query from PATH (- for stdin)"`
	Files       bool     `arg:"-l,--files" help:"files mode (store matching paths)"`
	Regex       bool     `arg:"--regex" help:"treat PATTERN as a regular expression"`
	IgnoreCase  bool     `arg:"-i,--ignore-case" help:"case-insensitive match"`
	Repo        []string `arg:"--repo,separate" placeholder:"PATTERN" help:"limit to matching repos"`
	ExcludeRepo []string `arg:"--exclude-repo,separate" placeholder:"PATTERN" help:"exclude matching repos"`
	JSON        bool     `arg:"--json" help:"print the created rule as JSON"`
	Pattern     string   `arg:"positional" placeholder:"PATTERN"`
	Globs       []string `arg:"positional" placeholder:"GLOB"`
}

func (rulesAddArgs) Description() string {
	return "Create or replace a predefined grep (rule)."
}

func buildRule(values rulesAddArgs) (api.Rule, error) {
	engine := strings.TrimSpace(values.Engine)
	if values.Grit {
		if engine != "" && engine != api.RuleEngineGritQL {
			return api.Rule{}, fmt.Errorf("--grit conflicts with --engine %q", engine)
		}
		engine = api.RuleEngineGritQL
	}
	mode := values.Mode
	if mode == "" && values.Files {
		mode = api.RuleModeFiles
	}
	rule := api.Rule{ID: values.ID, Name: values.Name, Mode: mode, Engine: engine}
	if engine == "" || engine == api.RuleEngineText {
		if err := setTextRuleRequest(&rule, values); err != nil {
			return api.Rule{}, err
		}
	} else if engine == api.RuleEngineGritQL {
		if err := setStructuralRuleRequest(&rule, values); err != nil {
			return api.Rule{}, err
		}
	}
	if _, err := rulespec.Normalize(rule); err != nil {
		return api.Rule{}, err
	}
	return rule, nil
}

func setTextRuleRequest(rule *api.Rule, values rulesAddArgs) error {
	if values.QueryFile != "" {
		return fmt.Errorf("--query-file requires a structural rule")
	}
	request := api.SearchRequest{}
	if values.Pattern != "" {
		request.Query = &values.Pattern
	}
	request.Globs = append([]string(nil), values.Globs...)
	if values.Regex {
		request.Regex = boolPointer(true)
	}
	if values.IgnoreCase {
		request.IgnoreCase = boolPointer(true)
	}
	request.Repo = append([]string(nil), values.Repo...)
	request.ExcludeRepo = append([]string(nil), values.ExcludeRepo...)
	rule.Request = request
	return nil
}

func setStructuralRuleRequest(rule *api.Rule, values rulesAddArgs) error {
	if values.Regex || values.IgnoreCase {
		return fmt.Errorf("--regex and --ignore-case are not valid for structural rules")
	}
	inline := values.Pattern
	globs := append([]string(nil), values.Globs...)
	if values.QueryFile != "" {
		if inline != "" {
			globs = append([]string{inline}, globs...)
		}
		inline = ""
	}
	query, err := loadRuleQuery(inline, values.QueryFile)
	if err != nil {
		return err
	}
	rule.Structural = &api.GritRequest{
		Query: query, Compatibility: api.GritCompatibilityV1, Globs: globs,
		Repositories: append([]string(nil), values.Repo...), ExcludeRepositories: append([]string(nil), values.ExcludeRepo...),
	}
	return nil
}

func loadRuleQuery(inline, path string) (string, error) {
	if inline != "" && path != "" {
		return "", fmt.Errorf("structural rule accepts query text or --query-file, not both")
	}
	if path == "" {
		if len(inline) > api.MaxGritQueryBytes {
			return "", fmt.Errorf("structural query exceeds the %d-byte maximum", api.MaxGritQueryBytes)
		}
		return inline, nil
	}
	if path == "-" {
		return readGritQuery(os.Stdin)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return readGritQuery(file)
}

func boolPointer(value bool) *bool { return &value }

func runRulesAdd(args []string, dependencies Dependencies) error {
	var values rulesAddArgs
	if err := parseRuleArgs("rules add", &values, args); err != nil {
		return err
	}
	rule, err := buildRule(values)
	if err != nil {
		return err
	}
	base := dependencies.serverDefault(values.Server)
	body, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	var created api.Rule
	if err := ruleRequest(http.MethodPost, base+"/public/rules", body, &created, dependencies); err != nil {
		return err
	}
	if values.JSON {
		return cliruntime.NewOutput(os.Stdout).WriteJSON(created)
	}
	if created.Engine == api.RuleEngineGritQL {
		return cliruntime.NewOutput(os.Stdout).WriteString(fmt.Sprintf("created structural rule %s (mode=%s, engine=%s)\n", created.ID, created.Mode, created.Engine))
	}
	return cliruntime.NewOutput(os.Stdout).WriteString(fmt.Sprintf("created rule %s (mode=%s)\n", created.ID, created.Mode))
}

func runRulesList(args []string, dependencies Dependencies) error {
	server, jsonOut, err := ruleServerFlags("rules list", args)
	if err != nil {
		return err
	}
	var set api.RuleSet
	if err := ruleRequest(http.MethodGet, dependencies.serverDefault(server)+"/public/rules", nil, &set, dependencies); err != nil {
		return err
	}
	if jsonOut {
		return cliruntime.NewOutput(os.Stdout).WriteJSON(set)
	}
	if len(set.Rules) == 0 {
		return cliruntime.NewOutput(os.Stdout).WriteString("no rules defined\n")
	}
	for _, r := range set.Rules {
		line := fmt.Sprintf("%s\t%s\t%s\n", r.ID, r.Mode, r.Name)
		if r.Engine == api.RuleEngineGritQL {
			line = fmt.Sprintf("%s\t%s\t%s\t%s\n", r.ID, r.Mode, r.Engine, r.Name)
		}
		if err := cliruntime.NewOutput(os.Stdout).WriteString(line); err != nil {
			return err
		}
	}
	return nil
}

func runRulesGet(args []string, dependencies Dependencies) error {
	id, server, jsonOut, err := ruleIDFlags("rules get", args)
	if err != nil {
		return err
	}
	var rule api.Rule
	if err := ruleRequest(http.MethodGet, dependencies.serverDefault(server)+"/public/rules/"+id, nil, &rule, dependencies); err != nil {
		return err
	}
	if jsonOut {
		return cliruntime.NewOutput(os.Stdout).WriteJSON(rule)
	}
	return cliruntime.NewOutput(os.Stdout).WriteJSON(rule)
}

func runRulesDelete(args []string, dependencies Dependencies) error {
	id, server, _, err := ruleIDFlags("rules rm", args)
	if err != nil {
		return err
	}
	if err := ruleRequest(http.MethodDelete, dependencies.serverDefault(server)+"/public/rules/"+id, nil, nil, dependencies); err != nil {
		return err
	}
	return cliruntime.NewOutput(os.Stdout).WriteString(fmt.Sprintf("deleted rule %s\n", id))
}

func runRulesResults(args []string, dependencies Dependencies) error {
	id, server, jsonOut, err := ruleIDFlags("rules results", args)
	if err != nil {
		return err
	}
	var results api.RuleResults
	if err := ruleRequest(http.MethodGet, dependencies.serverDefault(server)+"/public/rules/"+id+"/results", nil, &results, dependencies); err != nil {
		return err
	}
	if jsonOut {
		return cliruntime.NewOutput(os.Stdout).WriteJSON(results)
	}
	if len(results.Repos) == 0 {
		if err := cliruntime.NewOutput(os.Stdout).WriteString("no matches\n"); err != nil {
			return err
		}
		dependencies.requestExit(1)
		return nil
	}
	for _, r := range results.Repos {
		line := fmt.Sprintf("%s\t%d files\t%d matches", r.Repo, r.Files, r.Matches)
		if len(r.Paths) > 0 {
			line += "\t" + strings.Join(r.Paths, ",")
		}
		if err := cliruntime.NewOutput(os.Stdout).WriteString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// --- small shared helpers ---

// ruleRequest performs an authorized JSON request and decodes the response into
// out (when non-nil), surfacing non-2xx bodies as errors.
func ruleRequest(method, target string, body []byte, out any, dependencies Dependencies) error {
	var reader io.Reader
	contentType := ""
	if body != nil {
		reader = bytes.NewReader(body)
		contentType = "application/json"
	}
	req, err := dependencies.newRequest(method, target, contentType, reader)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func parseRuleArgs(program string, dest any, args []string) error {
	parser, err := arg.NewParser(arg.Config{Program: "grepple " + program}, dest)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	return nil
}

// ruleServerFlags parses the common --server/--json flags for a no-argument
// subcommand.
func ruleServerFlags(program string, args []string) (server string, jsonOut bool, err error) {
	var values struct {
		commonArgs
		JSON bool `arg:"--json"`
	}
	if err := parseRuleArgs(program, &values, args); err != nil {
		return "", false, err
	}
	return values.Server, values.JSON, nil
}

// ruleIDFlags parses a required positional ID plus --server/--json.
func ruleIDFlags(program string, args []string) (id, server string, jsonOut bool, err error) {
	var values struct {
		commonArgs
		JSON bool   `arg:"--json"`
		ID   string `arg:"positional,required" placeholder:"ID"`
	}
	if err := parseRuleArgs(program, &values, args); err != nil {
		return "", "", false, err
	}
	if values.ID == "" {
		return "", "", false, fmt.Errorf("a rule ID is required")
	}
	return values.ID, values.Server, values.JSON, nil
}
