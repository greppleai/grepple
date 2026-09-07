package shard

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"grepple/internal/api"
	"grepple/internal/search"
)

type zoektOptions struct {
	indexDir string
	repoRoot string
	port     int
	binDir   string
}

type zoektCoverage struct {
	known    bool
	indexed  bool
	fallback []string
}

type zoektProcess struct {
	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	waitErr error
}

func zoektQuery(p search.Params) string {
	if utf8.RuneCountInString(p.Query) < 3 {
		return ""
	}
	ct := "case:yes"
	if p.IgnoreCase {
		ct = "case:no"
	}
	// Zoekt's content: field is parsed as a Go (RE2) regexp and automatically
	// optimized into a substring search when the pattern has no regex
	// operators, so it works for both literal and regex queries and can use the
	// trigram index to pre-filter candidates. For literal queries we quote the
	// metacharacters so Zoekt matches the text verbatim; regex queries are
	// passed through after validating they are a legal RE2 pattern (an invalid
	// pattern would make Zoekt error, so we fall back to a scan instead).
	pattern := p.Query
	if !p.Regex {
		pattern = regexp.QuoteMeta(p.Query)
	} else if _, err := regexp.Compile(p.Query); err != nil {
		return ""
	}
	// Encode the pattern for the quoted query value: a literal backslash is
	// written as two backslashes and a double quote is escaped.
	escaped := strings.ReplaceAll(pattern, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return ct + ` content:"` + escaped + `"` + zoektFileFilter(p.Globs)
}

// zoektFileFilter turns the request globs into a Zoekt file: atom so the index
// pre-filters candidates by path instead of shipping every content match for
// grepple to discard. The regexps are a deliberate SUPERSET of the glob set:
// callers still post-filter with search.PathMatchesGlobs (search re-checks every
// candidate, count re-checks every returned file), so an over-broad file: atom
// can never drop a real match — it only narrows what the index returns. Zoekt
// paths are repo-relative (no owner/repo prefix) while grepple globs are anchored
// to the full display path, so the regexps are left unanchored at the start.
// If any glob cannot be translated safely the whole pushdown is skipped (an OR
// atom missing one alternative would wrongly exclude that glob's files).
func zoektFileFilter(globs []string) string {
	if len(globs) == 0 {
		return ""
	}
	alternatives := make([]string, 0, len(globs))
	for _, glob := range globs {
		re, ok := globToPathRegexp(glob)
		if !ok {
			return ""
		}
		alternatives = append(alternatives, "(?:"+re+")")
	}
	combined := strings.Join(alternatives, "|")
	escaped := strings.ReplaceAll(combined, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return ` file:"` + escaped + `"`
}

// globToPathRegexp converts one grepple glob into an RE2 path regexp that matches
// at least every path the glob matches (a superset). '**' crosses directory
// separators, '*' and '?' do not; a leading '**/' also matches zero segments so
// a root-level file still qualifies. The result is end-anchored (grepple globs are
// end-anchored and the repo-relative path shares the display path's tail) but
// start-unanchored (the owner/repo prefix is absent from Zoekt paths). Returns
// false when the glob cannot be translated safely (whitespace, unterminated
// character class), so the caller falls back to no pushdown.
func globToPathRegexp(glob string) (string, bool) {
	g := filepath.ToSlash(glob)
	if strings.ContainsAny(g, " \t\n") {
		return "", false
	}
	runes := []rune(g)
	var b strings.Builder
	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; c {
		case '*':
			i = writeGlobStar(runes, i, &b)
		case '?':
			b.WriteString(`[^/]`)
		case '[':
			next, ok := writeGlobClass(runes, i, &b)
			if !ok {
				return "", false // unterminated class
			}
			i = next
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\':
			b.WriteByte('\\')
			b.WriteRune(c)
		case '/':
			b.WriteByte('/')
		default:
			b.WriteRune(c)
		}
	}
	b.WriteString(`$`)
	return b.String(), true
}

// writeGlobStar renders one glob star run: '**/' crosses directories matching
// zero or more segments (so a root-level file still qualifies), a bare '**'
// crosses them, and a single '*' stays within one path segment. Returns the
// new scan index.
func writeGlobStar(runes []rune, i int, b *strings.Builder) int {
	if i+1 >= len(runes) || runes[i+1] != '*' {
		b.WriteString(`[^/]*`)
		return i
	}
	i++
	if i+1 < len(runes) && runes[i+1] == '/' {
		b.WriteString(`(?:.*/)?`) // '**/' — zero or more directory segments
		return i + 1
	}
	b.WriteString(`.*`)
	return i
}

// writeGlobClass renders one glob character class ('!' or '^' negation,
// backslashes escaped for RE2). ok is false for an unterminated class, so the
// caller can drop the whole pushdown.
func writeGlobClass(runes []rune, i int, b *strings.Builder) (next int, ok bool) {
	j := i + 1
	neg := false
	if j < len(runes) && (runes[j] == '!' || runes[j] == '^') {
		neg = true
		j++
	}
	k := j
	for k < len(runes) && runes[k] != ']' {
		k++
	}
	if k >= len(runes) {
		return 0, false // unterminated class
	}
	b.WriteString("[")
	if neg {
		b.WriteString("^")
	}
	for _, cc := range runes[j:k] {
		if cc == '\\' {
			b.WriteString(`\\`)
		} else {
			b.WriteRune(cc)
		}
	}
	b.WriteString("]")
	return k, true
}

func zoektBin(o zoektOptions, name string) string {
	if o.binDir != "" {
		return filepath.Join(o.binDir, name)
	}
	if executable, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(executable), name)
		if info, statErr := os.Stat(sibling); statErr == nil && !info.IsDir() && info.Mode()&0111 != 0 {
			return sibling
		}
	}
	return name
}

func configureChildProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

func zoektIndex(o zoektOptions, dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, zoektBin(o, "zoekt-git-index"),
		"-index", o.indexDir,
		"-repo_cache", o.repoRoot,
		"-file_limit", "2147483647",
		dir,
	)
	configureChildProcess(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("zoekt-git-index timed out: %w", ctx.Err())
		}
		return fmt.Errorf("zoekt-git-index failed: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

// zoektCandidates returns the candidate file paths for a query from the local
// Zoekt index, plus a truncated flag. grepple does not rank, so an incomplete
// (result-cap) response is not an error: the returned files are real, correct
// matches and the caller surfaces truncated=true so the agent can narrow.
// A real failure (crash, connection, decode) still returns an error, which
// triggers the full-scan fallback. Only file names are requested (ChunkMatches
// off) since the caller uses the paths, not the match chunks.
func zoektCandidates(o zoektOptions, q, root string, repos []api.RepoInfo) ([]string, bool, error) {
	body, _ := json.Marshal(map[string]any{"Q": q, "Opts": map[string]any{"ChunkMatches": false, "NumContextLines": 0}})
	client := http.Client{Timeout: 10 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/search", o.port)
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("zoekt search %d", resp.StatusCode)
	}
	var response struct {
		Result struct {
			FlushReason                   int
			Crashes                       int
			ShardsSkipped                 int
			FilesSkipped                  int
			FilesSkippedDueToCancellation int
			Files                         []struct {
				FileName, Repository string
			}
		}
	}
	if err = json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, false, err
	}
	// A crash means the index is unreliable — fall back to a full scan. Any other
	// short-circuit (files/shards skipped due to a result cap, an early flush, or
	// per-file cancellation) just means the match set is incomplete: return the
	// (valid) partial candidates and flag them truncated.
	if response.Result.Crashes != 0 {
		return nil, false, fmt.Errorf("Zoekt search crashed (crashes=%d)", response.Result.Crashes)
	}
	truncated := response.Result.FlushReason != 0 || response.Result.ShardsSkipped != 0 ||
		response.Result.FilesSkipped != 0 || response.Result.FilesSkippedDueToCancellation != 0
	knownRepos := map[string]bool{}
	for _, repo := range repos {
		knownRepos[repo.Repo] = true
	}
	set := map[string]bool{}
	for _, file := range response.Result.Files {
		if !knownRepos[file.Repository] {
			continue
		}
		set[filepath.Join(root, filepath.FromSlash(file.Repository), filepath.FromSlash(file.FileName))] = true
	}
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	return out, truncated, nil
}

