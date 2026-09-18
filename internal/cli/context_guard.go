package cli

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
)

const (
	contextGuardSchema       = "grepple-context-guard-v2"
	contextGuardStatsSchema  = "grepple-context-guard-stats-v2"
	contextGuardMinBlockSize = 128
	contextGuardMaxEntries   = 50_000
	contextGuardMaxFileSize  = 16 << 20
	contextGuardMaxOutput    = 64 << 20
	contextGuardLockTimeout  = 2 * time.Second
)

type renderedContextCache struct {
	Schema  string                          `json:"schema"`
	Entries map[string]renderedContextEntry `json:"entries"`
}

type renderedContextEntry struct {
	Bytes int `json:"bytes"`
}

type renderedContextStats struct {
	Schema            string  `json:"schema"`
	Period            int     `json:"period"`
	ResetReason       string  `json:"resetReason"`
	CreatedAt         string  `json:"createdAt"`
	UpdatedAt         string  `json:"updatedAt"`
	ObservedCalls     int     `json:"observedCalls"`
	ObservedBlocks    int     `json:"observedBlocks"`
	NewBlocks         int     `json:"newBlocks"`
	RemovedBlocks     int     `json:"removedBlocks"`
	InputBytes        int64   `json:"inputBytes"`
	ReturnedBytes     int64   `json:"returnedBytes"`
	GrossRemovedBytes int64   `json:"grossRemovedBytes"`
	NetSavedBytes     int64   `json:"netSavedBytes"`
	NetSavingsPercent float64 `json:"netSavingsPercent"`
}

func runContext(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return stdoutWriter().writeString("Manage session-agnostic rendered-context deduplication.\nUsage:\n  grepple context invalidate [--reason REASON]\n")
	}
	if len(args) == 0 || args[0] != "invalidate" {
		return fmt.Errorf("usage: grepple context invalidate [--reason REASON]")
	}
	reason := "manual"
	for index := 1; index < len(args); index++ {
		switch {
		case args[index] == "--reason" && index+1 < len(args):
			reason = strings.TrimSpace(args[index+1])
			index++
		case strings.HasPrefix(args[index], "--reason="):
			reason = strings.TrimSpace(strings.TrimPrefix(args[index], "--reason="))
		default:
			return fmt.Errorf("unknown context invalidate argument %q", args[index])
		}
	}
	if reason == "" {
		return fmt.Errorf("--reason requires a value")
	}
	return invalidateRenderedContext(reason)
}

