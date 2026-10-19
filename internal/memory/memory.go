// Package memory owns bounded, repository-local Markdown memories. Search callers
// may read headers alone; only explicit recall reads the body.
package memory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/parser"
	"go.yaml.in/yaml/v3"
)

// HeaderLimit bounds the discoverable frontmatter of one memory.
const HeaderLimit = 8 << 10

// EntryLimit bounds an entire explicitly recalled memory file.
const EntryLimit = 64 << 10

// SourceLimit bounds a source read used for an attachment checksum.
const SourceLimit = 8 << 20

var validName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var validChecksum = regexp.MustCompile(`^[0-9a-f]{64}$`)
var validSelector = regexp.MustCompile(`^@([a-z]+):([1-9][0-9]*)-([1-9][0-9]*)$`)

// Attachment identifies a repository-root-relative selected source and its
// captured checksum; optional matched-line and selector bounds narrow evidence.
type Attachment struct {
	Path      string   `yaml:"path"`
	Checksum  string   `yaml:"checksum"`
	LineRange []int    `yaml:"lineRange,omitempty"`
	Symbols   []string `yaml:"symbols,omitempty"`
}

// Header contains only discoverable memory metadata, never Markdown body text.
type Header struct {
	Schema          string       `yaml:"schema"`
	Name            string       `yaml:"name"`
	Description     string       `yaml:"description"`
	DomainAreas     []string     `yaml:"domainAreas,omitempty"`
	FunctionalAreas []string     `yaml:"functionalAreas,omitempty"`
	Keywords        []string     `yaml:"keywords,omitempty"`
	Scopes          []string     `yaml:"scopes,omitempty"`
	Files           []Attachment `yaml:"files,omitempty"`
}

// ValidName accepts only safe lowercase memory file stems.
func ValidName(name string) bool { return validName.MatchString(name) }

// Root requires an actual local repository marker; FindRoot alone permits unmarked cwd.
func Root(cwd string) (string, error) {
	root, err := directorymeta.FindRoot(cwd)
	if err != nil {
		return "", err
	}
	for _, marker := range []string{".git", ".grepple"} {
		if _, err := os.Lstat(filepath.Join(root, marker)); err == nil {
			return root, nil
		}
	}
	return "", errors.New("memory requires a local repository (.git or .grepple)")
}

// ValidPath rejects absolute, escaped, Git-internal and memory-internal paths.
func ValidPath(value string) bool {
	return value != "" && value != "." && path.Clean(value) == value && !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "../") && value != ".." && !strings.ContainsAny(value, "\\:\x00") && utf8.ValidString(value) && value != ".grepple" && !strings.HasPrefix(value, ".grepple/") && value != ".git" && !strings.HasPrefix(value, ".git/")
}

func selector(value string) (parser.ScopeSelector, error) {
	match := validSelector.FindStringSubmatch(value)
	if match == nil || !parser.PublicScopeKind(match[1]) {
		return parser.ScopeSelector{}, fmt.Errorf("invalid exact scope selector %q", value)
	}
	start, e1 := strconv.Atoi(match[2])
	end, e2 := strconv.Atoi(match[3])
	if e1 != nil || e2 != nil || end < start {
		return parser.ScopeSelector{}, fmt.Errorf("invalid scope bounds %q", value)
	}
	return parser.ScopeSelector{Kind: match[1], StartLine: start, EndLine: end, Exact: true}, nil
}

func normalizeLabels(values []string) ([]string, error) {
	if len(values) > 16 {
		return nil, errors.New("too many labels")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !directorymeta.ValidLabelV1(value) {
			return nil, fmt.Errorf("invalid memory label %q", value)
		}
		key := directorymeta.LabelKey(value)
		if seen[key] {
			return nil, fmt.Errorf("duplicate memory label %q", value)
		}
		seen[key] = true
		out = append(out, key)
	}
	slices.Sort(out)
	return out, nil
}

