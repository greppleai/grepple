package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
)

type architectureResponsibilitiesArgs struct {
	JSON    bool `arg:"--json" help:"emit complete directory responsibilities JSON"`
	Compact bool `arg:"--compact" help:"emit a bounded directory responsibility summary"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
}

type architectureResponsibilitiesOutput struct {
	Schema           string                                `json:"schema"`
	Files            int                                   `json:"files"`
	Sources          navigationSourceSummary               `json:"sources"`
	Responsibilities []architectureDirectoryResponsibility `json:"responsibilities"`
	Truncation       *navigationGraphTruncation            `json:"truncation,omitempty"`
}

type architectureDirectoryResponsibility struct {
	Directory       string              `json:"directory"`
	Files           int                 `json:"files"`
	Classifications []architectureCount `json:"classifications"`
	Languages       []architectureCount `json:"languages"`
	Declarations    []architectureCount `json:"declarations"`
	PublicCallables int                 `json:"publicCallables"`
	Entrypoints     int                 `json:"entrypoints"`
	Incoming        int                 `json:"incomingRelations"`
	Outgoing        int                 `json:"outgoingRelations"`
}

func runArchitectureResponsibilities(args []string) error {
	values := architectureResponsibilitiesArgs{MaxOutputBytes: DefaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple architecture responsibilities"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(os.Stdout)
			return nil
		}
		return err
	}
	if values.JSON == values.Compact {
		return fmt.Errorf("architecture responsibilities requires exactly one of --json or --compact")
	}
	if err := validateArchitectureOutputLimits(values.MaxFiles, values.MaxOutputBytes); err != nil {
		return err
	}
	if values.Repository != "" {
		response, requestErr := requestAnalysisRemote(context.Background(), api.AnalysisRequest{Operation: api.AnalysisResponsibilities, Repository: values.Repository, Paths: values.Paths, MaxFiles: values.MaxFiles}, serverDefault(values.Server))
		if requestErr != nil {
			return requestErr
		}
		if values.JSON {
			return stdoutWriter().writeJSON(response)
		}
		var output architectureResponsibilitiesOutput
		if err := json.Unmarshal(response.Result, &output); err != nil {
			return fmt.Errorf("decode remote responsibilities: %w", err)
		}
		return renderArchitectureResponsibilities(output, values.MaxOutputBytes)
	}
	architecture, err := buildDirectoryArchitecture(values.Paths, values.MaxFiles)
	if err != nil {
		return err
	}
	output := buildArchitectureResponsibilitiesOutput(architecture)
	if values.JSON {
		return stdoutWriter().writeJSON(output)
	}
	return renderArchitectureResponsibilities(output, values.MaxOutputBytes)
}

func buildArchitectureResponsibilitiesOutput(architecture directoryArchitecture) architectureResponsibilitiesOutput {
	incoming, outgoing := map[string]int{}, map[string]int{}
	for _, relation := range architecture.Relations {
		outgoing[relation.From] += relation.Count
		incoming[relation.To] += relation.Count
	}
	items := make([]architectureDirectoryResponsibility, 0, len(architecture.Directories))
	for _, directory := range architecture.Directories {
		items = append(items, architectureDirectoryResponsibility{Directory: directory.Path, Files: directory.Files, Classifications: directory.Classifications, Languages: directory.Languages, Declarations: directory.Declarations, PublicCallables: directory.PublicCallables, Entrypoints: directory.Entrypoints, Incoming: incoming[directory.Path], Outgoing: outgoing[directory.Path]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Directory < items[j].Directory })
	return architectureResponsibilitiesOutput{Schema: "grepple-directory-responsibilities-v2", Files: architecture.Files, Sources: architecture.Sources, Responsibilities: items, Truncation: architecture.Truncation}
}

func renderArchitectureResponsibilities(report architectureResponsibilitiesOutput, maxBytes int) error {
	output := stdoutWriter()
	if maxBytes > 0 {
		output = newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	if err := output.writeString(fmt.Sprintf("responsibilities %s files=%d directories=%d sources=%s\n", report.Schema, report.Files, len(report.Responsibilities), compactNavigationSourceSummary(report.Sources))); err != nil {
		return nil
	}
	if report.Truncation != nil {
		if err := output.writeString(fmt.Sprintf("! truncated %s limit=%d skipped=%d\n", report.Truncation.Reason, report.Truncation.Limit, report.Truncation.Skipped)); err != nil {
			return nil
		}
	}
	for _, item := range report.Responsibilities {
		if err := output.writeString(fmt.Sprintf("D %s files=%d public=%d entrypoints=%d incoming=%d outgoing=%d\n", item.Directory, item.Files, item.PublicCallables, item.Entrypoints, item.Incoming, item.Outgoing)); err != nil {
			return nil
		}
	}
	return nil
}
