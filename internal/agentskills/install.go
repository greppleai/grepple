package agentskills

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const markerName = ".grepple-owned.json"
const owner = "https://github.com/greppleai/grepple"

type marker struct {
	Owner string            `json:"owner"`
	Name  string            `json:"name"`
	Ref   string            `json:"ref"`
	Files map[string]string `json:"files"`
}

// Report records only owned changes, plus unmarked historical names left untouched.
type Report struct{ Installed, Removed, Preserved []string }
type change struct {
	name   string
	before string
	remove bool
}

// Synchronize validates every owned destination before replacing or retiring skills.
// Installation failures roll back directory renames; this is not a crash journal.
func Synchronize(ctx context.Context, root, ref string, bundle Bundle, dryRun bool) (Report, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	if err = checkDirectoryPath(absolute); err != nil {
		return Report{}, err
	}
	if dryRun {
		report, _, err := planInstall(absolute, bundle)
		return report, err
	}
	if err = ctx.Err(); err != nil {
		return Report{}, err
	}
	if err = os.MkdirAll(absolute, 0755); err != nil {
		return Report{}, err
	}
	lock := filepath.Join(absolute, ".grepple-setup.lock")
	if err = os.Mkdir(lock, 0700); err != nil {
		return Report{}, fmt.Errorf("setup directory is locked or inaccessible; check for another setup before removing %s: %w", lock, err)
	}
	defer os.Remove(lock)
	report, changes, err := planInstall(absolute, bundle)
	if err != nil {
		return report, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(absolute), ".grepple-setup-")
	if err != nil {
		return report, err
	}
	retainStage := false
	defer func() {
		if !retainStage {
			os.RemoveAll(stage)
		}
	}()
	if err = stageSkills(ctx, stage, ref, bundle); err != nil {
		return report, err
	}
	if err = applyChanges(ctx, absolute, stage, changes); err != nil {
		retainStage = true
		return report, fmt.Errorf("%w; recovery staging retained at %s", err, stage)
	}
	return report, nil
}
func checkDirectoryPath(root string) error {
	for current := root; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !info.IsDir() {
			return fmt.Errorf("setup directory contains a symlink or non-directory: %s", current)
		}
		if current == filepath.Dir(current) {
			return nil
		}
	}
}
func planInstall(root string, bundle Bundle) (Report, []change, error) {
	report := Report{}
	var changes []change
	active := map[string]bool{}
	for _, s := range bundle.catalog.Skills {
		active[s.Name] = true
	}
	for _, name := range OwnedNames() {
		state, err := inspectDestination(root, name, bundle)
		if err != nil {
			return report, nil, err
		}
		if state.exists && !state.managed {
			if active[name] {
				return report, nil, fmt.Errorf("refusing to replace unmanaged skill %s; move it aside first", name)
			}
			report.Preserved = append(report.Preserved, name)
			continue
		}
		if !state.exists && !active[name] {
			continue
		}
		changes = append(changes, change{name: name, before: state.hash, remove: !active[name]})
		if active[name] {
			report.Installed = append(report.Installed, name)
		} else {
			report.Removed = append(report.Removed, name)
		}
	}
	return report, changes, nil
}

type destinationState struct {
	exists, managed bool
	hash            string
}

