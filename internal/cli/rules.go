package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/rulespec"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/alexflint/go-arg"
)

// runRules dispatches the `grepple rules` subcommands for predefined searches.
func runRules(args []string) error {
	if len(args) == 0 {
		return rulesUsage()
	}
	switch args[0] {
	case "add", "create":
		return runRulesAdd(args[1:])
	case "list", "ls":
		return runRulesList(args[1:])
	case "get", "show":
		return runRulesGet(args[1:])
	case "rm", "remove", "delete":
		return runRulesDelete(args[1:])
	case "results":
		return runRulesResults(args[1:])
	case "-h", "--help", "help":
		return rulesUsage()
	default:
		return fmt.Errorf("unknown `grepple rules` subcommand %q (want add|list|get|rm|results)", args[0])
	}
}

func rulesUsage() error {
	return stdoutWriter().writeString(strings.Join([]string{
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
	Server        string   `arg:"-s,--server" placeholder:"URL" help:"remote router URL"`
	ID            string   `arg:"--id" placeholder:"ID" help:"stable rule id (default: slug of --name)"`
	Name          string   `arg:"--name" placeholder:"NAME" help:"human-readable name"`
	Mode          string   `arg:"--mode" placeholder:"MODE" help:"count | files"`
	Engine        string   `arg:"--engine" placeholder:"ENGINE" help:"text | gritql"`
	Grit          bool     `arg:"--grit" help:"create a gritql structural rule"`
	QueryFile     string   `arg:"-f,--query-file" placeholder:"PATH" help:"read structural query from PATH (- for stdin)"`
	Compatibility string   `arg:"--compatibility" placeholder:"VERSION" help:"structural compatibility version"`
	Files         bool     `arg:"-l,--files" help:"files mode (store matching paths)"`
	Regex         bool     `arg:"--regex" help:"treat PATTERN as a regular expression"`
	IgnoreCase    bool     `arg:"-i,--ignore-case" help:"case-insensitive match"`
	Repo          []string `arg:"--repo,separate" placeholder:"PATTERN" help:"limit to matching repos"`
	ExcludeRepo   []string `arg:"--exclude-repo,separate" placeholder:"PATTERN" help:"exclude matching repos"`
	JSON          bool     `arg:"--json" help:"print the created rule as JSON"`
	Pattern       string   `arg:"positional" placeholder:"PATTERN"`
	Globs         []string `arg:"positional" placeholder:"GLOB"`
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
	if values.QueryFile != "" || values.Compatibility != "" {
		return fmt.Errorf("--query-file and --compatibility require a structural rule")
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
	compatibility := values.Compatibility
	if compatibility == "" {
		compatibility = api.GritCompatibilityV1
	}
	rule.Structural = &api.GritRequest{
		Query: query, Compatibility: compatibility, Globs: globs,
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

func runRulesAdd(args []string) error {
	var values rulesAddArgs
	if err := parseRuleArgs("rules add", &values, args); err != nil {
		return err
	}
	rule, err := buildRule(values)
	if err != nil {
		return err
	}
	base := serverDefault(values.Server)
	body, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	var created api.Rule
	if err := ruleRequest(http.MethodPost, base+"/public/rules", body, &created); err != nil {
		return err
	}
	if values.JSON {
		return stdoutWriter().writeJSON(created)
	}
	if created.Engine == api.RuleEngineGritQL {
		return stdoutWriter().writeString(fmt.Sprintf("created structural rule %s (mode=%s, engine=%s)\n", created.ID, created.Mode, created.Engine))
	}
	return stdoutWriter().writeString(fmt.Sprintf("created rule %s (mode=%s)\n", created.ID, created.Mode))
}

func runRulesList(args []string) error {
	server, jsonOut, err := ruleServerFlags("rules list", args)
	if err != nil {
		return err
	}
	var set api.RuleSet
	if err := ruleRequest(http.MethodGet, serverDefault(server)+"/public/rules", nil, &set); err != nil {
		return err
	}
	if jsonOut {
		return stdoutWriter().writeJSON(set)
	}
	if len(set.Rules) == 0 {
		return stdoutWriter().writeString("no rules defined\n")
	}
	for _, r := range set.Rules {
		line := fmt.Sprintf("%s\t%s\t%s\n", r.ID, r.Mode, r.Name)
		if r.Engine == api.RuleEngineGritQL {
			line = fmt.Sprintf("%s\t%s\t%s\t%s\n", r.ID, r.Mode, r.Engine, r.Name)
		}
		if err := stdoutWriter().writeString(line); err != nil {
			return err
		}
	}
	return nil
}

func runRulesGet(args []string) error {
	id, server, jsonOut, err := ruleIDFlags("rules get", args)
	if err != nil {
		return err
	}
	var rule api.Rule
	if err := ruleRequest(http.MethodGet, serverDefault(server)+"/public/rules/"+id, nil, &rule); err != nil {
		return err
	}
	if jsonOut {
		return stdoutWriter().writeJSON(rule)
	}
	return stdoutWriter().writeJSON(rule)
}

func runRulesDelete(args []string) error {
	id, server, _, err := ruleIDFlags("rules rm", args)
	if err != nil {
		return err
	}
	if err := ruleRequest(http.MethodDelete, serverDefault(server)+"/public/rules/"+id, nil, nil); err != nil {
		return err
	}
	return stdoutWriter().writeString(fmt.Sprintf("deleted rule %s\n", id))
}

func runRulesResults(args []string) error {
	id, server, jsonOut, err := ruleIDFlags("rules results", args)
	if err != nil {
		return err
	}
	var results api.RuleResults
	if err := ruleRequest(http.MethodGet, serverDefault(server)+"/public/rules/"+id+"/results", nil, &results); err != nil {
		return err
	}
	if jsonOut {
		return stdoutWriter().writeJSON(results)
	}
	if len(results.Repos) == 0 {
		if err := stdoutWriter().writeString("no matches\n"); err != nil {
			return err
		}
		setExit(1)
		return nil
	}
	for _, r := range results.Repos {
		line := fmt.Sprintf("%s\t%d files\t%d matches", r.Repo, r.Files, r.Matches)
		if len(r.Paths) > 0 {
			line += "\t" + strings.Join(r.Paths, ",")
		}
		if err := stdoutWriter().writeString(line + "\n"); err != nil {
			return err
		}
	}
	return nil
}

// --- small shared helpers ---

// ruleRequest performs an authorized JSON request and decodes the response into
// out (when non-nil), surfacing non-2xx bodies as errors.
func ruleRequest(method, target string, body []byte, out any) error {
	var reader io.Reader
	contentType := ""
	if body != nil {
		reader = bytes.NewReader(body)
		contentType = "application/json"
	}
	req, err := authorizedRequest(method, target, contentType, reader)
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
		Server string `arg:"-s,--server" placeholder:"URL"`
		JSON   bool   `arg:"--json"`
	}
	if err := parseRuleArgs(program, &values, args); err != nil {
		return "", false, err
	}
	return values.Server, values.JSON, nil
}

// ruleIDFlags parses a required positional ID plus --server/--json.
func ruleIDFlags(program string, args []string) (id, server string, jsonOut bool, err error) {
	var values struct {
		Server string `arg:"-s,--server" placeholder:"URL"`
		JSON   bool   `arg:"--json"`
		ID     string `arg:"positional,required" placeholder:"ID"`
	}
	if err := parseRuleArgs(program, &values, args); err != nil {
		return "", "", false, err
	}
	if values.ID == "" {
		return "", "", false, fmt.Errorf("a rule ID is required")
	}
	return values.ID, values.Server, values.JSON, nil
}
