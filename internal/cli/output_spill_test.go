package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputSpillWritesJSONDescriptorAndContentAddressedArtifact(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(root, ".grepple", "output"))
	if err := os.WriteFile(filepath.Join(root, "grepple.json"), []byte(`{"output":{"spillThresholdBytes":64}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	chdirForConfigTest(t, root)
	payload := `{"schema":"example-v1","sources":{"parsed":2},"value":"` + strings.Repeat("x", 200) + `"}` + "\n"
	args := []string{"graph", "--json", "."}

	output := captureStdout(t, func() {
		if err := runWithOutputSpill(args, spillOptions{threshold: -1}, func() error {
			_, err := fmt.Fprint(os.Stdout, payload)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	})
	var descriptor spilledOutputDescriptor
	if err := json.Unmarshal([]byte(output), &descriptor); err != nil {
		t.Fatalf("descriptor %q: %v", output, err)
	}
	if descriptor.Schema != "grepple-artifact-v1" || descriptor.OriginalSchema != "example-v1" || descriptor.Format != "json" || !strings.Contains(descriptor.Rerun, "--no-spill") {
		t.Fatalf("descriptor = %+v", descriptor)
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(descriptor.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != payload {
		t.Fatalf("artifact content changed: %q", content)
	}
}

func TestOutputSpillKeepsSmallAndDisabledOutputOnStdout(t *testing.T) {
	root := t.TempDir()
	chdirForConfigTest(t, root)
	for _, test := range []struct {
		name    string
		options spillOptions
		value   string
	}{
		{name: "small", options: spillOptions{threshold: 64}, value: "small\n"},
		{name: "disabled", options: spillOptions{disabled: true}, value: strings.Repeat("x", 200)},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				if err := runWithOutputSpill(nil, test.options, func() error {
					_, err := fmt.Fprint(os.Stdout, test.value)
					return err
				}); err != nil {
					t.Fatal(err)
				}
			})
			if output != test.value {
				t.Fatalf("output = %q, want %q", output, test.value)
			}
		})
	}
}

func TestParseSpillOptionsRemovesGlobalFlags(t *testing.T) {
	args, options, err := parseSpillOptions([]string{"graph", "--spill-threshold-bytes", "1024", "--artifact-dir", "artifacts", "--json", "--no-spill", "."})
	if err != nil {
		t.Fatal(err)
	}
	if !options.disabled || options.threshold != 1024 || options.directory != "artifacts" || strings.Join(args, " ") != "graph --json ." {
		t.Fatalf("args=%q options=%+v", args, options)
	}
}

func TestOutputSpillUsesExplicitArtifactDirectoryWithContentAddressedName(t *testing.T) {
	root := t.TempDir()
	chdirForConfigTest(t, root)
	output := captureStdout(t, func() {
		if err := Run([]string{"languages", "--json", "--spill-threshold-bytes", "64", "--artifact-dir", "agent-output"}); err != nil {
			t.Fatal(err)
		}
	})
	var descriptor spilledOutputDescriptor
	if err := json.Unmarshal([]byte(output), &descriptor); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(descriptor.Path, "agent-output/") {
		t.Fatalf("descriptor path=%q", descriptor.Path)
	}
	name := strings.TrimSuffix(filepath.Base(descriptor.Path), filepath.Ext(descriptor.Path))
	if len(name) != 64 || descriptor.Digest != "sha256:"+name {
		t.Fatalf("descriptor=%+v", descriptor)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(descriptor.Path))); err != nil {
		t.Fatal(err)
	}
}

func TestOutputSpillRerunPreservesRepositoryScopeFlags(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GREPPLE_ARTIFACT_DIR", filepath.Join(root, ".grepple", "output"))
	chdirForConfigTest(t, root)
	output := captureStdout(t, func() {
		if err := Run([]string{"languages", "--json", "--production-only", "--no-config-ignore", "--spill-threshold-bytes", "64"}); err != nil {
			t.Fatal(err)
		}
	})
	var descriptor spilledOutputDescriptor
	if err := json.Unmarshal([]byte(output), &descriptor); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(descriptor.Rerun, "--production-only") || !strings.Contains(descriptor.Rerun, "--no-config-ignore") {
		t.Fatalf("rerun=%q", descriptor.Rerun)
	}
}
