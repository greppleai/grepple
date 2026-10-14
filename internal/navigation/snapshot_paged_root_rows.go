package navigation

import "path/filepath"

func pagedRelatedRootRows(r *pagedQueryReader, files []string, cwd string) []uint32 {
	roots := map[uint32]bool{}
	for _, file := range files {
		nodes := r.keyRows("files", filepath.Clean(file))
		if len(nodes) == 0 {
			alternative := snapshotSourcePath(file, cwd)
			if filepath.IsAbs(file) {
				if relative, err := filepath.Rel(cwd, file); err == nil {
					alternative = relative
				}
			}
			nodes = r.keyRows("files", alternative)
		}
		addSnapshotRows(roots, nodes)
	}
	return sortedSnapshotRows(roots)
}
func pagedFileRows(s *querySnapshot) (map[string][]uint32, error) {
	declarations, err := s.details.declarations.readAll()
	if err != nil {
		return nil, err
	}
	files := map[string][]uint32{}
	for _, d := range declarations {
		path := filepath.Clean(d.Path)
		files[path] = append(files[path], s.ids[d.ID])
	}
	return files, nil
}
