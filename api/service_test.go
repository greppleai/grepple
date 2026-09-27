package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchFacadeProjectsWithoutExposingRawMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Target() { // NEEDLE\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	query := "NEEDLE"
	request := SearchRequest{Query: &query}
	plan, err := ResolveSearch(request)
	if err != nil {
		t.Fatal(err)
	}
	plan = EnforceSearchPageLimit(plan, request)
	if plan.Options().Limit != DefaultSearchPageLimit {
		t.Fatalf("unexpected public page limit: %d", plan.Options().Limit)
	}
	batch, err := SearchFiles(plan, []string{path})
	if err != nil || batch.Len() != 1 {
		t.Fatalf("candidate scan: %v matches=%d", err, batch.Len())
	}
	results := ProjectSearchResults(batch, 0, 0, true)
	if len(results) != 1 || len(results[0].Matches) == 0 {
		t.Fatalf("wire results=%+v", results)
	}
	options := plan.Options()
	options.At = path + ":2"
	plan = NewSearchPlan(options)
	at, err := SearchAt(plan)
	if err != nil || at.Len() != 1 || len(ProjectSearchResults(at, 0, 0, true)) != 1 {
		t.Fatalf("exact range: %v matches=%d", err, at.Len())
	}
}

func TestNavigationArtifactFacadeIsOpaqueAndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte("package sample\ntype Public struct{}\nfunc Exported() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, stats := BuildNavigationArtifact([]string{path})
	types, declarations := artifactTypes(index), artifactDeclarations(index)
	if stats.Failed != 0 || stats.Parsed != 1 || len(types) != 1 || len(declarations) != 1 {
		t.Fatalf("navigation facts: stats=%+v types=%+v declarations=%+v", stats, types, declarations)
	}
	types[0].Name = "changed"
	if artifactTypes(index)[0].Name != "Public" {
		t.Fatal("visitor value mutated the stored graph")
	}
	content, err := index.Encode(strings.Repeat("a", 64), false)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeNavigationArtifact(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifactDeclarations(decoded)) != 1 || artifactTypes(decoded)[0].Name != "Public" {
		t.Fatalf("decoded graph=%+v", artifactTypes(decoded))
	}
}

func artifactTypes(index IndexedNavigation) []ArtifactTypeDeclaration {
	var declarations []ArtifactTypeDeclaration
	index.VisitTypeDeclarations(func(declaration ArtifactTypeDeclaration) { declarations = append(declarations, declaration) })
	return declarations
}

func artifactDeclarations(index IndexedNavigation) []ArtifactDeclaration {
	var declarations []ArtifactDeclaration
	index.VisitDeclarations(func(declaration ArtifactDeclaration) { declarations = append(declarations, declaration) })
	return declarations
}

func TestSourceLineRangeFacadeRetainsAuthoritativeEOFMiss(t *testing.T) {
	resolved, err := ResolveSourceLineRange(8, 9, 2)
	if resolved.Outcome != LineRangeFullMiss || !IsOutsideSourceLineRange(err) {
		t.Fatalf("full miss: result=%+v err=%v", resolved, err)
	}
	partial, err := ResolveSourceLineRange(2, 9, 2)
	if err != nil || partial.Outcome != LineRangePartialMiss || partial.ReturnedEnd != 2 || partial.Warning == "" {
		t.Fatalf("partial miss: result=%+v err=%v", partial, err)
	}
}

func TestSourceFlowFacadeGeneratesAndBoundsDiagram(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc Run() { Next() }\nfunc Next() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	diagram, err := GenerateSourceFlow(root, "Run", 1, 10)
	if err != nil || !strings.Contains(diagram, "flowchart") || !strings.Contains(diagram, "Run") {
		t.Fatalf("source flow=%q err=%v", diagram, err)
	}
	if _, err := GenerateSourceFlow(root, "Run", -1, 10); err == nil {
		t.Fatal("negative depth was accepted")
	}
	if _, err := GenerateSourceFlow(root, "Unknown", 1, 10); err == nil {
		t.Fatal("missing entry was accepted")
	}
}
