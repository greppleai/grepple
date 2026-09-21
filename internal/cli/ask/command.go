// Package ask implements the ask CLI command and composes providers with the reusable agent runtime.
package ask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/aiprovider"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
)

const defaultTimeout = 10 * time.Minute

type arguments struct {
	Provider string `arg:"--provider" placeholder:"NAME" help:"AI provider: anthropic, anthropic-subscription, bedrock, codex, copilot, or openai"`
	Model    string `arg:"--model" placeholder:"[PROVIDER/]MODEL" help:"research model, optionally prefixed with its provider"`
	cliruntime.CommonArgs
	Timeout  int      `arg:"--timeout-seconds" placeholder:"N" help:"overall deadline in seconds"`
	Question []string `arg:"positional" placeholder:"QUESTION"`
}

// SessionRequest contains the command-resolved values required by the research agent.
type SessionRequest struct {
	Question string
	Root     string
	Server   string
	Model    string
	Timeout  int
}

// SessionRunner executes the repository research agent assembled by the parent CLI.
type SessionRunner func(context.Context, *agent.Log, aiprovider.Provider, SessionRequest) (string, error)

// Dependencies are process-boundary services supplied by the parent CLI.
type Dependencies struct {
	Stdout     io.Writer
	Stderr     io.Writer
	RunSession SessionRunner
}

// Run parses and executes the ask command.
func Run(args []string, dependencies Dependencies) error {
	stdout, stderr := dependencies.Stdout, dependencies.Stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	values, help, err := parseArguments(args, stdout)
	if err != nil || help {
		return err
	}
	if dependencies.RunSession == nil {
		return fmt.Errorf("ask research session is not configured")
	}
	invocation, err := prepareInvocation(values)
	if err != nil {
		return err
	}
	log, err := agent.NewLogWithOptions(agent.LogOptions{Enabled: invocation.preferences.LogsEnabled, Retention: invocation.preferences.LogRetention})
	if err != nil {
		return err
	}
	if log.Path() != "" {
		fmt.Fprintln(stderr, "Ask log:", log.Path())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(values.Timeout)*time.Second)
	defer cancel()
	answer, runErr := dependencies.RunSession(ctx, log, invocation.provider, SessionRequest{
		Question: invocation.question, Root: invocation.root, Server: values.Server, Model: invocation.model, Timeout: values.Timeout,
	})
	if err := errors.Join(runErr, log.Close()); err != nil {
		return err
	}
	return cliruntime.NewOutput(stdout).WriteString(answer + "\n")
}

type preparedInvocation struct {
	provider    aiprovider.Provider
	preferences Preferences
	question    string
	model       string
	root        string
}

func parseArguments(args []string, output io.Writer) (arguments, bool, error) {
	values := arguments{Timeout: int(defaultTimeout.Seconds())}
	parser, err := arg.NewParser(arg.Config{Program: "grepple ask"}, &values)
	if err != nil {
		return values, false, err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(output)
			return values, true, nil
		}
		return values, false, err
	}
	return values, false, nil
}

func prepareInvocation(values arguments) (preparedInvocation, error) {
	question, err := validateArguments(values)
	if err != nil {
		return preparedInvocation{}, err
	}
	store, err := aiprovider.NewStore()
	if err != nil {
		return preparedInvocation{}, err
	}
	preferences, err := LoadPreferences()
	if err != nil {
		return preparedInvocation{}, err
	}
	providerName, model, err := ResolveSelection(values.Provider, values.Model, preferences.Model)
	if err != nil {
		return preparedInvocation{}, err
	}
	provider, err := aiprovider.NewRegistry(store, &http.Client{Timeout: 5 * time.Minute}).Provider(providerName)
	if err != nil {
		return preparedInvocation{}, err
	}
	if model == "" {
		model = provider.DefaultModel()
	}
	root, err := os.Getwd()
	if err != nil {
		return preparedInvocation{}, err
	}
	return preparedInvocation{provider: provider, preferences: preferences, question: question, model: model, root: root}, nil
}

func validateArguments(values arguments) (string, error) {
	question := strings.TrimSpace(strings.Join(values.Question, " "))
	if question == "" {
		return "", fmt.Errorf("ask requires a question")
	}
	if values.Timeout < 1 || values.Timeout > 3600 {
		return "", fmt.Errorf("--timeout-seconds must be between 1 and 3600")
	}
	return question, nil
}

// ResolveSelection applies explicit and configured provider/model precedence.
func ResolveSelection(explicitProvider, explicitModel, configuredModel string) (string, string, error) {
	provider := defaultProvider(explicitProvider)
	if explicit := strings.TrimSpace(explicitModel); explicit != "" {
		return resolveExplicitModel(provider, strings.TrimSpace(explicitProvider) != "", explicit)
	}
	return resolveConfiguredModel(provider, strings.TrimSpace(explicitProvider) != "", configuredModel)
}

func resolveExplicitModel(provider string, providerWasExplicit bool, selector string) (string, string, error) {
	selectedProvider, model, prefixed, err := parseModelSelector(selector)
	if err != nil {
		return "", "", err
	}
	if !prefixed {
		return provider, model, nil
	}
	if providerWasExplicit && provider != selectedProvider {
		return "", "", fmt.Errorf("--provider %q conflicts with --model provider %q", provider, selectedProvider)
	}
	return selectedProvider, model, nil
}

func resolveConfiguredModel(provider string, providerWasExplicit bool, configuredModel string) (string, string, error) {
	configured := strings.TrimSpace(configuredModel)
	if configured == "" {
		return provider, "", nil
	}
	selectedProvider, model, prefixed, err := parseModelSelector(configured)
	if err != nil {
		return "", "", fmt.Errorf("configured ask.model: %w", err)
	}
	if prefixed {
		if !providerWasExplicit || provider == selectedProvider {
			return selectedProvider, model, nil
		}
		return provider, "", nil
	}
	if provider == "codex" {
		return provider, model, nil
	}
	return provider, "", nil
}

func parseModelSelector(value string) (provider, model string, prefixed bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false, fmt.Errorf("model cannot be empty")
	}
	provider, model, found := strings.Cut(value, "/")
	if !found {
		return "", value, false, nil
	}
	provider, model = strings.TrimSpace(strings.ToLower(provider)), strings.TrimSpace(model)
	if provider == "" || model == "" {
		return "", "", false, fmt.Errorf("model must use <provider>/<model>")
	}
	return provider, model, true, nil
}

func defaultProvider(value string) string {
	if value = strings.TrimSpace(strings.ToLower(value)); value != "" {
		return value
	}
	return "codex"
}
