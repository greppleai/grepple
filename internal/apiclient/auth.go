package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/greppleai/grepple/internal/config"
)

const deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// LoginConfig advertises the server-owned device login protocol.
type LoginConfig struct {
	Provider string `json:"provider"`
	ClientID string `json:"clientId"`
	Scopes   string `json:"scopes"`
}

// DeviceCode starts a server-owned device flow.
type DeviceCode struct {
	DeviceCode       string `json:"device_code"`
	UserCode         string `json:"user_code"`
	VerificationURI  string `json:"verification_uri"`
	ExpiresIn        int    `json:"expires_in"`
	Interval         int    `json:"interval"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// DeviceToken is the result of a server-owned device flow.
type DeviceToken struct {
	Login                 string `json:"login"`
	AccessToken           string `json:"access_token"`
	TokenType             string `json:"token_type"`
	Scope                 string `json:"scope"`
	ExpiresIn             int    `json:"expires_in"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error"`
	ErrorDescription      string `json:"error_description"`
	Interval              int    `json:"interval"`
}

func (client *apiClient) LoginConfig(ctx context.Context, server string) (LoginConfig, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(server, "/auth/config"), nil)
	if err != nil {
		return LoginConfig{}, err
	}
	request.Header.Set("accept", "application/json")
	response, err := client.authDo(request)
	if err != nil {
		return LoginConfig{}, fmt.Errorf("fetch login config from %s: %w", server, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return LoginConfig{}, fmt.Errorf("server %s does not support login (no /auth/config)", server)
	}
	if response.StatusCode >= http.StatusMultipleChoices {
		return LoginConfig{}, fmt.Errorf("fetch login config from %s failed (%d)", server, response.StatusCode)
	}
	var config LoginConfig
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&config); err != nil {
		return LoginConfig{}, fmt.Errorf("decode login config: %w", err)
	}
	return config, nil
}

func (client *apiClient) RequestDeviceCode(ctx context.Context, host, clientID, scope string) (DeviceCode, error) {
	form := url.Values{"client_id": {clientID}, "scope": {scope}}
	request, err := formRequest(ctx, endpoint(host, "/login/device/code"), form)
	if err != nil {
		return DeviceCode{}, err
	}
	response, err := client.authDo(request)
	if err != nil {
		return DeviceCode{}, err
	}
	defer response.Body.Close()
	var code DeviceCode
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&code); err != nil {
		return code, err
	}
	if code.Error != "" {
		return code, fmt.Errorf("device code request failed: %s", firstNonEmpty(code.ErrorDescription, code.Error))
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || code.DeviceCode == "" || code.UserCode == "" || code.ExpiresIn <= 0 || code.ExpiresIn > 3600 {
		return code, fmt.Errorf("device code request returned incomplete authorization data")
	}
	if code.Interval <= 0 {
		code.Interval = 5
	}
	verification, err := url.Parse(code.VerificationURI)
	origin, originErr := url.Parse(host)
	if err != nil || originErr != nil || verification.Scheme != origin.Scheme || verification.Host != origin.Host || verification.User != nil {
		return code, fmt.Errorf("server returned a verification URL outside its origin")
	}
	return code, nil
}

func (client *apiClient) PollDeviceToken(ctx context.Context, host, clientID string, code DeviceCode, sleep func(time.Duration)) (DeviceToken, error) {
	interval := time.Duration(code.Interval) * time.Second
	deadline := client.now().Add(time.Duration(code.ExpiresIn) * time.Second)
	for client.now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return DeviceToken{}, err
		}
		token, err := client.pollDevice(ctx, host, clientID, code.DeviceCode)
		if err != nil {
			return DeviceToken{}, err
		}
		if token.AccessToken != "" {
			return token, nil
		}
		switch token.Error {
		case "authorization_pending":
		case "slow_down":
			if token.Interval > 0 {
				interval = time.Duration(token.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return DeviceToken{}, fmt.Errorf("the device code expired before you authorized; run `grepple login` again")
		case "access_denied":
			return DeviceToken{}, fmt.Errorf("authorization was denied")
		case "":
			return DeviceToken{}, fmt.Errorf("token endpoint returned neither a token nor an error")
		default:
			return DeviceToken{}, fmt.Errorf("authorization failed: %s", firstNonEmpty(token.ErrorDescription, token.Error))
		}
		if sleep != nil {
			sleep(interval)
		}
	}
	return DeviceToken{}, fmt.Errorf("timed out waiting for authorization")
}

// RevokeLogin invalidates the saved session, not a GREPPLE_TOKEN override.
func (client *apiClient) RevokeLogin(ctx context.Context, server string) error {
	settings, _ := config.LoadConfig("", true)
	credentials := settings.BackendCredentials()
	if credentials.Token == "" || credentials.AuthServer != serverBaseURL(server) {
		return fmt.Errorf("no saved session for authentication server")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(server, "/auth/logout"), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+credentials.Token)
	response, err := client.authDo(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("session revocation failed (HTTP %d)", response.StatusCode)
	}
	return nil
}

func formRequest(ctx context.Context, target string, values url.Values) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("content-type", "application/x-www-form-urlencoded")
	return request, nil
}

func firstNonEmpty(first, second string) string {
	if strings.TrimSpace(first) != "" {
		return first
	}
	return second
}

func (client *apiClient) authDo(request *http.Request) (*http.Response, error) {
	transport := *client.httpClient
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if transport.Timeout == 0 {
		transport.Timeout = 30 * time.Second
	}
	return transport.Do(request)
}

func (client *apiClient) pollDevice(ctx context.Context, host, clientID, code string) (DeviceToken, error) {
	form := url.Values{"client_id": {clientID}, "device_code": {code}, "grant_type": {deviceGrantType}}
	request, err := formRequest(ctx, endpoint(host, "/login/oauth/access_token"), form)
	if err != nil {
		return DeviceToken{}, err
	}
	response, err := client.authDo(request)
	if err != nil {
		return DeviceToken{}, err
	}
	defer response.Body.Close()
	var token DeviceToken
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return DeviceToken{}, fmt.Errorf("decode device token: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if token.Error == "" {
			return DeviceToken{}, fmt.Errorf("device token endpoint returned status %d", response.StatusCode)
		}
		token.AccessToken = ""
	}
	return token, nil
}
