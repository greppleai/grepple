package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/aiprovider"
)

const (
	defaultAskSteps       = 12
	defaultAskTimeout     = 10 * time.Minute
	defaultToolOutputSize = 64 << 10
	maxReadBytes          = 256 << 10
	maxReadLines          = 1000
)

type askArgs struct {
	Provider string   `arg:"--provider" placeholder:"NAME" help:"AI provider (default codex)"`
	Model    string   `arg:"--model" placeholder:"MODEL" help:"larger research model (user/provider default when omitted)"`
	Steps    int      `arg:"--max-steps" placeholder:"N" help:"maximum model/tool steps"`
	Timeout  int      `arg:"--timeout-seconds" placeholder:"N" help:"overall deadline in seconds"`
	Question []string `arg:"positional" placeholder:"QUESTION"`
}

type greppleToolInput struct {
	Args []string `json:"args" description:"Grepple CLI arguments, excluding the grepple executable"`
}

type readToolInput struct {
	Path      string `json:"path" description:"Repository-relative file path"`
	StartLine int    `json:"start_line,omitempty" description:"First 1-indexed line; defaults to 1"`
	EndLine   int    `json:"end_line,omitempty" description:"Last 1-indexed line; defaults to start+199"`
}

func validateAskArgs(values askArgs) (string, error) {
	question := strings.TrimSpace(strings.Join(values.Question, " "))
	if question == "" {
		return "", fmt.Errorf("ask requires a question")
	}
	if values.Steps < 1 || values.Steps > 30 {
		return "", fmt.Errorf("--max-steps must be between 1 and 30")
	}
	if values.Timeout < 1 || values.Timeout > 3600 {
		return "", fmt.Errorf("--timeout-seconds must be between 1 and 3600")
	}
	return question, nil
}

func runAsk(args []string) error {
	values := askArgs{Provider: "codex", Steps: defaultAskSteps, Timeout: int(defaultAskTimeout.Seconds())}
	parser, err := arg.NewParser(arg.Config{Program: "grepple ask"}, &values)
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
	question, err := validateAskArgs(values)
	if err != nil {
		return err
	}
	store, err := aiprovider.NewStore()
	if err != nil {
		return err
	}
	provider, err := aiprovider.NewRegistry(store, &http.Client{Timeout: 5 * time.Minute}).Provider(defaultAIProvider(values.Provider))
	if err != nil {
		return err
	}
	values.Model, err = resolveAskModel(values.Model, provider.DefaultModel())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(values.Timeout)*time.Second)
	defer cancel()
	model, err := provider.LanguageModel(ctx, values.Model)
	if err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	tools := []fantasy.AgentTool{
		fantasy.NewAgentTool("grepple", "Run any read-only Grepple CLI research command. Pass argv tokens without a shell. Use help, search, grit, graph, architecture, boundaries, sources, languages, get, tree, repos, refs, and related/at navigation as needed.", func(ctx context.Context, input greppleToolInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return runGreppleResearchTool(ctx, input)
		}),
		fantasy.NewAgentTool("read", "Read a bounded line range from one local file under the current working directory.", func(_ context.Context, input readToolInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			return runSimpleReadTool(root, input)
		}),
	}
	agent := fantasy.NewAgent(model,
		fantasy.WithSystemPrompt(askSystemPrompt(root)),
		fantasy.WithTools(tools...),
	)
	result, err := agent.Generate(ctx, fantasy.AgentCall{Prompt: question, StopWhen: []fantasy.StopCondition{fantasy.StepCountIs(values.Steps)}})
	if err != nil {
		return fmt.Errorf("ask %s/%s: %w", provider.Name(), values.Model, err)
	}
	answer := strings.TrimSpace(result.Response.Content.Text())
	if answer == "" {
		return fmt.Errorf("ask returned no text answer")
	}
	return stdoutWriter().writeString(answer + "\n")
}

func resolveAskModel(explicit, providerDefault string) (string, error) {
	if model := strings.TrimSpace(explicit); model != "" {
		return model, nil
	}
	model, err := configuredAIModel()
	if err != nil {
		return "", err
	}
	if model != "" {
		return model, nil
	}
	return providerDefault, nil
}

