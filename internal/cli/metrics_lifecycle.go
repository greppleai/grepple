package cli

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	agentmetrics "github.com/greppleai/grepple/internal/metrics"
)

const (
	metricsDirectoryEnv = "GREPPLE_METRICS_DIR"
	activeStateSchema   = "grepple-metrics-active-v1"
)

var metricsIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type metricsActiveState struct {
	Schema             string `json:"schema"`
	RunID              string `json:"runId"`
	TaskID             string `json:"taskId"`
	AssignedCohort     string `json:"assignedCohort"`
	Journal            string `json:"journal"`
	RepositoryRootHash string `json:"repositoryRootHash"`
}

type metricsEventOptions struct {
	runID   string
	eventID string
	at      string
}

func runMetricsStart(args []string) error {
	flags := flag.NewFlagSet("grepple metrics start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var common metricsEventOptions
	var taskID, cohort, repository, revision string
	flags.StringVar(&common.runID, "run", "", "run identifier")
	flags.StringVar(&common.eventID, "event-id", "", "event identifier")
	flags.StringVar(&common.at, "at", "", "event time in RFC3339 format")
	flags.StringVar(&taskID, "task", "", "opaque task identifier")
	flags.StringVar(&cohort, "cohort", "", "assigned cohort")
	flags.StringVar(&repository, "repository", "", "repository name")
	flags.StringVar(&revision, "revision", "", "repository revision")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected metrics start arguments: %s", strings.Join(flags.Args(), " "))
	}
	if taskID == "" || cohort == "" {
		return errors.New("metrics start requires --task and --cohort")
	}
	directory, err := metricsDirectory()
	if err != nil {
		return err
	}
	if _, err := readMetricsActiveState(directory); err == nil {
		return errors.New("a metrics run is already active for this repository")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if common.runID == "" {
		common.runID, err = newMetricsID()
		if err != nil {
			return err
		}
	}
	if !metricsIdentifier.MatchString(common.runID) {
		return errors.New("--run must contain only letters, digits, dot, underscore, or hyphen")
	}
	root := metricsRepositoryRoot()
	if repository == "" {
		repository = filepath.Base(root)
	}
	if revision == "" {
		revision = metricsGitRevision(root)
	}
	data := agentmetrics.RunStartData{TaskID: taskID, Repository: repository, Revision: revision, AssignedCohort: cohort}
	journal := filepath.Join(directory, "runs", common.runID+".jsonl")
	state := metricsActiveState{
		Schema: activeStateSchema, RunID: common.runID, TaskID: taskID, AssignedCohort: cohort,
		Journal: journal, RepositoryRootHash: metricsRepositoryHash(root),
	}
	if err := writeMetricsActiveState(directory, state); err != nil {
		return err
	}
	if err := appendMetricsEvent(journal, common, agentmetrics.EventRunStart, data); err != nil {
		_ = os.Remove(metricsActiveStatePath(directory))
		return err
	}
	return stdoutWriter().writeString(fmt.Sprintf("Started metrics run %s (%s)\n", common.runID, journal))
}

func runMetricsRecord(args []string) error {
	flags := flag.NewFlagSet("grepple metrics record", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var common metricsEventOptions
	var eventType, rawData string
	flags.StringVar(&common.runID, "run", "", "run identifier")
	flags.StringVar(&common.eventID, "event-id", "", "event identifier")
	flags.StringVar(&common.at, "at", "", "event time in RFC3339 format")
	flags.StringVar(&eventType, "event", "", "assistant, tool_call, tool_result, or compaction")
	flags.StringVar(&rawData, "data", "", "normalized event data JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected metrics record arguments: %s", strings.Join(flags.Args(), " "))
	}
	allowed := map[string]bool{agentmetrics.EventAssistant: true, agentmetrics.EventToolCall: true, agentmetrics.EventToolResult: true, agentmetrics.EventCompaction: true}
	if !allowed[eventType] {
		return errors.New("--event must be assistant, tool_call, tool_result, or compaction")
	}
	if rawData == "" {
		rawData = "{}"
	}
	data := json.RawMessage(rawData)
	if _, err := agentmetrics.DecodeEventData(eventType, data); err != nil {
		return err
	}
	directory, err := metricsDirectory()
	if err != nil {
		return err
	}
	journal, err := resolveMetricsJournal(directory, &common.runID)
	if err != nil {
		return err
	}
	return appendMetricsRawEvent(journal, common, eventType, data)
}

func runMetricsStatus(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("unexpected metrics status arguments: %s", strings.Join(args, " "))
	}
	directory, err := metricsDirectory()
	if err != nil {
		return err
	}
	state, err := readMetricsActiveState(directory)
	if errors.Is(err, os.ErrNotExist) {
		return stdoutWriter().writeString("No active metrics run.\n")
	}
	if err != nil {
		return err
	}
	return stdoutWriter().writeString(fmt.Sprintf("Active metrics run %s task=%s cohort=%s journal=%s\n", state.RunID, state.TaskID, state.AssignedCohort, state.Journal))
}

