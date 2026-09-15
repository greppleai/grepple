package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/aiprovider"
)

var errAIProviderHelp = errors.New("AI provider help displayed")

type aiProviderLoginArgs struct {
	Provider  string `arg:"positional" placeholder:"PROVIDER" help:"provider name (default codex)"`
	NoBrowser bool   `arg:"--no-browser" help:"print the device URL without opening a browser"`
}

type aiProviderNameArgs struct {
	Provider string `arg:"positional" placeholder:"PROVIDER" help:"provider name (default codex)"`
}

func runAIProvider(args []string) error {
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
		return runAIProviderLogin(registry, store, args[1:])
	case "logout":
		return runAIProviderLogout(registry, args[1:])
	case "list":
		return runAIProviderList(registry, args[1:])
	default:
		return fmt.Errorf("unknown ai-provider command %q", args[0])
	}
}

func runAIProviderLogin(registry *aiprovider.Registry, store *aiprovider.Store, args []string) error {
	values := aiProviderLoginArgs{}
	if err := parseAIProviderArgs("grepple ai-provider login", args, &values); err != nil {
		if errors.Is(err, errAIProviderHelp) {
			return nil
		}
		return err
	}
	provider, err := registry.Provider(defaultAIProvider(values.Provider))
	if err != nil {
		return err
	}
	// Device instructions must bypass the bounded stdout collector so they are
	// visible while Login waits for browser authorization.
	if err := provider.Login(context.Background(), aiprovider.LoginOptions{NoBrowser: values.NoBrowser, Output: os.Stderr}); err != nil {
		return err
	}
	return stdoutWriter().writeString(fmt.Sprintf("Logged in to %s; credentials stored in %s\n", provider.Name(), store.Path()))
}

func runAIProviderLogout(registry *aiprovider.Registry, args []string) error {
	values := aiProviderNameArgs{}
	if err := parseAIProviderArgs("grepple ai-provider logout", args, &values); err != nil {
		if errors.Is(err, errAIProviderHelp) {
			return nil
		}
		return err
	}
	provider, err := registry.Provider(defaultAIProvider(values.Provider))
	if err != nil {
		return err
	}
	if err := provider.Logout(); err != nil {
		return err
	}
	return stdoutWriter().writeString("Logged out of " + provider.Name() + ".\n")
}

func runAIProviderList(registry *aiprovider.Registry, args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		return stdoutWriter().writeString("Usage: grepple ai-provider list\n")
	}
	if len(args) != 0 {
		return fmt.Errorf("ai-provider list accepts no arguments")
	}
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
		if err := stdoutWriter().writeString(name + "\t" + status + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func parseAIProviderArgs(program string, args []string, target any) error {
	parser, err := arg.NewParser(arg.Config{Program: program}, target)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return errAIProviderHelp
		}
		return err
	}
	return nil
}

func defaultAIProvider(value string) string {
	if value = strings.TrimSpace(strings.ToLower(value)); value != "" {
		return value
	}
	return "codex"
}
