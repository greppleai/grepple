package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

// NavigationCacheDirectoryEnv enables the optional content-addressed navigation
// fact cache. Library callers remain side-effect free unless they set it.
const NavigationCacheDirectoryEnv = "GREPPLE_NAVIGATION_CACHE_DIR"

const navigationCacheSchema = "grepple-navigation-facts-v4"

type navigationCacheEntry struct {
	Schema    string          `json:"schema"`
	Digest    string          `json:"digest"`
	Recovered bool            `json:"recovered"`
	Graph     NavigationGraph `json:"graph"`
}

// CachedNavigationGraph returns path-instantiated navigation facts for source.
// The cache key covers source bytes, language, grammar ABI and grammar fingerprint.
// Cache read/write failures are ignored so cache state cannot affect graph output.
func CachedNavigationGraph(content, language, path string) (graph NavigationGraph, recovered, hit bool, err error) {
	digest := navigationCacheDigest(content, language)
	if cached, ok := readNavigationCache(digest); ok {
		return navigationGraphAtPath(cached.Graph, path), cached.Recovered, true, nil
	}
	document, err := ParseDocument(language, content)
	if err != nil {
		return NavigationGraph{}, false, false, err
	}
	defer document.Close()
	recovered = document.Root().HasError()
	neutral := NavigationGraphFromDocument(document, "")
	writeNavigationCache(digest, navigationCacheEntry{Schema: navigationCacheSchema, Digest: digest, Recovered: recovered, Graph: neutral})
	return navigationGraphAtPath(neutral, path), recovered, false, nil
}

// CachedNavigationGraphFromDocument reuses or records navigation facts for an
// already parsed document. It never reparses and leaves ownership with the caller.
func CachedNavigationGraphFromDocument(document *Document, path string) (NavigationGraph, bool) {
	if document == nil {
		return NavigationGraph{}, false
	}
	content, language := document.Source(), document.Language()
	digest := navigationCacheDigest(content, language)
	if cached, ok := readNavigationCache(digest); ok {
		return navigationGraphAtPath(cached.Graph, path), true
	}
	neutral := NavigationGraphFromDocument(document, "")
	writeNavigationCache(digest, navigationCacheEntry{
		Schema: navigationCacheSchema, Digest: digest,
		Recovered: document.Root().HasError(), Graph: neutral,
	})
	return navigationGraphAtPath(neutral, path), false
}

func navigationCacheDigest(content, language string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(navigationCacheSchema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(language))
	_, _ = hash.Write([]byte{0})
	if capabilities, ok := CapabilitiesForLanguage(language); ok {
		_, _ = hash.Write([]byte(strconv.FormatUint(uint64(capabilities.GrammarABI), 10)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(capabilities.GrammarFingerprint))
	}
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(content))
	return hex.EncodeToString(hash.Sum(nil))
}

func readNavigationCache(digest string) (navigationCacheEntry, bool) {
	directory := os.Getenv(NavigationCacheDirectoryEnv)
	if directory == "" {
		return navigationCacheEntry{}, false
	}
	content, err := os.ReadFile(filepath.Join(directory, digest+".json"))
	if err != nil {
		return navigationCacheEntry{}, false
	}
	var cached navigationCacheEntry
	if json.Unmarshal(content, &cached) != nil || cached.Schema != navigationCacheSchema || cached.Digest != digest {
		return navigationCacheEntry{}, false
	}
	return cached, true
}

func writeNavigationCache(digest string, cached navigationCacheEntry) {
	directory := os.Getenv(NavigationCacheDirectoryEnv)
	if directory == "" || os.MkdirAll(directory, 0o755) != nil {
		return
	}
	content, err := json.Marshal(cached)
	if err != nil {
		return
	}
	temporary, err := os.CreateTemp(directory, ".navigation-*.tmp")
	if err != nil {
		return
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(content); err == nil {
		err = temporary.Close()
	} else {
		_ = temporary.Close()
	}
	if err == nil {
		_ = os.Rename(temporaryPath, filepath.Join(directory, digest+".json"))
	}
}

func navigationGraphAtPath(graph NavigationGraph, path string) NavigationGraph {
	ids := make(map[string]string, len(graph.Declarations))
	for index := range graph.Declarations {
		declaration := &graph.Declarations[index]
		oldID := declaration.ID
		declaration.Path = path
		declaration.ID = navigationStableID("declaration", declaration.Language, path, declaration.Name, declaration.Kind, strconv.Itoa(declaration.Start), strconv.Itoa(declaration.End))
		ids[oldID] = declaration.ID
	}
	for index := range graph.Imports {
		graph.Imports[index].Path = path
	}
	for index := range graph.Exports {
		graph.Exports[index].Path = path
	}
	for index := range graph.Calls {
		call := &graph.Calls[index]
		call.Path = path
		call.CallerID = ids[call.CallerID]
		call.TargetID = ids[call.TargetID]
		for candidateIndex, candidateID := range call.CandidateTargetIDs {
			call.CandidateTargetIDs[candidateIndex] = ids[candidateID]
		}
		call.ID = navigationStableID("call", call.CallerID, strconv.Itoa(call.Line), call.Display, strconv.Itoa(index))
	}
	for index := range graph.Fields {
		graph.Fields[index].Path = path
	}
	for index := range graph.TypeUsages {
		usage := &graph.TypeUsages[index]
		usage.Path = path
		usage.CallerID = ids[usage.CallerID]
	}
	for index := range graph.MemberAccesses {
		access := &graph.MemberAccesses[index]
		access.Path = path
		access.CallerID = ids[access.CallerID]
		access.ID = navigationStableID("member-access", access.CallerID, strconv.Itoa(access.StartByte), access.Receiver, access.Member, access.Operation)
	}
	return graph
}
