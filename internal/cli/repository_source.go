package cli

import (
	"os"
	"path/filepath"

	"github.com/greppleai/grepple/internal/repositoryscope"
	"github.com/greppleai/grepple/search"
)

func repositoryScopeOptions() (repositoryscope.Options, error) {
	workingDirectory := mustGetwd()
	options := repositoryscope.Options{WorkingDirectory: workingDirectory, IgnoreRoot: workingDirectory, ProductionOnly: activeRepositoryOptions.productionOnly, Notices: os.Stderr}
	config, path, err := loadRepositoryConfig()
	if err != nil {
		return options, err
	}
	if path != "" && !activeRepositoryOptions.ignoreDisabled {
		options.IgnoreRoot = filepath.Dir(path)
		options.IgnorePaths = append([]string(nil), config.Ignore.Paths...)
	}
	return options, nil
}

func applyRepositorySourceConfig(params *search.Params) error {
	options, err := repositoryScopeOptions()
	if err != nil {
		return err
	}
	repositoryscope.Configure(params, options)
	return nil
}

func configureResolvedSearchParams(params search.Params, err error) (search.Params, error) {
	if err != nil {
		return params, err
	}
	err = applyRepositorySourceConfig(&params)
	return params, err
}
