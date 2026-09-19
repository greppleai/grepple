package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/greppleai/grepple/linerange"
	"github.com/greppleai/grepple/parser"

	"github.com/alexflint/go-arg"
)

type getArgs struct {
	commonArgs
	Lines   string `arg:"--lines" placeholder:"A:B" help:"inclusive 1-based range; ranges that start in-file clamp at EOF"`
	JSON    bool   `arg:"--json" help:"print repository metadata and content as JSON"`
	Outline bool   `arg:"-O,--outline" help:"print the file's structural outline (classes, funcs, ...) instead of its contents"`
	Depth   int    `arg:"--depth" placeholder:"N" help:"outline: cap nesting depth for JSON/YAML (0 = unlimited)"`
	Repo    string `arg:"positional,required" placeholder:"OWNER/REPOSITORY"`
	Path    string `arg:"positional,required" placeholder:"PATH"`
}

func (getArgs) Description() string {
	return "Fetch a file from an indexed repository."
}

func runGet(args []string) error {
	var values getArgs
	parser, err := arg.NewParser(arg.Config{Program: "grepple get"}, &values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	base := serverDefault(values.Server)
	target, err := getRawURL(values)
	if err != nil {
		return err
	}
	response, err := fetchRaw(target.String(), base)
	if err != nil {
		return reportLineRangeCommandError(err)
	}
	if response.RangeWarning != "" {
		fmt.Fprintln(os.Stderr, "warning:", response.RangeWarning)
	}
	if response.RangeOutcome == linerange.OutcomePartialMiss {
		recordStandaloneLineRangeOutcome(response.RangeOutcome)
	}
	return renderGet(values, response.Body)
}

// getRawURL builds the /public/raw URL with the repo, path, optional line
// range, and format query parameters.
func getRawURL(values getArgs) (*url.URL, error) {
	base := serverDefault(values.Server)
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

// rawFetchResult carries source bytes plus range metadata from /public/raw.
type rawFetchResult struct {
	Body         []byte
	RangeOutcome linerange.Outcome
	RangeWarning string
}

// fetchRaw performs the authorized GET and returns the body, surfacing the
// server's error text for non-2xx responses.
func fetchRaw(target, base string) (rawFetchResult, error) {
	return fetchRawContext(context.Background(), target, base)
}

func fetchRawContext(ctx context.Context, target, base string) (rawFetchResult, error) {
	req, err := authorizedRequest(http.MethodGet, target, "", nil)
	if err != nil {
		return rawFetchResult{}, err
	}
	req = req.WithContext(ctx)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return rawFetchResult{}, err
	}
	defer response.Body.Close()
	outcome := linerange.Outcome(response.Header.Get(linerange.HeaderOutcome))
	warning := response.Header.Get(linerange.HeaderWarning)
	if response.StatusCode >= http.StatusMultipleChoices {
		if outcome == linerange.OutcomeFullMiss {
			recordStandaloneLineRangeOutcome(outcome)
		}
		body, _ := io.ReadAll(response.Body)
		failure := fmt.Errorf("server %s returned %d: %s", base, response.StatusCode, string(body))
		if outcome == linerange.OutcomeFullMiss {
			return rawFetchResult{}, remoteFullLineRangeMissError{err: failure}
		}
		return rawFetchResult{}, failure
	}
	body, err := io.ReadAll(response.Body)
	return rawFetchResult{Body: body, RangeOutcome: outcome, RangeWarning: warning}, err
}

// renderGet prints the fetched body: as a structural outline under --outline
// (exit 1 when the file yields no symbols), raw content otherwise.
func renderGet(values getArgs, body []byte) error {
	if !values.Outline {
		return stdoutWriter().writeString(string(body))
	}
	outline := parser.OutlineFileDepth(values.Path, string(body), values.Depth)
	if values.JSON {
		return stdoutWriter().writeJSON(outline)
	}
	if len(outline.Symbols) == 0 {
		setExit(1)
	}
	return stdoutWriter().writeString(RenderOutlineOrContent(outline, string(body)))
}
