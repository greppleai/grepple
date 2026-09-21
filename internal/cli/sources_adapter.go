package cli

import (
	"io"
	"path/filepath"

	sourcescommand "github.com/greppleai/grepple/internal/cli/sources"
)

type sourceScopeReport = sourcescommand.Report

func sourceCommandDependencies(stdout io.Writer) sourcescommand.Dependencies {
	return sourcescommand.Dependencies{Stdout: stdout, Environment: sourceCommandEnvironment, WorkingDirectory: mustGetwd}
}

func sourceCommandEnvironment() (sourcescommand.Environment, error) {
	config, configPath, err := loadRepositoryConfig()
	if err != nil {
		return sourcescommand.Environment{}, err
	}
	root := mustGetwd()
	if configPath != "" {
		root = filepath.Dir(configPath)
	}
	return sourcescommand.Environment{
		Root:           root,
		ConfigPath:     configPath,
		IgnorePaths:    append([]string(nil), config.Ignore.Paths...),
		IgnoreDisabled: activeRepositoryOptions.ignoreDisabled,
		ProductionOnly: activeRepositoryOptions.productionOnly,
	}, nil
}

func buildSourceScopeReport(paths []string) (sourceScopeReport, error) {
	return sourcescommand.Build(paths, sourceCommandDependencies(nil))
}
