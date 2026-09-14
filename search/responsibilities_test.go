package search

import (
	"reflect"
	"testing"

	"github.com/greppleai/grepple/parser"
)

func TestAnalyzeResponsibilitiesFindsRepeatedCrossTypeWorkflows(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "validate", Name: "Request.Validate", Container: "Request", Language: "go", Path: "model/request.go", Start: 2},
			{ID: "normalize", Name: "Request.Normalize", Receiver: "Request", Language: "go", Path: "model/request.go", Start: 6},
			{ID: "one", Name: "Create", Package: "handlers", Language: "go", Path: "handlers/create.go", Start: 10},
			{ID: "two", Name: "Update", Package: "handlers", Language: "go", Path: "handlers/update.go", Start: 20},
			{ID: "inside", Name: "Request.Internal", Receiver: "Request", Language: "go", Path: "model/request.go", Start: 10},
		},
		Calls: []parser.NavigationCall{
			{ID: "c1", CallerID: "one", TargetID: "validate", Name: "Validate"},
			{ID: "c2", CallerID: "one", TargetID: "normalize", Name: "Normalize"},
			{ID: "c3", CallerID: "two", TargetID: "validate", Name: "Validate"},
			{ID: "c4", CallerID: "two", TargetID: "normalize", Name: "Normalize"},
			{ID: "internal", CallerID: "inside", TargetID: "validate", Name: "Validate"},
		},
		MemberAccesses: []parser.NavigationMemberAccess{
			{CallerID: "one", ReceiverType: "Request", Member: "State", Operation: "read"},
			{CallerID: "two", ReceiverType: "Request", Member: "State", Operation: "read"},
		},
	}
	report, err := AnalyzeResponsibilities(graph, "Request", 2)
	if err != nil {
		t.Fatal(err)
	}

	if report.Consumers != (ResponsibilityBreadth{Functions: 2, Files: 2, Packages: 1}) {
		t.Fatalf("breadth=%#v", report.Consumers)
	}
	if report.ExternalMethodSurface != (ResponsibilitySurface{External: 2, Declared: 3}) {
		t.Fatalf("surface=%#v", report.ExternalMethodSurface)
	}
	if len(report.MethodCoUsage) != 1 || !reflect.DeepEqual(report.MethodCoUsage[0].Methods, []string{"Normalize", "Validate"}) || report.MethodCoUsage[0].Occurrences != 2 {
		t.Fatalf("co-usage=%#v", report.MethodCoUsage)
	}
	if len(report.OrderedSequences) != 1 || !reflect.DeepEqual(report.OrderedSequences[0].Methods, []string{"Validate", "Normalize"}) {
		t.Fatalf("sequences=%#v", report.OrderedSequences)
	}
	if len(report.MemberMethodCombinations) != 1 || !reflect.DeepEqual(report.MemberMethodCombinations[0].Methods, []string{"Normalize()", "State(read)", "Validate()"}) {
		t.Fatalf("member combinations=%#v", report.MemberMethodCombinations)
	}
}

func TestAnalyzeResponsibilitiesCountsExplicitTypeUsageWithoutMemberCalls(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "method", Name: "Client.Save", Container: "Client", Language: "java"},
			{ID: "caller", Name: "Run", Language: "java", Path: "src/Run.java", Start: 4},
		},
		TypeUsages: []parser.NavigationTypeUsage{{CallerID: "caller", Type: "Client", Language: "java", Path: "src/Run.java", Line: 4}},
	}
	report, err := AnalyzeResponsibilities(graph, "Client", 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.Consumers.Functions != 1 || report.ConsumerDetails[0].Name != "Run" {
		t.Fatalf("report=%#v", report)
	}
}

func TestAnalyzeResponsibilitiesUsesLanguageNeutralReceiverEvidence(t *testing.T) {
	for _, language := range []string{"go", "java", "kotlin", "javascript", "typescript", "tsx", "python", "csharp", "c", "cpp", "rust", "shell"} {
		t.Run(language, func(t *testing.T) {
			graph := parser.NavigationGraph{
				Declarations: []parser.NavigationDeclaration{
					{ID: "method", Name: "Client.Save", Container: "Client", Language: language},
					{ID: "caller", Name: "Run", Language: language, Path: "src/run", Start: 1},
				},
				Calls: []parser.NavigationCall{{CallerID: "caller", Name: "Save", ReceiverType: "Client"}},
			}
			report, err := AnalyzeResponsibilities(graph, "Client", 1)
			if err != nil {
				t.Fatal(err)
			}
			if report.Consumers.Functions != 1 || !reflect.DeepEqual(report.Languages, []string{language}) {
				t.Fatalf("report=%#v", report)
			}
		})
	}
}

func TestAnalyzeDirectoryResponsibilitiesRanksTypesAndKeepsLanguagesSeparate(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "go-method", Name: "Client.Save", Container: "Client", Language: "go"},
			{ID: "go-one", Name: "One", Language: "go", Path: "go/one.go"},
			{ID: "go-two", Name: "Two", Language: "go", Path: "go/two.go"},
			{ID: "java-method", Name: "Client.save", Container: "Client", Language: "java"},
			{ID: "java-one", Name: "one", Language: "java", Path: "java/One.java"},
		},
		Calls: []parser.NavigationCall{
			{CallerID: "go-one", TargetID: "go-method", Name: "Save", Language: "go"},
			{CallerID: "go-two", TargetID: "go-method", Name: "Save", Language: "go"},
			{CallerID: "java-one", TargetID: "java-method", Name: "save", Language: "java"},
		},
	}
	reports, err := AnalyzeDirectoryResponsibilities(graph, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 || reports[0].Consumers.Functions != 2 || !reflect.DeepEqual(reports[0].Languages, []string{"go"}) || !reflect.DeepEqual(reports[1].Languages, []string{"java"}) {
		t.Fatalf("reports=%#v", reports)
	}
}

func TestAnalyzeResponsibilitiesValidatesInputs(t *testing.T) {
	if _, err := AnalyzeResponsibilities(parser.NavigationGraph{}, "", 1); err == nil {
		t.Fatal("expected empty type to fail")
	}
	if _, err := AnalyzeResponsibilities(parser.NavigationGraph{}, "Thing", 0); err == nil {
		t.Fatal("expected invalid minimum to fail")
	}
	if _, err := AnalyzeResponsibilities(parser.NavigationGraph{}, "Thing", 1); err == nil {
		t.Fatal("expected absent type to fail")
	}
}
