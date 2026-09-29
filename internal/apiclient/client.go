// Package apiclient provides the typed client for Grepple's remote HTTP API.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/greppleai/grepple/internal/config"
	"github.com/greppleai/grepple/internal/linerange"
	"github.com/greppleai/grepple/internal/wire"
)

const (
	maxErrorBody     = 64 << 10
	maxResponseBody  = 64 << 20
	tokenRefreshSkew = 5 * time.Minute
)

// APIClient is the typed boundary for Grepple server operations.
type APIClient interface {
	Search(context.Context, string, wire.SearchRequest) (wire.SearchResponse, error)
	ResolveNavigation(context.Context, string, wire.NavigationResolveRequest) (wire.NavigationResolveResponse, error)
	Grit(context.Context, string, wire.GritRequest) (wire.GritResponse, error)
	Analysis(context.Context, string, wire.AnalysisRequest) (wire.AnalysisResponse, error)
	Raw(context.Context, string, RawRequest) (RawResponse, error)
	Repos(context.Context, string) ([]wire.RepoListEntry, error)
	Tree(context.Context, string, TreeRequest) (wire.TreeResponse, error)
	CreateRule(context.Context, string, wire.Rule) (wire.Rule, error)
	Rules(context.Context, string) (wire.RuleSet, error)
	Rule(context.Context, string, string) (wire.Rule, error)
	DeleteRule(context.Context, string, string) error
	RuleResults(context.Context, string, string) (wire.RuleResults, error)
	LoginConfig(context.Context, string) (LoginConfig, error)
	RequestDeviceCode(context.Context, string, string, string) (DeviceCode, error)
	PollDeviceToken(context.Context, string, string, DeviceCode, func(time.Duration)) (DeviceToken, error)
	GitHubLogin(context.Context, string, string) string
}

// RawRequest identifies source content and optional line-range/JSON projection.
type RawRequest struct {
	Repo       string
	Path       string
	Start      string
	End        string
	FormatJSON bool
}

// RawResponse carries source bytes and line-range response metadata.
type RawResponse struct {
	Body         []byte
	RangeOutcome linerange.Outcome
	RangeWarning string
}

// TreeRequest identifies one remote repository tree projection.
type TreeRequest struct {
	Repo  string
	Path  string
	Depth int
}

// HTTPError describes a non-successful server response.
type HTTPError struct {
	Server     string
	StatusCode int
	Body       string
	Header     http.Header
}

func (err *HTTPError) Error() string {
	return fmt.Sprintf("server %s returned %d: %s", err.Server, err.StatusCode, err.Body)
}

// RangeOutcome extracts line-range metadata from an HTTP error.
func RangeOutcome(err error) linerange.Outcome {
	var responseError *HTTPError
	if errors.As(err, &responseError) {
		return linerange.Outcome(responseError.Header.Get(linerange.HeaderOutcome))
	}
	return ""
}

// Option configures the HTTP-backed client.
type Option func(*apiClient)

// WithHTTPClient replaces the default HTTP transport.
func WithHTTPClient(client *http.Client) Option {
	return func(target *apiClient) {
		if client != nil {
			target.httpClient = client
		}
	}
}

// WithErrorOutput replaces the destination for non-fatal authentication warnings.
func WithErrorOutput(output io.Writer) Option {
	return func(target *apiClient) {
		if output != nil {
			target.errorOutput = output
		}
	}
}

// New constructs the default HTTP-backed API client.
func New(options ...Option) APIClient {
	client := &apiClient{httpClient: http.DefaultClient, errorOutput: os.Stderr, now: time.Now}
	for _, option := range options {
		option(client)
	}
	return client
}

type apiClient struct {
	httpClient  *http.Client
	errorOutput io.Writer
	now         func() time.Time
}

func (client *apiClient) Search(ctx context.Context, server string, request wire.SearchRequest) (wire.SearchResponse, error) {
	var response wire.SearchResponse
	err := client.json(ctx, http.MethodPost, endpoint(server, "/public/search"), request, &response, false)
	return response, err
}

func (client *apiClient) ResolveNavigation(ctx context.Context, server string, request wire.NavigationResolveRequest) (wire.NavigationResolveResponse, error) {
	var response wire.NavigationResolveResponse
	err := client.json(ctx, http.MethodPost, endpoint(server, "/public/navigation/resolve"), request, &response, false)
	return response, err
}

