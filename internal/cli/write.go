package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/hashline"
	"github.com/pmezard/go-difflib/difflib"
)

const (
	writeSchema             = "grepple-write-v1"
	maxWriteRequestBytes    = 16 << 20
	maxWriteFileBytes       = 16 << 20
	maxWriteFiles           = 128
	maxWriteChanges         = 4096
	maxWriteDiagnosticLines = 5
	maxWriteDiagnosticBytes = 240
)

const writeHelp = `Apply one validated, hash-anchored transaction across multiple files.
Usage: grepple write [--root PATH] [--dry-run] [--json]
       grepple write edit --path PATH --start HASH [--end HASH] [--content-file PATH|-] [OPTIONS]

The default mode auto-detects either one strict grepple-write-v1 JSON value or a
literal ::grepple heredoc transaction from stdin. Existing JSON files use anchored
edits (operation defaults to edit):
  {"path":"file.go","changes":[{"hash_range_inclusive":["START","END"],"content_lines":["replacement"]}]}

Literal edit mode avoids JSON escaping for one edit. Replacement text is read
verbatim from stdin by default, or from --content-file PATH. One terminal newline
separates lines but does not add a blank line. Empty input deletes the range.
  grepple write edit --root . --path file.go --start START --end END <<'EOF'
  literal replacement text
  EOF

Literal heredoc transactions support the same edits, creates, and deletes:
  ::grepple file file.go
  ::grepple replace START END [--end-marker TOKEN]
  literal replacement text
  ::grepple end [TOKEN]
  ::grepple file new.go
  ::grepple create [--end-marker TOKEN]
  literal new-file content
  ::grepple end [TOKEN]
  ::grepple file old.go
  ::grepple delete 64-lowercase-hex-digest

Create and delete are also explicit JSON file operations:
  {"path":"new.go","operation":"create","content_lines":["package sample",""]}
  {"path":"old.go","operation":"delete","before_sha256":"64-lowercase-hex-digest"}

All paths, identities, files, hashes, ranges, and replacement lines are validated
before mutation. Ranges are inclusive and refer to the original snapshot. Empty
change content_lines deletes a range. Create requires an absent target beneath an
existing confined directory. Delete requires the exact current SHA-256 digest.
The transaction preserves existing newline style and permissions and rolls back
normal installation failures.

Default output is edit-ready HASH│LINE│content with a concise summary. Successful
edits return freshly recomputed anchors; dry runs also print deterministic unified
diffs and mark anchors as predicted. Use --json for the complete structured response.
`

type writeRequest struct {
	Schema string             `json:"schema"`
	Files  []writeRequestFile `json:"files"`
}

type writeRequestFile struct {
	Path         string        `json:"path"`
	Operation    string        `json:"operation,omitempty"`
	BeforeSHA256 string        `json:"before_sha256,omitempty"`
	ContentLines []string      `json:"content_lines,omitempty"`
	Changes      []writeChange `json:"changes,omitempty"`
}

type writeChange struct {
	HashRangeInclusive []string `json:"hash_range_inclusive"`
	ContentLines       []string `json:"content_lines"`
}

type writeResponse struct {
	Schema  string              `json:"schema"`
	Applied bool                `json:"applied"`
	DryRun  bool                `json:"dry_run"`
	Files   []writeResponseFile `json:"files,omitempty"`
	Error   *writeResponseError `json:"error,omitempty"`
}

type writeResponseFile struct {
	Path          string                `json:"path"`
	Operation     string                `json:"operation"`
	Changed       bool                  `json:"changed"`
	Changes       int                   `json:"changes"`
	BeforeSHA256  string                `json:"before_sha256,omitempty"`
	AfterSHA256   string                `json:"after_sha256,omitempty"`
	ChangeDetails []writeResponseChange `json:"change_details,omitempty"`
	Diff          string                `json:"diff,omitempty"`
}

type writeResponseChange struct {
	Index          int           `json:"index"`
	AfterStartLine int           `json:"after_start_line,omitempty"`
	AfterEndLine   int           `json:"after_end_line,omitempty"`
	Deleted        bool          `json:"deleted,omitempty"`
	Predicted      bool          `json:"predicted,omitempty"`
	Anchors        []writeAnchor `json:"anchors,omitempty"`
}

type writeResponseError struct {
	Code        string        `json:"code"`
	Message     string        `json:"message"`
	Path        string        `json:"path,omitempty"`
	ChangeIndex *int          `json:"change_index,omitempty"`
	Anchors     []writeAnchor `json:"anchors,omitempty"`
}

type writeAnchor struct {
	Hash    string `json:"hash"`
	Line    int    `json:"line"`
	Content string `json:"content"`
}

type writeOptions struct {
	root        string
	dryRun      bool
	json        bool
	help        bool
	literalEdit bool
	path        string
	start       string
	end         string
	contentFile string
}

