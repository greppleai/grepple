package search

import (
	"bytes"
	"context"
	"github.com/greppleai/grepple/api"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/greppleai/grepple/parser"
)

// readInvocations counts candidate files whose contents were read, so tests can
// assert that a bounded (windowed) query stops reading once the window is full
// instead of scanning every candidate.
var readInvocations atomic.Int64

// SplitLines splits content into lines, dropping the trailing empty element a
// final newline produces (so line numbers match editor expectations).
func SplitLines(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func displayPath(file string) string {
	cwd, _ := os.Getwd()
	return displayPathFrom(file, cwd)
}

// displayPathFrom renders file relative to a caller-supplied cwd. Hot loops
// resolve cwd once and reuse it here instead of calling os.Getwd() per file
// (an unnecessary syscall + alloc for every candidate; cwd is fixed during a
// search).
func displayPathFrom(file, cwd string) string {
	r, e := filepath.Rel(cwd, file)
	if e != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return file
	}
	if r == "." {
		return filepath.Base(file)
	}
	return filepath.ToSlash(r)
}

func matcher(p Params) (func(string) bool, error) {
	if p.Regex {
		flags := ""
		if p.IgnoreCase {
			flags = "(?i)"
		}
		r, e := regexp.Compile(flags + p.Query)
		if e != nil {
			return nil, e
		}
		return r.MatchString, nil
	}
	needle := p.Query
	if p.IgnoreCase {
		needle = strings.ToLower(needle)
	}
	// For case-insensitive substring search the caller folds each line once per
	// file (see the subject setup in Files) instead of this closure calling
	// strings.ToLower on every line, so here we only fold the needle.
	return func(s string) bool {
		return strings.Contains(s, needle)
	}, nil
}

// RepoID extracts the owner/repo prefix from a display path, or "" when the
// path is not repo-qualified (e.g. a plain relative path in a local search).
func RepoID(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	if len(parts) > 0 {
		return parts[0]
	}
	return path
}

// repoPattern is a single repo matcher: a filepath.Match glob plus the compiled
// anchored regexp form. Compiling the regexp once (here) instead of on every
// call is the whole point — repoMatches used to regexp.Compile per candidate.
type repoPattern struct {
	glob string
	re   *regexp.Regexp
}

func compileRepoPattern(pattern string) repoPattern {
	p := repoPattern{glob: pattern}
	if re, err := regexp.Compile("^(?:" + pattern + ")$"); err == nil {
		p.re = re
	}
	return p
}

func (p repoPattern) match(repo string) bool {
	if matched, err := filepath.Match(p.glob, repo); err == nil && matched {
		return true
	}
	return p.re != nil && p.re.MatchString(repo)
}

func compileRepoPatterns(patterns []string) []repoPattern {
	if len(patterns) == 0 {
		return nil
	}
	out := make([]repoPattern, 0, len(patterns))
	for _, pattern := range patterns {
		out = append(out, compileRepoPattern(pattern))
	}
	return out
}

// RepoFilter is a reusable, precompiled include/exclude repository matcher. Build
// it once per request and reuse it across a shard's repos / a query's candidates
// instead of recompiling pattern regexps on every call.
type RepoFilter struct {
	include []repoPattern
	exclude []repoPattern
}

// NewRepoFilter compiles the include (--repo) and exclude (--exclude-repo)
// patterns once.
func NewRepoFilter(include, exclude []string) *RepoFilter {
	return &RepoFilter{include: compileRepoPatterns(include), exclude: compileRepoPatterns(exclude)}
}

