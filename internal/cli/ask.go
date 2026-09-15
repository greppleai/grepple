package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/aiprovider"
)

const (
	defaultAskTimeout     = 10 * time.Minute
	defaultToolOutputSize = 64 << 10
	maxReadBytes          = 256 << 10
	maxReadLines          = 1000
)

type askArgs struct {
	Provider string   `arg:"--provider" placeholder:"NAME" help:"AI provider (default codex)"`
	Model    string   `arg:"--model" placeholder:"MODEL" help:"larger research model (user/provider default when omitted)"`
	Server   string   `arg:"--server" placeholder:"URL" help:"remote Grepple service available to research tools"`
	Timeout  int      `arg:"--timeout-seconds" placeholder:"N" help:"overall deadline in seconds"`
	Question []string `arg:"positional" placeholder:"QUESTION"`
}

type readToolInput struct {
	Path       string `json:"path" description:"Repository-relative file path"`
	Repository string `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for local workspace"`
	StartLine  int    `json:"start_line,omitempty" description:"First 1-indexed line; defaults to 1"`
	EndLine    int    `json:"end_line,omitempty" description:"Last 1-indexed line; defaults to start+199"`
	Outline    bool   `json:"outline,omitempty" description:"Return the file's structural outline instead of source lines"`
}

func validateAskArgs(values askArgs) (string, error) {
	question := strings.TrimSpace(strings.Join(values.Question, " "))
	if question == "" {
		return "", fmt.Errorf("ask requires a question")
	}
	if values.Timeout < 1 || values.Timeout > 3600 {
		return "", fmt.Errorf("--timeout-seconds must be between 1 and 3600")
	}
	return question, nil
}

func runAsk(args []string) error {
	values := askArgs{Provider: "codex", Timeout: int(defaultAskTimeout.Seconds())}
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
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	log, err := newAskLog()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Ask log:", log.Path())
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(values.Timeout)*time.Second)
	defer cancel()
	runErr := runLoggedAsk(ctx, log, provider, values, question, root)
	closeErr := log.Close()
	return errors.Join(runErr, closeErr)
}

func runLoggedAsk(ctx context.Context, log *askLog, provider aiprovider.Provider, values askArgs, question, root string) error {
	systemPrompt := askSystemPrompt(root)
	server := serverDefault(values.Server)
	session := newResearchSession(ctx, log, root, server)
	defer session.Close()
	tools := newAskResearchToolsForSession(session, root, server)
	if err := log.Record("session.start", map[string]any{
		"provider": provider.Name(), "model": values.Model, "question": question, "root": root, "server": server,
		"timeoutSeconds": values.Timeout, "systemPrompt": systemPrompt,
		"tools": askResearchToolInfo(tools),
	}); err != nil {
		return err
	}
	model, err := provider.LanguageModel(ctx, values.Model)
	if err != nil {
		return recordAskError(log, err)
	}
	agent := fantasy.NewAgent(model, fantasy.WithSystemPrompt(systemPrompt), fantasy.WithTools(tools...))
	result, err := agent.Stream(ctx, loggedAgentStreamCall(log, question))
	if err != nil {
		return recordAskError(log, fmt.Errorf("ask %s/%s: %w", provider.Name(), values.Model, err))
	}
	answer := strings.TrimSpace(result.Response.Content.Text())
	if answer == "" {
		return recordAskError(log, fmt.Errorf("ask returned no text answer"))
	}
	if err := log.Record("session.finish", map[string]any{"answer": answer, "usage": result.TotalUsage, "steps": len(result.Steps)}); err != nil {
		return err
	}
	return stdoutWriter().writeString(answer + "\n")
}

func loggedAgentStreamCall(log *askLog, question string) fantasy.AgentStreamCall {
	return fantasy.AgentStreamCall{
		Prompt:        question,
		OnStepStart:   func(step int) error { return log.Record("step.start", map[string]int{"step": step}) },
		OnToolCall:    func(call fantasy.ToolCallContent) error { return log.Record("tool.call", call) },
		OnToolResult:  func(result fantasy.ToolResultContent) error { return log.Record("tool.result", result) },
		OnStepFinish:  func(result fantasy.StepResult) error { return log.Record("step.finish", result) },
		OnAgentFinish: func(result *fantasy.AgentResult) error { return log.Record("agent.finish", result) },
		OnError:       func(err error) { _ = log.Record("agent.error", map[string]string{"error": err.Error()}) },
		OnStreamFinish: func(usage fantasy.Usage, reason fantasy.FinishReason, metadata fantasy.ProviderMetadata) error {
			return log.Record("stream.finish", map[string]any{"usage": usage, "finishReason": reason, "providerMetadata": metadata})
		},
	}
}

func recordAskError(log *askLog, err error) error {
	logErr := log.Record("session.error", map[string]string{"error": err.Error()})
	return errors.Join(err, logErr)
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
		"Use the focused research tools directly; there is no shell or CLI. Start with search_code count/files or inspect_architecture when scope is unknown, then use snippets, navigate_code, query_graph, structural_search, or read_file only as needed. " +
		"Use repository_tree and a repository selector for remote indexed source. Use explain_sources before completeness-sensitive conclusions. Cite repository/path:line ranges in the final answer. " +
		"Make at most two tool calls at a time and stop researching as soon as the evidence answers the question. Do not repeat equivalent searches or read whole files when snippets or navigation suffice. Return the text answer immediately once sufficient evidence is available. " +
		"Navigation is syntax-based, structural queries prove syntax rather than types or data flow, and bounded results can have more pages. Do not modify files, credentials, artifacts, or configuration. " +
		"The local workspace root is " + root + "."
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