// Normalize validates the strict v1 header and canonicalizes its labels and attachments.
func (h *Header) Normalize() error {
	if h.Schema != "grepple-memory-v1" || !ValidName(h.Name) {
		return errors.New("memory requires schema: grepple-memory-v1 and a safe name")
	}
	if !utf8.ValidString(h.Description) || len(h.Description) < 1 || len(h.Description) > 160 || strings.TrimSpace(h.Description) == "" {
		return errors.New("description must be 1-160 UTF-8 bytes")
	}
	for _, r := range h.Description {
		if unicode.IsControl(r) {
			return errors.New("description contains a control character")
		}
	}
	var err error
	if h.DomainAreas, err = normalizeLabels(h.DomainAreas); err != nil {
		return err
	}
	if h.FunctionalAreas, err = normalizeLabels(h.FunctionalAreas); err != nil {
		return err
	}
	if h.Keywords, err = normalizeLabels(h.Keywords); err != nil {
		return err
	}
	if len(h.Files) > 32 {
		return errors.New("too many file attachments")
	}
	if len(h.Scopes) > scopeLimit {
		return errors.New("too many memory scopes")
	}
	scopeSeen := map[string]bool{}
	for i, value := range h.Scopes {
		canonical, err := NormalizeScope(value)
		if err != nil {
			return err
		}
		if scopeSeen[canonical] {
			return fmt.Errorf("duplicate memory scope %q", canonical)
		}
		scopeSeen[canonical] = true
		h.Scopes[i] = canonical
	}
	slices.Sort(h.Scopes)
	if len(h.Files)+len(h.Scopes)+len(h.DomainAreas)+len(h.FunctionalAreas)+len(h.Keywords) == 0 {
		return errors.New("memory needs an attachment or a label")
	}
	seen := map[string]bool{}
	for i := range h.Files {
		f := &h.Files[i]
		if !ValidPath(f.Path) || !validChecksum.MatchString(f.Checksum) {
			return fmt.Errorf("invalid memory attachment %q", f.Path)
		}
		if len(f.LineRange) != 0 && (len(f.LineRange) != 2 || f.LineRange[0] < 1 || f.LineRange[1] < f.LineRange[0]) {
			return fmt.Errorf("invalid lineRange for %q", f.Path)
		}
		if len(f.Symbols) > 16 {
			return errors.New("too many symbol selectors")
		}
		for _, s := range f.Symbols {
			if _, err := selector(s); err != nil {
				return err
			}
		}
		slices.Sort(f.Symbols)
		if len(f.Symbols) > 1 {
			for j := 1; j < len(f.Symbols); j++ {
				if f.Symbols[j] == f.Symbols[j-1] {
					return errors.New("duplicate symbol selector")
				}
			}
		}
		key := fmt.Sprintf("%s/%v/%v", f.Path, f.LineRange, f.Symbols)
		if seen[key] {
			return fmt.Errorf("duplicate file attachment %q", f.Path)
		}
		seen[key] = true
	}
	slices.SortFunc(h.Files, func(a, b Attachment) int {
		return strings.Compare(fmt.Sprintf("%s/%v/%v", a.Path, a.LineRange, a.Symbols), fmt.Sprintf("%s/%v/%v", b.Path, b.LineRange, b.Symbols))
	})
	return nil
}

