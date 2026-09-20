package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/fantasy"
	internalagent "github.com/greppleai/grepple/internal/agent"
	agentresearch "github.com/greppleai/grepple/internal/agent/research"
	"github.com/greppleai/grepple/internal/aiprovider"
	askcommand "github.com/greppleai/grepple/internal/cli/ask"
	"github.com/greppleai/grepple/linerange"
)

const (
	defaultToolOutputSize = 64 << 10
	maxReadBytes          = 256 << 10
	maxReadLines          = 1000
)

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

func runAskSession(ctx context.Context, log *internalagent.Log, provider aiprovider.Provider, request askcommand.SessionRequest) (returnAnswer string, returnErr error) {
	systemPrompt := agentresearch.SystemPrompt(request.Root)
	server := serverDefault(request.Server)
	session := newResearchSession(ctx, log, request.Root, server)
	defer func() {
		session.Close()
		returnErr = errors.Join(returnErr, log.Record("session.performance", session.telemetry.Performance(time.Now())))
	}()
	tools := newAskResearchToolsForSession(session, request.Root, server)
	if err := log.Record("session.start", map[string]any{
		"provider": provider.Name(), "model": request.Model, "question": request.Question, "root": request.Root, "server": server,
		"timeoutSeconds": request.Timeout, "systemPrompt": systemPrompt,
		"tools": askResearchToolInfo(tools),
	}); err != nil {
		return "", err
	}
	result, err := internalagent.Run(ctx, log, internalagent.Request{
		Name: "ask", Prompt: request.Question, SystemPrompt: systemPrompt, Provider: provider.Name(), Model: request.Model,
		ModelFactory: func(ctx context.Context) (fantasy.LanguageModel, error) {
			return provider.LanguageModel(ctx, request.Model)
		},
		Tools: tools, Telemetry: session.telemetry,
	})
	if err != nil {
		return "", err
	}
	return result.Answer, nil
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
