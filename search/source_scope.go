package search

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/internal/sourcekind"
	"github.com/greppleai/grepple/parser"
)

// SourceScopeOptions controls a repository source-universe inspection.
type SourceScopeOptions struct {
	Root           string
	IgnoreRoot     string
	IgnorePaths    []string
	ProductionOnly bool
}

// SourcePathDecision explains whether and why one discovered path is selected.
type SourcePathDecision struct {
	Path           string `json:"path"`
	Language       string `json:"language,omitempty"`
	Classification string `json:"classification,omitempty"`
	Selected       bool   `json:"selected"`
	Reason         string `json:"reason"`
	Explicit       bool   `json:"explicit,omitempty"`
	Subtree        bool   `json:"subtree,omitempty"`
}

// InspectSourceScope walks the supplied paths and returns deterministic source
// selection decisions. Explicit files bypass ignore and production filters.
func InspectSourceScope(inputs []string, options SourceScopeOptions) ([]SourcePathDecision, error) {
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
	inspector := sourceScopeInspector{
		root:      root,
		ignore:    sourceIgnoreConfig{root: ignoreRoot, patterns: options.IgnorePaths, productionOnly: options.ProductionOnly},
		gitignore: gitignoreCache{},
		seen:      map[string]bool{},
	}
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

type sourceScopeInspector struct {
	root      string
	ignore    sourceIgnoreConfig
	gitignore gitignoreCache
	seen      map[string]bool
	decisions []SourcePathDecision
}

func (inspector *sourceScopeInspector) walk(path string, entry os.DirEntry, walkErr error) error {
	if walkErr != nil {
		inspector.addDecision(path, SourcePathDecision{Selected: false, Reason: "unreadable"})
		return nil
	}
	if entry.Type()&os.ModeSymlink != 0 {
		inspector.addDecision(path, SourcePathDecision{Selected: false, Reason: "symlink", Subtree: entry.IsDir()})
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if entry.IsDir() {
		if isUnconditionalSourceSubtree(entry.Name()) {
			return filepath.SkipDir
		}
		return nil
	}
	inspector.addFile(path, false)
	return nil
}

func (inspector *sourceScopeInspector) addFile(path string, explicit bool) {
	if inspector.seen[path] {
		return
	}
	inspector.seen[path] = true
	classification := sourcekind.Classify(path, inspector.ignore.root)
	decision := SourcePathDecision{
		Language: parser.LanguageFor(path), Classification: string(classification),
		Selected: true, Reason: "selected", Explicit: explicit,
	}
	if pathContainsGitDirectory(path) || inspector.ignore.builtIn(path) {
		decision.Selected, decision.Reason = false, "built-in-subtree"
	} else if !explicit {
		switch {
		case pathIgnoredFromRootCached(path, inspector.root, inspector.gitignore):
			decision.Selected, decision.Reason = false, "gitignore"
		case inspector.ignore.filter().Ignored(path):
			decision.Selected, decision.Reason = false, "config-ignore"
		case inspector.ignore.productionOnly && classification != sourcekind.Production:
			decision.Selected, decision.Reason = false, "non-production"
		}
	} else if inspector.ignore.filter().Ignored(path) {
		decision.Reason = "selected-explicit-config-bypass"
	} else if inspector.ignore.productionOnly && classification != sourcekind.Production {
		decision.Reason = "selected-explicit-production-bypass"
	}
	inspector.addDecision(path, decision)
}

func (inspector *sourceScopeInspector) addDecision(path string, decision SourcePathDecision) {
	display, err := filepath.Rel(inspector.root, path)
	if err != nil || display == ".." || strings.HasPrefix(display, ".."+string(filepath.Separator)) {
		display = path
	}
	decision.Path = filepath.ToSlash(display)
	inspector.decisions = append(inspector.decisions, decision)
}

func isUnconditionalSourceSubtree(name string) bool {
	switch name {
	case ".git", ".grepple", ".worktrees":
		return true
	default:
		return false
	}
}
