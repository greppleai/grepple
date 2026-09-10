package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestAnchoredLineOutputUsesConfiguredProvider(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile("sample.go", []byte("package sample\r\n// needle\r\nfunc run() {}\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(directory, "settings.json")
	settings := userSettings{Anchors: anchorSettings{
		DefaultProvider: "test",
		Providers: map[string]anchorProviderSettings{
			"test": {Command: []string{os.Args[0], "-test.run=TestAnchorProviderProcess"}},
		},
	}}
	writeJSONFile(t, settingsPath, settings)
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	t.Setenv("GREPPLE_TEST_ANCHOR_PROVIDER", "1")

	output := captureStdout(t, func() {
		if err := Run([]string{"--anchors", "--line-only", "needle", "sample.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if output != "sample.go\n\nA02│2│// needle\n" {
		t.Fatalf("anchored output = %q", output)
	}
}

func TestAnchorProviderProcess(_ *testing.T) {
	if os.Getenv("GREPPLE_TEST_ANCHOR_PROVIDER") != "1" {
		return
	}
	var request anchorProtocolRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(41)
	}
	response := anchorProtocolResponse{ProtocolVersion: request.ProtocolVersion}
	for _, file := range request.Files {
		output := anchorProtocolResponseFile{Path: file.Path, SHA256: file.SHA256}
		for _, line := range file.Lines {
			output.Anchors = append(output.Anchors, anchorProtocolLine{Line: line, Anchor: fmt.Sprintf("A%02d", line)})
		}
		response.Files = append(response.Files, output)
	}
	if err := json.NewEncoder(os.Stdout).Encode(response); err != nil {
		os.Exit(42)
	}
	os.Exit(0)
}

func TestEmptyAnchorRequestUsesJSONArrays(t *testing.T) {
	request, _, err := buildAnchorRequest(&cliOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"protocol_version":1,"files":[]}` {
		t.Fatalf("empty request = %s", encoded)
	}
}

func TestNoAnchorableResultsSkipProvider(t *testing.T) {
	t.Setenv("GREPPLE_SETTINGS", filepath.Join(t.TempDir(), "missing.json"))
	options := &cliOptions{Anchors: true}
	if err := prepareResultAnchors(options, nil); err != nil {
		t.Fatal(err)
	}
	if options.AnchorLines == nil {
		t.Fatal("expected initialized empty anchor lookup")
	}
}

func TestSegmentRendererEmitsHashLineContentRows(t *testing.T) {
	var output strings.Builder
	renderer := segmentRenderer{
		output:  newOutputWriter(&output),
		anchors: anchorLookup{"sample.go": {1: "abc", 2: "def"}},
	}
	result := api.FileResult{Path: "sample.go", Segments: []api.ResultSegment{{Kind: "lines", Start: 1, End: 2, Text: "first\nsecond"}}}
	if err := renderer.Render([]api.FileResult{result}); err != nil {
		t.Fatal(err)
	}
	if rendered := output.String(); rendered != "sample.go\n\nabc│1│first\ndef│2│second\n" {
		t.Fatalf("segment output = %q", rendered)
	}
}

func TestAnchorResponseRejectsMissingAndUnsafeAnchors(t *testing.T) {
	request := anchorProtocolRequest{ProtocolVersion: 1, Files: []anchorProtocolRequestFile{{Path: "/tmp/a", SHA256: "digest", Lines: []int{1}}}}
	paths := map[string]string{"/tmp/a": "a"}
	for _, anchor := range []string{"", "bad│anchor", "bad\nanchor"} {
		response := anchorProtocolResponse{ProtocolVersion: 1, Files: []anchorProtocolResponseFile{{Path: "/tmp/a", SHA256: "digest", Anchors: []anchorProtocolLine{{Line: 1, Anchor: anchor}}}}}
		if _, err := validateAnchorResponse(request, response, paths); err == nil {
			t.Fatalf("unsafe anchor %q was accepted", anchor)
		}
	}
}

func TestAnchorResponseRejectsDuplicateAnchorsWithinFile(t *testing.T) {
	request := anchorProtocolRequest{ProtocolVersion: 1, Files: []anchorProtocolRequestFile{{Path: "/tmp/a", SHA256: "digest", Lines: []int{1, 2}}}}
	response := anchorProtocolResponse{ProtocolVersion: 1, Files: []anchorProtocolResponseFile{{
		Path: "/tmp/a", SHA256: "digest", Anchors: []anchorProtocolLine{{Line: 1, Anchor: "same"}, {Line: 2, Anchor: "same"}},
	}}}
	if _, err := validateAnchorResponse(request, response, map[string]string{"/tmp/a": "a"}); err == nil {
		t.Fatal("duplicate anchors were accepted")
	}
}

func TestAnchorProviderSettingsRequireAbsoluteExecutable(t *testing.T) {
	err := validateAnchorProviderSettings(anchorProviderSettings{Command: []string{"node", "provider.mjs"}})
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("relative provider command error = %v", err)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}
