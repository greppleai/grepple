package api

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var repositoryNamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// ValidateRepositoryIndexConfig validates repository identities and branch/tag
// glob syntax before the configuration crosses a trust or filesystem boundary.
func ValidateRepositoryIndexConfig(config RepositoryIndexConfig) error {
	for index, target := range config.Repositories {
		repo := strings.TrimSpace(target.Repo)
		parts := strings.Split(repo, "/")
		if !repositoryNamePattern.MatchString(repo) || len(parts) != 2 || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
			return fmt.Errorf("index.repositories[%d].repo must be OWNER/REPO", index)
		}
		if err := validateRefPatterns(index, "branches", target.Branches); err != nil {
			return err
		}
		if err := validateRefPatterns(index, "tags", target.Tags); err != nil {
			return err
		}
	}
	return nil
}

func validateRefPatterns(repositoryIndex int, kind string, patterns []string) error {
	for patternIndex, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return fmt.Errorf("index.repositories[%d].%s[%d] must not be empty", repositoryIndex, kind, patternIndex)
		}
		if _, err := filepath.Match(pattern, "candidate"); err != nil {
			return fmt.Errorf("index.repositories[%d].%s[%d]: %w", repositoryIndex, kind, patternIndex, err)
		}
	}
	return nil
}
