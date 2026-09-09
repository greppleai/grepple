package grepplecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"grepple/internal/api"
	"grepple/internal/search"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

func searchRemote(options *cliOptions, server string) ([]api.FileResult, error) {
	body, err := json.Marshal(searchRequestFromParams(options.Params))
	if err != nil {
		return nil, err
	}
	req, err := authorizedRequest(http.MethodPost, strings.TrimRight(server, "/")+"/public/search", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("server %s returned %d: %s", server, response.StatusCode, string(message))
	}
	var result api.SearchResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	warnPartialResults(result)
	return dropExcludedRepos(result.Results, options.Params.ExcludeRepo), nil
}

// searchRequestFromParams converts resolved CLI parameters into the wire
// request: zero values stay unset so the server applies its own defaults.
func searchRequestFromParams(params search.Params) api.SearchRequest {
	query := params.Query
	request := api.SearchRequest{
		Query:       &query,
		Globs:       params.Globs,
		Regex:       &params.Regex,
		IgnoreCase:  &params.IgnoreCase,
		MaxSegments: &params.MaxSegments,
		Files:       params.Files,
	}
	if len(params.Repo) > 0 {
		request.Repo = params.Repo
	}
	if len(params.ExcludeRepo) > 0 {
		request.ExcludeRepo = params.ExcludeRepo
	}
	if params.MaxFiles > 0 {
		request.MaxFiles = &params.MaxFiles
	}
	if params.Skip > 0 {
		request.Skip = &params.Skip
	}
	if params.Limit > 0 {
		request.Limit = &params.Limit
	}
	if params.Context > 0 {
		request.Context = params.Context
	}
	if params.BeforeContext != params.Context {
		request.BeforeContext = &params.BeforeContext
	}
	if params.AfterContext != params.Context {
		request.AfterContext = &params.AfterContext
	}
	if params.SkipSegments {
		request.SkipSegments = true
	}
	return request
}

// warnPartialResults reports shard failures and index-cap truncation for a
// distributed search response on stderr.
func warnPartialResults(result api.SearchResponse) {
	if len(result.ShardErrors) > 0 {
		fmt.Fprintf(os.Stderr, "warning: partial results — %d shard(s) unavailable:\n", len(result.ShardErrors))
		for _, shardErr := range result.ShardErrors {
			fmt.Fprintf(os.Stderr, "  - %s\n", shardErr)
		}
	}
	if result.Truncated {
		fmt.Fprintln(os.Stderr, "note: results truncated (index result cap hit) — narrow with --repo or a more specific pattern for the complete set")
	}
}

