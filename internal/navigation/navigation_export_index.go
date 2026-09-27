package navigation

import (
	"sort"

	"github.com/greppleai/grepple/internal/parser"
)

// navigationExportIndex indexes the complete corpus exports by language and
// import name. Entries point into the original slice to preserve its order.
type navigationExportIndex struct {
	exports     []parser.NavigationExport
	byPackage   map[string][]int
	byQualified map[string][]int
}

func (corpus *navigationCorpus) exportIndex() *navigationExportIndex {
	if corpus.exportLookup != nil {
		return corpus.exportLookup
	}
	index := &navigationExportIndex{
		exports:     corpus.graph.Exports,
		byPackage:   make(map[string][]int),
		byQualified: make(map[string][]int),
	}
	for offset, item := range corpus.graph.Exports {
		family := navigationLanguageFamily(item.Language) + "\x00"
		index.byPackage[family+item.ImportPath] = append(index.byPackage[family+item.ImportPath], offset)
		qualified := family + navigationQualifiedExportName(item.ImportPath, item.Name)
		index.byQualified[qualified] = append(index.byQualified[qualified], offset)
	}
	corpus.exportLookup = index
	return index
}

func (index *navigationExportIndex) targetFiles(family, importPath string, includePackage, includeQualified bool) []string {
	key := family + "\x00" + importPath
	var offsets []int
	if includePackage {
		offsets = append(offsets, index.byPackage[key]...)
	}
	if includeQualified {
		offsets = append(offsets, index.byQualified[key]...)
	}
	if includePackage && includeQualified {
		sort.Ints(offsets)
	}
	files := make([]string, 0, len(offsets))
	for _, offset := range offsets {
		if len(files) == 0 || files[len(files)-1] != index.exports[offset].Path {
			files = append(files, index.exports[offset].Path)
		}
	}
	return files
}
