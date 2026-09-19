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
	contextGuardSchema         = "grepple-context-segments-v2"
	contextGuardStatsSchema    = "grepple-context-stats-v5"
	contextGuardMaxEntries     = 50_000
	contextGuardMaxLineEntries = 200_000
	contextGuardMaxFileSize    = 16 << 20
	contextGuardLockTimeout    = 2 * time.Second
)

type renderedContextCache struct {
	Schema  string                                 `json:"schema"`
	Entries map[string]renderedContextEntry        `json:"entries"`
	Files   map[string]renderedContextFileCoverage `json:"files,omitempty"`
}

type renderedContextFileCoverage struct {
	Lines map[int]string `json:"lines"`
}

type renderedContextEntry struct {
	SourceDigest string `json:"sourceDigest"`
	SourceBytes  int    `json:"sourceBytes"`
}

type renderedContextStats struct {
	Schema             string                   `json:"schema"`
	Period             int                      `json:"period"`
	ResetReason        string                   `json:"resetReason"`
	CreatedAt          string                   `json:"createdAt"`
	UpdatedAt          string                   `json:"updatedAt"`
	TotalBytesReturned int64                    `json:"totalBytesReturned"`
	TotalBytesRemoved  int64                    `json:"totalBytesRemoved"`
	TotalBytesEmitted  int64                    `json:"totalBytesEmitted"`
	SavingsPercent     float64                  `json:"savingsPercent"`
	Calls              renderedContextCallStats `json:"calls"`
	Details            renderedContextDetails   `json:"details"`
}

type renderedContextCallStats struct {
	Total               int `json:"total"`
	Reads               int `json:"reads"`
	StructuredReads     int `json:"structuredReads"`
	FocusedReads        int `json:"focusedReads"`
	OtherReads          int `json:"otherReads"`
	BypassReads         int `json:"bypassReads"`
	PotentialSpillReads int `json:"potentialSpillReads"`
	Writes              int `json:"writes"`
	SuccessfulWrites    int `json:"successfulWrites"`
	AppliedWrites       int `json:"appliedWrites"`
	FailedWrites        int `json:"failedWrites"`
}

type renderedContextDetails struct {
	ResultFiles                   int `json:"resultFiles"`
	SegmentsEmitted               int `json:"segmentsEmitted"`
	SegmentsRemoved               int `json:"segmentsRemoved"`
	SegmentsRemovedByLineCoverage int `json:"segmentsRemovedByLineCoverage"`
	SearchLinesRecorded           int `json:"searchLinesRecorded"`
	LineRangesRemoved             int `json:"lineRangesRemoved"`
	LinesRemoved                  int `json:"linesRemoved"`
	WriteAnchorsRecorded          int `json:"writeAnchorsRecorded"`
}

type legacyRenderedContextStats struct {
	Schema                      string `json:"schema"`
	Period                      int    `json:"period"`
	ResetReason                 string `json:"resetReason"`
	CreatedAt                   string `json:"createdAt"`
	UpdatedAt                   string `json:"updatedAt"`
	ObservedCalls               int    `json:"observedCalls"`
	DeduplicationEnabledCalls   int    `json:"deduplicationEnabledCalls"`
	BypassRequestedCalls        int    `json:"bypassRequestedCalls"`
	NonStructuralCalls          int    `json:"nonStructuralCalls"`
	PotentialSpillCalls         int    `json:"potentialSpillCalls"`
	WriteCalls                  int    `json:"writeCalls"`
	WriteAnchorsRecorded        int    `json:"writeAnchorsRecorded"`
	LineCoverageCalls           int    `json:"lineCoverageCalls"`
	SearchLinesRecorded         int    `json:"searchLinesRecorded"`
	SearchLineBytesEmitted      int64  `json:"searchLineBytesEmitted"`
	LineRangesRemoved           int    `json:"lineRangesRemoved"`
	LinesRemoved                int    `json:"linesRemoved"`
	LineCoverageRemovedSegments int    `json:"lineCoverageRemovedSegments"`
	ResultFiles                 int    `json:"resultFiles"`
	ReturnedBytes               int64  `json:"returnedBytes"`
	EmittedSegments             int    `json:"emittedSegments"`
	RemovedSegments             int    `json:"removedSegments"`
	EmittedSourceBytes          int64  `json:"emittedSourceBytes"`
	NetSavedBytes               int64  `json:"netSavedBytes"`
}

