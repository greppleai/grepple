package metrics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var shellSegments = regexp.MustCompile(`[;&|()\n]+`)

var exactTestExecutables = map[string]bool{
	"go": true, "cargo": true, "mvn": true, "gradle": true,
}

var alwaysAmbiguousExecutables = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "python": true, "python3": true,
	"ruby": true, "node": true, "perl": true, "rm": true, "mv": true,
	"cp": true, "install": true, "touch": true, "truncate": true, "tee": true,
	"patch": true, "sed": true, "make": true, "npm": true, "pnpm": true, "yarn": true,
}

var ambiguousGitSubcommands = map[string]bool{
	"apply": true, "checkout": true, "restore": true, "reset": true, "clean": true,
	"merge": true, "rebase": true, "cherry-pick": true,
}

var ambiguousGoSubcommands = map[string]bool{"generate": true, "fmt": true}

var ambiguousCargoSubcommands = map[string]bool{"fmt": true, "fix": true, "run": true}

type rawUsage struct {
	Input       int64 `json:"input"`
	Output      int64 `json:"output"`
	CacheRead   int64 `json:"cacheRead"`
	CacheWrite  int64 `json:"cacheWrite"`
	TotalTokens int64 `json:"totalTokens"`
	Cost        struct {
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cacheRead"`
		CacheWrite float64 `json:"cacheWrite"`
		Total      float64 `json:"total"`
	} `json:"cost"`
}

type rawMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	Usage      rawUsage        `json:"usage"`
	StopReason string          `json:"stopReason"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Details    json.RawMessage `json:"details"`
	IsError    bool            `json:"isError"`
	Timestamp  int64           `json:"timestamp"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type annotation struct {
	Schema             int      `json:"schema"`
	Event              string   `json:"event"`
	RunID              string   `json:"runId"`
	TaskID             string   `json:"taskId"`
	Repository         string   `json:"repository"`
	Revision           string   `json:"revision"`
	AssignedCohort     string   `json:"assignedCohort"`
	Outcome            string   `json:"outcome"`
	HumanInterventions *int     `json:"humanInterventions"`
	EvaluatorScore     *float64 `json:"evaluatorScore"`
	Rubric             string   `json:"rubric"`
	Regressions        *int     `json:"regressions"`
	FirstEditSurvived  *bool    `json:"firstEditSurvived"`
}

type callKind struct {
	navigation bool
	read       bool
	mutation   bool
	test       bool
	verify     bool
	grepple    bool
	ambiguous  bool
	mode       string
}

type analyzedCall struct {
	id                 string
	name               string
	args               json.RawMessage
	kind               callKind
	paths              []string
	signature          string
	turn               int
	ordinal            int
	when               time.Time
	usage              Usage
	success            bool
	resolved           bool
	zero               bool
	bytes              int
	lines              int
	added              int
	removed            int
	churnKnown         bool
	oldHash            string
	newHash            string
	transitionRecorded bool
}

type runSegment struct {
	entries  []entry
	start    annotation
	end      annotation
	inferred bool
}

type segmentAnalysis struct {
	run                 *Run
	calls               []*analyzedCall
	byID                map[string]*analyzedCall
	inspected           map[string]bool
	edited              map[string]bool
	seenSignatures      map[string]bool
	seenReads           map[string]map[string]bool
	cumulative          Usage
	firstEvent          time.Time
	lastEvent           time.Time
	turn                int
	callOrdinal         int
	failedTestSinceEdit bool
	priorEditContents   map[string]map[string]bool
}

// AnalyzeFile analyzes the selected root-to-leaf branch of one Pi session.
// Passing an empty leaf selects the most recently appended entry.
func AnalyzeFile(path, leaf string) ([]Run, error) {
	branch, err := readSession(path, leaf)
	if err != nil {
		return nil, err
	}
	segments, err := splitRuns(branch.Entries)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	result := make([]Run, 0, len(segments))
	for index, segment := range segments {
		run, err := analyzeSegment(branch.Header, segment, index)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		result = append(result, run)
	}
	return result, nil
}

