package search

import (
	"reflect"
	"testing"

	boundaryanalysis "github.com/greppleai/grepple/internal/boundaryanalysis"
	"github.com/greppleai/grepple/parser"
)

func TestBoundaryPolicyClassifiesLayersContainmentAndReviewedSignals(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "target", Name: "Run", Language: "go", Path: "internal/store/run.go"},
			{ID: "one", Name: "One", Language: "go", Path: "service/one.go", Start: 10},
			{ID: "two", Name: "Two", Language: "go", Path: "service/two.go", Start: 20},
		},
		Calls: []parser.NavigationCall{
			{CallerID: "one", TargetID: "target", Language: "go"}, {CallerID: "one", TargetID: "target", Language: "go"},
			{CallerID: "two", TargetID: "target", Language: "go"}, {CallerID: "two", TargetID: "target", Language: "go"},
		},
	}
	policy := BoundaryPolicy{
		Schema:          BoundaryPolicySchema,
		Layers:          []BoundaryLayer{{Name: "storage", Paths: []string{"internal/store"}}, {Name: "service", Paths: []string{"service"}}},
		Containments:    []BoundaryContainmentRule{{Name: "store-internal", OwnerPaths: []string{"internal/store"}, ConsumerPaths: []string{"internal/store"}}},
		Classifications: []BoundaryPathClassification{{Category: "utility-hub", Paths: []string{"internal/store"}}},
	}
	candidates, err := AnalyzeBoundariesWithPolicy(graph, 2, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates=%#v", candidates)
	}
	candidate := candidates[0]
	if candidate.Spread != BoundarySpreadCrossLayer || candidate.Containment != BoundaryContainmentEscaped || candidate.Risk != BoundaryRiskInformational {
		t.Fatalf("classification=%#v", candidate)
	}
	if !reflect.DeepEqual(candidate.Signals, []string{"misplaced-function", "repeated-protocol", "utility-hub"}) {
		t.Fatalf("signals=%v", candidate.Signals)
	}
}

func TestBoundaryPolicyReportsResolvedFacadeBypass(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "implementation", Name: "Execute", Language: "go", Path: "internal/engine/run.go", Start: 5},
			{ID: "facade", Name: "Run", Language: "go", Path: "engine/engine.go", Start: 7},
			{ID: "client", Name: "Client", Language: "go", Path: "client/client.go", Start: 9},
		},
		Calls: []parser.NavigationCall{
			{CallerID: "facade", TargetID: "implementation", Confidence: "import-resolved", Line: 8},
			{CallerID: "client", TargetID: "implementation", Confidence: "import-resolved", Line: 10},
		},
	}
	policy := BoundaryPolicy{Schema: BoundaryPolicySchema, Facades: []BoundaryFacadeRule{{Name: "engine", FacadePaths: []string{"engine/**"}, ImplementationPaths: []string{"internal/**"}}}}
	bypasses := AnalyzeFacadeBypasses(graph, policy)
	if len(bypasses) != 1 || bypasses[0].Caller.ID != "client" || bypasses[0].Target.ID != "implementation" {
		t.Fatalf("bypasses=%#v", bypasses)
	}
}

func TestBoundaryTypeFieldsExposeRepresentationAndTransitiveAPI(t *testing.T) {
	graph := parser.NavigationGraph{Fields: []parser.NavigationField{
		{OwnerType: "First", Name: "Node", Type: "Node", ImportPath: "github.com/vendor/tree", Language: "go", Path: "one.go", Package: "sample", Line: 3, Visibility: parser.NavigationVisibilityPublic},
		{OwnerType: "Second", Name: "Node", Type: "Node", ImportPath: "github.com/vendor/tree", Language: "go", Path: "two.go", Package: "sample", Line: 4, Visibility: parser.NavigationVisibilityPublic},
	}}
	spreads, err := AnalyzeTypeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(spreads) != 1 {
		t.Fatalf("spreads=%#v", spreads)
	}
	spread := spreads[0]
	if spread.Roles.Fields != 2 || spread.Surfaces.FieldRepresentation != 2 || spread.Surfaces.PublicAPI != 2 || len(spread.PublicExposures) != 2 || spread.Spread != BoundarySpreadPublicAPI || spread.Risk != BoundaryRiskCritical {
		t.Fatalf("field spread=%#v", spread)
	}
	if !reflect.DeepEqual(spread.Reasons, []string{"third-party-public-api", "transitive-public-exposure"}) {
		t.Fatalf("reasons=%v", spread.Reasons)
	}
}