// Allow reports whether repo passes the filter: it must match an include pattern
// (or there are none) and must not match any exclude pattern. A nil filter
// allows everything. This mirrors repoMatches(repo, include) &&
// !repoMatchesAny(repo, exclude).
func (f *RepoFilter) Allow(repo string) bool {
	if f == nil {
		return true
	}
	if len(f.include) > 0 {
		matched := false
		for _, p := range f.include {
			if p.match(repo) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, p := range f.exclude {
		if p.match(repo) {
			return false
		}
	}
	return true
}

// repoMatches / repoMatchesAny retain the original one-shot API (used by the
// exported RepoMatches/RepoMatchesAny wrappers). They compile per call, so hot
// paths use a prebuilt RepoFilter instead.
func repoMatches(repo string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if compileRepoPattern(pattern).match(repo) {
			return true
		}
	}
	return false
}

func repoMatchesAny(repo string, patterns []string) bool {
	return len(patterns) > 0 && repoMatches(repo, patterns)
}

// Files runs the content search over candidates (or discovers files itself
// when candidates is nil): matches every candidate against the pattern and repo
// filters, in deterministic display-path order, windowed by p.Skip/p.Limit.
func Files(p Params, candidates []string) ([]FileMatch, error) {
	m, e := matcher(p)
	if e != nil {
		return nil, e
	}
	files := candidates
	if files == nil {
		files, e = collectCandidateFiles(p.Globs, p.Root)
		if e != nil {
			return nil, e
		}
	}
	// Read candidates in the deterministic output order and, for a bounded query,
	// stop once the window is full instead of reading every candidate. Zoekt can
	// return thousands of candidates while --limit keeps only a handful, so this
	// avoids reading the long tail. --count and unbounded queries (Limit 0) still
	// scan everything, since they need the complete match set.
	limit := resultLimit(p)
	scan := candidateScan{p: p, m: m, repoFilter: NewRepoFilter(p.Repo, p.ExcludeRepo), fromIndex: candidates != nil}
	var out []FileMatch
	if limit <= 0 {
		out = scan.scanAll(files)
	} else {
		out = scan.scanWindowed(files, p.Skip+limit)
	}
	out = applyResultWindow(out, p)
	if p.Related {
		attachRelated(out, scan.relatedFiles(files), p.FollowRelated)
	}
	// Skip the structural parse entirely when the caller will not render
	// segments (--count, --files, --line-only, --context, --json-matches): those
	// modes need only match lines, so parsing every returned file is wasted work.
	if !p.SkipSegments {
		runParallel(len(out), func(index int) {
			analyzeMatchStructure(&out[index], p)
		})
	}
	return out, nil
}

// candidateScan bundles the per-candidate scan inputs (params, matcher, repo
// filter, and whether candidates came from the index) shared by both scan
// strategies below.
type candidateScan struct {
	p          Params
	m          func(string) bool
	repoFilter *RepoFilter
	fromIndex  bool
}

func (s candidateScan) relatedFiles(files []string) []string {
	cwd, _ := os.Getwd()
	filtered := make([]string, 0, len(files))
	for _, file := range files {
		display := displayPathFrom(file, cwd)
		if s.fromIndex && (!withinRoot(file, s.p.Root) || !pathMatchesGlobs(display, s.p.Globs) || pathContainsGitDirectory(file)) {
			continue
		}
		if s.repoFilter.Allow(RepoID(display)) {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// scanAll reads every candidate in parallel and sorts by display path —
// deterministic order (owner/repo/dir/file), stable and reproducible across
// shards and reindexes. There is no relevance ranking: an agent narrows with
// --repo / a tighter pattern rather than relying on an opaque score.
func (s candidateScan) scanAll(files []string) []FileMatch {
	cwd, _ := os.Getwd()
	results := make([]*FileMatch, len(files))
	runParallel(len(files), func(index int) {
		results[index] = scanCandidate(s.p, s.m, files[index], displayPathFrom(files[index], cwd), s.repoFilter, s.fromIndex)
	})
	out := collectMatches(results)
	sortMatches(out)
	return out
}

// scanWindowed sorts candidates by display path up front (so matches are emitted
// in final order) and reads them in batches, stopping once enough matched files
// have accumulated for the skip+limit window.
func (s candidateScan) scanWindowed(files []string, target int) []FileMatch {
	type candidate struct{ file, display string }
	cwd, _ := os.Getwd()
	cands := make([]candidate, len(files))
	for i, f := range files {
		cands[i] = candidate{file: f, display: displayPathFrom(f, cwd)}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].display < cands[j].display })
	var out []FileMatch
	batch := WorkerCount() * 8
	if batch < 1 {
		batch = 1
	}
	for start := 0; start < len(cands) && len(out) < target; start += batch {
		end := min(start+batch, len(cands))
		results := make([]*FileMatch, end-start)
		runParallel(end-start, func(k int) {
			c := cands[start+k]
			results[k] = scanCandidate(s.p, s.m, c.file, c.display, s.repoFilter, s.fromIndex)
		})
		out = append(out, collectMatches(results)...)
	}
	return out // already ordered by display path
}

// StdinPath is the virtual display path for piped standard input: when stdin
// is a pipe or redirect (not a terminal) and no path/glob argument is given,
// grepple searches the stream instead of walking the filesystem (grep/ripgrep
// behavior) and reports matches under this name.
const StdinPath = "<stdin>"

// Content searches in-memory content as one virtual file (used for piped
// stdin). It applies the same matching, binary (NUL) skip, and structural
// segment rules as file search; the virtual file has no extension, so segments
// use the plain-text fallback. Returns nil (and no error) when nothing matches
// or the content is binary.
func Content(p Params, name string, content []byte) (*FileMatch, error) {
	m, e := matcher(p)
	if e != nil {
		return nil, e
	}
	fm := scanContent(p, m, content, name, name)
	if fm != nil && !p.SkipSegments {
		analyzeMatchStructure(fm, p)
	}
	return fm, nil
}

// scanCandidate applies the in-memory guards, reads the file, and returns a
// FileMatch when the content matches or nil to skip. fromIndex is true when the
// candidates came from the Zoekt index (git-tracked, already inside the repo and
// not .gitignored), letting us skip the path guards that only matter for a local
// filesystem walk.
func scanCandidate(p Params, m func(string) bool, file, display string, repoFilter *RepoFilter, fromIndex bool) *FileMatch {
	if fromIndex {
		if !withinRoot(file, p.Root) || !pathMatchesGlobs(display, p.Globs) || pathContainsGitDirectory(file) {
			return nil
		}
	}
	if !repoFilter.Allow(RepoID(display)) {
		return nil
	}
	readInvocations.Add(1)
	contentBytes, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	return scanContent(p, m, contentBytes, file, display)
}

// scanContent matches in-memory bytes line by line. It returns nil for binary
// content (NUL byte) and when nothing matches.
func scanContent(p Params, m func(string) bool, contentBytes []byte, file, display string) *FileMatch {
	// Detect NUL (binary) on the raw bytes — no string copy — then convert once.
	if bytes.IndexByte(contentBytes, 0) >= 0 {
		return nil
	}
	content := strings.ToValidUTF8(string(contentBytes), "\uFFFD")
	lines := SplitLines(content)
	// For case-insensitive substring search, fold the whole file once rather than
	// once per line inside the matcher (identical results, far fewer allocations).
	subject := lines
	if p.IgnoreCase && !p.Regex {
		subject = SplitLines(strings.ToLower(content))
	}
	hits := map[int]bool{}
	for lineIndex, line := range subject {
		if m(line) != p.InvertMatch {
			hits[lineIndex+1] = true
		}
	}
	if len(hits) == 0 {
		return nil
	}
	return &FileMatch{
		File:        file,
		DisplayPath: display,
		Content:     content,
		Language:    parser.LanguageFor(file),
		MatchLines:  hits,
	}
}

// collectMatches drops the nil (skipped) entries, preserving order.
func collectMatches(results []*FileMatch) []FileMatch {
	out := make([]FileMatch, 0, len(results))
	for _, r := range results {
		if r != nil {
			out = append(out, *r)
		}
	}
	return out
}

// ListFilePaths returns the candidate paths matching the request's globs and
// repo filters, in deterministic display-path order, windowed by p.Skip/p.Limit.
// It backs --files listings and never reads file contents.
func ListFilePaths(p Params, candidates []string) ([]string, error) {
	return ListFilePathsContext(context.Background(), p, candidates)
}

// ListFilePathsContext is ListFilePaths with cancellation during discovery.
func ListFilePathsContext(ctx context.Context, p Params, candidates []string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	files := candidates
	var e error
	if files == nil {
		files, e = collectListingFilesContext(ctx, p.Globs, p.Root)
		if e != nil {
			return nil, e
		}
	}
	out, err := listingDisplayPaths(ctx, p, files, candidates != nil)
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return applyPathWindow(out, p), nil
}
func listingDisplayPaths(ctx context.Context, p Params, files []string, supplied bool) ([]string, error) {
	repoFilter := NewRepoFilter(p.Repo, p.ExcludeRepo)
	cwd, _ := os.Getwd()
	var out []string
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		display := displayPathFrom(file, cwd)
		if supplied && (!withinRoot(file, p.Root) || !pathMatchesGlobs(display, p.Globs)) {
			continue
		}
		if repoFilter.Allow(RepoID(display)) {
			out = append(out, display)
		}
	}
	return out, nil
}

func applyPathWindow(paths []string, p Params) []string {
	if p.Skip > 0 {
		if p.Skip >= len(paths) {
			return paths[:0]
		}
		paths = paths[p.Skip:]
	}
	if limit := resultLimit(p); limit > 0 && len(paths) > limit {
		paths = paths[:limit]
	}
	return paths
}

// resultLimit returns the effective per-page cap from Limit and MaxFiles
// (whichever is smaller and positive); 0 means unbounded.
func resultLimit(p Params) int {
	limit := p.Limit
	if p.MaxFiles > 0 && (limit == 0 || p.MaxFiles < limit) {
		limit = p.MaxFiles
	}
	return limit
}

// applyResultWindow drops the first Skip matches and caps the remainder to the
// effective result limit. It assumes the input is already ranked.
func applyResultWindow(out []FileMatch, p Params) []FileMatch {
	if p.Skip > 0 {
		if p.Skip >= len(out) {
			return out[:0]
		}
		out = out[p.Skip:]
	}
	if limit := resultLimit(p); limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortMatches(out []FileMatch) {
	sort.Slice(out, func(i, j int) bool {
		return out[i].DisplayPath < out[j].DisplayPath
	})
}

// analyzeMatchStructure asks parser to build structural output once for this
// returned file. Result construction reuses the attached segments.
func analyzeMatchStructure(fm *FileMatch, p Params) {
	fm.Segments = parser.BuildSegments(fm.Content, fm.Language, fm.MatchLines, p.MaxSegments)
	fm.SegmentsReady = true
}

// AggregateRepoCounts tallies per-repository file and match-line counts from a
// set of FileMatches (files = matched files, matches = matching lines). The repo
// id is derived from each match's display path. Results are returned in the
// deterministic narrowing order defined by SortRepoCounts.
func AggregateRepoCounts(matches []FileMatch) []api.RepoCount {
	type agg struct{ files, matches int }
	repos := map[string]*agg{}
	for _, m := range matches {
		repo := RepoID(m.DisplayPath)
		a := repos[repo]
		if a == nil {
			a = &agg{}
			repos[repo] = a
		}
		a.files++
		a.matches += len(m.MatchLines)
	}
	out := make([]api.RepoCount, 0, len(repos))
	for repo, a := range repos {
		out = append(out, api.RepoCount{Repo: repo, Files: a.files, Matches: a.matches})
	}
	SortRepoCounts(out)
	return out
}

// SortRepoCounts orders counts by matches desc, then files desc, then repo asc.
// The order is deterministic and surfaces the biggest concentrations first so an
// agent can narrow to the hottest repositories.
func SortRepoCounts(counts []api.RepoCount) {
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Matches != counts[j].Matches {
			return counts[i].Matches > counts[j].Matches
		}
		if counts[i].Files != counts[j].Files {
			return counts[i].Files > counts[j].Files
		}
		return counts[i].Repo < counts[j].Repo
	})
}
