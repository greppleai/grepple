// Package pihooks implements hooks used by pi and compatible agent clients.
package pihooks

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	codeparser "github.com/greppleai/grepple/internal/parser"
)

// AllowMarker is the explicit escape hatch for commands which genuinely need a
// legacy search program (usually to filter another command's output).
const AllowMarker = "grep-guard:allow"

var (
	blockedCommands = stringSet("find", "grep", "egrep", "fgrep", "rg", "ag", "ack", "fd", "fdfind", "locate")
	wrapperCommands = stringSet("sudo", "command", "builtin", "exec", "xargs", "time", "nice", "env", "noglob")
	assignment      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

func stringSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// FindBlockedInvocation returns the basename of the first blocked command
// actually invoked by command. Shell syntax is parsed rather than split as
// text, so command names merely appearing in arguments, comments, or quoted
// strings do not cause a match.
func FindBlockedInvocation(command string) string {
	return findBlockedInvocation(command, 0)
}

func findBlockedInvocation(command string, depth int) string {
	if strings.Contains(command, AllowMarker) || depth > 8 {
		return ""
	}
	document, err := codeparser.ParseDocument("shell", command)
	if err != nil {
		return "" // The hook must fail open if its parser cannot be initialized.
	}
	defer document.Close()
	var hit string
	if err := document.Read(func(view codeparser.DocumentView) error {
		hit = findInTree(view.Root(), []byte(command), depth)
		return nil
	}); err != nil {
		return ""
	}
	return hit
}

func findInTree(node codeparser.ViewNode, source []byte, depth int) string {
	if node.Kind() == "command" {
		if hit := blockedInCommand(node, source, depth); hit != "" {
			return hit
		}
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		if hit := findInTree(node.NamedChild(i), source, depth); hit != "" {
			return hit
		}
	}
	return ""
}

func blockedInCommand(command codeparser.ViewNode, source []byte, depth int) string {
	nameNode := command.ChildByFieldName("name")
	if !nameNode.Valid() {
		return ""
	}
	name, ok := literalWord(nodeSource(nameNode, source))
	if !ok {
		return ""
	}

	words := []string{name}
	// Only direct children after the command name are argv words. Descendant
	// commands (such as $(grep ...)) are visited separately by findInTree.
	for i := 0; i < command.NamedChildCount(); i++ {
		child := command.NamedChild(i)
		if child.Range().StartByte < nameNode.Range().EndByte || !isArgumentNode(child.Kind()) {
			continue
		}
		word, literal := literalWord(nodeSource(child, source))
		if !literal {
			words = append(words, "\x00")
			continue
		}
		words = append(words, word)
	}
	if hit := blockedInWords(words); hit != "" {
		return hit
	}
	return blockedInInterpretedScript(words, depth)
}

func isArgumentNode(kind string) bool {
	switch kind {
	case "word", "number", "string", "raw_string", "concatenation", "simple_expansion", "command_substitution", "process_substitution", "arithmetic_expansion", "translated_string":
		return true
	default:
		return false
	}
}

func blockedInWords(words []string) string {
	index := effectiveCommandIndex(words)
	if index < 0 {
		return ""
	}
	base := path.Base(words[index])
	if _, blocked := blockedCommands[base]; blocked {
		return base
	}
	return ""
}

func effectiveCommandIndex(words []string) int {
	for index := 0; index < len(words); {
		name := words[index]
		if _, wrapper := wrapperCommands[name]; !wrapper {
			return index
		}
		index = wrapperCommandIndex(name, words, index+1)
		for index < len(words) && assignment.MatchString(words[index]) {
			index++
		}
	}
	return -1
}

func blockedInInterpretedScript(words []string, depth int) string {
	if script := envSplitScript(words); script != "" {
		return findBlockedInvocation(script, depth+1)
	}
	index := effectiveCommandIndex(words)
	if index < 0 {
		return ""
	}
	base := path.Base(words[index])
	arguments := words[index+1:]
	if isShellInterpreter(base) {
		if script := shellCommandString(arguments); script != "" {
			return findBlockedInvocation(script, depth+1)
		}
	}
	if base == "eval" && len(arguments) > 0 {
		return findBlockedInvocation(strings.Join(arguments, " "), depth+1)
	}
	return ""
}

func envSplitScript(words []string) string {
	for index := 0; index < len(words); {
		wrapper := words[index]
		if _, ok := wrapperCommands[wrapper]; !ok {
			return ""
		}
		if wrapper == "env" {
			for argument := index + 1; argument+1 < len(words); argument++ {
				if words[argument] == "-S" || words[argument] == "--split-string" {
					return words[argument+1]
				}
			}
		}
		index = wrapperCommandIndex(wrapper, words, index+1)
	}
	return ""
}

func isShellInterpreter(command string) bool {
	switch command {
	case "sh", "bash", "dash", "zsh", "ksh":
		return true
	default:
		return false
	}
}

func shellCommandString(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--" {
			return ""
		}
		if strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") && strings.Contains(argument[1:], "c") && index+1 < len(arguments) {
			return arguments[index+1]
		}
	}
	return ""
}

