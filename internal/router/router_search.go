package router

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"grepple/internal/api"
	"grepple/internal/search"

	"go.uber.org/zap"
)

// backendResult is one shard's response (or error) to a fanned-out search.
type backendResult struct {
	resp    api.SearchResponse
	err     error
	base    string
	elapsed time.Duration
}

// shardStat is one backend's timing entry in the per-search log line.
type shardStat struct {
	Backend string `json:"backend"`
	Ms      int64  `json:"ms"`
	OK      bool   `json:"ok"`
}

func routeSearch(o routerOptions, q api.SearchRequest) (api.SearchResponse, error) {
	reqID := newRequestID()
	p, e := search.ResolveRequest(q)
	if e != nil {
		return api.SearchResponse{}, e
	}
	// Client-facing page policy: one page is at most MaxPageLimit result files
	// (DefaultPageLimit when the client does not set a limit). The fan-out
	// requests built below deliberately exceed the cap (skip+limit), so it is
	// enforced here at the public boundary, never inside ResolveRequest.
	search.EnforcePageLimit(&p, q)
	start := time.Now()
	ch := fanOutShards(o, shardWindowRequest(q, p), reqID)
	all, repoCounts, truncated, backendErrors := collectShardResults(o, ch, p, q, reqID, start)
	// Degrade gracefully: only fail the whole search when every shard failed.
	// Otherwise return the results from the shards that responded and report the
	// failed shards in ShardErrors, so a single busy/restarting/indexing shard
	// does not take down all searches.
	if len(backendErrors) == len(o.backends) {
		return api.SearchResponse{}, fmt.Errorf("all %d shards failed: %v", len(o.backends), backendErrors)
	}
	shardErrors := shardErrorStrings(backendErrors)
	if q.CountByRepo {
		return api.SearchResponse{RepoCounts: mergeRepoCounts(repoCounts), ShardErrors: shardErrors, Truncated: truncated}, nil
	}
	return api.SearchResponse{Results: mergeShardResults(all, p), ShardErrors: shardErrors, Truncated: truncated}, nil
}

// shardWindowRequest pushes the paging window down so each shard returns only
// the top (skip+limit) ranked files it needs to contribute, with skip=0 (the
// final skip applies after the global merge). MaxFiles acts as a limit too, so
// it folds into the effective limit and is dropped from the shard request.
// Counts must cover the full match set, so they never push a window down.
func shardWindowRequest(q api.SearchRequest, p search.Params) api.SearchRequest {
	shardRequest := q
	zero := 0
	shardRequest.Skip = &zero
	shardRequest.MaxFiles = nil
	effectiveLimit := p.Limit
	if p.MaxFiles > 0 && (effectiveLimit == 0 || p.MaxFiles < effectiveLimit) {
		effectiveLimit = p.MaxFiles
	}
	if effectiveLimit > 0 {
		combined := p.Skip + effectiveLimit
		shardRequest.Limit = &combined
	} else {
		shardRequest.Limit = nil
	}
	if q.CountByRepo {
		shardRequest.Limit = nil
	}
	return shardRequest
}

// fanOutShards posts the shard request to every backend in parallel; the returned
// channel receives one result (or per-shard error) per backend.
func fanOutShards(o routerOptions, shardRequest api.SearchRequest, reqID string) chan backendResult {
	ch := make(chan backendResult, len(o.backends))
	for _, b := range o.backends {
		go func(base string) {
			started := time.Now()
			send := func(r backendResult) {
				r.base = base
				r.elapsed = time.Since(started)
				ch <- r
			}
			data, _ := json.Marshal(shardRequest)
			req, _ := http.NewRequest("POST", base+"/search", bytes.NewReader(data))
			req.Header.Set("content-type", "application/json")
			req.Header.Set("X-Request-Id", reqID)
			resp, e := (&http.Client{Timeout: o.timeout}).Do(req)
			if e != nil {
				send(backendResult{err: fmt.Errorf("backend %s failed: %w", base, e)})
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				send(backendResult{err: fmt.Errorf("backend %s returned %d", base, resp.StatusCode)})
				return
			}
			var x api.SearchResponse
			if err := json.NewDecoder(resp.Body).Decode(&x); err != nil {
				send(backendResult{err: fmt.Errorf("decode backend %s response: %w", base, err)})
			} else {
				send(backendResult{resp: x})
			}
		}(b)
	}
	return ch
}

