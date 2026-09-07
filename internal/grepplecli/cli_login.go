package grepplecli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// GitHub OAuth device-flow grant. The device flow is used because grepple is a
// CLI with no redirect URI and no client secret: the user authorizes in a
// browser and the CLI polls for the token.
const deviceGrantType = "urn:ietf:params:oauth:grant-type:device_code"

type deviceCodeResponse struct {
	DeviceCode       string `json:"device_code"`
	UserCode         string `json:"user_code"`
	VerificationURI  string `json:"verification_uri"`
	ExpiresIn        int    `json:"expires_in"`
	Interval         int    `json:"interval"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type deviceTokenResponse struct {
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

func envDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// githubWebHost / githubAPIHost are overridable (GHE, or tests pointing at a
// mock server).
func githubWebHost() string { return envDefault("GREPPLE_GITHUB_HOST", "https://github.com") }
func githubAPIHost() string { return envDefault("GREPPLE_GITHUB_API", "https://api.github.com") }

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// requestDeviceCode starts the device flow and returns the user/device codes.
func requestDeviceCode(client *http.Client, host, clientID, scope string) (deviceCodeResponse, error) {
	form := url.Values{"client_id": {clientID}, "scope": {scope}}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(host, "/")+"/login/device/code", strings.NewReader(form.Encode()))
	if err != nil {
		return deviceCodeResponse{}, err
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return deviceCodeResponse{}, err
	}
	defer resp.Body.Close()
	var dc deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return dc, err
	}
	if dc.Error != "" {
		return dc, fmt.Errorf("device code request failed: %s", firstNonEmpty(dc.ErrorDescription, dc.Error))
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return dc, fmt.Errorf("device code request returned no code (is the client ID a device-flow-enabled GitHub App/OAuth App?)")
	}
	if dc.Interval <= 0 {
		dc.Interval = 5
	}
	return dc, nil
}

// pollDeviceToken polls the token endpoint until the user authorizes, the code
// expires, or authorization is denied. sleep is injected so tests run instantly.
func pollDeviceToken(client *http.Client, host, clientID string, dc deviceCodeResponse, sleep func(time.Duration)) (deviceTokenResponse, error) {
	interval := time.Duration(dc.Interval) * time.Second
	deadline := time.Now().Add(time.Duration(max(dc.ExpiresIn, 60)) * time.Second)
	for time.Now().Before(deadline) {
		form := url.Values{
			"client_id":   {clientID},
			"device_code": {dc.DeviceCode},
			"grant_type":  {deviceGrantType},
		}
		req, err := http.NewRequest(http.MethodPost, strings.TrimRight(host, "/")+"/login/oauth/access_token", strings.NewReader(form.Encode()))
		if err != nil {
			return deviceTokenResponse{}, err
		}
		req.Header.Set("accept", "application/json")
		req.Header.Set("content-type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			return deviceTokenResponse{}, err
		}
		var tr deviceTokenResponse
		_ = json.NewDecoder(resp.Body).Decode(&tr)
		resp.Body.Close()
		if tr.AccessToken != "" {
			return tr, nil
		}
		switch tr.Error {
		case "authorization_pending":
			// keep waiting
		case "slow_down":
			if tr.Interval > 0 {
				interval = time.Duration(tr.Interval) * time.Second
			} else {
				interval += 5 * time.Second
			}
		case "expired_token":
			return deviceTokenResponse{}, fmt.Errorf("the device code expired before you authorized; run `grepple login` again")
		case "access_denied":
			return deviceTokenResponse{}, fmt.Errorf("authorization was denied")
		case "":
			return deviceTokenResponse{}, fmt.Errorf("token endpoint returned neither a token nor an error")
		default:
			return deviceTokenResponse{}, fmt.Errorf("authorization failed: %s", firstNonEmpty(tr.ErrorDescription, tr.Error))
		}
		sleep(interval)
	}
	return deviceTokenResponse{}, fmt.Errorf("timed out waiting for authorization")
}

// fetchGitHubLogin returns the authenticated user's login (best-effort).
func fetchGitHubLogin(client *http.Client, apiHost, token string) string {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(apiHost, "/")+"/user", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var u struct {
		Login string `json:"login"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&u)
	return u.Login
}

func openBrowser(target string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{target}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		cmd, args = "xdg-open", []string{target}
	}
	return exec.Command(cmd, args...).Start()
}

// loginConfig is the router's advertised device-flow configuration.
type loginConfig struct {
	ClientID string `json:"clientId"`
	Scopes   string `json:"scopes"`
}

// fetchLoginConfig retrieves the GitHub client ID (and scopes) from the grepple
// server so the client ID lives only server-side, never in the CLI.
func fetchLoginConfig(client *http.Client, server string) (loginConfig, error) {
	var cfg loginConfig
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(server, "/")+"/auth/config", nil)
	if err != nil {
		return cfg, err
	}
	req.Header.Set("accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return cfg, fmt.Errorf("fetch login config from %s: %w", server, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return cfg, fmt.Errorf("server %s does not support login (no /auth/config)", server)
	}
	if resp.StatusCode >= 300 {
		return cfg, fmt.Errorf("fetch login config from %s failed (%d)", server, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode login config: %w", err)
	}
	return cfg, nil
}

// runLogin performs the GitHub device-flow login and stores the token in
// ~/.grepple/config.json. The GitHub client ID is fetched from the grepple server
// (--url), so it is configured only server-side.
func runLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	serverURL := fs.String("url", serverDefault(""), "grepple server URL to fetch the login client ID from")
	scope := fs.String("scope", "", "OAuth scopes, space-separated (overrides the server's advertised scopes)")
	noBrowser := fs.Bool("no-browser", false, "do not attempt to open a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	cfg, err := fetchLoginConfig(client, *serverURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return fmt.Errorf("server %s did not advertise a GitHub client ID (set GITHUB_CLIENT_ID on the router)", *serverURL)
	}
	scopes := firstNonEmpty(strings.TrimSpace(*scope), firstNonEmpty(strings.TrimSpace(cfg.Scopes), "read:user"))

	dc, err := requestDeviceCode(client, githubWebHost(), cfg.ClientID, scopes)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nTo authorize grepple, open:\n  %s\nand enter the code:\n  %s\n\n", dc.VerificationURI, dc.UserCode)
	if !*noBrowser {
		_ = openBrowser(dc.VerificationURI)
	}
	fmt.Fprintln(os.Stderr, "Waiting for authorization…")

	tok, err := pollDeviceToken(client, githubWebHost(), cfg.ClientID, dc, time.Sleep)
	if err != nil {
		return err
	}
	login := fetchGitHubLogin(client, githubAPIHost(), tok.AccessToken)
	if err := storeLogin(tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, tok.RefreshTokenExpiresIn, login); err != nil {
		return fmt.Errorf("store token: %w", err)
	}
	path, _ := userConfigPath()
	if login != "" {
		fmt.Fprintf(os.Stderr, "Logged in as %s. Token saved to %s\n", login, path)
	} else {
		fmt.Fprintf(os.Stderr, "Logged in. Token saved to %s\n", path)
	}
	if tok.RefreshToken != "" {
		fmt.Fprintln(os.Stderr, "This token auto-renews — you won't need to log in again until the refresh token expires.")
	}
	return nil
}

// runLogout removes the stored token.
func runLogout(_ []string) error {
	if err := clearToken(); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Logged out; token removed from config.")
	return nil
}
