package cli

import (
	"path/filepath"

	"github.com/greppleai/grepple/search"
)

func applyRepositorySourceConfig(params *search.Params) error {
	config, path, err := loadRepositoryConfig()
	if err != nil {
		return err
	}
	if params == nil || path == "" {
		return nil
	}
	params.IgnorePaths = append([]string(nil), config.Ignore.Paths...)
	params.IgnoreRoot = filepath.Dir(path)
	return nil
}

func configureResolvedSearchParams(params search.Params, err error) (search.Params, error) {
	if err != nil {
		return params, err
	}
	err = applyRepositorySourceConfig(&params)
	return params, err
}
