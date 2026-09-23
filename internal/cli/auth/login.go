package auth

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/greppleai/grepple/internal/authstate"
	"github.com/greppleai/grepple/internal/cliruntime"
)

func envDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func firstNonEmpty(first, second string) string {
	if strings.TrimSpace(first) != "" {
		return first
	}
	return second
}

// githubWebHost / githubAPIHost are overridable (GHE, or tests pointing at a
// mock server).
func githubWebHost() string { return envDefault("GREPPLE_GITHUB_HOST", "https://github.com") }
func githubAPIHost() string { return envDefault("GREPPLE_GITHUB_API", "https://api.github.com") }

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

// runLogin performs the GitHub device-flow login and stores the token in
// ~/.grepple/config.json. The GitHub client ID is fetched from the grepple server
// (--url), so it is configured only server-side.
func executeLogin(application cliruntime.Context, args []string) error {
	values := &LoginArgs{URL: application.Configuration().ServerDefault("")}
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(application.Stderr())
	fs.StringVar(&values.URL, "url", values.URL, "grepple server URL to fetch the login client ID from")
	fs.StringVar(&values.Scope, "scope", "", "OAuth scopes, space-separated (overrides the server's advertised scopes)")
	fs.BoolVar(&values.NoBrowser, "no-browser", false, "do not attempt to open a browser")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return executeLoginArgs(application, values)
}

func executeLoginArgs(application cliruntime.Context, values *LoginArgs) error {
	serverURL := values.URL
	if serverURL == "" {
		serverURL = application.Configuration().ServerDefault("")
	}
	client := application.APIClient()
	cfg, err := client.LoginConfig(context.Background(), serverURL)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return fmt.Errorf("server %s did not advertise a GitHub client ID (set GITHUB_CLIENT_ID on the router)", serverURL)
	}
	scopes := firstNonEmpty(strings.TrimSpace(values.Scope), firstNonEmpty(strings.TrimSpace(cfg.Scopes), "read:user"))

	dc, err := client.RequestDeviceCode(context.Background(), githubWebHost(), cfg.ClientID, scopes)
	if err != nil {
		return err
	}
	fmt.Fprintf(application.Stderr(), "\nTo authorize grepple, open:\n  %s\nand enter the code:\n  %s\n\n", dc.VerificationURI, dc.UserCode)
	if !values.NoBrowser {
		_ = openBrowser(dc.VerificationURI)
	}
	fmt.Fprintln(application.Stderr(), "Waiting for authorization…")

	tok, err := client.PollDeviceToken(context.Background(), githubWebHost(), cfg.ClientID, dc, time.Sleep)
	if err != nil {
		return err
	}
	login := client.GitHubLogin(context.Background(), githubAPIHost(), tok.AccessToken)
	if err := authstate.StoreLogin(tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, tok.RefreshTokenExpiresIn, login); err != nil {
		return fmt.Errorf("store token: %w", err)
	}
	path, _ := authstate.Path()
	if login != "" {
		fmt.Fprintf(application.Stderr(), "Logged in as %s. Token saved to %s\n", login, path)
	} else {
		fmt.Fprintf(application.Stderr(), "Logged in. Token saved to %s\n", path)
	}
	if tok.RefreshToken != "" {
		fmt.Fprintln(application.Stderr(), "This token auto-renews — you won't need to log in again until the refresh token expires.")
	}
	return nil
}

// runLogout removes the stored token.
func executeLogout(application cliruntime.Context, _ []string) error {
	if err := authstate.Clear(); err != nil {
		return err
	}
	fmt.Fprintln(application.Stderr(), "Logged out; token removed from config.")
	return nil
}
