package parser

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNavigationCacheProtobufRoundTripsNativeGraph(t *testing.T) {
	graph := NavigationGraph{
		Declarations:     []NavigationDeclaration{{ID: "declaration", Name: "Run", Signature: "func (s *Service) Run(value Input) Result", Kind: "method", Language: "go", Path: "", Container: "Service", Receiver: "Service", ResultType: "Result", ResultImportPath: "example/result", Package: "sample", PackageID: "example/sample", ModuleID: "example", Scope: "sample", Entrypoint: "process", Visibility: NavigationVisibilityPublic, VisibilityDetail: "exported", Start: 7, End: 12}},
		TypeDeclarations: []NavigationTypeDeclaration{{Name: "Service", Kind: "struct", Language: "go", Path: "", Container: "Outer", Package: "sample", PackageID: "example/sample", ModuleID: "example", Start: 2, End: 6}},
		Imports:          []NavigationImport{{Alias: "result", ImportPath: "example/result", Imported: "Result", Kind: "named", Scope: "sample", VisibilityDetail: "private", TargetPathHint: "result/result.go", Inline: true, Language: "go", Path: "", Line: 1, TargetPaths: []string{"result/result.go", "result/other.go"}}},
		Calls:            []NavigationCall{{ID: "call", CallerID: "declaration", TargetID: "target", CandidateTargetIDs: []string{"target-a", "target-b"}, Name: "Build", Display: "result.Build", Qualifier: "result", ImportPath: "example/result", ReceiverType: "Factory", ReceiverRootType: "Root", ReceiverRootImport: "example/root", ReceiverMembers: []string{"Factory", "Build"}, ReceiverFactory: "NewFactory", ReceiverFactoryImport: "example/factory", ResolvedName: "Build", Confidence: "import-resolved", Language: "go", Path: "", Line: 9, EnclosingStart: 7, EnclosingEnd: 12}},
		Exports:          []NavigationExport{{Name: "Run", LocalName: "localRun", ImportPath: "./run", ImportedName: "default", Scope: "sample", VisibilityDetail: "public", Language: "typescript", Path: "", Line: 3}},
		Fields:           []NavigationField{{OwnerType: "Service", Name: "Client", Type: "Client", ImportPath: "example/client", Language: "go", Path: "", Package: "sample", Line: 4, Visibility: NavigationVisibilityPublic, Embedded: true}},
		TypeUsages:       []NavigationTypeUsage{{CallerID: "declaration", Type: "Result", ImportPath: "example/result", Role: "result", Language: "go", Path: "", Line: 7}},
		MemberAccesses:   []NavigationMemberAccess{{ID: "access", CallerID: "declaration", ReceiverType: "Service", Receiver: "service", Member: "Client", Operation: "read", Language: "go", Path: "", Line: 8, StartByte: 123}},
		RepositoryRoots:  []string{"/workspace/repository"},
	}
	entry := navigationCacheEntry{Schema: navigationCacheSchema, Digest: "digest", Recovered: true, Graph: graph}
	encoded, err := marshalNavigationCacheEntry(entry)
	if err != nil {
		t.Fatal(err)
	}
	again, err := marshalNavigationCacheEntry(entry)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(encoded, again) {
		t.Fatal("protobuf navigation cache encoding is not deterministic")
	}
	decoded, err := unmarshalNavigationCacheEntry(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, entry) {
		t.Fatalf("protobuf round trip differs:\n got: %#v\nwant: %#v", decoded, entry)
	}
}

func TestNavigationCacheProtobufPreservesNilAndEmptySlices(t *testing.T) {
	graphs := []NavigationGraph{
		{},
		{Declarations: []NavigationDeclaration{}, Calls: []NavigationCall{}, RepositoryRoots: []string{}},
	}
	for _, graph := range graphs {
		entry := navigationCacheEntry{Schema: navigationCacheSchema, Digest: "digest", Graph: graph}
		encoded, err := marshalNavigationCacheEntry(entry)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := unmarshalNavigationCacheEntry(encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded.Graph, graph) {
			t.Fatalf("slice presence differs: got %#v want %#v", decoded.Graph, graph)
		}
	}
}

func TestNavigationCacheProtobufRejectsCorruption(t *testing.T) {
	entry := navigationCacheEntry{Schema: navigationCacheSchema, Digest: "digest", Graph: NavigationGraph{Declarations: []NavigationDeclaration{{Name: "Run"}}}}
	encoded, err := marshalNavigationCacheEntry(entry)
	if err != nil {
		t.Fatal(err)
	}
	encoded[len(encoded)-1] ^= 0xff
	if _, err := unmarshalNavigationCacheEntry(encoded); err == nil {
		t.Fatal("corrupt protobuf cache entry was accepted")
	}
}

func TestNavigationCacheProtobufCoversNativeFactFields(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		fields int
	}{
		{"declaration", NavigationDeclaration{}, 19},
		{"type declaration", NavigationTypeDeclaration{}, 10},
		{"import", NavigationImport{}, 12},
		{"call", NavigationCall{}, 21},
		{"export", NavigationExport{}, 9},
		{"field", NavigationField{}, 10},
		{"type usage", NavigationTypeUsage{}, 7},
		{"member access", NavigationMemberAccess{}, 10},
		{"graph", NavigationGraph{}, 9},
	}
	for _, test := range cases {
		if fields := reflect.TypeOf(test.value).NumField(); fields != test.fields {
			t.Fatalf("%s gained an unaccounted cache field: got %d fields, codec covers %d", test.name, fields, test.fields)
		}
	}
}

func TestCachedNavigationGraphRebuildsCorruptProtobufEntry(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(NavigationCacheDirectoryEnv, directory)
	cold, _, hit := cachedTestNavigationGraph(t, navigationCacheTestContent, "sample/main.go")
	if hit {
		t.Fatal("cold protobuf cache unexpectedly hit")
	}
	digest := navigationCacheDigest(navigationCacheTestContent, "go")
	path := filepath.Join(directory, digest+".pb")
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	rebuilt, _, hit := cachedTestNavigationGraph(t, navigationCacheTestContent, "sample/main.go")
	if hit || !reflect.DeepEqual(rebuilt, cold) {
		t.Fatalf("corrupt cache was not rebuilt with identical facts: hit=%v", hit)
	}
	warm, _, hit := cachedTestNavigationGraph(t, navigationCacheTestContent, "sample/main.go")
	if !hit || !reflect.DeepEqual(warm, cold) {
		t.Fatalf("rebuilt protobuf cache did not become reusable: hit=%v", hit)
	}
}

func TestCachedNavigationGraphWritesProtobufEntry(t *testing.T) {
	directory := t.TempDir()
	t.Setenv(NavigationCacheDirectoryEnv, directory)
	if _, _, hit, err := CachedNavigationGraph(navigationCacheTestContent, "go", "sample/main.go"); err != nil || hit {
		t.Fatalf("cold cache hit=%v err=%v", hit, err)
	}
	digest := navigationCacheDigest(navigationCacheTestContent, "go")
	if _, err := os.Stat(filepath.Join(directory, digest+".pb")); err != nil {
		t.Fatalf("protobuf cache entry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, digest+".json")); !os.IsNotExist(err) {
		t.Fatalf("legacy JSON cache entry unexpectedly exists: %v", err)
	}
}
