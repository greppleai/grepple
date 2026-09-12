package search

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ignored implements the common .gitignore forms used by source repositories.
// Pattern matching deliberately uses filepath.Match, which has the same syntax
// as filepath.Glob.
func ignored(rel string, dirs map[string][]string) bool {
	rel = filepath.Clean(rel)
	isIgnored := false
	for dir, patterns := range dirs {
		sub, inScope := ignoreScope(rel, dir)
		if !inScope {
			continue
		}
		for _, raw := range patterns {
			pattern, negated, directoryOnly := normalizeIgnorePattern(raw)
			if pattern == "" {
				continue
			}
			if matchIgnorePattern(pattern, sub, directoryOnly) {
				isIgnored = !negated
			}
		}
	}
	return isIgnored
}

// ignoreScope returns rel relative to a gitignore directory, or inScope=false
// when rel lies outside that directory's reach.
func ignoreScope(rel, dir string) (sub string, inScope bool) {
	dir = filepath.Clean(dir)
	if dir == "." {
		dir = ""
	}
	if dir != "" && rel != dir && !strings.HasPrefix(rel, dir+string(filepath.Separator)) {
		return "", false
	}
	if dir == "" {
		return rel, true
	}
	return strings.TrimPrefix(strings.TrimPrefix(rel, dir), string(filepath.Separator)), true
}

// normalizeIgnorePattern strips the gitignore markers from a raw pattern line:
// '!' negation, a leading '/' anchor, and a trailing '/' directory-only mark.
func normalizeIgnorePattern(raw string) (pattern string, negated, directoryOnly bool) {
	negated = strings.HasPrefix(raw, "!")
	pattern = strings.TrimPrefix(strings.TrimPrefix(raw, "!"), "/")
	directoryOnly = strings.HasSuffix(pattern, "/")
	pattern = strings.TrimSuffix(pattern, "/")
	return pattern, negated, directoryOnly
}

// matchIgnorePattern reports whether sub (a path relative to the pattern's
// directory) matches one normalized gitignore pattern. Patterns containing a
// separator match the whole sub-path; bare patterns match any path segment.
// Directory-only patterns additionally require sub to be the named directory
// or beneath it.
func matchIgnorePattern(pattern, sub string, directoryOnly bool) bool {
	matched := false
	if strings.ContainsRune(pattern, filepath.Separator) || strings.Contains(pattern, "/") {
		matched, _ = filepath.Match(filepath.FromSlash(pattern), sub)
	} else {
		for _, part := range strings.Split(sub, string(filepath.Separator)) {
			if ok, _ := filepath.Match(pattern, part); ok {
				matched = true
				break
			}
		}
	}
	if directoryOnly && !(sub == pattern || strings.HasPrefix(sub, pattern+string(filepath.Separator))) {
		matched = false
	}
	return matched
}

