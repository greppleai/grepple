package router

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// appInstallation identifies a single GitHub App installation and the account
// (organization or user) whose repositories that installation should discover.
type appInstallation struct {
	Org            string `json:"org"`
	InstallationID int64  `json:"installationId"`
}

// appConfig is one GitHub App (an app id plus its RSA private key) together with
// the installations it should authenticate. A single app may declare many
// installations (for example one installation per organization across an
// enterprise), and many apps may be configured, each with their own
// installations. For the common single-installation case the Org and
// InstallationID fields may be set directly instead of Installations.
type appConfig struct {
	AppID          int64             `json:"appId"`
	PrivateKey     string            `json:"privateKey"`
	PrivateKeyFile string            `json:"privateKeyFile"`
	Installations  []appInstallation `json:"installations"`
	Org            string            `json:"org"`
	InstallationID int64             `json:"installationId"`
}

// githubAppsConfig is the object form accepted by GITHUB_APP_CONFIG /
// GITHUB_APP_CONFIG_FILE. A bare JSON array of appConfig is also accepted.
type githubAppsConfig struct {
	Apps []appConfig `json:"apps"`
}

// installationToken mints and caches an installation access token for one
// GitHub App installation. Installation tokens expire after roughly an hour, so
// the token is refreshed lazily shortly before it expires.
type installationToken struct {
	appID          int64
	installationID int64
	org            string
	key            *rsa.PrivateKey
	apiBase        string
	client         *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (it *installationToken) httpClient() *http.Client {
	if it.client != nil {
		return it.client
	}
	return http.DefaultClient
}

// get returns a valid installation access token, minting a new one when the
// cache is empty or within a minute of expiry.
func (it *installationToken) get() (string, error) {
	it.mu.Lock()
	defer it.mu.Unlock()
	if it.token != "" && time.Now().Before(it.expiry.Add(-time.Minute)) {
		return it.token, nil
	}
	assertion, err := signAppJWT(it.appID, it.key, time.Now())
	if err != nil {
		return "", err
	}
	target := fmt.Sprintf("%s/app/installations/%d/access_tokens", strings.TrimRight(it.apiBase, "/"), it.installationID)
	req, _ := http.NewRequest(http.MethodPost, target, nil)
	req.Header.Set("authorization", "Bearer "+assertion)
	req.Header.Set("accept", "application/vnd.github+json")
	req.Header.Set("user-agent", "grepple-router")
	req.Header.Set("x-github-api-version", "2022-11-28")
	resp, err := it.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("installation %d token request failed (%d): %s", it.installationID, resp.StatusCode, strings.TrimSpace(string(body[:min(len(body), 200)])))
	}
	var parsed struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if parsed.Token == "" {
		return "", fmt.Errorf("installation %d returned an empty token", it.installationID)
	}
	it.token = parsed.Token
	if parsed.ExpiresAt.IsZero() {
		it.expiry = time.Now().Add(time.Hour)
	} else {
		it.expiry = parsed.ExpiresAt
	}
	return it.token, nil
}

// githubAuth resolves the GitHub token to use for API discovery and cloning. It
// prefers a GitHub App installation token scoped to the repository's owning
// organization and falls back to the static personal access token (useful for
// local testing) when no installation matches.
type githubAuth struct {
	fallback string
	byOrg    map[string]*installationToken
	installs []*installationToken
}

func (a *githubAuth) hasInstallations() bool {
	return a != nil && len(a.installs) > 0
}

func (a *githubAuth) installations() []*installationToken {
	if a == nil {
		return nil
	}
	return a.installs
}

// tokenForOrg returns the token to use for the given organization: the matching
// installation token when configured, otherwise the fallback PAT.
func (a *githubAuth) tokenForOrg(org string) string {
	if a == nil {
		return ""
	}
	if it, ok := a.byOrg[strings.ToLower(org)]; ok {
		if token, err := it.get(); err == nil && token != "" {
			return token
		}
	}
	return a.fallback
}

