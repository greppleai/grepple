package navigation

import (
	"fmt"
	"path/filepath"

	"github.com/greppleai/grepple/internal/parser"
)

// RelatedProjection preserves global declaration/type lookup facts, but reads
// only root incoming calls and the requested outgoing frontier. Unresolved calls
// must remain selected: they carry external dependency qualification evidence.
func (s *storedSnapshot) RelatedProjection(files []string, buildCWD string, depth int) (parser.NavigationGraph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return parser.NavigationGraph{}, fmt.Errorf("navigation generation closed")
	}
	if s.snapshot.ids == nil {
		return parser.NavigationGraph{}, fmt.Errorf("declaration-only navigation lease")
	}
	if depth < 1 || depth > 10 {
		return parser.NavigationGraph{}, fmt.Errorf("invalid related depth")
	}
	declarations, err := s.snapshot.details.declarations.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	wanted := map[string]bool{}
	for _, file := range files {
		wanted[snapshotSourcePath(file, buildCWD)] = true
	}
	roots := map[uint32]bool{}
	for _, declaration := range declarations {
		if wanted[snapshotSourcePath(declaration.Path, buildCWD)] {
			roots[s.snapshot.ids[declaration.ID]] = true
		}
	}
	included, selected := s.snapshot.relatedRows(roots, depth)
	graph, err := s.snapshot.project(included, selected)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	graph.Declarations = declarations
	graph.TypeDeclarations, err = s.snapshot.details.types.readAll()
	return graph, err
}
func snapshotSourcePath(path, cwd string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path)
}
func (s *querySnapshot) relatedRows(roots map[uint32]bool, depth int) (map[uint32]bool, map[uint32]bool) {
	included, selected := map[uint32]bool{}, map[uint32]bool{}
	frontier := map[uint32]bool{}
	for node := range roots {
		included[node] = true
		frontier[node] = true
		s.selectRelatedEdges(s.incoming, node, included, selected, nil)
	}
	for level := 0; level < depth; level++ {
		next := map[uint32]bool{}
		for node := range frontier {
			s.selectRelatedEdges(s.outgoing, node, included, selected, next)
		}
		frontier = next
	}
	return included, selected
}
func (s *querySnapshot) selectRelatedEdges(csr snapshotCSR, node uint32, included, selected, next map[uint32]bool) {
	for _, row := range csr.edges[csr.offsets[node]:csr.offsets[node+1]] {
		group := s.groups[row]
		selected[group] = true
		addSnapshotRows(selected, s.duplicates[group])
		call := s.calls[row]
		included[call.caller] = true
		for _, target := range s.targets[call.targets : call.targets+call.count] {
			if next != nil {
				next[target] = true
			}
			included[target] = true
		}
	}
}
