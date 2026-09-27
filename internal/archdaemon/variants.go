package archdaemon

import "github.com/greppleai/grepple/internal/analysis"

// variant revalidates a source snapshot before returning one focused report.
func (s *service) variant(payload request, kind string) (response, bool) {
	_, base, stable, err := architectureSnapshot(payload.Root, payload.Paths, payload.MaxFiles)
	if err != nil || !stable {
		return response{}, false
	}
	key, ok := selectedVariantKey(base, payload, kind)
	if !ok {
		return response{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, hit := s.cache.getVariant(payload.Root, kind, key)
	if !hit {
		return response{}, false
	}
	switch kind {
	case graphVariant:
		graph, ok := value.(analysis.GraphReport)
		if ok {
			return response{Graph: &graph}, true
		}
	case resolveVariant:
		projection, ok := value.(ResolveProjection)
		if ok {
			return response{Projection: &projection}, true
		}
	}
	return response{}, false
}