func splitRuns(entries []entry) ([]runSegment, error) {
	var segments []runSegment
	var current *runSegment
	foundMarker := false
	for _, value := range entries {
		marker, ok, err := parseAnnotation(value)
		if err != nil {
			return nil, err
		}
		if isRunStart(ok, marker) {
			foundMarker = true
			segments, current = beginRunSegment(segments, current, marker, value)
			continue
		}
		if current == nil {
			continue
		}
		current.entries = append(current.entries, value)
		if !isMatchingRunEnd(ok, marker, current.start.RunID) {
			continue
		}
		current.end = marker
		segments = append(segments, *current)
		current = nil
	}
	if current != nil {
		segments = append(segments, *current)
	}
	if !foundMarker {
		return []runSegment{{entries: entries, inferred: true}}, nil
	}
	return segments, nil
}

func isRunStart(ok bool, marker annotation) bool {
	return ok && marker.Event == "run_start"
}

func beginRunSegment(segments []runSegment, current *runSegment, marker annotation, value entry) ([]runSegment, *runSegment) {
	if current != nil {
		segments = append(segments, *current)
	}
	return segments, &runSegment{start: marker, entries: []entry{value}}
}

func isMatchingRunEnd(ok bool, marker annotation, runID string) bool {
	return ok && marker.Event == "run_end" && (marker.RunID == "" || marker.RunID == runID)
}

func parseAnnotation(value entry) (annotation, bool, error) {
	if value.Type != "custom" || value.CustomType != AnnotationType {
		return annotation{}, false, nil
	}
	var marker annotation
	if err := json.Unmarshal(value.Data, &marker); err != nil {
		return annotation{}, false, fmt.Errorf("annotation %q: %w", value.ID, err)
	}
	if marker.Schema != 1 {
		return annotation{}, false, fmt.Errorf("annotation %q has unsupported schema %d", value.ID, marker.Schema)
	}
	return marker, true, nil
}

func analyzeSegment(header sessionHeader, segment runSegment, index int) (Run, error) {
	run := initialRun(header, segment, index)
	analysis := newSegmentAnalysis(&run)
	for _, value := range segment.entries {
		if err := analysis.processEntry(value); err != nil {
			return Run{}, err
		}
	}
	analysis.finalize()
	return run, nil
}

func initialRun(header sessionHeader, segment runSegment, index int) Run {
	run := Run{
		Schema:         "grepple-agent-metrics-v1",
		SessionID:      header.ID,
		RunID:          segment.start.RunID,
		TaskID:         segment.start.TaskID,
		Repository:     segment.start.Repository,
		Revision:       segment.start.Revision,
		AssignedCohort: segment.start.AssignedCohort,
		GreppleModes:   make(map[string]int),
		Inferred:       segment.inferred,
		Complete:       segment.end.Event == "run_end",
		Outcome:        Outcome{Status: "unknown"},
		Missing:        make([]string, 0),
	}
	if run.RunID == "" {
		run.RunID = fmt.Sprintf("%s:%d", header.ID, index+1)
	}
	if run.Repository == "" {
		run.Repository = filepath.Base(header.CWD)
	}
	if segment.end.Event == "run_end" {
		applyOutcome(&run, segment.end)
	}
	if segment.inferred {
		run.Missing = append(run.Missing, "explicit_run_boundaries")
	}
	if !run.Complete {
		run.Missing = append(run.Missing, "run_end")
	}
	return run
}

func newSegmentAnalysis(run *Run) *segmentAnalysis {
	return &segmentAnalysis{
		run:               run,
		calls:             make([]*analyzedCall, 0),
		byID:              make(map[string]*analyzedCall),
		inspected:         make(map[string]bool),
		edited:            make(map[string]bool),
		seenSignatures:    make(map[string]bool),
		seenReads:         make(map[string]map[string]bool),
		priorEditContents: make(map[string]map[string]bool),
	}
}

