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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testKeyPEM(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block)), key
}

func TestSignAppJWTVerifies(t *testing.T) {
	pemStr, key := testKeyPEM(t)
	parsed, err := parsePrivateKey(pemStr)
	if err != nil {
		t.Fatalf("parsePrivateKey: %v", err)
	}
	token, err := signAppJWT(4242, parsed, time.Now())
	if err != nil {
		t.Fatalf("signAppJWT: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	// Signature must verify against the public key over header.claims.
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
	// Issuer claim must be the app id.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
		Exp int64  `json:"exp"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims.Iss != "4242" {
		t.Fatalf("iss=%q want 4242", claims.Iss)
	}
	if claims.Exp <= claims.Iat {
		t.Fatalf("exp %d must be after iat %d", claims.Exp, claims.Iat)
	}
}

func TestParsePrivateKeyPKCS8(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if _, err := parsePrivateKey(pemStr); err != nil {
		t.Fatalf("parse PKCS#8: %v", err)
	}
	if _, err := parsePrivateKey("not a key"); err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestParseAppConfigJSONObjectAndArray(t *testing.T) {
	object := `{"apps":[{"appId":1,"privateKey":"x","installations":[{"org":"a","installationId":10}]}]}`
	got, err := parseAppConfigJSON([]byte(object))
	if err != nil || len(got) != 1 || got[0].AppID != 1 || len(got[0].Installations) != 1 {
		t.Fatalf("object form: %#v err=%v", got, err)
	}
	array := `[{"appId":2,"privateKey":"y","org":"b","installationId":20}]`
	got, err = parseAppConfigJSON([]byte(array))
	if err != nil || len(got) != 1 || got[0].AppID != 2 || got[0].Org != "b" {
		t.Fatalf("array form: %#v err=%v", got, err)
	}
}

func TestBuildGithubAuthMultiAppAndSingleAppMultiInstall(t *testing.T) {
	pemA, _ := testKeyPEM(t)
	pemB, _ := testKeyPEM(t)
	apps := []appConfig{
		// One app, multiple installations (enterprise: one org per installation).
		{AppID: 100, PrivateKey: pemA, Installations: []appInstallation{
			{Org: "OrgOne", InstallationID: 11},
			{Org: "OrgTwo", InstallationID: 22},
		}},
		// A second app + org pair.
		{AppID: 200, PrivateKey: pemB, Org: "OrgThree", InstallationID: 33},
	}
	auth, err := buildGithubAuth("pat-fallback", apps, "https://api.github.com")
	if err != nil {
		t.Fatalf("buildGithubAuth: %v", err)
	}
	if len(auth.installations()) != 3 {
		t.Fatalf("expected 3 installations, got %d", len(auth.installations()))
	}
	// Org lookup is case-insensitive and maps to the right app + installation.
	cases := map[string]struct {
		appID  int64
		instID int64
	}{
		"orgone":   {100, 11},
		"orgtwo":   {100, 22},
		"orgthree": {200, 33},
	}
	for org, want := range cases {
		it, ok := auth.byOrg[org]
		if !ok {
			t.Fatalf("missing installation for %q", org)
		}
		if it.appID != want.appID || it.installationID != want.instID {
			t.Fatalf("%s -> app %d inst %d, want app %d inst %d", org, it.appID, it.installationID, want.appID, want.instID)
		}
	}
}

func TestBuildGithubAuthErrors(t *testing.T) {
	pemA, _ := testKeyPEM(t)
	// Duplicate org across installations must be rejected.
	_, err := buildGithubAuth("", []appConfig{
		{AppID: 1, PrivateKey: pemA, Installations: []appInstallation{
			{Org: "dup", InstallationID: 1},
			{Org: "dup", InstallationID: 2},
		}},
	}, "https://api.github.com")
	if err == nil {
		t.Fatal("expected duplicate-org error")
	}
	// Missing installation.
	if _, err := buildGithubAuth("", []appConfig{{AppID: 1, PrivateKey: pemA}}, "x"); err == nil {
		t.Fatal("expected missing-installation error")
	}
	// Missing key.
	if _, err := buildGithubAuth("", []appConfig{{AppID: 1, Org: "a", InstallationID: 1}}, "x"); err == nil {
		t.Fatal("expected missing-key error")
	}
}

func TestTokenForRepoUsesInstallationTokenAndCaches(t *testing.T) {
	pemStr, _ := testKeyPEM(t)
	var tokenCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/55/access_tokens" && r.Method == http.MethodPost {
			atomic.AddInt32(&tokenCalls, 1)
			if !strings.HasPrefix(r.Header.Get("authorization"), "Bearer ") {
				t.Errorf("missing Bearer JWT: %q", r.Header.Get("authorization"))
			}
			w.Header().Set("content-type", "application/json")
			// expires_at far in the future so the token stays cached.
			fmt.Fprint(w, `{"token":"ghs_installationtoken","expires_at":"2999-01-01T00:00:00Z"}`)
			return
		}
		http.Error(w, "unexpected", http.StatusNotFound)
	}))
	defer server.Close()

	auth, err := buildGithubAuth("pat-fallback", []appConfig{
		{AppID: 7, PrivateKey: pemStr, Org: "acme", InstallationID: 55},
	}, server.URL)
	if err != nil {
		t.Fatal(err)
	}

	// Repo in the configured org resolves to the installation token.
	if got := auth.tokenForRepo("acme/widgets"); got != "ghs_installationtoken" {
		t.Fatalf("tokenForRepo(acme/widgets)=%q want installation token", got)
	}
	// Second call is served from cache (no extra token request).
	if got := auth.tokenForOrg("ACME"); got != "ghs_installationtoken" {
		t.Fatalf("case-insensitive org lookup failed: %q", got)
	}
	if n := atomic.LoadInt32(&tokenCalls); n != 1 {
		t.Fatalf("expected token minted once (cached), got %d calls", n)
	}
	// Repo outside any configured org falls back to the PAT.
	if got := auth.tokenForRepo("other/repo"); got != "pat-fallback" {
		t.Fatalf("fallback token=%q want pat-fallback", got)
	}
	// A nil auth is safe and yields no token.
	var nilAuth *githubAuth
	if got := nilAuth.tokenForRepo("x/y"); got != "" {
		t.Fatalf("nil auth token=%q want empty", got)
	}
}

func TestListInstallationReposPaginates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Bearer inst-token" {
			t.Errorf("unexpected auth header %q", r.Header.Get("authorization"))
		}
		page := r.URL.Query().Get("page")
		w.Header().Set("content-type", "application/json")
		if page == "1" {
			repos := make([]string, 0, 100)
			for i := 0; i < 100; i++ {
				repos = append(repos, fmt.Sprintf(`{"full_name":"acme/repo%d","clone_url":"https://x/%d.git","default_branch":"main"}`, i, i))
			}
			fmt.Fprintf(w, `{"repositories":[%s]}`, strings.Join(repos, ","))
			return
		}
		fmt.Fprint(w, `{"repositories":[{"full_name":"acme/last","clone_url":"https://x/last.git","default_branch":"main"}]}`)
	}))
	defer server.Close()

	repos, err := listInstallationRepos(server.URL, "inst-token")
	if err != nil {
		t.Fatalf("listInstallationRepos: %v", err)
	}
	if len(repos) != 101 {
		t.Fatalf("expected 101 repos across pages, got %d", len(repos))
	}
	if repos[100].FullName != "acme/last" {
		t.Fatalf("last repo=%q", repos[100].FullName)
	}
}

func TestOrgOf(t *testing.T) {
	if orgOf("owner/name") != "owner" {
		t.Fatal("orgOf owner/name")
	}
	if orgOf("bare") != "bare" {
		t.Fatal("orgOf bare")
	}
}

func TestLoadAppConfigsFlatEnvMultiInstall(t *testing.T) {
	// Isolate from any ambient config.
	for _, k := range []string{"GITHUB_APP_CONFIG", "GITHUB_APP_CONFIG_FILE", "GITHUB_APP_ORG", "GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY_FILE"} {
		t.Setenv(k, "")
	}
	pemStr, _ := testKeyPEM(t)
	t.Setenv("GITHUB_APP_ID", "321")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", pemStr)
	t.Setenv("GITHUB_APP_INSTALLATIONS", "orgA:11, orgB:22")

	apps, err := loadAppConfigs("")
	if err != nil {
		t.Fatalf("loadAppConfigs: %v", err)
	}
	if len(apps) != 1 || apps[0].AppID != 321 || len(apps[0].Installations) != 2 {
		t.Fatalf("unexpected flat config: %#v", apps)
	}
	if apps[0].Installations[1].Org != "orgB" || apps[0].Installations[1].InstallationID != 22 {
		t.Fatalf("install[1]=%#v", apps[0].Installations[1])
	}
	// End-to-end through buildGithubAuth so the routing table is exercised.
	auth, err := buildGithubAuth("", apps, "https://api.github.com")
	if err != nil {
		t.Fatalf("buildGithubAuth: %v", err)
	}
	if !auth.hasInstallations() || len(auth.installations()) != 2 {
		t.Fatalf("expected 2 installations, got %d", len(auth.installations()))
	}

	// Malformed installation entry is a hard error.
	if _, err := parseInstallationList("missingcolon"); err == nil {
		t.Fatal("expected error for entry without colon")
	}
}

func TestParseArgsWiresGithubApp(t *testing.T) {
	for _, k := range []string{
		"GREPPLE_PORT", "GREPPLE_TIMEOUT", "GREPPLE_BACKENDS", "SHARD_HOSTS",
		"WATCH_REPOS", "WATCH_ORGS", "GITHUB_TOKEN",
		"GITHUB_APP_CONFIG", "GITHUB_APP_CONFIG_FILE", "GITHUB_APP_ORG",
		"GITHUB_APP_INSTALLATION_ID", "GITHUB_APP_PRIVATE_KEY_FILE", "GITHUB_APP_INSTALLATIONS",
	} {
		t.Setenv(k, "")
	}
	pemStr, _ := testKeyPEM(t)
	t.Setenv("GITHUB_TOKEN", "pat-local")
	t.Setenv("GITHUB_APP_ID", "777")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", pemStr)
	t.Setenv("GITHUB_APP_ORG", "acme")
	t.Setenv("GITHUB_APP_INSTALLATION_ID", "4242")

	options, err := parseArgs([]string{"--backend", "http://shard-a:8787"})
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if !options.auth.hasInstallations() {
		t.Fatal("expected auth to carry a GitHub App installation")
	}
	// App-owned org routes to the installation; other orgs fall back to the PAT.
	if got := options.auth.byOrg["acme"]; got == nil || got.installationID != 4242 || got.appID != 777 {
		t.Fatalf("acme installation misconfigured: %#v", got)
	}
	if got := options.auth.tokenForRepo("someone/else"); got != "pat-local" {
		t.Fatalf("fallback token=%q want pat-local", got)
	}
}

// TestBuildGithubAuthReadsPrivateKeyFile verifies the App private key can be
// supplied as a mounted file path (privateKeyFile / GITHUB_APP_PRIVATE_KEY_FILE)
// instead of inline PEM, and that a missing file is a clear error.
func TestBuildGithubAuthReadsPrivateKeyFile(t *testing.T) {
	pemStr, _ := testKeyPEM(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "app.pem")
	if err := os.WriteFile(keyPath, []byte(pemStr), 0o600); err != nil {
		t.Fatal(err)
	}
	auth, err := buildGithubAuth("", []appConfig{{
		AppID:          99,
		PrivateKeyFile: keyPath,
		Installations:  []appInstallation{{Org: "acme", InstallationID: 7}},
	}}, "https://api.github.com")
	if err != nil {
		t.Fatalf("buildGithubAuth from file: %v", err)
	}
	if it, ok := auth.byOrg["acme"]; !ok || it.key == nil {
		t.Fatalf("installation token missing or key not parsed from file (ok=%v)", ok)
	}

	if _, err := buildGithubAuth("", []appConfig{{
		AppID:          99,
		PrivateKeyFile: filepath.Join(dir, "missing.pem"),
		Installations:  []appInstallation{{Org: "acme", InstallationID: 7}},
	}}, "x"); err == nil {
		t.Fatal("expected an error for a missing privateKeyFile")
	}
}