// ParseHeader reads at most HeaderLimit+1 bytes, without reading the body.
func ParseHeader(data []byte, expectedName string) (Header, int, error) {
	if len(data) > HeaderLimit {
		return Header{}, 0, errors.New("memory header exceeds 8 KiB")
	}
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return Header{}, 0, errors.New("missing memory frontmatter")
	}
	end := bytes.Index(data[4:], []byte("\n---\n"))
	if end < 0 {
		return Header{}, 0, errors.New("unterminated memory frontmatter")
	}
	end += 4
	source := data[4:end]
	if !utf8.Valid(source) || bytes.IndexByte(source, 0) >= 0 {
		return Header{}, 0, errors.New("invalid memory frontmatter text")
	}
	for _, r := range string(source) {
		if unicode.IsControl(r) && r != '\n' {
			return Header{}, 0, errors.New("control character in memory frontmatter")
		}
	}
	var node yaml.Node
	if err := yaml.Unmarshal(source, &node); err != nil {
		return Header{}, 0, err
	}
	if err := safeYAML(&node); err != nil {
		return Header{}, 0, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	decoder.KnownFields(true)
	var h Header
	if err := decoder.Decode(&h); err != nil {
		return Header{}, 0, fmt.Errorf("invalid memory frontmatter: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Header{}, 0, errors.New("multiple YAML documents in memory")
	}
	if h.Name != expectedName {
		return Header{}, 0, errors.New("memory filename and name disagree")
	}
	if err := h.Normalize(); err != nil {
		return Header{}, 0, err
	}
	return h, end + 5, nil
}

func safeYAML(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode || node.Style&yaml.TaggedStyle != 0 {
		return errors.New("memory YAML aliases and custom tags are forbidden")
	}
	for _, child := range node.Content {
		if err := safeYAML(child); err != nil {
			return err
		}
	}
	return nil
}

// Encode validates both the header and private body and produces canonical Markdown.
func Encode(h Header, body []byte) ([]byte, error) {
	if err := h.Normalize(); err != nil {
		return nil, err
	}
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 || len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("memory body must be nonempty UTF-8")
	}
	yamlBody, err := yaml.Marshal(h)
	if err != nil {
		return nil, err
	}
	if len(yamlBody)+9 > HeaderLimit {
		return nil, errors.New("memory header exceeds 8 KiB")
	}
	data := make([]byte, 0, len(yamlBody)+len(body)+9)
	data = append(data, []byte("---\n")...)
	data = append(data, yamlBody...)
	data = append(data, []byte("---\n")...)
	data = append(data, body...)
	if len(data) > EntryLimit {
		return nil, errors.New("memory exceeds 64 KiB")
	}
	return data, nil
}

// Source attaches a repository-confined regular file to a memory with its current digest.
func Source(root, relative string) (string, string, error) {
	if !ValidPath(relative) {
		return "", "", fmt.Errorf("invalid source path %q", relative)
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return "", "", err
	}
	defer dir.Close()
	info, err := dir.Lstat(relative)
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() {
		return "", "", errors.New("attachment must be a regular source file")
	}
	file, err := dir.Open(relative)
	if err != nil {
		return "", "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, SourceLimit+1))
	if err != nil {
		return "", "", err
	}
	if len(data) > SourceLimit || bytes.IndexByte(data, 0) >= 0 {
		return "", "", errors.New("attachment exceeds source limit or is binary")
	}
	digest := sha256.Sum256(data)
	return string(data), hex.EncodeToString(digest[:]), nil
}

// VerifyAttachment checks the current checksum and any exact line/selector bounds.
func VerifyAttachment(root string, f Attachment) error {
	content, digest, err := Source(root, f.Path)
	if err != nil {
		return err
	}
	if digest != f.Checksum {
		return errors.New("source checksum changed")
	}
	lines := strings.Count(content, "\n") + 1
	if strings.HasSuffix(content, "\n") {
		lines--
	}
	if len(f.LineRange) > 0 && f.LineRange[1] > lines {
		return errors.New("line range exceeds source")
	}
	language := parser.ScopeContentLanguageFor(f.Path)
	for _, value := range f.Symbols {
		s, err := selector(value)
		if err != nil {
			return err
		}
		if _, status := parser.ResolveScopeSelector(content, language, s, nil); status != parser.ScopeVerified {
			return fmt.Errorf("unverified symbol selector %q", value)
		}
	}
	return nil
}