func (client *apiClient) Grit(ctx context.Context, server string, request wire.GritRequest) (wire.GritResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return wire.GritResponse{}, err
	}
	if len(body) > wire.MaxGritRequestBodyBytes {
		return wire.GritResponse{}, fmt.Errorf("structural request exceeds its maximum encoded size")
	}
	payload, err := client.request(ctx, http.MethodPost, endpoint(server, "/public/grit"), "application/json", bytes.NewReader(body), maxResponseBody)
	if err != nil {
		return wire.GritResponse{}, err
	}
	var response wire.GritResponse
	if err := decodeStrict(payload, &response, "structural"); err != nil {
		return wire.GritResponse{}, err
	}
	if response.Findings == nil {
		response.Findings = []wire.GritFinding{}
	}
	if response.Diagnostics == nil {
		response.Diagnostics = []wire.GritDiagnostic{}
	}
	if response.Truncations == nil {
		response.Truncations = []wire.GritTruncation{}
	}
	if response.ShardErrors == nil {
		response.ShardErrors = []string{}
	}
	return response, nil
}

func (client *apiClient) Analysis(ctx context.Context, server string, request wire.AnalysisRequest) (wire.AnalysisResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return wire.AnalysisResponse{}, err
	}
	payload, err := client.request(ctx, http.MethodPost, endpoint(server, "/public/analysis"), "application/json", bytes.NewReader(body), maxResponseBody)
	if err != nil {
		return wire.AnalysisResponse{}, err
	}
	var response wire.AnalysisResponse
	if err := decodeStrict(payload, &response, "analysis"); err != nil {
		return wire.AnalysisResponse{}, err
	}
	if err := validateAnalysisResponse(request, response); err != nil {
		return wire.AnalysisResponse{}, err
	}
	return response, nil
}

func (client *apiClient) Raw(ctx context.Context, server string, request RawRequest) (RawResponse, error) {
	target, err := url.Parse(endpoint(server, "/public/raw"))
	if err != nil {
		return RawResponse{}, err
	}
	query := target.Query()
	query.Set("repo", request.Repo)
	query.Set("path", request.Path)
	if request.Start != "" {
		query.Set("start", request.Start)
	}
	if request.End != "" {
		query.Set("end", request.End)
	}
	if request.FormatJSON {
		query.Set("format", "json")
	}
	target.RawQuery = query.Encode()
	response, err := client.do(ctx, http.MethodGet, target.String(), "", nil)
	if err != nil {
		return RawResponse{}, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(response.Body)
	return RawResponse{
		Body: body, RangeOutcome: linerange.Outcome(response.Header.Get(linerange.HeaderOutcome)), RangeWarning: response.Header.Get(linerange.HeaderWarning),
	}, readErr
}

func (client *apiClient) Repos(ctx context.Context, server string) ([]wire.RepoListEntry, error) {
	var response wire.ReposResponse
	if err := client.json(ctx, http.MethodGet, endpoint(server, "/public/repos"), nil, &response, false); err != nil {
		return nil, err
	}
	return response.Repos, nil
}

func (client *apiClient) Tree(ctx context.Context, server string, request TreeRequest) (wire.TreeResponse, error) {
	target, err := url.Parse(endpoint(server, "/public/tree"))
	if err != nil {
		return wire.TreeResponse{}, err
	}
	query := target.Query()
	query.Set("repo", request.Repo)
	if request.Path != "" {
		query.Set("path", request.Path)
	}
	query.Set("depth", fmt.Sprint(request.Depth))
	target.RawQuery = query.Encode()
	var response wire.TreeResponse
	err = client.json(ctx, http.MethodGet, target.String(), nil, &response, false)
	return response, err
}

func (client *apiClient) CreateRule(ctx context.Context, server string, rule wire.Rule) (wire.Rule, error) {
	var response wire.Rule
	err := client.json(ctx, http.MethodPost, endpoint(server, "/public/rules"), rule, &response, false)
	return response, err
}

func (client *apiClient) Rules(ctx context.Context, server string) (wire.RuleSet, error) {
	var response wire.RuleSet
	err := client.json(ctx, http.MethodGet, endpoint(server, "/public/rules"), nil, &response, false)
	return response, err
}

func (client *apiClient) Rule(ctx context.Context, server, id string) (wire.Rule, error) {
	var response wire.Rule
	err := client.json(ctx, http.MethodGet, ruleEndpoint(server, id), nil, &response, false)
	return response, err
}

func (client *apiClient) DeleteRule(ctx context.Context, server, id string) error {
	_, err := client.request(ctx, http.MethodDelete, ruleEndpoint(server, id), "", nil, 0)
	return err
}

func (client *apiClient) RuleResults(ctx context.Context, server, id string) (wire.RuleResults, error) {
	var response wire.RuleResults
	err := client.json(ctx, http.MethodGet, ruleEndpoint(server, id)+"/results", nil, &response, false)
	return response, err
}

func (client *apiClient) json(ctx context.Context, method, target string, input, output any, strict bool) error {
	var body io.Reader
	contentType := ""
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
		contentType = "application/json"
	}
	payload, err := client.request(ctx, method, target, contentType, body, maxResponseBody)
	if err != nil || output == nil {
		return err
	}
	if strict {
		return decodeStrict(payload, output, "JSON")
	}
	return json.Unmarshal(payload, output)
}

