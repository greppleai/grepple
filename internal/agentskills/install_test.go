package agentskills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name string, files map[string][]byte) {
	t.Helper()
	for path, data := range files {
		target := filepath.Join(root, name, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
func TestInstallRepeatRetirementAndUnrelatedPreservation(t *testing.T) {
	bundle := repositoryBundle(t)
	root := filepath.Join(t.TempDir(), "skills")
	writeSkill(t, root, "user-owned", map[string][]byte{"SKILL.md": []byte("private instructions")})
	report, err := Synchronize(context.Background(), root, "v0.1.0", bundle, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Installed) != len(bundle.files) {
		t.Fatal("incomplete install")
	}
	if _, err = Synchronize(context.Background(), root, "v0.1.0", bundle, false); err != nil {
		t.Fatal("repeat setup:", err)
	}
	name := bundle.catalog.Skills[0].Name
	bundle.catalog.Skills = bundle.catalog.Skills[1:]
	delete(bundle.files, name)
	report, err = Synchronize(context.Background(), root, "v0.2.0", bundle, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Removed) != 1 || report.Removed[0] != name {
		t.Fatal("retired name not removed", report)
	}
	if _, err = os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
		t.Fatal("retired skill remained")
	}
	data, err := os.ReadFile(filepath.Join(root, "user-owned", "SKILL.md"))
	if err != nil || string(data) != "private instructions" {
		t.Fatal("unrelated skill changed")
	}
}
func TestKnownLegacyRemovedButUnrecognizedLegacyPreserved(t *testing.T) {
	bundle := repositoryBundle(t)
	root := t.TempDir()
	name := "local-grep-and-search"
	legacy := []byte("known old skill")
	bundle.catalog.Legacy = append(bundle.catalog.Legacy, legacySkill{Name: name, SHA256s: []string{digest(legacy)}})
	writeSkill(t, root, name, map[string][]byte{"SKILL.md": legacy})
	other := "change-impact-analysis"
	writeSkill(t, root, other, map[string][]byte{"SKILL.md": []byte("user-customized historical name")})
	report, err := Synchronize(context.Background(), root, "v0.1.0", bundle, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Removed) != 1 || report.Removed[0] != name || len(report.Preserved) != 1 || report.Preserved[0] != other {
		t.Fatal(report)
	}
}
func TestModifiedAndUnmanagedActiveSkillAbortBeforeOtherChanges(t *testing.T) {
	bundle := repositoryBundle(t)
	root := t.TempDir()
	if _, err := Synchronize(context.Background(), root, "v0.1.0", bundle, false); err != nil {
		t.Fatal(err)
	}
	name := bundle.catalog.Skills[0].Name
	target := filepath.Join(root, name, "SKILL.md")
	if err := os.WriteFile(target, []byte("local edits"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Synchronize(context.Background(), root, "v0.2.0", bundle, false)
	if err == nil || !strings.Contains(err.Error(), "modified") {
		t.Fatal("modified managed skill overwritten")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "local edits" {
		t.Fatal("edits lost")
	}
	root = t.TempDir()
	writeSkill(t, root, name, map[string][]byte{"SKILL.md": []byte("unmanaged")})
	if _, err = Synchronize(context.Background(), root, "v0.2.0", bundle, false); err == nil {
		t.Fatal("unmanaged skill overwritten")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatal("partial install on conflict")
	}
}
func TestDryRunAndCancellationAreReadOnly(t *testing.T) {
	bundle := repositoryBundle(t)
	root := filepath.Join(t.TempDir(), "new", "skills")
	if _, err := Synchronize(context.Background(), root, "v0.1.0", bundle, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(root)); !os.IsNotExist(err) {
		t.Fatal("dry run created paths")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Synchronize(ctx, root, "v0.1.0", bundle, false); err == nil {
		t.Fatal("cancelled install succeeded")
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("cancelled install mutated root")
	}
}
func TestDestinationSymlinksAndLocksRefused(t *testing.T) {
	bundle := repositoryBundle(t)
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, bundle.catalog.Skills[0].Name)); err != nil {
		t.Skip(err)
	}
	if _, err := Synchronize(context.Background(), root, "v0.1.0", bundle, false); err == nil {
		t.Fatal("followed destination symlink")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("symlink destination changed")
	}
	root = t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".grepple-setup.lock"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Synchronize(context.Background(), root, "v0.1.0", bundle, false); err == nil {
		t.Fatal("concurrent install allowed")
	}
}
func TestFailedLaterInstallRollsBackEarlierChanges(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	bundle := repositoryBundle(t)
	first := bundle.catalog.Skills[0].Name
	second := bundle.catalog.Skills[1].Name
	writeSkill(t, root, first, map[string][]byte{"SKILL.md": []byte("old")})
	old, err := treeFiles(filepath.Join(root, first))
	if err != nil {
		t.Fatal(err)
	}
	if err = stageSkills(context.Background(), stage, "v0.1.0", bundle); err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(filepath.Join(stage, "new", second)); err != nil {
		t.Fatal(err)
	}
	err = applyChanges(context.Background(), root, stage, []change{{name: first, before: treeHash(old)}, {name: second}})
	if err == nil {
		t.Fatal("injected failure not detected")
	}
	data, _ := os.ReadFile(filepath.Join(root, first, "SKILL.md"))
	if string(data) != "old" {
		t.Fatal("old skill not restored")
	}
	if _, err = os.Stat(filepath.Join(root, second)); !os.IsNotExist(err) {
		t.Fatal("partial new skill left behind")
	}
}
func TestSnapshotFenceDetectsConcurrentMutation(t *testing.T) {
	root := t.TempDir()
	name := "grepple-write"
	writeSkill(t, root, name, map[string][]byte{"SKILL.md": []byte("old")})
	files, _ := treeFiles(filepath.Join(root, name))
	before := treeHash(files)
	writeSkill(t, root, name, map[string][]byte{"SKILL.md": []byte("concurrent edit")})
	if err := checkSnapshot(filepath.Join(root, name), before); err == nil {
		t.Fatal("concurrent edit not detected")
	}
}
