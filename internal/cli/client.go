package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/search"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func searchRemote(options *cliOptions, server string) ([]api.FileResult, error) {
	return searchRemoteContext(context.Background(), options, server)
}

func searchRemoteContext(ctx context.Context, options *cliOptions, server string) ([]api.FileResult, error) {
	body, err := json.Marshal(searchRequestFromParams(options.Params))
	if err != nil {
		return nil, err
	}
	req, err := authorizedRequest(http.MethodPost, strings.TrimRight(server, "/")+"/public/search", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
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

func requestGritRemote(ctx context.Context, request api.GritRequest, server string) (api.GritResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return api.GritResponse{}, err
	}
	if len(body) > api.MaxGritRequestBodyBytes {
		return api.GritResponse{}, fmt.Errorf("structural request exceeds its maximum encoded size")
	}
	httpRequest, err := authorizedRequest(http.MethodPost, strings.TrimRight(server, "/")+"/public/grit", "application/json", bytes.NewReader(body))
	if err != nil {
		return api.GritResponse{}, err
	}
	httpRequest = httpRequest.WithContext(ctx)
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return api.GritResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return api.GritResponse{}, fmt.Errorf("server %s returned %d: %s", server, response.StatusCode, string(message))
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, (64<<20)+1))
	if err != nil {
		return api.GritResponse{}, err
	}
	if len(payload) > 64<<20 {
		return api.GritResponse{}, fmt.Errorf("server structural response exceeds its maximum size")
	}
	var result api.GritResponse
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return api.GritResponse{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return api.GritResponse{}, fmt.Errorf("server structural response must contain exactly one JSON value")
	}
	if result.Findings == nil {
		result.Findings = []api.GritFinding{}
	}
	if result.Diagnostics == nil {
		result.Diagnostics = []api.GritDiagnostic{}
	}
	if result.Truncations == nil {
		result.Truncations = []api.GritTruncation{}
	}
	if result.ShardErrors == nil {
		result.ShardErrors = []string{}
	}
	return result, nil
}

func requestAnalysisRemote(ctx context.Context, request api.AnalysisRequest, server string) (api.AnalysisResponse, error) {
	request.ProductionOnly = request.ProductionOnly || activeRepositoryOptions.productionOnly
	request.NoConfigIgnore = request.NoConfigIgnore || activeRepositoryOptions.ignoreDisabled
	request.NoRepoConfig = request.NoRepoConfig || activeRepositoryOptions.disabled
	body, err := json.Marshal(request)
	if err != nil {
		return api.AnalysisResponse{}, err
	}
	httpRequest, err := authorizedRequest(http.MethodPost, strings.TrimRight(server, "/")+"/public/analysis", "application/json", bytes.NewReader(body))
	if err != nil {
		return api.AnalysisResponse{}, err
	}
	response, err := http.DefaultClient.Do(httpRequest.WithContext(ctx))
	if err != nil {
		return api.AnalysisResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return api.AnalysisResponse{}, fmt.Errorf("server %s returned %d: %s", server, response.StatusCode, string(message))
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, (64<<20)+1))
	if err != nil {
		return api.AnalysisResponse{}, err
	}
	if len(payload) > 64<<20 {
		return api.AnalysisResponse{}, fmt.Errorf("server analysis response exceeds its maximum size")
	}
	var result api.AnalysisResponse
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return api.AnalysisResponse{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return api.AnalysisResponse{}, fmt.Errorf("server analysis response must contain exactly one JSON value")
	}
	if err := validateAnalysisResponse(request, result); err != nil {
		return api.AnalysisResponse{}, err
	}
	warnRemoteAnalysis(result)
	return result, nil
}

func validateAnalysisResponse(request api.AnalysisRequest, response api.AnalysisResponse) error {
	if response.Schema != "grepple-remote-analysis-v1" {
		return fmt.Errorf("server analysis schema %q is unsupported", response.Schema)
	}
	if response.Operation != request.Operation || response.Repository != request.Repository {
		return fmt.Errorf("server returned mismatched analysis identity")
	}
	if !response.Found {
		return fmt.Errorf("repository not indexed: %s", request.Repository)
	}
	if len(response.Result) == 0 {
		return fmt.Errorf("server returned an empty %s analysis", request.Operation)
	}
	var resultHeader struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(response.Result, &resultHeader); err != nil {
		return fmt.Errorf("invalid %s analysis result: %w", request.Operation, err)
	}
	expected := map[api.AnalysisOperation]string{
		api.AnalysisGraph: "grepple-navigation-graph-v6", api.AnalysisArchitecture: "grepple-directory-architecture-v5",
		api.AnalysisBoundaries: "grepple-boundaries-v3", api.AnalysisResponsibilities: "grepple-directory-responsibilities-v2",
	}[request.Operation]
	if resultHeader.Schema != expected {
		return fmt.Errorf("server %s analysis schema %q is unsupported; expected %q", request.Operation, resultHeader.Schema, expected)
	}
	return nil
}

func warnRemoteAnalysis(response api.AnalysisResponse) {
	for _, notice := range response.Notices {
		fmt.Fprintln(os.Stderr, "analysis notice:", notice)
	}
	for _, shardError := range response.ShardErrors {
		fmt.Fprintln(os.Stderr, "partial analysis:", shardError)
	}
	if !response.Complete {
		fmt.Fprintln(os.Stderr, "analysis is incomplete; inspect result source counts and truncation")
	}
}

// searchRequestFromParams converts resolved CLI parameters into the wire
// request: zero values stay unset so the server applies its own defaults.
func searchRequestFromParams(params search.Params) api.SearchRequest {
	query := params.Query
	request := api.SearchRequest{
		Query:           &query,
		Globs:           params.Globs,
		Regex:           &params.Regex,
		IgnoreCase:      &params.IgnoreCase,
		InvertMatch:     &params.InvertMatch,
		Sort:            params.Sort,
		Files:           params.Files,
		LineRanges:      params.LineRanges,
		EnclosingRanges: params.EnclosingRanges,
		Related:         params.Related,
		At:              params.At,
		FollowRelated:   params.FollowRelated,
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
		InvertMatch:  &params.InvertMatch,
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