// wrapperCommandIndex returns the argv index at which a wrapper's command
// begins. Options that consume a following value need special treatment; a
// generic "skip dash words" loop mistakes that value for the command.
func wrapperCommandIndex(wrapper string, words []string, index int) int {
	for index < len(words) {
		word := words[index]
		if word == "--" {
			return index + 1
		}
		if word == "-" && wrapper == "env" {
			index++
			continue
		}
		if !strings.HasPrefix(word, "-") || word == "-" {
			return index
		}

		consumeNext := optionConsumesNext(wrapper, word)
		index++
		if consumeNext && index < len(words) {
			index++
		}
	}
	return index
}

func optionConsumesNext(wrapper, option string) bool {
	if strings.Contains(option, "=") {
		return false
	}
	options := map[string]map[string]struct{}{
		"sudo":  stringSet("-u", "--user", "-g", "--group", "-h", "--host", "-p", "--prompt", "-C", "--close-from", "-T", "--command-timeout", "-R", "--chroot", "-D", "--chdir", "-r", "--role", "-t", "--type"),
		"xargs": stringSet("-a", "--arg-file", "-E", "--eof", "-I", "--replace", "-L", "--max-lines", "-n", "--max-args", "-P", "--max-procs", "-s", "--max-chars", "--process-slot-var", "--delimiter"),
		"time":  stringSet("-f", "--format", "-o", "--output"),
		"nice":  stringSet("-n", "--adjustment"),
		"env":   stringSet("-u", "--unset", "-C", "--chdir", "-S", "--split-string"),
		"exec":  stringSet("-a"),
	}
	_, consumes := options[wrapper][option]
	return consumes
}

// literalWord performs shell quote removal for a statically-known argv word.
// Expansions are intentionally rejected: their runtime value is unknowable.
func literalWord(word string) (string, bool) {
	var result strings.Builder
	for index := 0; index < len(word); {
		var ok bool
		switch word[index] {
		case '\'', '"':
			index, ok = appendQuotedWord(&result, word, index)
		case '\\':
			index, ok = appendEscapedByte(&result, word, index)
		case '$', '`':
			return "", false
		default:
			result.WriteByte(word[index])
			index++
			ok = true
		}
		if !ok {
			return "", false
		}
	}
	return result.String(), true
}

func appendQuotedWord(result *strings.Builder, word string, index int) (int, bool) {
	quote := word[index]
	for index++; index < len(word) && word[index] != quote; index++ {
		if quote == '"' && (word[index] == '$' || word[index] == '`') {
			return index, false
		}
		if quote == '"' && word[index] == '\\' && index+1 < len(word) && strings.ContainsRune("$`\"\\\n", rune(word[index+1])) {
			index++
		}
		result.WriteByte(word[index])
	}
	if index == len(word) {
		return index, false
	}
	return index + 1, true
}

func appendEscapedByte(result *strings.Builder, word string, index int) (int, bool) {
	if index+1 >= len(word) {
		return index, false
	}
	index++
	if word[index] != '\n' {
		result.WriteByte(word[index])
	}
	return index + 1, true
}

func nodeSource(node codeparser.ViewNode, _ []byte) string {
	return node.Text()
}

// DenialReason builds the tool-result feedback for a blocked command.
func DenialReason(hit string) string {
	return strings.Join([]string{
		fmt.Sprintf("Blocked: '%s' is disabled for local search. Use the grepple CLI instead (see the local-grep-and-search skill):", hit),
		`  content search:    grepple "pattern" ["**/*.go"] [--line-only | --count | --files-with-matches | --limit N]`,
		`  list filenames:    grepple -l "**/*.go" [dir]`,
		`  file outline:      grepple --outline <file-or-glob>`,
		`Grepple uses JavaScript regex by default; -E is accepted for that mode, -F selects literals, and -r is unnecessary but accepted.`,
		`For edits: Grepple locates PATH:LINE, then Read supplies hash-anchored context for Edit.`,
		`Never use find/grep/rg/ag/ack/fd for content search, file listing by name, or match counting in the working directory.`,
		`Filtering COMMAND OUTPUT also works with grepple (pipe support): 'go test ./... | grepple -F FAIL'. If you truly need the legacy tool, re-run with '# ` + AllowMarker + `' appended.`,
	}, "\n")
}

// HookOutput is the Claude-compatible PreToolUse response emitted on denial.
type HookOutput struct {
	HookSpecificOutput HookSpecificOutput `json:"hookSpecificOutput"`
}

// HookSpecificOutput contains the protocol's permission decision.
type HookSpecificOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

// HandleHook parses a PreToolUse payload and returns its structured denial.
// An empty result means allow. Malformed or irrelevant input fails open, as
// required by the hooks contract.
func HandleHook(input []byte) []byte {
	var payload struct {
		ToolInput struct {
			Command any `json:"command"`
		} `json:"tool_input"`
	}
	if json.Unmarshal(input, &payload) != nil {
		return nil
	}
	command, ok := payload.ToolInput.Command.(string)
	if !ok {
		return nil
	}
	hit := FindBlockedInvocation(command)
	if hit == "" {
		return nil
	}
	output, err := json.Marshal(HookOutput{HookSpecificOutput: HookSpecificOutput{
		HookEventName:            "PreToolUse",
		PermissionDecision:       "deny",
		PermissionDecisionReason: DenialReason(hit),
	}})
	if err != nil {
		return nil
	}
	return output
}
