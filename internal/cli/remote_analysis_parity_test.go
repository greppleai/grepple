package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	publicanalysis "github.com/greppleai/grepple/analysis"
)

func TestPublicAnalysisArchitectureMatchesLocalProjection(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"a.go":        "package sample\nfunc A() { B() }\n",
		"nested/b.go": "package nested\nfunc B() {}\n",
	}
	for path, content := range files {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
	local, err := buildDirectoryArchitecture(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]publicanalysis.Source, 0, len(files))
	for path, content := range files {
		sources = append(sources, publicanalysis.Source{Path: path, Content: []byte(content)})
	}
	universe, err := publicanalysis.NewUniverse(sources, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	remote := publicanalysis.BuildArchitecture(universe)
	localJSON, _ := json.Marshal(local)
	remoteJSON, _ := json.Marshal(remote)
	var localValue, remoteValue any
	if err := json.Unmarshal(localJSON, &localValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(remoteJSON, &remoteValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(localValue, remoteValue) {
		t.Fatalf("local and public architecture differ\nlocal=%s\npublic=%s", localJSON, remoteJSON)
	}
}

func TestArchitectureResponsibilitiesAreDeterministic(t *testing.T) {
	architecture := directoryArchitecture{Files: 2, Directories: []architectureDirectory{{Path: "z", Files: 1}, {Path: "a", Files: 1}}, Relations: []architectureRelation{{From: "a", To: "z", Count: 3}}}
	first := buildArchitectureResponsibilitiesOutput(architecture)
	second := buildArchitectureResponsibilitiesOutput(architecture)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("responsibilities differ: %s != %s", firstJSON, secondJSON)
	}
	if first.Responsibilities[0].Directory != "a" || first.Responsibilities[0].Outgoing != 3 || first.Responsibilities[1].Incoming != 3 {
		t.Fatalf("responsibilities=%+v", first.Responsibilities)
	}
}

func TestPublicAnalysisGraphMatchesLocalProjection(t *testing.T) {
	root := t.TempDir()
	content := "package sample\nfunc Target() {}\nfunc Caller() { Target() }\n"
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	local := buildNavigationGraphOutputFromPaths([]string{"main.go"}, 0)
	universe, err := publicanalysis.NewUniverse([]publicanalysis.Source{{Path: "main.go", Content: []byte(content)}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer universe.Close()
	remote, err := publicanalysis.BuildGraph(universe, nil)
	if err != nil {
		t.Fatal(err)
	}
	localJSON, _ := json.Marshal(local)
	remoteJSON, _ := json.Marshal(remote)
	var localValue, remoteValue any
	_ = json.Unmarshal(localJSON, &localValue)
	_ = json.Unmarshal(remoteJSON, &remoteValue)
	if !reflect.DeepEqual(localValue, remoteValue) {
		t.Fatalf("local and public graph differ\nlocal=%s\npublic=%s", localJSON, remoteJSON)
	}
}