func TestBoundaryWorkflowFieldSurfaceRequiresOwnedFieldFact(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "a", Name: "Parser.Parse", Receiver: "Parser", Language: "go", Path: "owner.go"},
			{ID: "b", Name: "Parser.Validate", Receiver: "Parser", Language: "go", Path: "owner.go"},
			{ID: "one", Name: "One", Language: "go", Path: "one.go"},
			{ID: "two", Name: "Two", Language: "go", Path: "two.go"},
		},
		Calls:          []parser.NavigationCall{{CallerID: "one", TargetID: "a"}, {CallerID: "one", TargetID: "b"}, {CallerID: "two", TargetID: "a"}, {CallerID: "two", TargetID: "b"}},
		Fields:         []parser.NavigationField{{OwnerType: "Parser", Name: "State", Type: "string", Language: "go", Path: "owner.go"}},
		MemberAccesses: []parser.NavigationMemberAccess{{CallerID: "one", ReceiverType: "Parser", Member: "State", Language: "go"}, {CallerID: "two", ReceiverType: "Parser", Member: "Missing", Language: "go"}},
	}
	candidates, err := AnalyzeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ExternalFieldSurface != (BoundarySurface{External: 1, Declared: 1}) {
		t.Fatalf("field surface=%#v", candidates)
	}
	graph.Fields = nil
	withoutFacts, err := AnalyzeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if withoutFacts[0].ExternalFieldSurface != (BoundarySurface{}) {
		t.Fatalf("unowned field leaked into surface=%#v", withoutFacts[0].ExternalFieldSurface)
	}
}

func TestBoundaryLocalTypeIncludesOwnerPackageInSpread(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations: []parser.NavigationDeclaration{
			{ID: "owner", Name: "Widget.Run", Kind: "method", Language: "go", Path: "model/widget.go"},
			{ID: "one", Name: "one", Language: "go", Path: "service/one.go"},
			{ID: "two", Name: "two", Language: "go", Path: "service/two.go"},
		},
		TypeUsages: []parser.NavigationTypeUsage{{CallerID: "one", Type: "Widget", Role: "local", Language: "go", Path: "service/one.go"}, {CallerID: "two", Type: "Widget", Role: "local", Language: "go", Path: "service/two.go"}},
	}
	spreads, err := AnalyzeTypeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(spreads) != 1 || spreads[0].Spread != BoundarySpreadCrossPackage || spreads[0].Risk != BoundaryRiskLow || spreads[0].Containment != BoundaryContainmentUnknown {
		t.Fatalf("spread=%#v", spreads)
	}
}

func TestBoundaryPolicyDownranksReviewedIntentionalRolesWithoutHidingEvidence(t *testing.T) {
	for _, category := range []string{"declarative-configuration", "lifecycle-cleanup", "adapter-protocol"} {
		t.Run(category, func(t *testing.T) {
			graph := parser.NavigationGraph{
				Declarations: []parser.NavigationDeclaration{{ID: "one", Name: "one", Language: "go", Path: "config/one.go"}, {ID: "two", Name: "two", Language: "go", Path: "config/two.go"}},
				TypeUsages:   []parser.NavigationTypeUsage{{CallerID: "one", Type: "Client", ImportPath: "github.com/vendor/client", Role: "local", Language: "go", Path: "config/one.go"}, {CallerID: "two", Type: "Client", ImportPath: "github.com/vendor/client", Role: "local", Language: "go", Path: "config/two.go"}},
			}
			policy := BoundaryPolicy{Classifications: []BoundaryPathClassification{{Category: category, Paths: []string{"config"}}}}
			spreads, err := AnalyzeTypeBoundariesWithPolicy(graph, 2, policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(spreads) != 1 || spreads[0].Risk != BoundaryRiskMedium || !boundaryanalysis.HasCategory(spreads[0].Reasons, "repository-approved-usage-role") || len(spreads[0].UsageDetails) != 2 {
				t.Fatalf("%s spread=%#v", category, spreads)
			}
		})
	}
}

func TestBoundaryParallelAbstractionSignalRequiresMatchingReviewedSurface(t *testing.T) {
	graph := parser.NavigationGraph{}
	for _, owner := range []string{"one/owner.go", "two/owner.go"} {
		prefix := owner[:3]
		graph.Declarations = append(graph.Declarations,
			parser.NavigationDeclaration{ID: prefix + "a", Name: "A", Language: "go", Path: owner},
			parser.NavigationDeclaration{ID: prefix + "b", Name: "B", Language: "go", Path: owner},
			parser.NavigationDeclaration{ID: prefix + "1", Name: "One", Language: "go", Path: prefix + "/first.go"},
			parser.NavigationDeclaration{ID: prefix + "2", Name: "Two", Language: "go", Path: prefix + "/second.go"},
		)
		graph.Calls = append(graph.Calls,
			parser.NavigationCall{CallerID: prefix + "1", TargetID: prefix + "a"}, parser.NavigationCall{CallerID: prefix + "1", TargetID: prefix + "b"},
			parser.NavigationCall{CallerID: prefix + "2", TargetID: prefix + "a"}, parser.NavigationCall{CallerID: prefix + "2", TargetID: prefix + "b"},
		)
	}
	candidates, err := AnalyzeBoundaries(graph, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || !boundaryanalysis.HasCategory(candidates[0].Signals, "parallel-abstraction") || !boundaryanalysis.HasCategory(candidates[1].Signals, "parallel-abstraction") {
		t.Fatalf("parallel candidates=%#v", candidates)
	}
}
