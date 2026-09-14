package search

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestAnalyzeBoundariesFindsRepeatedExternalOwnerWorkflow(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "parse", Name: "Parser.Parse", Language: "go", Path: "parser.go"},
			{ID: "validate", Name: "Parser.Validate", Language: "go", Path: "parser.go"},
			{ID: "local", Name: "localWorkflow", Language: "go", Path: "parser.go"},
			{ID: "one", Name: "One", Language: "go", Path: "one.go", Start: 10},
			{ID: "two", Name: "Two", Language: "go", Path: "two.go", Start: 20},
		},
		Calls: []parser.NavigationCall{
			{CallerID: "local", TargetID: "parse", Language: "go"},
			{CallerID: "local", TargetID: "validate", Language: "go"},
			{CallerID: "one", TargetID: "parse", Language: "go"},
			{CallerID: "one", TargetID: "validate", Language: "go"},
			{CallerID: "two", TargetID: "parse", Language: "go"},
			{CallerID: "two", TargetID: "validate", Language: "go"},
		},
	}
	candidates, err := AnalyzeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates=%#v", candidates)
	}
	candidate := candidates[0]
	if candidate.OwnerFile != "parser.go" || candidate.Language != "go" || candidate.Risk != BoundaryRiskLow || !reflect.DeepEqual(candidate.Reasons, []string{"repeated-owner-file-workflow", "broad-owner-callable-surface"}) || candidate.Consumers != (BoundaryBreadth{Functions: 2, Files: 2, Packages: 1}) {
		t.Fatalf("candidate=%#v", candidate)
	}
	if len(candidate.CallableCoUsage) != 1 || !reflect.DeepEqual(candidate.CallableCoUsage[0].Interactions, []string{"Parse", "Validate"}) || candidate.CallableCoUsage[0].Occurrences != 2 {
		t.Fatalf("co-usage=%#v", candidate.CallableCoUsage)
	}
	if len(candidate.OrderedSequences) != 1 || !reflect.DeepEqual(candidate.OrderedSequences[0].Interactions, []string{"Parse", "Validate"}) {
		t.Fatalf("sequences=%#v", candidate.OrderedSequences)
	}
	for _, consumer := range candidate.ConsumerDetails {
		if consumer.Path == "parser.go" {
			t.Fatalf("owner-local caller leaked into candidate: %#v", candidate)
		}
	}
}

func TestAnalyzeBoundariesKeepsOwnerFilesAndLanguagesSeparate(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "go-a", Name: "A", Language: "go", Path: "go/owner.go"},
			{ID: "go-b", Name: "B", Language: "go", Path: "go/owner.go"},
			{ID: "go-one", Name: "One", Language: "go", Path: "go/one.go"},
			{ID: "go-two", Name: "Two", Language: "go", Path: "go/two.go"},
			{ID: "java-a", Name: "A", Language: "java", Path: "java/Owner.java"},
			{ID: "java-b", Name: "B", Language: "java", Path: "java/Owner.java"},
			{ID: "java-one", Name: "one", Language: "java", Path: "java/One.java"},
			{ID: "java-two", Name: "two", Language: "java", Path: "java/Two.java"},
		},
		Calls: []parser.NavigationCall{
			{CallerID: "go-one", TargetID: "go-a", Language: "go"}, {CallerID: "go-one", TargetID: "go-b", Language: "go"},
			{CallerID: "go-two", TargetID: "go-a", Language: "go"}, {CallerID: "go-two", TargetID: "go-b", Language: "go"},
			{CallerID: "java-one", TargetID: "java-a", Language: "java"}, {CallerID: "java-one", TargetID: "java-b", Language: "java"},
			{CallerID: "java-two", TargetID: "java-a", Language: "java"}, {CallerID: "java-two", TargetID: "java-b", Language: "java"},
		},
	}
	candidates, err := AnalyzeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].OwnerFile != "go/owner.go" || candidates[0].Language != "go" || candidates[1].OwnerFile != "java/Owner.java" || candidates[1].Language != "java" {
		t.Fatalf("candidates=%#v", candidates)
	}
}

