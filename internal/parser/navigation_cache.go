package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// NavigationCacheDirectoryEnv enables the optional content-addressed navigation
// fact cache. Library callers remain side-effect free unless they set it.
const NavigationCacheDirectoryEnv = "GREPPLE_NAVIGATION_CACHE_DIR"

const navigationCacheSchema = "grepple-navigation-facts-v27"

const maxNavigationCacheEntryBytes = 64 << 20

// NavigationFactArtifactSchema identifies the portable packed-protobuf fact format.
const NavigationFactArtifactSchema = navigationCacheSchema

// NavigationFactArtifact is one path-neutral native graph serialized for cache or server distribution.
type NavigationFactArtifact struct {
	Digest    string
	Recovered bool
	Graph     NavigationGraph
}

type navigationCacheEntry struct {
	Schema    string
	Digest    string
	Recovered bool
	Graph     NavigationGraph
}

// NavigationFactDigest returns the content, language, grammar, and schema identity used by fact artifacts.
func NavigationFactDigest(content, language string) string {
	return navigationCacheDigest(content, language)
}

// MarshalNavigationFactArtifact serializes the native graph without a JSON mirror.
func MarshalNavigationFactArtifact(artifact NavigationFactArtifact) ([]byte, error) {
	return marshalNavigationCacheEntry(navigationCacheEntry{Schema: navigationCacheSchema, Digest: artifact.Digest, Recovered: artifact.Recovered, Graph: artifact.Graph})
}

// UnmarshalNavigationFactArtifact validates and decodes one portable native graph artifact.
func UnmarshalNavigationFactArtifact(content []byte) (NavigationFactArtifact, error) {
	entry, err := unmarshalNavigationCacheEntry(content)
	if err != nil {
		return NavigationFactArtifact{}, err
	}
	if entry.Schema != navigationCacheSchema {
		return NavigationFactArtifact{}, fmt.Errorf("unsupported navigation fact schema %q", entry.Schema)
	}
	return NavigationFactArtifact{Digest: entry.Digest, Recovered: entry.Recovered, Graph: entry.Graph}, nil
}

// CachedNavigationGraph returns path-instantiated navigation facts for source.
// The cache key covers source bytes, language, grammar ABI and grammar fingerprint.
// Cache read/write failures are ignored so cache state cannot affect graph output.
func CachedNavigationGraph(content, language, path string) (graph NavigationGraph, recovered, hit bool, err error) {
	digest := navigationCacheDigest(content, language)
	if cached, ok := readNavigationCache(digest); ok {
		return navigationGraphAtPath(cached.Graph, path), cached.Recovered, true, nil
	}
	document, err := parseDocument(language, content)
	if err != nil {
		return NavigationGraph{}, false, false, err
	}
	defer document.Close()
	recovered = document.Root().HasError()
	neutral := navigationGraphFromDocument(document, "")
	writeNavigationCache(digest, navigationCacheEntry{Schema: navigationCacheSchema, Digest: digest, Recovered: recovered, Graph: neutral})
	return navigationGraphAtPath(neutral, path), recovered, false, nil
}

// cachedNavigationGraphFromDocument reuses or records graph facts for one exact
// source path. It never reparses and leaves the document owned by its caller.
// Unlike the content-only cache above, entrypoint facts may depend on path.
func cachedNavigationGraphFromDocument(document *Document, path string) NavigationGraph {
	if document == nil {
		return NavigationGraph{}
	}
	if os.Getenv(NavigationCacheDirectoryEnv) == "" {
		return navigationGraphFromDocument(document, path)
	}
	document.mu.RLock()
	defer document.mu.RUnlock()
	if document.tree == nil {
		return NavigationGraph{}
	}
	digest := navigationPathCacheDigest(document.source, document.language, path)
	if cached, ok := readNavigationCache(digest); ok {
		return cached.Graph
	}
	root := document.tree.RootNode()
	graph := navigationGraphFromTree(root, document.source, document.language, path)
	writeNavigationCache(digest, navigationCacheEntry{
		Schema: navigationCacheSchema, Digest: digest,
		Recovered: root.HasError(), Graph: graph,
	})
	return graph
}

func navigationPathCacheDigest(content, language, path string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("document-navigation\x00"))
	_, _ = hash.Write([]byte(navigationCacheDigest(content, language)))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(path))
	return hex.EncodeToString(hash.Sum(nil))
}
func navigationCacheDigest(content, language string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(navigationCacheSchema))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(language))
	_, _ = hash.Write([]byte{0})
	if capabilities, ok := capabilitiesForLanguage(language); ok {
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
	path := filepath.Join(directory, digest+".pb")
	information, err := os.Stat(path)
	if err != nil || information.Size() < 0 || information.Size() > maxNavigationCacheEntryBytes {
		return navigationCacheEntry{}, false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return navigationCacheEntry{}, false
	}
	cached, err := unmarshalNavigationCacheEntry(content)
	if err != nil || cached.Schema != navigationCacheSchema || cached.Digest != digest {
		return navigationCacheEntry{}, false
	}
	return cached, true
}

func writeNavigationCache(digest string, cached navigationCacheEntry) {
	directory := os.Getenv(NavigationCacheDirectoryEnv)
	if directory == "" || os.MkdirAll(directory, 0o755) != nil {
		return
	}
	content, err := marshalNavigationCacheEntry(cached)
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
		_ = os.Rename(temporaryPath, filepath.Join(directory, digest+".pb"))
	}
}

func navigationGraphAtPath(graph NavigationGraph, path string) NavigationGraph {
	ids := make(map[string]string, len(graph.Declarations))
	for index := range graph.Declarations {
		declaration := &graph.Declarations[index]
		oldID := declaration.ID
		declaration.Path = path
		declaration.ID = navigationDeclarationStableID(*declaration)
		ids[oldID] = declaration.ID
	}
	for index := range graph.TypeDeclarations {
		graph.TypeDeclarations[index].Path = path
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