type segmentContextGuard struct {
	directory                   string
	cachePath                   string
	cache                       renderedContextCache
	release                     func()
	observed                    bool
	deduplicate                 bool
	recordSegments              bool
	recordLines                 bool
	cacheDirty                  bool
	writeCall                   bool
	writeApplied                bool
	writeFailed                 bool
	structuredRead              bool
	lineCoverageCall            bool
	bypassRequested             bool
	bypassReason                string
	resultFiles                 int
	returnedBytes               int
	writeAnchorsRecorded        int
	searchLinesRecorded         int
	searchLineBytesEmitted      int
	lineRangesRemoved           int
	linesRemoved                int
	lineBytesRemoved            int
	lineRenderedBytesRemoved    int
	lineMarkerBytes             int
	lineCoverageRemovedSegments int
	emittedSegments             int
	removedSegments             int
	emittedBytes                int
	removedBytes                int
	markerBytes                 int
	lineEntries                 int
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
	cache := readRenderedContextCache(cachePath)
	return &segmentContextGuard{directory: directory, cachePath: cachePath, cache: cache, release: release, deduplicate: true, recordSegments: true, lineEntries: renderedContextLineEntries(cache)}, nil
}

func (guard *segmentContextGuard) close() {
	if guard == nil {
		return
	}
	defer guard.release()
	if !guard.observed {
		return
	}
	if guard.cacheDirty {
		_ = writeRenderedContextJSON(guard.cachePath, guard.cache)
	}
	_ = updateRenderedContextStats(guard)
}

func (guard *segmentContextGuard) seen(source string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment) bool {
	if guard == nil || !guard.deduplicate || !completeStructuralSegment(segment) {
		return false
	}
	guard.observed = true
	key, _, _ := structuralSegmentIdentity(source, artifact, segment)
	_, exact := guard.cache.Entries[key]
	covered := !exact && artifact == nil && guard.segmentCoveredByLines(source, segment)
	if !exact && !covered {
		return false
	}
	if covered {
		guard.lineCoverageRemovedSegments++
	}
	guard.removedSegments++
	guard.removedBytes += len(segment.Text)
	return true
}

func (guard *segmentContextGuard) record(source string, artifact *api.NavigationArtifactIdentity, segment api.ResultSegment) {
	if guard == nil || !guard.recordSegments || !completeStructuralSegment(segment) {
		return
	}
	guard.observed = true
	key, sourceDigest, sourceBytes := structuralSegmentIdentity(source, artifact, segment)
	if len(guard.cache.Entries) < contextGuardMaxEntries {
		guard.cache.Entries[key] = renderedContextEntry{SourceDigest: sourceDigest, SourceBytes: sourceBytes}
		guard.cacheDirty = true
	}
	if artifact == nil {
		guard.recordSegmentLines(source, segment)
	}
	guard.emittedSegments++
	guard.emittedBytes += sourceBytes
}

func (guard *segmentContextGuard) recordMarker(bytes int) {
	if guard != nil {
		guard.markerBytes += bytes
	}
}

type contextSourceLine struct {
	number int
	text   string
}

type contextLineRun struct {
	start int
	end   int
	omit  bool
}

