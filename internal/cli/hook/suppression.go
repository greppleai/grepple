package hook

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/greppleai/grepple/internal/parser"
)

// hookSuppressions contains source-authored directives keyed by the comment's
// line. A leading directive applies only across a contiguous comment block;
// code or a blank line ends the attachment.
type hookSuppressions struct {
	directives  map[int]map[string]bool
	commentOnly map[int]bool
}

func filterSuppressedFindings(root string, findings []Finding) ([]Finding, error) {
	if len(findings) == 0 {
		return findings, nil
	}
	repository, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("read hook suppressions: %w", err)
	}
	defer repository.Close()

	byPath := make(map[string]hookSuppressions)
	filtered := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		suppressions, ok := byPath[finding.Path]
		if !ok {
			suppressions, err = readHookSuppressions(repository, finding.Path)
			if err != nil {
				return nil, err
			}
			byPath[finding.Path] = suppressions
		}
		if !suppressions.applies(finding.Line, finding.ID) {
			filtered = append(filtered, finding)
		}
	}
	return filtered, nil
}

func readHookSuppressions(root *os.Root, path string) (hookSuppressions, error) {
	file, err := root.Open(path)
	if err != nil {
		return hookSuppressions{}, fmt.Errorf("read hook suppressions in %s: %w", path, err)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, maxCachedSourceBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return hookSuppressions{}, fmt.Errorf("read hook suppressions in %s: %w", path, readErr)
	}
	if closeErr != nil {
		return hookSuppressions{}, fmt.Errorf("close hook source in %s: %w", path, closeErr)
	}
	if len(content) > maxCachedSourceBytes {
		return hookSuppressions{}, fmt.Errorf("hook source %s exceeds %d bytes", path, maxCachedSourceBytes)
	}
	result := hookSuppressions{}
	if !strings.Contains(string(content), "//grepple") {
		return result, nil
	}
	doc, err := parser.ParseDocument(parser.LanguageFor(path), string(content))
	if err != nil {
		return result, fmt.Errorf("parse hook suppressions in %s: %w", path, err)
	}
	defer doc.Close()
	result.directives = make(map[int]map[string]bool)
	result.commentOnly = make(map[int]bool)
	parser.WalkNamed(doc.Root(), func(node parser.Node) {
		if node.Kind() != "comment" && node.Kind() != "line_comment" {
			return
		}
		text := strings.TrimSuffix(node.Text(), "\r")
		if !strings.HasPrefix(text, "//") || strings.ContainsAny(text, "\r\n") {
			return
		}
		rng := node.Range()
		line := rng.Start.Line
		prefix := content[:rng.StartByte]
		if start := strings.LastIndexByte(string(prefix), '\n'); start >= 0 {
			prefix = prefix[start+1:]
		}
		if strings.TrimSpace(string(prefix)) == "" {
			result.commentOnly[line] = true
		}
		if !strings.HasPrefix(text, "//grepple ") && !strings.HasPrefix(text, "//grepple\t") {
			return
		}
		fields := strings.Fields(strings.TrimPrefix(text, "//grepple"))
		if len(fields) < 2 || !hookID.MatchString(fields[0]) {
			return // missing hook ID or required reason: no suppression
		}
		if result.directives[line] == nil {
			result.directives[line] = make(map[string]bool)
		}
		result.directives[line][fields[0]] = true
	})
	return result, nil
}

func (s hookSuppressions) applies(line int, id string) bool {
	if s.directives[line][id] {
		return true // trailing comment on the finding's line
	}
	for previous := line - 1; s.commentOnly[previous]; previous-- {
		if s.directives[previous][id] {
			return true
		}
	}
	return false
}