// tokenForRepo resolves the token for an "owner/name" repository.
func (a *githubAuth) tokenForRepo(repo string) string {
	return a.tokenForOrg(orgOf(repo))
}

// orgOf returns the owner segment of an "owner/name" repository identifier.
func orgOf(repo string) string {
	if i := strings.IndexByte(repo, '/'); i >= 0 {
		return repo[:i]
	}
	return repo
}

// buildGithubAuth validates the app configs, parses each private key once, and
// builds the org -> installation token routing table. fallback is the static
// PAT (may be empty). apiBase is the GitHub REST base URL.
func buildGithubAuth(fallback string, apps []appConfig, apiBase string) (*githubAuth, error) {
	auth := &githubAuth{fallback: fallback, byOrg: map[string]*installationToken{}}
	for _, app := range apps {
		if app.AppID == 0 {
			return nil, fmt.Errorf("github app config is missing appId")
		}
		key, err := appPrivateKey(app)
		if err != nil {
			return nil, err
		}
		installs, err := appInstallations(app)
		if err != nil {
			return nil, err
		}
		if err := registerInstallations(auth, app, key, installs, apiBase); err != nil {
			return nil, err
		}
	}
	return auth, nil
}

// appPrivateKey loads and parses one app's PEM private key, falling back to
// the configured file when the inline key is empty.
func appPrivateKey(app appConfig) (*rsa.PrivateKey, error) {
	keyPEM := app.PrivateKey
	if strings.TrimSpace(keyPEM) == "" && app.PrivateKeyFile != "" {
		raw, err := os.ReadFile(app.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("app %d: read privateKeyFile: %w", app.AppID, err)
		}
		keyPEM = string(raw)
	}
	if strings.TrimSpace(keyPEM) == "" {
		return nil, fmt.Errorf("app %d: privateKey or privateKeyFile is required", app.AppID)
	}
	key, err := parsePrivateKey(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("app %d: %w", app.AppID, err)
	}
	return key, nil
}

// appInstallations normalizes the app's installation list: the single
// org + installationId shorthand becomes a one-element list, and at least
// one installation is required.
func appInstallations(app appConfig) ([]appInstallation, error) {
	installs := app.Installations
	if len(installs) == 0 && (app.Org != "" || app.InstallationID != 0) {
		installs = []appInstallation{{Org: app.Org, InstallationID: app.InstallationID}}
	}
	if len(installs) == 0 {
		return nil, fmt.Errorf("app %d: at least one installation (org + installationId) is required", app.AppID)
	}
	return installs, nil
}

// registerInstallations routes each installation's org to its token source,
// rejecting incomplete entries and orgs claimed by two installations.
func registerInstallations(auth *githubAuth, app appConfig, key *rsa.PrivateKey, installs []appInstallation, apiBase string) error {
	for _, inst := range installs {
		if inst.Org == "" || inst.InstallationID == 0 {
			return fmt.Errorf("app %d: each installation requires org and installationId", app.AppID)
		}
		orgKey := strings.ToLower(inst.Org)
		if _, exists := auth.byOrg[orgKey]; exists {
			return fmt.Errorf("org %q is configured for more than one installation", inst.Org)
		}
		it := &installationToken{
			appID:          app.AppID,
			installationID: inst.InstallationID,
			org:            inst.Org,
			key:            key,
			apiBase:        apiBase,
		}
		auth.byOrg[orgKey] = it
		auth.installs = append(auth.installs, it)
	}
	return nil
}

func parsePrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, fmt.Errorf("invalid PEM private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("unsupported private key (want PKCS#1 or PKCS#8 RSA): %w", err)
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not an RSA key")
	}
	return rsaKey, nil
}