type preparedWriteFile struct {
	requestPath string
	path        string
	operation   string
	mode        os.FileMode
	info        os.FileInfo
	original    []byte
	updated     []byte
	changes     []resolvedWriteChange
	stagedPath  string
	backupPath  string
}

type resolvedWriteChange struct {
	start        int
	end          int
	requestIndex int
	content      []string
}

type writeFailure struct {
	code        string
	message     string
	path        string
	changeIndex *int
	anchors     []writeAnchor
}

type writeResponseCounter struct {
	writer  io.Writer
	written int
}

func (counter *writeResponseCounter) Write(content []byte) (int, error) {
	written, err := counter.writer.Write(content)
	counter.written += written
	return written, err
}

func runWrite(args []string) error {
	options, err := parseWriteOptions(args)
	if err != nil {
		return err
	}
	if options.help {
		return stdoutWriter().writeString(writeHelp)
	}
	var request writeRequest
	var failure *writeFailure
	if options.literalEdit {
		request, failure = decodeLiteralWriteRequest(options, os.Stdin)
	} else {
		request, failure = decodeWriteRequest(os.Stdin)
	}
	if failure != nil {
		if err := emitWriteResponse(os.Stdout, failedWriteResponse(options.dryRun, failure), options.json); err != nil {
			return err
		}
		requestExit(1)
		return nil
	}
	response, failure := executeWriteRequest(options.root, options.dryRun, request)
	if failure != nil {
		response = failedWriteResponse(options.dryRun, failure)
	}
	output := &writeResponseCounter{writer: os.Stdout}
	if err := emitWriteResponse(output, response, options.json); err != nil {
		return err
	}
	if failure == nil && response.Applied && !options.json {
		recordWriteResponseContext(options.root, response, output.written)
	}
	if failure != nil {
		requestExit(1)
	}
	return nil
}

func parseWriteOptions(args []string) (writeOptions, error) {
	options := writeOptions{root: ".", contentFile: "-"}
	if len(args) > 0 && args[0] == "edit" {
		options.literalEdit = true
		args = args[1:]
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--help", "-h":
			options.help = true
		case "--json":
			options.json = true
		case "--dry-run":
			options.dryRun = true
		default:
			if !writeOptionTakesValue(argument) {
				return options, fmt.Errorf("unknown write option %q", argument)
			}
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return options, fmt.Errorf("%s requires a value", argument)
			}
			index++
			setWriteOptionValue(&options, argument, args[index])
		}
	}
	return validateWriteOptions(options)
}

func writeOptionTakesValue(argument string) bool {
	switch argument {
	case "--root", "--path", "--start", "--end", "--content-file":
		return true
	default:
		return false
	}
}

func setWriteOptionValue(options *writeOptions, flag, value string) {
	switch flag {
	case "--root":
		options.root = value
	case "--path":
		options.path = value
	case "--start":
		options.start = value
	case "--end":
		options.end = value
	case "--content-file":
		options.contentFile = value
	}
}

func validateWriteOptions(options writeOptions) (writeOptions, error) {
	if options.help {
		return options, nil
	}
	if !options.literalEdit && (options.path != "" || options.start != "" || options.end != "" || options.contentFile != "-") {
		return options, fmt.Errorf("--path, --start, --end, and --content-file require 'grepple write edit'")
	}
	if !options.literalEdit {
		return options, nil
	}
	if options.path == "" {
		return options, fmt.Errorf("write edit requires --path")
	}
	if options.start == "" {
		return options, fmt.Errorf("write edit requires --start")
	}
	if options.end == "" {
		options.end = options.start
	}
	return options, nil
}

func decodeLiteralWriteRequest(options writeOptions, stdin io.Reader) (writeRequest, *writeFailure) {
	reader := stdin
	var contentFile *os.File
	if options.contentFile != "-" {
		file, err := os.Open(options.contentFile)
		if err != nil {
			return writeRequest{}, newWriteFailure("invalid_request", fmt.Sprintf("open literal content: %v", err), options.path, nil)
		}
		contentFile = file
		reader = file
		defer contentFile.Close()
	}
	content, err := io.ReadAll(io.LimitReader(reader, maxWriteFileBytes+1))
	if err != nil {
		return writeRequest{}, newWriteFailure("invalid_request", fmt.Sprintf("read literal content: %v", err), options.path, nil)
	}
	if len(content) > maxWriteFileBytes {
		return writeRequest{}, newWriteFailure("file_too_large", fmt.Sprintf("literal content exceeds %d bytes", maxWriteFileBytes), options.path, nil)
	}
	lines, err := literalWriteLines(content)
	if err != nil {
		return writeRequest{}, newWriteFailure("invalid_content", err.Error(), options.path, intPointer(0))
	}
	if failure := validateWriteContentLines(options.path, intPointer(0), lines); failure != nil {
		return writeRequest{}, failure
	}
	return writeRequest{Schema: writeSchema, Files: []writeRequestFile{{
		Path: options.path,
		Changes: []writeChange{{
			HashRangeInclusive: []string{options.start, options.end},
			ContentLines:       lines,
		}},
	}}}, nil
}