func contextLineRuns(guard *segmentContextGuard, source string, lines []contextSourceLine) []contextLineRun {
	if len(lines) == 0 {
		return nil
	}
	runs := make([]contextLineRun, 0, len(lines))
	start := 0
	covered := guard.lineCovered(source, lines[0].number, lines[0].text)
	for index := 1; index <= len(lines); index++ {
		nextCovered := false
		consecutive := false
		if index < len(lines) {
			nextCovered = guard.lineCovered(source, lines[index].number, lines[index].text)
			consecutive = lines[index].number == lines[index-1].number+1
		}
		if index < len(lines) && nextCovered == covered && consecutive {
			continue
		}
		runs = append(runs, contextLineRun{start: start, end: index, omit: covered && index-start >= 6})
		start, covered = index, nextCovered
	}
	return runs
}

func (guard *segmentContextGuard) lineCovered(source string, line int, content string) bool {
	if guard == nil || !guard.deduplicate || line < 1 {
		return false
	}
	path, ok := localContextSourcePath(source)
	if !ok {
		return false
	}
	coverage, ok := guard.cache.Files[path]
	return ok && coverage.Lines[line] == contextLineDigest(content)
}

func (guard *segmentContextGuard) recordSearchLine(source string, line int, content string) {
	if guard == nil || !guard.recordLines {
		return
	}
	guard.searchLineBytesEmitted += len(content)
	path, ok := localContextSourcePath(source)
	if !ok {
		return
	}
	if guard.recordContextLine(path, line, content) {
		guard.searchLinesRecorded++
	}
}

func (guard *segmentContextGuard) recordLineRangeOmission(lines, sourceBytes, renderedBytes, markerBytes int) {
	if guard == nil || lines < 1 {
		return
	}
	guard.lineRangesRemoved++
	guard.linesRemoved += lines
	guard.lineBytesRemoved += sourceBytes
	guard.lineRenderedBytesRemoved += renderedBytes
	guard.lineMarkerBytes += markerBytes
}

func (guard *segmentContextGuard) segmentCoveredByLines(source string, segment api.ResultSegment) bool {
	path, ok := localContextSourcePath(source)
	if !ok {
		return false
	}
	coverage, ok := guard.cache.Files[path]
	if !ok {
		return false
	}
	for index, line := range strings.Split(segment.Text, "\n") {
		if coverage.Lines[segment.Start+index] != contextLineDigest(line) {
			return false
		}
	}
	return true
}

func (guard *segmentContextGuard) recordSegmentLines(source string, segment api.ResultSegment) {
	path, ok := localContextSourcePath(source)
	if !ok {
		return
	}
	for index, line := range strings.Split(segment.Text, "\n") {
		guard.recordContextLine(path, segment.Start+index, line)
	}
}

func (guard *segmentContextGuard) recordWriteAnchors(source string, anchors []writeAnchor) {
	path, ok := localContextSourcePath(source)
	if !ok {
		return
	}
	for _, anchor := range anchors {
		if anchor.Line < 1 {
			continue
		}
		guard.writeAnchorsRecorded++
		guard.recordContextLine(path, anchor.Line, anchor.Content)
	}
}

func (guard *segmentContextGuard) recordContextLine(path string, line int, content string) bool {
	if line < 1 {
		return false
	}
	coverage, ok := guard.cache.Files[path]
	if !ok {
		if guard.lineEntries >= contextGuardMaxLineEntries {
			return false
		}
		coverage = renderedContextFileCoverage{Lines: map[int]string{}}
	}
	digest := contextLineDigest(content)
	if coverage.Lines[line] == digest {
		return false
	}
	if _, exists := coverage.Lines[line]; !exists {
		if guard.lineEntries >= contextGuardMaxLineEntries {
			return false
		}
		guard.lineEntries++
	}
	coverage.Lines[line] = digest
	guard.cache.Files[path] = coverage
	guard.cacheDirty = true
	return true
}

func (guard *segmentContextGuard) removeContextFile(source string) {
	path, ok := localContextSourcePath(source)
	if !ok {
		return
	}
	if coverage, exists := guard.cache.Files[path]; exists {
		guard.lineEntries -= len(coverage.Lines)
		delete(guard.cache.Files, path)
		guard.cacheDirty = true
	}
}