// CollectFilesUnder walks only the given directories (applying .gitignore) and
// returns the files found, confined to root and de-duplicated. It lets callers
// scope a listing to specific repositories instead of walking the whole tree.
func CollectFilesUnder(dirs []string, root string) ([]string, error) {
	seen := map[string]bool{}
	var files []string
	for _, dir := range dirs {
		walked, err := walkCandidateFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, file := range walked {
			if withinRoot(file, root) && !seen[file] {
				seen[file] = true
				files = append(files, file)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func collectCandidateFiles(globs []string, root string) ([]string, error) {
	return collectCandidateFilesContext(context.Background(), globs, root)
}

func collectCandidateFilesContext(ctx context.Context, globs []string, root string) ([]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if len(globs) == 0 {
		files, err := walkCandidateFilesContext(ctx, cwd)
		if err != nil {
			return nil, err
		}
		return confineFiles(files, root), nil
	}

	c := newCandidateCollector(ctx, root, cwd)
	for _, pattern := range globs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var err error
		if strings.Contains(pattern, "**") {
			err = c.addRecursiveGlob(pattern)
		} else {
			err = c.addPlainGlob(pattern)
		}
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(c.files)
	return c.files, nil
}

// candidateCollector accumulates de-duplicated, root-confined file paths for
// collectCandidateFiles.
type candidateCollector struct {
	ctx         context.Context
	root, cwd   string
	ignoreCache gitignoreCache
	seen        map[string]bool
	files       []string
}

func newCandidateCollector(ctx context.Context, root, cwd string) *candidateCollector {
	return &candidateCollector{ctx: ctx, root: root, cwd: cwd, ignoreCache: gitignoreCache{}, seen: map[string]bool{}}
}

// add records one matched file, applying root confinement, .git exclusion,
// gitignore filtering, and de-duplication.
func (c *candidateCollector) add(absolute string) {
	if !withinRoot(absolute, c.root) {
		return
	}
	if !pathContainsGitDirectory(absolute) && !pathIgnoredFromRootCached(absolute, c.cwd, c.ignoreCache) && !c.seen[absolute] {
		c.seen[absolute] = true
		c.files = append(c.files, absolute)
	}
}

// addSubtree adds every file under a matched directory; its walk already
// applied .gitignore and .git exclusion, so only confinement and de-dup apply.
func (c *candidateCollector) addSubtree(dir string) error {
	if !withinRoot(dir, c.root) {
		return nil
	}
	nested, err := walkCandidateFilesContext(c.ctx, dir)
	if err != nil {
		return err
	}
	for _, path := range nested {
		if withinRoot(path, c.root) && !c.seen[path] {
			c.seen[path] = true
			c.files = append(c.files, path)
		}
	}
	return nil
}

// addPlainGlob resolves one non-recursive glob: directories contribute their
// subtree, files themselves.
func (c *candidateCollector) addPlainGlob(pattern string) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := c.ctx.Err(); err != nil {
			return err
		}
		absolute, err := filepath.Abs(match)
		if err != nil {
			return err
		}
		info, err := os.Stat(absolute)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := c.addSubtree(absolute); err != nil {
				return err
			}
			continue
		}
		c.add(absolute)
	}
	return nil
}

// addRecursiveGlob resolves one ** pattern and adds each match.
func (c *candidateCollector) addRecursiveGlob(pattern string) error {
	matches, err := expandRecursiveGlob(c.ctx, pattern, c.cwd)
	if err != nil {
		return err
	}
	for _, match := range matches {
		c.add(match)
	}
	return nil
}

// collectListingFiles resolves positionals for path-listing modes (--files and
// --outline). Bare positionals that name an existing directory act as scope
// roots (where to look); positionals containing glob metacharacters, or naming
// individual files, act as path filters (what to keep). A file is listed when it
// lies under a scope root (or the CWD when none are given) AND matches a glob
// filter (or any file when no filters are given). This makes "grepple -l GLOB DIR"
// mean "paths matching GLOB, scoped to DIR" instead of unioning every file under
// DIR with the glob results.
func collectListingFiles(globs []string, root string) ([]string, error) {
	return collectListingFilesContext(context.Background(), globs, root)
}

func collectListingFilesContext(ctx context.Context, globs []string, root string) ([]string, error) {
	roots, filters := splitGlobRoots(globs)

	discoveryGlobs := roots
	if len(roots) == 0 {
		// filters may be empty here, which makes collectCandidateFiles walk the CWD.
		discoveryGlobs = filters
	}
	candidates, err := collectCandidateFilesContext(ctx, discoveryGlobs, root)
	if err != nil {
		return nil, err
	}

	// When scope roots and glob filters are combined, discovery walked the roots;
	// keep only the files that also match one of the filters.
	if len(roots) > 0 && len(filters) > 0 {
		cwd, _ := os.Getwd()
		kept := candidates[:0]
		for _, file := range candidates {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if pathMatchesGlobs(displayPathFrom(file, cwd), filters) {
				kept = append(kept, file)
			}
		}
		candidates = kept
	}
	return candidates, nil
}

// splitGlobRoots partitions listing positionals into directory scope roots and
// glob path filters. A positional is a scope root only when it has no glob
// metacharacters and names an existing directory; everything else is a filter.
func splitGlobRoots(globs []string) (roots, filters []string) {
	for _, g := range globs {
		if !hasGlobMeta(g) {
			if info, err := os.Stat(g); err == nil && info.IsDir() {
				roots = append(roots, g)
				continue
			}
		}
		filters = append(filters, g)
	}
	return roots, filters
}

// hasGlobMeta reports whether a positional contains filepath.Match wildcards.
func hasGlobMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// withinRoot reports whether path is root itself or nested beneath it. An empty
// root disables confinement so local CLI searches keep their usual reach.
func withinRoot(path, root string) bool {
	if root == "" {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func confineFiles(files []string, root string) []string {
	if root == "" {
		return files
	}
	out := files[:0]
	for _, file := range files {
		if withinRoot(file, root) {
			out = append(out, file)
		}
	}
	return out
}

// expandRecursiveGlob resolves a pattern containing ** by walking the longest
// wildcard-free prefix and matching each file against the full pattern.
func expandRecursiveGlob(ctx context.Context, pattern, cwd string) ([]string, error) {
	base := globBaseDir(pattern, cwd)
	var matches []string
	err := filepath.WalkDir(base, globWalkAction(ctx, pattern, cwd, base, &matches))
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// globBaseDir returns the longest wildcard-free directory prefix of pattern,
// resolved against cwd — the walk root for a ** glob.
func globBaseDir(pattern, cwd string) string {
	var baseParts []string
	for _, part := range strings.Split(filepath.ToSlash(pattern), "/") {
		if strings.ContainsAny(part, "*?[") {
			break
		}
		baseParts = append(baseParts, part)
	}
	if len(baseParts) == 0 {
		return cwd
	}
	return filepath.Join(cwd, filepath.Join(baseParts...))
}

// globWalkAction returns the WalkDir action that collects files matching the
// full glob pattern, skipping .git directories and unreadable entries.
func globWalkAction(ctx context.Context, pattern, cwd, base string, matches *[]string) fs.WalkDirFunc {
	return func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" && path != base {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return nil
		}
		if globMatch(pattern, rel) {
			*matches = append(*matches, path)
		}
		return nil
	}
}

func walkCandidateFiles(root string) ([]string, error) {
	return walkCandidateFilesContext(context.Background(), root)
}

func walkCandidateFilesContext(ctx context.Context, root string) ([]string, error) {
	w := &candidateWalker{ctx: ctx, root: root, ignorePatterns: map[string][]string{}}
	if err := filepath.WalkDir(root, w.walk); err != nil {
		return nil, err
	}
	sort.Strings(w.files)
	return w.files, nil
}

// candidateWalker accumulates non-ignored file paths during a WalkDir, loading
// each directory's .gitignore on first visit.
type candidateWalker struct {
	ctx            context.Context
	root           string
	ignorePatterns map[string][]string
	files          []string
}

// walk collects one visited path: directories contribute their .gitignore
// (and are skipped, along with .git, when ignored); files are appended unless
// ignored.
func (w *candidateWalker) walk(path string, entry fs.DirEntry, walkErr error) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if walkErr != nil {
		return nil
	}
	if entry.IsDir() && entry.Name() == ".git" && path != w.root {
		return filepath.SkipDir
	}
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return nil
	}
	if entry.IsDir() {
		return w.walkDir(path, rel)
	}
	if !ignored(rel, w.ignorePatterns) {
		w.files = append(w.files, path)
	}
	return nil
}

