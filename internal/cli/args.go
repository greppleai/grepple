package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/alexflint/go-arg"
	searchcommand "github.com/greppleai/grepple/internal/cli/search"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/outputspill"
)

const DefaultResultLimit = searchcommand.DefaultResultLimit
const DefaultTextOutputBytes = searchcommand.DefaultTextOutputBytes

// Run parses one complete application argument tree, then executes the selected command.
func Run(args []string) error {
	configureProcessDefaults()
	if command, commandArgs := explicitApplicationCommand(args); command != "" {
		if err := validateCommandAvailability(command, commandArgs); err != nil {
			return err
		}
	}
	values, help, err := parseApplicationArgs(args)
	if err != nil || help {
		return err
	}
	repositoryOptions := values.repositoryOptions()
	spill := values.spillOptions()
	if values.Artifacts != nil && values.Artifacts.Clean != nil {
		spill.Disabled = true
	}
	exitState := &cliruntime.ExitState{}
	repository, _, err := cliruntime.LoadInvocationRepositoryConfig(repositoryOptions)
	if err != nil {
		return err
	}
	descriptorArgs := normalizedApplicationArgs(args)
	err = outputspill.Run(descriptorArgs, spill, repository.Output.SpillThresholdBytes, cliruntime.WorkingDirectory(), func(threshold int) error {
		context := newCommandContextWith(repositoryOptions, threshold, exitState.Request)
		return executeArguments(context, values)
	})
	if err != nil {
		return err
	}
	if code := exitState.Requested(); code != 0 {
		return cliruntime.NewExitError(code)
	}
	return nil
}

func parseApplicationArgs(args []string) (*Arguments, bool, error) {
	if len(args) > 1 && args[0] == "help" {
		if command, _ := explicitApplicationCommand(args[1:]); command == "" {
			return nil, false, fmt.Errorf("unknown help topic %q", args[1])
		}
	}
	values := &Arguments{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple"}, values)
	if err != nil {
		return nil, false, err
	}
	if err := parser.Parse(normalizedApplicationArgs(args)); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			if values.Search != nil {
				fmt.Fprintln(os.Stdout, values.Search.Description())
			}
			parser.WriteHelp(os.Stdout)
			return values, true, nil
		}
		return nil, false, err
	}
	if values.SpillThreshold < -1 {
		return nil, false, fmt.Errorf("--spill-threshold-bytes must be non-negative")
	}
	return values, false, nil
}

func normalizedApplicationArgs(args []string) []string {
	if len(args) > 0 && args[0] == "help" {
		if len(args) == 1 {
			args = []string{"--help"}
		} else {
			args = append(append([]string(nil), args[1:]...), "--help")
		}
	}
	normalized := append([]string(nil), args...)
	if !hasExplicitApplicationCommand(normalized) {
		normalized = append([]string{"search"}, normalized...)
	}
	for index, value := range normalized {
		switch value {
		case "graph":
			next := nextApplicationCommandToken(normalized, index+1)
			if !map[string]bool{"build": true, "resolve": true, "diff": true, "callers": true, "callees": true, "impact": true, "dependencies": true, "dependents": true}[next] {
				normalized = insertApplicationArgument(normalized, index+1, "build")
			}
			return normalized
		case "grit":
			next := nextApplicationCommandToken(normalized, index+1)
			if next != "run" && next != "explain" {
				normalized = insertApplicationArgument(normalized, index+1, "run")
			}
			return normalized
		}
	}
	return normalized
}
func nextApplicationCommandToken(args []string, start int) string {
	for index := start; index < len(args); index++ {
		value := args[index]
		if value == "--artifact-dir" || value == "--spill-threshold-bytes" {
			index++
			continue
		}
		if strings.HasPrefix(value, "--artifact-dir=") || strings.HasPrefix(value, "--spill-threshold-bytes=") || value == "--no-spill" || value == "--no-repo-config" || value == "--no-config-ignore" || value == "--production-only" || value == "--daemon" {
			continue
		}
		return value
	}
	return ""
}

func insertApplicationArgument(args []string, index int, value string) []string {
	args = append(args, "")
	copy(args[index+1:], args[index:])
	args[index] = value
	return args
}

func explicitApplicationCommand(args []string) (string, []string) {
	commands := map[string]bool{"search": true, "write": true, "grit": true, "graph": true, "anchors": true, "boundaries": true, "examples": true, "artifacts": true, "context": true, "extract": true, "architecture": true, "sources": true, "init": true, "verify": true, "languages": true, "rules": true, "repos": true, "get": true, "tree": true, "refs": true, "ask": true, "ai-provider": true, "login": true, "logout": true, "version": true}
	for index := 0; index < len(args); index++ {
		value := args[index]
		if value == "--artifact-dir" || value == "--spill-threshold-bytes" {
			index++
			continue
		}
		if strings.HasPrefix(value, "--artifact-dir=") || strings.HasPrefix(value, "--spill-threshold-bytes=") || value == "--no-spill" || value == "--no-repo-config" || value == "--no-config-ignore" || value == "--production-only" || value == "--daemon" {
			continue
		}
		if commands[value] {
			return value, args[index+1:]
		}
		return "", nil
	}
	return "", nil
}

func hasExplicitApplicationCommand(args []string) bool {
	commands := map[string]bool{"search": true, "write": true, "grit": true, "graph": true, "anchors": true, "boundaries": true, "examples": true, "artifacts": true, "context": true, "extract": true, "architecture": true, "sources": true, "init": true, "verify": true, "languages": true, "rules": true, "repos": true, "get": true, "tree": true, "refs": true, "ask": true, "ai-provider": true, "login": true, "logout": true, "version": true}
	for index := 0; index < len(args); index++ {
		value := args[index]
		if value == "--help" || value == "-h" || value == "--version" {
			return true
		}
		if value == "--artifact-dir" || value == "--spill-threshold-bytes" {
			index++
			continue
		}
		if strings.HasPrefix(value, "--artifact-dir=") || strings.HasPrefix(value, "--spill-threshold-bytes=") || value == "--no-spill" || value == "--no-repo-config" || value == "--no-config-ignore" || value == "--production-only" || value == "--daemon" {
			continue
		}
		return commands[value]
	}
	return false
}

func validateCommandAvailability(command string, args []string) error {
	availability := map[string]string{
		"write": "local-only", "anchors": "local-only", "examples": "source-independent", "artifacts": "local-only", "context": "local-only",
		"languages": "source-independent", "extract": "local-only", "sources": "local-only", "init": "local-only", "verify": "local-only", "ai-provider": "source-independent",
	}
	remoteOnly := map[string]bool{"get": true, "repos": true, "refs": true, "rules": true, "login": true, "logout": true}
	for _, argument := range args {
		if argument == "--" {
			break
		}
		option := argument
		if equals := strings.IndexByte(option, '='); equals >= 0 {
			option = option[:equals]
		}
		if mode, unsupported := availability[command]; unsupported && (option == "--remote" || option == "-R" || option == "--repo" || option == "--server") {
			return fmt.Errorf("%s is %s; remote selector %s is not supported", command, mode, option)
		}
		if remoteOnly[command] && option == "--local" {
			return fmt.Errorf("%s uses the remote service; --local is not supported", command)
		}
	}
	return nil
}