func recordWriteResponseContext(root string, response writeResponse, returnedBytes int, applied, failed, recordAnchors bool) {
	if !contextGuardEnabled() {
		return
	}
	guard, err := openSegmentContextGuard()
	if err != nil {
		return
	}
	guard.observed = true
	guard.deduplicate = false
	guard.recordSegments = false
	guard.writeCall = true
	guard.writeApplied = applied
	guard.writeFailed = failed
	guard.resultFiles = len(response.Files)
	guard.returnedBytes = returnedBytes
	defer guard.close()
	if !recordAnchors || activeInlineOutputThreshold < 1 || returnedBytes > activeInlineOutputThreshold {
		return
	}
	resolvedRoot, err := resolveWriteRoot(root)
	if err != nil {
		return
	}
	for _, file := range response.Files {
		if !file.Changed {
			continue
		}
		source := file.Path
		if !filepath.IsAbs(source) {
			source = filepath.Join(resolvedRoot, source)
		}
		if file.Operation == "delete" {
			guard.removeContextFile(source)
			continue
		}
		guard.recordWriteAnchors(source, responseFileAnchors(file))
	}
}

func localContextSourcePath(source string) (string, bool) {
	if source == "" || strings.ContainsRune(source, '\x00') {
		return "", false
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return "", false
	}
	return filepath.Clean(absolute), true
}