func (analysis *segmentAnalysis) processEntry(value entry) error {
	when := entryTime(value)
	analysis.observeTime(when)
	switch value.Type {
	case "compaction":
		analysis.run.Compactions++
	case "model_change":
		analysis.run.Provider = value.Provider
		analysis.run.Model = value.ModelID
	case "thinking_level_change":
		analysis.run.ThinkingLevel = value.ThinkingLevel
	case "custom":
		return analysis.processCustom(value)
	case "message":
		return analysis.processMessage(value, when)
	}
	return nil
}

func (analysis *segmentAnalysis) observeTime(when time.Time) {
	if analysis.firstEvent.IsZero() || (!when.IsZero() && when.Before(analysis.firstEvent)) {
		analysis.firstEvent = when
	}
	if when.After(analysis.lastEvent) {
		analysis.lastEvent = when
	}
}

func (analysis *segmentAnalysis) processCustom(value entry) error {
	marker, ok, err := parseAnnotation(value)
	if err != nil {
		return err
	}
	if ok && marker.Event == "run_end" {
		applyOutcome(analysis.run, marker)
	}
	return nil
}

func (analysis *segmentAnalysis) processMessage(value entry, when time.Time) error {
	var message rawMessage
	if err := json.Unmarshal(value.Message, &message); err != nil {
		return fmt.Errorf("message %q: %w", value.ID, err)
	}
	if message.Timestamp > 0 {
		when = time.UnixMilli(message.Timestamp)
	}
	switch message.Role {
	case "assistant":
		return analysis.processAssistant(value.ID, message, when)
	case "toolResult":
		return analysis.processToolResult(value.ID, message, when)
	default:
		return nil
	}
}

func (analysis *segmentAnalysis) processAssistant(entryID string, message rawMessage, when time.Time) error {
	analysis.turn++
	analysis.run.Turns++
	usage := convertUsage(message.Usage)
	analysis.cumulative.add(usage)
	analysis.run.Usage.add(usage)
	if message.Provider != "" {
		analysis.run.Provider = message.Provider
	}
	if message.Model != "" {
		analysis.run.Model = message.Model
	}
	if message.StopReason == "error" || message.StopReason == "aborted" {
		analysis.run.Retries++
	}
	blocks, err := decodeBlocks(message.Content)
	if err != nil {
		return fmt.Errorf("assistant message %q: %w", entryID, err)
	}
	for _, block := range blocks {
		if block.Type == "toolCall" {
			analysis.recordCall(block, when)
		}
	}
	return nil
}

func (analysis *segmentAnalysis) recordCall(block contentBlock, when time.Time) {
	analysis.callOrdinal++
	kind := classifyCall(block.Name, block.Arguments)
	paths := extractPaths(block.Name, block.Arguments)
	call := &analyzedCall{id: block.ID, name: block.Name, args: block.Arguments, kind: kind, paths: paths, signature: callSignature(block.Name, block.Arguments), turn: analysis.turn, ordinal: analysis.callOrdinal, when: when, usage: analysis.cumulative}
	call.added, call.removed = editChurn(block.Name, block.Arguments)
	call.churnKnown = editChurnKnown(block.Name, block.Arguments)
	call.oldHash, call.newHash = editTransition(block.Name, block.Arguments)
	call.transitionRecorded = call.oldHash != "" || call.newHash != ""
	analysis.calls = append(analysis.calls, call)
	analysis.byID[call.id] = call
	analysis.run.Tools.Total++
	countKind(analysis.run, call)
	analysis.recordRepeatedCall(call)
	analysis.recordPaths(call)
	if kind.mutation {
		analysis.recordMutation(call)
	}
}

