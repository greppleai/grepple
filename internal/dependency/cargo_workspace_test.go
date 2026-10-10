package dependency

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCargoInheritedVersionReproducesCodexDiscovery(t *testing.T) {
	root := t.TempDir()
	writeCargoFixture(t, filepath.Join(root, "Cargo.toml"), "[workspace]\nmembers = [\"cli\"]\n[workspace.package]\nversion = \"0.0.0\"\n")
	path := filepath.Join(root, "cli", "Cargo.toml")
	writeCargoFixture(t, path, "[package]\nname = \"codex-cli\"\nversion.workspace = true\nedition.workspace = true\nlicense.workspace = true\n[dependencies]\nanyhow = { workspace = true }\n")
	module, err := NewCargoResolver().DiscoverModule(path, "cli")
	if err != nil {
		t.Fatal(err)
	}
	if module == nil || module.Version != "0.0.0" || module.Module != "codex-cli" || module.Root != "cli" {
		t.Fatalf("module=%#v", module)
	}
	if _, err := NewCargoResolver().Resolve(path); err != nil {
		t.Fatal(err)
	}
}
func TestCargoWorkspaceVersionEvidenceBoundaries(t *testing.T) {
	for _, test := range []struct{ name, workspace, child, reference, want string }{
		{"literal", "[workspace.package]\nversion=\"1.2.3\"\n", "version=\"4.5.6\"", "", "4.5.6"},
		{"inherited", "[workspace.package]\nversion=\"1.2.3\"\n", "version.workspace=true", "", "1.2.3"},
		{"explicit", "[workspace.package]\nversion=\"1.2.3\"\n", "version.workspace=true", "workspace=\"..\"", "1.2.3"},
		{"outside", "[workspace.package]\nversion=\"1.2.3\"\n", "version.workspace=true", "workspace=\"../..\"", ""},
		{"false", "[workspace.package]\nversion=\"1.2.3\"\n", "version.workspace=false", "", ""},
		{"missing", "[workspace]\n", "version.workspace=true", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeCargoFixture(t, filepath.Join(root, "Cargo.toml"), test.workspace)
			path := filepath.Join(root, "crate", "Cargo.toml")
			writeCargoFixture(t, path, "[package]\nname=\"crate\"\n"+test.child+"\n"+test.reference+"\n")
			module, err := NewCargoResolver().DiscoverModule(path, "crate")
			if err != nil {
				t.Fatal(err)
			}
			assertCargoArtifactVersion(t, module, test.want)
		})
	}
}
func assertCargoArtifactVersion(t *testing.T, module *ArtifactModule, want string) {
	t.Helper()
	if want == "" {
		if module != nil {
			t.Fatal("invented workspace evidence")
		}
		return
	}
	if module == nil || module.Version != want {
		t.Fatalf("module=%#v", module)
	}
}
func TestCargoInheritanceDoesNotReadAncestorOrSymlinkOutsideRepository(t *testing.T) {
	ancestor := t.TempDir()
	writeCargoFixture(t, filepath.Join(ancestor, "Cargo.toml"), "[workspace.package]\nversion=\"9.9.9\"\n")
	root := filepath.Join(ancestor, "repo")
	path := filepath.Join(root, "crate", "Cargo.toml")
	writeCargoFixture(t, path, "[package]\nname=\"crate\"\nversion.workspace=true\n")
	resolver := NewCargoResolver()
	module, err := resolver.DiscoverModule(path, "crate")
	if err != nil || module != nil {
		t.Fatalf("ancestor evidence: %#v %v", module, err)
	}
	if err := os.Symlink(filepath.Join(ancestor, "Cargo.toml"), filepath.Join(root, "Cargo.toml")); err != nil {
		t.Fatal(err)
	}
	module, err = resolver.DiscoverModule(path, "crate")
	if err != nil || module != nil {
		t.Fatalf("symlink evidence: %#v %v", module, err)
	}
}
func TestCargoActualCopiedCodexManifests(t *testing.T) {
	root := os.Getenv("GREPPLE_CODEX_CARGO_FIXTURE")
	if root == "" {
		t.Skip("opt-in read-only copied Codex manifests")
	}
	module, err := NewCargoResolver().DiscoverModule(filepath.Join(root, "cli", "Cargo.toml"), "cli")
	if err != nil {
		t.Fatal(err)
	}
	if module == nil || module.Module != "codex-cli" || module.Version != "0.0.0" {
		t.Fatalf("module=%#v", module)
	}
}
func writeCargoFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