// collectShardResults gathers the fan-out and logs one structured line per
// search with per-shard timings, so a slow or failing shard is visible
// without per-file noise (see docs/logging.md).
func collectShardResults(o routerOptions, ch chan backendResult, p search.Params, q api.SearchRequest, reqID string, start time.Time) (all []api.FileResult, repoCounts []api.RepoCount, truncated bool, backendErrors []error) {
	stats := make([]shardStat, 0, len(o.backends))
	for range o.backends {
		result := <-ch
		all = append(all, result.resp.Results...)
		repoCounts = append(repoCounts, result.resp.RepoCounts...)
		truncated = truncated || result.resp.Truncated
		ok := result.err == nil
		if !ok {
			backendErrors = append(backendErrors, result.err)
		}
		stats = append(stats, shardStat{Backend: shardName(result.base), Ms: result.elapsed.Milliseconds(), OK: ok})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Ms > stats[j].Ms })
	total := time.Since(start)
	lg := o.logger
	if lg == nil {
		lg = zap.NewNop()
	}
	lg.Info("search", []zap.Field{
		zap.String("reqId", reqID),
		zap.Int64("totalMs", total.Milliseconds()),
		zap.Int("queryLen", len(p.Query)),
		zap.Bool("countByRepo", q.CountByRepo),
		zap.Bool("files", q.Files),
		zap.Int("backends", len(o.backends)),
		zap.Int("failed", len(backendErrors)),
		zap.Any("shards", stats),
	}...)
	return all, repoCounts, truncated, backendErrors
}

// shardErrorStrings renders the per-shard failures for the wire response.
func shardErrorStrings(backendErrors []error) []string {
	var shardErrors []string
	for _, err := range backendErrors {
		shardErrors = append(shardErrors, err.Error())
	}
	return shardErrors
}

// mergeRepoCounts sums per-repo tallies across shards. Sharding is by repo,
// so each repo appears on one shard, but summing is safe regardless.
func mergeRepoCounts(repoCounts []api.RepoCount) []api.RepoCount {
	merged := map[string]api.RepoCount{}
	for _, c := range repoCounts {
		acc := merged[c.Repo]
		acc.Repo = c.Repo
		acc.Files += c.Files
		acc.Matches += c.Matches
		merged[c.Repo] = acc
	}
	out := make([]api.RepoCount, 0, len(merged))
	for _, c := range merged {
		out = append(out, c)
	}
	search.SortRepoCounts(out)
	return out
}

// mergeShardResults applies the deterministic global order by path
// (owner/repo/dir/file — no relevance ranking: results are a stable,
// reproducible list an agent narrows with filters rather than trusting an
// opaque score), dedupes, and applies the final skip + limit window.
func mergeShardResults(all []api.FileResult, p search.Params) []api.FileResult {
	sort.Slice(all, func(i, j int) bool {
		return all[i].Path < all[j].Path
	})
	seen := map[string]bool{}
	out := []api.FileResult{}
	for _, x := range all {
		if !seen[x.Path] {
			seen[x.Path] = true
			out = append(out, x)
		}
	}
	if p.Skip > 0 {
		if p.Skip >= len(out) {
			out = out[:0]
		} else {
			out = out[p.Skip:]
		}
	}
	limit := p.Limit
	if p.MaxFiles > 0 && (limit == 0 || p.MaxFiles < limit) {
		limit = p.MaxFiles
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// shardName trims a backend base URL to a short, log-friendly identifier (the
// first host label), e.g. http://grepple-shard-5.grepple-shard-headless:8787 ->
// grepple-shard-5. Falls back to the raw base when it can't be parsed.
func shardName(base string) string {
	s := base
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, ":/"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return base
	}
	return s
}

// newRequestID returns a short random hex id used to correlate one search across
// the router log line and every shard's log line (sent as the X-Request-Id
// header). It is best-effort: on the astronomically unlikely rand failure it
// falls back to a timestamp so the field is never empty.
func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "t" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
