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
	"github.com/greppleai/grepple/linerange"
)

const (
	defaultAskTimeout     = 10 * time.Minute
	defaultToolOutputSize = 64 << 10
	maxReadBytes          = 256 << 10
	maxReadLines          = 1000
)

type askArgs struct {
	Provider string `arg:"--provider" placeholder:"NAME" help:"AI provider: anthropic, anthropic-subscription, bedrock, codex, copilot, or openai"`
	Model    string `arg:"--model" placeholder:"[PROVIDER/]MODEL" help:"research model, optionally prefixed with its provider"`
	commonArgs
	Timeout  int      `arg:"--timeout-seconds" placeholder:"N" help:"overall deadline in seconds"`
	Question []string `arg:"positional" placeholder:"QUESTION"`
}

type readToolInput struct {
	Path       string              `json:"path,omitempty" description:"One repository-relative file path; omit when files is provided"`
	Files      []readToolFileInput `json:"files,omitempty" description:"Up to eight local or indexed-repository file ranges to read in one call"`
	Repository string              `json:"repository,omitempty" description:"Exact indexed OWNER/REPO[@REF] selector; omit for local workspace"`
	StartLine  int                 `json:"start_line,omitempty" description:"First 1-indexed line for path; defaults to 1"`
	EndLine    int                 `json:"end_line,omitempty" description:"Last 1-indexed line for path; defaults to start+199; explicit ranges clamp at EOF when start exists"`
	Outline    bool                `json:"outline,omitempty" description:"Return path's structural outline instead of source lines"`
}

type readToolFileInput struct {
	Path      string `json:"path" description:"Repository-relative file path"`
	StartLine int    `json:"start_line,omitempty" description:"First 1-indexed line; defaults to 1"`
	EndLine   int    `json:"end_line,omitempty" description:"Last 1-indexed line; defaults to start+199"`
	Outline   bool   `json:"outline,omitempty" description:"Return the structural outline instead of source lines"`
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
	values := askArgs{Timeout: int(defaultAskTimeout.Seconds())}
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
	registry := aiprovider.NewRegistry(store, &http.Client{Timeout: 5 * time.Minute})
	preferences, err := loadConfiguredAskPreferences()
	if err != nil {
		return err
	}
	providerName, modelName, err := resolveAskSelection(values.Provider, values.Model, preferences.Model)
	if err != nil {
		return err
	}
	provider, err := registry.Provider(providerName)
	if err != nil {
		return err
	}
	if modelName == "" {
		modelName = provider.DefaultModel()
	}
	values.Provider = provider.Name()
	values.Model = modelName
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	log, err := newAskLogWithOptions(askLogOptions{enabled: preferences.LogsEnabled, retention: preferences.LogRetention})
	if err != nil {
		return err
	}
	if log.Path() != "" {
		fmt.Fprintln(os.Stderr, "Ask log:", log.Path())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(values.Timeout)*time.Second)
	defer cancel()
	runErr := runLoggedAsk(ctx, log, provider, values, question, root)
	closeErr := log.Close()
	return errors.Join(runErr, closeErr)
}