func literalWriteLines(content []byte) ([]string, error) {
	if len(content) == 0 {
		return []string{}, nil
	}
	lines, _, err := writeLogicalLines(content)
	if err != nil {
		return nil, err
	}
	if content[len(content)-1] == '\n' {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func decodeWriteRequest(reader io.Reader) (writeRequest, *writeFailure) {
	content, err := io.ReadAll(io.LimitReader(reader, maxWriteRequestBytes+1))
	if err != nil {
		return writeRequest{}, newWriteFailure("invalid_request", fmt.Sprintf("read request: %v", err), "", nil)
	}
	if len(content) > maxWriteRequestBytes {
		return writeRequest{}, newWriteFailure("request_too_large", fmt.Sprintf("request exceeds %d bytes", maxWriteRequestBytes), "", nil)
	}
	trimmed := bytes.TrimSpace(content)
	if bytes.HasPrefix(trimmed, []byte(writeHeredocDirective)) {
		return decodeHeredocWriteRequest(content)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var request writeRequest
	if err := decoder.Decode(&request); err != nil {
		return writeRequest{}, newWriteFailure("invalid_request", fmt.Sprintf("decode request: %v", err), "", nil)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return writeRequest{}, newWriteFailure("invalid_request", "request must contain exactly one JSON value", "", nil)
	}
	return request, nil
}

func executeWriteRequest(root string, dryRun bool, request writeRequest) (writeResponse, *writeFailure) {
	response := writeResponse{Schema: writeSchema, DryRun: dryRun}
	if request.Schema != writeSchema {
		return response, newWriteFailure("invalid_schema", fmt.Sprintf("schema must be %q", writeSchema), "", nil)
	}
	if len(request.Files) == 0 || len(request.Files) > maxWriteFiles {
		return response, newWriteFailure("invalid_request", fmt.Sprintf("files must contain between 1 and %d entries", maxWriteFiles), "", nil)
	}
	resolvedRoot, err := resolveWriteRoot(root)
	if err != nil {
		return response, newWriteFailure("invalid_root", err.Error(), "", nil)
	}
	prepared, summaries, failure := prepareWriteFiles(resolvedRoot, request.Files)
	if failure != nil {
		return response, failure
	}
	response.Files = summaries
	if dryRun {
		markWriteResponsePredicted(&response)
		return response, nil
	}
	if failure := commitWriteFiles(prepared); failure != nil {
		return response, failure
	}
	response.Applied = true
	return response, nil
}

func resolveWriteRoot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("write root is not a directory")
	}
	return resolved, nil
}

func prepareWriteFiles(root string, files []writeRequestFile) ([]preparedWriteFile, []writeResponseFile, *writeFailure) {
	prepared := make([]preparedWriteFile, 0, len(files))
	summaries := make([]writeResponseFile, 0, len(files))
	totalChanges := 0
	for _, file := range files {
		operation := normalizedWriteOperation(file.Operation)
		if operation == "edit" {
			totalChanges += len(file.Changes)
		} else {
			totalChanges++
		}
		if totalChanges > maxWriteChanges {
			return nil, nil, newWriteFailure("invalid_request", fmt.Sprintf("transaction may contain at most %d changes", maxWriteChanges), file.Path, nil)
		}
		item, summary, failure := prepareWriteFile(root, file, operation)
		if failure != nil {
			return nil, nil, failure
		}
		if duplicatePreparedWriteFile(prepared, item) {
			return nil, nil, newWriteFailure("duplicate_path", "file appears more than once in the transaction", file.Path, nil)
		}
		prepared = append(prepared, item)
		summaries = append(summaries, summary)
	}
	return prepared, summaries, nil
}

func normalizedWriteOperation(operation string) string {
	if strings.TrimSpace(operation) == "" {
		return "edit"
	}
	return strings.TrimSpace(operation)
}

func duplicatePreparedWriteFile(files []preparedWriteFile, candidate preparedWriteFile) bool {
	for _, file := range files {
		if file.path == candidate.path || (file.info != nil && candidate.info != nil && os.SameFile(file.info, candidate.info)) {
			return true
		}
	}
	return false
}

func prepareWriteFile(root string, request writeRequestFile, operation string) (preparedWriteFile, writeResponseFile, *writeFailure) {
	switch operation {
	case "edit", "delete":
		return prepareExistingWriteFile(root, request, operation)
	case "create":
		return prepareCreatedWriteFile(root, request)
	default:
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("invalid_operation", "operation must be edit, create, or delete", request.Path, nil)
	}
}

func prepareExistingWriteFile(root string, request writeRequestFile, operation string) (preparedWriteFile, writeResponseFile, *writeFailure) {
	path, info, err := confinedWritePath(root, request.Path)
	if err != nil {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("invalid_path", err.Error(), request.Path, nil)
	}
	content, failure := readWriteFile(path, request.Path)
	if failure != nil {
		return preparedWriteFile{}, writeResponseFile{}, failure
	}
	item := preparedWriteFile{requestPath: filepath.ToSlash(filepath.Clean(request.Path)), path: path, operation: operation, mode: info.Mode(), info: info, original: content}
	if operation == "delete" {
		if _, _, err := writeLogicalLines(content); err != nil {
			return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("unsupported_newlines", err.Error(), request.Path, nil)
		}
		if len(request.Changes) != 0 || request.ContentLines != nil || !validWriteDigest(request.BeforeSHA256) {
			return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("invalid_delete", "delete requires before_sha256 and does not accept changes or content_lines", request.Path, nil)
		}
		before := writeDigest(content)
		if request.BeforeSHA256 != before {
			return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("stale_file", "before_sha256 does not match the current file", request.Path, nil)
		}
		summary := writeResponseFile{Path: item.requestPath, Operation: operation, Changed: true, Changes: 1, BeforeSHA256: before, Diff: writeUnifiedDiff(item.requestPath, operation, content, nil)}
		return item, summary, nil
	}
	if request.ContentLines != nil || request.BeforeSHA256 != "" || len(request.Changes) == 0 {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("invalid_edit", "edit requires one or more changes and does not accept file-level content_lines or before_sha256", request.Path, nil)
	}
	lines, separator, err := writeLogicalLines(content)
	if err != nil {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("unsupported_newlines", err.Error(), request.Path, nil)
	}
	changes, changeFailure := resolveWriteChanges(request.Path, string(content), lines, request.Changes)
	if changeFailure != nil {
		return preparedWriteFile{}, writeResponseFile{}, changeFailure
	}
	updatedLines := applyWriteChanges(lines, changes)
	updated := []byte(strings.Join(updatedLines, separator))
	if len(updated) > maxWriteFileBytes {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("file_too_large", fmt.Sprintf("result exceeds %d bytes", maxWriteFileBytes), request.Path, nil)
	}
	item.updated = updated
	item.changes = changes
	summary := writeResponseFile{Path: item.requestPath, Operation: operation, Changed: !bytes.Equal(content, updated), Changes: len(changes), BeforeSHA256: writeDigest(content), AfterSHA256: writeDigest(updated), ChangeDetails: writeChangeDetails(updated, updatedLines, changes), Diff: writeUnifiedDiff(item.requestPath, operation, content, updated)}
	return item, summary, nil
}

func prepareCreatedWriteFile(root string, request writeRequestFile) (preparedWriteFile, writeResponseFile, *writeFailure) {
	if request.ContentLines == nil || len(request.Changes) != 0 || request.BeforeSHA256 != "" {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("invalid_create", "create requires content_lines and does not accept changes or before_sha256", request.Path, nil)
	}
	if failure := validateWriteContentLines(request.Path, nil, request.ContentLines); failure != nil {
		return preparedWriteFile{}, writeResponseFile{}, failure
	}
	path, err := confinedCreatePath(root, request.Path)
	if err != nil {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("invalid_path", err.Error(), request.Path, nil)
	}
	updated := []byte(strings.Join(request.ContentLines, "\n"))
	if len(updated) > maxWriteFileBytes {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("file_too_large", fmt.Sprintf("result exceeds %d bytes", maxWriteFileBytes), request.Path, nil)
	}
	requestPath := filepath.ToSlash(filepath.Clean(request.Path))
	item := preparedWriteFile{requestPath: requestPath, path: path, operation: "create", mode: 0o644, updated: updated}
	anchors := boundedWriteAnchors(request.ContentLines, hashline.Lines(string(updated)), 0, len(request.ContentLines))
	details := []writeResponseChange{{Index: 0, AfterStartLine: 1, AfterEndLine: len(request.ContentLines), Anchors: anchors}}
	if len(request.ContentLines) == 0 {
		details[0].AfterStartLine, details[0].AfterEndLine = 0, 0
	}
	summary := writeResponseFile{Path: requestPath, Operation: "create", Changed: true, Changes: 1, AfterSHA256: writeDigest(updated), ChangeDetails: details, Diff: writeUnifiedDiff(requestPath, "create", nil, updated)}
	return item, summary, nil
}

func readWriteFile(path, requestPath string) ([]byte, *writeFailure) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, newWriteFailure("read_failed", err.Error(), requestPath, nil)
	}
	if len(content) > maxWriteFileBytes {
		return nil, newWriteFailure("file_too_large", fmt.Sprintf("file exceeds %d bytes", maxWriteFileBytes), requestPath, nil)
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return nil, newWriteFailure("binary_file", "NUL-containing files cannot be edited", requestPath, nil)
	}
	return content, nil
}

