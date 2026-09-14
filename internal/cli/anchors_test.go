package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greppleai/grepple/api"
)

func TestAnchorsEnabledBySettingsUseConfiguredProvider(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	if err := os.WriteFile("sample.go", []byte("package sample\r\n// needle\r\nfunc run() {}\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(directory, "settings.json")
	settings := userSettings{Anchors: anchorSettings{
		EnabledByDefault: true,
		DefaultProvider:  "test",
		Providers: map[string]anchorProviderSettings{
			"test": {Command: []string{os.Args[0], "-test.run=TestAnchorProviderProcess"}},
		},
	}}
	writeJSONFile(t, settingsPath, settings)
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	t.Setenv("GREPPLE_TEST_ANCHOR_PROVIDER", "1")

	output := captureStdout(t, func() {
		if err := Run([]string{"--line-only", "needle", "sample.go"}); err != nil {
			t.Fatal(err)
		}
	})
	if output != "sample.go\n\nA02│2│// needle\n" {
		t.Fatalf("anchored output = %q", output)
	}
}

func TestAnchorsDoctorHelpIsRecursive(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"help", "anchors", "doctor"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "temporary-file protocol round trip") || !strings.Contains(output, "--provider") || !strings.Contains(output, "--json") {
		t.Fatalf("anchor doctor help is incomplete:\n%s", output)
	}
}

func TestAnchorsDoctorReportsIdentityProtocolAndRoundTrip(t *testing.T) {
	directory := t.TempDir()
	settingsPath := filepath.Join(directory, "settings.json")
	writeJSONFile(t, settingsPath, userSettings{Anchors: anchorSettings{
		DefaultProvider: "test", Providers: map[string]anchorProviderSettings{
			"test": {Command: []string{os.Args[0], "-test.run=TestAnchorProviderProcess"}},
		},
	}})
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	t.Setenv("GREPPLE_TEST_ANCHOR_PROVIDER", "1")
	output := captureStdout(t, func() {
		if err := Run([]string{"anchors", "doctor"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"anchor doctor OK protocol=1", "provider: test", "[ok] executable", "[ok] round-trip", "response="} {
		if !strings.Contains(output, expected) {
			t.Fatalf("doctor output missing %q:\n%s", expected, output)
		}
	}

	jsonText := captureStdout(t, func() {
		if err := Run([]string{"anchors", "doctor", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var report anchorDoctorReport
	if err := json.Unmarshal([]byte(jsonText), &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != anchorDoctorSchema || !report.OK || report.Provider != "test" || report.ProtocolVersion != 1 || report.ResponseBytes == 0 {
		t.Fatalf("unexpected doctor JSON: %#v", report)
	}
}

func TestAnchorsDoctorGivesSetupGuidanceWithoutConfiguration(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "missing.json")
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	report := diagnoseAnchorProvider("")
	if report.OK || len(report.Guidance) == 0 || report.Checks[len(report.Checks)-1].Status != "failed" {
		t.Fatalf("missing configuration report: %#v", report)
	}
	if !strings.Contains(strings.Join(report.Guidance, "\n"), settingsPath) {
		t.Fatalf("guidance does not identify settings path: %#v", report.Guidance)
	}
}

func TestAnchorsDoctorReportsTimeout(t *testing.T) {
	directory := t.TempDir()
	settingsPath := filepath.Join(directory, "settings.json")
	writeJSONFile(t, settingsPath, userSettings{Anchors: anchorSettings{
		DefaultProvider: "slow", Providers: map[string]anchorProviderSettings{
			"slow": {Command: []string{os.Args[0], "-test.run=TestAnchorProviderProcess"}, TimeoutMS: 10},
		},
	}})
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	t.Setenv("GREPPLE_TEST_ANCHOR_PROVIDER", "1")
	t.Setenv("GREPPLE_TEST_ANCHOR_PROVIDER_SLOW", "1")
	report := diagnoseAnchorProvider("")
	last := report.Checks[len(report.Checks)-1]
	if report.OK || last.Name != "round-trip" || !strings.Contains(last.Detail, "timed out after 10ms") {
		t.Fatalf("timeout report: %#v", report)
	}
}

func TestAnchorsSetupPreviewsWithoutWriting(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	output := captureStdout(t, func() {
		if err := Run([]string{"anchors", "setup", "--provider", "test", "--command", os.Args[0], "--command-arg=-test.run=TestAnchorProviderProcess", "--set-default"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "no files changed") || !strings.Contains(output, `"default_provider": "test"`) || !strings.Contains(output, "rerun with --write") {
		t.Fatalf("setup preview is incomplete:\n%s", output)
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("preview changed settings: %v", err)
	}
}

func TestAnchorsSetupWritesUserSettingsExplicitly(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".grepple", "settings.json")
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	output := captureStdout(t, func() {
		if err := Run([]string{"anchors", "setup", "--provider", "test", "--command", os.Args[0], "--command-arg=-test.run=TestAnchorProviderProcess", "--timeout-ms", "2500", "--set-default", "--enable-by-default", "--write"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "anchor setup written") || !strings.Contains(output, "anchors doctor --provider test") {
		t.Fatalf("setup write output is incomplete:\n%s", output)
	}
	settings, err := loadUserSettings()
	if err != nil {
		t.Fatal(err)
	}
	provider := settings.Anchors.Providers["test"]
	if !settings.Anchors.EnabledByDefault || settings.Anchors.DefaultProvider != "test" || provider.TimeoutMS != 2500 || len(provider.Command) != 2 {
		t.Fatalf("written settings: %#v", settings)
	}
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("settings permissions = %o", info.Mode().Perm())
	}
}

func TestAnchorsSetupRequiresForceToReplaceProvider(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv("GREPPLE_SETTINGS", settingsPath)
	writeJSONFile(t, settingsPath, userSettings{Anchors: anchorSettings{Providers: map[string]anchorProviderSettings{"test": {Command: []string{os.Args[0]}, TimeoutMS: 10}}}})
	args := []string{"anchors", "setup", "--provider", "test", "--command", os.Args[0], "--timeout-ms", "20", "--write"}
	if err := Run(args); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("replacement error = %v", err)
	}
	args = append(args, "--force")
	if err := Run(args); err != nil {
		t.Fatal(err)
	}
	settings, err := loadUserSettings()
	if err != nil || settings.Anchors.Providers["test"].TimeoutMS != 20 {
		t.Fatalf("forced settings=%#v err=%v", settings, err)
	}
}

func TestAnchorsSetupHelpStatesWriteSafety(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"help", "anchors", "setup"}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(output, "preview is the default") || !strings.Contains(output, "--write") || !strings.Contains(output, "--force") {
		t.Fatalf("setup help omits safety contract:\n%s", output)
	}
}

func TestAnchorProviderProcess(_ *testing.T) {
	if os.Getenv("GREPPLE_TEST_ANCHOR_PROVIDER") != "1" {
		return
	}
	if os.Getenv("GREPPLE_TEST_ANCHOR_PROVIDER_SLOW") == "1" {
		time.Sleep(200 * time.Millisecond)
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
