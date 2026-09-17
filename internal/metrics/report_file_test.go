package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadReportAcceptsOnlyCanonicalGeneratedJSON(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	report := BuildReport([]Run{{
		RunID: "run-1", Agent: "pi", StartedAt: start, EndedAt: start.Add(time.Second),
		Usage: Usage{TotalTokens: 10}, Outcome: Outcome{Status: "success"},
	}})
	path := filepath.Join(t.TempDir(), "report.json")
	writeReportTestJSON(t, path, report)

	decoded, err := ReadReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, report) {
		t.Fatalf("decoded report differs:\n%#v\n%#v", decoded, report)
	}

	unknown := strings.TrimSuffix(string(mustMarshalReport(t, report)), "}") + `,"repository":"secret"}`
	if err := os.WriteFile(path, []byte(unknown), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(path); err == nil {
		t.Fatal("expected unknown-field rejection")
	}

	report.Groups[0].SampleSize = 99
	writeReportTestJSON(t, path, report)
	if _, err := ReadReport(path); err == nil {
		t.Fatal("expected noncanonical report rejection")
	}
}

func writeReportTestJSON(t *testing.T, path string, report Report) {
	t.Helper()
	if err := os.WriteFile(path, mustMarshalReport(t, report), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustMarshalReport(t *testing.T, report Report) []byte {
	t.Helper()
	content, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
