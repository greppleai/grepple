// Package storagepaths resolves user-owned cache and output artifact locations.
package storagepaths

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OutputArtifacts returns the configured or default output artifact directory.
func OutputArtifacts() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("GREPPLE_ARTIFACT_DIR")); configured != "" {
		if filepath.IsAbs(configured) {
			return filepath.Clean(configured), nil
		}
		return filepath.Abs(configured)
	}
	cache, err := os.UserCacheDir()
	if err == nil && strings.TrimSpace(cache) != "" {
		return filepath.Join(cache, "grepple", "output"), nil
	}
	return filepath.Join(os.TempDir(), "grepple", "output"), nil
}

// Cache returns the repository-specific cache directory.
func Cache(workingDirectory string) string {
	if configured := strings.TrimSpace(os.Getenv("GREPPLE_CACHE_DIR")); configured != "" {
		return configured
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.Clean(workingDirectory))))[:16]
	cache, err := os.UserCacheDir()
	if err == nil && strings.TrimSpace(cache) != "" {
		candidate := filepath.Join(cache, "grepple", "cache", key)
		if os.MkdirAll(candidate, 0o700) == nil {
			return candidate
		}
	}
	fallback := filepath.Join(os.TempDir(), "grepple", "cache", key)
	_ = os.MkdirAll(fallback, 0o700)
	return fallback
}
