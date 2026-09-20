package architecture

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"reflect"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
)

const architectureComparisonSchema = "grepple-directory-architecture-comparison-v1"

type architectureCompareArgs struct {
	JSON           bool   `arg:"--json" help:"emit the complete normalized comparison as JSON"`
	Compact        bool   `arg:"--compact" help:"emit the first source-linked difference"`
	MaxOutputBytes int    `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Before         string `arg:"positional,required" placeholder:"BEFORE.json" help:"earlier directory architecture JSON"`
	After          string `arg:"positional,required" placeholder:"AFTER.json" help:"later directory architecture JSON"`
}

func (architectureCompareArgs) Description() string {
	return "Normalize and compare complete directory architecture reports before diagnosing byte drift. Exactly one of --json or --compact is required."
}

type architectureComparison struct {
	Schema        string                  `json:"schema"`
	Before        string                  `json:"before"`
	After         string                  `json:"after"`
	SemanticEqual bool                    `json:"semanticEqual"`
	ByteEqual     bool                    `json:"byteEqual"`
	Difference    *architectureDifference `json:"difference,omitempty"`
}

type architectureDifference struct {
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

func runArchitectureCompare(args []string, dependencies Dependencies) error {
	values := architectureCompareArgs{MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture compare", args, &values); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	if values.JSON == values.Compact {
		return fmt.Errorf("architecture compare requires exactly one of --json or --compact")
	}
	if values.MaxOutputBytes < 0 {
		return fmt.Errorf("architecture compare limits must be non-negative")
	}
	before, beforeBytes, err := readDirectoryArchitecture(values.Before)
	if err != nil {
		return fmt.Errorf("read before architecture: %w", err)
	}
	after, afterBytes, err := readDirectoryArchitecture(values.After)
	if err != nil {
		return fmt.Errorf("read after architecture: %w", err)
	}
	comparison := compareDirectoryArchitectures(values.Before, values.After, beforeBytes, afterBytes, before, after)
	if values.JSON {
		err = stdoutWriter().writeJSON(comparison)
	} else {
		err = renderArchitectureComparison(comparison, values.MaxOutputBytes)
	}
	if err != nil {
		return err
	}
	if !comparison.SemanticEqual || !comparison.ByteEqual {
		dependencies.requestExit(1)
	}
	return nil
}

func readDirectoryArchitecture(filePath string) (Report, []byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return Report{}, nil, err
	}
	content, err = unwrapRemoteDirectoryArchitecture(content)
	if err != nil {
		return Report{}, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var architecture Report
	if err := decoder.Decode(&architecture); err != nil {
		return directoryArchitecture{}, nil, err
	}
	if err := requireArchitectureJSONEnd(decoder); err != nil {
		return directoryArchitecture{}, nil, err
	}
	if architecture.Schema != directoryArchitectureSchema {
		return directoryArchitecture{}, nil, fmt.Errorf("schema %q is unsupported; expected %q", architecture.Schema, directoryArchitectureSchema)
	}
	if len(architecture.SourceFiles) != architecture.Files {
		return directoryArchitecture{}, nil, fmt.Errorf("sourceFiles has %d entries; files reports %d", len(architecture.SourceFiles), architecture.Files)
	}
	return architecture, content, nil
}

func unwrapRemoteDirectoryArchitecture(content []byte) ([]byte, error) {
	var header struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(content, &header); err != nil || header.Schema != "grepple-remote-analysis-v1" {
		return content, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var response api.AnalysisResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	if err := requireArchitectureJSONEnd(decoder); err != nil {
		return nil, err
	}
	if response.Operation != api.AnalysisArchitecture {
		return nil, fmt.Errorf("remote analysis operation %q is unsupported; expected %q", response.Operation, api.AnalysisArchitecture)
	}
	return response.Result, nil
}

func requireArchitectureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("multiple JSON values are not supported")
}

func compareDirectoryArchitectures(beforePath, afterPath string, beforeBytes, afterBytes []byte, before, after directoryArchitecture) architectureComparison {
	before = normalizeDirectoryArchitecture(before)
	after = normalizeDirectoryArchitecture(after)
	difference := firstArchitectureDifference(before, after)
	semanticEqual := difference == nil
	byteEqual := bytes.Equal(beforeBytes, afterBytes)
	if semanticEqual && !byteEqual {
		difference = architectureEncodingDifference(beforeBytes, afterBytes)
	}
	return architectureComparison{
		Schema:        architectureComparisonSchema,
		Before:        beforePath,
		After:         afterPath,
		SemanticEqual: semanticEqual,
		ByteEqual:     byteEqual,
		Difference:    difference,
	}
}

func normalizeDirectoryArchitecture(value directoryArchitecture) directoryArchitecture {
	value.Root = normalizeArchitecturePath(value.Root)
	value.SourceFiles = append([]architectureSourceFile{}, value.SourceFiles...)
	for index := range value.SourceFiles {
		value.SourceFiles[index].Path = normalizeArchitecturePath(value.SourceFiles[index].Path)
	}
	sort.Slice(value.SourceFiles, func(i, j int) bool {
		return architectureSourceFileSortKey(value.SourceFiles[i]) < architectureSourceFileSortKey(value.SourceFiles[j])
	})

	value.Directories = append([]architectureDirectory{}, value.Directories...)
	for index := range value.Directories {
		directory := &value.Directories[index]
		directory.Path = normalizeArchitecturePath(directory.Path)
		directory.Classifications = normalizeArchitectureCounts(directory.Classifications)
		directory.Languages = normalizeArchitectureCounts(directory.Languages)
		directory.Declarations = normalizeArchitectureCounts(directory.Declarations)
	}
	sort.Slice(value.Directories, func(i, j int) bool { return value.Directories[i].Path < value.Directories[j].Path })

	value.Symbols = append([]architectureSymbol{}, value.Symbols...)
	for index := range value.Symbols {
		value.Symbols[index].Path = normalizeArchitecturePath(value.Symbols[index].Path)
		value.Symbols[index].Directory = normalizeArchitecturePath(value.Symbols[index].Directory)
	}
	sort.Slice(value.Symbols, func(i, j int) bool {
		return architectureSymbolSortKey(value.Symbols[i]) < architectureSymbolSortKey(value.Symbols[j])
	})

	value.Relations = append([]architectureRelation{}, value.Relations...)
	for index := range value.Relations {
		relation := &value.Relations[index]
		relation.From = normalizeArchitecturePath(relation.From)
		relation.To = normalizeArchitecturePath(relation.To)
		relation.Classifications = normalizeArchitectureCounts(relation.Classifications)
		relation.Evidence = append([]architectureRelationEvidence{}, relation.Evidence...)
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

func normalizeArchitectureCounts(values []architectureCount) []architectureCount {
	result := append([]architectureCount{}, values...)
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

func firstArchitectureDifference(before, after directoryArchitecture) *architectureDifference {
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

func architectureSourceFileFacts(values []architectureSourceFile) []architectureFact {
	facts := make([]architectureFact, 0, len(values))
	for _, value := range values {
		facts = append(facts, architectureFact{kind: "file", key: architectureSourceFileKey(value), identity: value.Path, path: value.Path, value: value})
	}
	return facts
}

func architectureDirectoryFacts(values []architectureDirectory) []architectureFact {
	facts := make([]architectureFact, 0, len(values))
	for _, value := range values {
		facts = append(facts, architectureFact{kind: "directory", key: value.Path, identity: value.Path, path: value.Path, value: value})
	}
	return facts
}

func architectureSymbolFacts(values []architectureSymbol) []architectureFact {
	facts := make([]architectureFact, 0, len(values))
	for _, value := range values {
		identity := strings.Join([]string{value.Path, value.Language, value.Kind, value.Container, value.Name}, "|")
		facts = append(facts, architectureFact{kind: "declaration", key: architectureSymbolComparisonKey(value), identity: identity, path: value.Path, start: value.Start, end: value.End, value: value})
	}
	return facts
}

func architectureRelationFacts(values []architectureRelation) []architectureFact {
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

func architectureMetadataFacts(value directoryArchitecture) []architectureFact {
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

func firstArchitectureFactDifference(before, after []architectureFact) *architectureDifference {
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

func newArchitectureDifference(change string, before, after *architectureFact) *architectureDifference {
	fact := before
	if fact == nil {
		fact = after
	}
	difference := &architectureDifference{Kind: fact.kind, Change: change, Identity: fact.identity, Path: fact.path, StartLine: fact.start, EndLine: fact.end}
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
func architectureEncodingDifference(before, after []byte) *architectureDifference {
	offset := 0
	for offset < len(before) && offset < len(after) && before[offset] == after[offset] {
		offset++
	}
	line, column := 1, 1
	for _, value := range before[:min(offset, len(before))] {
		if value == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
	}
	difference := &architectureDifference{Kind: "encoding", Change: "changed", Identity: "json-bytes", ByteOffset: &offset, ByteLine: line, ByteColumn: column}
	if offset < len(before) {
		value := int(before[offset])
		difference.BeforeByte = &value
	}
	if offset < len(after) {
		value := int(after[offset])
		difference.AfterByte = &value
	}
	return difference
}

func architectureSourceFileKey(value architectureSourceFile) string {
	return value.Path
}

func architectureSourceFileSortKey(value architectureSourceFile) string {
	return strings.Join([]string{value.Path, value.Language, value.Classification}, "|")
}

func architectureSymbolComparisonKey(value architectureSymbol) string {
	return architectureStringKey(value.Path, value.Language, value.Kind, value.Container, value.Name)
}

func architectureSymbolSortKey(value architectureSymbol) string {
	return fmt.Sprintf("%s|%09d|%09d|%s|%s|%s|%s", architectureSymbolComparisonKey(value), value.Start, value.End, value.Classification, value.Directory, value.Visibility, value.Entrypoint)
}

func architectureRelationComparisonKey(value architectureRelation) string {
	return architectureStringKey(value.From, value.To, value.Kind)
}

func architectureRelationSortKey(value architectureRelation) string {
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
func architectureEvidenceComparisonKey(value architectureRelationEvidence) string {
	return architectureStringKey(value.Path, fmt.Sprintf("%09d", value.Line), value.Caller, value.Target, value.Kind, value.Classification, value.Confidence, value.ImportPath, value.Role)
}

func renderArchitectureComparison(comparison architectureComparison, maxBytes int) error {
	writer := architectureOutputWriter(maxBytes)
	if err := writer.writeString(fmt.Sprintf("architecture-compare %s semantic-equal=%t byte-equal=%t before=%s after=%s\n", comparison.Schema, comparison.SemanticEqual, comparison.ByteEqual, comparison.Before, comparison.After)); err != nil {
		return nil
	}
	if comparison.Difference == nil {
		return nil
	}
	difference := comparison.Difference
	if difference.Kind == "encoding" {
		return renderArchitectureEncodingDifference(writer, difference)
	}
	if err := writer.writeString(fmt.Sprintf("! %s %s identity=%s at=%s\n", difference.Kind, difference.Change, difference.Identity, architectureDifferenceLocation(difference))); err != nil {
		return nil
	}
	return renderArchitectureDifferenceValues(writer, difference)
}

func renderArchitectureEncodingDifference(writer *outputWriter, difference *architectureDifference) error {
	return writer.writeString(fmt.Sprintf("! encoding changed at byte=%d line=%d column=%d before-byte=%s after-byte=%s\n", *difference.ByteOffset, difference.ByteLine, difference.ByteColumn, formatArchitectureByte(difference.BeforeByte), formatArchitectureByte(difference.AfterByte)))
}

func architectureDifferenceLocation(difference *architectureDifference) string {
	if difference.Path == "" {
		return "unknown"
	}
	location := difference.Path
	if difference.StartLine > 0 {
		location = fmt.Sprintf("%s:%d", location, difference.StartLine)
		if difference.EndLine > difference.StartLine {
			location = fmt.Sprintf("%s-%d", location, difference.EndLine)
		}
	}
	return location
}

func renderArchitectureDifferenceValues(writer *outputWriter, difference *architectureDifference) error {
	if len(difference.Before) > 0 {
		if err := writer.writeString("before=" + string(difference.Before) + "\n"); err != nil {
			return nil
		}
	}
	if len(difference.After) > 0 {
		if err := writer.writeString("after=" + string(difference.After) + "\n"); err != nil {
			return nil
		}
	}
	return nil
}
func formatArchitectureByte(value *int) string {
	if value == nil {
		return "eof"
	}
	return fmt.Sprintf("0x%02x", *value)
}