// walkDir absorbs path's .gitignore and reports whether the subtree is
// ignored (SkipDir) — the walk root itself is never skipped.
func (w *candidateWalker) walkDir(path, rel string) error {
	base := rel
	if base == "." {
		base = ""
	}
	w.ignorePatterns[base] = append(w.ignorePatterns[base], parseGitignore(path)...)
	if path != w.root && ignored(rel, w.ignorePatterns) {
		return filepath.SkipDir
	}
	return nil
}

// parseGitignore returns the effective patterns of dir's .gitignore file
// (blank lines and '#' comments stripped); a missing file yields none.
func parseGitignore(dir string) []string {
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		return nil
	}
	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	return patterns
}

// gitignoreCache memoizes each directory's own .gitignore patterns so a batch of
// candidate checks that share ancestor directories reads and parses each
// .gitignore once instead of re-reading it for every file.
type gitignoreCache map[string][]string

func (c gitignoreCache) patterns(dir string) []string {
	if p, ok := c[dir]; ok {
		return p
	}
	patterns := []string{}
	patterns = append(patterns, parseGitignore(dir)...)
	c[dir] = patterns
	return patterns
}

func pathIgnoredFromRoot(path, root string) bool {
	return pathIgnoredFromRootCached(path, root, gitignoreCache{})
}