func inspectDestination(root, name string, bundle Bundle) (destinationState, error) {
	current := filepath.Join(root, name)
	info, err := os.Lstat(current)
	if os.IsNotExist(err) {
		return destinationState{}, nil
	}
	if err != nil {
		return destinationState{}, err
	}
	if !info.IsDir() {
		return destinationState{}, fmt.Errorf("owned destination is not a real directory: %s", name)
	}
	snapshot, err := treeFiles(current)
	if err != nil {
		return destinationState{}, err
	}
	managed, err := managedTree(name, snapshot)
	if err != nil {
		return destinationState{}, err
	}
	if !managed {
		managed = matchesLegacy(name, snapshot, bundle.catalog.Legacy) || matchesBundle(name, snapshot, bundle)
	}
	return destinationState{exists: true, managed: managed, hash: treeHash(snapshot)}, nil
}
func treeFiles(root string) (map[string][]byte, error) {
	result := map[string][]byte{}
	total := 0
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, data, err := readTreeFile(root, current, entry)
		if err != nil {
			return err
		}
		total += len(data)
		if total > maxBundleBytes || len(result) >= 65 {
			return fmt.Errorf("skill directory exceeds safety limits")
		}
		result[relative] = data
		return nil
	})
	return result, err
}
func readTreeFile(root, current string, entry fs.DirEntry) (string, []byte, error) {
	if entry.Type()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("refusing symlink in skill directory")
	}
	info, err := entry.Info()
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", nil, fmt.Errorf("skill files must be regular")
	}
	if info.Size() > maxFileBytes {
		return "", nil, fmt.Errorf("skill file exceeds safety limit")
	}
	relative, err := filepath.Rel(root, current)
	if err != nil {
		return "", nil, err
	}
	data, err := readLocalFile(current)
	return filepath.ToSlash(relative), data, err
}
func treeHash(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var data strings.Builder
	for _, name := range names {
		data.WriteString(name)
		data.WriteByte(0)
		data.WriteString(digest(files[name]))
		data.WriteByte(0)
	}
	return digest([]byte(data.String()))
}
func managedTree(name string, files map[string][]byte) (bool, error) {
	data, exists := files[markerName]
	if !exists {
		return false, nil
	}
	var owned marker
	if err := json.Unmarshal(data, &owned); err != nil {
		return false, fmt.Errorf("invalid ownership marker for %s", name)
	}
	if owned.Owner != owner || owned.Name != name || len(owned.Files) == 0 || len(files) != len(owned.Files)+1 {
		return false, fmt.Errorf("modified or invalid managed skill %s; move it aside first", name)
	}
	for path, hash := range owned.Files {
		data, exists := files[path]
		if !exists || !safeRelative(path) || path == markerName || digest(data) != hash {
			return false, fmt.Errorf("managed skill %s was modified; move it aside first", name)
		}
	}
	return true, nil
}
func matchesLegacy(name string, files map[string][]byte, legacy []legacySkill) bool {
	if len(files) != 1 {
		return false
	}
	data, exists := files["SKILL.md"]
	if !exists {
		return false
	}
	for _, old := range legacy {
		if old.Name == name {
			for _, hash := range old.SHA256s {
				if digest(data) == hash {
					return true
				}
			}
		}
	}
	return false
}
func matchesBundle(name string, files map[string][]byte, bundle Bundle) bool {
	expected, exists := bundle.files[name]
	if !exists || len(files) != len(expected) {
		return false
	}
	return treeHash(files) == treeHash(expected)
}
func stageSkills(ctx context.Context, stage, ref string, bundle Bundle) error {
	for name, files := range bundle.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		directory := filepath.Join(stage, "new", name)
		owned := marker{Owner: owner, Name: name, Ref: ref, Files: map[string]string{}}
		for path, data := range files {
			target := filepath.Join(directory, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(target, data, 0644); err != nil {
				return err
			}
			owned.Files[path] = digest(data)
		}
		data, err := json.MarshalIndent(owned, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(directory, markerName), append(data, '\n'), 0644); err != nil {
			return err
		}
	}
	return nil
}
func applyChanges(ctx context.Context, root, stage string, changes []change) (result error) {
	if err := os.MkdirAll(filepath.Join(stage, "old"), 0700); err != nil {
		return err
	}
	var applied []appliedChange
	defer func() {
		if result != nil {
			result = rollbackChanges(root, stage, applied, result)
		}
	}()
	for _, item := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, touched, err := applyOne(root, stage, item)
		if touched {
			applied = append(applied, state)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func applyOne(root, stage string, item change) (appliedChange, bool, error) {
	state := appliedChange{change: item}
	target := filepath.Join(root, item.name)
	if err := checkSnapshot(target, item.before); err != nil {
		return state, false, err
	}
	touched := false
	if item.before != "" {
		if err := os.Rename(target, filepath.Join(stage, "old", item.name)); err != nil {
			return state, false, err
		}
		touched = true
	}
	if item.remove {
		return state, touched, nil
	}
	source := filepath.Join(stage, "new", item.name)
	files, err := treeFiles(source)
	if err != nil {
		return state, touched, err
	}
	if err = checkSnapshot(target, ""); err != nil {
		return state, touched, err
	}
	if err = os.Rename(source, target); err != nil {
		return state, touched, err
	}
	state.after = treeHash(files)
	return state, true, nil
}

type appliedChange struct {
	change
	after string
}

func checkSnapshot(target, before string) error {
	if before == "" {
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("skill destination appeared during setup")
	}
	files, err := treeFiles(target)
	if err != nil {
		return err
	}
	if treeHash(files) != before {
		return fmt.Errorf("skill changed during setup")
	}
	return nil
}
func rollbackChanges(root, stage string, changes []appliedChange, cause error) error {
	for i := len(changes) - 1; i >= 0; i-- {
		if err := rollbackOne(root, stage, changes[i]); err != nil {
			return fmt.Errorf("%w; rollback failed: %v", cause, err)
		}
	}
	return cause
}
func rollbackOne(root, stage string, item appliedChange) error {
	target := filepath.Join(root, item.name)
	if item.after != "" {
		if err := checkSnapshot(target, item.after); err != nil {
			return fmt.Errorf("rollback refused a concurrent edit: %w", err)
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}
	if item.before != "" {
		if err := checkSnapshot(target, ""); err != nil {
			return err
		}
		return os.Rename(filepath.Join(stage, "old", item.name), target)
	}
	return nil
}
