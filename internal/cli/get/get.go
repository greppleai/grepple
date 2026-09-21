package get

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	cliruntime "github.com/greppleai/grepple/internal/cli/runtime"
	"github.com/greppleai/grepple/linerange"
	"github.com/greppleai/grepple/parser"

	"github.com/alexflint/go-arg"
)

// Request contains parsed get command options.
type Request struct {
	cliruntime.CommonArgs
	Lines   string `arg:"--lines" placeholder:"A:B" help:"inclusive 1-based range; ranges that start in-file clamp at EOF"`
	JSON    bool   `arg:"--json" help:"print repository metadata and content as JSON"`
	Outline bool   `arg:"-O,--outline" help:"print the file's structural outline (classes, funcs, ...) instead of its contents"`
	Depth   int    `arg:"--depth" placeholder:"N" help:"outline: cap nesting depth for JSON/YAML (0 = unlimited)"`
	Repo    string `arg:"positional,required" placeholder:"OWNER/REPOSITORY"`
	Path    string `arg:"positional,required" placeholder:"PATH"`
}

// Description returns the command description used by the argument parser.
func (Request) Description() string {
	return "Fetch a file from an indexed repository."
}

// command owns indexed-file retrieval dependencies.
type command struct{ dependencies Dependencies }

// New constructs the get command.
func New(dependencies Dependencies) cliruntime.Command { return &command{dependencies: dependencies} }

// Run executes the get command. Deprecated: construct the command with New.
func Run(args []string, dependencies Dependencies) error { return New(dependencies).Run(args) }

// Run executes the get command.
func (command *command) Run(args []string) error {
	dependencies := command.dependencies
	var values Request
	parser, err := arg.NewParser(arg.Config{Program: "grepple get"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(dependencies.stdout())
			return nil
		}
		return err
	}
	base := dependencies.serverDefault(values.Server)
	target, err := RawURL(values, dependencies)
	if err != nil {
		return err
	}
	response, err := FetchContext(context.Background(), target.String(), base, dependencies)
	if err != nil {
		return dependencies.reportRangeError(err)
	}
	if response.RangeWarning != "" {
		fmt.Fprintln(dependencies.stderr(), "warning:", response.RangeWarning)
	}
	if response.RangeOutcome == linerange.OutcomePartialMiss {
		dependencies.recordRangeOutcome(response.RangeOutcome)
	}
	return render(values, response.Body, dependencies)
}

// RawURL builds the /public/raw URL.
func RawURL(values Request, dependencies Dependencies) (*url.URL, error) {
	base := dependencies.serverDefault(values.Server)
	target, err := url.Parse(strings.TrimRight(base, "/") + "/public/raw")
	if err != nil {
		return nil, err
	}
	query := target.Query()
	query.Set("repo", values.Repo)
	query.Set("path", values.Path)
	if values.Lines != "" {
		lineRange := strings.SplitN(values.Lines, ":", 2)
		if lineRange[0] != "" {
			query.Set("start", lineRange[0])
		}
		if len(lineRange) > 1 && lineRange[1] != "" {
			query.Set("end", lineRange[1])
		}
	}
	if values.JSON && !values.Outline {
		query.Set("format", "json")
	}
	target.RawQuery = query.Encode()
	return target, nil
}

// FetchResult carries source bytes plus range metadata from /public/raw.
type FetchResult struct {
	Body         []byte
	RangeOutcome linerange.Outcome
	RangeWarning string
}

// FetchContext performs the authorized GET and returns source and range metadata.
func FetchContext(ctx context.Context, target, base string, dependencies Dependencies) (FetchResult, error) {
	req, err := dependencies.newRequest(http.MethodGet, target, "", nil)
	if err != nil {
		return FetchResult{}, err
	}
	req = req.WithContext(ctx)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return FetchResult{}, err
	}
	defer response.Body.Close()
	outcome := linerange.Outcome(response.Header.Get(linerange.HeaderOutcome))
	warning := response.Header.Get(linerange.HeaderWarning)
	if response.StatusCode >= http.StatusMultipleChoices {
		if outcome == linerange.OutcomeFullMiss {
			dependencies.recordRangeOutcome(outcome)
		}
		body, _ := io.ReadAll(response.Body)
		failure := fmt.Errorf("server %s returned %d: %s", base, response.StatusCode, string(body))
		if outcome == linerange.OutcomeFullMiss {
			return FetchResult{}, dependencies.fullMissError(failure)
		}
		return FetchResult{}, failure
	}
	body, err := io.ReadAll(response.Body)
	return FetchResult{Body: body, RangeOutcome: outcome, RangeWarning: warning}, err
}

func render(values Request, body []byte, dependencies Dependencies) error {
	if !values.Outline {
		return cliruntime.NewOutput(dependencies.stdout()).WriteString(string(body))
	}
	outline := parser.OutlineFileDepth(values.Path, string(body), values.Depth)
	if values.JSON {
		return cliruntime.NewOutput(dependencies.stdout()).WriteJSON(outline)
	}
	if len(outline.Symbols) == 0 {
		dependencies.requestExit(1)
	}
	return cliruntime.NewOutput(dependencies.stdout()).WriteString(dependencies.renderOutline(outline, string(body)))
}
