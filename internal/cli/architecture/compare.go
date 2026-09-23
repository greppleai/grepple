package architecture

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/greppleai/grepple/analysis"
	"github.com/greppleai/grepple/api"
)

const architectureComparisonSchema = "grepple-directory-architecture-comparison-v1"

type architectureCompareArgs struct {
	JSON           bool   `arg:"--json" help:"emit the complete normalized comparison as JSON"`
	MaxOutputBytes int    `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited)"`
	Before         string `arg:"positional,required" placeholder:"BEFORE.json" help:"earlier directory architecture JSON"`
	After          string `arg:"positional,required" placeholder:"AFTER.json" help:"later directory architecture JSON"`
}

func (architectureCompareArgs) Description() string {
	return "Normalize and compare complete directory architecture reports before diagnosing byte drift. Human output is the default; --json emits the complete comparison."
}

type architectureComparison struct {
	Schema        string                  `json:"schema"`
	Before        string                  `json:"before"`
	After         string                  `json:"after"`
	SemanticEqual bool                    `json:"semanticEqual"`
	ByteEqual     bool                    `json:"byteEqual"`
	Difference    *architectureDifference `json:"difference,omitempty"`
}

type architectureDifference = analysis.ArchitectureDifference

func runArchitectureCompare(args []string, dependencies Dependencies) error {
	values := architectureCompareArgs{MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture compare", args, &values, dependencies.Stdout); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	return executeArchitectureCompare(&values, dependencies)
}

func executeArchitectureCompare(values *CompareArgs, dependencies Dependencies) error {
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
		err = stdoutWriter(dependencies).writeJSON(comparison)
	} else {
		err = renderArchitectureComparison(comparison, values.MaxOutputBytes, dependencies)
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
	difference := analysis.CompareArchitectures(before, after)
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

func renderArchitectureComparison(comparison architectureComparison, maxBytes int, dependencies Dependencies) error {
	writer := architectureOutputWriter(maxBytes, dependencies)
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
