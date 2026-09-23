package architecture

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDirectoryTextGroupsRelationsWithoutRepeatingDirectoryInventory(t *testing.T) {
	root := writeArchitectureRelationsFixture(t)
	writeArchitectureFixture(t, root, "orphan/isolated.go", "package orphan\nfunc Isolated() {}\n")
	writeArchitectureFixture(t, root, "worker/job.go", "package worker\nimport _ \"example.com/project/search\"\n")
	chdirForConfigTest(t, root)

	text := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--max-output-bytes", "0", "."}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{
		"architecture grepple-directory-architecture-v5", "sources=discovered:", "coverage imports=",
		"rulespec -> search\n  import", "  resolved-call", "  type-reference", "at=rulespec/rule.go:",
		"worker -> search\n  import", "\nunconnected orphan\n",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("human directory output missing %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "\nD ") || strings.Contains(text, "\nR ") || strings.Contains(text, "unconnected .") || strings.Contains(text, "unconnected rulespec") || strings.Contains(text, "unconnected search") {
		t.Fatalf("human directory output repeats the inventory or mislabels a connected directory:\n%s", text)
	}

	jsonText := captureStdout(t, func() {
		if err := runArchitecture([]string{"directory", "--json", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var report directoryArchitecture
	if err := json.Unmarshal([]byte(jsonText), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Directories) != 5 || len(report.Relations) != 4 || report.Relations[0].Evidence[0].Path == "" {
		t.Fatalf("JSON must retain the full inventory and relation evidence: %+v", report)
	}
}

func TestUnconnectedDirectoriesExcludeAncestorsOfHiddenChildren(t *testing.T) {
	visible := []architectureDirectory{{Path: "."}, {Path: "internal"}}
	all := append(append([]architectureDirectory{}, visible...), architectureDirectory{Path: "internal/owner"})
	var output strings.Builder
	write := func(format string, args ...any) bool { fmt.Fprintf(&output, format+"\n", args...); return true }
	if !renderUnconnectedArchitectureDirectories(write, visible, all, nil) || output.Len() != 0 {
		t.Fatalf("hidden descendants are not disconnected: %q", output.String())
	}
	if !renderUnconnectedArchitectureDirectories(write, all, all, nil) || output.String() != "unconnected internal/owner\n" {
		t.Fatalf("leaf without a relation must remain visible: %q", output.String())
	}
}
