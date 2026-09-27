package render

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestTreeRendersDirectoryFirstHierarchy(t *testing.T) {
	data := api.TreeResponse{
		Repo: "owner/repo",
		Path: "src",
		Entries: []api.TreeEntry{
			{Path: "z.go"},
			{Path: "pkg/b.go"},
			{Path: "pkg/a.go"},
			{Path: "a.go"},
		},
	}
	var output bytes.Buffer
	if err := Tree(data, &output, false); err != nil {
		t.Fatal(err)
	}
	want := "owner/repo/src\n├── pkg/\n│   ├── a.go\n│   └── b.go\n├── a.go\n└── z.go\n"
	if output.String() != want {
		t.Fatalf("output=%q, want %q", output.String(), want)
	}
}

func TestTreeRendersJSON(t *testing.T) {
	data := api.TreeResponse{Repo: "owner/repo", MetadataStatus: "missing", Entries: []api.TreeEntry{{Path: "main.go", MetadataStatus: "stale", MetadataIssues: []string{"checksum mismatch"}}}}
	var output bytes.Buffer
	if err := Tree(data, &output, true); err != nil {
		t.Fatal(err)
	}
	var decoded api.TreeResponse
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Repo != data.Repo || decoded.MetadataStatus != "missing" || len(decoded.Entries) != 1 || decoded.Entries[0].Path != "main.go" || decoded.Entries[0].MetadataStatus != "stale" || len(decoded.Entries[0].MetadataIssues) != 1 {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestTreeRendersDirectoryDescriptions(t *testing.T) {
	data := api.TreeResponse{Repo: ".", Description: "Repository root.", Entries: []api.TreeEntry{{Path: "parser", Dir: true, Description: "Parses source."}}}
	var output bytes.Buffer
	if err := Tree(data, &output, false); err != nil {
		t.Fatal(err)
	}
	want := ". — Repository root.\n└── parser/ — Parses source.\n"
	if output.String() != want {
		t.Fatalf("output=%q want=%q", output.String(), want)
	}
}

func TestTreeRendersSortedAreaLabelsAndJSON(t *testing.T) {
	data := api.TreeResponse{Repo: ".", Areas: []string{"backend", "frontend"}, Entries: []api.TreeEntry{
		{Path: "pkg", Dir: true, Description: "Services.", Areas: []string{"backend", "shared"}},
		{Path: "main.go", Areas: []string{"frontend"}, MetadataStatus: "stale"},
	}}
	var text bytes.Buffer
	if err := Tree(data, &text, false); err != nil {
		t.Fatal(err)
	}
	want := ". [areas: backend,frontend]\n├── pkg/ — Services. [areas: backend,shared]\n└── main.go [areas: frontend] [metadata: stale]\n"
	if text.String() != want {
		t.Fatalf("tree=%q want=%q", text.String(), want)
	}
	var jsonText bytes.Buffer
	if err := Tree(data, &jsonText, true); err != nil {
		t.Fatal(err)
	}
	var decoded api.TreeResponse
	if err := json.Unmarshal(jsonText.Bytes(), &decoded); err != nil || len(decoded.Areas) != 2 || len(decoded.Entries[0].Areas) != 2 {
		t.Fatalf("JSON areas=%+v err=%v", decoded, err)
	}
}

func TestTreeRendersFileDescriptions(t *testing.T) {
	data := api.TreeResponse{Repo: ".", Entries: []api.TreeEntry{{Path: "main.go", Description: "Starts the application."}}}
	var output bytes.Buffer
	if err := Tree(data, &output, false); err != nil {
		t.Fatal(err)
	}
	want := ".\n└── main.go — Starts the application.\n"
	if output.String() != want {
		t.Fatalf("output=%q want=%q", output.String(), want)
	}
}

func TestTreeRendersStaleAndMissingMetadataStatuses(t *testing.T) {
	data := api.TreeResponse{
		Repo:           ".",
		Description:    "Repository root.",
		MetadataStatus: "stale",
		Entries: []api.TreeEntry{
			{Path: "pkg", Dir: true, MetadataStatus: "missing"},
			{Path: "main.go", Description: "Starts the application.", MetadataStatus: "stale"},
		},
	}
	var output bytes.Buffer
	if err := Tree(data, &output, false); err != nil {
		t.Fatal(err)
	}
	want := ". — Repository root. [metadata: stale]\n├── pkg/ [metadata: missing]\n└── main.go — Starts the application. [metadata: stale]\n"
	if output.String() != want {
		t.Fatalf("output=%q want=%q", output.String(), want)
	}
}
