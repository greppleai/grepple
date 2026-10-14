package navigation

import (
	"fmt"
	"github.com/greppleai/grepple/internal/parser"
)

// RelatedProjection retains established global declaration/type lookup within
// a coherent partition while paging only the selected call frontier.
func (s *pagedStoredSnapshot) RelatedProjection(files []string, cwd string, depth int) (parser.NavigationGraph, error) {
	if depth < 1 || depth > 10 {
		return parser.NavigationGraph{}, fmt.Errorf("invalid related depth")
	}
	pagedQueryAdmission <- struct{}{}
	defer func() { <-pagedQueryAdmission }()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return parser.NavigationGraph{}, fmt.Errorf("navigation generation closed")
	}
	r := newPagedQueryReader(s)
	included, selected, frontier := map[uint32]bool{}, map[uint32]bool{}, map[uint32]bool{}
	for _, node := range pagedRelatedRootRows(r, files, cwd) {
		included[node] = true
		frontier[node] = true
		s.selectPagedRelated(r, r.rows("in", node), included, selected, nil)
	}
	for level := 0; level < depth && len(frontier) > 0 && r.err == nil; level++ {
		next := map[uint32]bool{}
		for _, node := range sortedSnapshotRows(frontier) {
			s.selectPagedRelated(r, r.rows("out", node), included, selected, next)
		}
		frontier = next
	}
	if r.err != nil {
		return parser.NavigationGraph{}, r.err
	}
	graph, err := projectPagedSnapshot(r, included, selected)
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	graph.Declarations, err = s.details.declarations.readAll()
	if err != nil {
		return parser.NavigationGraph{}, err
	}
	graph.TypeDeclarations, err = s.details.types.readAll()
	return graph, err
}
func (s *pagedStoredSnapshot) selectPagedRelated(r *pagedQueryReader, edges []uint32, included, selected, next map[uint32]bool) {
	for _, edge := range edges {
		call, targets := r.call(edge)
		if r.err != nil {
			return
		}
		group := r.number("groups", edge)
		selected[group] = true
		addSnapshotRows(selected, r.rows("dup", group))
		included[call.caller] = true
		for _, target := range targets {
			included[target] = true
			if next != nil {
				next[target] = true
			}
		}
		if len(included)+len(selected) > pagedSelectionLimit {
			r.err = fmt.Errorf("%w: paged related selection limit", ErrSnapshotBudget)
			return
		}
	}
}
func (s *pagedStoredSnapshot) Declarations() ([]parser.NavigationDeclaration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return nil, fmt.Errorf("navigation generation closed")
	}
	return s.details.declarations.readAll()
}
func (s *pagedStoredSnapshot) TypeDeclarations() ([]parser.NavigationTypeDeclaration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return nil, fmt.Errorf("navigation generation closed")
	}
	return s.details.types.readAll()
}
