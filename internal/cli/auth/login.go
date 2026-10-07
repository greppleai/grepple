package auth

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/config"
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

// executeLogin performs server-owned device authorization and stores a scoped local session.
func executeLogin(application cliruntime.Context, args []string) error {
	values := &LoginArgs{URL: application.Configuration().ServerDefault("")}
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	fs.SetOutput(application.Stderr())
	fs.StringVar(&values.URL, "url", values.URL, "grepple authentication server URL")
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
	serverURL, err := loginServerURL(serverURL)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := application.APIClient()
	cfg, err := client.LoginConfig(ctx, serverURL)
	if err != nil {
		return err
	}
	if cfg.Provider != "local" || strings.TrimSpace(cfg.ClientID) == "" {
		return fmt.Errorf("server %s does not advertise local device authentication", serverURL)
	}

	dc, err := client.RequestDeviceCode(ctx, serverURL, cfg.ClientID, "")
	if err != nil {
		return err
	}
	fmt.Fprintf(application.Stderr(), "\nTo authorize grepple, open:\n  %s\nand enter the code:\n  %s\n\n", dc.VerificationURI, dc.UserCode)
	if !values.NoBrowser {
		_ = openBrowser(dc.VerificationURI)
	}
	fmt.Fprintln(application.Stderr(), "Waiting for authorization…")

	tok, err := client.PollDeviceToken(ctx, serverURL, cfg.ClientID, dc, func(delay time.Duration) {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	})
	if err != nil {
		return err
	}
	login := tok.Login
	// Backend login remains available when unrelated user preferences are invalid.
	settings, _ := config.LoadConfig("", true)
	if err := settings.StoreBackendLogin(tok.AccessToken, tok.RefreshToken, tok.ExpiresIn, tok.RefreshTokenExpiresIn, login, serverURL); err != nil {
		return fmt.Errorf("store token: %w", err)
	}
	path, _ := settings.BackendAuthPath()
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
	// Logout must not be blocked by malformed, unrelated settings.
	settings, _ := config.LoadConfig("", true)
	credentials := settings.BackendCredentials()
	if credentials.Token != "" && credentials.AuthServer != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := application.APIClient().RevokeLogin(ctx, credentials.AuthServer); err != nil {
			fmt.Fprintln(application.Stderr(), "Warning: server-side session revocation failed; removing local credentials.")
		}
	}
	if err := settings.ClearBackendLogin(); err != nil {
		return err
	}
	fmt.Fprintln(application.Stderr(), "Logged out; token removed from config.")
	return nil
}

func loginServerURL(server string) (string, error) {
	target, err := url.Parse(server)
	if err != nil || target.Host == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return "", fmt.Errorf("invalid authentication server URL")
	}
	if target.Scheme != "https" && !(target.Scheme == "http" && (target.Hostname() == "localhost" || target.Hostname() == "127.0.0.1" || target.Hostname() == "::1")) {
		return "", fmt.Errorf("login requires HTTPS except on loopback")
	}
	return strings.TrimRight(server, "/"), nil
}
