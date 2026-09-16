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

const writeHelp = `Apply one validated, hash-anchored transaction across multiple existing files.
Usage: grepple write [--root PATH] [--dry-run]

The request is one strict JSON value on stdin:
{
  "schema": "grepple-write-v1",
  "files": [{
    "path": "relative/file.go",
    "changes": [{
      "hash_range_inclusive": ["START", "END"],
      "content_lines": ["replacement", "lines"]
    }]
  }]
}

All paths and ranges are validated before any file changes. Ranges are inclusive,
refer to the original file snapshot, and may not overlap. An empty content_lines
deletes the range. Existing LF or CRLF style and file permissions are preserved.
Symlink files, escaping paths, mixed newlines, creates, and binary files are refused.
Use --dry-run to validate and report the transaction without writing.
`

type writeRequest struct {
	Schema string             `json:"schema"`
	Files  []writeRequestFile `json:"files"`
}

type writeRequestFile struct {
	Path    string        `json:"path"`
	Changes []writeChange `json:"changes"`
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
	Path         string `json:"path"`
	Changed      bool   `json:"changed"`
	Changes      int    `json:"changes"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
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
	root   string
	dryRun bool
	help   bool
}

type preparedWriteFile struct {
	requestPath string
	path        string
	mode        os.FileMode
	info        os.FileInfo
	original    []byte
	updated     []byte
	changes     int
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

func runWrite(args []string) error {
	options, err := parseWriteOptions(args)
	if err != nil {
		return err
	}
	if options.help {
		return stdoutWriter().writeString(writeHelp)
	}
	request, failure := decodeWriteRequest(os.Stdin)
	if failure != nil {
		if err := emitWriteResponse(os.Stdout, failedWriteResponse(options.dryRun, failure)); err != nil {
			return err
		}
		requestExit(1)
		return nil
	}
	response, failure := executeWriteRequest(options.root, options.dryRun, request)
	if failure != nil {
		response = failedWriteResponse(options.dryRun, failure)
	}
	if err := emitWriteResponse(os.Stdout, response); err != nil {
		return err
	}
	if failure != nil {
		requestExit(1)
	}
	return nil
}

func parseWriteOptions(args []string) (writeOptions, error) {
	options := writeOptions{root: "."}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--help", "-h":
			options.help = true
		case "--dry-run":
			options.dryRun = true
		case "--root":
			if index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" {
				return options, fmt.Errorf("--root requires a path")
			}
			index++
			options.root = args[index]
		default:
			return options, fmt.Errorf("unknown write option %q", args[index])
		}
	}
	return options, nil
}

func decodeWriteRequest(reader io.Reader) (writeRequest, *writeFailure) {
	content, err := io.ReadAll(io.LimitReader(reader, maxWriteRequestBytes+1))
	if err != nil {
		return writeRequest{}, newWriteFailure("invalid_request", fmt.Sprintf("read request: %v", err), "", nil)
	}
	if len(content) > maxWriteRequestBytes {
		return writeRequest{}, newWriteFailure("request_too_large", fmt.Sprintf("request exceeds %d bytes", maxWriteRequestBytes), "", nil)
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
		totalChanges += len(file.Changes)
		if len(file.Changes) == 0 || totalChanges > maxWriteChanges {
			return nil, nil, newWriteFailure("invalid_request", fmt.Sprintf("each file needs changes and the transaction may contain at most %d changes", maxWriteChanges), file.Path, nil)
		}
		item, summary, failure := prepareWriteFile(root, file)
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

func duplicatePreparedWriteFile(files []preparedWriteFile, candidate preparedWriteFile) bool {
	for _, file := range files {
		if file.path == candidate.path || os.SameFile(file.info, candidate.info) {
			return true
		}
	}
	return false
}

func prepareWriteFile(root string, request writeRequestFile) (preparedWriteFile, writeResponseFile, *writeFailure) {
	path, info, err := confinedWritePath(root, request.Path)
	if err != nil {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("invalid_path", err.Error(), request.Path, nil)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("read_failed", err.Error(), request.Path, nil)
	}
	if len(content) > maxWriteFileBytes {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("file_too_large", fmt.Sprintf("file exceeds %d bytes", maxWriteFileBytes), request.Path, nil)
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("binary_file", "NUL-containing files cannot be edited", request.Path, nil)
	}
	lines, separator, err := writeLogicalLines(content)
	if err != nil {
		return preparedWriteFile{}, writeResponseFile{}, newWriteFailure("unsupported_newlines", err.Error(), request.Path, nil)
	}
	changes, failure := resolveWriteChanges(request.Path, string(content), lines, request.Changes)
	if failure != nil {
		return preparedWriteFile{}, writeResponseFile{}, failure
	}
	updatedLines := applyWriteChanges(lines, changes)
	updated := []byte(strings.Join(updatedLines, separator))
	before := writeDigest(content)
	after := writeDigest(updated)
	item := preparedWriteFile{requestPath: filepath.ToSlash(filepath.Clean(request.Path)), path: path, mode: info.Mode(), info: info, original: content, updated: updated, changes: len(changes)}
	summary := writeResponseFile{Path: item.requestPath, Changed: !bytes.Equal(content, updated), Changes: len(changes), BeforeSHA256: before, AfterSHA256: after}
	return item, summary, nil
}

func confinedWritePath(root, requested string) (string, os.FileInfo, error) {
	if strings.TrimSpace(requested) == "" || filepath.IsAbs(requested) {
		return "", nil, fmt.Errorf("write path must be repository-relative")
	}
	clean := filepath.Clean(requested)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("write path escapes the root")
	}
	requestedPath := filepath.Join(root, clean)
	requestedInfo, err := os.Lstat(requestedPath)
	if err != nil {
		return "", nil, fmt.Errorf("write target must already exist: %w", err)
	}
	if requestedInfo.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("write target may not be a symlink")
	}
	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(requestedPath))
	if err != nil {
		return "", nil, fmt.Errorf("resolve write directory: %w", err)
	}
	resolved := filepath.Join(resolvedParent, filepath.Base(requestedPath))
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("write path escapes the root")
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
	for _, line := range request.ContentLines {
		if strings.ContainsAny(line, "\r\n") {
			return resolvedWriteChange{}, newWriteFailure("invalid_content", "content_lines entries may not contain newline characters", path, &requestIndex)
		}
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

func commitWriteFiles(files []preparedWriteFile) *writeFailure {
	changed := make([]*preparedWriteFile, 0, len(files))
	for index := range files {
		if bytes.Equal(files[index].original, files[index].updated) {
			continue
		}
		changed = append(changed, &files[index])
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
		current, err := os.ReadFile(file.path)
		if err != nil || !bytes.Equal(current, file.original) {
			return newWriteFailure("concurrent_change", "file changed after anchor validation", file.requestPath, nil)
		}
	}
	return installWriteFiles(changed)
}

func stageWriteFiles(files []*preparedWriteFile) *writeFailure {
	for _, file := range files {
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
	moved := 0
	for index, file := range files {
		backup, err := reserveWriteBackup(file.path)
		if err != nil {
			rollbackErr := restoreWriteBackups(files[:moved])
			return newWriteFailure("write_failed", writeRollbackMessage(err, rollbackErr), file.requestPath, nil)
		}
		file.backupPath = backup
		if err := os.Rename(file.path, backup); err != nil {
			_ = os.Remove(backup)
			file.backupPath = ""
			rollbackErr := restoreWriteBackups(files[:moved])
			return newWriteFailure("write_failed", writeRollbackMessage(err, rollbackErr), file.requestPath, nil)
		}
		moved = index + 1
	}
	installed := 0
	for index, file := range files {
		if err := os.Rename(file.stagedPath, file.path); err != nil {
			rollbackErr := rollbackInstalledWriteFiles(files, installed)
			return newWriteFailure("write_failed", writeRollbackMessage(err, rollbackErr), file.requestPath, nil)
		}
		file.stagedPath = ""
		installed = index + 1
	}
	for _, file := range files {
		_ = os.Remove(file.backupPath)
		file.backupPath = ""
	}
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

func rollbackInstalledWriteFiles(files []*preparedWriteFile, installed int) error {
	failures := []string{}
	for index := 0; index < installed; index++ {
		if err := os.Remove(files[index].path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Sprintf("remove staged replacement %s: %v", files[index].path, err))
		}
	}
	if err := restoreWriteBackups(files); err != nil {
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

func emitWriteResponse(writer io.Writer, response writeResponse) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(response)
}
