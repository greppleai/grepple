package boundaries

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/navigation"
	"github.com/greppleai/grepple/parser"
)

const boundaryCacheSchema = "grepple-boundary-cache-v7"

type boundaryGraphCache struct {
	Schema string      `json:"schema"`
	Digest string      `json:"digest"`
	Graph  GraphOutput `json:"graph"`
}

func buildCachedBoundaryGraph(globs []string, maxFiles int, useCache bool, dependencies Dependencies) (GraphOutput, string, error) {
	paths, err := dependencies.resolvePaths(globs)
	if err != nil {
		return GraphOutput{}, "", err
	}
	if !useCache {
		return dependencies.buildGraph(paths, maxFiles, navigation.BuildOptions{DisableCache: true}), "disabled", nil
	}
	digest, err := boundaryInputDigest(paths, maxFiles)
	if err != nil {
		return GraphOutput{}, "", err
	}
	cachePath := filepath.Join(dependencies.cacheDirectory(), "boundaries", digest+".json")
	if cached, ok := readBoundaryGraphCache(cachePath, digest); ok {
		return cached, "hit", nil
	}
	output := dependencies.buildGraph(paths, maxFiles, navigation.BuildOptions{})
	_ = writeBoundaryGraphCache(cachePath, boundaryGraphCache{Schema: boundaryCacheSchema, Digest: digest, Graph: output})
	return output, "miss", nil
}

func boundaryInputDigest(paths []string, maxFiles int) (string, error) {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s\x00%d\x00", boundaryCacheSchema, maxFiles)
	sortedPaths := append([]string(nil), paths...)
	sort.Strings(sortedPaths)
	for _, path := range sortedPaths {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		language := parser.LanguageFor(path)
		capabilities, _ := parser.CapabilitiesForLanguage(language)
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00%d\x00%s\x00", filepath.ToSlash(path), language, capabilities.GrammarABI, capabilities.GrammarFingerprint)
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	for _, path := range navigation.RepositoryContextFiles(sortedPaths) {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(hash, "repository-context\x00%s\x00", filepath.ToSlash(path))
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readBoundaryGraphCache(path, digest string) (GraphOutput, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return GraphOutput{}, false
	}
	var cached boundaryGraphCache
	if json.Unmarshal(content, &cached) != nil || cached.Schema != boundaryCacheSchema || cached.Digest != digest {
		return GraphOutput{}, false
	}
	return cached.Graph, true
}

func writeBoundaryGraphCache(path string, cached boundaryGraphCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".boundaries-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(content); err == nil {
		err = temporary.Close()
	} else {
		_ = temporary.Close()
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
