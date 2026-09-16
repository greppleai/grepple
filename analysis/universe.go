// Package analysis provides deterministic, read-only projections over a caller-supplied source universe.
package analysis

import (
	"bytes"
	"sort"

	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

// Source is one repository-relative source file and its complete content.
type Source struct {
	Path    string
	Content []byte
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
	sources    []parsedSource
	paths      []string
	graph      parser.NavigationGraph
	summary    SourceSummary
	truncation *Truncation
}

// NewUniverse parses each selected source once. Sources must use repository-relative paths.
func NewUniverse(input []Source, maxFiles int) (*Universe, error) {
	ordered := append([]Source(nil), input...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	universe := &Universe{summary: SourceSummary{Discovered: len(ordered)}}
	eligible := ordered[:0]
	for _, source := range ordered {
		capabilities, supported := parser.CapabilitiesForLanguage(parser.LanguageFor(source.Path))
		if !supported || !capabilities.Navigation || bytes.IndexByte(source.Content, 0) >= 0 {
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
	documents := make([]search.NavigationDocumentSource, 0, len(eligible))
	for _, source := range eligible {
		document, err := parser.ParseDocument(parser.LanguageFor(source.Path), string(source.Content))
		if err != nil {
			universe.summary.Failed++
			continue
		}
		universe.sources = append(universe.sources, parsedSource{path: source.Path, document: document, outline: parser.OutlineFromDocument(source.Path, document)})
		universe.paths = append(universe.paths, source.Path)
		documents = append(documents, search.NavigationDocumentSource{Path: source.Path, Document: document})
	}
	graph, stats := search.BuildNavigationGraphFromDocuments(documents, search.NavigationBuildOptions{})
	universe.graph = graph
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