func runMetricsEnd(args []string) error {
	flags := flag.NewFlagSet("grepple metrics end", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	var common metricsEventOptions
	var outcome, score, interventions, regressions, survived, rubric string
	flags.StringVar(&common.runID, "run", "", "run identifier")
	flags.StringVar(&common.eventID, "event-id", "", "event identifier")
	flags.StringVar(&common.at, "at", "", "event time in RFC3339 format")
	flags.StringVar(&outcome, "outcome", "", "success, failure, abandoned, or unknown")
	flags.StringVar(&score, "score", "-", "evaluator score or -")
	flags.StringVar(&interventions, "human-interventions", "-", "count or -")
	flags.StringVar(&regressions, "regressions", "-", "count or -")
	flags.StringVar(&survived, "first-edit-survived", "-", "boolean or -")
	flags.StringVar(&rubric, "rubric", "", "opaque rubric name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected metrics end arguments: %s", strings.Join(flags.Args(), " "))
	}
	data := agentmetrics.RunEndData{Outcome: outcome, Rubric: rubric}
	var err error
	if data.EvaluatorScore, err = parseOptionalFloat(score); err != nil {
		return fmt.Errorf("--score: %w", err)
	}
	if data.HumanInterventions, err = parseOptionalInt(interventions); err != nil {
		return fmt.Errorf("--human-interventions: %w", err)
	}
	if data.Regressions, err = parseOptionalInt(regressions); err != nil {
		return fmt.Errorf("--regressions: %w", err)
	}
	if data.FirstEditSurvived, err = parseOptionalBool(survived); err != nil {
		return fmt.Errorf("--first-edit-survived: %w", err)
	}
	encoded, err := agentmetrics.MarshalJournalData(data)
	if err != nil {
		return err
	}
	if _, err := agentmetrics.DecodeEventData(agentmetrics.EventRunEnd, encoded); err != nil {
		return err
	}
	directory, err := metricsDirectory()
	if err != nil {
		return err
	}
	state, stateErr := readMetricsActiveState(directory)
	journal, err := resolveMetricsJournal(directory, &common.runID)
	if err != nil {
		return err
	}
	if err := appendMetricsRawEvent(journal, common, agentmetrics.EventRunEnd, encoded); err != nil {
		return err
	}
	if stateErr == nil && state.RunID == common.runID {
		if err := os.Remove(metricsActiveStatePath(directory)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("clear active metrics run: %w", err)
		}
	}
	return stdoutWriter().writeString(fmt.Sprintf("Ended metrics run %s with outcome %s\n", common.runID, outcome))
}

func appendMetricsEvent(path string, common metricsEventOptions, eventType string, data any) error {
	encoded, err := agentmetrics.MarshalJournalData(data)
	if err != nil {
		return err
	}
	return appendMetricsRawEvent(path, common, eventType, encoded)
}

