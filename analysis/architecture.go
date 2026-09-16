package analysis

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/sourcekind"
	"github.com/greppleai/grepple/parser"
)

// ArchitectureSchema identifies the normalized directory architecture contract.
const ArchitectureSchema = "grepple-directory-architecture-v5"

// ArchitectureReport is a complete language-neutral directory projection.
type ArchitectureReport struct {
	Schema           string                       `json:"schema"`
	Root             string                       `json:"root"`
	Files            int                          `json:"files"`
	Sources          SourceSummary                `json:"sources"`
	SourceFiles      []ArchitectureSourceFile     `json:"sourceFiles"`
	Directories      []ArchitectureDirectory      `json:"directories"`
	Symbols          []ArchitectureSymbol         `json:"symbols"`
	Relations        []ArchitectureRelation       `json:"relations"`
	RepositoryRoots  []string                     `json:"repositoryRoots,omitempty"`
	RelationCoverage ArchitectureRelationCoverage `json:"relationCoverage"`
	Truncation       *Truncation                  `json:"truncation,omitempty"`
}

// ArchitectureSourceFile records one selected source and its classification.
type ArchitectureSourceFile struct {
	Path           string `json:"path"`
	Language       string `json:"language"`
	Classification string `json:"classification"`
}

// ArchitectureDirectory aggregates source and declaration facts for one directory.
type ArchitectureDirectory struct {
	Path            string              `json:"path"`
	Files           int                 `json:"files"`
	Classifications []ArchitectureCount `json:"classifications"`
	Languages       []ArchitectureCount `json:"languages"`
	Declarations    []ArchitectureCount `json:"declarations"`
	PublicCallables int                 `json:"publicCallables"`
	Entrypoints     int                 `json:"entrypoints"`
}

// ArchitectureCount is one deterministically ordered named tally.
type ArchitectureCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ArchitectureSymbol is a source-linked declaration outline.
type ArchitectureSymbol struct {
	Name           string                      `json:"name"`
	Kind           string                      `json:"kind"`
	Language       string                      `json:"language"`
	Classification string                      `json:"classification"`
	Path           string                      `json:"path"`
	Directory      string                      `json:"directory"`
	Container      string                      `json:"container,omitempty"`
	Visibility     parser.NavigationVisibility `json:"visibility,omitempty"`
	Entrypoint     string                      `json:"entrypoint,omitempty"`
	Start          int                         `json:"startLine"`
	End            int                         `json:"endLine"`
}

// ArchitectureRelation groups source-linked cross-directory evidence.
type ArchitectureRelation struct {
	From            string                         `json:"from"`
	To              string                         `json:"to"`
	Kind            string                         `json:"kind"`
	Count           int                            `json:"count"`
	Classifications []ArchitectureCount            `json:"classifications"`
	Evidence        []ArchitectureRelationEvidence `json:"evidence"`
}

// ArchitectureRelationEvidence is one exact source-backed relation.
type ArchitectureRelationEvidence struct {
	Path           string `json:"path"`
	Line           int    `json:"line"`
	Caller         string `json:"caller"`
	Target         string `json:"target"`
	Kind           string `json:"kind"`
	Classification string `json:"classification"`
	Confidence     string `json:"confidence"`
	ImportPath     string `json:"importPath,omitempty"`
	Role           string `json:"role,omitempty"`
}

// ArchitectureRelationCoverage reports resolved, ambiguous, and unsupported relation facts.
type ArchitectureRelationCoverage struct {
	ImportFacts                int      `json:"importFacts"`
	ResolvedImports            int      `json:"resolvedImports"`
	AmbiguousImports           int      `json:"ambiguousImports"`
	UnresolvedImports          int      `json:"unresolvedImports"`
	TypeReferences             int      `json:"typeReferences"`
	ResolvedTypeReferences     int      `json:"resolvedTypeReferences"`
	AmbiguousTypeReferences    int      `json:"ambiguousTypeReferences"`
	UnresolvedTypeReferences   int      `json:"unresolvedTypeReferences"`
	UnqualifiedTypeReferences  int      `json:"unqualifiedTypeReferences"`
	UnsupportedImportLanguages []string `json:"unsupportedImportLanguages"`
}

