package write

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/hashline"
)

func TestExecuteWriteRequestAppliesMultiFileTransaction(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.txt")
	secondPath := filepath.Join(root, "second.txt")
	first := "one\ntwo\nthree\n"
	second := "alpha\r\nbeta\r\n"
	writeTestFile(t, firstPath, first, 0o640)
	writeTestFile(t, secondPath, second, 0o600)
	firstHashes := hashline.Lines(first)
	secondHashes := hashline.Lines(second)
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{
		{Path: "first.txt", Changes: []writeChange{
			{HashRangeInclusive: []string{firstHashes[2], firstHashes[2]}, ContentLines: []string{"THREE"}},
			{HashRangeInclusive: []string{firstHashes[0], firstHashes[1]}, ContentLines: []string{"ONE", "TWO", "inserted"}},
		}},
		{Path: "second.txt", Changes: []writeChange{{HashRangeInclusive: []string{secondHashes[0], secondHashes[0]}, ContentLines: []string{}}}},
	}}
	response, failure := executeWriteRequest(root, false, request)
	if failure != nil {
		t.Fatalf("failure=%+v", failure)
	}
	if !response.Applied || response.DryRun || len(response.Files) != 2 {
		t.Fatalf("response=%+v", response)
	}
	details := response.Files[0].ChangeDetails
	if len(details) != 2 || details[0].Index != 0 || details[0].AfterStartLine != 4 || details[1].Index != 1 || details[1].AfterStartLine != 1 || details[1].AfterEndLine != 3 {
		t.Fatalf("change details not in request order: %+v", details)
	}
	assertWriteFile(t, firstPath, "ONE\nTWO\ninserted\nTHREE\n", 0o640)
	assertWriteFile(t, secondPath, "beta\r\n", 0o600)
}

func TestExecuteWriteRequestRejectsStaleFileWithoutPartialWrites(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.txt")
	secondPath := filepath.Join(root, "second.txt")
	first := "one\ntwo\n"
	second := "alpha\nbeta\n"
	writeTestFile(t, firstPath, first, 0o600)
	writeTestFile(t, secondPath, second, 0o600)
	firstHashes := hashline.Lines(first)
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{
		{Path: "first.txt", Changes: []writeChange{{HashRangeInclusive: []string{firstHashes[0], firstHashes[0]}, ContentLines: []string{"ONE"}}}},
		{Path: "second.txt", Changes: []writeChange{{HashRangeInclusive: []string{"AAA", "BBB"}, ContentLines: []string{"nope"}}}},
	}}
	response, failure := executeWriteRequest(root, false, request)
	if failure == nil || failure.code != "stale_anchor" || failure.path != "second.txt" || len(failure.anchors) == 0 {
		t.Fatalf("response=%+v failure=%+v", response, failure)
	}
	assertWriteFile(t, firstPath, first, 0o600)
	assertWriteFile(t, secondPath, second, 0o600)
}

func TestExecuteWriteRequestDryRunValidatesWithoutMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	content := "before\n"
	writeTestFile(t, path, content, 0o600)
	hashes := hashline.Lines(content)
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{Path: "file.txt", Changes: []writeChange{{HashRangeInclusive: []string{hashes[0], hashes[0]}, ContentLines: []string{"after"}}}}}}
	response, failure := executeWriteRequest(root, true, request)
	if failure != nil || response.Applied || !response.DryRun || len(response.Files) != 1 || !response.Files[0].Changed {
		t.Fatalf("response=%+v failure=%+v", response, failure)
	}
	assertWriteFile(t, path, content, 0o600)
}

