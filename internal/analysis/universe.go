// Package analysis provides deterministic, read-only projections over a caller-supplied source universe.
package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
)

// Source is one repository-relative source file and its complete content. ReadError
// preserves discovery accounting when a caller cannot load a discovered source.
type Source struct {
	Path      string
	Content   []byte
	ReadError error
}

// ReadSources loads already selected paths while retaining read failures for completeness accounting.
func ReadSources(paths []string) []Source {
	sources := make([]Source, 0, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(path)
		sources = append(sources, Source{Path: path, Content: content, ReadError: err})
	}
	return sources
}

// SourceSummary reports analysis completeness.
type SourceSummary struct {
	Discovered int `json:"discovered"`
	Selected   int `json:"selected"`
	Parsed     int `json:"parsed"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
	Recovered  int `json:"recovered"`
}

// Truncation reports omitted source files.
type Truncation struct {
	Reason  string `json:"reason"`
	Limit   int    `json:"limit"`
	Skipped int    `json:"skipped"`
}

type parsedSource struct {
	path     string
	document *parser.Document
	outline  parser.FileOutline
}

// Universe owns parsed documents and one resolved navigation graph.
type Universe struct {
	sources            []parsedSource
	paths              []string
	graph              parser.NavigationGraph
	navigationAnalysis *search.NavigationAnalysis
	summary            SourceSummary
	truncation         *Truncation
}

// NewUniverse parses each selected source once. Sources must use repository-relative paths.
func NewUniverse(input []Source, maxFiles int) (*Universe, error) {
	return NewUniverseWithOptions(input, maxFiles, navigation.BuildOptions{})
}

// NewUniverseWithOptions parses each selected source once using the requested graph options.
func NewUniverseWithOptions(input []Source, maxFiles int, options navigation.BuildOptions) (*Universe, error) {
	ordered := append([]Source(nil), input...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	universe := &Universe{summary: SourceSummary{Discovered: len(ordered)}}
	parserService := parser.NewParser()
	eligible := ordered[:0]
	for _, source := range ordered {
		capabilities, supported := parserService.CapabilitiesForLanguage(parserService.LanguageFor(source.Path))
		if !supported || !capabilities.Navigation {
			universe.summary.Skipped++
			continue
		}
		eligible = append(eligible, source)
	}
	if maxFiles > 0 && len(eligible) > maxFiles {
		universe.truncation = &Truncation{Reason: "max_files", Limit: maxFiles, Skipped: len(eligible) - maxFiles}
		eligible = eligible[:maxFiles]
	}
	universe.summary.Selected = len(eligible)
	documents := make([]navigation.DocumentSource, 0, len(eligible))
	for _, source := range eligible {
		if source.ReadError != nil {
			universe.summary.Failed++
			continue
		}
		if bytes.IndexByte(source.Content, 0) >= 0 {
			universe.summary.Skipped++
			continue
		}
		document, err := parserService.Parse(parserService.LanguageFor(source.Path), string(source.Content))
		if err != nil {
			universe.summary.Failed++
			continue
		}
		universe.sources = append(universe.sources, parsedSource{path: source.Path, document: document, outline: parserService.Outline(document, source.Path)})
		universe.paths = append(universe.paths, source.Path)
		documents = append(documents, navigation.DocumentSource{Path: source.Path, Document: document})
	}
	navigationAnalysis, stats := search.BuildNavigationAnalysisFromDocuments(documents, options)
	universe.navigationAnalysis = navigationAnalysis
	universe.graph = navigationAnalysis.Graph()
	universe.summary.Parsed = stats.Parsed
	universe.summary.Skipped += stats.Skipped
	universe.summary.Failed += stats.Failed
	universe.summary.Recovered = stats.Recovered
	return universe, nil
}

// Close releases every caller-owned parser document.
func (u *Universe) Close() {
	if u == nil {
		return
	}
	for _, source := range u.sources {
		source.document.Close()
	}
	u.sources = nil
}

// Graph returns the immutable resolved graph projection.
func (u *Universe) Graph() parser.NavigationGraph { return u.graph }

// NavigationAnalysis returns the reusable resolved search index.
func (u *Universe) NavigationAnalysis() *search.NavigationAnalysis { return u.navigationAnalysis }

// Document returns the caller-owned parsed document for one source path.
func (u *Universe) Document(path string) *parser.Document {
	if u == nil {
		return nil
	}
	requested, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	for _, source := range u.sources {
		candidate, candidateErr := filepath.Abs(source.path)
		if candidateErr == nil && filepath.Clean(candidate) == filepath.Clean(requested) {
			return source.document
		}
	}
	return nil
}

// Summary returns source completeness counters.
func (u *Universe) Summary() SourceSummary { return u.summary }

// Truncation returns a copy of source truncation metadata.
func (u *Universe) Truncation() *Truncation {
	if u.truncation == nil {
		return nil
	}
	cloned := *u.truncation
	return &cloned
}