func (analysis *segmentAnalysis) recordRepeatedCall(call *analyzedCall) {
	if analysis.seenSignatures[call.signature] {
		analysis.run.RepeatedCalls++
	}
	analysis.seenSignatures[call.signature] = true
	if !call.kind.read {
		return
	}
	rangeKey := readRangeSignature(call)
	for _, path := range call.paths {
		seen := analysis.seenReads[path]
		if seen == nil {
			seen = make(map[string]bool)
			analysis.seenReads[path] = seen
		}
		if seen[rangeKey] {
			analysis.run.RedundantReads++
		}
		seen[rangeKey] = true
	}
}

func (analysis *segmentAnalysis) recordPaths(call *analyzedCall) {
	for _, path := range call.paths {
		if call.kind.navigation || call.kind.read {
			analysis.inspected[path] = true
		}
		if call.kind.mutation {
			analysis.edited[path] = true
		}
	}
}

func (analysis *segmentAnalysis) recordMutation(call *analyzedCall) {
	analysis.run.EditOperations++
	analysis.run.AddedLines += call.added
	analysis.run.RemovedLines += call.removed
	for _, path := range call.paths {
		delete(analysis.seenReads, path)
		analysis.recordEditTransition(path, call)
	}
	if analysis.run.FirstAttemptedMutation == nil {
		analysis.run.FirstAttemptedMutation = milestone(analysis.firstEvent, call)
	}
	if analysis.failedTestSinceEdit {
		analysis.run.TestFixCycles++
		analysis.failedTestSinceEdit = false
	}
}

func (analysis *segmentAnalysis) recordEditTransition(path string, call *analyzedCall) {
	prior := analysis.priorEditContents[path]
	if prior == nil {
		prior = make(map[string]bool)
		analysis.priorEditContents[path] = prior
	}
	if call.newHash != "" && prior[call.newHash] {
		analysis.run.RevertProxies++
	}
	if call.oldHash != "" {
		prior[call.oldHash] = true
	}
}

func (analysis *segmentAnalysis) processToolResult(entryID string, message rawMessage, when time.Time) error {
	call := analysis.byID[message.ToolCallID]
	if call == nil {
		return nil
	}
	call.resolved = true
	call.success = !message.IsError && successfulDetails(message.Details)
	textBytes, lines, empty, err := contentSize(message.Content)
	if err != nil {
		return fmt.Errorf("tool result %q: %w", entryID, err)
	}
	call.bytes, call.lines, call.zero = textBytes, lines, empty
	analysis.applyEditResultEvidence(call, message.Details)
	analysis.run.ToolResultBytes += int64(textBytes)
	analysis.run.ToolResultLines += int64(lines)
	analysis.recordToolOutcome(call)
	analysis.recordTestOutcome(call, when)
	return nil
}

func (analysis *segmentAnalysis) applyEditResultEvidence(call *analyzedCall, details json.RawMessage) {
	if !call.kind.mutation || !call.success {
		return
	}
	added, removed, oldHash, newHash, churnKnown := editResultEvidence(details)
	if churnKnown {
		analysis.run.AddedLines += added - call.added
		analysis.run.RemovedLines += removed - call.removed
		call.added, call.removed, call.churnKnown = added, removed, true
	}
	if call.transitionRecorded || (oldHash == "" && newHash == "") {
		return
	}
	call.oldHash, call.newHash = oldHash, newHash
	for _, path := range call.paths {
		analysis.recordEditTransition(path, call)
	}
	call.transitionRecorded = true
}

func (analysis *segmentAnalysis) recordToolOutcome(call *analyzedCall) {
	if call.success {
		analysis.run.Tools.Successful++
	} else {
		analysis.run.Tools.Failed++
	}
	if call.zero {
		analysis.run.Tools.ZeroResult++
	}
	if call.kind.mutation && call.success && analysis.run.FirstSuccessfulMutation == nil {
		analysis.run.FirstSuccessfulMutation = milestone(analysis.firstEvent, call)
	}
}

