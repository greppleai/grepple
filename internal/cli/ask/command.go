// Package ask implements the ask CLI command and composes providers with the reusable agent runtime.
package ask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/aiprovider"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/config"
)

const defaultTimeout = 10 * time.Minute

type Args struct {
	Provider string `arg:"--provider" placeholder:"NAME" help:"AI provider: anthropic, anthropic-subscription, bedrock, codex, copilot, or openai"`
	Model    string `arg:"--model" placeholder:"[PROVIDER/]MODEL" help:"research model, optionally prefixed with its provider"`
	cliruntime.CommonArgs
	Timeout  int      `arg:"--timeout-seconds" default:"600" placeholder:"N" help:"overall deadline in seconds"`
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

// sessionRunner executes the repository research agent owned by this command.
type sessionRunner func(context.Context, *agent.Log, aiprovider.Provider, SessionRequest) (string, error)

type command struct {
	application cliruntime.Context
	runSession  sessionRunner
}

// New constructs the ask command from the common command context.
func New(application cliruntime.Context) cliruntime.Command {
	return &command{application: application, runSession: func(ctx context.Context, log *agent.Log, provider aiprovider.Provider, request SessionRequest) (string, error) {
		return runAskSession(application, ctx, log, provider, request)
	}}
}

func newWithSessionRunner(application cliruntime.Context, runner sessionRunner) cliruntime.Command {
	return &command{application: application, runSession: runner}
}

// Run parses and executes the ask command.
func (command *command) Run(args []string) error {
	if command.application == nil {
		return fmt.Errorf("ask command context is unavailable")
	}
	stdout := command.application.Stdout()
	values, help, err := parseArguments(args, stdout)
	if err != nil || help {
		return err
	}
	return command.execute(&values)
}

// DefaultArgs returns ask arguments with the default overall deadline.
func DefaultArgs() Args { return Args{Timeout: int(defaultTimeout.Seconds())} }

// Execute runs research from application-parsed arguments.
func Execute(application cliruntime.Context, values *Args) error {
	return New(application).(*command).execute(values)
}

func (command *command) execute(values *Args) error {
	if command.application == nil {
		return fmt.Errorf("ask command context is unavailable")
	}
	stdout, stderr := command.application.Stdout(), command.application.Stderr()
	if command.runSession == nil {
		return fmt.Errorf("ask research session is not configured")
	}
	invocation, err := prepareInvocation(command.application, *values)
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
	answer, runErr := command.runSession(ctx, log, invocation.provider, SessionRequest{
		Question: invocation.question, Root: invocation.root, Server: values.Server, Model: invocation.model, Timeout: values.Timeout,
	})
	if err := errors.Join(runErr, log.Close()); err != nil {
		return err
	}
	return cliruntime.NewOutput(stdout).WriteString(answer + "\n")
}

type preparedInvocation struct {
	provider    aiprovider.Provider
	preferences config.AskPreferences
	question    string
	model       string
	root        string
}

func parseArguments(args []string, output io.Writer) (Args, bool, error) {
	values := DefaultArgs()
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

func prepareInvocation(application cliruntime.Context, values Args) (preparedInvocation, error) {
	question, err := validateArguments(values)
	if err != nil {
		return preparedInvocation{}, err
	}
	store, err := aiprovider.NewStore()
	if err != nil {
		return preparedInvocation{}, err
	}
	preferences, err := application.Configuration().AskPreferences()
	if err != nil {
		return preparedInvocation{}, err
	}
	providerName, model, err := aiprovider.ResolveSelection(values.Provider, values.Model, preferences.Model)
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
	root := application.Repository().WorkingDirectory()
	if strings.TrimSpace(root) == "" {
		return preparedInvocation{}, fmt.Errorf("working directory is unavailable")
	}
	return preparedInvocation{provider: provider, preferences: preferences, question: question, model: model, root: root}, nil
}

func validateArguments(values Args) (string, error) {
	question := strings.TrimSpace(strings.Join(values.Question, " "))
	if question == "" {
		return "", fmt.Errorf("ask requires a question")
	}
	if values.Timeout < 1 || values.Timeout > 3600 {
		return "", fmt.Errorf("--timeout-seconds must be between 1 and 3600")
	}
	return question, nil
}
