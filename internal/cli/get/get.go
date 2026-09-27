package get

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/apiclient"
	cliruntime "github.com/greppleai/grepple/internal/cliruntime"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/internal/linerange"
	"github.com/greppleai/grepple/internal/parser"
)

// request contains parsed get command options.
type Args struct {
	cliruntime.CommonArgs
	Lines   string   `arg:"--lines" placeholder:"A:B" help:"inclusive 1-based range; ranges that start in-file clamp at EOF"`
	JSON    bool     `arg:"--json" help:"print repository metadata and content as JSON"`
	Outline bool     `arg:"-O,--outline" help:"print the file's structural outline (classes, funcs, ...) instead of its contents"`
	Kinds   []string `arg:"--kind,separate" placeholder:"CATEGORY" help:"outline: show only types, functions, or variables; repeat or comma-separate"`
	Depth   int      `arg:"--depth" placeholder:"N" help:"outline: cap nesting depth for JSON/YAML (0 = unlimited)"`
	Repo    string   `arg:"positional,required" placeholder:"OWNER/REPOSITORY"`
	Path    string   `arg:"positional,required" placeholder:"PATH"`
}

// request is retained for package-local compatibility.
type request = Args

func (Args) Description() string {
	return "Fetch a file from an indexed repository."
}

// command owns indexed-file retrieval behavior.
type command struct{ context cliruntime.Context }

// New constructs the get command from the common command context.
func New(context cliruntime.Context) cliruntime.Command { return &command{context: context} }

// Run executes the get command.
func (command *command) Run(args []string) error {
	application := command.context
	if application == nil {
		return fmt.Errorf("get command context is unavailable")
	}
	var values Args
	parser, err := arg.NewParser(arg.Config{Program: "grepple get"}, &values)
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
	return Execute(application, &values)
}

// Execute fetches a repository file from application-parsed arguments.
func Execute(application cliruntime.Context, values *Args) error {
	if application == nil {
		return fmt.Errorf("get command context is unavailable")
	}
	if _, err := rendercommand.ParseOutlineKinds(values.Kinds, values.Outline); err != nil {
		return err
	}
	base := application.Configuration().ServerDefault(values.Server)
	request := apiclient.RawRequest{Repo: values.Repo, Path: values.Path, FormatJSON: values.JSON && !values.Outline}
	if values.Lines != "" {
		lineRange := strings.SplitN(values.Lines, ":", 2)
		request.Start = lineRange[0]
		if len(lineRange) > 1 {
			request.End = lineRange[1]
		}
	}
	response, err := application.APIClient().Raw(context.Background(), base, request)
	if err != nil {
		if apiclient.RangeOutcome(err) == linerange.OutcomeFullMiss {
			rendercommand.RecordStandaloneLineRangeOutcome(linerange.OutcomeFullMiss, application.Configuration().ContextGuardEnabled())
			return reportRangeError(application, err, true)
		}
		return reportRangeError(application, err, false)
	}
	if response.RangeWarning != "" {
		fmt.Fprintln(application.Stderr(), "warning:", response.RangeWarning)
	}
	if response.RangeOutcome == linerange.OutcomePartialMiss {
		rendercommand.RecordStandaloneLineRangeOutcome(response.RangeOutcome, application.Configuration().ContextGuardEnabled())
	}
	return render(application, *values, response.Body)
}

func render(application cliruntime.Context, values Args, body []byte) error {
	if !values.Outline {
		return cliruntime.NewOutput(application.Stdout()).WriteString(string(body))
	}
	outline := parser.OutlineFileDepth(values.Path, string(body), values.Depth)
	kinds, err := rendercommand.ParseOutlineKinds(values.Kinds, true)
	if err != nil {
		return err
	}
	outline = rendercommand.FilterOutline(outline, kinds)
	if values.JSON {
		return cliruntime.NewOutput(application.Stdout()).WriteJSON(outline)
	}
	if len(outline.Symbols) == 0 {
		application.RequestExit(1)
	}
	if len(kinds) > 0 {
		return cliruntime.NewOutput(application.Stdout()).WriteString(rendercommand.Outline(outline))
	}
	return cliruntime.NewOutput(application.Stdout()).WriteString(rendercommand.OutlineOrContent(outline, string(body)))
}
