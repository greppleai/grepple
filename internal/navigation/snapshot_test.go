package navigation

import (
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func assertSnapshotParity(t *testing.T, graph parser.NavigationGraph) {
	t.Helper()
	snapshot, err := NewGraphOperations().PrepareQuery(graph)
	if err != nil {
		t.Fatal(err)
	}
	for _, direction := range []NavigationQueryDirection{NavigationQueryCallers, NavigationQueryCallees, NavigationQueryDependencies, NavigationQueryDependents, NavigationQueryImpact} {
		for depth := 1; depth <= 4; depth++ {
			for _, declaration := range graph.Declarations {
				assertSnapshotQueryParity(t, graph, snapshot, []string{declaration.ID}, direction, depth)
				assertSnapshotQueryParity(t, graph, snapshot, []string{declaration.ID, graph.Declarations[0].ID}, direction, depth)
			}
		}
	}
}

func TestSnapshotDifferentialRichFactsAndDuplicates(t *testing.T) {
	graph := navigationQueryFixture()
	graph.TypeDeclarations = append(graph.TypeDeclarations, parser.NavigationTypeDeclaration{Name: "Same", Path: "same/a.go"}, parser.NavigationTypeDeclaration{Name: "Same", Path: "same/b.go"})
	graph.Declarations = append(graph.Declarations, graph.Declarations[0])
	duplicate := graph.Calls[0]
	duplicate.CallerID = "missing"
	duplicate.TargetID = "unknown"
	graph.Calls = append(graph.Calls, duplicate)
	graph.Calls = append(graph.Calls, parser.NavigationCall{ID: "direct-with-extra-candidates", CallerID: "root", TargetID: "a", CandidateTargetIDs: []string{"candidate-one", "candidate-two"}}, parser.NavigationCall{ID: "empty-candidate-slice", CallerID: "root", TargetID: "a", CandidateTargetIDs: []string{}})
	assertSnapshotParity(t, graph)
}

func TestSnapshotDifferentialGeneratedGraphs(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	for trial := 0; trial < 12; trial++ {
		graph := parser.NavigationGraph{RepositoryRoots: []string{"repo"}}
		for n := 0; n < 8; n++ {
			id := fmt.Sprint(n)
			path := fmt.Sprintf("%d.go", n%3)
			graph.Declarations = append(graph.Declarations, parser.NavigationDeclaration{ID: id, Path: path, Receiver: "Widget"})
			graph.Fields = append(graph.Fields, parser.NavigationField{OwnerType: "Widget", Path: path})
			graph.Imports = append(graph.Imports, parser.NavigationImport{Path: path})
			graph.TypeDeclarations = append(graph.TypeDeclarations, parser.NavigationTypeDeclaration{Path: path})
			graph.TypeUsages = append(graph.TypeUsages, parser.NavigationTypeUsage{CallerID: id})
			graph.MemberAccesses = append(graph.MemberAccesses, parser.NavigationMemberAccess{CallerID: id})
		}
		for n := 0; n < 18; n++ {
			c := parser.NavigationCall{ID: fmt.Sprint(n), CallerID: fmt.Sprint(random.Intn(10)), Path: "call.go", Confidence: []string{"exact", "unique-terminal", "unresolved", "ambiguous"}[n%4]}
			if n%3 == 0 {
				c.CandidateTargetIDs = []string{fmt.Sprint(random.Intn(10)), fmt.Sprint(random.Intn(10))}
			} else {
				c.TargetID = fmt.Sprint(random.Intn(10))
			}
			graph.Calls = append(graph.Calls, c)
		}
		assertSnapshotParity(t, graph)
		assertPagedSnapshotParity(t, graph)
	}
}

func TestSnapshotConcurrentReadersAndErrors(t *testing.T) {
	snapshot, err := NewGraphOperations().PrepareQuery(navigationQueryFixture())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := snapshot.Query([]string{"root"}, NavigationQueryImpact, 3); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, test := range []struct {
		roots     []string
		direction NavigationQueryDirection
		depth     int
	}{{nil, NavigationQueryCallees, 1}, {[]string{"missing"}, NavigationQueryCallees, 1}, {[]string{"root"}, "bad", 1}, {[]string{"root"}, NavigationQueryCallees, 0}} {
		if _, err := snapshot.Query(test.roots, test.direction, test.depth); err == nil {
			t.Fatal("accepted invalid query")
		}
	}
}

func BenchmarkSnapshotNeighborhood(b *testing.B) {
	for _, size := range []int{1000, 10000} {
		graph := snapshotNeighborhoodFixture(size)
		snapshot, err := NewGraphOperations().PrepareQuery(graph)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("legacy-%d", size), func(b *testing.B) {
			benchmarkSnapshotQuery(b, func() (parser.NavigationGraph, error) {
				return queryNavigationGraph(graph, []string{"0"}, NavigationQueryCallees, 2)
			})
		})
		b.Run(fmt.Sprintf("snapshot-%d", size), func(b *testing.B) {
			benchmarkSnapshotQuery(b, func() (parser.NavigationGraph, error) {
				return snapshot.Query([]string{"0"}, NavigationQueryCallees, 2)
			})
		})
	}
}

func snapshotNeighborhoodFixture(size int) parser.NavigationGraph {
	graph := parser.NavigationGraph{}
	for n := 0; n < size; n++ {
		id := fmt.Sprint(n)
		graph.Declarations = append(graph.Declarations, parser.NavigationDeclaration{ID: id, Path: id + ".go"})
		graph.Calls = append(graph.Calls, parser.NavigationCall{ID: id, CallerID: id, TargetID: fmt.Sprint((n + 1) % size)})
	}
	return graph
}

func benchmarkSnapshotQuery(b *testing.B, query func() (parser.NavigationGraph, error)) {
	b.Helper()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		if _, err := query(); err != nil {
			b.Fatal(err)
		}
	}
}

func assertSnapshotQueryParity(t *testing.T, graph parser.NavigationGraph, snapshot QuerySnapshot, roots []string, direction NavigationQueryDirection, depth int) {
	t.Helper()
	expected, err := queryNavigationGraph(graph, roots, direction, depth)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := snapshot.Query(roots, direction, depth)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("parity %s depth=%d roots=%v\nactual=%+v\nexpected=%+v", direction, depth, roots, actual, expected)
	}
}