func TestCommitWriteFilesRejectsConcurrentChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	content := "before\n"
	writeTestFile(t, path, content, 0o600)
	hashes := hashline.Lines(content)
	requestFiles := []writeRequestFile{{Path: "file.txt", Changes: []writeChange{{HashRangeInclusive: []string{hashes[0], hashes[0]}, ContentLines: []string{"after"}}}}}
	prepared, _, failure := prepareWriteFiles(root, requestFiles)
	if failure != nil {
		t.Fatalf("prepare failure=%+v", failure)
	}
	writeTestFile(t, path, "concurrent\n", 0o600)
	if failure := commitWriteFiles(prepared); failure == nil || failure.code != "concurrent_change" {
		t.Fatalf("commit failure=%+v", failure)
	}
	assertWriteFile(t, path, "concurrent\n", 0o600)
}

func TestInstallWriteFilesRollsBackEditsAndDeletesWhenCreateAppears(t *testing.T) {
	root := t.TempDir()
	editContent := "before\n"
	deleteContent := "delete\n"
	writeTestFile(t, filepath.Join(root, "edit.txt"), editContent, 0o600)
	writeTestFile(t, filepath.Join(root, "delete.txt"), deleteContent, 0o640)
	hash := hashline.Lines(editContent)[0]
	requestFiles := []writeRequestFile{
		{Path: "edit.txt", Changes: []writeChange{{HashRangeInclusive: []string{hash, hash}, ContentLines: []string{"after"}}}},
		{Path: "delete.txt", Operation: "delete", BeforeSHA256: writeDigest([]byte(deleteContent))},
		{Path: "create.txt", Operation: "create", ContentLines: []string{"created", ""}},
	}
	prepared, _, failure := prepareWriteFiles(root, requestFiles)
	if failure != nil {
		t.Fatal(failure)
	}
	files := make([]*preparedWriteFile, 0, len(prepared))
	for index := range prepared {
		files = append(files, &prepared[index])
	}
	if failure := stageWriteFiles(files); failure != nil {
		t.Fatal(failure)
	}
	defer cleanupStagedWriteFiles(files)
	writeTestFile(t, filepath.Join(root, "create.txt"), "concurrent\n", 0o600)
	if failure := installWriteFiles(files); failure == nil || failure.code != "write_failed" {
		t.Fatalf("failure=%+v", failure)
	}
	assertWriteFile(t, filepath.Join(root, "edit.txt"), editContent, 0o600)
	assertWriteFile(t, filepath.Join(root, "delete.txt"), deleteContent, 0o640)
	assertWriteFile(t, filepath.Join(root, "create.txt"), "concurrent\n", 0o600)
}

func TestExecuteWriteRequestRejectsOverlapsAndDuplicateFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	content := "one\ntwo\nthree\n"
	writeTestFile(t, path, content, 0o600)
	hashes := hashline.Lines(content)
	overlap := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{Path: "file.txt", Changes: []writeChange{
		{HashRangeInclusive: []string{hashes[0], hashes[1]}, ContentLines: []string{"first"}},
		{HashRangeInclusive: []string{hashes[1], hashes[2]}, ContentLines: []string{"second"}},
	}}}}
	if _, failure := executeWriteRequest(root, false, overlap); failure == nil || failure.code != "overlapping_changes" {
		t.Fatalf("overlap failure=%+v", failure)
	}
	duplicate := writeRequest{Schema: writeSchema, Files: []writeRequestFile{
		{Path: "file.txt", Changes: []writeChange{{HashRangeInclusive: []string{hashes[0], hashes[0]}, ContentLines: []string{"first"}}}},
		{Path: "./file.txt", Changes: []writeChange{{HashRangeInclusive: []string{hashes[1], hashes[1]}, ContentLines: []string{"second"}}}},
	}}
	if _, failure := executeWriteRequest(root, false, duplicate); failure == nil || failure.code != "duplicate_path" {
		t.Fatalf("duplicate failure=%+v", failure)
	}
	assertWriteFile(t, path, content, 0o600)
}

func TestExecuteWriteRequestRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require privileges")
	}
	root := t.TempDir()
	outsideRoot := t.TempDir()
	outside := filepath.Join(outsideRoot, "outside.txt")
	writeTestFile(t, outside, "outside\n", 0o600)
	if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{Path: "escape.txt", Changes: []writeChange{{HashRangeInclusive: []string{"AAA", "AAA"}, ContentLines: []string{"changed"}}}}}}
	if _, failure := executeWriteRequest(root, false, request); failure == nil || failure.code != "invalid_path" {
		t.Fatalf("failure=%+v", failure)
	}
	if err := os.Symlink(outsideRoot, filepath.Join(root, "directory")); err != nil {
		t.Fatal(err)
	}
	directoryRequest := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{Path: "directory/outside.txt", Changes: []writeChange{{HashRangeInclusive: []string{"AAA", "AAA"}, ContentLines: []string{"changed"}}}}}}
	if _, failure := executeWriteRequest(root, false, directoryRequest); failure == nil || failure.code != "invalid_path" {
		t.Fatalf("directory symlink failure=%+v", failure)
	}
	assertWriteFile(t, outside, "outside\n", 0o600)
}

func TestExecuteWriteRequestRequiresExplicitContentLines(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	content := "before\n"
	writeTestFile(t, path, content, 0o600)
	hash := hashline.Lines(content)[0]
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{Path: "file.txt", Changes: []writeChange{{HashRangeInclusive: []string{hash, hash}}}}}}
	if _, failure := executeWriteRequest(root, false, request); failure == nil || failure.code != "invalid_content" {
		t.Fatalf("failure=%+v", failure)
	}
	assertWriteFile(t, path, content, 0o600)
}

func TestExecuteWriteRequestCreatesAndDeletesTransactionally(t *testing.T) {
	root := t.TempDir()
	deletedPath := filepath.Join(root, "deleted.txt")
	deletedContent := "remove me\n"
	writeTestFile(t, deletedPath, deletedContent, 0o600)
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{
		{Path: "created.txt", Operation: "create", ContentLines: []string{"alpha", "beta", ""}},
		{Path: "deleted.txt", Operation: "delete", BeforeSHA256: writeDigest([]byte(deletedContent))},
	}}
	response, failure := executeWriteRequest(root, false, request)
	if failure != nil || !response.Applied || len(response.Files) != 2 {
		t.Fatalf("response=%+v failure=%+v", response, failure)
	}
	assertWriteFile(t, filepath.Join(root, "created.txt"), "alpha\nbeta\n", 0o644)
	if _, err := os.Stat(deletedPath); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}
	if response.Files[0].Operation != "create" || len(response.Files[0].ChangeDetails) != 1 || len(response.Files[0].ChangeDetails[0].Anchors) == 0 {
		t.Fatalf("create response=%+v", response.Files[0])
	}
	if !strings.Contains(response.Files[0].Diff, "--- /dev/null") || !strings.Contains(response.Files[1].Diff, "+++ /dev/null") {
		t.Fatalf("missing create/delete diff: %+v", response.Files)
	}
}

func TestExecuteWriteRequestCreatesMissingDirectoryTree(t *testing.T) {
	root := t.TempDir()
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{
		Path: "generated/client/models.go", Operation: "create", ContentLines: []string{"package client", ""},
	}}}
	response, failure := executeWriteRequest(root, false, request)
	if failure != nil || !response.Applied {
		t.Fatalf("response=%+v failure=%+v", response, failure)
	}
	assertWriteFile(t, filepath.Join(root, "generated", "client", "models.go"), "package client\n", 0o644)
	for _, directory := range []string{"generated", filepath.Join("generated", "client")} {
		info, err := os.Stat(filepath.Join(root, directory))
		if err != nil || !info.IsDir() {
			t.Fatalf("created directory %q: info=%v err=%v", directory, info, err)
		}
	}
}

func TestWriteCreateDryRunDoesNotCreateDirectoryTree(t *testing.T) {
	root := t.TempDir()
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{
		Path: "predicted/deep/new.go", Operation: "create", ContentLines: []string{"package deep"},
	}}}
	response, failure := executeWriteRequest(root, true, request)
	if failure != nil || response.Applied || !response.DryRun {
		t.Fatalf("response=%+v failure=%+v", response, failure)
	}
	if _, err := os.Stat(filepath.Join(root, "predicted")); !os.IsNotExist(err) {
		t.Fatalf("dry run created directory tree: %v", err)
	}
}

