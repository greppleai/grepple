package tree

import (
	"context"
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
	sourcedomain "github.com/greppleai/grepple/internal/sources"
	"github.com/greppleai/grepple/internal/wire"
)

// areaFlags consumes one value per --area so a following PATH stays positional.
type areaFlags []string

func (areas *areaFlags) UnmarshalText(value []byte) error {
	*areas = append(*areas, string(value))
	return nil
}

// Request contains parsed tree command options.
type Request struct {
	cliruntime.CommonArgs
	Depth      int       `arg:"--depth" default:"1" placeholder:"N" help:"levels to descend"`
	JSON       bool      `arg:"--json" help:"print raw JSON"`
	Kind       string    `arg:"--kind" placeholder:"KIND" help:"show only locally classified files of this kind (production, test, fixture, generated, vendor, unknown)"`
	Areas      areaFlags `arg:"--area" placeholder:"AREA" help:"show files tagged with any requested area and their parent directories; repeatable, local only"`
	Repository string    `arg:"--repo" placeholder:"OWNER/REPOSITORY[@REF]" help:"show one exact indexed repository instead of the local checkout"`
	Repo       string    `arg:"-"`
	Path       string    `arg:"-"`
	Targets    []string  `arg:"positional" placeholder:"PATH" help:"local path (default .); legacy remote syntax also accepts OWNER/REPOSITORY [PATH]"`
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
	var kind sourcedomain.Kind
	if values.Kind != "" {
		var valid bool
		kind, valid = sourcedomain.ParseKind(values.Kind)
		if !valid {
			return fmt.Errorf("invalid --kind %q: expected production, test, fixture, generated, vendor, or unknown", values.Kind)
		}
		if remote {
			return fmt.Errorf("--kind is only supported for local trees; indexed tree entries have no source-kind metadata")
		}
	}
	for _, area := range values.Areas {
		if !directorymeta.ValidArea(area) {
			return fmt.Errorf("invalid --area %q: expected a lowercase name with single interior hyphens", area)
		}
	}
	if remote && len(values.Areas) > 0 {
		return fmt.Errorf("--area is only supported for local trees; indexed tree entries have no verified area metadata")
	}
	var data wire.TreeResponse
	if remote {
		configuration := application.Configuration()
		server := values.Server
		if configuration != nil {
			server = configuration.ServerDefault(server)
		}
		data, err = application.APIClient().Tree(context.Background(), server, apiclient.TreeRequest{Repo: values.Repo, Path: values.Path, Depth: values.Depth})
	} else {
		data, err = command.local(values.Path, values.Depth, kind, []string(values.Areas))
	}
	if err != nil {
		return err
	}
	if err := rendercommand.Tree(data, application.Stdout(), values.JSON); err != nil {
		return err
	}
	if !values.JSON && len(data.Entries) == 0 {
		application.RequestExit(1)
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