func confinedWritePath(root, requested string) (string, os.FileInfo, error) {
	resolved, requestedInfo, err := confinedWriteTarget(root, requested)
	if err != nil {
		return "", nil, err
	}
	if requestedInfo == nil {
		return "", nil, fmt.Errorf("write target must already exist")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("stat write target: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("write target must be a regular file")
	}
	return resolved, info, nil
}

func confinedCreatePath(root, requested string) (string, error) {
	resolved, info, err := confinedWriteTarget(root, requested)
	if err != nil {
		return "", err
	}
	if info != nil {
		return "", fmt.Errorf("create target already exists")
	}
	return resolved, nil
}

func confinedWriteTarget(root, requested string) (string, os.FileInfo, error) {
	if strings.TrimSpace(requested) == "" || filepath.IsAbs(requested) {
		return "", nil, fmt.Errorf("write path must be repository-relative")
	}
	clean := filepath.Clean(requested)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("write path escapes the root")
	}
	requestedPath := filepath.Join(root, clean)
	requestedInfo, err := os.Lstat(requestedPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("inspect write target: %w", err)
	}
	if requestedInfo != nil && requestedInfo.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("write target may not be a symlink")
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(requestedPath))
	if err != nil {
		return "", nil, fmt.Errorf("resolve write directory: %w", err)
	}
	parentInfo, err := os.Stat(resolvedParent)
	if err != nil || !parentInfo.IsDir() {
		return "", nil, fmt.Errorf("write parent must be an existing directory")
	}
	resolved := filepath.Join(resolvedParent, filepath.Base(requestedPath))
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("write path escapes the root")
	}
	return resolved, requestedInfo, nil
}