func (analysis *segmentAnalysis) recordTestOutcome(call *analyzedCall, when time.Time) {
	if !call.kind.test {
		return
	}
	if call.success && analysis.run.FirstPassingTest == nil {
		completed := *call
		if !when.IsZero() {
			completed.when = when
		}
		analysis.run.FirstPassingTest = milestone(analysis.firstEvent, &completed)
	}
	if !call.success {
		analysis.failedTestSinceEdit = true
	}
}

func (analysis *segmentAnalysis) finalize() {
	run := analysis.run
	run.StartedAt = analysis.firstEvent
	run.EndedAt = analysis.lastEvent
	run.EstimatedToolResultTokens = int64(math.Ceil(float64(run.ToolResultBytes) / 4))
	run.DistinctInspectedFiles = len(analysis.inspected)
	run.DistinctEditedFiles = len(analysis.edited)
	deriveConversions(run, analysis.calls)
	if run.FirstEvidence == nil {
		run.Missing = append(run.Missing, "first_useful_evidence")
	}
	if run.FirstAttemptedMutation == nil {
		run.Missing = append(run.Missing, "first_attempted_mutation")
	}
	if run.FirstSuccessfulMutation == nil {
		run.Missing = append(run.Missing, "first_successful_mutation")
	}
	if run.FirstPassingTest == nil {
		run.Missing = append(run.Missing, "first_passing_test")
	}
	if run.Outcome.Status == "unknown" {
		run.Missing = append(run.Missing, "task_outcome")
	}
	if run.Outcome.FirstEditSurvived == nil {
		run.Missing = append(run.Missing, "first_edit_survival")
	}
	run.Missing = append(run.Missing, "semantic_relevance")
	if run.Tools.Ambiguous > 0 {
		run.Missing = append(run.Missing, "ambiguous_shell_classification")
	}
	for _, call := range analysis.calls {
		if call.kind.mutation && !call.churnKnown {
			run.Missing = append(run.Missing, "edit_churn")
			break
		}
	}
	if run.Complete {
		completionCall := &analyzedCall{turn: run.Turns, ordinal: run.Tools.Total, when: run.EndedAt, usage: run.Usage}
		run.Completion = milestone(run.StartedAt, completionCall)
	}
	sort.Strings(run.Missing)
}

func convertUsage(value rawUsage) Usage {
	cost := value.Cost.Total
	if cost == 0 {
		cost = value.Cost.Input + value.Cost.Output + value.Cost.CacheRead + value.Cost.CacheWrite
	}
	return Usage{Input: value.Input, Output: value.Output, CacheRead: value.CacheRead, CacheWrite: value.CacheWrite, TotalTokens: value.TotalTokens, Cost: cost}
}

func decodeBlocks(raw json.RawMessage) ([]contentBlock, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return blocks, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return nil, nil
	}
	return nil, fmt.Errorf("unsupported content shape")
}

func contentSize(raw json.RawMessage) (int, int, bool, error) {
	blocks, err := decodeBlocks(raw)
	if err != nil {
		return 0, 0, false, err
	}
	bytes := 0
	lines := 0
	for _, block := range blocks {
		if block.Type != "text" {
			continue
		}
		bytes += len([]byte(block.Text))
		if block.Text != "" {
			lines += strings.Count(block.Text, "\n") + 1
		}
	}
	return bytes, lines, bytes == 0, nil
}

func successfulDetails(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var value struct {
		ExitCode *int `json:"exitCode"`
	}
	if json.Unmarshal(raw, &value) != nil || value.ExitCode == nil {
		return true
	}
	return *value.ExitCode == 0
}

func baseToolName(name string) string {
	name = strings.ToLower(name)
	if index := strings.LastIndexAny(name, ".:/"); index >= 0 {
		name = name[index+1:]
	}
	return name
}