func TestWriteCreateRollsBackCreatedDirectoryTree(t *testing.T) {
	root := t.TempDir()
	requests := []writeRequestFile{
		{Path: "generated/deep/first.go", Operation: "create", ContentLines: []string{"package deep"}},
		{Path: "blocked.go", Operation: "create", ContentLines: []string{"package blocked"}},
	}
	prepared, _, failure := prepareWriteFiles(root, requests)
	if failure != nil {
		t.Fatal(failure)
	}
	files := make([]*preparedWriteFile, 0, len(prepared))
	for index := range prepared {
		files = append(files, &prepared[index])
	}
	if failure := stageWriteFiles(files); failure != nil {
		t.Fatal(failure)
	}
	defer cleanupStagedWriteFiles(files)
	writeTestFile(t, filepath.Join(root, "blocked.go"), "concurrent\n", 0o600)
	if failure := installWriteFiles(files); failure == nil || failure.code != "write_failed" {
		t.Fatalf("failure=%+v", failure)
	}
	if _, err := os.Stat(filepath.Join(root, "generated")); !os.IsNotExist(err) {
		t.Fatalf("rollback retained created directory tree: %v", err)
	}
	assertWriteFile(t, filepath.Join(root, "blocked.go"), "concurrent\n", 0o600)
}

func TestWriteCreateRejectsMissingTreeThroughEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{
		Path: "linked/missing/new.go", Operation: "create", ContentLines: []string{"package escaped"},
	}}}
	if _, failure := executeWriteRequest(root, false, request); failure == nil || failure.code != "invalid_path" {
		t.Fatalf("failure=%+v", failure)
	}
	if _, err := os.Stat(filepath.Join(outside, "missing")); !os.IsNotExist(err) {
		t.Fatalf("escaping create mutated outside root: %v", err)
	}
}

func TestExecuteWriteRequestRejectsStaleDeleteWithoutCreating(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing.txt")
	writeTestFile(t, path, "current\n", 0o600)
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{
		{Path: "nested/new/created.txt", Operation: "create", ContentLines: []string{"new", ""}},
		{Path: "existing.txt", Operation: "delete", BeforeSHA256: strings.Repeat("0", 64)},
	}}
	if _, failure := executeWriteRequest(root, false, request); failure == nil || failure.code != "stale_file" {
		t.Fatalf("failure=%+v", failure)
	}
	if _, err := os.Stat(filepath.Join(root, "nested")); !os.IsNotExist(err) {
		t.Fatalf("create escaped rejected transaction: %v", err)
	}
	assertWriteFile(t, path, "current\n", 0o600)
}

func TestWriteDryRunReturnsPredictedAnchorsAndUnifiedDiff(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.go")
	content := "package sample\nfunc run() {}\n"
	writeTestFile(t, path, content, 0o600)
	hash := hashline.Lines(content)[1]
	request := writeRequest{Schema: writeSchema, Files: []writeRequestFile{{Path: "file.go", Changes: []writeChange{{HashRangeInclusive: []string{hash, hash}, ContentLines: []string{"func run() {", "}"}}}}}}
	response, failure := executeWriteRequest(root, true, request)
	if failure != nil || response.Applied || len(response.Files[0].ChangeDetails) != 1 || !response.Files[0].ChangeDetails[0].Predicted {
		t.Fatalf("response=%+v failure=%+v", response, failure)
	}
	if !strings.Contains(response.Files[0].Diff, "-func run() {}") || !strings.Contains(response.Files[0].Diff, "+func run() {") {
		t.Fatalf("diff=%q", response.Files[0].Diff)
	}
	var output bytes.Buffer
	if err := emitWriteResponse(&output, response, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "dry-run: no files changed") || !strings.Contains(output.String(), "│2│func run() {") || !strings.Contains(output.String(), "validated 1 files, 1 changes") {
		t.Fatalf("human output:\n%s", output.String())
	}
	assertWriteFile(t, path, content, 0o600)
}

