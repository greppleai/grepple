package agentskills

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func repositoryBundle(t *testing.T) Bundle {
	t.Helper()
	source, err := NewLocalSource(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := Load(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}
func TestCatalogMatchesRepositoryAndAllSkillDirectories(t *testing.T) {
	bundle := repositoryBundle(t)
	entries, err := os.ReadDir(filepath.Join("..", "..", ".agents", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(bundle.files) {
		t.Fatalf("uncatalogued skill directories: %d vs %d", len(entries), len(bundle.files))
	}
	for _, entry := range entries {
		if _, ok := bundle.files[entry.Name()]; !ok {
			t.Errorf("uncatalogued skill: %s", entry.Name())
		}
	}
	documentation, err := os.ReadFile(filepath.Join("..", "..", "docs", "write.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(documentation, bundle.files["grepple-write"]["references/write.md"]) {
		t.Fatal("bundled write reference is stale")
	}
}
func TestReleaseTagNeverFallsBackToMovingRefs(t *testing.T) {
	for _, version := range []string{"dev", "unknown", "main", "latest", "v0.0.5-dirty", "v0.0.5-3-gabcdef", "0.0.5-3-gabcdef-dirty", "v0.0.5/../main", "v01.2.3"} {
		if _, err := ReleaseTag(version); err == nil {
			t.Errorf("accepted %s", version)
		}
	}
	for _, version := range []string{"0.0.5", "v0.0.5", "v1.2.3-rc.1"} {
		tag, err := ReleaseTag(version)
		if err != nil || tag != "v"+strings.TrimPrefix(version, "v") {
			t.Errorf("%s => %s, %v", version, tag, err)
		}
	}
}
func TestOwnedNamesRetainHistoricalRegistry(t *testing.T) {
	names := OwnedNames()
	owned := map[string]bool{}
	for _, name := range names {
		owned[name] = true
	}
	for _, name := range []string{"architecture-boundary-review", "architecture-diagram-workflow", "architecture-lookup-discovery", "change-impact-analysis", "delegated-research-with-ask", "local-grep-and-search", "remote-grep-and-search", "structural-pattern-audit", "grepple-write"} {
		if !owned[name] {
			t.Errorf("historical name removed: %s", name)
		}
	}
	// Compare every available committed revision and the working tree. A shallow
	// checkout still validates the baseline above; full history guards future edits.
	commits, err := exec.Command("git", "log", "--format=%H", "--", "owned-skills.txt").Output()
	if err != nil {
		return
	}
	for _, commit := range strings.Fields(string(commits)) {
		data, err := exec.Command("git", "show", commit+":"+ownershipPath).Output()
		if err != nil {
			continue
		}
		old := ownershipLines(data)
		if len(names) < len(old) || !reflect.DeepEqual(names[:len(old)], old) {
			t.Fatal("ownership registry is not append-only relative to " + commit)
		}
	}
}
func ownershipLines(data []byte) []string {
	var names []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, line)
		}
	}
	return names
}

type mapSource map[string][]byte

func (source mapSource) Read(ctx context.Context, file string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := source[file]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}
func testSource(t *testing.T) mapSource {
	t.Helper()
	bundle := repositoryBundle(t)
	source := mapSource{catalogPath: catalogData, ownershipPath: ownershipData}
	for name, files := range bundle.files {
		for file, data := range files {
			source[".agents/skills/"+name+"/"+file] = data
		}
	}
	return source
}
func TestLoadRejectsTamperingAndIncompleteBundle(t *testing.T) {
	for _, file := range []string{catalogPath, ownershipPath, ".agents/skills/grepple-write/SKILL.md"} {
		t.Run(file, func(t *testing.T) {
			source := testSource(t)
			source[file] = []byte("modified")
			if _, err := Load(context.Background(), source); err == nil {
				t.Fatal("tampered source accepted")
			}
		})
	}
	source := testSource(t)
	delete(source, ".agents/skills/grepple-write/references/write.md")
	if _, err := Load(context.Background(), source); err == nil {
		t.Fatal("missing resource accepted")
	}
}
func TestCatalogValidationRejectsTraversalAndUnownedNames(t *testing.T) {
	for _, file := range []string{"../escape", "/absolute", "a\\b", "C:escape", markerName} {
		if err := validateSkill(skill{Name: "grepple-test", Files: []skillFile{{Path: file, SHA256: strings.Repeat("a", 64)}}}); err == nil {
			t.Errorf("accepted %q", file)
		}
	}
	var catalog catalog
	if err := json.Unmarshal(catalogData, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, s := range catalog.Skills {
		if !strings.HasPrefix(s.Name, "grepple-") {
			t.Errorf("unprefixed skill: %s", s.Name)
		}
	}
}