func (client *apiClient) request(ctx context.Context, method, target, contentType string, body io.Reader, maxBytes int64) ([]byte, error) {
	response, err := client.do(ctx, method, target, contentType, body)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if maxBytes <= 0 {
		return nil, nil
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxBytes {
		return nil, fmt.Errorf("server response exceeds its maximum size")
	}
	return payload, nil
}

func (client *apiClient) do(ctx context.Context, method, target, contentType string, body io.Reader) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("content-type", contentType)
	}
	if token := client.freshToken(target); token != "" {
		request.Header.Set("authorization", "Bearer "+token)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return response, nil
	}
	defer response.Body.Close()
	message, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	return nil, &HTTPError{Server: serverBaseURL(target), StatusCode: response.StatusCode, Body: string(message), Header: response.Header.Clone()}
}

func (client *apiClient) freshToken(target string) string {
	if token := os.Getenv("GREPPLE_TOKEN"); token != "" {
		return token
	}
	settings, err := config.LoadConfig("", true)
	if err != nil {
		fmt.Fprintf(client.errorOutput, "warning: user configuration unavailable (%v); using protected backend credentials\n", err)
	}
	credentials := settings.BackendCredentials()
	if credentials.Token == "" {
		return ""
	}
	if credentials.RefreshToken == "" || credentials.TokenExpiry == 0 || client.now().Add(tokenRefreshSkew).Unix() < credentials.TokenExpiry {
		return credentials.Token
	}
	refreshed, err := client.refreshLogin(serverBaseURL(target), credentials.RefreshToken)
	if err != nil {
		fmt.Fprintf(client.errorOutput, "warning: token refresh failed (%v) - retrying with the stored token\n", err)
		return credentials.Token
	}
	nextRefresh := refreshed.RefreshToken
	if nextRefresh == "" {
		nextRefresh = credentials.RefreshToken
	}
	if err := settings.StoreBackendLogin(refreshed.AccessToken, nextRefresh, refreshed.ExpiresIn, refreshed.RefreshTokenExpiresIn, ""); err != nil {
		return refreshed.AccessToken
	}
	return refreshed.AccessToken
}

type refreshTokenResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
}

func (client *apiClient) refreshLogin(server, refreshToken string) (refreshTokenResponse, error) {
	body, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(server, "/auth/refresh"), bytes.NewReader(body))
	if err != nil {
		return refreshTokenResponse{}, err
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("accept", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return refreshTokenResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return refreshTokenResponse{}, fmt.Errorf("refresh failed: HTTP %d", response.StatusCode)
	}
	var result refreshTokenResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return refreshTokenResponse{}, err
	}
	if result.AccessToken == "" {
		return refreshTokenResponse{}, fmt.Errorf("refresh response had no access token")
	}
	return result, nil
}

func decodeStrict(payload []byte, output any, label string) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("server %s response must contain exactly one JSON value", label)
	}
	return nil
}

func validateAnalysisResponse(request wire.AnalysisRequest, response wire.AnalysisResponse) error {
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
	expected := map[wire.AnalysisOperation]string{
		wire.AnalysisGraph: "grepple-navigation-graph-v7", wire.AnalysisArchitecture: "grepple-directory-architecture-v5",
		wire.AnalysisBoundaries: "grepple-boundaries-v3", wire.AnalysisResponsibilities: "grepple-directory-responsibilities-v2",
	}[request.Operation]
	if resultHeader.Schema != expected {
		return fmt.Errorf("server %s analysis schema %q is unsupported; expected %q", request.Operation, resultHeader.Schema, expected)
	}
	return nil
}

func endpoint(server, path string) string { return strings.TrimRight(server, "/") + path }
func ruleEndpoint(server, id string) string {
	return endpoint(server, "/public/rules/") + url.PathEscape(id)
}

func serverBaseURL(target string) string {
	if parsed, err := url.Parse(target); err == nil && parsed.Host != "" {
		return parsed.Scheme + "://" + parsed.Host
	}
	return strings.TrimRight(target, "/")
}
