package navigation

import (
	"path/filepath"
	"sort"
	"strings"
)

// navigationPathIndex shares path lookups across language resolvers for one corpus.
// Each bucket retains corpus order; resolvers keep their own matching rules.
type navigationPathIndex struct {
	byPath         map[string][]string
	byDirectory    map[string][]string
	byModule       map[string][]string
	pythonExact    map[string][]string
	pythonSuffixes map[string][]string
}

func (corpus *navigationCorpus) pathIndex() *navigationPathIndex {
	if corpus.paths != nil {
		return corpus.paths
	}
	files := corpus.files
	if len(files) == 0 && len(corpus.contents) != 0 {
		files = make([]string, 0, len(corpus.contents))
		for file := range corpus.contents {
			files = append(files, file)
		}
		sort.Strings(files)
	}
	index := &navigationPathIndex{
		byPath:         make(map[string][]string, len(files)),
		byDirectory:    make(map[string][]string, len(files)),
		byModule:       make(map[string][]string, len(files)),
		pythonExact:    make(map[string][]string),
		pythonSuffixes: make(map[string][]string),
	}
	for _, file := range files {
		path := filepath.Clean(file)
		index.byPath[path] = append(index.byPath[path], file)
		directory := filepath.Dir(path)
		index.byDirectory[directory] = append(index.byDirectory[directory], file)
		module := strings.TrimSuffix(path, filepath.Ext(file))
		index.byModule[module] = append(index.byModule[module], file)
		if filepath.Base(module) == "index" {
			parent := filepath.Dir(module)
			index.byModule[parent] = append(index.byModule[parent], file)
		}
		if pythonModule, ok := pythonModuleFilePath(file); ok {
			index.pythonExact[pythonModule] = append(index.pythonExact[pythonModule], file)
			// Absolute imports may resolve from any ancestor source root. Index
			// possible module suffixes; the Python resolver still checks ownership.
			for suffix := pythonModule; suffix != ""; {
				index.pythonSuffixes[suffix] = append(index.pythonSuffixes[suffix], file)
				separator := strings.IndexRune(suffix, filepath.Separator)
				if separator < 0 {
					break
				}
				suffix = suffix[separator+1:]
			}
		}
	}
	corpus.paths = index
	return index
}