func writeLogicalLines(content []byte) ([]string, string, error) {
	value := string(content)
	lfCount := strings.Count(value, "\n")
	crlfCount := strings.Count(value, "\r\n")
	if crlfCount > 0 {
		if crlfCount != lfCount || strings.Contains(strings.ReplaceAll(value, "\r\n", ""), "\r") {
			return nil, "", fmt.Errorf("mixed or bare carriage-return newlines are unsupported")
		}
		return strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n"), "\r\n", nil
	}
	if strings.Contains(value, "\r") {
		return nil, "", fmt.Errorf("bare carriage-return newlines are unsupported")
	}
	return strings.Split(value, "\n"), "\n", nil
}

func resolveWriteChanges(path, content string, lines []string, requests []writeChange) ([]resolvedWriteChange, *writeFailure) {
	hashes := hashline.Lines(content)
	byHash := make(map[string]int, len(hashes))
	for index, hash := range hashes {
		byHash[hash] = index
	}
	changes := make([]resolvedWriteChange, 0, len(requests))
	for requestIndex, request := range requests {
		change, failure := resolveWriteChange(path, requestIndex, request, lines, hashes, byHash)
		if failure != nil {
			return nil, failure
		}
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].start < changes[j].start })
	for index := 1; index < len(changes); index++ {
		if changes[index].start <= changes[index-1].end {
			requestIndex := changes[index].requestIndex
			return nil, newWriteFailure("overlapping_changes", "changes in one file may not overlap", path, &requestIndex)
		}
	}
	return changes, nil
}

func resolveWriteChange(path string, requestIndex int, request writeChange, lines, hashes []string, byHash map[string]int) (resolvedWriteChange, *writeFailure) {
	if len(request.HashRangeInclusive) != 2 || !hashline.Valid(request.HashRangeInclusive[0]) || !hashline.Valid(request.HashRangeInclusive[1]) {
		return resolvedWriteChange{}, newWriteFailure("invalid_range", "hash_range_inclusive must contain two valid three-character anchors", path, &requestIndex)
	}
	if request.ContentLines == nil {
		return resolvedWriteChange{}, newWriteFailure("invalid_content", "content_lines is required; use an empty array to delete the range", path, &requestIndex)
	}
	if failure := validateWriteContentLines(path, &requestIndex, request.ContentLines); failure != nil {
		return resolvedWriteChange{}, failure
	}
	start, startOK := byHash[request.HashRangeInclusive[0]]
	end, endOK := byHash[request.HashRangeInclusive[1]]
	if !startOK || !endOK {
		anchors := staleWriteAnchors(lines, hashes, start, startOK, end, endOK)
		return resolvedWriteChange{}, &writeFailure{code: "stale_anchor", message: "one or both anchors are not present in the current file", path: path, changeIndex: intPointer(requestIndex), anchors: anchors}
	}
	if start > end {
		return resolvedWriteChange{}, newWriteFailure("invalid_range", "start anchor occurs after end anchor", path, &requestIndex)
	}
	return resolvedWriteChange{start: start, end: end, requestIndex: requestIndex, content: append([]string(nil), request.ContentLines...)}, nil
}

