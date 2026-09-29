package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/config"
	"github.com/greppleai/grepple/internal/wire"
)

func loadTestUserSettings() (config.UserSettings, error) {
	settings, err := config.LoadConfig("", true)
	if err != nil {
		return config.UserSettings{}, err
	}
	return settings.User, nil
}

type cliOptions = Options
type userSettings = config.UserSettings
type anchorSettings = config.Anchors

func testCommandContext() cliruntime.Context {
	contextGuard := func() bool { return os.Getenv("GREPPLE_CONTEXT_GUARD_DIR") != "" }
	return cliruntime.Environment{
		Config: cliruntime.ConfigurationServices{
			ResolveServer: func(value string) string {
				if value != "" {
					return value
				}
				return os.Getenv("GREPPLE_SERVER")
			},
			ContextGuard:     contextGuard,
			LoadUserSettings: loadTestUserSettings,
		},
		RepositoryContext: cliruntime.RepositoryServices{CurrentFunc: cliruntime.CurrentRepository},
	}
}

func parseTestSearchArgs(args []string) (*Options, string, bool, error) {
	return parseSearchArgs(testCommandContext(), args)
}

func runTestSearch(args []string) error { return New(testCommandContext()).Run(args) }

func Run(args []string) error {
	if len(args) > 0 && args[0] == "search" {
		args = args[1:]
	}
	return runTestSearch(args)
}

func runTestRemote(options *cliOptions, server string) ([]wire.FileResult, error) {
	return SearchRemote(testCommandContext(), options, server)
}

func requestTestNavigationResolve(ctx context.Context, request wire.NavigationResolveRequest, server string) (wire.NavigationResolveResponse, error) {
	return ResolveNavigation(testCommandContext(), ctx, request, server)
}

func runGitForTest(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func writeGraphSource(t *testing.T, root, path, content string) {
	t.Helper()
	fullPath := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