func contextLineDigest(content string) string {
	digest := sha256.Sum256([]byte(normalizeRenderedAnchorLine(content)))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func renderedContextLineEntries(cache renderedContextCache) int {
	total := 0
	for _, coverage := range cache.Files {
		total += len(coverage.Lines)
	}
	return total
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
	fresh := renderedContextCache{Schema: contextGuardSchema, Entries: map[string]renderedContextEntry{}, Files: map[string]renderedContextFileCoverage{}}
	content, ok := readBoundedContextFile(path)
	if !ok {
		return fresh
	}
	var restored renderedContextCache
	if json.Unmarshal(content, &restored) != nil || (restored.Schema != contextGuardSchema && restored.Schema != "grepple-context-segments-v1") || restored.Entries == nil || len(restored.Entries) > contextGuardMaxEntries {
		return fresh
	}
	if restored.Files == nil {
		restored.Files = map[string]renderedContextFileCoverage{}
	}
	for path, coverage := range restored.Files {
		if coverage.Lines == nil {
			coverage.Lines = map[int]string{}
			restored.Files[path] = coverage
		}
	}
	if renderedContextLineEntries(restored) > contextGuardMaxLineEntries {
		return fresh
	}
	restored.Schema = contextGuardSchema
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
	stats.Calls.Total++
	stats.TotalBytesReturned += int64(guard.returnedBytes)
	stats.TotalBytesEmitted += int64(guard.emittedBytes + guard.searchLineBytesEmitted)
	stats.Details.ResultFiles += guard.resultFiles
	if guard.writeCall {
		stats.Calls.Writes++
		if guard.writeFailed {
			stats.Calls.FailedWrites++
		} else {
			stats.Calls.SuccessfulWrites++
		}
		if guard.writeApplied {
			stats.Calls.AppliedWrites++
		}
	} else {
		stats.Calls.Reads++
		switch {
		case guard.structuredRead:
			stats.Calls.StructuredReads++
		case guard.lineCoverageCall:
			stats.Calls.FocusedReads++
		default:
			stats.Calls.OtherReads++
		}
		if guard.bypassRequested {
			stats.Calls.BypassReads++
		}
		if guard.bypassReason == "potential-spill" {
			stats.Calls.PotentialSpillReads++
		}
	}
	stats.Details.WriteAnchorsRecorded += guard.writeAnchorsRecorded
	stats.Details.SearchLinesRecorded += guard.searchLinesRecorded
	stats.Details.LineRangesRemoved += guard.lineRangesRemoved
	stats.Details.LinesRemoved += guard.linesRemoved
	stats.Details.SegmentsRemovedByLineCoverage += guard.lineCoverageRemovedSegments
	stats.Details.SegmentsEmitted += guard.emittedSegments
	stats.Details.SegmentsRemoved += guard.removedSegments
	saved := guard.removedBytes - guard.markerBytes + guard.lineRenderedBytesRemoved - guard.lineMarkerBytes
	if saved > 0 {
		stats.TotalBytesRemoved += int64(saved)
	}
	if stats.TotalBytesReturned > 0 {
		stats.SavingsPercent = float64(stats.TotalBytesRemoved) * 100 / float64(stats.TotalBytesReturned)
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
	var identity struct {
		Schema string `json:"schema"`
		Period int    `json:"period"`
	}
	if json.Unmarshal(content, &identity) != nil || identity.Period != period {
		return fresh
	}
	if identity.Schema == contextGuardStatsSchema {
		var restored renderedContextStats
		if json.Unmarshal(content, &restored) == nil {
			return restored
		}
		return fresh
	}
	if !legacyContextStatsSchema(identity.Schema) {
		return fresh
	}
	var legacy legacyRenderedContextStats
	if json.Unmarshal(content, &legacy) != nil {
		return fresh
	}
	return migrateLegacyContextStats(legacy)
}

func legacyContextStatsSchema(schema string) bool {
	switch schema {
	case "grepple-context-segment-stats-v1", "grepple-context-segment-stats-v2", "grepple-context-segment-stats-v3", "grepple-context-segment-stats-v4":
		return true
	default:
		return false
	}
}

func migrateLegacyContextStats(legacy legacyRenderedContextStats) renderedContextStats {
	reads := legacy.ObservedCalls - legacy.WriteCalls
	if reads < 0 {
		reads = 0
	}
	structured := legacy.DeduplicationEnabledCalls - legacy.LineCoverageCalls
	other := reads - structured - legacy.LineCoverageCalls
	if structured < 0 {
		structured = 0
	}
	if other < 0 {
		other = 0
	}
	stats := renderedContextStats{
		Schema:             contextGuardStatsSchema,
		Period:             legacy.Period,
		ResetReason:        legacy.ResetReason,
		CreatedAt:          legacy.CreatedAt,
		UpdatedAt:          legacy.UpdatedAt,
		TotalBytesReturned: legacy.ReturnedBytes,
		TotalBytesRemoved:  legacy.NetSavedBytes,
		TotalBytesEmitted:  legacy.EmittedSourceBytes + legacy.SearchLineBytesEmitted,
		Calls: renderedContextCallStats{
			Total:               legacy.ObservedCalls,
			Reads:               reads,
			StructuredReads:     structured,
			FocusedReads:        legacy.LineCoverageCalls,
			OtherReads:          other,
			BypassReads:         legacy.BypassRequestedCalls,
			PotentialSpillReads: legacy.PotentialSpillCalls,
			Writes:              legacy.WriteCalls,
			SuccessfulWrites:    legacy.WriteCalls,
			AppliedWrites:       legacy.WriteCalls,
		},
		Details: renderedContextDetails{
			ResultFiles:                   legacy.ResultFiles,
			SegmentsEmitted:               legacy.EmittedSegments,
			SegmentsRemoved:               legacy.RemovedSegments,
			SegmentsRemovedByLineCoverage: legacy.LineCoverageRemovedSegments,
			SearchLinesRecorded:           legacy.SearchLinesRecorded,
			LineRangesRemoved:             legacy.LineRangesRemoved,
			LinesRemoved:                  legacy.LinesRemoved,
			WriteAnchorsRecorded:          legacy.WriteAnchorsRecorded,
		},
	}
	if stats.TotalBytesReturned > 0 {
		stats.SavingsPercent = float64(stats.TotalBytesRemoved) * 100 / float64(stats.TotalBytesReturned)
	}
	return stats
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