func staleWriteAnchors(lines, hashes []string, start int, startOK bool, end int, endOK bool) []writeAnchor {
	center := 0
	if startOK {
		center = start
	} else if endOK {
		center = end
	}
	first := center - maxWriteDiagnosticLines/2
	if first < 0 {
		first = 0
	}
	last := first + maxWriteDiagnosticLines
	if last > len(lines) {
		last = len(lines)
		first = last - maxWriteDiagnosticLines
		if first < 0 {
			first = 0
		}
	}
	anchors := make([]writeAnchor, 0, last-first)
	for index := first; index < last; index++ {
		anchors = append(anchors, writeAnchor{Hash: hashes[index], Line: index + 1, Content: truncateWriteDiagnostic(lines[index])})
	}
	return anchors
}

func truncateWriteDiagnostic(value string) string {
	if len(value) <= maxWriteDiagnosticBytes {
		return value
	}
	return value[:maxWriteDiagnosticBytes] + "…"
}

func applyWriteChanges(lines []string, changes []resolvedWriteChange) []string {
	result := append([]string(nil), lines...)
	for index := len(changes) - 1; index >= 0; index-- {
		change := changes[index]
		next := make([]string, 0, len(result)-(change.end-change.start+1)+len(change.content))
		next = append(next, result[:change.start]...)
		next = append(next, change.content...)
		next = append(next, result[change.end+1:]...)
		result = next
	}
	return result
}

func validWriteDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validateWriteContentLines(path string, changeIndex *int, lines []string) *writeFailure {
	for _, line := range lines {
		if strings.ContainsAny(line, "\r\n") {
			return newWriteFailure("invalid_content", "content_lines entries may not contain newline characters", path, changeIndex)
		}
		if strings.ContainsRune(line, '\x00') {
			return newWriteFailure("invalid_content", "content_lines entries may not contain NUL bytes", path, changeIndex)
		}
	}
	return nil
}

func writeChangeDetails(updated []byte, lines []string, changes []resolvedWriteChange) []writeResponseChange {
	hashes := hashline.Lines(string(updated))
	details := make([]writeResponseChange, 0, len(changes))
	offset := 0
	for _, change := range changes {
		afterStart := change.start + offset
		afterEnd := afterStart + len(change.content)
		first, last := afterStart-1, afterEnd+1
		deleted := len(change.content) == 0
		if deleted {
			last = afterStart + 1
		}
		anchors := boundedWriteAnchors(lines, hashes, first, last)
		detail := writeResponseChange{Index: change.requestIndex, Deleted: deleted, Anchors: anchors}
		if !deleted {
			detail.AfterStartLine = afterStart + 1
			detail.AfterEndLine = afterEnd
		}
		details = append(details, detail)
		offset += len(change.content) - (change.end - change.start + 1)
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Index < details[j].Index })
	return details
}

func boundedWriteAnchors(lines, hashes []string, first, last int) []writeAnchor {
	if first < 0 {
		first = 0
	}
	if last > len(lines) {
		last = len(lines)
	}
	if last < first {
		last = first
	}
	indices := make([]int, 0, last-first)
	for index := first; index < last; index++ {
		indices = append(indices, index)
	}
	anchors := make([]writeAnchor, 0, len(indices))
	for _, index := range indices {
		if index >= len(hashes) {
			continue
		}
		anchors = append(anchors, writeAnchor{Hash: hashes[index], Line: index + 1, Content: lines[index]})
	}
	return anchors
}

func writeUnifiedDiff(path, operation string, before, after []byte) string {
	from, to := "a/"+path, "b/"+path
	if operation == "create" {
		from = "/dev/null"
	}
	if operation == "delete" {
		to = "/dev/null"
	}
	a, b := writeDiffLines(before), writeDiffLines(after)
	if len(a) == 0 && len(b) == 0 && operation != "edit" {
		return fmt.Sprintf("--- %s\n+++ %s\n", from, to)
	}
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: a, B: b, FromFile: from, ToFile: to, Context: 3,
	})
	return diff
}

func writeDiffLines(content []byte) []string {
	if len(content) == 0 {
		return nil
	}
	return difflib.SplitLines(strings.ReplaceAll(string(content), "\r\n", "\n"))
}

func commitWriteFiles(files []preparedWriteFile) *writeFailure {
	changed := make([]*preparedWriteFile, 0, len(files))
	for index := range files {
		file := &files[index]
		if file.operation == "edit" && bytes.Equal(file.original, file.updated) {
			continue
		}
		changed = append(changed, file)
	}
	if len(changed) == 0 {
		return nil
	}
	if failure := stageWriteFiles(changed); failure != nil {
		cleanupStagedWriteFiles(changed)
		return failure
	}
	defer cleanupStagedWriteFiles(changed)
	for _, file := range changed {
		if file.operation == "create" {
			if _, err := os.Lstat(file.path); !errors.Is(err, os.ErrNotExist) {
				return newWriteFailure("concurrent_change", "create target appeared after validation", file.requestPath, nil)
			}
			continue
		}
		current, err := os.ReadFile(file.path)
		if err != nil || !bytes.Equal(current, file.original) {
			return newWriteFailure("concurrent_change", "file changed after validation", file.requestPath, nil)
		}
	}
	return installWriteFiles(changed)
}

