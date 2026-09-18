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

	"github.com/greppleai/grepple/api"
)

const (
	contextGuardSchema      = "grepple-context-segments-v1"
	contextGuardStatsSchema = "grepple-context-segment-stats-v1"
	contextGuardMaxEntries  = 50_000
	contextGuardMaxFileSize = 16 << 20
	contextGuardLockTimeout = 2 * time.Second
)

type renderedContextCache struct {
	Schema  string                          `json:"schema"`
	Entries map[string]renderedContextEntry `json:"entries"`
}

type renderedContextEntry struct {
	SourceDigest string `json:"sourceDigest"`
	SourceBytes  int    `json:"sourceBytes"`
}

type renderedContextStats struct {
	Schema             string  `json:"schema"`
	Period             int     `json:"period"`
	ResetReason        string  `json:"resetReason"`
	CreatedAt          string  `json:"createdAt"`
	UpdatedAt          string  `json:"updatedAt"`
	ObservedCalls      int     `json:"observedCalls"`
	EmittedSegments    int     `json:"emittedSegments"`
	RemovedSegments    int     `json:"removedSegments"`
	EmittedSourceBytes int64   `json:"emittedSourceBytes"`
	GrossRemovedBytes  int64   `json:"grossRemovedBytes"`
	NetSavedBytes      int64   `json:"netSavedBytes"`
	NetSavingsPercent  float64 `json:"netSavingsPercent"`
}

type segmentContextGuard struct {
	directory       string
	cachePath       string
	cache           renderedContextCache
	release         func()
	observed        bool
	emittedSegments int
	removedSegments int
	emittedBytes    int
	removedBytes    int
	markerBytes     int
}

func runContext(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return stdoutWriter().writeString("Manage session-agnostic structural-segment context deduplication.\nUsage:\n  grepple context invalidate [--reason REASON]\n")
	}
	if len(args) == 0 || args[0] != "invalidate" {
		return fmt.Errorf("usage: grepple context invalidate [--reason REASON]")
	}
	reason, err := parseContextInvalidationReason(args[1:])
	if err != nil {
		return err
	}
	return invalidateRenderedContext(reason)
}

func parseContextInvalidationReason(args []string) (string, error) {
	reason := "manual"
	for index := 0; index < len(args); index++ {
		switch {
		case args[index] == "--reason" && index+1 < len(args):
			reason = strings.TrimSpace(args[index+1])
			index++
		case strings.HasPrefix(args[index], "--reason="):
			reason = strings.TrimSpace(strings.TrimPrefix(args[index], "--reason="))
		default:
			return "", fmt.Errorf("unknown context invalidate argument %q", args[index])
		}
	}
	if reason == "" {
		return "", fmt.Errorf("--reason requires a value")
	}
	return reason, nil
}

func openSegmentContextGuard() (*segmentContextGuard, error) {
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
	return &segmentContextGuard{directory: directory, cachePath: cachePath, cache: readRenderedContextCache(cachePath), release: release}, nil
}

func (guard *segmentContextGuard) close() {
	if guard == nil {
		return
	}
	defer guard.release()
	if !guard.observed {
		return
	}
	if writeRenderedContextJSON(guard.cachePath, guard.cache) != nil {
		return
	}
	_ = updateRenderedContextStats(guard)
}

func (guard *segmentContextGuard) seen(source string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment) bool {
	if guard == nil || !completeStructuralSegment(segment) {
		return false
	}
	guard.observed = true
	key, _, _ := structuralSegmentIdentity(source, artifact, segment)
	if _, exists := guard.cache.Entries[key]; !exists {
		return false
	}
	guard.removedSegments++
	guard.removedBytes += len(segment.Text)
	return true
}

func (guard *segmentContextGuard) record(source string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment) {
	if guard == nil || !completeStructuralSegment(segment) {
		return
	}
	guard.observed = true
	key, sourceDigest, sourceBytes := structuralSegmentIdentity(source, artifact, segment)
	if len(guard.cache.Entries) < contextGuardMaxEntries {
		guard.cache.Entries[key] = renderedContextEntry{SourceDigest: sourceDigest, SourceBytes: sourceBytes}
	}
	guard.emittedSegments++
	guard.emittedBytes += sourceBytes
}

func (guard *segmentContextGuard) recordMarker(bytes int) {
	if guard != nil {
		guard.markerBytes += bytes
	}
}

func completeStructuralSegment(segment api.ResultSegment) bool {
	if segment.Kind == "" || segment.Kind == "spacing" || segment.Kind == "summary" || segment.Start < 1 || segment.End < segment.Start || segment.Text == "" {
		return false
	}
	return strings.Count(segment.Text, "\n")+1 == segment.End-segment.Start+1
}

func structuralSegmentIdentity(source string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment) (string, string, int) {
	sourceDigestBytes := sha256.Sum256([]byte(segment.Text))
	sourceDigest := hex.EncodeToString(sourceDigestBytes[:])
	artifactIdentity := "local"
	if artifact != nil {
		artifactIdentity = strings.Join([]string{artifact.Digest, artifact.Repository, artifact.Commit, artifact.Module, artifact.Version, artifact.RefKind}, "\x00")
	}
	identity := strings.Join([]string{artifactIdentity, source, segment.Kind, strconv.Itoa(segment.Start), strconv.Itoa(segment.End), sourceDigest}, "\x00")
	identityDigest := sha256.Sum256([]byte(identity))
	return "sha256:" + hex.EncodeToString(identityDigest[:]), "sha256:" + sourceDigest, len(segment.Text)
}

func segmentContextMarker(source string, segment api.ResultSegment, contextLabel string) string {
	if contextLabel != "" {
		return fmt.Sprintf("// … unchanged %s already emitted at %s:%d-%d (%d source bytes) …\n", contextLabel, source, segment.Start, segment.End, len(segment.Text))
	}
	return fmt.Sprintf("// … unchanged segment already emitted: %s:%d-%d (%d source bytes) …\n", source, segment.Start, segment.End, len(segment.Text))
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

func updateRenderedContextStats(guard *segmentContextGuard) error {
	period := latestRenderedContextStatsPeriod(guard.directory)
	if period < 0 {
		period = 0
	}
	path := renderedContextStatsPath(guard.directory, period)
	stats := readRenderedContextStats(path, period)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if stats.CreatedAt == "" {
		stats.CreatedAt, stats.ResetReason = now, "session-start"
	}
	stats.UpdatedAt = now
	stats.ObservedCalls++
	stats.EmittedSegments += guard.emittedSegments
	stats.RemovedSegments += guard.removedSegments
	stats.EmittedSourceBytes += int64(guard.emittedBytes)
	stats.GrossRemovedBytes += int64(guard.removedBytes)
	if saved := guard.removedBytes - guard.markerBytes; saved > 0 {
		stats.NetSavedBytes += int64(saved)
	}
	totalSource := stats.EmittedSourceBytes + stats.GrossRemovedBytes
	if totalSource > 0 {
		stats.NetSavingsPercent = float64(stats.NetSavedBytes) * 100 / float64(totalSource)
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