func classifyCall(name string, raw json.RawMessage) callKind {
	base := baseToolName(name)
	kind := callKind{}
	switch base {
	case "read":
		kind.read, kind.navigation = true, true
	case "search", "websearch", "session_search", "session_entry_get", "list", "glob", "grep", "find":
		kind.navigation = true
	case "edit", "write", "apply_patch":
		kind.mutation = true
	}
	if base != "bash" && base != "exec" && base != "shell" {
		return kind
	}
	command := stringArgument(raw, "command")
	grepple, mode := classifyGreppleCommand(command)
	if grepple {
		kind.grepple, kind.navigation = true, true
		kind.mode = mode
	} else if strings.Contains(strings.ToLower(command), "grepple") {
		kind.ambiguous = true
	}
	if classifyTestCommand(command) {
		kind.test, kind.verify = true, true
	} else if classifyAmbiguousShellCommand(command) {
		kind.ambiguous = true
	}
	return kind
}

func classifyGreppleCommand(command string) (bool, string) {
	for _, segment := range shellSegments.Split(command, -1) {
		words := shellWords(segment)
		executable := executableIndex(words)
		if executable < 0 || filepath.Base(words[executable]) != "grepple" {
			continue
		}
		return true, greppleMode(words[executable+1:])
	}
	return false, ""
}

func classifyTestCommand(command string) bool {
	for _, segment := range shellSegments.Split(command, -1) {
		words := shellWords(segment)
		index := executableIndex(words)
		if index >= 0 && isTestInvocation(filepath.Base(words[index]), words[index+1:]) {
			return true
		}
	}
	return false
}

func isTestInvocation(executable string, arguments []string) bool {
	if executable == "pytest" {
		return true
	}
	if len(arguments) == 0 {
		return false
	}
	if exactTestExecutables[executable] {
		return arguments[0] == "test"
	}
	switch executable {
	case "make", "pnpm", "yarn":
		return strings.Contains(arguments[0], "test")
	case "npm":
		return arguments[0] == "test" || (arguments[0] == "run" && len(arguments) > 1 && strings.Contains(arguments[1], "test"))
	default:
		return false
	}
}

// classifyAmbiguousShellCommand identifies shell shapes that can hide file
// mutation or test execution. They remain unknown rather than being counted as
// either absent or confidently observed.
func classifyAmbiguousShellCommand(command string) bool {
	lower := strings.ToLower(command)
	if strings.Contains(lower, ">") || strings.Contains(lower, "<<") {
		return true
	}
	for _, segment := range shellSegments.Split(command, -1) {
		words := shellWords(segment)
		index := executableIndex(words)
		if index >= 0 && isAmbiguousInvocation(filepath.Base(words[index]), words[index+1:]) {
			return true
		}
	}
	return false
}

func isAmbiguousInvocation(executable string, arguments []string) bool {
	if alwaysAmbiguousExecutables[executable] {
		return true
	}
	if len(arguments) == 0 {
		return false
	}
	switch executable {
	case "git":
		return ambiguousGitSubcommands[arguments[0]]
	case "go":
		return ambiguousGoSubcommands[arguments[0]]
	case "cargo":
		return ambiguousCargoSubcommands[arguments[0]]
	default:
		return false
	}
}

