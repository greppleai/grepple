//go:build duckdb

package main

import (
	"context"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

func filledColumnRecord[T any]() T {
	var record T
	value := reflect.ValueOf(&record).Elem()
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		switch field.Kind() {
		case reflect.String:
			field.SetString("UTF-8 α quote ' \" newline\n" + value.Type().Field(i).Name)
		case reflect.Int:
			field.SetInt(int64(100 + i))
		case reflect.Bool:
			field.SetBool(true)
		case reflect.Slice:
			slice := reflect.MakeSlice(field.Type(), 2, 2)
			slice.Index(0).SetString("one α")
			slice.Index(1).SetString("two ' \"")
			field.Set(slice)
		}
	}
	return record
}

func checkColumnCollection[T any](t *testing.T, s *columnDuckStore, key string, ids any, want []T) {
	t.Helper()
	got, err := columnFacts[T](context.Background(), s, key, ids)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %#v; want %#v", key, got, want)
	}
}

func TestColumnDuckEveryFieldRoundTrip(t *testing.T) {
	graph := parser.NavigationGraph{
		Declarations:     []parser.NavigationDeclaration{filledColumnRecord[parser.NavigationDeclaration]()},
		Calls:            []parser.NavigationCall{filledColumnRecord[parser.NavigationCall]()},
		Fields:           []parser.NavigationField{filledColumnRecord[parser.NavigationField]()},
		TypeDeclarations: []parser.NavigationTypeDeclaration{filledColumnRecord[parser.NavigationTypeDeclaration]()},
		Imports:          []parser.NavigationImport{filledColumnRecord[parser.NavigationImport]()},
		Exports:          []parser.NavigationExport{filledColumnRecord[parser.NavigationExport]()},
		TypeUsages:       []parser.NavigationTypeUsage{filledColumnRecord[parser.NavigationTypeUsage]()},
		MemberAccesses:   []parser.NavigationMemberAccess{filledColumnRecord[parser.NavigationMemberAccess]()},
		RepositoryRoots:  []string{"root α"},
	}
	// Preserve NULL versus an empty native list; neither may become a scalar.
	empty := graph.Calls[0]
	empty.CandidateTargetIDs = []string{}
	empty.ReceiverMembers = []string{}
	absent := graph.Calls[0]
	absent.CandidateTargetIDs = nil
	absent.ReceiverMembers = nil
	graph.Calls = append(graph.Calls, empty, absent)
	path := filepath.Join(t.TempDir(), "all-fields.duckdb")
	if _, err := buildColumnDuck(path, graph); err != nil {
		t.Fatal(err)
	}
	index, err := openColumnDuck(path)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	s := index.(*indexedQuery).store.(*columnDuckStore)
	checkColumnCollection(t, s, "defs", []string{graph.Declarations[0].ID}, graph.Declarations)
	checkColumnCollection(t, s, "calls", []int64{0, 1, 2}, graph.Calls)
	checkColumnCollection(t, s, "fields", []string{terminal(graph.Fields[0].OwnerType)}, graph.Fields)
	checkColumnCollection(t, s, "types", []string{graph.TypeDeclarations[0].Path}, graph.TypeDeclarations)
	checkColumnCollection(t, s, "imports", []string{graph.Imports[0].Path}, graph.Imports)
	checkColumnCollection(t, s, "exports", []string{graph.Exports[0].Path}, graph.Exports)
	checkColumnCollection(t, s, "usages", []string{graph.TypeUsages[0].CallerID}, graph.TypeUsages)
	checkColumnCollection(t, s, "members", []string{graph.MemberAccesses[0].CallerID}, graph.MemberAccesses)
	if !reflect.DeepEqual(s.roots, graph.RepositoryRoots) {
		t.Fatal("repository roots changed")
	}
}

func TestColumnDuckConcurrentReaders(t *testing.T) {
	graph := parityFixture()
	path := filepath.Join(t.TempDir(), "readers.duckdb")
	if _, err := buildColumnDuck(path, graph); err != nil {
		t.Fatal(err)
	}
	index, err := openColumnDuck(path)
	if err != nil {
		t.Fatal(err)
	}
	defer index.Close()
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func(n int) {
			defer group.Done()
			q := request{Roots: []string{"root"}, Direction: navigation.NavigationQueryImpact, Depth: 1 + n%4}
			got, err := index.Query(context.Background(), q)
			if err != nil {
				t.Error(err)
				return
			}
			want, err := navigation.NewGraphOperations().Query(graph, q.Roots, q.Direction, q.Depth)
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Error("concurrent query changed result")
			}
		}(i)
	}
	group.Wait()
}
