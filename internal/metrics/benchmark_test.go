package metrics

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func BenchmarkAnalyzeSession(b *testing.B) {
	path := filepath.Join("testdata", "comprehensive-v3.jsonl")
	b.ReportAllocs()
	for range b.N {
		if _, err := AnalyzeFile(path, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuildReportLargeSessionSet(b *testing.B) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := make([]Run, 10_000)
	for index := range runs {
		runs[index] = Run{
			RunID:          strconv.Itoa(index),
			AssignedCohort: []string{"control", "grepple"}[index%2],
			StartedAt:      start,
			EndedAt:        start.Add(time.Duration(index%1000) * time.Millisecond),
			Usage:          Usage{TotalTokens: int64(index % 10_000), Cost: float64(index%1000) / 1000},
			Outcome:        Outcome{Status: []string{"success", "failure"}[index%2]},
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = BuildReport(runs, "cohort", start)
	}
}
