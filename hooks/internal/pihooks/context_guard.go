package pihooks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	codeparser "github.com/greppleai/grepple/parser"
)

const (
	contextGuardSchema       = "grepple-context-guard-v1"
	contextGuardStatsSchema  = "grepple-context-guard-stats-v1"
	contextGuardMinBlockSize = 128
	contextGuardMaxEntries   = 50_000
	contextGuardMaxCacheSize = 16 << 20
	contextGuardLockTimeout  = 2 * time.Second
)

type contextGuardInput struct {
	SessionID     string          `json:"session_id"`
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	Source        string          `json:"source"`
}

type contextGuardCache struct {
	Schema    string                       `json:"schema"`
	SessionID string                       `json:"sessionId"`
	Entries   map[string]contextGuardEntry `json:"entries"`
}

type contextGuardEntry struct {
	Bytes int `json:"bytes"`
}

type contextGuardStats struct {
	Schema            string  `json:"schema"`
	SessionID         string  `json:"sessionId"`
	Period            int     `json:"period"`
	ResetReason       string  `json:"resetReason"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	ObservedResponses int     `json:"observedResponses"`
	GreppleResponses  int     `json:"greppleResponses"`
	ReadResponses     int     `json:"readResponses"`
	ObservedBlocks    int     `json:"observedBlocks"`
	NewBlocks         int     `json:"newBlocks"`
	RemovedBlocks     int     `json:"removedBlocks"`
	InputBytes        int64   `json:"inputBytes"`
	ReturnedBytes     int64   `json:"returnedBytes"`
	GrossRemovedBytes int64   `json:"grossRemovedBytes"`
	NetSavedBytes     int64   `json:"netSavedBytes"`
	NetSavingsPercent float64 `json:"netSavingsPercent"`
}

type contextGuardHookOutput struct {
	HookSpecificOutput contextGuardHookSpecificOutput `json:"hookSpecificOutput"`
}

type contextGuardHookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	UpdatedToolOutput string `json:"updatedToolOutput"`
}

// HandleContextGuard records human-readable Grepple output already returned to
// one Pi session. Repeated unchanged blocks are replaced with a compact notice.
// Cache failures fail open and never alter the tool response.
func HandleContextGuard(input []byte) []byte {
	var payload contextGuardInput
	if json.Unmarshal(input, &payload) != nil || payload.HookEventName != "PostToolUse" || payload.SessionID == "" {
		return nil
	}
	switch payload.ToolName {
	case "Bash":
		var toolInput struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(payload.ToolInput, &toolInput) != nil || !CommandInvokesGrepple(toolInput.Command) || commandRequestsJSON(toolInput.Command) {
			return nil
		}
	case "Read":
		// Pi has already resolved and read the local file. Its adjusted textual
		// response is safe to deduplicate using the same content digest contract.
	default:
		return nil
	}
	output, ok := contextGuardToolText(payload.ToolResponse)
	if !ok || !guardableContextOutput(output) {
		return nil
	}
	guarded, omittedBlocks, _, err := guardContextOutput(payload.SessionID, payload.ToolName, output)
	if err != nil || omittedBlocks == 0 {
		return nil
	}
	encoded, err := json.Marshal(contextGuardHookOutput{HookSpecificOutput: contextGuardHookSpecificOutput{
		HookEventName: "PostToolUse", UpdatedToolOutput: guarded,
	}})
	if err != nil {
		return nil
	}
	return encoded
}

func contextGuardToolText(response json.RawMessage) (string, bool) {
	var direct string
	if json.Unmarshal(response, &direct) == nil {
		return direct, true
	}
	var structured struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(response, &structured) != nil || len(structured.Content) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(structured.Content))
	for _, part := range structured.Content {
		if part.Type != "text" {
			return "", false
		}
		parts = append(parts, part.Text)
	}
	return strings.Join(parts, "\n"), true
}

// HandleContextInvalidation clears a session cache after successful compaction
// and when Pi starts from an explicitly cleared or compacted context.
func HandleContextInvalidation(input []byte) []byte {
	var payload contextGuardInput
	if json.Unmarshal(input, &payload) != nil || payload.SessionID == "" {
		return nil
	}
	switch {
	case payload.HookEventName == "PostCompact":
		_ = rotateContextGuardSession(payload.SessionID, "compact")
	case payload.HookEventName == "SessionStart" && (payload.Source == "clear" || payload.Source == "fork"):
		_ = rotateContextGuardSession(payload.SessionID, payload.Source)
	case payload.HookEventName == "SessionStart" && payload.Source == "compact":
		// PostCompact normally rotated the period. This fallback only clears a
		// cache left by an older hook version, avoiding a double rotation.
		_ = invalidateContextGuardSession(payload.SessionID)
	}
	return nil
}

// CommandInvokesGrepple reports whether parsed shell syntax executes a command
// whose basename is grepple, including commands nested in a shell -c script.
func CommandInvokesGrepple(command string) bool {
	return commandInvokesGrepple(command, 0)
}

func commandInvokesGrepple(command string, depth int) bool {
	if depth > 8 {
		return false
	}
	document, err := codeparser.ParseDocument("shell", command)
	if err != nil {
		return false
	}
	defer document.Close()
	found := false
	if document.Read(func(view codeparser.DocumentView) error {
		found = greppleInvocationInTree(view.Root(), []byte(command), depth)
		return nil
	}) != nil {
		return false
	}
	return found
}

func greppleInvocationInTree(node codeparser.ViewNode, source []byte, depth int) bool {
	if node.Kind() == "command" && greppleInvocationInCommand(node, source, depth) {
		return true
	}
	for index := 0; index < node.NamedChildCount(); index++ {
		if greppleInvocationInTree(node.NamedChild(index), source, depth) {
			return true
		}
	}
	return false
}

func greppleInvocationInCommand(command codeparser.ViewNode, source []byte, depth int) bool {
	nameNode := command.ChildByFieldName("name")
	if !nameNode.Valid() {
		return false
	}
	name, literal := literalWord(nodeSource(nameNode, source))
	if literal && filepath.Base(name) == "grepple" {
		return true
	}
	words := []string{name}
	for index := 0; index < command.NamedChildCount(); index++ {
		child := command.NamedChild(index)
		if child.Range().StartByte < nameNode.Range().EndByte || !isArgumentNode(child.Kind()) {
			continue
		}
		word, ok := literalWord(nodeSource(child, source))
		if !ok {
			word = "\x00"
		}
		words = append(words, word)
	}
	if script := envSplitScript(words); script != "" {
		return commandInvokesGrepple(script, depth+1)
	}
	index := effectiveCommandIndex(words)
	if index < 0 || !isShellInterpreter(filepath.Base(words[index])) {
		return false
	}
	script := shellCommandString(words[index+1:])
	return script != "" && commandInvokesGrepple(script, depth+1)
}

func commandRequestsJSON(command string) bool {
	return strings.Contains(command, "--json")
}

func guardableContextOutput(output string) bool {
	trimmed := strings.TrimSpace(output)
	return len(trimmed) >= contextGuardMinBlockSize &&
		!strings.HasPrefix(trimmed, "grepple output spilled ") &&
		!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")
}

func guardContextOutput(sessionID, toolName, output string) (string, int, int, error) {
	directory, err := contextGuardDirectory()
	if err != nil {
		return "", 0, 0, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", 0, 0, err
	}
	base := contextGuardSessionName(sessionID)
	cachePath := filepath.Join(directory, base+".json")
	release, err := acquireContextGuardLock(cachePath + ".lock")
	if err != nil {
		return "", 0, 0, err
	}
	defer release()

	cache := readContextGuardCache(cachePath, sessionID)
	blocks := splitContextBlocks(output)
	guarded := make([]string, 0, len(blocks))
	omittedBlocks, omittedBytes, newBlocks := 0, 0, 0
	for _, block := range blocks {
		if len(strings.TrimSpace(block)) < contextGuardMinBlockSize {
			guarded = append(guarded, block)
			continue
		}
		digest := contextBlockDigest(block)
		if _, seen := cache.Entries[digest]; seen {
			omittedBlocks++
			omittedBytes += len(block)
			continue
		}
		guarded = append(guarded, block)
		newBlocks++
		if len(cache.Entries) < contextGuardMaxEntries {
			cache.Entries[digest] = contextGuardEntry{Bytes: len(block)}
		}
	}
	if err := writeContextGuardCache(cachePath, cache); err != nil {
		return "", 0, 0, err
	}
	returned := output
	if omittedBlocks > 0 {
		marker := fmt.Sprintf("[grepple context guard: omitted %d unchanged output %s (%d bytes) already emitted in this session; cache resets after compaction]\n", omittedBlocks, pluralWord("block", omittedBlocks), omittedBytes)
		returned = marker + strings.Join(guarded, "")
	}
	// Statistics are observational and must never decide whether source is
	// suppressed. A stats write failure therefore leaves cache behavior intact.
	_ = updateContextGuardStats(directory, base, sessionID, toolName, len(blocks), newBlocks, omittedBlocks, len(output), len(returned), omittedBytes)
	return returned, omittedBlocks, omittedBytes, nil
}

func updateContextGuardStats(directory, base, sessionID, toolName string, observedBlocks, newBlocks, removedBlocks, inputBytes, returnedBytes, removedBytes int) error {
	period := latestContextGuardStatsPeriod(directory, base)
	if period < 0 {
		period = 0
	}
	path := contextGuardStatsPath(directory, base, period)
	stats := readContextGuardStats(path, sessionID, period)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if stats.CreatedAt == "" {
		stats.CreatedAt = now
		stats.ResetReason = "session-start"
	}
	stats.UpdatedAt = now
	stats.ObservedResponses++
	if toolName == "Read" {
		stats.ReadResponses++
	} else {
		stats.GreppleResponses++
	}
	stats.ObservedBlocks += observedBlocks
	stats.NewBlocks += newBlocks
	stats.RemovedBlocks += removedBlocks
	stats.InputBytes += int64(inputBytes)
	stats.ReturnedBytes += int64(returnedBytes)
	stats.GrossRemovedBytes += int64(removedBytes)
	if saved := inputBytes - returnedBytes; saved > 0 {
		stats.NetSavedBytes += int64(saved)
	}
	if stats.InputBytes > 0 {
		stats.NetSavingsPercent = float64(stats.NetSavedBytes) * 100 / float64(stats.InputBytes)
	}
	return writeContextGuardJSON(path, stats)
}

func splitContextBlocks(output string) []string {
	blocks := make([]string, 0, strings.Count(output, "\n\n")+1)
	for len(output) > 0 {
		end := strings.Index(output, "\n\n")
		if end < 0 {
			blocks = append(blocks, output)
			break
		}
		end += 2
		blocks = append(blocks, output[:end])
		output = output[end:]
	}
	return blocks
}

func contextBlockDigest(block string) string {
	digest := sha256.Sum256([]byte(block))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func contextGuardDirectory() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("GREPPLE_CONTEXT_GUARD_DIR")); configured != "" {
		return filepath.Clean(configured), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "context-guard"), nil
}

func contextGuardSessionName(sessionID string) string {
	if sessionID != "" && strings.IndexFunc(sessionID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
	}) < 0 {
		return sessionID
	}
	digest := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(digest[:])
}

func readContextGuardCache(path, sessionID string) contextGuardCache {
	cache := contextGuardCache{Schema: contextGuardSchema, SessionID: sessionID, Entries: map[string]contextGuardEntry{}}
	info, err := os.Stat(path)
	if err != nil || info.Size() > contextGuardMaxCacheSize {
		return cache
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return cache
	}
	var restored contextGuardCache
	if json.Unmarshal(content, &restored) != nil || restored.Schema != contextGuardSchema || restored.SessionID != sessionID || restored.Entries == nil || len(restored.Entries) > contextGuardMaxEntries {
		return cache
	}
	return restored
}

func writeContextGuardCache(path string, cache contextGuardCache) error {
	return writeContextGuardJSON(path, cache)
}

func contextGuardStatsPath(directory, base string, period int) string {
	return filepath.Join(directory, fmt.Sprintf("%s-stats-%d.json", base, period))
}

func latestContextGuardStatsPeriod(directory, base string) int {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return -1
	}
	prefix := base + "-stats-"
	latest := -1
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		periodText := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
		period, err := strconv.Atoi(periodText)
		if err == nil && period >= 0 && period > latest {
			latest = period
		}
	}
	return latest
}

func readContextGuardStats(path, sessionID string, period int) contextGuardStats {
	stats := contextGuardStats{Schema: contextGuardStatsSchema, SessionID: sessionID, Period: period}
	info, err := os.Stat(path)
	if err != nil || info.Size() > contextGuardMaxCacheSize {
		return stats
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return stats
	}
	var restored contextGuardStats
	if json.Unmarshal(content, &restored) != nil || restored.Schema != contextGuardStatsSchema || restored.SessionID != sessionID || restored.Period != period {
		return stats
	}
	return restored
}

func newContextGuardStats(sessionID string, period int, reason string) contextGuardStats {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return contextGuardStats{
		Schema: contextGuardStatsSchema, SessionID: sessionID, Period: period,
		ResetReason: reason, CreatedAt: now, UpdatedAt: now,
	}
}

func writeContextGuardJSON(path string, value any) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".context-guard-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func acquireContextGuardLock(path string) (func(), error) {
	deadline := time.Now().Add(contextGuardLockTimeout)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > contextGuardLockTimeout*2 {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("context guard lock timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func invalidateContextGuardSession(sessionID string) error {
	directory, err := contextGuardDirectory()
	if err != nil {
		return err
	}
	base := contextGuardSessionName(sessionID)
	path := filepath.Join(directory, base+".json")
	if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	release, err := acquireContextGuardLock(path + ".lock")
	if err != nil {
		return err
	}
	defer release()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func rotateContextGuardSession(sessionID, reason string) error {
	directory, err := contextGuardDirectory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	base := contextGuardSessionName(sessionID)
	cachePath := filepath.Join(directory, base+".json")
	release, err := acquireContextGuardLock(cachePath + ".lock")
	if err != nil {
		return err
	}
	defer release()
	if err := os.Remove(cachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	period := latestContextGuardStatsPeriod(directory, base)
	if period < 0 {
		period = 0
		if err := writeContextGuardJSON(contextGuardStatsPath(directory, base, period), newContextGuardStats(sessionID, period, "session-start")); err != nil {
			return err
		}
	}
	next := period + 1
	return writeContextGuardJSON(contextGuardStatsPath(directory, base, next), newContextGuardStats(sessionID, next, reason))
}

func pluralWord(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}
