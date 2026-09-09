package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/greppleai/grepple/parser"

	"github.com/alexflint/go-arg"
)

type getArgs struct {
	Server  string `arg:"-s,--server" placeholder:"URL" help:"remote shard/router URL"`
	Lines   string `arg:"--lines" placeholder:"A:B" help:"inclusive 1-based line range"`
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
	body, err := fetchRaw(target.String(), base)
	if err != nil {
		return err
	}
	return renderGet(values, body)
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

// fetchRaw performs the authorized GET and returns the body, surfacing the
// server's error text for non-2xx responses.
func fetchRaw(target, base string) ([]byte, error) {
	req, err := authorizedRequest(http.MethodGet, target, "", nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("server %s returned %d: %s", base, response.StatusCode, string(body))
	}
	return io.ReadAll(response.Body)
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