type directoryAccumulator struct {
	files, public, entrypoints               int
	classifications, languages, declarations map[string]int
}

// BuildArchitecture projects a language-neutral physical architecture from one parsed universe.
func BuildArchitecture(universe *Universe) ArchitectureReport {
	visibility := map[string][]parser.NavigationDeclaration{}
	for _, declaration := range universe.graph.Declarations {
		key := symbolKey(declaration.Path, declaration.Name)
		visibility[key] = append(visibility[key], declaration)
	}
	directories := map[string]*directoryAccumulator{}
	symbols := []ArchitectureSymbol{}
	for _, source := range universe.sources {
		directory := cleanDirectory(filepath.Dir(source.path))
		classification := classify(source.path)
		for _, ancestor := range directoryAncestors(directory) {
			entry := getDirectory(directories, ancestor)
			entry.files++
			entry.languages[source.outline.Language]++
			entry.classifications[classification]++
		}
		appendSymbols(&symbols, directories, source.outline.Symbols, source.outline.Language, source.path, directory, "", visibility)
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Path != symbols[j].Path {
			return symbols[i].Path < symbols[j].Path
		}
		if symbols[i].Start != symbols[j].Start {
			return symbols[i].Start < symbols[j].Start
		}
		return symbols[i].Name < symbols[j].Name
	})
	relations, coverage := buildRelations(universe.graph, universe.paths)
	files := make([]ArchitectureSourceFile, 0, len(universe.paths))
	for _, path := range universe.paths {
		files = append(files, ArchitectureSourceFile{Path: filepath.ToSlash(path), Language: parser.LanguageFor(path), Classification: classify(path)})
	}
	return ArchitectureReport{Schema: ArchitectureSchema, Root: ".", Files: len(universe.paths), Sources: universe.Summary(), SourceFiles: files, Directories: finalizeDirectories(directories), Symbols: symbols, Relations: relations, RepositoryRoots: universe.graph.RepositoryRoots, RelationCoverage: coverage, Truncation: universe.Truncation()}
}

func appendSymbols(target *[]ArchitectureSymbol, directories map[string]*directoryAccumulator, symbols []parser.Symbol, language, path, directory, container string, visibility map[string][]parser.NavigationDeclaration) {
	for _, symbol := range symbols {
		item := ArchitectureSymbol{Name: symbol.Name, Kind: symbol.Kind, Language: language, Classification: classify(path), Path: filepath.ToSlash(path), Directory: directory, Container: container, Start: symbol.Start, End: symbol.End}
		item.Visibility, item.Entrypoint = architectureSymbolNavigation(visibility[symbolKey(path, symbol.Name)], symbol.Start, symbol.End)
		*target = append(*target, item)
		for _, ancestor := range directoryAncestors(directory) {
			entry := getDirectory(directories, ancestor)
			entry.declarations[item.Kind]++
			if item.Visibility == parser.NavigationVisibilityPublic {
				entry.public++
			}
			if item.Entrypoint != "" {
				entry.entrypoints++
			}
		}
		next := item.Name
		if container != "" {
			next = container + "." + item.Name
		}
		appendSymbols(target, directories, symbol.Children, language, path, directory, next, visibility)
	}
}

func architectureSymbolNavigation(candidates []parser.NavigationDeclaration, start, end int) (parser.NavigationVisibility, string) {
	bestSpan := 0
	found := false
	var visibility parser.NavigationVisibility
	entrypoint := ""
	for _, candidate := range candidates {
		if start > candidate.End || candidate.Start > end {
			continue
		}
		span := candidate.End - candidate.Start
		if !found || span < bestSpan {
			visibility = candidate.Visibility
			entrypoint = candidate.Entrypoint
			bestSpan = span
			found = true
		}
	}
	return visibility, entrypoint
}

