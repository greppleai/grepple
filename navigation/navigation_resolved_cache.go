package navigation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/greppleai/grepple/parser"
)

// A resolved graph depends on repository context beyond individual source facts.
// Changing this schema invalidates old entries when graph construction changes.
const resolvedGraphCacheSchema = "grepple-resolved-navigation-v1"
const maxResolvedGraphCacheBytes = 128 << 20
const maxResolvedContextFileBytes = 8 << 20

var executableFingerprintOnce sync.Once
var executableFingerprint string

func resolvedGraphCacheKey(sources []DocumentSource, cwd string) (string, bool) {
	if len(sources) == 0 || cwd == "" || os.Getenv(parser.NavigationCacheDirectoryEnv) == "" {
		return "", false
	}
	executableFingerprintOnce.Do(func() {
		path, err := os.Executable()
		if err != nil {
			return
		}
		file, err := os.Open(path)
		if err != nil {
			return
		}
		defer file.Close()
		digest := sha256.New()
		if _, err := io.Copy(digest, file); err == nil {
			executableFingerprint = hex.EncodeToString(digest.Sum(nil))
		}
	})
	if executableFingerprint == "" {
		return "", false
	}
	digest := sha256.New()
	writeResolvedKeyPart(digest, resolvedGraphCacheSchema)
	writeResolvedKeyPart(digest, parser.NavigationFactArtifactSchema)
	writeResolvedKeyPart(digest, executableFingerprint)
	writeResolvedKeyPart(digest, cwd)
	for _, source := range sources {
		writeResolvedKeyPart(digest, source.Path)
		if source.Document == nil {
			writeResolvedKeyPart(digest, "nil-document")
			continue
		}
		language := source.Document.Language()
		writeResolvedKeyPart(digest, language)
		writeResolvedKeyPart(digest, parser.NavigationFactDigest(source.Document.Source(), language))
	}
	contextFiles, ok := resolvedGraphContextPaths(sources)
	if !ok {
		return "", false
	}
	for _, path := range contextFiles {
		writeResolvedKeyPart(digest, path)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			writeResolvedKeyPart(digest, "absent")
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxResolvedContextFileBytes {
			return "", false
		}
		file, err := os.Open(path)
		if err != nil {
			return "", false
		}
		content, readErr := io.ReadAll(io.LimitReader(file, maxResolvedContextFileBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(content) > maxResolvedContextFileBytes {
			return "", false
		}
		writeResolvedKeyPart(digest, string(content))
	}
	return hex.EncodeToString(digest.Sum(nil)), true
}

func resolvedGraphContextPaths(sources []DocumentSource) ([]string, bool) {
	candidates := make(map[string]bool)
	for _, source := range sources {
		if source.Document == nil || !supportsNavigation(source.Document.Language()) {
			continue
		}
		var names []string
		switch navigationLanguageFamily(source.Document.Language()) {
		case "go":
			names = []string{"go.mod", "go.work"}
		case "typescript", "javascript":
			names = []string{"tsconfig.json"}
		default:
			continue
		}
		absolute, err := filepath.Abs(source.Path)
		if err != nil {
			return nil, false
		}
		for directory := filepath.Dir(absolute); ; directory = filepath.Dir(directory) {
			for _, name := range names {
				candidates[filepath.Join(directory, name)] = true
			}
			if filepath.Dir(directory) == directory {
				break
			}
		}
	}
	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, true
}

func writeResolvedKeyPart(digest hash.Hash, value string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = digest.Write(size[:])
	_, _ = io.WriteString(digest, value)
}

func documentSourceStats(sources []DocumentSource) SourceStats {
	stats := SourceStats{Attempted: len(sources)}
	for _, source := range sources {
		if source.Document == nil || !supportsNavigation(source.Document.Language()) {
			stats.Skipped++
			continue
		}
		stats.Parsed++
		if source.Document.Root().HasError() {
			stats.Recovered++
		}
	}
	return stats
}

func resolvedGraphCachePath(key string) string {
	return filepath.Join(os.Getenv(parser.NavigationCacheDirectoryEnv), "resolved", key+".pb")
}

func readResolvedGraphCache(key string) (parser.NavigationGraph, bool) {
	path := resolvedGraphCachePath(key)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxResolvedGraphCacheBytes {
		return parser.NavigationGraph{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return parser.NavigationGraph{}, false
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maxResolvedGraphCacheBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(content) > maxResolvedGraphCacheBytes {
		return parser.NavigationGraph{}, false
	}
	artifact, err := parser.UnmarshalNavigationFactArtifact(content)
	if err != nil || artifact.Digest != key {
		return parser.NavigationGraph{}, false
	}
	return artifact.Graph, true
}

func writeResolvedGraphCache(key string, graph parser.NavigationGraph, recovered bool) {
	directory := filepath.Dir(resolvedGraphCachePath(key))
	if os.MkdirAll(directory, 0o755) != nil {
		return
	}
	content, err := parser.MarshalNavigationFactArtifact(parser.NavigationFactArtifact{Digest: key, Recovered: recovered, Graph: graph})
	if err != nil || len(content) > maxResolvedGraphCacheBytes {
		return
	}
	temporary, err := os.CreateTemp(directory, ".resolved-*.tmp")
	if err != nil {
		return
	}
	path := temporary.Name()
	defer os.Remove(path)
	if _, err = temporary.Write(content); err == nil {
		err = temporary.Close()
	} else {
		_ = temporary.Close()
	}
	if err == nil {
		_ = os.Rename(path, resolvedGraphCachePath(key))
	}
}