// signAppJWT builds a short-lived RS256 JWT used to authenticate as the GitHub
// App when requesting installation access tokens.
func signAppJWT(appID int64, key *rsa.PrivateKey, now time.Time) (string, error) {
	header := base64URL([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-30 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": strconv.FormatInt(appID, 10),
	})
	if err != nil {
		return "", err
	}
	signingInput := header + "." + base64URL(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64URL(signature), nil
}

func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// loadAppConfigs reads GitHub App configuration from GITHUB_APP_CONFIG (inline
// JSON), then the fileFlag path (--github-app-config) or GITHUB_APP_CONFIG_FILE,
// and merges the flat single-app GITHUB_APP_* environment convenience form.
func loadAppConfigs(fileFlag string) ([]appConfig, error) {
	var apps []appConfig
	raw := ""
	if inline := strings.TrimSpace(os.Getenv("GITHUB_APP_CONFIG")); inline != "" {
		raw = inline
	} else {
		file := strings.TrimSpace(fileFlag)
		if file == "" {
			file = strings.TrimSpace(os.Getenv("GITHUB_APP_CONFIG_FILE"))
		}
		if file != "" {
			b, err := os.ReadFile(file)
			if err != nil {
				return nil, fmt.Errorf("github app config file %q: %w", file, err)
			}
			raw = string(b)
		}
	}
	if strings.TrimSpace(raw) != "" {
		parsed, err := parseAppConfigJSON([]byte(raw))
		if err != nil {
			return nil, err
		}
		apps = append(apps, parsed...)
	}
	flat, ok, err := flatAppConfig()
	if err != nil {
		return nil, err
	}
	if ok {
		apps = append(apps, flat)
	}
	return apps, nil
}

func parseAppConfigJSON(raw []byte) ([]appConfig, error) {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "[") {
		var apps []appConfig
		if err := json.Unmarshal(raw, &apps); err != nil {
			return nil, fmt.Errorf("github app config JSON: %w", err)
		}
		return apps, nil
	}
	var wrapper githubAppsConfig
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil, fmt.Errorf("github app config JSON: %w", err)
	}
	return wrapper.Apps, nil
}

// flatAppConfig assembles a single appConfig from the GITHUB_APP_* environment
// variables, returning ok=false when GITHUB_APP_ID is unset.
func flatAppConfig() (appConfig, bool, error) {
	idStr := strings.TrimSpace(os.Getenv("GITHUB_APP_ID"))
	if idStr == "" {
		return appConfig{}, false, nil
	}
	appID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return appConfig{}, false, fmt.Errorf("GITHUB_APP_ID: %w", err)
	}
	cfg := appConfig{
		AppID:          appID,
		PrivateKey:     os.Getenv("GITHUB_APP_PRIVATE_KEY"),
		PrivateKeyFile: strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE")),
	}
	if list := strings.TrimSpace(os.Getenv("GITHUB_APP_INSTALLATIONS")); list != "" {
		installs, err := parseInstallationList(list)
		if err != nil {
			return appConfig{}, false, err
		}
		cfg.Installations = installs
	} else if org := strings.TrimSpace(os.Getenv("GITHUB_APP_ORG")); org != "" {
		instID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("GITHUB_APP_INSTALLATION_ID")), 10, 64)
		if err != nil {
			return appConfig{}, false, fmt.Errorf("GITHUB_APP_INSTALLATION_ID: %w", err)
		}
		cfg.Installations = []appInstallation{{Org: org, InstallationID: instID}}
	}
	return cfg, true, nil
}

// parseInstallationList parses "org:installationId,org2:installationId2".
func parseInstallationList(list string) ([]appInstallation, error) {
	var installs []appInstallation
	for _, pair := range strings.Split(list, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		org, idStr, found := strings.Cut(pair, ":")
		if !found {
			return nil, fmt.Errorf("GITHUB_APP_INSTALLATIONS entry %q must be org:installationId", pair)
		}
		instID, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("GITHUB_APP_INSTALLATIONS entry %q: %w", pair, err)
		}
		installs = append(installs, appInstallation{Org: strings.TrimSpace(org), InstallationID: instID})
	}
	return installs, nil
}
