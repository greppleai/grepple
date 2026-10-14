package dependency

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Inherited versions are evidence only from the manifest's containing workspace,
// bounded by the repository root passed by artifact discovery. Never consult an
// unrelated checkout ancestor or infer a version from a ref, lockfile or sibling.
func cargoArtifactVersion(manifest cargoManifest, path, relativeRoot string) (string, error) {
	if version, ok := manifest.Package.Version.(string); ok {
		return version, nil
	}
	inherited, ok := manifest.Package.Version.(map[string]any)
	if !ok || len(inherited) != 1 || inherited["workspace"] != true {
		return "", nil
	}
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	root, err := cargoArtifactRoot(dir, relativeRoot)
	if err != nil {
		return "", err
	}
	if manifest.Package.Workspace != "" {
		candidate := filepath.Join(dir, manifest.Package.Workspace)
		if filepath.IsAbs(manifest.Package.Workspace) {
			candidate = manifest.Package.Workspace
		}
		return cargoWorkspaceVersion(candidate, root)
	}
	for current := dir; ; current = filepath.Dir(current) {
		version, found, err := readCargoWorkspaceVersion(current, root)
		if err != nil || found {
			return version, err
		}
		if current == root {
			return "", nil
		}
	}
}
func cargoArtifactRoot(dir, relativeRoot string) (string, error) {
	relative := filepath.Clean(relativeRoot)
	if filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid Cargo artifact root")
	}
	root := dir
	if relative != "." {
		for range strings.Split(relative, string(filepath.Separator)) {
			root = filepath.Dir(root)
		}
	}
	return root, nil
}
func cargoWorkspaceVersion(directory, root string) (string, error) {
	version, _, err := readCargoWorkspaceVersion(filepath.Clean(directory), root)
	return version, err
}
func readCargoWorkspaceVersion(directory, root string) (string, bool, error) {
	if !cargoWithinRoot(directory, root) {
		return "", false, nil
	}
	path := filepath.Join(directory, "Cargo.toml")
	resolved, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false, err
	}
	if !cargoWithinRoot(resolved, resolvedRoot) {
		return "", false, nil
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return "", false, err
	}
	var manifest cargoManifest
	if err := toml.Unmarshal(content, &manifest); err != nil {
		return "", false, err
	}
	if manifest.Workspace == nil {
		return "", false, nil
	}
	pkg, _ := manifest.Workspace["package"].(map[string]any)
	version, _ := pkg["version"].(string)
	return version, true, nil
}
func cargoWithinRoot(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && !filepath.IsAbs(relative) && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
