package aiprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
)

const (
	codexProviderName = "codex"
	codexClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexIssuer       = "https://auth.openai.com"
	codexAPIBase      = "https://chatgpt.com/backend-api/codex"
)

type codexProvider struct {
	store  *Store
	client *http.Client
	issuer string
	apiURL string
}

type codexCredentials struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	IDToken      string    `json:"idToken"`
	AccountID    string    `json:"accountId"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type deviceCodeResponse struct {
	DeviceAuthID   string          `json:"device_auth_id"`
	UserCode       string          `json:"user_code"`
	LegacyUserCode string          `json:"usercode"`
	Interval       json.RawMessage `json:"interval"`
}

type deviceTokenResponse struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

func newCodexProvider(store *Store, client *http.Client) *codexProvider {
	return &codexProvider{store: store, client: client, issuer: environmentOr("GREPPLE_CODEX_ISSUER", codexIssuer), apiURL: environmentOr("GREPPLE_CODEX_API_URL", codexAPIBase)}
}

func (p *codexProvider) Name() string { return codexProviderName }

func (p *codexProvider) DefaultModel() string { return "gpt-5.3-codex" }

func (p *codexProvider) LoggedIn() (bool, error) {
	var credentials codexCredentials
	found, err := p.store.Load(p.Name(), &credentials)
	return found && credentials.AccessToken != "" && credentials.RefreshToken != "", err
}

func (p *codexProvider) Logout() error { return p.store.Delete(p.Name()) }

func (p *codexProvider) Login(ctx context.Context, options LoginOptions) error {
	output := options.Output
	if output == nil {
		output = io.Discard
	}
	device, interval, err := p.requestDeviceCode(ctx)
	if err != nil {
		return err
	}
	verificationURL := strings.TrimRight(p.issuer, "/") + "/codex/device"
	fmt.Fprintf(output, "Open %s and enter code %s\n", verificationURL, device.UserCode)
	if !options.NoBrowser {
		_ = openBrowser(verificationURL)
	}
	code, err := p.pollDeviceCode(ctx, device, interval)
	if err != nil {
		return err
	}
	tokens, err := p.exchangeCode(ctx, code)
	if err != nil {
		return err
	}
	credentials := codexCredentials{
		AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, IDToken: tokens.IDToken,
		AccountID: jwtAccountID(tokens.IDToken), UpdatedAt: time.Now().UTC(),
	}
	if credentials.AccountID == "" {
		credentials.AccountID = jwtAccountID(tokens.AccessToken)
	}
	if credentials.AccessToken == "" || credentials.RefreshToken == "" || credentials.AccountID == "" {
		return fmt.Errorf("Codex login returned incomplete credentials")
	}
	return p.store.Save(p.Name(), credentials)
}

func (p *codexProvider) LanguageModel(ctx context.Context, modelID string) (fantasy.LanguageModel, error) {
	credentials, err := p.credentials(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := fantasyopenai.New(
		fantasyopenai.WithName(p.Name()),
		fantasyopenai.WithAPIKey(credentials.AccessToken),
		fantasyopenai.WithBaseURL(p.apiURL),
		fantasyopenai.WithHeaders(map[string]string{"ChatGPT-Account-ID": credentials.AccountID, "originator": "grepple"}),
		fantasyopenai.WithHTTPClient(p.client),
		fantasyopenai.WithUseResponsesAPI(),
	)
	if err != nil {
		return nil, err
	}
	return provider.LanguageModel(ctx, modelID)
}

func (p *codexProvider) credentials(ctx context.Context) (codexCredentials, error) {
	var credentials codexCredentials
	found, err := p.store.Load(p.Name(), &credentials)
	if err != nil {
		return credentials, err
	}
	if !found || credentials.AccessToken == "" {
		return credentials, fmt.Errorf("Codex is not logged in; run 'grepple ai-provider login codex'")
	}
	if expiration := jwtExpiration(credentials.AccessToken); expiration.IsZero() {
		expiration = jwtExpiration(credentials.IDToken)
		if expiration.IsZero() || time.Until(expiration) > 5*time.Minute {
			return credentials, nil
		}
	} else if time.Until(expiration) > 5*time.Minute {
		return credentials, nil
	}
	return p.refresh(ctx, credentials)
}

func (p *codexProvider) requestDeviceCode(ctx context.Context) (deviceCodeResponse, time.Duration, error) {
	var response deviceCodeResponse
	err := p.postJSON(ctx, strings.TrimRight(p.issuer, "/")+"/api/accounts/deviceauth/usercode", map[string]string{"client_id": codexClientID}, &response, http.StatusOK)
	if err != nil {
		return response, 0, err
	}
	if response.UserCode == "" {
		response.UserCode = response.LegacyUserCode
	}
	interval := parseInterval(response.Interval)
	if interval < time.Second {
		interval = 5 * time.Second
	}
	if response.DeviceAuthID == "" || response.UserCode == "" {
		return response, 0, fmt.Errorf("Codex device authorization returned an incomplete code")
	}
	return response, interval, nil
}

func (p *codexProvider) pollDeviceCode(ctx context.Context, device deviceCodeResponse, interval time.Duration) (deviceTokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	endpoint := strings.TrimRight(p.issuer, "/") + "/api/accounts/deviceauth/token"
	for {
		var response deviceTokenResponse
		status, err := p.postJSONStatus(ctx, endpoint, map[string]string{"device_auth_id": device.DeviceAuthID, "user_code": device.UserCode}, &response)
		if err != nil {
			return response, err
		}
		if status >= 200 && status < 300 {
			if response.AuthorizationCode == "" || response.CodeVerifier == "" {
				return response, fmt.Errorf("Codex device authorization returned an incomplete token")
			}
			return response, nil
		}
		if status != http.StatusForbidden && status != http.StatusNotFound {
			return response, fmt.Errorf("Codex device authorization failed with status %d", status)
		}
		select {
		case <-ctx.Done():
			return response, fmt.Errorf("Codex device authorization: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (p *codexProvider) exchangeCode(ctx context.Context, code deviceTokenResponse) (oauthTokenResponse, error) {
	values := url.Values{
		"grant_type": {"authorization_code"}, "code": {code.AuthorizationCode},
		"redirect_uri": {strings.TrimRight(p.issuer, "/") + "/deviceauth/callback"},
		"client_id":    {codexClientID}, "code_verifier": {code.CodeVerifier},
	}
	return p.postForm(ctx, strings.TrimRight(p.issuer, "/")+"/oauth/token", values)
}

func (p *codexProvider) refresh(ctx context.Context, credentials codexCredentials) (codexCredentials, error) {
	var tokens oauthTokenResponse
	err := p.postJSON(ctx, strings.TrimRight(p.issuer, "/")+"/oauth/token", map[string]string{
		"client_id": codexClientID, "grant_type": "refresh_token", "refresh_token": credentials.RefreshToken,
	}, &tokens, http.StatusOK)
	if err != nil {
		return credentials, fmt.Errorf("refresh Codex credentials: %w", err)
	}
	if tokens.AccessToken != "" {
		credentials.AccessToken = tokens.AccessToken
	}
	if tokens.RefreshToken != "" {
		credentials.RefreshToken = tokens.RefreshToken
	}
	if tokens.IDToken != "" {
		credentials.IDToken = tokens.IDToken
		if accountID := jwtAccountID(tokens.IDToken); accountID != "" {
			credentials.AccountID = accountID
		}
	}
	credentials.UpdatedAt = time.Now().UTC()
	if err := p.store.Save(p.Name(), credentials); err != nil {
		return credentials, err
	}
	return credentials, nil
}

func (p *codexProvider) postJSON(ctx context.Context, endpoint string, value, target any, expected int) error {
	status, err := p.postJSONStatus(ctx, endpoint, value, target)
	if err != nil {
		return err
	}
	if status != expected {
		return fmt.Errorf("provider returned status %d", status)
	}
	return nil
}

func (p *codexProvider) postJSONStatus(ctx context.Context, endpoint string, value, target any) (int, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return 0, err
	}
	request.Header.Set("content-type", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode >= 200 && response.StatusCode < 300 && target != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target); err != nil {
			return response.StatusCode, err
		}
	}
	return response.StatusCode, nil
}

func (p *codexProvider) postForm(ctx context.Context, endpoint string, values url.Values) (oauthTokenResponse, error) {
	var tokens oauthTokenResponse
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return tokens, err
	}
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	response, err := p.client.Do(request)
	if err != nil {
		return tokens, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return tokens, fmt.Errorf("provider returned status %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokens); err != nil {
		return tokens, err
	}
	return tokens, nil
}

func parseInterval(raw json.RawMessage) time.Duration {
	value := strings.Trim(string(raw), `"`)
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 1 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func jwtPayload(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	content, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var payload map[string]any
	if json.Unmarshal(content, &payload) != nil {
		return nil
	}
	return payload
}

func jwtAccountID(token string) string {
	payload := jwtPayload(token)
	if value, ok := payload["chatgpt_account_id"].(string); ok {
		return value
	}
	if auth, ok := payload["https://api.openai.com/auth"].(map[string]any); ok {
		if value, ok := auth["chatgpt_account_id"].(string); ok {
			return value
		}
	}
	return ""
}

func jwtExpiration(token string) time.Time {
	value, ok := jwtPayload(token)["exp"].(float64)
	if !ok || value <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(value), 0)
}

func environmentOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func openBrowser(target string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{target}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		command, args = "xdg-open", []string{target}
	}
	return exec.Command(command, args...).Start()
}
