// Package sources owns repository source policy, discovery, metadata-backed classification, and inspection.
package sources

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/parser"
)

type InspectionOptions struct {
	Root           string
	IgnoreRoot     string
	IgnorePaths    []string
	ProductionOnly bool
}

type Decision struct {
	Path           string `json:"path"`
	Language       string `json:"language,omitempty"`
	Classification string `json:"classification,omitempty"`
	Selected       bool   `json:"selected"`
	Reason         string `json:"reason"`
	Explicit       bool   `json:"explicit,omitempty"`
	Subtree        bool   `json:"subtree,omitempty"`
}

// Inspect walks inputs and returns deterministic source-selection decisions.
// Explicit files bypass configured ignores and production-only filtering.
func Inspect(inputs []string, options InspectionOptions) ([]Decision, error) {
	if len(inputs) == 0 {
		inputs = []string{"."}
	}
	root := options.Root
	if root == "" {
		root, _ = os.Getwd()
	}
	ignoreRoot := options.IgnoreRoot
	if ignoreRoot == "" {
		ignoreRoot = root
	}
	classifier := NewClassifier(root)
	inspector := scopeInspector{root: root, ignore: ignoreConfig{root: ignoreRoot, patterns: options.IgnorePaths, productionOnly: options.ProductionOnly, classifier: classifier}, gitignore: ignoreCache{}, seen: map[string]bool{}}
	for _, input := range inputs {
		absolute, err := filepath.Abs(input)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(absolute)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			inspector.addFile(absolute, true)
			continue
		}
		if err := filepath.WalkDir(absolute, inspector.walk); err != nil {
			return nil, err
		}
	}
	sort.Slice(inspector.decisions, func(i, j int) bool { return inspector.decisions[i].Path < inspector.decisions[j].Path })
	return inspector.decisions, nil
}

type scopeInspector struct {
	root      string
	ignore    ignoreConfig
	gitignore ignoreCache
	seen      map[string]bool
	decisions []Decision
}

func (i *scopeInspector) walk(path string, entry os.DirEntry, walkErr error) error {
	if walkErr != nil {
		i.addDecision(path, Decision{Reason: "unreadable"})
		return nil
	}
	if entry.Type()&os.ModeSymlink != 0 {
		i.addDecision(path, Decision{Reason: "symlink", Subtree: entry.IsDir()})
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if entry.Name() == directorymeta.FileName {
		return nil
	}
	if entry.IsDir() {
		if unconditionalSubtree(entry.Name()) {
			return filepath.SkipDir
		}
		return nil
	}
	i.addFile(path, false)
	return nil
}
func (i *scopeInspector) addFile(path string, explicit bool) {
	if i.seen[path] {
		return
	}
	i.seen[path] = true
	classification := i.ignore.kind(path)
	decision := Decision{Language: parser.LanguageFor(path), Classification: string(classification), Selected: true, Reason: "selected", Explicit: explicit}
	if containsGitDirectory(path) || i.ignore.builtIn(path) {
		decision.Selected, decision.Reason = false, "built-in-subtree"
	} else if !explicit {
		switch {
		case ignoredFromRootCached(path, i.root, i.gitignore):
			decision.Selected, decision.Reason = false, "gitignore"
		case i.ignore.filter().Ignored(path):
			decision.Selected, decision.Reason = false, "config-ignore"
		case i.ignore.productionOnly && classification != Production:
			decision.Selected, decision.Reason = false, "non-production"
		}
	} else if i.ignore.filter().Ignored(path) {
		decision.Reason = "selected-explicit-config-bypass"
	} else if i.ignore.productionOnly && classification != Production {
		decision.Reason = "selected-explicit-production-bypass"
	}
	i.addDecision(path, decision)
}
func (i *scopeInspector) addDecision(path string, decision Decision) {
	display, err := filepath.Rel(i.root, path)
	if err != nil || display == ".." || strings.HasPrefix(display, ".."+string(filepath.Separator)) {
		display = path
	}
	decision.Path = filepath.ToSlash(display)
	i.decisions = append(i.decisions, decision)
}
func unconditionalSubtree(name string) bool {
	return name == ".git" || name == ".grepple" || name == ".worktrees"
}
