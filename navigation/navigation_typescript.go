package navigation

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxTypeScriptConfigBytes = 1 << 20

type typeScriptConfig struct {
	CompilerOptions struct {
		BaseURL string              `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

func typeScriptAliasImportTargets(files []string, sourceFile, importPath string) []string {
	configPath := nearestTypeScriptConfig(sourceFile)
	if configPath == "" {
		return nil
	}
	config, ok := readTypeScriptConfig(configPath)
	if !ok {
		return nil
	}
	base := filepath.Dir(configPath)
	if config.CompilerOptions.BaseURL != "" {
		base = filepath.Join(base, filepath.FromSlash(config.CompilerOptions.BaseURL))
	}
	patterns := make([]string, 0, len(config.CompilerOptions.Paths))
	for pattern := range config.CompilerOptions.Paths {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	targets := []string{}
	for _, pattern := range patterns {
		wildcard, matches := typeScriptPathPatternMatch(pattern, importPath)
		if !matches {
			continue
		}
		for _, replacement := range config.CompilerOptions.Paths[pattern] {
			target := strings.Replace(filepath.FromSlash(replacement), "*", filepath.FromSlash(wildcard), 1)
			targets = append(targets, matchingTypeScriptModuleFiles(files, filepath.Join(base, target))...)
		}
	}
	if len(targets) == 0 && config.CompilerOptions.BaseURL != "" {
		targets = matchingTypeScriptModuleFiles(files, filepath.Join(base, filepath.FromSlash(importPath)))
	}
	sort.Strings(targets)
	return compactSortedStrings(targets)
}

func nearestTypeScriptConfig(sourceFile string) string {
	directory := filepath.Dir(sourceFile)
	for {
		candidate := filepath.Join(directory, "tsconfig.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}

func readTypeScriptConfig(path string) (typeScriptConfig, bool) {
	file, err := os.Open(path)
	if err != nil {
		return typeScriptConfig{}, false
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxTypeScriptConfigBytes+1))
	if err != nil || len(content) > maxTypeScriptConfigBytes {
		return typeScriptConfig{}, false
	}
	var config typeScriptConfig
	if json.Unmarshal(stripJSONTrailingCommas(stripJSONComments(content)), &config) != nil {
		return typeScriptConfig{}, false
	}
	return config, true
}

func stripJSONComments(content []byte) []byte {
	stripper := jsonCommentStripper{content: append([]byte(nil), content...)}
	stripper.run()
	return stripper.content
}

type jsonCommentStripper struct {
	content  []byte
	index    int
	inString bool
	escaped  bool
}

func (stripper *jsonCommentStripper) run() {
	for stripper.index < len(stripper.content) {
		if stripper.inString {
			stripper.consumeStringByte()
		} else if stripper.content[stripper.index] == '"' {
			stripper.inString = true
		} else {
			stripper.consumeComment()
		}
		stripper.index++
	}
}

func (stripper *jsonCommentStripper) consumeStringByte() {
	if stripper.escaped {
		stripper.escaped = false
		return
	}
	if stripper.content[stripper.index] == '\\' {
		stripper.escaped = true
	} else if stripper.content[stripper.index] == '"' {
		stripper.inString = false
	}
}

func (stripper *jsonCommentStripper) consumeComment() {
	if stripper.index+1 >= len(stripper.content) || stripper.content[stripper.index] != '/' {
		return
	}
	switch stripper.content[stripper.index+1] {
	case '/':
		stripper.consumeLineComment()
	case '*':
		stripper.consumeBlockComment()
	}
}

func (stripper *jsonCommentStripper) consumeLineComment() {
	for stripper.index < len(stripper.content) && stripper.content[stripper.index] != '\n' {
		stripper.content[stripper.index] = ' '
		stripper.index++
	}
	stripper.index--
}

func (stripper *jsonCommentStripper) consumeBlockComment() {
	stripper.content[stripper.index], stripper.content[stripper.index+1] = ' ', ' '
	stripper.index += 2
	for stripper.index < len(stripper.content)-1 && !(stripper.content[stripper.index] == '*' && stripper.content[stripper.index+1] == '/') {
		if stripper.content[stripper.index] != '\n' && stripper.content[stripper.index] != '\r' {
			stripper.content[stripper.index] = ' '
		}
		stripper.index++
	}
	if stripper.index < len(stripper.content)-1 {
		stripper.content[stripper.index], stripper.content[stripper.index+1] = ' ', ' '
		stripper.index++
	}
}

func stripJSONTrailingCommas(content []byte) []byte {
	stripper := jsonTrailingCommaStripper{content: append([]byte(nil), content...)}
	stripper.run()
	return stripper.content
}

type jsonTrailingCommaStripper struct {
	content  []byte
	inString bool
	escaped  bool
}

func (stripper *jsonTrailingCommaStripper) run() {
	for index, value := range stripper.content {
		if stripper.inString {
			stripper.consumeStringByte(value)
		} else if value == '"' {
			stripper.inString = true
		} else if value == ',' && stripper.trailing(index) {
			stripper.content[index] = ' '
		}
	}
}

func (stripper *jsonTrailingCommaStripper) consumeStringByte(value byte) {
	if stripper.escaped {
		stripper.escaped = false
		return
	}
	if value == '\\' {
		stripper.escaped = true
	} else if value == '"' {
		stripper.inString = false
	}
}

func (stripper *jsonTrailingCommaStripper) trailing(index int) bool {
	next := index + 1
	for next < len(stripper.content) && strings.ContainsRune(" \t\r\n", rune(stripper.content[next])) {
		next++
	}
	return next < len(stripper.content) && (stripper.content[next] == '}' || stripper.content[next] == ']')
}

func typeScriptPathPatternMatch(pattern, value string) (string, bool) {
	index := strings.Index(pattern, "*")
	if index < 0 {
		return "", pattern == value
	}
	prefix, suffix := pattern[:index], pattern[index+1:]
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) || len(value) < len(prefix)+len(suffix) {
		return "", false
	}
	return value[len(prefix) : len(value)-len(suffix)], true
}

func matchingTypeScriptModuleFiles(files []string, target string) []string {
	target = strings.TrimSuffix(filepath.Clean(target), filepath.Ext(target))
	result := []string{}
	for _, file := range files {
		candidate := strings.TrimSuffix(filepath.Clean(file), filepath.Ext(file))
		if candidate == target || filepath.Base(candidate) == "index" && filepath.Dir(candidate) == target {
			result = append(result, file)
		}
	}
	return result
}

type ecmaNavigationIndex struct{ baseLanguageNavigationIndex }

func (*ecmaNavigationIndex) filterCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if call.importPath != "" && len(call.importTargetFiles) == 0 && !strings.HasPrefix(call.importPath, ".") {
		return nil
	}
	if call.importPath != "" || call.qualifier != "" || strings.Contains(call.display, ".") {
		return candidates
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		return candidate.file == call.file
	})
}

func (index *ecmaNavigationIndex) importTargets(sourceFile, _, importPath, _, _ string) navigationImportTargets {
	if strings.HasPrefix(importPath, ".") {
		result := []string{}
		for _, candidateFile := range index.corpus.files {
			if navigationRelativeImportMatches(sourceFile, importPath, candidateFile) {
				result = append(result, candidateFile)
			}
		}
		return navigationImportTargets{files: result}
	}
	return navigationImportTargets{files: typeScriptAliasImportTargets(index.corpus.files, sourceFile, importPath)}
}

func navigationRelativeImportMatches(sourceFile, importPath, candidateFile string) bool {
	imported := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), filepath.FromSlash(importPath)))
	candidate := strings.TrimSuffix(filepath.Clean(candidateFile), filepath.Ext(candidateFile))
	imported = strings.TrimSuffix(imported, filepath.Ext(imported))
	return candidate == imported || filepath.Base(candidate) == "index" && filepath.Dir(candidate) == imported
}
