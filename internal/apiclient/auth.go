package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

// LoginConfig is the router-advertised GitHub device-flow configuration.
type LoginConfig struct {
	ClientID string `json:"clientId"`
	Scopes   string `json:"scopes"`
}

// DeviceCode starts a GitHub OAuth device flow.
type DeviceCode struct {
	DeviceCode       string `json:"device_code"`
	UserCode         string `json:"user_code"`
	VerificationURI  string `json:"verification_uri"`
	ExpiresIn        int    `json:"expires_in"`
	Interval         int    `json:"interval"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// DeviceToken is the result of a GitHub OAuth device flow.
type DeviceToken struct {
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
	response, err := client.httpClient.Do(request)
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
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
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
	response, err := client.httpClient.Do(request)
	if err != nil {
		return DeviceCode{}, err
	}
	defer response.Body.Close()
	var code DeviceCode
	if err := json.NewDecoder(response.Body).Decode(&code); err != nil {
		return code, err
	}
	if code.Error != "" {
		return code, fmt.Errorf("device code request failed: %s", firstNonEmpty(code.ErrorDescription, code.Error))
	}
	if code.DeviceCode == "" || code.UserCode == "" {
		return code, fmt.Errorf("device code request returned no code (is the client ID a device-flow-enabled GitHub App/OAuth App?)")
	}
	if code.Interval <= 0 {
		code.Interval = 5
	}
	return code, nil
}

func (client *apiClient) PollDeviceToken(ctx context.Context, host, clientID string, code DeviceCode, sleep func(time.Duration)) (DeviceToken, error) {
	interval := time.Duration(code.Interval) * time.Second
	deadline := client.now().Add(time.Duration(max(code.ExpiresIn, 60)) * time.Second)
	for client.now().Before(deadline) {
		form := url.Values{"client_id": {clientID}, "device_code": {code.DeviceCode}, "grant_type": {deviceGrantType}}
		request, err := formRequest(ctx, endpoint(host, "/login/oauth/access_token"), form)
		if err != nil {
			return DeviceToken{}, err
		}
		response, err := client.httpClient.Do(request)
		if err != nil {
			return DeviceToken{}, err
		}
		var token DeviceToken
		_ = json.NewDecoder(response.Body).Decode(&token)
		response.Body.Close()
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

func (client *apiClient) GitHubLogin(ctx context.Context, apiHost, token string) string {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(apiHost, "/user"), nil)
	if err != nil {
		return ""
	}
	request.Header.Set("authorization", "Bearer "+token)
	request.Header.Set("accept", "application/vnd.github+json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	var user struct {
		Login string `json:"login"`
	}
	_ = json.NewDecoder(response.Body).Decode(&user)
	return user.Login
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
