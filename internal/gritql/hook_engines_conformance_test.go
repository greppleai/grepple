package gritql

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/greppleai/grepple/internal/parser"
)

type hookEngineFixture struct {
	SchemaVersion int              `json:"schema_version"`
	Suite         string           `json:"suite"`
	Provenance    Provenance       `json:"provenance"`
	Cases         []hookEngineCase `json:"cases"`
}

type hookEngineCase struct {
	Name     string             `json:"name"`
	Engine   string             `json:"engine"`
	Sources  map[string]string  `json:"sources"`
	Relation *hookRelationCase  `json:"relation,omitempty"`
	Metric   *hookMetricCase    `json:"metric,omitempty"`
	Expected hookEngineExpected `json:"expected"`
}

type hookRelationCase struct {
	LeftQuery           string   `json:"left_query"`
	RightQuery          string   `json:"right_query,omitempty"`
	PartitionQuery      string   `json:"partition_query,omitempty"`
	LeftKey             string   `json:"left_key"`
	LeftProjection      string   `json:"left_projection,omitempty"`
	RightKey            string   `json:"right_key"`
	RightDescendantKind string   `json:"right_descendant_kind,omitempty"`
	PartitionKey        string   `json:"partition_key,omitempty"`
	Scope               string   `json:"scope"`
	Mode                string   `json:"mode,omitempty"`
	LeftInclude         []string `json:"left_include,omitempty"`
	UniqueLeft          bool     `json:"unique_left,omitempty"`
	MaxFiles            int      `json:"max_files,omitempty"`
}

type hookMetricCase struct {
	Query string `json:"query"`
	Path  string `json:"path"`
}

type hookEngineExpected struct {
	Hits          []string       `json:"hits,omitempty"`
	Scores        map[string]int `json:"scores,omitempty"`
	Above         int            `json:"above,omitempty"`
	ErrorContains string         `json:"error_contains,omitempty"`
}

func TestHookEngineConformanceFixture(t *testing.T) {
	seen := map[string]bool{}
	for _, test := range loadHookEngineFixture(t) {
		if test.Name == "" || seen[test.Name] {
			t.Fatalf("duplicate or empty hook case %q", test.Name)
		}
		seen[test.Name] = true
		t.Run(test.Name, func(t *testing.T) { runHookEngineCase(t, test) })
	}
}

func loadHookEngineFixture(t *testing.T) []hookEngineCase {
	t.Helper()
	content, err := os.ReadFile("testdata/conformance/hook-engines/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	var fixture hookEngineFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || fixture.SchemaVersion != 1 || fixture.Suite != "hook-engines" || len(fixture.Cases) == 0 || fixture.Provenance.Kind == "" {
		t.Fatalf("invalid hook-engine fixture: %+v trailing=%v", fixture, err)
	}
	return fixture.Cases
}

func runHookEngineCase(t *testing.T, test hookEngineCase) {
	t.Helper()
	switch test.Engine {
	case "gritql-relational-v1":
		if test.Relation == nil || test.Metric != nil {
			t.Fatal("relation fixture must specify only relation")
		}
		runHookRelationConformance(t, test)
	case MetricsCompatibility:
		if test.Metric == nil || test.Relation != nil {
			t.Fatal("metric fixture must specify only metric")
		}
		runHookMetricConformance(t, test)
	default:
		t.Fatalf("unknown hook engine %q", test.Engine)
	}
}

func runHookRelationConformance(t *testing.T, test hookEngineCase) {
	t.Helper()
	config := test.Relation
	filesystem := fstest.MapFS{}
	var candidates []ScanCandidate
	for path, source := range test.Sources {
		filesystem[path] = &fstest.MapFile{Data: []byte(source)}
		candidates = append(candidates, ScanCandidate{Path: path, ReadPath: path})
	}
	slices.SortFunc(candidates, func(a, b ScanCandidate) int { return strings.Compare(a.Path, b.Path) })
	left := relationProgram(t, config.LeftQuery)
	right := left
	if config.RightQuery != "" {
		right = relationProgram(t, config.RightQuery)
	}
	spec := RelationSpec{
		Left: left, Right: right, LeftKey: RelationKey{Binding: config.LeftKey, Projection: config.LeftProjection},
		RightKey: RelationKey{Binding: config.RightKey, DescendantKind: config.RightDescendantKind},
		Scope:    config.Scope, Mode: config.Mode, LeftInclude: config.LeftInclude, UniqueLeft: config.UniqueLeft,
	}
	if config.PartitionQuery != "" {
		spec.Partition = relationProgram(t, config.PartitionQuery)
		spec.PartitionKey = RelationKey{Binding: config.PartitionKey}
	}
	hits, err := ScanFilesRelation(context.Background(), filesystem, candidates, spec, ScanOptions{MaxFiles: config.MaxFiles})
	if want := test.Expected.ErrorContains; want != "" {
		if err == nil || !strings.Contains(err.Error(), want) || len(hits) != 0 {
			t.Fatalf("relation hits=%+v error=%v want error containing %q", hits, err, want)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(hits))
	for _, hit := range hits {
		got = append(got, hit.Left.Path()+"|"+hit.Right.Path()+"|"+hit.KeyText)
	}
	slices.Sort(got)
	want := slices.Clone(test.Expected.Hits)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("relation hits=%q want %q", got, want)
	}
}

func runHookMetricConformance(t *testing.T, test hookEngineCase) {
	t.Helper()
	metric, err := CompileMetric([]byte(test.Metric.Query), CompileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if metric.Above != test.Expected.Above {
		t.Fatalf("metric threshold=%d want %d", metric.Above, test.Expected.Above)
	}
	content, ok := test.Sources[test.Metric.Path]
	if !ok || len(test.Sources) != 1 {
		t.Fatal("metric fixture requires exactly one source at metric.path")
	}
	document, err := parser.NewParser().Parse(metric.Spec.Scope.Language(), content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	results, err := AnalyzeMetrics(context.Background(), metric.Spec, document, EvaluateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]int, len(results))
	for _, result := range results {
		if _, duplicate := got[result.Name]; duplicate {
			t.Fatalf("duplicate metric scope %q", result.Name)
		}
		got[result.Name] = result.Score
	}
	if !reflect.DeepEqual(got, test.Expected.Scores) {
		t.Fatalf("metric scores=%v want %v", got, test.Expected.Scores)
	}
}