//revive:disable-next-line:cognitive-complexity
func buildRelations(graph parser.NavigationGraph, paths []string) ([]ArchitectureRelation, ArchitectureRelationCoverage) {
	declarations := map[string]parser.NavigationDeclaration{}
	packageFiles := map[string][]string{}
	for _, declaration := range graph.Declarations {
		declarations[declaration.ID] = declaration
		if declaration.PackageID != "" {
			packageFiles[declaration.PackageID] = append(packageFiles[declaration.PackageID], declaration.Path)
		}
	}
	grouped := map[string]*ArchitectureRelation{}
	seen := map[string]bool{}
	for _, call := range graph.Calls {
		caller, callerOK := declarations[call.CallerID]
		target, targetOK := declarations[call.TargetID]
		if !callerOK || !targetOK || !strongConfidence(call.Confidence) {
			continue
		}
		addRelation(grouped, seen, cleanDirectory(filepath.Dir(caller.Path)), cleanDirectory(filepath.Dir(target.Path)), ArchitectureRelationEvidence{Path: call.Path, Line: call.Line, Caller: caller.Name, Target: target.Name, Kind: "resolved-call", Classification: classify(call.Path), Confidence: call.Confidence})
	}
	coverage := ArchitectureRelationCoverage{UnsupportedImportLanguages: unsupportedImportLanguages(paths)}
	importTargets := map[string][]string{}
	counted := map[string]bool{}
	for _, item := range graph.Imports {
		key := item.Path + "\x00" + item.ImportPath
		importTargets[key] = append(importTargets[key], item.TargetPaths...)
		statement := fmt.Sprintf("%s\x00%d\x00%s", item.Path, item.Line, item.ImportPath)
		if counted[statement] {
			continue
		}
		counted[statement] = true
		coverage.ImportFacts++
		targets := targetDirectories(item.TargetPaths)
		switch len(targets) {
		case 0:
			coverage.UnresolvedImports++
		case 1:
			coverage.ResolvedImports++
			addRelation(grouped, seen, cleanDirectory(filepath.Dir(item.Path)), targets[0], ArchitectureRelationEvidence{Path: item.Path, Line: item.Line, Caller: item.Alias, Target: item.ImportPath, Kind: "import", Classification: classify(item.Path), Confidence: "local-import-resolved", ImportPath: item.ImportPath})
		default:
			coverage.AmbiguousImports++
		}
	}
	usages := append([]parser.NavigationTypeUsage(nil), graph.TypeUsages...)
	for _, field := range graph.Fields {
		usages = append(usages, parser.NavigationTypeUsage{CallerID: field.OwnerType + "." + field.Name, Type: field.Type, ImportPath: field.ImportPath, Role: "field", Language: field.Language, Path: field.Path, Line: field.Line})
	}
	for _, usage := range usages {
		coverage.TypeReferences++
		if usage.ImportPath == "" {
			coverage.UnqualifiedTypeReferences++
			continue
		}
		targets := append([]string(nil), packageFiles[usage.ImportPath]...)
		targets = append(targets, importTargets[usage.Path+"\x00"+usage.ImportPath]...)
		directories := targetDirectories(targets)
		switch len(directories) {
		case 0:
			coverage.UnresolvedTypeReferences++
		case 1:
			coverage.ResolvedTypeReferences++
			caller := declarations[usage.CallerID].Name
			if caller == "" {
				caller = usage.CallerID
			}
			addRelation(grouped, seen, cleanDirectory(filepath.Dir(usage.Path)), directories[0], ArchitectureRelationEvidence{Path: usage.Path, Line: usage.Line, Caller: caller, Target: usage.Type, Kind: "type-reference", Classification: classify(usage.Path), Confidence: "local-import-resolved", ImportPath: usage.ImportPath, Role: usage.Role})
		default:
			coverage.AmbiguousTypeReferences++
		}
	}
	return finalizeRelations(grouped), coverage
}

