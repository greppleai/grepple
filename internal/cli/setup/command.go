// Package setup installs version-matched Grepple skills for supported agent harnesses.
package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/agentskills"
	"github.com/greppleai/grepple/internal/cliruntime"
)

// Args owns agent selection, installation scope and explicit development overrides.
type Args struct {
	Agent     string `arg:"positional" help:"agent harness: pi, claude, opencode, or codex"`
	AgentFlag string `arg:"--agent" help:"agent harness (alternative to positional agent)"`
	Project   bool   `arg:"--project" help:"install into this project's agent skills directory instead of user skills"`
	DryRun    bool   `arg:"--dry-run" help:"download and validate, then report without filesystem changes"`
	List      bool   `arg:"--list" help:"list active skills and append-only owned names without downloading"`
	SourceDir string `arg:"--source-dir" placeholder:"PATH" help:"explicit development checkout; must match this binary's skill catalog"`
}
type command struct {
	application cliruntime.Context
	version     string
}

// New constructs a setup command with build identity injected by the composition root.
func New(application cliruntime.Context, version string) cliruntime.Command {
	return &command{application: application, version: version}
}
func (command *command) Run(args []string) error {
	values := &Args{}
	parser, err := arg.NewParser(arg.Config{Program: "grepple setup"}, values)
	if err != nil {
		return err
	}
	if err = parser.Parse(args); errors.Is(err, arg.ErrHelp) {
		parser.WriteHelp(command.application.Stdout())
		return nil
	}
	if err != nil {
		return err
	}
	return Execute(command.application, values, command.version)
}

// Execute installs only Grepple-owned skills and leaves harness settings/auth untouched.
func Execute(application cliruntime.Context, values *Args, version string) error {
	if values.List {
		return writeCatalog(application)
	}
	agent, err := selectedAgent(values)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root, err := destination(agent, home, application.Repository().WorkingDirectory(), values.Project, os.Getenv)
	if err != nil {
		return err
	}
	source, ref, err := selectedSource(values.SourceDir, version)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	bundle, err := agentskills.Load(ctx, source)
	if err != nil {
		return err
	}
	report, err := agentskills.Synchronize(ctx, root, ref, bundle, values.DryRun)
	if err != nil {
		return err
	}
	verb := "Installed"
	if values.DryRun {
		verb = "Would install"
	}
	if _, err = fmt.Fprintf(application.Stdout(), "%s %d Grepple skills for %s from %s in %s\n", verb, len(report.Installed), agent, ref, root); err != nil {
		return err
	}
	for _, name := range report.Removed {
		fmt.Fprintf(application.Stdout(), "Retired: %s\n", name)
	}
	for _, name := range report.Preserved {
		fmt.Fprintf(application.Stderr(), "Preserved unrecognized historical skill: %s\n", name)
	}
	if !values.DryRun {
		_, err = fmt.Fprintln(application.Stdout(), "Restart/reload your agent to discover the updated skills. No settings or credentials were changed.")
	}
	return err
}
func selectedAgent(values *Args) (string, error) {
	if values.Agent != "" && values.AgentFlag != "" {
		return "", fmt.Errorf("use a positional agent or --agent, not both")
	}
	agent := values.Agent
	if agent == "" {
		agent = values.AgentFlag
	}
	switch agent {
	case "pi", "claude", "opencode", "codex":
		return agent, nil
	default:
		return "", fmt.Errorf("setup requires pi, claude, opencode, or codex (or --list)")
	}
}
func destination(agent, home, cwd string, project bool, getenv func(string) string) (string, error) {
	if project {
		switch agent {
		case "pi":
			return filepath.Join(cwd, ".pi", "skills"), nil
		case "claude":
			return filepath.Join(cwd, ".claude", "skills"), nil
		case "opencode":
			return filepath.Join(cwd, ".opencode", "skills"), nil
		case "codex":
			return filepath.Join(cwd, ".agents", "skills"), nil
		}
	}
	switch agent {
	case "pi":
		base := getenv("PI_CODING_AGENT_DIR")
		if base == "" {
			base = filepath.Join(home, ".pi", "agent")
		}
		return absoluteSkills(base)
	case "claude":
		return filepath.Join(home, ".claude", "skills"), nil
	case "opencode":
		base := getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		if !filepath.IsAbs(base) {
			return "", fmt.Errorf("XDG_CONFIG_HOME must be absolute")
		}
		return filepath.Join(base, "opencode", "skills"), nil
	case "codex":
		return filepath.Join(home, ".agents", "skills"), nil
	default:
		return "", fmt.Errorf("unsupported agent %q", agent)
	}
}
func absoluteSkills(base string) (string, error) {
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("PI_CODING_AGENT_DIR must be absolute")
	}
	return filepath.Join(base, "skills"), nil
}
func selectedSource(local, version string) (agentskills.Source, string, error) {
	if local != "" {
		source, err := agentskills.NewLocalSource(local)
		return source, "local-development", err
	}
	tag, err := agentskills.ReleaseTag(version)
	if err != nil {
		return nil, "", err
	}
	source, err := agentskills.NewRemoteSource(tag)
	return source, tag, err
}
func writeCatalog(application cliruntime.Context) error {
	active, err := agentskills.ActiveNames()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(application.Stdout(), "Active skills:\n%s\nOwned names (append-only, including retired):\n%s\n", strings.Join(active, "\n"), strings.Join(agentskills.OwnedNames(), "\n"))
	return err
}