func stageWriteFiles(files []*preparedWriteFile) *writeFailure {
	for _, file := range files {
		if file.operation == "delete" {
			continue
		}
		temporary, err := os.CreateTemp(filepath.Dir(file.path), ".grepple-write-new-*")
		if err != nil {
			return newWriteFailure("write_failed", err.Error(), file.requestPath, nil)
		}
		file.stagedPath = temporary.Name()
		_, err = temporary.Write(file.updated)
		if err == nil {
			preservedMode := file.mode.Perm() | file.mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky)
			err = temporary.Chmod(preservedMode)
		}
		if err == nil {
			err = temporary.Sync()
		}
		closeErr := temporary.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return newWriteFailure("write_failed", err.Error(), file.requestPath, nil)
		}
	}
	return nil
}

func installWriteFiles(files []*preparedWriteFile) *writeFailure {
	backedUp, failure := backupWriteFiles(files)
	if failure != nil {
		return failure
	}
	if failure := installStagedWriteFiles(files, backedUp); failure != nil {
		return failure
	}
	for _, file := range backedUp {
		_ = os.Remove(file.backupPath)
		file.backupPath = ""
	}
	return nil
}

func backupWriteFiles(files []*preparedWriteFile) ([]*preparedWriteFile, *writeFailure) {
	backedUp := make([]*preparedWriteFile, 0, len(files))
	for _, file := range files {
		if file.operation == "create" {
			continue
		}
		backup, err := reserveWriteBackup(file.path)
		if err != nil {
			rollbackErr := restoreWriteBackups(backedUp)
			return nil, newWriteFailure("write_failed", writeRollbackMessage(err, rollbackErr), file.requestPath, nil)
		}
		file.backupPath = backup
		if err := os.Rename(file.path, backup); err != nil {
			_ = os.Remove(backup)
			file.backupPath = ""
			rollbackErr := restoreWriteBackups(backedUp)
			return nil, newWriteFailure("write_failed", writeRollbackMessage(err, rollbackErr), file.requestPath, nil)
		}
		backedUp = append(backedUp, file)
		captured, readErr := os.ReadFile(backup)
		if readErr != nil || !bytes.Equal(captured, file.original) {
			rollbackErr := restoreWriteBackups(backedUp)
			primary := fmt.Errorf("file changed during installation")
			if readErr != nil {
				primary = readErr
			}
			return nil, newWriteFailure("concurrent_change", writeRollbackMessage(primary, rollbackErr), file.requestPath, nil)
		}
	}
	return backedUp, nil
}

func installStagedWriteFiles(files, backedUp []*preparedWriteFile) *writeFailure {
	installed := make([]*preparedWriteFile, 0, len(files))
	for _, file := range files {
		if file.operation == "delete" {
			continue
		}
		err := installStagedWriteFile(file)
		if err != nil {
			rollbackErr := rollbackInstalledWriteFiles(installed, backedUp)
			return newWriteFailure("write_failed", writeRollbackMessage(err, rollbackErr), file.requestPath, nil)
		}
		installed = append(installed, file)
	}
	return nil
}

func installStagedWriteFile(file *preparedWriteFile) error {
	if file.operation == "create" {
		return os.Link(file.stagedPath, file.path)
	}
	if err := os.Rename(file.stagedPath, file.path); err != nil {
		return err
	}
	file.stagedPath = ""
	return nil
}

