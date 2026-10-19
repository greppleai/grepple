package memory

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/parser"
)

const (
	scanLimit          = 256
	pageFileLimit      = 32
	pageBytesLimit     = 16 << 20
	metadataBytesLimit = 1 << 20
	referenceLimit     = 3
	selectorLimit      = 64
	matchEvidenceLimit = 4096
)

// MatchedFile is a selected, paged LOCAL content result. Lines and Spans are
// actual matches, never context, enclosing ranges, or pre-page candidates.
// Path is root-relative, not a display path from a remote result.
type MatchedFile struct {
	Path  string
	Lines []int
	Spans []LineSpan
}

// LineSpan covers the reported matched start/end lines (inclusive).
type LineSpan struct{ Start, End int }

// Reference is the bounded, body-free presentation of a qualified memory.
type Reference struct {
	Name        string
	Description string
}

// Discovery counts only qualified, inspected entries. An incomplete scan never
// claims that Omitted is a total for entries beyond its bounded universe.
type Discovery struct {
	References []Reference
	Omitted    int
	Complete   bool
}

type pageSource struct {
	lines             map[int]bool
	spans             []LineSpan
	content, checksum string
	readable          bool
	lineCount         int
	matched           bool
}

// Discover reads memory headers only. Invalid entries are skipped and reported
// as incomplete; a missing directory leaves the ordinary search output intact.
// No body, query text, or unselected source is read.
func Discover(root string, page []MatchedFile) Discovery {
	result := Discovery{Complete: true}
	if len(page) == 0 {
		return result
	}
	dir, err := directory(root, false)
	if errors.Is(err, os.ErrNotExist) {
		return result
	}
	if err != nil {
		result.Complete = false
		return result
	}
	defer dir.Close()
	handle, err := dir.Open(".")
	if err != nil {
		result.Complete = false
		return result
	}
	entries, readErr := handle.ReadDir(scanLimit + 1)
	_ = handle.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		result.Complete = false
		return result
	}
	if len(entries) > scanLimit {
		// ReadDir(n) exposes filesystem order; do not return a nondeterministic
		// subset when the directory exceeds the scan budget.
		result.Complete = false
		return result
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	memoryFile := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			memoryFile = true
			break
		}
	}
	if !memoryFile {
		return result
	}

	// Confine all caller paths and work before consulting headers or metadata.
	sources := make(map[string]*pageSource)
	totalBytes := int64(0)
	if len(page) > pageFileLimit*4 {
		result.Complete = false
		page = page[:pageFileLimit*4]
	}
	evidenceUsed := 0
	base, rootErr := os.OpenRoot(root)
	if rootErr != nil {
		result.Complete = false
		return result
	}
	defer base.Close()
	for _, file := range page {
		evidenceUsed += len(file.Lines) + len(file.Spans)
		if evidenceUsed > matchEvidenceLimit {
			result.Complete = false
			break
		}
		if len(file.Lines)+len(file.Spans) == 0 || !ValidPath(file.Path) {
			continue
		}
		if existing, found := sources[file.Path]; found {
			for _, line := range file.Lines {
				if line > 0 && line <= existing.lineCount {
					existing.lines[line] = true
				}
			}
			for _, span := range file.Spans {
				if span.Start >= 1 && span.End >= span.Start && span.End <= existing.lineCount {
					existing.spans = append(existing.spans, span)
				}
			}
			continue
		}
		if len(sources) >= pageFileLimit {
			result.Complete = false
			break
		}
		source := &pageSource{lines: map[int]bool{}, matched: true}
		for _, line := range file.Lines {
			if line > 0 {
				source.lines[line] = true
			}
		}
		source.spans = append(source.spans, file.Spans...)
		if len(source.lines)+len(source.spans) == 0 {
			continue
		}
		sources[file.Path] = source
		// Confined Lstat refuses symlinked final paths before charging the budget.
		info, e := base.Lstat(filepath.FromSlash(file.Path))
		if e != nil || !info.Mode().IsRegular() {
			continue
		}
		if info.Size() > SourceLimit || totalBytes+info.Size() > pageBytesLimit {
			result.Complete = false
			continue
		}
		totalBytes += info.Size()
		source.content, source.checksum, e = Source(root, file.Path)
		source.readable = e == nil
		if e != nil {
			result.Complete = false
			continue
		}
		totalBytes += int64(len(source.content)) - info.Size()
		if totalBytes > pageBytesLimit {
			result.Complete = false
			source.readable = false
			continue
		}
		source.lineCount = strings.Count(source.content, "\n") + 1
		if strings.HasSuffix(source.content, "\n") {
			source.lineCount--
		}
		for line := range source.lines {
			if line > source.lineCount {
				delete(source.lines, line)
			}
		}
		valid := source.spans[:0]
		for _, span := range source.spans {
			if span.Start >= 1 && span.End >= span.Start && span.End <= source.lineCount {
				valid = append(valid, span)
			}
		}
		source.spans = valid
		if len(source.lines)+len(source.spans) == 0 {
			source.readable = false
			source.matched = false
		}
	}
	if len(sources) == 0 {
		return result
	}

	// A metadata snapshot is optional. Read at most 1 MiB, and only when a
	// memory asks for labels; file-owned entries alone may confer membership.
	var document directorymeta.DocumentV1
	metadataLoaded := false
	metadataOK := false
	labels := func() {
		if metadataLoaded {
			return
		}
		metadataLoaded = true
		base, e := os.OpenRoot(root)
		if e != nil {
			result.Complete = false
			return
		}
		defer base.Close()
		file, e := base.Open(directorymeta.RepositoryFile)
		if errors.Is(e, os.ErrNotExist) {
			return
		}
		if e != nil {
			result.Complete = false
			return
		}
		defer file.Close()
		info, e := file.Stat()
		if e != nil || !info.Mode().IsRegular() {
			result.Complete = false
			return
		}
		if info.Size() > metadataBytesLimit {
			result.Complete = false
			return
		}
		content, e := io.ReadAll(io.LimitReader(file, metadataBytesLimit+1))
		if e != nil || len(content) > metadataBytesLimit {
			result.Complete = false
			return
		}
		document, e = directorymeta.ParseDocumentV1(content)
		if e != nil {
			return
		}
		metadataOK = true
	}
	candidates := make([]struct {
		ref  Reference
		rank int
	}, 0, referenceLimit)
	selectorUsed := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		if !ValidName(name) || !entry.Type().IsRegular() {
			result.Complete = false
			continue
		}
		info, e := entry.Info()
		if e != nil || info.Size() > EntryLimit {
			result.Complete = false
			continue
		}
		h, e := HeaderOnly(root, name)
		if e != nil {
			result.Complete = false
			continue
		}
		rank := 0
		for _, attachment := range h.Files {
			source := sources[attachment.Path]
			if source == nil || !source.readable || source.checksum != attachment.Checksum {
				continue
			}
			if len(attachment.Symbols) > 0 {
				language := parser.ScopeContentLanguageFor(attachment.Path)
				for _, value := range attachment.Symbols {
					scope, e := selector(value)
					if e != nil {
						continue
					}
					if !intersects(source, scope.StartLine, scope.EndLine) {
						continue
					}
					if selectorUsed >= selectorLimit {
						result.Complete = false
						continue
					}
					selectorUsed++
					if _, status := parser.ResolveScopeSelector(source.content, language, scope, nil); status == parser.ScopeVerified {
						rank = 5
						break
					}
				}
			}
			if len(attachment.LineRange) == 2 && attachment.LineRange[1] <= source.lineCount && intersects(source, attachment.LineRange[0], attachment.LineRange[1]) && rank < 4 {
				rank = 4
			}
			if len(attachment.Symbols) == 0 && len(attachment.LineRange) == 0 && rank < 2 {
				rank = 3
			}
		}
		if rank == 0 && len(h.DomainAreas)+len(h.FunctionalAreas)+len(h.Keywords) > 0 {
			labels()
			if metadataOK {
				for filename, source := range sources {
					if !source.readable {
						continue
					}
					key := path.Dir(filename)
					if key == "" {
						key = "."
					}
					for _, f := range document.Directory[key].Files {
						if f.Name != path.Base(filename) || f.Checksum != source.checksum {
							continue
						}
						if matchesCategory(h.DomainAreas, f.DomainAreas) && matchesCategory(h.FunctionalAreas, f.FunctionalAreas) && matchesCategory(h.Keywords, f.Keywords) {
							rank = 2
						}
						break
					}
					if rank > 0 {
						break
					}
				}
			}
		}
		if rank == 0 {
			for _, scope := range h.Scopes {
				for filename, source := range sources {
					if source.matched && ScopeMatches(scope, filename) {
						rank = 1
						break
					}
				}
				if rank > 0 {
					break
				}
			}
		}
		if rank > 0 {
			candidates = append(candidates, struct {
				ref  Reference
				rank int
			}{Reference{Name: h.Name, Description: h.Description}, rank})
		}
	}
	slices.SortFunc(candidates, func(a, b struct {
		ref  Reference
		rank int
	}) int {
		if a.rank > b.rank {
			return -1
		}
		if a.rank < b.rank {
			return 1
		}
		return strings.Compare(a.ref.Name, b.ref.Name)
	})
	for i, item := range candidates {
		if i < referenceLimit {
			result.References = append(result.References, item.ref)
		} else {
			result.Omitted++
		}
	}
	return result
}

func intersects(source *pageSource, start, end int) bool {
	for line := range source.lines {
		if line >= start && line <= end {
			return true
		}
	}
	for _, span := range source.spans {
		if span.Start <= end && span.End >= start {
			return true
		}
	}
	return false
}
func matchesCategory(wanted, actual []string) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, a := range actual {
		for _, w := range wanted {
			if directorymeta.LabelKey(a) == w {
				return true
			}
		}
	}
	return false
}