func askSystemPrompt(root string) string {
	return "You are Grepple's internal read-only research agent. Answer the user's question with precise, source-backed evidence. " +
		"Use the grepple tool aggressively to search, navigate, inspect indexed repositories, and retrieve exact ranges. Use the read tool only for bounded local files. " +
		"Start with counts, files, outlines, architecture, or repository trees; narrow before retrieving bodies. Cite repository/path:line ranges in the final answer. " +
		"Do not modify files, credentials, rules, artifacts, or configuration. Never invoke grepple ask or ai-provider. " +
		"The local workspace root is " + root + "."
}

func runGreppleResearchTool(ctx context.Context, input greppleToolInput) (fantasy.ToolResponse, error) {
	if len(input.Args) == 0 {
		return fantasy.NewTextErrorResponse("args must not be empty"), nil
	}
	if reason := deniedResearchCommand(input.Args); reason != "" {
		return fantasy.NewTextErrorResponse(reason), nil
	}
	executable, err := os.Executable()
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	args := append([]string(nil), input.Args...)
	args = append(args, "--no-spill")
	command := exec.CommandContext(ctx, executable, args...)
	output := &boundedBuffer{limit: defaultToolOutputSize}
	command.Stdout, command.Stderr = output, output
	err = command.Run()
	text := output.String()
	if output.truncated {
		text += "\n[tool output truncated; narrow the command]\n"
	}
	if err != nil {
		text += "\nexit: " + err.Error()
		return fantasy.NewTextErrorResponse(text), nil
	}
	return fantasy.NewTextResponse(text), nil
}

func deniedResearchCommand(args []string) string {
	command := strings.ToLower(args[0])
	switch command {
	case "ask", "ai-provider", "login", "logout":
		return "recursive or credential-mutating Grepple commands are unavailable to the research agent"
	case "artifacts":
		if len(args) > 1 && args[1] == "clean" {
			return "artifact deletion is unavailable to the research agent"
		}
	case "anchors":
		if containsArg(args, "--write") {
			return "anchor configuration writes are unavailable to the research agent"
		}
	case "rules":
		if len(args) > 1 && args[1] != "list" && args[1] != "results" {
			return "saved-rule mutations are unavailable to the research agent"
		}
	}
	return ""
}

func containsArg(args []string, value string) bool {
	for _, item := range args {
		if item == value {
			return true
		}
	}
	return false
}

func runSimpleReadTool(root string, input readToolInput) (fantasy.ToolResponse, error) {
	path, err := confinedReadPath(root, input.Path)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	if len(content) > maxReadBytes || bytes.IndexByte(content, 0) >= 0 {
		return fantasy.NewTextErrorResponse("file is binary or exceeds the 256 KiB read limit; use grepple search/outline instead"), nil
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	start := input.StartLine
	if start < 1 {
		start = 1
	}
	end := input.EndLine
	if end == 0 {
		end = start + 199
	}
	if end < start || end-start+1 > maxReadLines {
		return fantasy.NewTextErrorResponse("read range must be ordered and at most 1000 lines"), nil
	}
	if start > len(lines) {
		return fantasy.NewTextErrorResponse("start_line is beyond the file"), nil
	}
	end = min(end, len(lines))
	var output strings.Builder
	for line := start; line <= end; line++ {
		fmt.Fprintf(&output, "%d│%s\n", line, lines[line-1])
	}
	return fantasy.NewTextResponse(output.String()), nil
}

func confinedReadPath(root, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" || filepath.IsAbs(requested) {
		return "", fmt.Errorf("read path must be repository-relative")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Clean(requested)))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("read path escapes the working directory")
	}
	return path, nil
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *boundedBuffer) Write(content []byte) (int, error) {
	original := len(content)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		_, _ = b.buffer.Write(content[:min(remaining, len(content))])
	}
	if original > remaining {
		b.truncated = true
	}
	return original, nil
}

func (b *boundedBuffer) String() string { return b.buffer.String() }

var _ io.Writer = (*boundedBuffer)(nil)
