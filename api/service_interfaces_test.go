package api

import (
	"reflect"
	"strings"
	"testing"
)

func TestExecutionHandlesAreInterfacesAndWireResultsRemainValues(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"navigation": reflect.TypeOf((*IndexedNavigation)(nil)).Elem(),
		"plan":       reflect.TypeOf((*SearchPlan)(nil)).Elem(),
		"batch":      reflect.TypeOf((*SearchBatch)(nil)).Elem(),
		"filter":     reflect.TypeOf((*SearchRepoFilter)(nil)).Elem(),
		"policy":     reflect.TypeOf((*BoundaryPolicy)(nil)).Elem(),
	} {
		if typ.Kind() != reflect.Interface {
			t.Errorf("%s execution handle is %s, not an interface", name, typ.Kind())
		}
	}
	if got := reflect.TypeOf(SearchResponse{}).Kind(); got != reflect.Struct {
		t.Fatalf("JSON response kind=%s, want concrete struct", got)
	}
}

func TestSearchPlanCopiesOptionsAndEnforcesLimitWithoutMutatingOriginal(t *testing.T) {
	options := SearchPlanOptions{
		Query: "needle", Globs: []string{"*.go"}, Repo: []string{"owner/repo"},
		ExcludeRepo: []string{"owner/other"}, IgnorePaths: []string{"vendor/**"},
	}
	plan := NewSearchPlan(options)
	options.Globs[0], options.Repo[0], options.ExcludeRepo[0], options.IgnorePaths[0] = "changed", "changed", "changed", "changed"
	first := plan.Options()
	if first.Globs[0] != "*.go" || first.Repo[0] != "owner/repo" || first.ExcludeRepo[0] != "owner/other" || first.IgnorePaths[0] != "vendor/**" {
		t.Fatalf("plan retained caller-owned mutable slices: %+v", first)
	}
	first.Globs[0] = "changed again"
	if plan.Options().Globs[0] != "*.go" {
		t.Fatal("options view mutated the stored plan")
	}
	limited := EnforceSearchPageLimit(plan, SearchRequest{})
	if limited.Options().Limit != DefaultSearchPageLimit || plan.Options().Limit != 0 {
		t.Fatal("enforcing the public page limit mutated the original plan")
	}
}

type forgedBoundaryPolicy struct{}

func (forgedBoundaryPolicy) isBoundaryPolicy() {}

func TestAnalysisRejectsUnrecognizedPolicyImplementation(t *testing.T) {
	request := AnalysisRequest{Operation: AnalysisBoundaries, MinOccurrences: 1}
	sources := []AnalysisSource{{Path: "sample.go", Content: []byte("package sample\nfunc Run() {}\n")}}
	if _, complete, err := AnalyzeSources(request, sources, forgedBoundaryPolicy{}, ""); err == nil || complete || !strings.Contains(err.Error(), "invalid boundary policy implementation") {
		t.Fatalf("unrecognized policy must fail closed: complete=%t err=%v", complete, err)
	}
	if artifact, err := DecodeNavigationArtifact([]byte("invalid")); err == nil || artifact != nil {
		t.Fatalf("invalid artifact must not return a handle: artifact=%v err=%v", artifact, err)
	}
}
