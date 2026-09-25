package navigation

import (
	"path/filepath"
	"strings"

	"github.com/greppleai/grepple/dependency"
)

type phpNavigationIndex struct {
	baseLanguageNavigationIndex
	autoload map[string]phpAutoloadConfig
}

type phpAutoloadConfig struct {
	rules map[string][]string
	err   error
}

func (*phpNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if call.importPath != "" && len(call.importTargetFiles) == 0 {
		return nil
	}
	return candidates
}

func (index *phpNavigationIndex) importTargets(sourceFile, _, importPath, imported, _ string) navigationImportTargets {
	importPath = strings.TrimPrefix(strings.TrimSpace(importPath), "\\")
	if importPath == "" {
		return navigationImportTargets{}
	}
	qualified := importPath
	if offset := strings.LastIndex(qualified, "\\"); offset >= 0 {
		qualified = qualified[:offset] + "." + qualified[offset+1:]
	}
	files := index.corpus.exportIndex().targetFiles(index.family, qualified, imported == "*", imported != "*")
	if sourceFile == "" || imported == "*" {
		return navigationImportTargets{files: files}
	}
	projects := dependency.DefaultRegistry().Projects("php", filepath.Dir(sourceFile))
	if len(projects) == 0 {
		return navigationImportTargets{files: files}
	}
	manifest := projects[0].ManifestPath
	if index.autoload == nil {
		index.autoload = make(map[string]phpAutoloadConfig)
	}
	config, found := index.autoload[manifest]
	if !found {
		config.rules, config.err = dependency.ComposerAutoloadRules(manifest)
		index.autoload[manifest] = config
	}
	if config.err != nil {
		return navigationImportTargets{}
	}
	longest := -1
	var expected []string
	for prefix, dirs := range config.rules {
		prefix = strings.TrimPrefix(prefix, "\\")
		if !strings.HasPrefix(importPath, prefix) || (prefix != "" && !strings.HasSuffix(prefix, "\\") && len(importPath) > len(prefix) && importPath[len(prefix)] != '\\') {
			continue
		}
		if len(prefix) < longest {
			continue
		}
		if len(prefix) > longest {
			longest, expected = len(prefix), nil
		}
		remainder := strings.TrimPrefix(importPath, prefix)
		for _, dir := range dirs {
			target, err := filepath.Abs(filepath.Join(filepath.Dir(manifest), dir, strings.ReplaceAll(remainder, "\\", string(filepath.Separator))+".php"))
			if err == nil {
				expected = append(expected, target)
			}
		}
	}
	if longest < 0 {
		return navigationImportTargets{files: files}
	}
	var matched []string
	for _, file := range files {
		absolute, err := filepath.Abs(file)
		if err != nil {
			continue
		}
		for _, target := range expected {
			if absolute == target {
				matched = append(matched, file)
				break
			}
		}
	}
	return navigationImportTargets{files: matched}
}