func runLoggedAsk(ctx context.Context, log *askLog, provider aiprovider.Provider, values askArgs, question, root string) (returnErr error) {
	systemPrompt := askSystemPrompt(root)
	server := serverDefault(values.Server)
	session := newResearchSession(ctx, log, root, server)
	defer func() {
		session.Close()
		returnErr = errors.Join(returnErr, log.Record("session.performance", session.telemetry.performance(time.Now())))
	}()
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
	session.telemetry.startStream(time.Now())
	result, err := agent.Stream(ctx, loggedAgentStreamCall(log, session.telemetry, question))
	session.telemetry.finishStream(time.Now())
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

func loggedAgentStreamCall(log *askLog, telemetry *askTelemetry, question string) fantasy.AgentStreamCall {
	return fantasy.AgentStreamCall{
		Prompt: question,
		OnStepStart: func(step int) error {
			telemetry.beginLLM(step, time.Now())
			return log.Record("step.start", map[string]int{"step": step})
		},
		OnChunk: func(part fantasy.StreamPart) error {
			telemetry.recordChunk(part, time.Now())
			return nil
		},
		OnToolCall:    func(call fantasy.ToolCallContent) error { return log.Record("tool.call", call) },
		OnToolResult:  func(result fantasy.ToolResultContent) error { return log.Record("tool.result", result) },
		OnStepFinish:  func(result fantasy.StepResult) error { return log.Record("step.finish", result) },
		OnAgentFinish: func(result *fantasy.AgentResult) error { return log.Record("agent.finish", result) },
		OnError: func(err error) {
			if timing, ok := telemetry.finishLLM(time.Now(), fantasy.Usage{}, fantasy.FinishReasonError); ok {
				_ = log.Record("llm.timing", timing)
			}
			_ = log.Record("agent.error", map[string]string{"error": err.Error()})
		},
		OnStreamFinish: func(usage fantasy.Usage, reason fantasy.FinishReason, metadata fantasy.ProviderMetadata) error {
			streamErr := log.Record("stream.finish", map[string]any{"usage": usage, "finishReason": reason, "providerMetadata": metadata})
			timing, ok := telemetry.finishLLM(time.Now(), usage, reason)
			if !ok {
				return streamErr
			}
			return errors.Join(streamErr, log.Record("llm.timing", timing))
		},
	}
}

func recordAskError(log *askLog, err error) error {
	logErr := log.Record("session.error", map[string]string{"error": err.Error()})
	return errors.Join(err, logErr)
}

func resolveAskSelection(explicitProvider, explicitModel, configuredModel string) (string, string, error) {
	provider := defaultAIProvider(explicitProvider)
	if explicit := strings.TrimSpace(explicitModel); explicit != "" {
		return resolveExplicitAskModel(provider, strings.TrimSpace(explicitProvider) != "", explicit)
	}
	return resolveConfiguredAskModel(provider, strings.TrimSpace(explicitProvider) != "", configuredModel)
}

func resolveExplicitAskModel(provider string, providerWasExplicit bool, selector string) (string, string, error) {
	selectedProvider, model, prefixed, err := parseAskModelSelector(selector)
	if err != nil {
		return "", "", err
	}
	if !prefixed {
		return provider, model, nil
	}
	if providerWasExplicit && provider != selectedProvider {
		return "", "", fmt.Errorf("--provider %q conflicts with --model provider %q", provider, selectedProvider)
	}
	return selectedProvider, model, nil
}

func resolveConfiguredAskModel(provider string, providerWasExplicit bool, configuredModel string) (string, string, error) {
	configured := strings.TrimSpace(configuredModel)
	if configured == "" {
		return provider, "", nil
	}
	selectedProvider, model, prefixed, err := parseAskModelSelector(configured)
	if err != nil {
		return "", "", fmt.Errorf("configured ask.model: %w", err)
	}
	if prefixed {
		if !providerWasExplicit || provider == selectedProvider {
			return selectedProvider, model, nil
		}
		return provider, "", nil
	}
	// Unprefixed user values predate multiple providers and remain Codex-only.
	if provider == "codex" {
		return provider, model, nil
	}
	return provider, "", nil
}

func parseAskModelSelector(value string) (provider, model string, prefixed bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false, fmt.Errorf("model cannot be empty")
	}
	provider, model, found := strings.Cut(value, "/")
	if !found {
		return "", value, false, nil
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return "", "", false, fmt.Errorf("model must use <provider>/<model>")
	}
	return provider, model, true, nil
}

func askSystemPrompt(root string) string {
	return "You are Grepple's internal read-only source retrieval agent. Return precise, source-backed evidence; do not act as a code reviewer or provide a second-model approval. " +
		"Use read_file with its files array to batch-read known local ranges, preserving every HASH│LINE│content row exactly when anchors are present. Use focused discovery tools only when paths or ranges are unknown; after discovery, read the final local ranges and never substitute unanchored search snippets for anchored source rows. " +
		"Use repository_tree and a repository selector for remote indexed source. Use explain_sources before completeness-sensitive conclusions. Cite repository/path:line ranges in synthesized answers. " +
		"Make at most two tool calls at a time and stop as soon as the requested source evidence is available. Do not repeat equivalent searches. Return requested batch reads directly rather than narrating a review. " +
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
	normalizedContent, err := normalizeAnchorContent(string(content))
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	lines := linerange.SplitLines(normalizedContent)
	start, end, resolved, explicitEnd, rangeErr := resolveReadToolRange(input, len(lines))
	if rangeErr != nil {
		recordLineRangeError(rangeErr)
		return fantasy.NewTextErrorResponse(rangeErr.Error()), nil
	}
	if explicitEnd && resolved.Outcome == linerange.OutcomePartialMiss {
		recordStandaloneLineRangeOutcome(resolved.Outcome)
	}
	lineNumbers := make([]int, 0, end-start+1)
	for line := start; line <= end; line++ {
		lineNumbers = append(lineNumbers, line)
	}
	anchors, anchored, err := defaultReadAnchors(path, normalizedContent, lineNumbers)
	if err != nil {
		return fantasy.NewTextErrorResponse("generate read anchors: " + err.Error()), nil
	}
	var output strings.Builder
	for line := start; line <= end; line++ {
		if anchored {
			fmt.Fprintf(&output, "%s│%d│%s\n", anchors[line], line, lines[line-1])
		} else {
			fmt.Fprintf(&output, "%d│%s\n", line, lines[line-1])
		}
	}
	if explicitEnd && resolved.Warning != "" {
		fmt.Fprintf(&output, "warning: %s\n", resolved.Warning)
	}
	return fantasy.NewTextResponse(output.String()), nil
}

func resolveReadToolRange(input readToolInput, lineCount int) (int, int, linerange.Result, bool, error) {
	start := input.StartLine
	if start < 1 {
		start = 1
	}
	end := input.EndLine
	explicitEnd := end > 0
	if !explicitEnd {
		end = start + 199
	}
	if end < start || end-start+1 > maxReadLines {
		return 0, 0, linerange.Result{}, explicitEnd, fmt.Errorf("read range must be ordered and at most 1000 lines")
	}
	resolved, err := linerange.Resolve(start, end, lineCount)
	return start, resolved.ReturnedEnd, resolved, explicitEnd, err
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
