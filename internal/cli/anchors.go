package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/hashline"
	"github.com/greppleai/grepple/search"
)

const (
	anchorProtocolVersion   = 1
	maxAnchorResponseBytes  = 4 << 20
	maxAnchorProviderStderr = 8 << 10
	maxAnchorLength         = 128
	anchorOutputSeparator   = "│"
)

type anchorLookup map[string]map[int]string

type anchorProtocolRequest struct {
	ProtocolVersion int                         `json:"protocol_version"`
	Files           []anchorProtocolRequestFile `json:"files"`
}

type anchorProtocolRequestFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
	Lines   []int  `json:"lines"`
}

type anchorProtocolResponse struct {
	ProtocolVersion int                          `json:"protocol_version"`
	Files           []anchorProtocolResponseFile `json:"files"`
}

type anchorProtocolResponseFile struct {
	Path    string               `json:"path"`
	SHA256  string               `json:"sha256"`
	Anchors []anchorProtocolLine `json:"anchors"`
}

type anchorProtocolLine struct {
	Line   int    `json:"line"`
	Anchor string `json:"anchor"`
}

type anchorFileSelection struct {
	displayPath string
	lines       map[int]string
}

func prepareResultAnchors(options *cliOptions, results []api.FileResult) error {
	if !options.Anchors {
		return nil
	}
	request, displayPaths, err := buildAnchorRequest(options, results)
	if err != nil {
		return err
	}
	if len(request.Files) == 0 {
		options.AnchorLines = make(anchorLookup)
		return nil
	}
	native, err := useNativeAnchorProvider()
	if err != nil {
		return err
	}
	if native {
		anchors, nativeErr := nativeAnchorLookup(request, displayPaths)
		if nativeErr != nil {
			return nativeErr
		}
		options.AnchorLines = anchors
		return nil
	}
	_, provider, err := resolveAnchorProvider("")
	if err != nil {
		return err
	}
	response, err := invokeAnchorProvider(provider, request)
	if err != nil {
		return err
	}
	anchors, err := validateAnchorResponse(request, response, displayPaths)
	if err != nil {
		return err
	}
	options.AnchorLines = anchors
	return nil
}

func useNativeAnchorProvider() (bool, error) {
	settings, err := loadUserSettings()
	if err != nil {
		return false, err
	}
	if !settings.Anchors.EnabledByDefault {
		return true, nil
	}
	return settings.Anchors.DefaultProvider == "" || settings.Anchors.DefaultProvider == "native", nil
}

func nativeAnchorLookup(request anchorProtocolRequest, displayPaths map[string]string) (anchorLookup, error) {
	lookup := make(anchorLookup, len(request.Files))
	for _, file := range request.Files {
		hashes := hashline.Lines(file.Content)
		displayPath := displayPaths[file.Path]
		lookup[displayPath] = make(map[int]string, len(file.Lines))
		for _, line := range file.Lines {
			if line < 1 || line > len(hashes) {
				return nil, fmt.Errorf("invalid anchor line %d for %s", line, displayPath)
			}
			lookup[displayPath][line] = hashes[line-1]
		}
	}
	return lookup, nil
}

func defaultReadAnchors(path, content string, lines []int) (map[int]string, bool, error) {
	settings, err := loadUserSettings()
	if err != nil {
		return nil, false, err
	}
	request := anchorProtocolRequest{ProtocolVersion: anchorProtocolVersion, Files: []anchorProtocolRequestFile{{
		Path: path, Content: content, SHA256: anchorDigest(content), Lines: lines,
	}}}
	if !settings.Anchors.EnabledByDefault || settings.Anchors.DefaultProvider == "" || settings.Anchors.DefaultProvider == "native" {
		anchors, nativeErr := nativeAnchorLookup(request, map[string]string{path: path})
		if nativeErr != nil {
			return nil, false, nativeErr
		}
		return anchors[path], true, nil
	}
	_, provider, err := resolveAnchorProvider("")
	if err != nil {
		return nil, false, err
	}
	response, err := invokeAnchorProvider(provider, request)
	if err != nil {
		return nil, false, err
	}
	anchors, err := validateAnchorResponse(request, response, map[string]string{path: path})
	if err != nil {
		return nil, false, err
	}
	return anchors[path], true, nil
}

