package tree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/apiclient"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/wire"
)

// selectorFlags consumes one argument per flag so the following PATH stays positional.
type selectorFlags []string

func (values *selectorFlags) UnmarshalText(value []byte) error {
	*values = append(*values, string(value))
	return nil
}

// Request contains parsed tree command options.
type Request struct {
	cliruntime.CommonArgs
	Depth           int           `arg:"--depth" default:"1" placeholder:"N" help:"levels to descend"`
	JSON            bool          `arg:"--json" help:"print raw JSON"`
	Purposes        selectorFlags `arg:"--purpose" placeholder:"LABEL" help:"match local file purpose; repeatable"`
	DomainAreas     selectorFlags `arg:"--domain-area" placeholder:"LABEL" help:"match local file domain area; repeatable"`
	FunctionalAreas selectorFlags `arg:"--functional-area" placeholder:"LABEL" help:"match local file functional area; repeatable"`
	Keywords        selectorFlags `arg:"--keyword" placeholder:"LABEL" help:"match local file keyword; repeatable"`
	Repository      string        `arg:"--repo" placeholder:"OWNER/REPOSITORY[@REF]" help:"show one exact indexed repository instead of the local checkout"`
	Repo            string        `arg:"-"`
	Path            string        `arg:"-"`
	Targets         []string      `arg:"positional" placeholder:"PATH" help:"local path (default .); legacy remote syntax also accepts OWNER/REPOSITORY [PATH]"`
}

// Description returns the command description used by the argument parser.
func (Request) Description() string {
	return "Show the local source tree by default; --repo selects an exact indexed repository."
}

// command owns tree command behavior and its private local implementation.
type command struct {
	context cliruntime.Context
	local   localTree
}

// Run executes the tree command.
func (command *command) Run(args []string) error {
	application := command.context
	values := DefaultArgs()
	parser, err := arg.NewParser(arg.Config{Program: "grepple tree"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(application.Stdout())
			return nil
		}
		return err
	}
	return command.execute(&values)
}

// DefaultArgs returns tree arguments with application defaults.
func DefaultArgs() Request { return Request{Depth: 1} }

// Execute renders a tree from application-parsed arguments.
func Execute(application cliruntime.Context, values *Request) error {
	return (&command{context: application, local: newLocal(application)}).execute(values)
}

func (command *command) execute(values *Request) error {
	application := command.context
	if application == nil {
		return fmt.Errorf("tree command context is unavailable")
	}
	if values.Depth < 1 {
		return fmt.Errorf("--depth must be a positive integer")
	}
	remote, err := normalizeRequest(values)
	if err != nil {
		return err
	}
	selections := directorymeta.SelectorsV1{Purposes: []string(values.Purposes), DomainAreas: []string(values.DomainAreas), FunctionalAreas: []string(values.FunctionalAreas), Keywords: []string(values.Keywords)}
	for _, category := range []struct {
		name   string
		values []string
	}{
		{"--purpose", selections.Purposes}, {"--domain-area", selections.DomainAreas}, {"--functional-area", selections.FunctionalAreas}, {"--keyword", selections.Keywords},
	} {
		if remote && len(category.values) > 0 {
			return fmt.Errorf("%s is only supported for local trees; remote indexes have no verified file annotations", category.name)
		}
	}
	selector, err := directorymeta.CompileSelectorsV1(selections)
	if err != nil {
		return err
	}
	var data wire.TreeResponse
	if remote {
		configuration := application.Configuration()
		server := values.Server
		if configuration != nil {
			server = configuration.ServerDefault(server)
		}
		data, err = application.APIClient().Tree(application.Invocation(), server, apiclient.TreeRequest{Repo: values.Repo, Path: values.Path, Depth: values.Depth})
	} else {
		data, err = command.local(values.Path, values.Depth, selector)
	}
	if err != nil {
		return err
	}
	if err := rendercommand.Tree(data, application.Stdout(), values.JSON); err != nil {
		return err
	}
	if len(data.Entries) == 0 {
		if selector.Requested() && !remote {
			fmt.Fprintln(application.Stderr(), "No selected files match current metadata; run grepple overview or grepple verify to inspect missing, invalid or stale annotations.")
		}
		if !values.JSON || selector.Requested() && !remote {
			application.RequestExit(1)
		}
	}
	return nil
}

func normalizeRequest(values *Request) (bool, error) {
	if values == nil {
		return false, fmt.Errorf("tree request is required")
	}
	if values.Repository != "" {
		if len(values.Targets) > 1 {
			return false, fmt.Errorf("tree --repo accepts at most one PATH")
		}
		values.Repo = values.Repository
		if len(values.Targets) == 1 {
			values.Path = values.Targets[0]
		}
		return true, nil
	}
	if len(values.Targets) > 2 {
		return false, fmt.Errorf("tree accepts one local PATH or legacy OWNER/REPOSITORY [PATH]")
	}
	if len(values.Targets) == 2 {
		values.Repo, values.Path = values.Targets[0], values.Targets[1]
		return true, nil
	}
	if len(values.Targets) == 0 {
		values.Path = "."
		return false, nil
	}
	candidate := values.Targets[0]
	if legacyRepositoryTarget(candidate) {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			values.Repo = candidate
			return true, nil
		}
	}
	values.Path = candidate
	return false, nil
}

func legacyRepositoryTarget(value string) bool {
	if strings.HasPrefix(value, ".") || filepath.IsAbs(value) {
		return false
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(value)), "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && parts[0] != ".." && parts[1] != ".."
}
