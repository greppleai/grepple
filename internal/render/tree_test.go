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