func reserveWriteBackup(path string) (string, error) {
	backup, err := os.CreateTemp(filepath.Dir(path), ".grepple-write-backup-*")
	if err != nil {
		return "", err
	}
	name := backup.Name()
	if err := backup.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func restoreWriteBackups(files []*preparedWriteFile) error {
	failures := []string{}
	for index := len(files) - 1; index >= 0; index-- {
		file := files[index]
		if file.backupPath == "" {
			continue
		}
		if err := os.Rename(file.backupPath, file.path); err != nil {
			failures = append(failures, fmt.Sprintf("restore %s from %s: %v", file.path, file.backupPath, err))
			continue
		}
		file.backupPath = ""
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func rollbackInstalledWriteFiles(installed, backedUp []*preparedWriteFile) error {
	failures := []string{}
	for _, file := range installed {
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Sprintf("remove staged replacement %s: %v", file.path, err))
		}
	}
	if err := restoreWriteBackups(backedUp); err != nil {
		failures = append(failures, err.Error())
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func writeRollbackMessage(primary, rollback error) string {
	if rollback == nil {
		return primary.Error()
	}
	return primary.Error() + "; rollback incomplete: " + rollback.Error()
}

func cleanupStagedWriteFiles(files []*preparedWriteFile) {
	for _, file := range files {
		if file.stagedPath != "" {
			_ = os.Remove(file.stagedPath)
		}
	}
}

func writeDigest(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func newWriteFailure(code, message, path string, changeIndex *int) *writeFailure {
	return &writeFailure{code: code, message: message, path: path, changeIndex: changeIndex}
}

func failedWriteResponse(dryRun bool, failure *writeFailure) writeResponse {
	return writeResponse{Schema: writeSchema, DryRun: dryRun, Error: &writeResponseError{Code: failure.code, Message: failure.message, Path: failure.path, ChangeIndex: failure.changeIndex, Anchors: failure.anchors}}
}

func markWriteResponsePredicted(response *writeResponse) {
	for fileIndex := range response.Files {
		for changeIndex := range response.Files[fileIndex].ChangeDetails {
			response.Files[fileIndex].ChangeDetails[changeIndex].Predicted = true
		}
	}
}

func emitWriteResponse(writer io.Writer, response writeResponse, jsonMode bool) error {
	if jsonMode {
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(response)
	}
	if response.Error != nil {
		return emitWriteError(writer, response.Error)
	}
	if response.DryRun {
		if err := emitWriteDiffs(writer, response.Files); err != nil {
			return err
		}
	}
	if err := emitWriteResponseFiles(writer, response); err != nil {
		return err
	}
	return emitWriteSummary(writer, response)
}

func emitWriteError(writer io.Writer, response *writeResponseError) error {
	if _, err := fmt.Fprintf(writer, "write rejected: %s: %s\n", response.Code, response.Message); err != nil {
		return err
	}
	if response.Path != "" {
		if _, err := fmt.Fprintf(writer, "path: %s\n", response.Path); err != nil {
			return err
		}
	}
	if response.ChangeIndex != nil {
		if _, err := fmt.Fprintf(writer, "change: %d\n", *response.ChangeIndex); err != nil {
			return err
		}
	}
	return emitWriteAnchors(writer, response.Anchors)
}

func emitWriteDiffs(writer io.Writer, files []writeResponseFile) error {
	if _, err := io.WriteString(writer, "dry-run: no files changed\n\n"); err != nil {
		return err
	}
	for _, file := range files {
		if file.Diff == "" {
			continue
		}
		if _, err := io.WriteString(writer, file.Diff); err != nil {
			return err
		}
		if !strings.HasSuffix(file.Diff, "\n") {
			if _, err := io.WriteString(writer, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

func emitWriteResponseFiles(writer io.Writer, response writeResponse) error {
	printed := false
	for _, file := range response.Files {
		anchors := responseFileAnchors(file)
		if len(anchors) == 0 && file.Operation == "edit" {
			continue
		}
		if err := emitWriteResponseFile(writer, file, anchors, printed || response.DryRun, response.DryRun); err != nil {
			return err
		}
		printed = true
	}
	return nil
}

func emitWriteResponseFile(writer io.Writer, file writeResponseFile, anchors []writeAnchor, leading, predicted bool) error {
	if leading {
		if _, err := io.WriteString(writer, "\n"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(writer, "%s\n\n", file.Path); err != nil {
		return err
	}
	if predicted && len(anchors) > 0 {
		if _, err := fmt.Fprintf(writer, "predicted anchors for sha256:%s\n\n", file.AfterSHA256); err != nil {
			return err
		}
	}
	return emitWriteFileBody(writer, file.Operation, anchors)
}

func emitWriteFileBody(writer io.Writer, operation string, anchors []writeAnchor) error {
	switch {
	case operation == "create" && len(anchors) == 0:
		_, err := io.WriteString(writer, "created empty file\n")
		return err
	case operation == "delete":
		_, err := io.WriteString(writer, "deleted\n")
		return err
	default:
		return emitWriteAnchors(writer, anchors)
	}
}

func emitWriteSummary(writer io.Writer, response writeResponse) error {
	files, changes := 0, 0
	for _, file := range response.Files {
		if file.Changed {
			files++
		}
		changes += file.Changes
	}
	verb := "applied"
	if response.DryRun {
		verb = "validated"
	}
	_, err := fmt.Fprintf(writer, "\n%s %d files, %d changes\n", verb, files, changes)
	return err
}

func responseFileAnchors(file writeResponseFile) []writeAnchor {
	byLine := map[int]writeAnchor{}
	for _, change := range file.ChangeDetails {
		for _, anchor := range change.Anchors {
			byLine[anchor.Line] = anchor
		}
	}
	lines := make([]int, 0, len(byLine))
	for line := range byLine {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	anchors := make([]writeAnchor, 0, len(lines))
	for _, line := range lines {
		anchors = append(anchors, byLine[line])
	}
	return anchors
}

func emitWriteAnchors(writer io.Writer, anchors []writeAnchor) error {
	for _, anchor := range anchors {
		if _, err := fmt.Fprintf(writer, "%s%s%d%s%s\n", anchor.Hash, anchorOutputSeparator, anchor.Line, anchorOutputSeparator, normalizeRenderedAnchorLine(anchor.Content)); err != nil {
			return err
		}
	}
	return nil
}