func shellWords(segment string) []string {
	raw := strings.Fields(segment)
	words := make([]string, 0, len(raw))
	for _, word := range raw {
		word = strings.Trim(word, "'\"")
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}

func executableIndex(words []string) int {
	index := 0
	for index < len(words) && strings.Contains(words[index], "=") && !strings.HasPrefix(words[index], "=") {
		index++
	}
	if index < len(words) && (words[index] == "command" || words[index] == "sudo" || words[index] == "env") {
		index++
		for index < len(words) && (strings.HasPrefix(words[index], "-") || strings.Contains(words[index], "=")) {
			index++
		}
	}
	if index >= len(words) {
		return -1
	}
	return index
}

func greppleMode(arguments []string) string {
	for _, field := range arguments {
		if field == "" || strings.HasPrefix(field, "-") {
			continue
		}
		switch field {
		case "search", "graph", "anchors", "boundaries", "artifacts", "rules", "architecture", "sources", "metrics":
			return field
		default:
			return "search"
		}
	}
	return "search"
}

func countKind(run *Run, call *analyzedCall) {
	kind := call.kind
	if kind.navigation {
		run.Tools.Navigation++
	}
	if kind.read {
		run.Tools.Read++
	}
	if kind.mutation {
		run.Tools.Mutation++
	}
	if kind.test {
		run.Tools.Test++
	}
	if kind.verify {
		run.Tools.Verification++
	}
	if kind.grepple {
		run.Tools.Grepple++
		run.ObservedGreppleUse = true
		run.GreppleModes[kind.mode]++
	}
	if kind.ambiguous {
		run.Tools.Ambiguous++
	}
}

func milestone(start time.Time, call *analyzedCall) *Milestone {
	elapsed := int64(0)
	if !start.IsZero() && !call.when.IsZero() {
		elapsed = call.when.Sub(start).Milliseconds()
		if elapsed < 0 {
			elapsed = 0
		}
	}
	return &Milestone{ElapsedMS: elapsed, Turn: call.turn, CallsBefore: max(call.ordinal-1, 0), InclusiveCall: call.ordinal, CumulativeUsage: call.usage}
}

func deriveConversions(run *Run, calls []*analyzedCall) {
	for index, source := range calls {
		if !source.kind.navigation || !source.success {
			continue
		}
		used := deriveSourceConversions(run, source, calls[index+1:])
		if used && run.FirstEvidence == nil {
			run.FirstEvidence = milestone(run.StartedAt, source)
		}
	}
}

func deriveSourceConversions(run *Run, source *analyzedCall, laterCalls []*analyzedCall) bool {
	used := false
	for _, later := range laterCalls {
		if (!later.kind.read && !later.kind.mutation) || !pathsRelated(source.paths, later.paths) {
			continue
		}
		used = true
		if later.kind.read {
			run.SearchToRead++
		}
		if later.kind.mutation {
			run.SearchToEdit++
		}
	}
	return used
}

func pathsRelated(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	for _, a := range left {
		for _, b := range right {
			if a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") {
				return true
			}
		}
	}
	return false
}

