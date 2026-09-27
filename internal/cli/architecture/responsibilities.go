package architecture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/wire"
)

type architectureResponsibilitiesArgs struct {
	JSON bool `arg:"--json" help:"emit complete directory responsibilities JSON"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
}

type architectureResponsibilitiesOutput = analysis.ResponsibilityReport

type architectureDirectoryResponsibility = analysis.DirectoryResponsibility

func runArchitectureResponsibilities(args []string, dependencies Dependencies) error {
	values := architectureResponsibilitiesArgs{MaxOutputBytes: defaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple architecture responsibilities"}, &values)
	if err != nil {
		return err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			argumentParser.WriteHelp(outputDestination(dependencies))
			return nil
		}
		return err
	}
	return executeArchitectureResponsibilities(&values, dependencies)
}

func executeArchitectureResponsibilities(values *ResponsibilitiesArgs, dependencies Dependencies) error {
	if err := validateArchitectureOutputLimits(values.MaxFiles, values.MaxOutputBytes); err != nil {
		return err
	}
	if values.Repository != "" {
		response, requestErr := dependencies.remote(context.Background(), wire.AnalysisRequest{Operation: wire.AnalysisResponsibilities, Repository: values.Repository, Paths: values.Paths, MaxFiles: values.MaxFiles}, dependencies.serverDefault(values.Server))
		if requestErr != nil {
			return requestErr
		}
		if values.JSON {
			return stdoutWriter(dependencies).writeJSON(response)
		}
		var output architectureResponsibilitiesOutput
		if err := json.Unmarshal(response.Result, &output); err != nil {
			return fmt.Errorf("decode remote responsibilities: %w", err)
		}
		return renderArchitectureResponsibilities(output, values.MaxOutputBytes, dependencies)
	}
	architecture, err := Build(values.Paths, values.MaxFiles, dependencies)
	if err != nil {
		return err
	}
	output := buildArchitectureResponsibilitiesOutput(architecture)
	if values.JSON {
		return stdoutWriter(dependencies).writeJSON(output)
	}
	return renderArchitectureResponsibilities(output, values.MaxOutputBytes, dependencies)
}

func buildArchitectureResponsibilitiesOutput(architecture Report) architectureResponsibilitiesOutput {
	return analysis.BuildResponsibilitiesFromArchitecture(architecture)
}

func renderArchitectureResponsibilities(report architectureResponsibilitiesOutput, maxBytes int, dependencies Dependencies) error {
	output := stdoutWriter(dependencies)
	if maxBytes > 0 {
		output = newBoundedOutputWriter(outputDestination(dependencies), maxBytes)
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
