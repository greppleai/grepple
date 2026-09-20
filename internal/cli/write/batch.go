package write

import (
	"fmt"
	"strings"
)

const writeHeredocDirective = "::grepple"

func decodeHeredocWriteRequest(content []byte) (writeRequest, *writeFailure) {
	lines, _, err := writeLogicalLines(content)
	if err != nil {
		return writeRequest{}, newWriteFailure("invalid_request", fmt.Sprintf("decode heredoc request: %v", err), "", nil)
	}
	decoder := writeHeredocDecoder{lines: lines, request: writeRequest{Schema: writeSchema, Files: []writeRequestFile{}}, current: -1}
	return decoder.decode()
}

type writeHeredocDecoder struct {
	lines   []string
	request writeRequest
	current int
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
			return writeRequest{}, decoder.failure(index+1, "expected a ::grepple directive")
		}
		next, failure := decoder.applyDirective(index, command, arguments)
		if failure != nil {
			return writeRequest{}, failure
		}
		index = next
	}
	if len(decoder.request.Files) == 0 {
		return writeRequest{}, writeHeredocFailure(1, "", "request requires at least one file")
	}
	return decoder.request, nil
}

func (decoder *writeHeredocDecoder) applyDirective(index int, command, arguments string) (int, *writeFailure) {
	switch command {
	case "file":
		return index + 1, decoder.addFile(index+1, arguments)
	case "replace":
		return decoder.addReplace(index, arguments)
	case "create":
		return decoder.addCreate(index, arguments)
	case "delete":
		return index + 1, decoder.addDelete(index+1, arguments)
	case "end":
		return index + 1, decoder.closeFile(index+1, arguments)
	default:
		return 0, decoder.failure(index+1, fmt.Sprintf("unknown directive %q", command))
	}
}

func (decoder *writeHeredocDecoder) addFile(line int, path string) *writeFailure {
	if path == "" {
		return writeHeredocFailure(line, "", "file requires a path")
	}
	decoder.request.Files = append(decoder.request.Files, writeRequestFile{Path: path})
	decoder.current = len(decoder.request.Files) - 1
	return nil
}

func (decoder *writeHeredocDecoder) addReplace(index int, arguments string) (int, *writeFailure) {
	file, failure := decoder.currentFile(index+1, "replace")
	if failure != nil {
		return 0, failure
	}
	if file.Operation != "" {
		return 0, decoder.failure(index+1, "replace cannot follow create or delete")
	}
	start, end, marker, err := parseWriteHeredocReplace(arguments)
	if err != nil {
		return 0, decoder.failure(index+1, err.Error())
	}
	body, next, failure := readWriteHeredocBody(decoder.lines, index+1, marker, file.Path)
	if failure != nil {
		return 0, failure
	}
	file.Changes = append(file.Changes, writeChange{HashRangeInclusive: []string{start, end}, ContentLines: body})
	return next, nil
}

func (decoder *writeHeredocDecoder) addCreate(index int, arguments string) (int, *writeFailure) {
	file, failure := decoder.currentFile(index+1, "create")
	if failure != nil {
		return 0, failure
	}
	if file.Operation != "" || len(file.Changes) != 0 || file.ContentLines != nil {
		return 0, decoder.failure(index+1, "create must be the file's only operation")
	}
	marker, err := parseWriteHeredocMarker(arguments)
	if err != nil {
		return 0, decoder.failure(index+1, err.Error())
	}
	body, next, failure := readWriteHeredocBody(decoder.lines, index+1, marker, file.Path)
	if failure != nil {
		return 0, failure
	}
	file.Operation = "create"
	file.ContentLines = body
	return next, nil
}

func (decoder *writeHeredocDecoder) addDelete(line int, arguments string) *writeFailure {
	file, failure := decoder.currentFile(line, "delete")
	if failure != nil {
		return failure
	}
	fields := strings.Fields(arguments)
	if len(fields) != 1 {
		return decoder.failure(line, "delete requires exactly one SHA-256 digest")
	}
	if file.Operation != "" || len(file.Changes) != 0 || file.ContentLines != nil {
		return decoder.failure(line, "delete must be the file's only operation")
	}
	file.Operation = "delete"
	file.BeforeSHA256 = fields[0]
	return nil
}

func (decoder *writeHeredocDecoder) closeFile(line int, arguments string) *writeFailure {
	if arguments != "" {
		return decoder.failure(line, "unexpected named end outside a literal body")
	}
	if decoder.current < 0 {
		return writeHeredocFailure(line, "", "end requires an open file")
	}
	decoder.current = -1
	return nil
}

func (decoder *writeHeredocDecoder) currentFile(line int, operation string) (*writeRequestFile, *writeFailure) {
	if decoder.current < 0 {
		return nil, writeHeredocFailure(line, "", operation+" requires a preceding file directive")
	}
	return &decoder.request.Files[decoder.current], nil
}

func (decoder *writeHeredocDecoder) failure(line int, message string) *writeFailure {
	return writeHeredocFailure(line, currentWriteHeredocPath(decoder.request, decoder.current), message)
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
	if len(fields) == 0 {
		return "", "", "", fmt.Errorf("replace requires a start anchor")
	}
	start, end := fields[0], fields[0]
	position := 1
	if position < len(fields) && fields[position] != "--end-marker" {
		end = fields[position]
		position++
	}
	marker, err := parseWriteHeredocMarkerFields(fields[position:])
	if err != nil {
		return "", "", "", err
	}
	return start, end, marker, nil
}

func parseWriteHeredocMarker(arguments string) (string, error) {
	return parseWriteHeredocMarkerFields(strings.Fields(arguments))
}

func parseWriteHeredocMarkerFields(fields []string) (string, error) {
	if len(fields) == 0 {
		return "", nil
	}
	if len(fields) != 2 || fields[0] != "--end-marker" || fields[1] == "" {
		return "", fmt.Errorf("expected optional --end-marker TOKEN")
	}
	return fields[1], nil
}

func readWriteHeredocBody(lines []string, start int, marker, path string) ([]string, int, *writeFailure) {
	terminator := writeHeredocDirective + " end"
	if marker != "" {
		terminator += " " + marker
	}
	for index := start; index < len(lines); index++ {
		if lines[index] != terminator {
			continue
		}
		body := append([]string{}, lines[start:index]...)
		return body, index + 1, nil
	}
	return nil, 0, writeHeredocFailure(start+1, path, fmt.Sprintf("literal body is missing terminator %q", terminator))
}

func currentWriteHeredocPath(request writeRequest, current int) string {
	if current < 0 || current >= len(request.Files) {
		return ""
	}
	return request.Files[current].Path
}

func writeHeredocFailure(line int, path, message string) *writeFailure {
	return newWriteFailure("invalid_request", fmt.Sprintf("heredoc line %d: %s", line, message), path, nil)
}
