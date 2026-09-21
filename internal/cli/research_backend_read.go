package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/fantasy"
	internalagent "github.com/greppleai/grepple/internal/agent"
	"github.com/greppleai/grepple/internal/aiprovider"
	askcommand "github.com/greppleai/grepple/internal/cli/ask"
	rendercommand "github.com/greppleai/grepple/internal/render"
	"github.com/greppleai/grepple/linerange"
)

const (
	defaultToolOutputSize = 64 << 10
	maxReadBytes          = 256 << 10
	maxReadLines          = 1000
)

type readToolInput = askcommand.ReadInput
type readToolFileInput = askcommand.ReadRange

func runAskSession(ctx context.Context, log *internalagent.Log, provider aiprovider.Provider, request askcommand.SessionRequest) (string, error) {
	server := serverDefault(request.Server)
	session := newResearchSession(ctx, log, request.Root, server)
	backend := &cliResearchBackend{root: request.Root, server: server, session: session}
	return askcommand.RunResearch(ctx, log, provider, request, backend)
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
		rendercommand.RecordLineRangeError(rangeErr, contextGuardEnabled())
		return fantasy.NewTextErrorResponse(rangeErr.Error()), nil
	}
	if explicitEnd && resolved.Outcome == linerange.OutcomePartialMiss {
		rendercommand.RecordStandaloneLineRangeOutcome(resolved.Outcome, contextGuardEnabled())
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