func buildAnchorRequest(options *cliOptions, results []api.FileResult) (anchorProtocolRequest, map[string]string, error) {
	selections := collectAnchorSelections(options, results)
	request := anchorProtocolRequest{
		ProtocolVersion: anchorProtocolVersion,
		Files:           make([]anchorProtocolRequestFile, 0, len(selections)),
	}
	displayPaths := make(map[string]string, len(selections))
	for _, selection := range selections {
		file, err := anchorRequestFile(selection)
		if err != nil {
			return anchorProtocolRequest{}, nil, err
		}
		request.Files = append(request.Files, file)
		displayPaths[file.Path] = selection.displayPath
	}
	return request, displayPaths, nil
}

func collectAnchorSelections(options *cliOptions, results []api.FileResult) []anchorFileSelection {
	selections := make([]anchorFileSelection, 0, len(results))
	for _, result := range results {
		selection := anchorFileSelection{displayPath: result.Path, lines: make(map[int]string)}
		collectAnchorSelectionLines(options, result, selection.lines)
		if len(selection.lines) > 0 {
			selections = append(selections, selection)
		}
	}
	return selections
}

func collectAnchorSelectionLines(options *cliOptions, result api.FileResult, lines map[int]string) {
	if options.Params.BeforeContext > 0 || options.Params.AfterContext > 0 {
		for _, line := range result.Context {
			lines[line.Line] = normalizeRenderedAnchorLine(line.Text)
		}
		return
	}
	if options.LineOnly {
		for _, match := range result.Matches {
			lines[match.Line] = normalizeRenderedAnchorLine(match.Text)
		}
		return
	}
	for _, segment := range result.Segments {
		collectAnchorSegmentLines(lines, segment)
	}
}

func collectAnchorSegmentLines(lines map[int]string, segment api.ResultSegment) {
	if segment.Kind != "lines" {
		return
	}
	for index, line := range strings.Split(segment.Text, "\n") {
		lines[segment.Start+index] = normalizeRenderedAnchorLine(line)
	}
}

func anchorRequestFile(selection anchorFileSelection) (anchorProtocolRequestFile, error) {
	absolutePath, err := filepath.Abs(selection.displayPath)
	if err != nil {
		return anchorProtocolRequestFile{}, fmt.Errorf("resolve anchor path %s: %w", selection.displayPath, err)
	}
	contentBytes, err := os.ReadFile(absolutePath)
	if err != nil {
		return anchorProtocolRequestFile{}, fmt.Errorf("read anchor source %s: %w", selection.displayPath, err)
	}
	content, err := normalizeAnchorContent(string(contentBytes))
	if err != nil {
		return anchorProtocolRequestFile{}, fmt.Errorf("anchor source %s: %w", selection.displayPath, err)
	}
	fileLines := search.SplitLines(content)
	lineNumbers := sortedAnchorLines(selection.lines)
	for _, line := range lineNumbers {
		if line < 1 || line > len(fileLines) || fileLines[line-1] != selection.lines[line] {
			return anchorProtocolRequestFile{}, fmt.Errorf("anchor source %s changed after search; rerun Grepple", selection.displayPath)
		}
	}
	return anchorProtocolRequestFile{Path: absolutePath, Content: content, SHA256: anchorDigest(content), Lines: lineNumbers}, nil
}

func normalizeAnchorContent(content string) (string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.Contains(content, "\r") {
		return "", fmt.Errorf("bare carriage returns are not supported because anchor line numbering would be ambiguous")
	}
	return content, nil
}

func normalizeRenderedAnchorLine(line string) string {
	return strings.TrimSuffix(line, "\r")
}

func sortedAnchorLines(lines map[int]string) []int {
	result := make([]int, 0, len(lines))
	for line := range lines {
		result = append(result, line)
	}
	sort.Ints(result)
	return result
}