// pathIgnoredFromRootCached reports whether path is .gitignored, reading each
// ancestor directory's .gitignore through the supplied cache. Pass one cache
// across a whole candidate batch to avoid re-reading shared ancestors.
func pathIgnoredFromRootCached(path, root string, cache gitignoreCache) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	patterns := map[string][]string{}
	directory := root
	for {
		if dirPatterns := cache.patterns(directory); len(dirPatterns) > 0 {
			base, _ := filepath.Rel(root, directory)
			if base == "." {
				base = ""
			}
			patterns[base] = dirPatterns
		}
		if directory == filepath.Dir(path) {
			break
		}
		next := filepath.Join(directory, strings.Split(strings.TrimPrefix(filepath.Dir(path), directory+string(filepath.Separator)), string(filepath.Separator))[0])
		if next == directory || !strings.HasPrefix(next, root) {
			break
		}
		directory = next
	}
	return ignored(rel, patterns)
}

func pathMatchesGlobs(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, pattern := range patterns {
		if globMatch(pattern, path) {
			return true
		}
	}
	return false
}

// PathMatchesGlobs reports whether path matches any of the glob patterns,
// with an empty pattern list matching everything. It is the authoritative glob
// matcher used to filter index-returned candidates (the Zoekt file: atom is
// only a superset pre-filter).
func PathMatchesGlobs(path string, patterns []string) bool {
	return pathMatchesGlobs(path, patterns)
}

// globMatch extends filepath.Match with ** support, where ** spans zero or
// more path segments. Each remaining segment is matched with filepath.Match.
func globMatch(pattern, name string) bool {
	pattern = filepath.ToSlash(pattern)
	name = filepath.ToSlash(name)
	if !strings.Contains(pattern, "**") {
		if matched, err := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(name)); err == nil && matched {
			return true
		}
		return false
	}
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(patternParts, nameParts []string) bool {
	if len(patternParts) == 0 {
		return len(nameParts) == 0
	}
	if patternParts[0] == "**" {
		rest := patternParts[1:]
		if len(rest) == 0 {
			return true
		}
		for index := 0; index <= len(nameParts); index++ {
			if matchSegments(rest, nameParts[index:]) {
				return true
			}
		}
		return false
	}
	if len(nameParts) == 0 {
		return false
	}
	if matched, err := filepath.Match(patternParts[0], nameParts[0]); err != nil || !matched {
		return false
	}
	return matchSegments(patternParts[1:], nameParts[1:])
}

func pathContainsGitDirectory(path string) bool {
	for _, part := range strings.Split(filepath.Clean(path), string(filepath.Separator)) {
		if part == ".git" {
			return true
		}
	}
	return false
}
