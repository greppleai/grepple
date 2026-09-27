package analysis

import (
	"encoding/json"
	"fmt"
	pathpkg "path"
	"reflect"
	"sort"
	"strings"
)

// ArchitectureDifference describes the first deterministic difference between
// two normalized architecture reports. Byte fields are reserved for transport
// adapters that also compare snapshot encodings.
type ArchitectureDifference struct {
	Kind       string          `json:"kind"`
	Change     string          `json:"change"`
	Identity   string          `json:"identity"`
	Path       string          `json:"path,omitempty"`
	StartLine  int             `json:"startLine,omitempty"`
	EndLine    int             `json:"endLine,omitempty"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	ByteOffset *int            `json:"byteOffset,omitempty"`
	BeforeByte *int            `json:"beforeByte,omitempty"`
	AfterByte  *int            `json:"afterByte,omitempty"`
	ByteLine   int             `json:"byteLine,omitempty"`
	ByteColumn int             `json:"byteColumn,omitempty"`
}

type architectureFact struct {
	kind     string
	key      string
	identity string
	path     string
	start    int
	end      int
	value    any
}

// CompareArchitectures returns the first semantic difference after canonical
// path, ordering, count, evidence, and repository-root normalization.
func CompareArchitectures(before, after ArchitectureReport) *ArchitectureDifference {
	before = NormalizeArchitectureReport(before)
	after = NormalizeArchitectureReport(after)
	groups := [][2][]architectureFact{
		{architectureSourceFileFacts(before.SourceFiles), architectureSourceFileFacts(after.SourceFiles)},
		{architectureSymbolFacts(before.Symbols), architectureSymbolFacts(after.Symbols)},
		{architectureRelationFacts(before.Relations), architectureRelationFacts(after.Relations)},
		{architectureDirectoryFacts(before.Directories), architectureDirectoryFacts(after.Directories)},
		{architectureMetadataFacts(before), architectureMetadataFacts(after)},
	}
	for _, group := range groups {
		if difference := firstArchitectureFactDifference(group[0], group[1]); difference != nil {
			return difference
		}
	}
	return nil
}

// NormalizeArchitectureReport returns a deterministic comparison projection
// without mutating the supplied report.
func NormalizeArchitectureReport(value ArchitectureReport) ArchitectureReport {
	value.Root = normalizeArchitecturePath(value.Root)
	value.SourceFiles = append([]ArchitectureSourceFile{}, value.SourceFiles...)
	for index := range value.SourceFiles {
		value.SourceFiles[index].Path = normalizeArchitecturePath(value.SourceFiles[index].Path)
	}
	sort.Slice(value.SourceFiles, func(i, j int) bool {
		return architectureSourceFileSortKey(value.SourceFiles[i]) < architectureSourceFileSortKey(value.SourceFiles[j])
	})

	value.Directories = append([]ArchitectureDirectory{}, value.Directories...)
	for index := range value.Directories {
		directory := &value.Directories[index]
		directory.Path = normalizeArchitecturePath(directory.Path)
		directory.Classifications = normalizeArchitectureCounts(directory.Classifications)
		directory.Languages = normalizeArchitectureCounts(directory.Languages)
		directory.Declarations = normalizeArchitectureCounts(directory.Declarations)
	}
	sort.Slice(value.Directories, func(i, j int) bool { return value.Directories[i].Path < value.Directories[j].Path })

	value.Symbols = append([]ArchitectureSymbol{}, value.Symbols...)
	for index := range value.Symbols {
		value.Symbols[index].Path = normalizeArchitecturePath(value.Symbols[index].Path)
		value.Symbols[index].Directory = normalizeArchitecturePath(value.Symbols[index].Directory)
	}
	sort.Slice(value.Symbols, func(i, j int) bool {
		return architectureSymbolSortKey(value.Symbols[i]) < architectureSymbolSortKey(value.Symbols[j])
	})

	value.Relations = append([]ArchitectureRelation{}, value.Relations...)
	for index := range value.Relations {
		relation := &value.Relations[index]
		relation.From = normalizeArchitecturePath(relation.From)
		relation.To = normalizeArchitecturePath(relation.To)
		relation.Classifications = normalizeArchitectureCounts(relation.Classifications)
		relation.Evidence = append([]ArchitectureRelationEvidence{}, relation.Evidence...)
		for evidenceIndex := range relation.Evidence {
			relation.Evidence[evidenceIndex].Path = normalizeArchitecturePath(relation.Evidence[evidenceIndex].Path)
		}
		sort.Slice(relation.Evidence, func(i, j int) bool {
			return architectureEvidenceComparisonKey(relation.Evidence[i]) < architectureEvidenceComparisonKey(relation.Evidence[j])
		})
	}
	sort.Slice(value.Relations, func(i, j int) bool {
		return architectureRelationSortKey(value.Relations[i]) < architectureRelationSortKey(value.Relations[j])
	})
	value.RepositoryRoots = append([]string{}, value.RepositoryRoots...)
	sort.Strings(value.RepositoryRoots)
	value.RelationCoverage.UnsupportedImportLanguages = append([]string{}, value.RelationCoverage.UnsupportedImportLanguages...)
	sort.Strings(value.RelationCoverage.UnsupportedImportLanguages)
	return value
}

func normalizeArchitectureCounts(values []ArchitectureCount) []ArchitectureCount {
	result := append([]ArchitectureCount{}, values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].Count < result[j].Count
	})
	return result
}

func normalizeArchitecturePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	if value == "" {
		return "."
	}
	clean := pathpkg.Clean(value)
	if clean == "" || clean == "./" {
		return "."
	}
	return strings.TrimPrefix(clean, "./")
}

func architectureSourceFileFacts(values []ArchitectureSourceFile) []architectureFact {
	facts := make([]architectureFact, 0, len(values))
	for _, value := range values {
		facts = append(facts, architectureFact{kind: "file", key: value.Path, identity: value.Path, path: value.Path, value: value})
	}
	return facts
}

func architectureDirectoryFacts(values []ArchitectureDirectory) []architectureFact {
	facts := make([]architectureFact, 0, len(values))
	for _, value := range values {
		facts = append(facts, architectureFact{kind: "directory", key: value.Path, identity: value.Path, path: value.Path, value: value})
	}
	return facts
}

func architectureSymbolFacts(values []ArchitectureSymbol) []architectureFact {
	facts := make([]architectureFact, 0, len(values))
	for _, value := range values {
		identity := strings.Join([]string{value.Path, value.Language, value.Kind, value.Container, value.Name}, "|")
		facts = append(facts, architectureFact{kind: "declaration", key: architectureSymbolComparisonKey(value), identity: identity, path: value.Path, start: value.Start, end: value.End, value: value})
	}
	return facts
}

func architectureRelationFacts(values []ArchitectureRelation) []architectureFact {
	facts := make([]architectureFact, 0, len(values))
	for _, value := range values {
		path, line := "", 0
		if len(value.Evidence) > 0 {
			path, line = value.Evidence[0].Path, value.Evidence[0].Line
		}
		identity := strings.Join([]string{value.From, value.To, value.Kind}, "|")
		facts = append(facts, architectureFact{kind: "relation", key: architectureRelationComparisonKey(value), identity: identity, path: path, start: line, end: line, value: value})
	}
	return facts
}

func architectureMetadataFacts(value ArchitectureReport) []architectureFact {
	return []architectureFact{
		{kind: "metadata", key: "files", identity: "files", value: value.Files},
		{kind: "metadata", key: "relationCoverage", identity: "relationCoverage", value: value.RelationCoverage},
		{kind: "metadata", key: "repositoryRoots", identity: "repositoryRoots", value: value.RepositoryRoots},
		{kind: "metadata", key: "root", identity: "root", path: value.Root, value: value.Root},
		{kind: "metadata", key: "schema", identity: "schema", value: value.Schema},
		{kind: "metadata", key: "sources", identity: "sources", value: value.Sources},
		{kind: "metadata", key: "truncation", identity: "truncation", value: value.Truncation},
	}
}

func firstArchitectureFactDifference(before, after []architectureFact) *ArchitectureDifference {
	before = ordinalArchitectureFacts(before)
	after = ordinalArchitectureFacts(after)
	for beforeIndex, afterIndex := 0, 0; beforeIndex < len(before) || afterIndex < len(after); {
		if beforeIndex == len(before) {
			return newArchitectureDifference("added", nil, &after[afterIndex])
		}
		if afterIndex == len(after) {
			return newArchitectureDifference("removed", &before[beforeIndex], nil)
		}
		beforeFact, afterFact := before[beforeIndex], after[afterIndex]
		switch {
		case beforeFact.key < afterFact.key:
			return newArchitectureDifference("removed", &beforeFact, nil)
		case beforeFact.key > afterFact.key:
			return newArchitectureDifference("added", nil, &afterFact)
		case !reflect.DeepEqual(beforeFact.value, afterFact.value):
			return newArchitectureDifference("changed", &beforeFact, &afterFact)
		default:
			beforeIndex++
			afterIndex++
		}
	}
	return nil
}

func ordinalArchitectureFacts(values []architectureFact) []architectureFact {
	result := append([]architectureFact{}, values...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].key < result[j].key })
	occurrences := make(map[string]int)
	for index := range result {
		base := result[index].key
		result[index].key = fmt.Sprintf("%s#%06d", base, occurrences[base])
		occurrences[base]++
	}
	return result
}

func newArchitectureDifference(change string, before, after *architectureFact) *ArchitectureDifference {
	fact := before
	if fact == nil {
		fact = after
	}
	difference := &ArchitectureDifference{Kind: fact.kind, Change: change, Identity: fact.identity, Path: fact.path, StartLine: fact.start, EndLine: fact.end}
	if before != nil {
		difference.Before = marshalArchitectureDifferenceValue(before.value)
	}
	if after != nil {
		difference.After = marshalArchitectureDifferenceValue(after.value)
		if difference.Path == "" {
			difference.Path, difference.StartLine, difference.EndLine = after.path, after.start, after.end
		}
	}
	return difference
}

func marshalArchitectureDifferenceValue(value any) json.RawMessage {
	content, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal architecture comparison value: %v", err))
	}
	return content
}

func architectureSourceFileSortKey(value ArchitectureSourceFile) string {
	return strings.Join([]string{value.Path, value.Language, value.Classification}, "|")
}

func architectureSymbolComparisonKey(value ArchitectureSymbol) string {
	return architectureStringKey(value.Path, value.Language, value.Kind, value.Container, value.Name)
}

func architectureSymbolSortKey(value ArchitectureSymbol) string {
	return fmt.Sprintf("%s|%09d|%09d|%s|%s|%s|%s", architectureSymbolComparisonKey(value), value.Start, value.End, value.Classification, value.Directory, value.Visibility, value.Entrypoint)
}

func architectureRelationComparisonKey(value ArchitectureRelation) string {
	return architectureStringKey(value.From, value.To, value.Kind)
}

func architectureRelationSortKey(value ArchitectureRelation) string {
	var result strings.Builder
	fmt.Fprintf(&result, "%s|%09d", architectureRelationComparisonKey(value), value.Count)
	for _, classification := range value.Classifications {
		fmt.Fprintf(&result, "|%s:%09d", classification.Name, classification.Count)
	}
	for _, evidence := range value.Evidence {
		result.WriteByte('|')
		result.WriteString(architectureEvidenceComparisonKey(evidence))
	}
	return result.String()
}

func architectureStringKey(values ...string) string {
	content, err := json.Marshal(values)
	if err != nil {
		panic(fmt.Sprintf("marshal architecture comparison key: %v", err))
	}
	return string(content)
}

func architectureEvidenceComparisonKey(value ArchitectureRelationEvidence) string {
	return architectureStringKey(value.Path, fmt.Sprintf("%09d", value.Line), value.Caller, value.Target, value.Kind, value.Classification, value.Confidence, value.ImportPath, value.Role)
}