func anchorDigest(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func invokeAnchorProvider(provider anchorProviderSettings, request anchorProtocolRequest) (anchorProtocolResponse, error) {
	input, err := json.Marshal(request)
	if err != nil {
		return anchorProtocolResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(provider.TimeoutMS)*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, provider.Command[0], provider.Command[1:]...)
	command.Stdin = bytes.NewReader(input)
	var stdout limitedAnchorBuffer
	stdout.limit = maxAnchorResponseBytes
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.MultiWriter(&limitedStderrWriter{buffer: &stderr, limit: maxAnchorProviderStderr})
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return anchorProtocolResponse{}, fmt.Errorf("anchor provider timed out after %dms", provider.TimeoutMS)
		}
		return anchorProtocolResponse{}, fmt.Errorf("anchor provider failed: %w%s", err, anchorStderrSuffix(stderr.String()))
	}
	var response anchorProtocolResponse
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return anchorProtocolResponse{}, fmt.Errorf("parse anchor provider response: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return anchorProtocolResponse{}, fmt.Errorf("parse anchor provider response: %w", err)
	}
	return response, nil
}

type limitedAnchorBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *limitedAnchorBuffer) Write(value []byte) (int, error) {
	if buffer.Len()+len(value) > buffer.limit {
		return 0, fmt.Errorf("anchor provider response exceeds %d bytes", buffer.limit)
	}
	return buffer.Buffer.Write(value)
}

type limitedStderrWriter struct {
	buffer *bytes.Buffer
	limit  int
}

func (writer *limitedStderrWriter) Write(value []byte) (int, error) {
	remaining := writer.limit - writer.buffer.Len()
	if remaining > 0 {
		_, _ = writer.buffer.Write(value[:min(len(value), remaining)])
	}
	return len(value), nil
}

func anchorStderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	return ": " + stderr
}

func validateAnchorResponse(request anchorProtocolRequest, response anchorProtocolResponse, displayPaths map[string]string) (anchorLookup, error) {
	if response.ProtocolVersion != anchorProtocolVersion {
		return nil, fmt.Errorf("anchor provider protocol version %d is unsupported; expected %d", response.ProtocolVersion, anchorProtocolVersion)
	}
	requested := make(map[string]anchorProtocolRequestFile, len(request.Files))
	for _, file := range request.Files {
		requested[file.Path] = file
	}
	result := make(anchorLookup, len(request.Files))
	for _, file := range response.Files {
		requestFile, ok := requested[file.Path]
		if !ok || file.SHA256 != requestFile.SHA256 {
			return nil, fmt.Errorf("anchor provider returned an unknown or stale file %s", file.Path)
		}
		lineAnchors, err := validateFileAnchors(requestFile, file.Anchors)
		if err != nil {
			return nil, fmt.Errorf("anchor provider file %s: %w", file.Path, err)
		}
		result[displayPaths[file.Path]] = lineAnchors
		delete(requested, file.Path)
	}
	if len(requested) > 0 {
		return nil, fmt.Errorf("anchor provider omitted %d requested files", len(requested))
	}
	return result, nil
}

func validateFileAnchors(file anchorProtocolRequestFile, anchors []anchorProtocolLine) (map[int]string, error) {
	requested := make(map[int]bool, len(file.Lines))
	for _, line := range file.Lines {
		requested[line] = true
	}
	result := make(map[int]string, len(anchors))
	anchorLines := make(map[string]int, len(anchors))
	for _, item := range anchors {
		if !requested[item.Line] || result[item.Line] != "" {
			return nil, fmt.Errorf("unexpected or duplicate line %d", item.Line)
		}
		if item.Anchor == "" || len(item.Anchor) > maxAnchorLength || strings.ContainsAny(item.Anchor, "\r\n"+anchorOutputSeparator) {
			return nil, fmt.Errorf("invalid anchor for line %d", item.Line)
		}
		if previousLine, exists := anchorLines[item.Anchor]; exists {
			return nil, fmt.Errorf("anchor %q is duplicated on lines %d and %d", item.Anchor, previousLine, item.Line)
		}
		anchorLines[item.Anchor] = item.Line
		result[item.Line] = item.Anchor
	}
	if len(result) != len(requested) {
		return nil, fmt.Errorf("returned %d anchors for %d requested lines", len(result), len(requested))
	}
	return result, nil
}

func (anchors anchorLookup) line(path string, line int) string {
	return anchors[path][line]
}