func extractPaths(name string, raw json.RawMessage) []string {
	base := baseToolName(name)
	if base == "bash" || base == "exec" || base == "shell" {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	keys := []string{"path", "file", "filename", "cwd", "scope"}
	seen := make(map[string]bool)
	var result []string
	for _, key := range keys {
		text, ok := value[key].(string)
		if !ok || text == "" {
			continue
		}
		path := filepath.ToSlash(filepath.Clean(text))
		if !seen[path] {
			seen[path] = true
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

func stringArgument(raw json.RawMessage, key string) string {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	var result string
	_ = json.Unmarshal(value[key], &result)
	return result
}

func readRangeSignature(call *analyzedCall) string {
	if baseToolName(call.name) != "read" {
		return call.signature
	}
	var arguments struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	if json.Unmarshal(call.args, &arguments) != nil {
		return call.signature
	}
	if arguments.Offset <= 0 {
		arguments.Offset = 1
	}
	if arguments.Limit < 0 {
		arguments.Limit = 0
	}
	return fmt.Sprintf("%d:%d", arguments.Offset, arguments.Limit)
}

func callSignature(name string, raw json.RawMessage) string {
	sum := sha256.Sum256(append([]byte(strings.ToLower(name)+"\x00"), raw...))
	return hex.EncodeToString(sum[:8])
}

func editChurn(name string, raw json.RawMessage) (int, int) {
	base := baseToolName(name)
	if base != "edit" && base != "write" && base != "apply_patch" {
		return 0, 0
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return 0, 0
	}
	return churnValue(value)
}

func editChurnKnown(name string, raw json.RawMessage) bool {
	base := baseToolName(name)
	if base != "edit" && base != "apply_patch" {
		return false
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	_, oldOK := value["oldText"].(string)
	_, newOK := value["newText"].(string)
	return oldOK && newOK
}

func editResultEvidence(raw json.RawMessage) (int, int, string, string, bool) {
	var details struct {
		Patch   string `json:"patch"`
		Metrics *struct {
			Classification string `json:"classification"`
			AddedLines     *int   `json:"added_lines"`
			RemovedLines   *int   `json:"removed_lines"`
		} `json:"metrics"`
	}
	if json.Unmarshal(raw, &details) != nil {
		return 0, 0, "", "", false
	}
	oldHash, newHash := patchTransition(details.Patch)
	metrics := details.Metrics
	if metrics == nil || metrics.Classification != "applied" || metrics.AddedLines == nil || metrics.RemovedLines == nil {
		return 0, 0, oldHash, newHash, false
	}
	return *metrics.AddedLines, *metrics.RemovedLines, oldHash, newHash, true
}

func patchTransition(patch string) (string, string) {
	var removed, added []string
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "@@"), strings.HasPrefix(line, `\ No newline`):
			continue
		case strings.HasPrefix(line, "-"):
			removed = append(removed, strings.TrimSuffix(line[1:], "\r"))
		case strings.HasPrefix(line, "+"):
			added = append(added, strings.TrimSuffix(line[1:], "\r"))
		}
	}
	var oldHash, newHash string
	if len(removed) > 0 {
		oldHash = textFingerprint(strings.Join(removed, "\n"))
	}
	if len(added) > 0 {
		newHash = textFingerprint(strings.Join(added, "\n"))
	}
	return oldHash, newHash
}

func editTransition(name string, raw json.RawMessage) (string, string) {
	base := baseToolName(name)
	if base != "edit" && base != "apply_patch" {
		return "", ""
	}
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return "", ""
	}
	oldText, oldOK := value["oldText"].(string)
	newText, newOK := value["newText"].(string)
	if !oldOK || !newOK {
		return "", ""
	}
	return textFingerprint(oldText), textFingerprint(newText)
}

func textFingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}

func churnValue(value any) (int, int) {
	switch current := value.(type) {
	case map[string]any:
		return churnMap(current)
	case []any:
		return churnSlice(current)
	default:
		return 0, 0
	}
}

func churnMap(values map[string]any) (int, int) {
	added, removed := 0, 0
	for key, child := range values {
		a, r := churnField(key, child)
		added, removed = added+a, removed+r
	}
	return added, removed
}

func churnSlice(values []any) (int, int) {
	added, removed := 0, 0
	for _, child := range values {
		a, r := churnValue(child)
		added, removed = added+a, removed+r
	}
	return added, removed
}

func churnField(key string, value any) (int, int) {
	switch key {
	case "content", "newText":
		text, _ := value.(string)
		return lineCount(text), 0
	case "oldText":
		text, _ := value.(string)
		return 0, lineCount(text)
	case "content_lines":
		lines, _ := value.([]any)
		return len(lines), 0
	default:
		return churnValue(value)
	}
}

func lineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func applyOutcome(run *Run, marker annotation) {
	if marker.Outcome != "" {
		run.Outcome.Status = marker.Outcome
	}
	run.Outcome.HumanInterventions = marker.HumanInterventions
	run.Outcome.EvaluatorScore = marker.EvaluatorScore
	run.Outcome.Rubric = marker.Rubric
	run.Outcome.Regressions = marker.Regressions
	run.Outcome.FirstEditSurvived = marker.FirstEditSurvived
	if marker.TaskID != "" {
		run.TaskID = marker.TaskID
	}
}
