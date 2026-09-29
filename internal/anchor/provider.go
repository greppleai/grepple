// Package anchor generates stable edit anchors through the native implementation
// or a configured external provider.
package anchor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/greppleai/grepple/internal/config"
	"github.com/greppleai/grepple/internal/hashline"
)

const (
	ProtocolVersion  = 1
	MaxResponseBytes = 4 << 20
	MaxStderrBytes   = 8 << 10
	maxAnchorLength  = 128
	outputSeparator  = "│"
)

// File describes normalized source content and the lines that need anchors.
type File struct {
	Path        string
	DisplayPath string
	Content     string
	Lines       []int
}

// Lookup maps display paths and source lines to edit anchors.
type Lookup map[string]map[int]string

type protocolRequest struct {
	ProtocolVersion int                   `json:"protocol_version"`
	Files           []protocolRequestFile `json:"files"`
}

type protocolRequestFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
	Lines   []int  `json:"lines"`
}

type protocolResponse struct {
	ProtocolVersion int                    `json:"protocol_version"`
	Files           []protocolResponseFile `json:"files"`
}

type protocolResponseFile struct {
	Path    string         `json:"path"`
	SHA256  string         `json:"sha256"`
	Anchors []protocolLine `json:"anchors"`
}

type protocolLine struct {
	Line   int    `json:"line"`
	Anchor string `json:"anchor"`
}

// Generate creates anchors for files using the configured provider.
func Generate(files []File) (Lookup, error) {
	request, displayPaths, err := buildRequest(files)
	if err != nil {
		return nil, err
	}
	if len(request.Files) == 0 {
		return make(Lookup), nil
	}
	settings, err := config.LoadConfig("", true)
	if err != nil {
		return nil, err
	}
	if useNativeProvider(settings) {
		return nativeLookup(request, displayPaths)
	}
	_, provider, err := settings.ResolveProvider("")
	if err != nil {
		return nil, err
	}
	return generateWithProvider(provider, request, displayPaths)
}

// Read generates anchors for selected lines using the configured provider.
func Read(path, content string, lines []int) (map[int]string, bool, error) {
	lookup, err := Generate([]File{{Path: path, DisplayPath: path, Content: content, Lines: lines}})
	if err != nil {
		return nil, false, err
	}
	return lookup[path], true, nil
}

// RoundTrip invokes an explicit provider and validates its response. It returns
// the encoded response size for provider diagnostics.
func RoundTrip(provider config.Provider, file File) (int, error) {
	request, displayPaths, err := buildRequest([]File{file})
	if err != nil {
		return 0, err
	}
	response, err := invokeProvider(provider, request)
	if err != nil {
		return 0, err
	}
	if _, err := validateResponse(request, response, displayPaths); err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return 0, err
	}
	return len(encoded), nil
}

// UseNativeProvider reports whether native anchors are enabled.
func UseNativeProvider() (bool, error) {
	settings, err := config.LoadConfig("", true)
	if err != nil {
		return false, err
	}
	return useNativeProvider(settings), nil
}

func useNativeProvider(settings *config.Config) bool {
	if !settings.User.Anchors.EnabledByDefault {
		return true
	}
	return settings.User.Anchors.DefaultProvider == "" || settings.User.Anchors.DefaultProvider == "native"
}

// NormalizeContent normalizes source before anchor generation.
func NormalizeContent(content string) (string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if strings.Contains(content, "\r") {
		return "", fmt.Errorf("bare carriage returns are not supported because anchor line numbering would be ambiguous")
	}
	return content, nil
}

func buildRequest(files []File) (protocolRequest, map[string]string, error) {
	request := protocolRequest{ProtocolVersion: ProtocolVersion, Files: make([]protocolRequestFile, 0, len(files))}
	displayPaths := make(map[string]string, len(files))
	for _, file := range files {
		content, err := NormalizeContent(file.Content)
		if err != nil {
			return protocolRequest{}, nil, fmt.Errorf("anchor source %s: %w", file.DisplayPath, err)
		}
		lines := sortedUniqueLines(file.Lines)
		lineCount := len(strings.Split(content, "\n"))
		for _, line := range lines {
			if line < 1 || line > lineCount {
				return protocolRequest{}, nil, fmt.Errorf("invalid anchor line %d for %s", line, file.DisplayPath)
			}
		}
		displayPath := file.DisplayPath
		if displayPath == "" {
			displayPath = file.Path
		}
		request.Files = append(request.Files, protocolRequestFile{Path: file.Path, Content: content, SHA256: digest(content), Lines: lines})
		displayPaths[file.Path] = displayPath
	}
	return request, displayPaths, nil
}

func sortedUniqueLines(lines []int) []int {
	seen := make(map[int]bool, len(lines))
	result := make([]int, 0, len(lines))
	for _, line := range lines {
		if !seen[line] {
			seen[line] = true
			result = append(result, line)
		}
	}
	sort.Ints(result)
	return result
}

func generateWithProvider(provider config.Provider, request protocolRequest, displayPaths map[string]string) (Lookup, error) {
	response, err := invokeProvider(provider, request)
	if err != nil {
		return nil, err
	}
	return validateResponse(request, response, displayPaths)
}

func nativeLookup(request protocolRequest, displayPaths map[string]string) (Lookup, error) {
	lookup := make(Lookup, len(request.Files))
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

func digest(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func invokeProvider(provider config.Provider, request protocolRequest) (protocolResponse, error) {
	input, err := json.Marshal(request)
	if err != nil {
		return protocolResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(provider.TimeoutMS)*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, provider.Command[0], provider.Command[1:]...)
	command.Stdin = bytes.NewReader(input)
	var stdout limitedBuffer
	stdout.limit = MaxResponseBytes
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.MultiWriter(&limitedStderrWriter{buffer: &stderr, limit: MaxStderrBytes})
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return protocolResponse{}, fmt.Errorf("anchor provider timed out after %dms", provider.TimeoutMS)
		}
		return protocolResponse{}, fmt.Errorf("anchor provider failed: %w%s", err, stderrSuffix(stderr.String()))
	}
	var response protocolResponse
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return protocolResponse{}, fmt.Errorf("parse anchor provider response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return protocolResponse{}, fmt.Errorf("parse anchor provider response: %w", err)
	}
	return response, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
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

func stderrSuffix(stderr string) string {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return ""
	}
	return ": " + stderr
}

func validateResponse(request protocolRequest, response protocolResponse, displayPaths map[string]string) (Lookup, error) {
	if response.ProtocolVersion != ProtocolVersion {
		return nil, fmt.Errorf("anchor provider protocol version %d is unsupported; expected %d", response.ProtocolVersion, ProtocolVersion)
	}
	requested := make(map[string]protocolRequestFile, len(request.Files))
	for _, file := range request.Files {
		requested[file.Path] = file
	}
	result := make(Lookup, len(request.Files))
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

func validateFileAnchors(file protocolRequestFile, anchors []protocolLine) (map[int]string, error) {
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
		if item.Anchor == "" || len(item.Anchor) > maxAnchorLength || strings.ContainsAny(item.Anchor, "\r\n"+outputSeparator) {
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
