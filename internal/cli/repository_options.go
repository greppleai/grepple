package cli

import (
	"fmt"
	"strings"
	"sync"
)

type repositoryInvocationOptions struct {
	disabled       bool
	ignoreDisabled bool
	productionOnly bool
}

var (
	repositoryInvocationMutex sync.Mutex
	activeRepositoryOptions   repositoryInvocationOptions
)

func parseRepositoryInvocationOptions(args []string) ([]string, repositoryInvocationOptions, error) {
	var options repositoryInvocationOptions
	filtered := make([]string, 0, len(args))
	literal := false
	for _, argument := range args {
		if literal {
			filtered = append(filtered, argument)
			continue
		}
		if argument == "--" {
			literal = true
			filtered = append(filtered, argument)
			continue
		}
		switch argument {
		case "--no-repo-config":
			options.disabled = true
		case "--no-config-ignore":
			options.ignoreDisabled = true
		case "--production-only":
			options.productionOnly = true
		default:
			if strings.HasPrefix(argument, "--no-repo-config=") || strings.HasPrefix(argument, "--no-config-ignore=") || strings.HasPrefix(argument, "--production-only=") {
				return nil, options, fmt.Errorf("%s does not accept a value", strings.SplitN(argument, "=", 2)[0])
			}
			filtered = append(filtered, argument)
		}
	}
	return filtered, options, nil
}

func activeRepositoryScopeFlags() []string {
	flags := make([]string, 0, 3)
	if activeRepositoryOptions.disabled {
		flags = append(flags, "--no-repo-config")
	}
	if activeRepositoryOptions.ignoreDisabled {
		flags = append(flags, "--no-config-ignore")
	}
	if activeRepositoryOptions.productionOnly {
		flags = append(flags, "--production-only")
	}
	return flags
}

func appendActiveRepositoryScopeFlags(parts []string) []string {
	return append(parts, activeRepositoryScopeFlags()...)
}