func TestWriteCreateRejectsUnsafeContentAndTargets(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "existing.txt"), "current\n", 0o600)
	tests := []struct {
		name    string
		request writeRequestFile
		code    string
	}{
		{name: "nul", request: writeRequestFile{Path: "new.txt", Operation: "create", ContentLines: []string{"bad\x00value"}}, code: "invalid_content"},
		{name: "existing", request: writeRequestFile{Path: "existing.txt", Operation: "create", ContentLines: []string{"new"}}, code: "invalid_path"},
		{name: "parent file", request: writeRequestFile{Path: "existing.txt/new.txt", Operation: "create", ContentLines: []string{"new"}}, code: "invalid_path"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, failure := executeWriteRequest(root, false, writeRequest{Schema: writeSchema, Files: []writeRequestFile{test.request}})
			if failure == nil || failure.code != test.code {
				t.Fatalf("failure=%+v", failure)
			}
		})
	}
	assertWriteFile(t, filepath.Join(root, "existing.txt"), "current\n", 0o600)
}

func TestDecodeWriteRequestIsStrictAndBounded(t *testing.T) {
	valid := `{"schema":"grepple-write-v1","files":[{"path":"a","changes":[{"hash_range_inclusive":["AAA","AAA"],"content_lines":[]}]}]}`
	request, failure := decodeWriteRequest(strings.NewReader(valid))
	if failure != nil || request.Schema != writeSchema {
		t.Fatalf("request=%+v failure=%+v", request, failure)
	}
	for _, invalid := range []string{valid + `{}`, `{"schema":"grepple-write-v1","extra":true,"files":[]}`} {
		if _, failure := decodeWriteRequest(strings.NewReader(invalid)); failure == nil || failure.code != "invalid_request" {
			t.Fatalf("invalid=%q failure=%+v", invalid, failure)
		}
	}
}

func TestDecodeWriteHeredocRequestSupportsJSONFeatureParity(t *testing.T) {
	digest := strings.Repeat("a", 64)
	literalLine := "    return \"anything ' ` $ here\""
	input := fmt.Sprintf(`
::grepple file internal/foo.go
::grepple replace a83 e11 --end-marker GREPPLE_WRITE_ab12
func foo() {
%s
}
::grepple end
::grepple file literal.go
::grepple end GREPPLE_WRITE_ab12
::grepple replace c22
::grepple end

::grepple file internal/new file.go
::grepple create --end-marker CREATE_ab12
package sample

::grepple end CREATE_ab12

::grepple file internal/old.go
::grepple delete %s
::grepple end
`, literalLine, digest)
	request, failure := decodeWriteRequest(strings.NewReader(input))
	if failure != nil {
		t.Fatal(failure)
	}
	want := writeRequest{Schema: writeSchema, Files: []writeRequestFile{
		{Path: "internal/foo.go", Changes: []writeChange{
			{HashRangeInclusive: []string{"a83", "e11"}, ContentLines: []string{"func foo() {", literalLine, "}", "::grepple end", "::grepple file literal.go"}},
			{HashRangeInclusive: []string{"c22", "c22"}, ContentLines: []string{}},
		}},
		{Path: "internal/new file.go", Operation: "create", ContentLines: []string{"package sample", ""}},
		{Path: "internal/old.go", Operation: "delete", BeforeSHA256: digest},
	}}
	if !reflect.DeepEqual(request, want) {
		t.Fatalf("request=%#v\nwant=%#v", request, want)
	}
	crlf := strings.ReplaceAll(input, "\n", "\r\n")
	if request, failure := decodeWriteRequest(strings.NewReader(crlf)); failure != nil || !reflect.DeepEqual(request, want) {
		t.Fatalf("CRLF request=%#v failure=%+v", request, failure)
	}
}

