package grepplecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"grepple/internal/api"
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
	return SafeWrite(strings.Join([]string{
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
		"add flags: --id --name --mode(count|files) -l/--files --regex -i/--ignore-case",
		"           --repo PATTERN --exclude-repo PATTERN",
		"",
	}, "\n"))
}

type rulesAddArgs struct {
	Server      string   `arg:"-s,--server" placeholder:"URL" help:"remote router URL"`
	ID          string   `arg:"--id" placeholder:"ID" help:"stable rule id (default: slug of --name)"`
	Name        string   `arg:"--name" placeholder:"NAME" help:"human-readable name"`
	Mode        string   `arg:"--mode" placeholder:"MODE" help:"count | files"`
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

func runRulesAdd(args []string) error {
	var values rulesAddArgs
	if err := parseRuleArgs("rules add", &values, args); err != nil {
		return err
	}
	req := api.SearchRequest{}
	if values.Pattern != "" {
		q := values.Pattern
		req.Query = &q
	}
	if len(values.Globs) > 0 {
		req.Globs = values.Globs
	}
	if values.Regex {
		b := true
		req.Regex = &b
	}
	if values.IgnoreCase {
		b := true
		req.IgnoreCase = &b
	}
	if len(values.Repo) > 0 {
		req.Repo = values.Repo
	}
	if len(values.ExcludeRepo) > 0 {
		req.ExcludeRepo = values.ExcludeRepo
	}
	mode := values.Mode
	if mode == "" && values.Files {
		mode = api.RuleModeFiles
	}
	rule := api.Rule{ID: values.ID, Name: values.Name, Mode: mode, Request: req}

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
		return JSONWrite(created)
	}
	return SafeWrite(fmt.Sprintf("created rule %s (mode=%s)\n", created.ID, created.Mode))
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
		return JSONWrite(set)
	}
	if len(set.Rules) == 0 {
		return SafeWrite("no rules defined\n")
	}
	for _, r := range set.Rules {
		if err := SafeWrite(fmt.Sprintf("%s\t%s\t%s\n", r.ID, r.Mode, r.Name)); err != nil {
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
		return JSONWrite(rule)
	}
	return JSONWrite(rule)
}

func runRulesDelete(args []string) error {
	id, server, _, err := ruleIDFlags("rules rm", args)
	if err != nil {
		return err
	}
	if err := ruleRequest(http.MethodDelete, serverDefault(server)+"/public/rules/"+id, nil, nil); err != nil {
		return err
	}
	return SafeWrite(fmt.Sprintf("deleted rule %s\n", id))
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
		return JSONWrite(results)
	}
	if len(results.Repos) == 0 {
		if err := SafeWrite("no matches\n"); err != nil {
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
		if err := SafeWrite(line + "\n"); err != nil {
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
