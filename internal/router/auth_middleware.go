package router

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

// authCacheTTL bounds how long a validated token is trusted before grepple
// re-checks it against GitHub. This keeps the happy path off GitHub on every
// request while ensuring revoked tokens / removed org members lose access
// within the window.
const authCacheTTL = 5 * time.Minute

type authEntry struct {
	login  string
	expiry time.Time
}

// tokenAuthCache caches positively-validated tokens (keyed by a hash so raw
// tokens are never held as map keys) for authCacheTTL.
type tokenAuthCache struct {
	mu      sync.Mutex
	entries map[string]authEntry
}

func newTokenAuthCache() *tokenAuthCache {
	return &tokenAuthCache{entries: map[string]authEntry{}}
}

func (c *tokenAuthCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if time.Now().After(e.expiry) {
		delete(c.entries, key)
		return "", false
	}
	return e.login, true
}

func (c *tokenAuthCache) put(key, login string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = authEntry{login: login, expiry: time.Now().Add(authCacheTTL)}
}

func bearerToken(c fiber.Ctx) string {
	h := c.Get("Authorization")
	if len(h) >= 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// githubUserLogin validates a token by fetching the authenticated user and
// returns their login. An empty/invalid token yields an error.
func githubUserLogin(client *http.Client, apiBase, token string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(apiBase, "/")+"/user", nil)
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("invalid token")
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("user lookup failed (%d)", resp.StatusCode)
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return "", err
	}
	if u.Login == "" {
		return "", fmt.Errorf("user lookup returned no login")
	}
	return u.Login, nil
}

// githubOrgMember reports whether the token's user is a member of org, using
// the user's own token (requires the read:org scope). GitHub returns 200 with
// state active|pending for members and 404 for non-members.
func githubOrgMember(client *http.Client, apiBase, token, org string) (bool, error) {
	req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(apiBase, "/")+"/user/memberships/orgs/"+org, nil)
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("membership lookup failed (%d)", resp.StatusCode)
	}
	var m struct {
		State string `json:"state"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&m)
	return m.State == "active", nil
}

// checkOrgAuth guards one /public request: a pass-through when auth is off,
// and a cached or freshly validated GitHub token plus org membership
// otherwise. Positively validated tokens are cached.
func checkOrgAuth(s *routerState, c fiber.Ctx) error {
	if !s.o.authRequired {
		return c.Next()
	}
	token := bearerToken(c)
	if token == "" {
		return errorJSON(c, fiber.StatusUnauthorized, "authentication required: run `grepple login`")
	}
	key := hashToken(token)
	if login, ok := s.authCache.get(key); ok {
		c.Locals("user", login)
		return c.Next()
	}
	client := &http.Client{Timeout: min(s.o.timeout, 5*time.Second)}
	login, err := githubUserLogin(client, s.o.githubAPIBase, token)
	if err != nil {
		return errorJSON(c, fiber.StatusUnauthorized, "invalid or expired token: run `grepple login`")
	}
	if !requireOrgMembership(s, c, client, token) {
		return nil // denial response already written by requireOrgMembership
	}
	s.authCache.put(key, login)
	c.Locals("user", login)
	return c.Next()
}

// requireOrgMembership reports whether the request may proceed: true when no
// org allowlist is configured or the token's user belongs to at least one
// allowed org. On denial it writes the 403/502 response itself and returns
// false (errorJSON answers nil after writing, like any fiber handler).
func requireOrgMembership(s *routerState, c fiber.Ctx, client *http.Client, token string) bool {
	orgs := s.o.allowedAuthOrgs()
	if len(orgs) == 0 {
		return true
	}
	for org := range orgs {
		ok, err := githubOrgMember(client, s.o.githubAPIBase, token, org)
		if err != nil {
			_ = errorJSON(c, fiber.StatusBadGateway, "organization membership check failed")
			return false
		}
		if ok {
			return true
		}
	}
	_ = errorJSON(c, fiber.StatusForbidden, "access denied: not a member of an authorized organization")
	return false
}