func TestWriteHeredocCommandAppliesMixedTransaction(t *testing.T) {
	root := t.TempDir()
	editContent := "before\nsecond\n"
	deleteContent := "delete me\n"
	writeTestFile(t, filepath.Join(root, "edit.txt"), editContent, 0o640)
	writeTestFile(t, filepath.Join(root, "delete.txt"), deleteContent, 0o600)
	hash := hashline.Lines(editContent)[0]
	replacement := "after \" ' ` $ literal"
	request := fmt.Sprintf(`::grepple file edit.txt
::grepple replace %s
%s
::grepple end

::grepple file created.txt
::grepple create
created

::grepple end

::grepple file delete.txt
::grepple delete %s
`, hash, replacement, writeDigest([]byte(deleteContent)))
	withStdin(t, request, func() {
		captureStdout(t, func() {
			if err := Run([]string{"--root", root}, Dependencies{}); err != nil {
				t.Fatal(err)
			}
		})
	})
	assertWriteFile(t, filepath.Join(root, "edit.txt"), replacement+"\nsecond\n", 0o640)
	assertWriteFile(t, filepath.Join(root, "created.txt"), "created\n", 0o644)
	if _, err := os.Stat(filepath.Join(root, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists: %v", err)
	}
}

func TestWriteHeredocDryRunUsesStructuredOutputWithoutMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	content := "before\n"
	writeTestFile(t, path, content, 0o600)
	hash := hashline.Lines(content)[0]
	request := fmt.Sprintf("::grepple file file.txt\n::grepple replace %s\nafter\n::grepple end\n", hash)
	var output string
	withStdin(t, request, func() {
		output = captureStdout(t, func() {
			if err := Run([]string{"--root", root, "--dry-run", "--json"}, Dependencies{}); err != nil {
				t.Fatal(err)
			}
		})
	})
	var response writeResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	if response.Applied || !response.DryRun || len(response.Files) != 1 || !response.Files[0].Changed {
		t.Fatalf("response=%+v", response)
	}
	assertWriteFile(t, path, content, 0o600)
}

func TestDecodeWriteHeredocRequestRejectsMalformedEnvelopes(t *testing.T) {
	tests := []string{
		"::grepple replace AAA\n::grepple end\n",
		"::grepple file file.go\n::grepple unknown\n",
		"::grepple file file.go\n::grepple replace AAA\nunterminated\n",
		"::grepple file file.go\n::grepple replace AAA --end-marker TOKEN\n::grepple end OTHER\n",
		"::grepple file file.go\n::grepple create\n::grepple end\n::grepple replace AAA\n::grepple end\n",
		"::grepple file file.go\nnot a directive\n",
	}
	for _, input := range tests {
		if _, failure := decodeWriteRequest(strings.NewReader(input)); failure == nil || failure.code != "invalid_request" {
			t.Fatalf("input=%q failure=%+v", input, failure)
		}
	}
}

func TestLiteralWriteLinesTreatsTerminalNewlineAsSeparator(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{name: "empty deletes", want: []string{}},
		{name: "one line", content: "literal", want: []string{"literal"}},
		{name: "terminal newline", content: "literal\n", want: []string{"literal"}},
		{name: "intentional blank line", content: "literal\n\n", want: []string{"literal", ""}},
		{name: "crlf", content: "first\r\nsecond\r\n", want: []string{"first", "second"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := literalWriteLines([]byte(test.content))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("lines=%#v want=%#v", got, test.want)
			}
		})
	}
	if _, err := literalWriteLines([]byte("first\rsecond")); err == nil {
		t.Fatal("bare carriage return succeeded")
	}
}

func TestWriteEditReadsLiteralContentFromStdin(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.kt")
	before := "before\n"
	replacement := `val payload = """{"labels":["owner"]}"""` + "\n"
	writeTestFile(t, path, before, 0o600)
	hash := hashline.Lines(before)[0]
	withStdin(t, replacement, func() {
		captureStdout(t, func() {
			if err := Run([]string{"edit", "--root", root, "--path", "file.kt", "--start", hash}, Dependencies{}); err != nil {
				t.Fatal(err)
			}
		})
	})
	assertWriteFile(t, path, replacement, 0o600)
}

