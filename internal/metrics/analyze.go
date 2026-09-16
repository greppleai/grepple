package metrics

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type normalizedCall struct {
	data       ToolCallData
	ordinal    int
	when       time.Time
	usage      Usage
	success    bool
	resolved   bool
	churnKnown bool
}

type journalAnalysis struct {
	run                  Run
	started              bool
	ended                bool
	hasAssistant         bool
	commandVolumeMissing bool
	editChurnMissing     bool
	calls                []*normalizedCall
	callsByID            map[string]*normalizedCall
	seenShapes           map[string]bool
	seenReads            map[string]bool
	inspected            map[string]bool
	edited               map[string]bool
	fingerprints         map[string]map[string]bool
	failedTestSinceEdit  bool
	lastEvent            time.Time
}

// AnalyzeFile deterministically converts one Grepple metrics journal into runs.
func AnalyzeFile(path string) ([]Run, error) {
	events, err := ReadJournal(path)
	if err != nil {
		return nil, err
	}
	analyses := make(map[string]*journalAnalysis)
	for _, event := range events {
		analysis := analyses[event.RunID]
		if analysis == nil {
			analysis = newJournalAnalysis(event.RunID)
			analyses[event.RunID] = analysis
		}
		if err := analysis.process(event); err != nil {
			return nil, fmt.Errorf("event %q: %w", event.EventID, err)
		}
	}
	runIDs := make([]string, 0, len(analyses))
	for runID := range analyses {
		runIDs = append(runIDs, runID)
	}
	sort.Strings(runIDs)
	runs := make([]Run, 0, len(runIDs))
	for _, runID := range runIDs {
		run, err := analyses[runID].finalize()
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func newJournalAnalysis(runID string) *journalAnalysis {
	return &journalAnalysis{
		run: Run{
			Schema:       "grepple-agent-metrics-v1",
			RunID:        runID,
			GreppleModes: make(map[string]int),
			Outcome:      Outcome{Status: "unknown"},
			Missing:      make([]string, 0),
		},
		callsByID:    make(map[string]*normalizedCall),
		seenShapes:   make(map[string]bool),
		seenReads:    make(map[string]bool),
		inspected:    make(map[string]bool),
		edited:       make(map[string]bool),
		fingerprints: make(map[string]map[string]bool),
	}
}

func (analysis *journalAnalysis) process(event JournalEvent) error {
	if event.Time.After(analysis.lastEvent) {
		analysis.lastEvent = event.Time
	}
	decoded, err := DecodeEventData(event.Event, event.Data)
	if err != nil {
		return err
	}
	if event.Event != EventRunStart && !analysis.started {
		return fmt.Errorf("%s appears before run_start", event.Event)
	}
	if analysis.ended {
		return fmt.Errorf("%s appears after run_end", event.Event)
	}
	switch data := decoded.(type) {
	case *RunStartData:
		return analysis.processStart(event.Time, *data)
	case *AssistantData:
		analysis.processAssistant(*data)
	case *ToolCallData:
		return analysis.processToolCall(event.Time, *data)
	case *ToolResultData:
		return analysis.processToolResult(event.Time, *data)
	case *CommandData:
		analysis.processCommand(event.Time, *data)
	case *struct{}:
		analysis.run.Compactions++
	case *RunEndData:
		return analysis.processEnd(event.Time, *data)
	}
	return nil
}

func (analysis *journalAnalysis) processStart(at time.Time, data RunStartData) error {
	if analysis.started {
		return fmt.Errorf("duplicate run_start for run %q", analysis.run.RunID)
	}
	analysis.started = true
	analysis.run.Agent = data.Agent
	analysis.run.StartedAt = at
	return nil
}

func (analysis *journalAnalysis) processAssistant(data AssistantData) {
	analysis.hasAssistant = true
	analysis.run.Turns++
	analysis.run.Usage.add(data.Usage)
}

func (analysis *journalAnalysis) processCommand(at time.Time, data CommandData) {
	analysis.run.Tools.Total++
	analysis.run.Tools.Navigation++
	analysis.run.Tools.Grepple++
	if data.Success {
		analysis.run.Tools.Successful++
	} else {
		analysis.run.Tools.Failed++
	}
	analysis.run.ObservedGreppleUse = true
	mode := data.GreppleMode
	if mode == "" {
		mode = data.Name
	}
	analysis.run.GreppleModes[mode]++
	analysis.commandVolumeMissing = true
	if data.Success && analysis.run.FirstEvidence == nil {
		analysis.run.FirstEvidence = analysis.milestone(at, analysis.run.Tools.Total, analysis.run.Turns, analysis.run.Usage)
	}
}

func (analysis *journalAnalysis) processToolCall(at time.Time, data ToolCallData) error {
	if analysis.callsByID[data.CallID] != nil {
		return fmt.Errorf("duplicate tool callId %q", data.CallID)
	}
	analysis.run.Tools.Total++
	call := &normalizedCall{data: data, ordinal: analysis.run.Tools.Total, when: at, usage: analysis.run.Usage}
	analysis.calls = append(analysis.calls, call)
	analysis.callsByID[data.CallID] = call
	analysis.countCategory(data.Category)
	analysis.observeGreppleTool(data)
	analysis.trackRepeatedCall(data)
	analysis.trackResourceAccess(data)
	analysis.trackMutationCall(call, at)
	return nil
}

func (analysis *journalAnalysis) observeGreppleTool(data ToolCallData) {
	if data.GreppleMode == "" {
		return
	}
	analysis.run.Tools.Grepple++
	analysis.run.ObservedGreppleUse = true
	analysis.run.GreppleModes[data.GreppleMode]++
}

func (analysis *journalAnalysis) trackRepeatedCall(data ToolCallData) {
	shape := data.Tool + "\x00" + data.ArgumentShape
	if analysis.seenShapes[shape] {
		analysis.run.RepeatedCalls++
	}
	analysis.seenShapes[shape] = true
}

func (analysis *journalAnalysis) trackResourceAccess(data ToolCallData) {
	if data.ResourceID != "" && (data.Category == "navigation" || data.Category == "read") {
		analysis.inspected[data.ResourceID] = true
	}
	if data.Category != "read" || data.ResourceID == "" {
		return
	}
	rangeKey := data.ResourceID + "\x00" + optionalInt(data.Offset) + "\x00" + optionalInt(data.Limit)
	if analysis.seenReads[rangeKey] {
		analysis.run.RedundantReads++
	}
	analysis.seenReads[rangeKey] = true
}

func (analysis *journalAnalysis) trackMutationCall(call *normalizedCall, at time.Time) {
	if call.data.Category != "mutation" {
		return
	}
	analysis.run.EditOperations++
	if call.data.ResourceID != "" {
		analysis.edited[call.data.ResourceID] = true
		for key := range analysis.seenReads {
			if strings.HasPrefix(key, call.data.ResourceID+"\x00") {
				delete(analysis.seenReads, key)
			}
		}
	}
	if analysis.run.FirstAttemptedMutation == nil {
		analysis.run.FirstAttemptedMutation = analysis.callMilestone(call, at)
	}
	if analysis.failedTestSinceEdit {
		analysis.run.TestFixCycles++
		analysis.failedTestSinceEdit = false
	}
}

func (analysis *journalAnalysis) countCategory(category string) {
	switch category {
	case "navigation":
		analysis.run.Tools.Navigation++
	case "read":
		analysis.run.Tools.Read++
	case "mutation":
		analysis.run.Tools.Mutation++
	case "test":
		analysis.run.Tools.Test++
	case "verification":
		analysis.run.Tools.Verification++
	case "ambiguous":
		analysis.run.Tools.Ambiguous++
	}
}

func (analysis *journalAnalysis) processToolResult(at time.Time, data ToolResultData) error {
	call := analysis.callsByID[data.CallID]
	if call == nil {
		return fmt.Errorf("tool_result references unknown callId %q", data.CallID)
	}
	if call.resolved {
		return fmt.Errorf("duplicate tool_result for callId %q", data.CallID)
	}
	call.resolved = true
	call.success = data.Success
	analysis.run.ToolResultBytes += data.Bytes
	analysis.run.ToolResultLines += data.Lines
	if data.Success {
		analysis.run.Tools.Successful++
	} else {
		analysis.run.Tools.Failed++
	}
	if data.ZeroResult {
		analysis.run.Tools.ZeroResult++
	}
	if call.data.Category == "mutation" {
		analysis.processMutationResult(call, at, data)
	}
	if call.data.Category == "test" || data.TestOutcome != "" {
		if data.TestOutcome == "pass" && analysis.run.FirstPassingTest == nil {
			analysis.run.FirstPassingTest = analysis.callMilestone(call, at)
		}
		if data.TestOutcome == "fail" {
			analysis.failedTestSinceEdit = true
		}
	}
	return nil
}

func (analysis *journalAnalysis) processMutationResult(call *normalizedCall, at time.Time, data ToolResultData) {
	if !data.Success {
		return
	}
	if analysis.run.FirstSuccessfulMutation == nil {
		analysis.run.FirstSuccessfulMutation = analysis.callMilestone(call, at)
	}
	if data.AddedLines == nil || data.RemovedLines == nil {
		analysis.editChurnMissing = true
	} else {
		call.churnKnown = true
		analysis.run.AddedLines += *data.AddedLines
		analysis.run.RemovedLines += *data.RemovedLines
	}
	if call.data.ResourceID == "" {
		return
	}
	history := analysis.fingerprints[call.data.ResourceID]
	if history == nil {
		history = make(map[string]bool)
		analysis.fingerprints[call.data.ResourceID] = history
	}
	if data.AfterFingerprint != "" && history[data.AfterFingerprint] {
		analysis.run.RevertProxies++
	}
	if data.BeforeFingerprint != "" {
		history[data.BeforeFingerprint] = true
	}
	if data.AfterFingerprint != "" {
		history[data.AfterFingerprint] = true
	}
}

func (analysis *journalAnalysis) processEnd(at time.Time, data RunEndData) error {
	if analysis.ended {
		return fmt.Errorf("duplicate run_end for run %q", analysis.run.RunID)
	}
	analysis.ended = true
	analysis.run.Complete = true
	analysis.run.Outcome = Outcome{
		Status: data.Outcome, HumanInterventions: data.HumanInterventions,
		EvaluatorScore: data.EvaluatorScore, Rubric: data.Rubric,
		Regressions: data.Regressions, FirstEditSurvived: data.FirstEditSurvived,
	}
	analysis.run.Completion = analysis.milestone(at, analysis.run.Tools.Total, analysis.run.Turns, analysis.run.Usage)
	return nil
}

func (analysis *journalAnalysis) finalize() (Run, error) {
	if !analysis.started {
		return Run{}, fmt.Errorf("run %q has no run_start", analysis.run.RunID)
	}
	analysis.run.EndedAt = analysis.lastEvent
	analysis.run.EstimatedToolResultTokens = (analysis.run.ToolResultBytes + 3) / 4
	analysis.run.DistinctInspectedFiles = len(analysis.inspected)
	analysis.run.DistinctEditedFiles = len(analysis.edited)
	analysis.deriveConversions()
	if !analysis.hasAssistant {
		analysis.addMissing("agent_turns", "assistant_usage")
	}
	if !analysis.ended {
		analysis.addMissing("run_end")
	}
	if analysis.run.Outcome.Status == "unknown" {
		analysis.addMissing("task_outcome")
	}
	if analysis.run.FirstEvidence == nil {
		analysis.addMissing("first_useful_evidence")
	}
	if analysis.run.FirstAttemptedMutation == nil {
		analysis.addMissing("first_attempted_mutation")
	}
	if analysis.run.FirstSuccessfulMutation == nil {
		analysis.addMissing("first_successful_mutation")
	}
	if analysis.run.FirstPassingTest == nil {
		analysis.addMissing("first_passing_test")
	}
	if analysis.run.Outcome.FirstEditSurvived == nil {
		analysis.addMissing("first_edit_survival")
	}
	if analysis.commandVolumeMissing {
		analysis.addMissing("tool_result_volume")
	}
	if analysis.editChurnMissing {
		analysis.addMissing("edit_churn")
	}
	for _, call := range analysis.calls {
		if !call.resolved {
			analysis.addMissing("tool_results")
			break
		}
	}
	analysis.addMissing("retries", "semantic_relevance")
	if analysis.run.Tools.Ambiguous > 0 {
		analysis.addMissing("ambiguous_tool_classification")
	}
	sort.Strings(analysis.run.Missing)
	return analysis.run, nil
}

func (analysis *journalAnalysis) deriveConversions() {
	for index, source := range analysis.calls {
		if source.data.Category != "navigation" || !source.success || source.data.ResourceID == "" {
			continue
		}
		used := analysis.deriveConversionsFrom(source, analysis.calls[index+1:])
		if used && analysis.run.FirstEvidence == nil {
			analysis.run.FirstEvidence = analysis.callMilestone(source, source.when)
		}
	}
}

func (analysis *journalAnalysis) deriveConversionsFrom(source *normalizedCall, laterCalls []*normalizedCall) bool {
	used := false
	for _, later := range laterCalls {
		if later.data.Category != "read" && later.data.Category != "mutation" {
			continue
		}
		if source.data.ResourceID != later.data.ResourceID {
			continue
		}
		used = true
		if later.data.Category == "read" {
			analysis.run.SearchToRead++
		} else {
			analysis.run.SearchToEdit++
		}
	}
	return used
}

func (analysis *journalAnalysis) callMilestone(call *normalizedCall, at time.Time) *Milestone {
	return analysis.milestone(at, call.ordinal, call.data.Turn, call.usage)
}

func (analysis *journalAnalysis) milestone(at time.Time, ordinal, turn int, usage Usage) *Milestone {
	elapsed := at.Sub(analysis.run.StartedAt).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	return &Milestone{ElapsedMS: elapsed, Turn: turn, CallsBefore: max(ordinal-1, 0), InclusiveCall: ordinal, CumulativeUsage: usage}
}

func (analysis *journalAnalysis) addMissing(values ...string) {
	for _, value := range values {
		found := false
		for _, existing := range analysis.run.Missing {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			analysis.run.Missing = append(analysis.run.Missing, value)
		}
	}
}

func optionalInt(value *int) string {
	if value == nil {
		return "*"
	}
	return strconv.Itoa(*value)
}
