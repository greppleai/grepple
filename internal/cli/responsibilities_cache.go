package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/parser"
)

const responsibilityCacheSchema = "grepple-responsibility-cache-v3"

type responsibilityGraphCache struct {
	Schema string                `json:"schema"`
	Digest string                `json:"digest"`
	Graph  navigationGraphOutput `json:"graph"`
}

func buildCachedResponsibilityGraph(globs []string, maxFiles int, useCache bool) (navigationGraphOutput, string, error) {
	paths, err := navigationInputPaths(globs)
	if err != nil {
		return navigationGraphOutput{}, "", err
	}
	if !useCache {
		return buildNavigationGraphOutputFromPaths(paths, maxFiles), "disabled", nil
	}
	digest, err := responsibilityInputDigest(paths, maxFiles)
	if err != nil {
		return navigationGraphOutput{}, "", err
	}
	cachePath := filepath.Join(".grepple", "cache", "responsibilities", digest+".json")
	if cached, ok := readResponsibilityGraphCache(cachePath, digest); ok {
		return cached, "hit", nil
	}
	output := buildNavigationGraphOutputFromPaths(paths, maxFiles)
	_ = writeResponsibilityGraphCache(cachePath, responsibilityGraphCache{Schema: responsibilityCacheSchema, Digest: digest, Graph: output})
	return output, "miss", nil
}

func responsibilityInputDigest(paths []string, maxFiles int) (string, error) {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%s\x00%d\x00", responsibilityCacheSchema, maxFiles)
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
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readResponsibilityGraphCache(path, digest string) (navigationGraphOutput, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return navigationGraphOutput{}, false
	}
	var cached responsibilityGraphCache
	if json.Unmarshal(content, &cached) != nil || cached.Schema != responsibilityCacheSchema || cached.Digest != digest {
		return navigationGraphOutput{}, false
	}
	return cached.Graph, true
}

func writeResponsibilityGraphCache(path string, cached responsibilityGraphCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content, err := json.Marshal(cached)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".responsibilities-*.tmp")
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