func TestWriteEditReadsLiteralContentFile(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "file.txt")
	payload := filepath.Join(t.TempDir(), "replacement.txt")
	before := "before\n"
	writeTestFile(t, target, before, 0o640)
	writeTestFile(t, payload, "first\nsecond\n", 0o600)
	hash := hashline.Lines(before)[0]
	captureStdout(t, func() {
		if err := Run([]string{"edit", "--root", root, "--path", "file.txt", "--start", hash, "--end", hash, "--content-file", payload}, Dependencies{}); err != nil {
			t.Fatal(err)
		}
	})
	assertWriteFile(t, target, "first\nsecond\n", 0o640)
}

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func assertWriteFile(t *testing.T, path, expected string, mode os.FileMode) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, []byte(expected)) {
		t.Fatalf("content=%q expected=%q", content, expected)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != mode.Perm() {
		t.Fatalf("mode=%o expected=%o", info.Mode().Perm(), mode.Perm())
	}
}

func TestWriteCommandReadsStrictJSONFromStdin(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	content := "before\n"
	writeTestFile(t, path, content, 0o600)
	hash := hashline.Lines(content)[0]
	request := fmt.Sprintf(`{"schema":"grepple-write-v1","files":[{"path":"file.txt","changes":[{"hash_range_inclusive":[%q,%q],"content_lines":["after"]}]}]}`, hash, hash)
	var output string
	withStdin(t, request, func() {
		output = captureStdout(t, func() {
			if err := Run([]string{"--root", root, "--json"}, Dependencies{}); err != nil {
				t.Fatal(err)
			}
		})
	})
	var response writeResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatalf("decode response %q: %v", output, err)
	}
	if !response.Applied || response.Schema != writeSchema {
		t.Fatalf("response=%+v", response)
	}
	assertWriteFile(t, path, "after\n", 0o600)
}

func TestWriteCommandDefaultsToEditReadyHumanOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	content := "before\n"
	writeTestFile(t, path, content, 0o600)
	hash := hashline.Lines(content)[0]
	request := fmt.Sprintf(`{"schema":"grepple-write-v1","files":[{"path":"file.txt","changes":[{"hash_range_inclusive":[%q,%q],"content_lines":["after"]}]}]}`, hash, hash)
	var output string
	withStdin(t, request, func() {
		output = captureStdout(t, func() {
			if err := Run([]string{"--root", root}, Dependencies{}); err != nil {
				t.Fatal(err)
			}
		})
	})
	updatedHash := hashline.Lines("after\n")[0]
	if !strings.Contains(output, "file.txt\n\n"+updatedHash+"│1│after\n") || !strings.Contains(output, "applied 1 files, 1 changes") {
		t.Fatalf("human output:\n%s", output)
	}
}

func TestParseWriteEditOptions(t *testing.T) {
	options, err := parseWriteOptions([]string{"edit", "--path", "file.kt", "--start", "Ab3"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.literalEdit || options.path != "file.kt" || options.start != "Ab3" || options.end != "Ab3" || options.contentFile != "-" {
		t.Fatalf("options=%+v", options)
	}
	for _, args := range [][]string{{"edit", "--start", "Ab3"}, {"edit", "--path", "file.kt"}, {"--path", "file.kt"}} {
		if _, err := parseWriteOptions(args); err == nil {
			t.Fatalf("args %v succeeded", args)
		}
	}
}

func TestWriteHelpDocumentsTransactionalInput(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"--help"}, Dependencies{}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"Usage: grepple write", "grepple write edit", "--content-file", "grepple-write-v1", "::grepple file", "--end-marker", "hash_range_inclusive", "operation", "before_sha256", "--dry-run", "--json", "HASH│LINE│content"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("write help missing %q:\n%s", expected, output)
		}
	}
}