// zoektRepoCounts returns per-repository match tallies for a query entirely from
// the Zoekt index — no filesystem reads. files = matching files, matches =
// distinct matching lines (derived from Zoekt's match ranges), matching the
// content-search count semantics. Only repositories in allow are counted. As the
// counts are index-based they can include a few binary/generated files that a
// content scan would skip. A crash is an error; a result-cap short-circuit sets
// truncated.
func zoektRepoCounts(o zoektOptions, q string, allow []api.RepoInfo, globs []string) ([]api.RepoCount, bool, error) {
	body, _ := json.Marshal(map[string]any{"Q": q, "Opts": map[string]any{"ChunkMatches": true, "NumContextLines": 0}})
	client := http.Client{Timeout: 30 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/api/search", o.port)
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, false, fmt.Errorf("zoekt search %d", resp.StatusCode)
	}
	var response struct {
		Result struct {
			FlushReason                   int
			Crashes                       int
			ShardsSkipped                 int
			FilesSkipped                  int
			FilesSkippedDueToCancellation int
			Files                         []zoektChunkedFile
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, false, err
	}
	if response.Result.Crashes != 0 {
		return nil, false, fmt.Errorf("Zoekt search crashed (crashes=%d)", response.Result.Crashes)
	}
	truncated := response.Result.FlushReason != 0 || response.Result.ShardsSkipped != 0 ||
		response.Result.FilesSkipped != 0 || response.Result.FilesSkippedDueToCancellation != 0
	allowed := make(map[string]bool, len(allow))
	for _, r := range allow {
		allowed[r.Repo] = true
	}
	type agg struct{ files, matches int }
	perRepo := map[string]*agg{}
	for _, f := range response.Result.Files {
		if !allowed[f.Repository] {
			continue
		}
		// The file: atom in the query is only a superset pre-filter; apply the
		// authoritative matcher so counts respect --files globs exactly.
		if !search.PathMatchesGlobs(f.Repository+"/"+f.FileName, globs) {
			continue
		}
		a := perRepo[f.Repository]
		if a == nil {
			a = &agg{}
			perRepo[f.Repository] = a
		}
		a.files++
		a.matches += len(matchLineSet(f))
	}
	counts := make([]api.RepoCount, 0, len(perRepo))
	for repo, a := range perRepo {
		counts = append(counts, api.RepoCount{Repo: repo, Files: a.files, Matches: a.matches})
	}
	search.SortRepoCounts(counts)
	return counts, truncated, nil
}

// zoektChunkedFile is one file in a zoekt /api/search response when chunk
// matches are requested.
type zoektChunkedFile struct {
	Repository   string
	FileName     string
	ChunkMatches []struct {
		Ranges []struct {
			Start struct{ LineNumber int }
		}
	}
}

// matchLineSet returns the distinct line numbers a file's chunk ranges touch,
// matching the content-scan count semantics (matches = distinct matching
// lines, not range occurrences).
func matchLineSet(f zoektChunkedFile) map[int]bool {
	lines := map[int]bool{}
	for _, cm := range f.ChunkMatches {
		for _, rg := range cm.Ranges {
			lines[rg.Start.LineNumber] = true
		}
	}
	return lines
}

func reconcileZoektShards(o zoektOptions, repos []api.RepoInfo) error {
	known := map[string]bool{}
	for _, repo := range repos {
		known[zoektShardPrefix(repo.Repo)] = true
	}
	entries, err := os.ReadDir(o.indexDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".zoekt") {
			continue
		}
		versionAt := strings.LastIndex(entry.Name(), "_v")
		if versionAt < 0 {
			continue
		}
		if known[entry.Name()[:versionAt]] {
			continue
		}
		if err := os.Remove(filepath.Join(o.indexDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func zoektShardPrefix(repo string) string {
	prefix := url.QueryEscape(repo)
	if len(prefix) <= 200 {
		return prefix
	}
	sum := sha1.Sum([]byte(prefix))
	return prefix[:200] + fmt.Sprintf("%x", sum[:])[:8]
}

func removeZoektRepo(o zoektOptions, repo string) error {
	pattern := filepath.Join(o.indexDir, zoektShardPrefix(repo)+"_v*.zoekt")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func waitForZoektVersion(o zoektOptions, repo, version string) error {
	query := "repo:^" + regexp.QuoteMeta(repo) + "$"
	body, _ := json.Marshal(map[string]any{"Q": query, "Opts": map[string]any{"ChunkMatches": true}})
	client := http.Client{Timeout: time.Second}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/search", o.port)
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		found, err := probeZoektVersion(&client, endpoint, body, repo, version)
		switch {
		case err != nil:
			lastErr = err
		case found:
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("Zoekt did not load repository %s at %s: %v", repo, version, lastErr)
}

// probeZoektVersion runs one search round-trip and reports whether the
// repository's shard already serves the wanted commit version. Transport,
// HTTP status, and decode failures are all returned as errors; the caller
// retries until its deadline.
func probeZoektVersion(client *http.Client, endpoint string, body []byte, repo, version string) (bool, error) {
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	var result struct {
		Result struct {
			Files []struct {
				Repository string
				Version    string
			}
		}
	}
	decodeErr := json.NewDecoder(resp.Body).Decode(&result)
	resp.Body.Close()
	if decodeErr != nil {
		return false, decodeErr
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("zoekt search %d", resp.StatusCode)
	}
	for _, file := range result.Result.Files {
		if file.Repository == repo && file.Version == version {
			return true, nil
		}
	}
	return false, nil
}

func inspectZoektCoverage(dir string) zoektCoverage {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tracked, err := gitFileList(ctx, dir, "ls-files", "-z")
	if err != nil {
		return zoektCoverage{}
	}
	untracked, err := gitFileList(ctx, dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return zoektCoverage{}
	}
	modified, err := gitFileList(ctx, dir, "diff", "--name-only", "-z")
	if err != nil {
		return zoektCoverage{}
	}
	staged, err := gitFileList(ctx, dir, "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return zoektCoverage{}
	}

	fallback := map[string]bool{}
	addFallback := func(relative string) {
		if relative != "" {
			fallback[filepath.Join(dir, filepath.FromSlash(relative))] = true
		}
	}
	for _, relative := range append(append([]string{}, untracked...), append(modified, staged...)...) {
		addFallback(relative)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sourcegraph", "ignore")); err == nil {
		for _, relative := range tracked {
			addFallback(relative)
		}
		return zoektCoverage{known: true, fallback: sortedPaths(fallback)}
	}

	indexed := false
	for _, relative := range tracked {
		path := filepath.Join(dir, filepath.FromSlash(relative))
		switch classifyTrackedFile(path) {
		case trackedFileFallback:
			fallback[path] = true
		case trackedFileIndexed:
			indexed = true
		}
	}
	return zoektCoverage{known: true, indexed: indexed, fallback: sortedPaths(fallback)}
}

// trackedFileDisposition is how one git-tracked file counts toward a
// repository's index coverage.
type trackedFileDisposition int

const (
	// trackedFileSkip excludes the file from coverage entirely: missing from
	// disk, an unreadable entry, or a submodule gitlink (zoekt indexes the
	// superrepo's own files only, and a directory is not scannable content).
	trackedFileSkip trackedFileDisposition = iota
	// trackedFileFallback marks the file scanner-only: symlink, oversized, or
	// content the index cannot serve (and unreadable files, so one bad entry
	// never fails the whole repository's coverage).
	trackedFileFallback
	// trackedFileIndexed marks the file served by the zoekt index.
	trackedFileIndexed
)

// classifyTrackedFile decides how one git-tracked file at path counts toward
// coverage (see trackedFileDisposition).
func classifyTrackedFile(path string) trackedFileDisposition {
	info, err := os.Lstat(path)
	if err != nil {
		return trackedFileSkip
	}
	if info.IsDir() {
		return trackedFileSkip // submodule gitlink
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Size() > 2147483647 {
		return trackedFileFallback
	}
	needsFallback, err := fileNeedsZoektFallback(path)
	if err != nil || needsFallback {
		return trackedFileFallback
	}
	return trackedFileIndexed
}

func gitFileList(ctx context.Context, dir string, args ...string) ([]string, error) {
	commandArgs := append([]string{"-C", dir}, args...)
	output, err := exec.CommandContext(ctx, "git", commandArgs...).Output()
	if err != nil {
		return nil, err
	}
	paths := strings.Split(string(output), "\x00")
	if len(paths) > 0 && paths[len(paths)-1] == "" {
		paths = paths[:len(paths)-1]
	}
	return paths, nil
}

// fileNeedsZoektFallback reports whether a file's content defeats the zoekt
// index (invalid UTF-8 or more than 20k distinct trigrams) and must be scanned
// as a fallback. A NUL byte means binary: not indexed, but not scanned either.
func fileNeedsZoektFallback(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, 64<<10)
	scan := fallbackScan{seen: make(map[[3]rune]struct{}, 20001)}
	for {
		current, size, err := reader.ReadRune()
		if err == io.EOF {
			return scan.invalidUTF8 || scan.tooManyTrigrams, nil
		}
		if err != nil {
			return false, err
		}
		if scan.add(current, size) {
			return false, nil // NUL byte: binary, stop early
		}
	}
}

// fallbackScan accumulates the two index-defeat verdicts while a file is
// scanned rune by rune.
type fallbackScan struct {
	seen            map[[3]rune]struct{}
	previous        [2]rune
	count           int
	invalidUTF8     bool
	tooManyTrigrams bool
}

// add consumes one rune, updating the invalid-UTF8 and trigram-diversity
// verdicts. It reports true at a NUL byte (binary content: stop scanning).
func (s *fallbackScan) add(current rune, size int) bool {
	if current == 0 {
		return true
	}
	if current == utf8.RuneError && size == 1 {
		s.invalidUTF8 = true
	}
	if !s.tooManyTrigrams && s.count >= 2 {
		s.seen[[3]rune{s.previous[0], s.previous[1], current}] = struct{}{}
		if len(s.seen) > 20000 {
			s.tooManyTrigrams = true
			s.seen = nil
		}
	}
	s.previous[0], s.previous[1] = s.previous[1], current
	s.count++
	return false
}

func sortedPaths(set map[string]bool) []string {
	paths := make([]string, 0, len(set))
	for path := range set {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func startZoekt(o zoektOptions) (*zoektProcess, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, zoektBin(o, "zoekt-webserver"),
		"-index", o.indexDir,
		"-rpc",
		"-listen", fmt.Sprintf("127.0.0.1:%d", o.port),
	)
	configureChildProcess(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	process := &zoektProcess{cancel: cancel, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		process.mu.Lock()
		process.waitErr = err
		process.mu.Unlock()
		close(process.done)
	}()

	// Wait for readiness in the background so shard startup (and readiness) is not
	// blocked by zoekt loading its shards, which can take a while with many/large
	// shards. Until the webserver is listening, searches fall back to scanning.
	go waitZoektReady(o, process)
	return process, nil
}

// waitZoektReady polls the zoekt-webserver until it responds, logging when it is
// ready. It returns early if the process exits.
func waitZoektReady(o zoektOptions, process *zoektProcess) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-process.done:
			return
		default:
		}
		if resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d", o.port)); err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				fmt.Fprintf(os.Stderr, "zoekt-webserver ready on 127.0.0.1:%d\n", o.port)
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Fprintf(os.Stderr, "warning: zoekt-webserver not ready after 2m; searches use scanner fallback until it is\n")
}

func (p *zoektProcess) running() bool {
	if p == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *zoektProcess) err() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.waitErr
}

func (p *zoektProcess) stop() {
	if p == nil {
		return
	}
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
}
