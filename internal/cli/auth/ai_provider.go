package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/aiprovider"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"golang.org/x/term"
)

var errAIProviderHelp = errors.New("AI provider help displayed")

type aiProviderLoginArgs struct {
	Provider  string `arg:"positional" placeholder:"PROVIDER" help:"anthropic, anthropic-subscription, bedrock, codex, copilot, or openai (default codex)"`
	NoBrowser bool   `arg:"--no-browser" help:"do not open a browser during device login"`
}

type aiProviderNameArgs struct {
	Provider string `arg:"positional" placeholder:"PROVIDER" help:"registered provider name (default codex)"`
}

func runAIProvider(application cliruntime.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("ai-provider requires login, logout, or list")
	}
	store, err := aiprovider.NewStore()
	if err != nil {
		return err
	}
	registry := aiprovider.NewRegistry(store, &http.Client{Timeout: 30 * time.Second})
	switch args[0] {
	case "login":
		return runAIProviderLogin(application, registry, store, args[1:])
	case "logout":
		return runAIProviderLogout(application, registry, args[1:])
	case "list":
		return runAIProviderList(application, registry, args[1:])
	default:
		return fmt.Errorf("unknown ai-provider command %q", args[0])
	}
}

func runAIProviderLogin(application cliruntime.Context, registry *aiprovider.Registry, store *aiprovider.Store, args []string) error {
	values := ProviderLoginArgs{}
	if err := parseAIProviderArgs(application.Stdout(), "grepple ai-provider login", args, &values); err != nil {
		if errors.Is(err, errAIProviderHelp) {
			return nil
		}
		return err
	}
	return executeAIProviderLogin(application, registry, store, &values)
}

func executeAIProviderLogin(application cliruntime.Context, registry *aiprovider.Registry, store *aiprovider.Store, values *ProviderLoginArgs) error {
	provider, err := registry.Provider(defaultAIProvider(values.Provider))
	if err != nil {
		return err
	}
	// Device instructions must bypass the bounded stdout collector so they are
	// visible while Login waits for browser authorization.
	loginOptions := aiprovider.LoginOptions{NoBrowser: values.NoBrowser, Output: application.Stderr(), ReadSecret: func(ctx context.Context, prompt string) (string, error) {
		return readAIProviderSecret(application, ctx, prompt)
	}}
	if err := provider.Login(context.Background(), loginOptions); err != nil {
		return err
	}
	return cliruntime.NewOutput(application.Stdout()).WriteString(fmt.Sprintf("Authentication configured for %s; provider state: %s\n", provider.Name(), store.Path()))
}

func runAIProviderLogout(application cliruntime.Context, registry *aiprovider.Registry, args []string) error {
	values := ProviderNameArgs{}
	if err := parseAIProviderArgs(application.Stdout(), "grepple ai-provider logout", args, &values); err != nil {
		if errors.Is(err, errAIProviderHelp) {
			return nil
		}
		return err
	}
	return executeAIProviderLogout(application, registry, &values)
}

func executeAIProviderLogout(application cliruntime.Context, registry *aiprovider.Registry, values *ProviderNameArgs) error {
	provider, err := registry.Provider(defaultAIProvider(values.Provider))
	if err != nil {
		return err
	}
	if err := provider.Logout(); err != nil {
		return err
	}
	return cliruntime.NewOutput(application.Stdout()).WriteString("Logged out of " + provider.Name() + ".\n")
}

func runAIProviderList(application cliruntime.Context, registry *aiprovider.Registry, args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		return cliruntime.NewOutput(application.Stdout()).WriteString("Usage: grepple ai-provider list\n")
	}
	if len(args) != 0 {
		return fmt.Errorf("ai-provider list accepts no arguments")
	}
	return executeAIProviderList(application, registry)
}

func executeAIProviderList(application cliruntime.Context, registry *aiprovider.Registry) error {
	for _, name := range registry.Names() {
		provider, _ := registry.Provider(name)
		loggedIn, err := provider.LoggedIn()
		if err != nil {
			return err
		}
		status := "logged-out"
		if loggedIn {
			status = "logged-in"
		}
		if err := cliruntime.NewOutput(application.Stdout()).WriteString(name + "\t" + status + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func parseAIProviderArgs(output io.Writer, program string, args []string, target any) error {
	parser, err := arg.NewParser(arg.Config{Program: program}, target)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(output)
			return errAIProviderHelp
		}
		return err
	}
	return nil
}

func readAIProviderSecret(application cliruntime.Context, ctx context.Context, prompt string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	fmt.Fprint(application.Stderr(), prompt+": ")
	if input, ok := application.Stdin().(*os.File); ok && term.IsTerminal(int(input.Fd())) {
		secret, err := term.ReadPassword(int(input.Fd()))
		fmt.Fprintln(application.Stderr())
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(secret)), nil
	}
	line, err := bufio.NewReader(application.Stdin()).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func defaultAIProvider(value string) string {
	if value = strings.TrimSpace(strings.ToLower(value)); value != "" {
		return value
	}
	return "codex"
}