func TestAnalyzeBoundariesUsesFileOwnershipForEveryLanguage(t *testing.T) {
	for _, language := range []string{"go", "java", "kotlin", "javascript", "typescript", "tsx", "python", "csharp", "c", "cpp", "rust", "shell"} {
		t.Run(language, func(t *testing.T) {
			graph := parser.NavigationGraph{
				Declarations: []parser.NavigationDeclaration{
					{ID: "parse", Name: "Parse", Language: language, Path: "owner/source"},
					{ID: "validate", Name: "Validate", Language: language, Path: "owner/source"},
					{ID: "one", Name: "One", Language: language, Path: "consumer/one"},
					{ID: "two", Name: "Two", Language: language, Path: "consumer/two"},
				},
				Calls: []parser.NavigationCall{
					{CallerID: "one", TargetID: "parse", Language: language}, {CallerID: "one", TargetID: "validate", Language: language},
					{CallerID: "two", TargetID: "parse", Language: language}, {CallerID: "two", TargetID: "validate", Language: language},
				},
			}
			candidates, err := AnalyzeBoundaries(graph, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(candidates) != 1 || candidates[0].OwnerFile != "owner/source" || candidates[0].Language != language {
				t.Fatalf("candidates=%#v", candidates)
			}
		})
	}
}

func TestAnalyzeBoundariesValidatesMinimum(t *testing.T) {
	if _, err := AnalyzeBoundaries(parser.NavigationGraph{}, 0); err == nil {
		t.Fatal("expected invalid minimum to fail")
	}
	if _, err := AnalyzeTypeBoundaries(parser.NavigationGraph{}, 0); err == nil {
		t.Fatal("expected invalid type minimum to fail")
	}
}

func TestAnalyzeTypeBoundariesFindsImportedAndOwnedTypeSpread(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "owner", Name: "Widget.Run", Kind: "method", Language: "go", Path: "widget.go"},
			{ID: "public", Name: "Public", Language: "go", Path: "one.go", Package: "sample", Visibility: parser.NavigationVisibilityPublic, Start: 10},
			{ID: "private", Name: "private", Language: "go", Path: "two.go", Package: "sample", Visibility: parser.NavigationVisibilityNonPublic, Start: 20},
			{ID: "test", Name: "TestNode", Language: "go", Path: "node_test.go", Package: "sample", Visibility: parser.NavigationVisibilityPublic, Start: 30},
		},
		TypeUsages: []parser.NavigationTypeUsage{
			{CallerID: "public", Type: "Node", ImportPath: "github.com/tree-sitter/go-tree-sitter", Role: "parameter", Language: "go", Path: "one.go", Line: 10},
			{CallerID: "private", Type: "Node", ImportPath: "github.com/tree-sitter/go-tree-sitter", Role: "local", Language: "go", Path: "two.go", Line: 21},
			{CallerID: "test", Type: "Node", ImportPath: "github.com/tree-sitter/go-tree-sitter", Role: "parameter", Language: "go", Path: "node_test.go", Line: 30},
			{CallerID: "public", Type: "Widget", Role: "parameter", Language: "go", Path: "one.go", Line: 10},
			{CallerID: "private", Type: "Widget", Role: "local", Language: "go", Path: "two.go", Line: 21},
			{CallerID: "public", Type: "string", Role: "parameter", Language: "go", Path: "one.go", Line: 10},
			{CallerID: "private", Type: "string", Role: "local", Language: "go", Path: "two.go", Line: 21},
		},
	}
	spreads, err := AnalyzeTypeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(spreads) != 2 {
		t.Fatalf("spreads=%#v", spreads)
	}
	external := spreads[0]
	if external.CanonicalType != "github.com/tree-sitter/go-tree-sitter.Node" || !external.External || external.Origin != BoundaryTypeOriginThirdParty || external.Risk != BoundaryRiskCritical || !reflect.DeepEqual(external.Reasons, []string{"third-party-public-api"}) || external.Consumers.Files != 3 || external.Production.Files != 2 || external.Tests.Files != 1 || len(external.PublicExposures) != 1 {
		t.Fatalf("external spread=%#v", external)
	}
	if external.Roles.Parameters != 2 || external.Roles.Locals != 1 {
		t.Fatalf("external roles=%#v", external.Roles)
	}
	owned := spreads[1]
	if owned.CanonicalType != "Widget" || owned.OwnerFile != "widget.go" || owned.External || owned.Origin != BoundaryTypeOriginLocal || owned.Risk != BoundaryRiskInformational || !reflect.DeepEqual(owned.Reasons, []string{"local-package-internal-spread"}) {
		t.Fatalf("owned spread=%#v", owned)
	}
}