func addRelation(grouped map[string]*ArchitectureRelation, seen map[string]bool, from, to string, evidence ArchitectureRelationEvidence) {
	if from == "" || to == "" || from == to {
		return
	}
	key := from + "\x00" + to + "\x00" + evidence.Kind
	evidenceKey := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s", key, evidence.Path, evidence.Line, evidence.Caller, evidence.Target, evidence.Role)
	if seen[evidenceKey] {
		return
	}
	seen[evidenceKey] = true
	relation := grouped[key]
	if relation == nil {
		relation = &ArchitectureRelation{From: from, To: to, Kind: evidence.Kind, Evidence: []ArchitectureRelationEvidence{}}
		grouped[key] = relation
	}
	relation.Count++
	relation.Evidence = append(relation.Evidence, evidence)
}

func finalizeRelations(grouped map[string]*ArchitectureRelation) []ArchitectureRelation {
	result := make([]ArchitectureRelation, 0, len(grouped))
	for _, relation := range grouped {
		classifications := map[string]int{}
		for _, evidence := range relation.Evidence {
			classifications[evidence.Classification]++
		}
		relation.Classifications = counts(classifications)
		sort.Slice(relation.Evidence, func(i, j int) bool {
			if relation.Evidence[i].Path != relation.Evidence[j].Path {
				return relation.Evidence[i].Path < relation.Evidence[j].Path
			}
			if relation.Evidence[i].Line != relation.Evidence[j].Line {
				return relation.Evidence[i].Line < relation.Evidence[j].Line
			}
			return relation.Evidence[i].Target < relation.Evidence[j].Target
		})
		result = append(result, *relation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].From != result[j].From {
			return result[i].From < result[j].From
		}
		if result[i].To != result[j].To {
			return result[i].To < result[j].To
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

func finalizeDirectories(index map[string]*directoryAccumulator) []ArchitectureDirectory {
	paths := make([]string, 0, len(index))
	for path := range index {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]ArchitectureDirectory, 0, len(paths))
	for _, path := range paths {
		entry := index[path]
		result = append(result, ArchitectureDirectory{Path: path, Files: entry.files, Classifications: counts(entry.classifications), Languages: counts(entry.languages), Declarations: counts(entry.declarations), PublicCallables: entry.public, Entrypoints: entry.entrypoints})
	}
	return result
}

func counts(values map[string]int) []ArchitectureCount {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]ArchitectureCount, 0, len(names))
	for _, name := range names {
		result = append(result, ArchitectureCount{Name: name, Count: values[name]})
	}
	return result
}
func getDirectory(index map[string]*directoryAccumulator, path string) *directoryAccumulator {
	entry := index[path]
	if entry == nil {
		entry = &directoryAccumulator{classifications: map[string]int{}, languages: map[string]int{}, declarations: map[string]int{}}
		index[path] = entry
	}
	return entry
}
func directoryAncestors(directory string) []string {
	directory = cleanDirectory(directory)
	if directory == "." {
		return []string{"."}
	}
	parts := strings.Split(directory, "/")
	result := []string{"."}
	for i := range parts {
		result = append(result, strings.Join(parts[:i+1], "/"))
	}
	return result
}
func cleanDirectory(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "" {
		return "."
	}
	return clean
}
func symbolKey(path, name string) string {
	return filepath.ToSlash(filepath.Clean(path)) + "\x00" + name
}
func classify(path string) string { return string(sourcekind.Classify(path, ".")) }
func strongConfidence(value string) bool {
	return value == "exact" || value == "import-resolved" || value == "context-resolved"
}
func targetDirectories(paths []string) []string {
	values := make([]string, 0, len(paths))
	for _, path := range paths {
		values = append(values, cleanDirectory(filepath.Dir(path)))
	}
	sort.Strings(values)
	return compactStrings(values)
}
func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
func unsupportedImportLanguages(paths []string) []string {
	set := map[string]bool{}
	for _, path := range paths {
		language := parser.LanguageFor(path)
		capability, ok := parser.CapabilitiesForLanguage(language)
		if ok && capability.Navigation && !capability.ImportNavigation {
			set[language] = true
		}
	}
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
