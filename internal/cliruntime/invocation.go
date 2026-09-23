package cliruntime

import (
	"fmt"
	"strings"
)

// ParseRepositoryInvocationOptions removes process-wide repository flags from args.
func ParseRepositoryInvocationOptions(args []string) ([]string, RepositoryInvocationOptions, error) {
	var options RepositoryInvocationOptions
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
			options.NoRepositoryConfig = true
		case "--no-config-ignore":
			options.NoConfigIgnore = true
		case "--production-only":
			options.ProductionOnly = true
		default:
			if strings.HasPrefix(argument, "--no-repo-config=") || strings.HasPrefix(argument, "--no-config-ignore=") || strings.HasPrefix(argument, "--production-only=") {
				return nil, options, fmt.Errorf("%s does not accept a value", strings.SplitN(argument, "=", 2)[0])
			}
			filtered = append(filtered, argument)
		}
	}
	return filtered, options, nil
}

// RepositoryScopeFlags returns copyable flags for the invocation policy.
func RepositoryScopeFlags(options RepositoryInvocationOptions) []string {
	flags := make([]string, 0, 3)
	if options.NoRepositoryConfig {
		flags = append(flags, "--no-repo-config")
	}
	if options.NoConfigIgnore {
		flags = append(flags, "--no-config-ignore")
	}
	if options.ProductionOnly {
		flags = append(flags, "--production-only")
	}
	return flags
}