func guardRenderedOutputFile(path string, outputJSON bool) ([]byte, error) {
	if outputJSON {
		return os.ReadFile(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > contextGuardMaxOutput {
		return os.ReadFile(path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	guarded, err := guardRenderedOutput(content)
	if err != nil {
		return content, nil
	}
	return guarded, nil
}

type renderedContextObservation struct {
	blocks        []string
	eligible      int
	newBlocks     int
	removedBlocks int
	removedBytes  int
}

func guardRenderedOutput(content []byte) ([]byte, error) {
	blocks := splitRenderedContextBlocks(string(content))
	eligible := countRenderedSourceBlocks(blocks)
	if eligible == 0 {
		return content, nil
	}
	directory, err := renderedContextDirectory()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	cachePath := filepath.Join(directory, "cache.json")
	release, err := acquireRenderedContextLock(cachePath + ".lock")
	if err != nil {
		return nil, err
	}
	defer release()
	cache := readRenderedContextCache(cachePath)
	observation := observeRenderedContext(blocks, &cache)
	if err := writeRenderedContextJSON(cachePath, cache); err != nil {
		return nil, err
	}
	result := renderContextObservation(observation)
	_ = updateRenderedContextStats(directory, eligible, observation.newBlocks, observation.removedBlocks, len(content), len(result), observation.removedBytes)
	return []byte(result), nil
}

func countRenderedSourceBlocks(blocks []string) int {
	count := 0
	for _, block := range blocks {
		if renderedSourceBlock(block) {
			count++
		}
	}
	return count
}

func observeRenderedContext(blocks []string, cache *renderedContextCache) renderedContextObservation {
	observation := renderedContextObservation{blocks: make([]string, 0, len(blocks)+1)}
	for _, block := range blocks {
		if !renderedSourceBlock(block) {
			observation.blocks = append(observation.blocks, block)
			continue
		}
		digest := renderedContextDigest(block)
		if _, seen := cache.Entries[digest]; seen {
			observation.removedBlocks++
			observation.removedBytes += len(block)
			continue
		}
		observation.blocks = append(observation.blocks, block)
		observation.newBlocks++
		if len(cache.Entries) < contextGuardMaxEntries {
			cache.Entries[digest] = renderedContextEntry{Bytes: len(block)}
		}
	}
	observation.eligible = observation.newBlocks + observation.removedBlocks
	return observation
}

func renderContextObservation(observation renderedContextObservation) string {
	result := strings.Join(observation.blocks, "")
	if observation.removedBlocks == 0 {
		return result
	}
	marker := fmt.Sprintf("[grepple context guard: omitted %d unchanged source %s (%d bytes) already emitted since the last context invalidation]\n", observation.removedBlocks, pluralizeContextWord("block", observation.removedBlocks), observation.removedBytes)
	return marker + result
}

func renderedSourceBlock(block string) bool {
	return len(strings.TrimSpace(block)) >= contextGuardMinBlockSize && renderedBlockContainsSource(block)
}

func splitRenderedContextBlocks(output string) []string {
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

func renderedBlockContainsSource(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		first := strings.Index(line, "│")
		if first < 1 {
			continue
		}
		second := strings.Index(line[first+len("│"):], "│")
		if second < 1 {
			continue
		}
		lineNumber := line[first+len("│") : first+len("│")+second]
		if _, err := strconv.Atoi(lineNumber); err == nil {
			return true
		}
	}
	return false
}

func renderedContextDirectory() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("GREPPLE_CONTEXT_GUARD_DIR")); configured != "" {
		return filepath.Clean(configured), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "context-guard"), nil
}

func renderedContextDigest(block string) string {
	digest := sha256.Sum256([]byte(block))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func readRenderedContextCache(path string) renderedContextCache {
	fresh := renderedContextCache{Schema: contextGuardSchema, Entries: map[string]renderedContextEntry{}}
	content, ok := readBoundedContextFile(path)
	if !ok {
		return fresh
	}
	var restored renderedContextCache
	if json.Unmarshal(content, &restored) != nil || restored.Schema != contextGuardSchema || restored.Entries == nil || len(restored.Entries) > contextGuardMaxEntries {
		return fresh
	}
	return restored
}

func updateRenderedContextStats(directory string, observed, added, removed, inputBytes, returnedBytes, removedBytes int) error {
	period := latestRenderedContextStatsPeriod(directory)
	if period < 0 {
		period = 0
	}
	path := renderedContextStatsPath(directory, period)
	stats := readRenderedContextStats(path, period)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if stats.CreatedAt == "" {
		stats.CreatedAt, stats.ResetReason = now, "session-start"
	}
	stats.UpdatedAt = now
	stats.ObservedCalls++
	stats.ObservedBlocks += observed
	stats.NewBlocks += added
	stats.RemovedBlocks += removed
	stats.InputBytes += int64(inputBytes)
	stats.ReturnedBytes += int64(returnedBytes)
	stats.GrossRemovedBytes += int64(removedBytes)
	if saved := inputBytes - returnedBytes; saved > 0 {
		stats.NetSavedBytes += int64(saved)
	}
	if stats.InputBytes > 0 {
		stats.NetSavingsPercent = float64(stats.NetSavedBytes) * 100 / float64(stats.InputBytes)
	}
	return writeRenderedContextJSON(path, stats)
}

func invalidateRenderedContext(reason string) error {
	directory, err := renderedContextDirectory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	cachePath := filepath.Join(directory, "cache.json")
	release, err := acquireRenderedContextLock(cachePath + ".lock")
	if err != nil {
		return err
	}
	defer release()
	if err := os.Remove(cachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	period := latestRenderedContextStatsPeriod(directory)
	if period < 0 {
		return writeRenderedContextJSON(renderedContextStatsPath(directory, 0), newRenderedContextStats(0, reason))
	}
	next := period + 1
	return writeRenderedContextJSON(renderedContextStatsPath(directory, next), newRenderedContextStats(next, reason))
}

func renderedContextStatsPath(directory string, period int) string {
	return filepath.Join(directory, fmt.Sprintf("stats-%d.json", period))
}

func latestRenderedContextStatsPeriod(directory string) int {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return -1
	}
	latest := -1
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "stats-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		period, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "stats-"), ".json"))
		if err == nil && period >= 0 && period > latest {
			latest = period
		}
	}
	return latest
}

func readRenderedContextStats(path string, period int) renderedContextStats {
	fresh := renderedContextStats{Schema: contextGuardStatsSchema, Period: period}
	content, ok := readBoundedContextFile(path)
	if !ok {
		return fresh
	}
	var restored renderedContextStats
	if json.Unmarshal(content, &restored) != nil || restored.Schema != contextGuardStatsSchema || restored.Period != period {
		return fresh
	}
	return restored
}

func newRenderedContextStats(period int, reason string) renderedContextStats {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return renderedContextStats{Schema: contextGuardStatsSchema, Period: period, ResetReason: reason, CreatedAt: now, UpdatedAt: now}
}

func readBoundedContextFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() > contextGuardMaxFileSize {
		return nil, false
	}
	content, err := os.ReadFile(path)
	return content, err == nil
}

func writeRenderedContextJSON(path string, value any) error {
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

func acquireRenderedContextLock(path string) (func(), error) {
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

func pluralizeContextWord(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}