func appendMetricsRawEvent(path string, common metricsEventOptions, eventType string, data json.RawMessage) error {
	if common.eventID == "" {
		var err error
		common.eventID, err = newMetricsID()
		if err != nil {
			return err
		}
	}
	at := time.Now().UTC()
	if common.at != "" {
		parsed, err := time.Parse(time.RFC3339Nano, common.at)
		if err != nil {
			return fmt.Errorf("--at: %w", err)
		}
		at = parsed.UTC()
	}
	return agentmetrics.AppendJournalEvent(path, agentmetrics.JournalEvent{
		Schema: agentmetrics.JournalSchema, EventID: common.eventID, Time: at,
		RunID: common.runID, Event: eventType, Data: data,
	})
}

func resolveMetricsJournal(directory string, runID *string) (string, error) {
	if *runID == "" {
		state, err := readMetricsActiveState(directory)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", errors.New("no active metrics run; pass --run or use metrics start")
			}
			return "", err
		}
		*runID = state.RunID
		return state.Journal, nil
	}
	if !metricsIdentifier.MatchString(*runID) {
		return "", errors.New("--run must contain only letters, digits, dot, underscore, or hyphen")
	}
	return filepath.Join(directory, "runs", *runID+".jsonl"), nil
}

func metricsDirectory() (string, error) {
	if directory := os.Getenv(metricsDirectoryEnv); directory != "" {
		return filepath.Clean(directory), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".grepple", "metrics"), nil
}

func metricsActiveStatePath(directory string) string {
	return filepath.Join(directory, "active", metricsRepositoryHash(metricsRepositoryRoot())+".json")
}

func readMetricsActiveState(directory string) (metricsActiveState, error) {
	path := metricsActiveStatePath(directory)
	data, err := os.ReadFile(path)
	if err != nil {
		return metricsActiveState{}, err
	}
	var state metricsActiveState
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return metricsActiveState{}, fmt.Errorf("decode active metrics state: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return metricsActiveState{}, fmt.Errorf("decode active metrics state: %w", err)
	}
	expectedHash := metricsRepositoryHash(metricsRepositoryRoot())
	expectedJournal := filepath.Join(directory, "runs", state.RunID+".jsonl")
	if state.Schema != activeStateSchema || !metricsIdentifier.MatchString(state.RunID) || filepath.Clean(state.Journal) != expectedJournal || state.RepositoryRootHash != expectedHash {
		return metricsActiveState{}, errors.New("invalid active metrics state")
	}
	return state, nil
}

func writeMetricsActiveState(directory string, state metricsActiveState) error {
	path := metricsActiveStatePath(directory)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create active metrics directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("secure active metrics directory: %w", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return errors.New("a metrics run is already active for this repository")
	}
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func metricsRepositoryRoot() string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "."
	}
	command := exec.Command("git", "-C", workingDirectory, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err == nil && strings.TrimSpace(string(output)) != "" {
		return filepath.Clean(strings.TrimSpace(string(output)))
	}
	return filepath.Clean(workingDirectory)
}

func metricsGitRevision(root string) string {
	output, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func metricsRepositoryHash(root string) string {
	digest := sha256.Sum256([]byte(root))
	return hex.EncodeToString(digest[:12])
}

func newMetricsID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate metrics identifier: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func parseOptionalInt(value string) (*int, error) {
	if value == "-" || value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return nil, errors.New("must be a non-negative integer or -")
	}
	return &parsed, nil
}

func parseOptionalFloat(value string) (*float64, error) {
	if value == "-" || value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, errors.New("must be a number or -")
	}
	return &parsed, nil
}

func parseOptionalBool(value string) (*bool, error) {
	if value == "-" || value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, errors.New("must be true, false, or -")
	}
	return &parsed, nil
}
