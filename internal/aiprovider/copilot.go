package aiprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"charm.land/fantasy"
	fantasycompat "charm.land/fantasy/providers/openaicompat"
)

const (
	copilotProviderName = "copilot"
	// GitHub's public Copilot OAuth application client ID is used by editor integrations.
	copilotClientID      = "Iv1.b507a08c87ecfe98"
	githubLoginBase      = "https://github.com/login"
	githubAPIBase        = "https://api.github.com"
	copilotAPIBase       = "https://api.githubcopilot.com"
	copilotEditorVersion = "vscode/1.95.0"
	copilotPluginVersion = "copilot-chat/0.26.7"
)

type copilotProvider struct {
	store      *Store
	client     *http.Client
	loginURL   string
	githubAPI  string
	copilotAPI string
	clientID   string
}

type copilotCredentials struct {
	GitHubToken string    `json:"githubToken"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type githubDeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type githubDeviceToken struct {
	AccessToken string `json:"access_token"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

type copilotToken struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

func newCopilotProvider(store *Store, client *http.Client) *copilotProvider {
	return &copilotProvider{
		store: store, client: client,
		loginURL:   environmentOr("GREPPLE_GITHUB_LOGIN_URL", githubLoginBase),
		githubAPI:  environmentOr("GREPPLE_GITHUB_API_URL", githubAPIBase),
		copilotAPI: environmentOr("GREPPLE_COPILOT_API_URL", copilotAPIBase),
		clientID:   environmentOr("GREPPLE_COPILOT_CLIENT_ID", copilotClientID),
	}
}

func (p *copilotProvider) Name() string { return copilotProviderName }

func (p *copilotProvider) DefaultModel() string { return "gpt-4.1" }

func (p *copilotProvider) Login(ctx context.Context, options LoginOptions) error {
	output := options.Output
	if output == nil {
		output = io.Discard
	}
	device, err := p.requestDeviceCode(ctx)
	if err != nil {
		return err
	}
	verificationURL := device.VerificationURI
	if verificationURL == "" {
		verificationURL = "https://github.com/login/device"
	}
	fmt.Fprintf(output, "\nTo authorize Grepple with GitHub Copilot, open:\n  %s\nand enter the code:\n  %s\n\n", verificationURL, device.UserCode)
	if !options.NoBrowser {
		_ = openBrowser(verificationURL)
	}
	token, err := p.pollDeviceToken(ctx, device)
	if err != nil {
		return err
	}
	if _, err := p.exchangeCopilotToken(ctx, token.AccessToken); err != nil {
		return fmt.Errorf("verify GitHub Copilot subscription: %w", err)
	}
	return p.store.Save(p.Name(), copilotCredentials{GitHubToken: token.AccessToken, UpdatedAt: time.Now().UTC()})
}

func (p *copilotProvider) Logout() error { return p.store.Delete(p.Name()) }

func (p *copilotProvider) LoggedIn() (bool, error) {
	var credentials copilotCredentials
	found, err := p.store.Load(p.Name(), &credentials)
	return found && credentials.GitHubToken != "", err
}

func (p *copilotProvider) LanguageModel(ctx context.Context, modelID string) (fantasy.LanguageModel, error) {
	var credentials copilotCredentials
	found, err := p.store.Load(p.Name(), &credentials)
	if err != nil {
		return nil, err
	}
	if !found || credentials.GitHubToken == "" {
		return nil, fmt.Errorf("GitHub Copilot is not logged in; run 'grepple ai-provider login copilot'")
	}
	token, err := p.exchangeCopilotToken(ctx, credentials.GitHubToken)
	if err != nil {
		return nil, err
	}
	provider, err := fantasycompat.New(
		fantasycompat.WithName(p.Name()),
		fantasycompat.WithAPIKey(token.Token),
		fantasycompat.WithBaseURL(p.copilotAPI),
		fantasycompat.WithHeaders(copilotHeaders()),
		fantasycompat.WithHTTPClient(p.client),
	)
	if err != nil {
		return nil, err
	}
	return provider.LanguageModel(ctx, modelID)
}

func (p *copilotProvider) requestDeviceCode(ctx context.Context) (githubDeviceCode, error) {
	var device githubDeviceCode
	values := url.Values{"client_id": {p.clientID}, "scope": {"read:user"}}
	if err := p.postGitHubForm(ctx, strings.TrimRight(p.loginURL, "/")+"/device/code", values, &device); err != nil {
		return device, err
	}
	if device.DeviceCode == "" || device.UserCode == "" {
		return device, fmt.Errorf("GitHub device authorization returned an incomplete code")
	}
	return device, nil
}

func (p *copilotProvider) pollDeviceToken(ctx context.Context, device githubDeviceCode) (githubDeviceToken, error) {
	timeout := time.Duration(device.ExpiresIn) * time.Second
	if timeout <= 0 || timeout > 15*time.Minute {
		timeout = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	interval := time.Duration(device.Interval) * time.Second
	if interval < time.Second {
		interval = 5 * time.Second
	}
	values := url.Values{
		"client_id": {p.clientID}, "device_code": {device.DeviceCode},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	for {
		var token githubDeviceToken
		if err := p.postGitHubForm(ctx, strings.TrimRight(p.loginURL, "/")+"/oauth/access_token", values, &token); err != nil {
			return token, err
		}
		switch token.Error {
		case "":
			if token.AccessToken == "" {
				return token, fmt.Errorf("GitHub device authorization returned an empty token")
			}
			return token, nil
		case "authorization_pending":
		case "slow_down":
			interval += 5 * time.Second
		case "expired_token", "access_denied":
			return token, fmt.Errorf("GitHub device authorization failed: %s", token.Error)
		default:
			return token, fmt.Errorf("GitHub device authorization failed: %s: %s", token.Error, token.Description)
		}
		select {
		case <-ctx.Done():
			return token, fmt.Errorf("GitHub device authorization: %w", ctx.Err())
		case <-time.After(interval):
		}
	}
}

func (p *copilotProvider) exchangeCopilotToken(ctx context.Context, githubToken string) (copilotToken, error) {
	var token copilotToken
	endpoint := strings.TrimRight(p.githubAPI, "/") + "/copilot_internal/v2/token"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return token, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "token "+githubToken)
	for key, value := range copilotHeaders() {
		request.Header.Set(key, value)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return token, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return token, fmt.Errorf("GitHub Copilot token exchange returned status %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return token, err
	}
	if token.Token == "" {
		return token, fmt.Errorf("GitHub Copilot token exchange returned an empty token")
	}
	return token, nil
}

func (p *copilotProvider) postGitHubForm(ctx context.Context, endpoint string, values url.Values, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("GitHub device authorization returned status %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target)
}

func copilotHeaders() map[string]string {
	return map[string]string{
		"Copilot-Integration-Id": "vscode-chat",
		"Editor-Version":         copilotEditorVersion,
		"Editor-Plugin-Version":  copilotPluginVersion,
		"User-Agent":             "GitHubCopilotChat/" + strings.TrimPrefix(copilotPluginVersion, "copilot-chat/"),
		"X-GitHub-Api-Version":   "2022-11-28",
		"OpenAI-Intent":          "conversation-panel",
	}
}