// dropExcludedRepos removes results from excluded repositories client-side,
// behind the server-side filter.
func dropExcludedRepos(results []api.FileResult, exclude []string) []api.FileResult {
	if len(exclude) == 0 {
		return results
	}
	filtered := results[:0]
	for _, item := range results {
		if !contains(exclude, item.Repo) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// countRemote fetches per-repository match tallies from the router. It sends an
// unbounded CountByRepo probe (no paging window, no segments) so the counts are
// complete yet the payload is tiny regardless of how many files match.
func countRemote(options *cliOptions, server string) ([]api.RepoCount, error) {
	params := options.Params
	query := params.Query
	request := api.SearchRequest{
		Query:        &query,
		Globs:        params.Globs,
		Regex:        &params.Regex,
		IgnoreCase:   &params.IgnoreCase,
		CountByRepo:  true,
		SkipSegments: true,
	}
	if len(params.Repo) > 0 {
		request.Repo = params.Repo
	}
	if len(params.ExcludeRepo) > 0 {
		request.ExcludeRepo = params.ExcludeRepo
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := authorizedRequest(http.MethodPost, strings.TrimRight(server, "/")+"/public/search", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("server %s returned %d: %s", server, response.StatusCode, string(message))
	}
	var result api.SearchResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.ShardErrors) > 0 {
		fmt.Fprintf(os.Stderr, "warning: partial counts \u2014 %d shard(s) unavailable:\n", len(result.ShardErrors))
		for _, shardErr := range result.ShardErrors {
			fmt.Fprintf(os.Stderr, "  - %s\n", shardErr)
		}
	}
	if result.Truncated {
		fmt.Fprintln(os.Stderr, "note: counts truncated (index result cap hit) — narrow with --repo or a more specific pattern for exact totals")
	}
	return result.RepoCounts, nil
}

// authorizedRequest builds an HTTP request that carries the stored GitHub token
// as a bearer credential when one is configured (via `grepple login` or
// GREPPLE_TOKEN); otherwise it is an ordinary unauthenticated request.
func authorizedRequest(method, target, contentType string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("content-type", contentType)
	}
	if token := freshToken(target); token != "" {
		req.Header.Set("authorization", "Bearer "+token)
	}
	return req, nil
}

// tokenRefreshSkew renews a login token this long before it actually expires, so
// an in-flight request never races the expiry.
const tokenRefreshSkew = 5 * time.Minute

// freshToken returns a valid bearer token for requests to the given target URL,
// transparently refreshing a stored, expiring `grepple login` token shortly before
// it expires so developers are not forced to re-run `grepple login` every few
// hours. GREPPLE_TOKEN always wins and is used verbatim (its lifecycle belongs to
// the caller). A refresh failure is non-fatal: the existing token is returned so
// the request still goes out and surfaces a real 401 with the login hint, rather
// than turning a transient refresh error into a hard command failure.
func freshToken(target string) string {
	if v := os.Getenv("GREPPLE_TOKEN"); v != "" {
		return v
	}
	c := loadConfig()
	if c.Token == "" {
		return ""
	}
	if c.RefreshToken == "" || c.TokenExpiry == 0 {
		return c.Token // legacy or non-expiring token: nothing to refresh
	}
	if time.Now().Add(tokenRefreshSkew).Unix() < c.TokenExpiry {
		return c.Token // still comfortably valid
	}
	refreshed, err := refreshLogin(serverBaseURL(target), c.RefreshToken)
	if err != nil {
		// Non-fatal, but not silent: a routed-but-missing or misconfigured
		// refresh endpoint is otherwise invisible behind the resulting 401.
		fmt.Fprintf(os.Stderr, "warning: token refresh failed (%v) - retrying with the stored token\n", err)
		return c.Token
	}
	// GitHub rotates the refresh token; keep the old one only if none was
	// returned (e.g. a server that does not rotate).
	nextRefresh := refreshed.RefreshToken
	if nextRefresh == "" {
		nextRefresh = c.RefreshToken
	}
	if err := storeLogin(refreshed.AccessToken, nextRefresh, refreshed.ExpiresIn, refreshed.RefreshTokenExpiresIn, ""); err != nil {
		return refreshed.AccessToken // use it even if persisting failed
	}
	return refreshed.AccessToken
}

// refreshLogin asks the grepple server to exchange a refresh token for a new user
// access token. The exchange runs server-side because it needs the GitHub App
// client secret, which never leaves the router.
func refreshLogin(serverBase, refreshToken string) (deviceTokenResponse, error) {
	body, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(serverBase, "/")+"/auth/refresh", bytes.NewReader(body))
	if err != nil {
		return deviceTokenResponse{}, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return deviceTokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return deviceTokenResponse{}, fmt.Errorf("refresh failed: HTTP %d", resp.StatusCode)
	}
	var tr deviceTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return deviceTokenResponse{}, err
	}
	if tr.AccessToken == "" {
		return deviceTokenResponse{}, fmt.Errorf("refresh response had no access token")
	}
	return tr, nil
}

// serverBaseURL reduces a full request URL to its scheme://host origin so refresh
// hits the same server the request targets.
func serverBaseURL(target string) string {
	if u, err := url.Parse(target); err == nil && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	return strings.TrimRight(target, "/")
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func printFileResult(result api.FileResult) error {
	if err := SafeWrite(result.Path + "\n\n"); err != nil {
		return err
	}
	width := segmentLineWidth(result.Segments)
	sort.Slice(result.Segments, func(i, j int) bool {
		return result.Segments[i].Start < result.Segments[j].Start
	})
	cursor := 1
	for _, segment := range result.Segments {
		if segment.Start > cursor {
			if err := SafeWrite(Collapsed(segment.Start - cursor)); err != nil {
				return err
			}
		}
		if err := printSegment(segment, width); err != nil {
			return err
		}
		cursor = segment.End + 1
	}
	return nil
}

// segmentLineWidth returns the line-number column width needed for the
// largest segment end.
func segmentLineWidth(segments []api.ResultSegment) int {
	width := 1
	for _, segment := range segments {
		if digits := len(fmt.Sprint(segment.End)); digits > width {
			width = digits
		}
	}
	return width
}

// printSegment renders one segment with aligned line numbers: a summary
// segment prints only its header line, others print every covered line.
func printSegment(segment api.ResultSegment, width int) error {
	if segment.Kind == "summary" {
		return SafeWrite(fmt.Sprintf("%*d   %s\n", width, segment.Start, segment.Text))
	}
	for index, line := range strings.Split(segment.Text, "\n") {
		if err := SafeWrite(fmt.Sprintf("%*d   %s\n", width, segment.Start+index, line)); err != nil {
			return err
		}
	}
	return nil
}