func TestBoundaryTypeOriginClassification(t *testing.T) {
	context := boundaryDependencyContext{roots: []string{"example.com/app", "com.example.app"}}
	tests := []struct {
		language string
		path     string
		want     BoundaryTypeOrigin
	}{
		{"go", "context", BoundaryTypeOriginStandardLibrary},
		{"go", "io/fs", BoundaryTypeOriginStandardLibrary},
		{"go", "example.com/app/parser", BoundaryTypeOriginFirstParty},
		{"go", "github.com/tree-sitter/go-tree-sitter", BoundaryTypeOriginThirdParty},
		{"go", "corp/shared", BoundaryTypeOriginUnresolved},
		{"typescript", "../model", BoundaryTypeOriginFirstParty},
		{"typescript", "node:fs", BoundaryTypeOriginStandardLibrary},
		{"typescript", "@app/model", BoundaryTypeOriginUnresolved},
		{"java", "com.example.app.model", BoundaryTypeOriginFirstParty},
		{"java", "java.util", BoundaryTypeOriginStandardLibrary},
		{"kotlin", "kotlin.collections", BoundaryTypeOriginStandardLibrary},
		{"csharp", "System.IO", BoundaryTypeOriginStandardLibrary},
		{"python", "requests", BoundaryTypeOriginUnresolved},
	}
	for _, test := range tests {
		if got := boundaryTypeOriginForImport(test.language, test.path, context); got != test.want {
			t.Errorf("origin(%q, %q) = %q, want %q", test.language, test.path, got, test.want)
		}
	}
}

func TestClassifyTypeBoundaryRisk(t *testing.T) {
	public := []BoundaryTypeUsage{{Public: true}}
	tests := []struct {
		name   string
		spread BoundaryTypeSpread
		risk   BoundaryRisk
		reason string
	}{
		{"third-party public API", BoundaryTypeSpread{Origin: BoundaryTypeOriginThirdParty, Production: BoundaryBreadth{Files: 2}, PublicExposures: public}, BoundaryRiskCritical, "third-party-public-api"},
		{"third-party production", BoundaryTypeSpread{Origin: BoundaryTypeOriginThirdParty, Production: BoundaryBreadth{Files: 2}}, BoundaryRiskHigh, "third-party-production-spread"},
		{"first-party public API", BoundaryTypeSpread{Origin: BoundaryTypeOriginFirstParty, Production: BoundaryBreadth{Files: 2}, PublicExposures: public}, BoundaryRiskMedium, "first-party-public-api"},
		{"unresolved public API", BoundaryTypeSpread{Origin: BoundaryTypeOriginUnresolved, Production: BoundaryBreadth{Files: 2}, PublicExposures: public}, BoundaryRiskMedium, "unresolved-origin-public-api"},
		{"local package internal", BoundaryTypeSpread{Origin: BoundaryTypeOriginLocal, Production: BoundaryBreadth{Files: 2, Packages: 1}}, BoundaryRiskInformational, "local-package-internal-spread"},
		{"standard library", BoundaryTypeSpread{Origin: BoundaryTypeOriginStandardLibrary, Production: BoundaryBreadth{Files: 2}}, BoundaryRiskInformational, "standard-library-spread"},
		{"test only third-party", BoundaryTypeSpread{Origin: BoundaryTypeOriginThirdParty, Tests: BoundaryBreadth{Files: 2}}, BoundaryRiskInformational, "test-only-spread"},
	}
	for _, test := range tests {
		risk, reasons := classifyTypeBoundary(test.spread)
		if risk != test.risk || !reflect.DeepEqual(reasons, []string{test.reason}) {
			t.Errorf("%s: risk=%q reasons=%v, want %q [%s]", test.name, risk, reasons, test.risk, test.reason)
		}
	}
}
