// Package metrics validates and analyzes privacy-safe coding-agent journals.
package metrics

import "time"

// Usage records provider-reported token counts and cost for a run or milestone.
type Usage struct {
	Input       int64   `json:"input"`
	Output      int64   `json:"output"`
	CacheRead   int64   `json:"cacheRead"`
	CacheWrite  int64   `json:"cacheWrite"`
	TotalTokens int64   `json:"totalTokens"`
	Cost        float64 `json:"cost"`
}

func (u *Usage) add(v Usage) {
	u.Input += v.Input
	u.Output += v.Output
	u.CacheRead += v.CacheRead
	u.CacheWrite += v.CacheWrite
	u.TotalTokens += v.TotalTokens
	u.Cost += v.Cost
}

// Milestone captures elapsed work and cumulative usage through a significant run event.
type Milestone struct {
	ElapsedMS       int64 `json:"elapsedMs"`
	Turn            int   `json:"turn"`
	CallsBefore     int   `json:"callsBefore"`
	InclusiveCall   int   `json:"inclusiveCall"`
	CumulativeUsage Usage `json:"cumulativeUsage"`
}

// ToolCounts summarizes agent-visible tool calls by outcome and conservative category.
type ToolCounts struct {
	Total        int `json:"total"`
	Successful   int `json:"successful"`
	Failed       int `json:"failed"`
	Navigation   int `json:"navigation"`
	Read         int `json:"read"`
	Mutation     int `json:"mutation"`
	Test         int `json:"test"`
	Verification int `json:"verification"`
	Grepple      int `json:"grepple"`
	Ambiguous    int `json:"ambiguous"`
	ZeroResult   int `json:"zeroResult"`
}

// Outcome contains explicit task evaluation annotations; pointer fields are nil when unknown.
type Outcome struct {
	Status             string   `json:"status"`
	HumanInterventions *int     `json:"humanInterventions"`
	EvaluatorScore     *float64 `json:"evaluatorScore"`
	Rubric             string   `json:"rubric"`
	Regressions        *int     `json:"regressions"`
	FirstEditSurvived  *bool    `json:"firstEditSurvived"`
}

// Run is the privacy-safe analysis of one logical journal run.
type Run struct {
	Schema                    string         `json:"schema"`
	RunID                     string         `json:"runId"`
	TaskID                    string         `json:"taskId"`
	Repository                string         `json:"repository"`
	Revision                  string         `json:"revision"`
	AssignedCohort            string         `json:"assignedCohort"`
	ObservedGreppleUse        bool           `json:"observedGreppleUse"`
	GreppleModes              map[string]int `json:"greppleModes"`
	StartedAt                 time.Time      `json:"startedAt"`
	EndedAt                   time.Time      `json:"endedAt"`
	Complete                  bool           `json:"complete"`
	Model                     string         `json:"model"`
	Provider                  string         `json:"provider"`
	ThinkingLevel             string         `json:"thinkingLevel"`
	Turns                     int            `json:"turns"`
	Usage                     Usage          `json:"usage"`
	Tools                     ToolCounts     `json:"tools"`
	ToolResultBytes           int64          `json:"toolResultBytes"`
	ToolResultLines           int64          `json:"toolResultLines"`
	EstimatedToolResultTokens int64          `json:"estimatedToolResultTokens"`
	DistinctInspectedFiles    int            `json:"distinctInspectedFiles"`
	DistinctEditedFiles       int            `json:"distinctEditedFiles"`
	Compactions               int            `json:"compactions"`
	Retries                   int            `json:"retries"`
	FirstEvidence             *Milestone     `json:"firstEvidence"`
	FirstAttemptedMutation    *Milestone     `json:"firstAttemptedMutation"`
	FirstSuccessfulMutation   *Milestone     `json:"firstSuccessfulMutation"`
	FirstPassingTest          *Milestone     `json:"firstPassingTest"`
	Completion                *Milestone     `json:"completion"`
	RepeatedCalls             int            `json:"repeatedCalls"`
	RedundantReads            int            `json:"redundantReads"`
	SearchToRead              int            `json:"searchToRead"`
	SearchToEdit              int            `json:"searchToEdit"`
	EditOperations            int            `json:"editOperations"`
	AddedLines                int            `json:"addedLines"`
	RemovedLines              int            `json:"removedLines"`
	RevertProxies             int            `json:"revertProxies"`
	TestFixCycles             int            `json:"testFixCycles"`
	Outcome                   Outcome        `json:"outcome"`
	Missing                   []string       `json:"missing"`
}

// Report contains analyzed runs, grouped summaries, and descriptive comparisons.
type Report struct {
	Schema      string       `json:"schema"`
	Generated   time.Time    `json:"generatedAt"`
	Runs        []Run        `json:"runs"`
	Groups      []Group      `json:"groups"`
	Comparisons []Comparison `json:"comparisons,omitempty"`
}

// ComparisonReport contains an explicit target-minus-baseline cohort comparison.
type ComparisonReport struct {
	Schema    string     `json:"schema"`
	Generated time.Time  `json:"generatedAt"`
	GroupBy   string     `json:"groupBy"`
	Baseline  Group      `json:"baseline"`
	Target    Group      `json:"target"`
	Delta     Comparison `json:"delta"`
}

// Distribution summarizes a metric across one report group.
type Distribution struct {
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	P50    float64 `json:"p50"`
	P90    float64 `json:"p90"`
}

// Group summarizes runs sharing the requested reporting dimension.
type Group struct {
	Name                    string        `json:"name"`
	SampleSize              int           `json:"sampleSize"`
	UsageSampleSize         int           `json:"usageSampleSize"`
	OutcomeSampleSize       int           `json:"outcomeSampleSize"`
	Successful              int           `json:"successful"`
	SuccessRate             *float64      `json:"successRate"`
	Tokens                  *Distribution `json:"tokens"`
	Cost                    *Distribution `json:"cost"`
	ElapsedMS               Distribution  `json:"elapsedMs"`
	TokensPerSuccessfulTask *float64      `json:"tokensPerSuccessfulTask"`
	CostPerSuccessfulTask   *float64      `json:"costPerSuccessfulTask"`
	TimePerSuccessfulTask   *float64      `json:"timePerSuccessfulTask"`
}

// Comparison reports descriptive differences from one baseline group.
type Comparison struct {
	Baseline string           `json:"baseline"`
	Target   string           `json:"target"`
	Metrics  map[string]Delta `json:"metrics"`
}

// Delta is a target-minus-baseline difference; Percent is nil for a zero baseline.
type Delta struct {
	Absolute float64  `json:"absolute"`
	Percent  *float64 `json:"percent"`
}
