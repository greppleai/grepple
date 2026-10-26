package write

import (
	"fmt"
	"strings"

	"github.com/greppleai/grepple/internal/hashline"
)

const writeHeredocDirective = "::grepple"

func decodeHeredocWriteRequest(content []byte) (writeRequest, *writeFailure) {
	lines, _, err := writeLogicalLines(content)
	if err != nil {
		return writeRequest{}, newWriteFailure("invalid_request", fmt.Sprintf("decode heredoc request: %v", err), "", nil)
	}
	decoder := writeHeredocDecoder{lines: lines, request: writeRequest{Schema: writeSchema, Files: []writeRequestFile{}}}
	return decoder.decode()
}

type writeHeredocDecoder struct {
	lines   []string
	request writeRequest
}

func (decoder *writeHeredocDecoder) decode() (writeRequest, *writeFailure) {
	for index := 0; index < len(decoder.lines); {
		line := decoder.lines[index]
		if strings.TrimSpace(line) == "" {
			index++
			continue
		}
		command, arguments, ok := parseWriteHeredocDirective(line)
		if !ok {
			return writeRequest{}, writeHeredocFailure(index+1, "", "expected a ::grepple directive")
		}
		var next int
		var failure *writeFailure
		switch command {
		case "replace":
			next, failure = decoder.addReplace(index, arguments)
		case "create":
			next, failure = decoder.addCreate(index, arguments)
		case "delete":
			next, failure = decoder.addDelete(index, arguments)
		default:
			failure = writeHeredocFailure(index+1, "", fmt.Sprintf("unknown directive %q", command))
		}
		if failure != nil {
			return writeRequest{}, failure
		}
		index = next
	}
	if len(decoder.request.Files) == 0 {
		return writeRequest{}, writeHeredocFailure(1, "", "request requires at least one operation")
	}
	return decoder.request, nil
}

func (decoder *writeHeredocDecoder) addReplace(index int, arguments string) (int, *writeFailure) {
	path, start, end, err := parseWriteHeredocReplace(arguments)
	if err != nil {
		return 0, writeHeredocFailure(index+1, "", err.Error())
	}
	file := decoder.editFile(path)
	if file.Operation != "" {
		return 0, writeHeredocFailure(index+1, path, "replace cannot follow create or delete")
	}
	body, next := readWriteHeredocBody(decoder.lines, index+1)
	file.Changes = append(file.Changes, writeChange{HashRangeInclusive: []string{start, end}, ContentLines: body})
	return next, nil
}

func (decoder *writeHeredocDecoder) editFile(path string) *writeRequestFile {
	// Nonadjacent edits of the same file share the original snapshot and must
	// produce one request file; the write engine rejects overlapping ranges.
	for index := range decoder.request.Files {
		if decoder.request.Files[index].Path == path {
			return &decoder.request.Files[index]
		}
	}
	decoder.request.Files = append(decoder.request.Files, writeRequestFile{Path: path})
	return &decoder.request.Files[len(decoder.request.Files)-1]
}

func (decoder *writeHeredocDecoder) addCreate(index int, arguments string) (int, *writeFailure) {
	path := strings.TrimSpace(arguments)
	if path == "" {
		return 0, writeHeredocFailure(index+1, "", "create requires a path")
	}
	for _, file := range decoder.request.Files {
		if file.Path == path {
			return 0, writeHeredocFailure(index+1, path, "create must be the file's only operation")
		}
	}
	body, next := readWriteHeredocBody(decoder.lines, index+1)
	decoder.request.Files = append(decoder.request.Files, writeRequestFile{Path: path, Operation: "create", ContentLines: body})
	return next, nil
}

func (decoder *writeHeredocDecoder) addDelete(index int, arguments string) (int, *writeFailure) {
	fields := strings.Fields(arguments)
	if len(fields) < 2 {
		return 0, writeHeredocFailure(index+1, "", "delete requires a path and SHA-256 digest")
	}
	path := strings.Join(fields[:len(fields)-1], " ")
	for _, file := range decoder.request.Files {
		if file.Path == path {
			return 0, writeHeredocFailure(index+1, path, "delete must be the file's only operation")
		}
	}
	decoder.request.Files = append(decoder.request.Files, writeRequestFile{Path: path, Operation: "delete", BeforeSHA256: fields[len(fields)-1]})
	return index + 1, nil
}

func parseWriteHeredocDirective(line string) (string, string, bool) {
	if line == writeHeredocDirective {
		return "", "", true
	}
	if !strings.HasPrefix(line, writeHeredocDirective+" ") && !strings.HasPrefix(line, writeHeredocDirective+"\t") {
		return "", "", false
	}
	value := strings.TrimSpace(line[len(writeHeredocDirective):])
	separator := strings.IndexAny(value, " \t")
	if separator < 0 {
		return value, "", true
	}
	return value[:separator], strings.TrimSpace(value[separator:]), true
}

func parseWriteHeredocReplace(arguments string) (string, string, string, error) {
	fields := strings.Fields(arguments)
	if len(fields) < 2 {
		return "", "", "", fmt.Errorf("replace requires a path and start anchor")
	}
	// A second trailing three-character hash is the optional end anchor.
	// A path whose final component is itself a three-character token is
	// ambiguous; use the JSON request for such paths.
	last := len(fields) - 1
	if !hashline.Valid(fields[last]) {
		return "", "", "", fmt.Errorf("replace requires a valid three-character anchor")
	}
	if len(fields) >= 3 && hashline.Valid(fields[last-1]) {
		path := strings.Join(fields[:last-1], " ")
		return path, fields[last-1], fields[last], nil
	}
	return strings.Join(fields[:last], " "), fields[last], fields[last], nil
}

func readWriteHeredocBody(lines []string, start int) ([]string, int) {
	for index := start; index < len(lines); index++ {
		if strings.HasPrefix(lines[index], writeHeredocDirective) {
			return append([]string{}, lines[start:index]...), index
		}
	}
	end := len(lines)
	if end > start && lines[end-1] == "" {
		end-- // The terminal newline separates lines; it is not a body line.
	}
	return append([]string{}, lines[start:end]...), len(lines)
}

func writeHeredocFailure(line int, path, message string) *writeFailure {
	return newWriteFailure("invalid_request", fmt.Sprintf("heredoc line %d: %s", line, message), path, nil)
}
