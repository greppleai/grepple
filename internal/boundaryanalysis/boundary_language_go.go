package boundaryanalysis

import (
	"strings"

	"github.com/greppleai/grepple/parser"
)

type goBoundaryLanguagePolicy struct{ baseBoundaryLanguagePolicy }

func (goBoundaryLanguagePolicy) standardLibraryImport(path string) bool {
	first := boundaryFirstImportComponent(path)
	switch first {
	case "archive", "arena", "bufio", "builtin", "bytes", "cmp", "compress", "container", "context", "crypto", "database", "debug", "embed", "encoding", "errors", "expvar", "flag", "fmt", "go", "hash", "html", "image", "index", "io", "iter", "log", "maps", "math", "mime", "net", "os", "path", "plugin", "reflect", "regexp", "runtime", "slices", "sort", "strconv", "strings", "structs", "sync", "syscall", "testing", "text", "time", "unicode", "unique", "unsafe", "weak":
		return true
	default:
		return false
	}
}

func (goBoundaryLanguagePolicy) thirdPartyImport(path string) bool {
	return strings.Contains(boundaryFirstImportComponent(path), ".")
}

func (goBoundaryLanguagePolicy) publicTypeUsage(declaration parser.NavigationDeclaration, role string) bool {
	if role != "parameter" && role != "result" && role != "receiver" {
		return true
	}
	container := boundaryTerminalTypeName(declaration.Container)
	if container == "" {
		if separator := strings.IndexByte(declaration.Name, '.'); separator > 0 {
			container = boundaryTerminalTypeName(declaration.Name[:separator])
		}
	}
	return container == "" || container[0] < 'a' || container[0] > 'z'
}

func boundaryFirstImportComponent(path string) string {
	if separator := strings.IndexByte(path, '/'); separator >= 0 {
		return path[:separator]
	}
	return path
}
