package pihooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	fallbackGuide  = "general"
	commentGroup   = "comments"
	defaultMessage = "revive reported a violation."
)

var validGuideName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Guide is remediation documentation selected for a diagnostic group.
type Guide struct {
	Name    string
	Content string
}

// ParseReviveReport decodes revive's JSON formatter output.
func ParseReviveReport(output string) ([]Diagnostic, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var diagnostics []Diagnostic
	if err := json.Unmarshal([]byte(trimmed), &diagnostics); err != nil {
		return nil, fmt.Errorf("parse revive report: %w", err)
	}
	if diagnostics == nil {
		return nil, fmt.Errorf("revive JSON output was not an array of failures")
	}
	return diagnostics, nil
}

func diagnosticPath(diagnostic Diagnostic) string {
	if diagnostic.Position.Start.Filename == "" {
		return "unknown file"
	}
	return diagnostic.Position.Start.Filename
}

func diagnosticRule(diagnostic Diagnostic) string {
	if diagnostic.RuleName == "" {
		return "unknown"
	}
	return diagnostic.RuleName
}

func diagnosticGroupKey(diagnostic Diagnostic) string {
	switch diagnosticRule(diagnostic) {
	case "exported", "package-comments":
		return commentGroup
	default:
		return diagnosticRule(diagnostic)
	}
}

// SelectNextDiagnosticGroup chooses one coherent rule/file group for feedback.
func SelectNextDiagnosticGroup(diagnostics []Diagnostic) []Diagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	sorted := append([]Diagnostic(nil), diagnostics...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sorted[i], sorted[j]
		if diagnosticPath(left) != diagnosticPath(right) {
			return diagnosticPath(left) < diagnosticPath(right)
		}
		if left.Position.Start.Line != right.Position.Start.Line {
			return left.Position.Start.Line < right.Position.Start.Line
		}
		if left.Position.Start.Column != right.Position.Start.Column {
			return left.Position.Start.Column < right.Position.Start.Column
		}
		return diagnosticRule(left) < diagnosticRule(right)
	})

	byPath := make(map[string][]Diagnostic)
	for _, diagnostic := range sorted {
		path := diagnosticPath(diagnostic)
		byPath[path] = append(byPath[path], diagnostic)
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		left, right := byPath[paths[i]], byPath[paths[j]]
		if len(left) != len(right) {
			return len(left) > len(right)
		}
		return paths[i] < paths[j]
	})
	selected := byPath[paths[0]]
	group := diagnosticGroupKey(selected[0])
	result := make([]Diagnostic, 0, len(selected))
	for _, diagnostic := range selected {
		if diagnosticGroupKey(diagnostic) == group {
			result = append(result, diagnostic)
		}
	}
	return result
}

// GuideNameForDiagnostic returns the safe guide basename for a diagnostic.
func GuideNameForDiagnostic(diagnostic Diagnostic) string {
	rule := diagnosticRule(diagnostic)
	if rule == "unknown" || !validGuideName.MatchString(rule) {
		return fallbackGuide
	}
	return rule
}

// LoadGuide loads a guide from the standalone hook module, falling back to the
// general guide when a rule has no dedicated documentation.
func LoadGuide(hookRoot string, diagnostic Diagnostic) (Guide, error) {
	name := GuideNameForDiagnostic(diagnostic)
	content, err := os.ReadFile(filepath.Join(hookRoot, "guides", name+".md"))
	if err == nil {
		return Guide{Name: name, Content: string(content)}, nil
	}
	content, fallbackErr := os.ReadFile(filepath.Join(hookRoot, "guides", fallbackGuide+".md"))
	if fallbackErr != nil {
		return Guide{}, fallbackErr
	}
	return Guide{Name: fallbackGuide, Content: string(content)}, nil
}

// LoadGuideForGroup selects the comments guide for merged documentation rules.
func LoadGuideForGroup(hookRoot string, group []Diagnostic) (Guide, error) {
	if len(group) > 0 && diagnosticGroupKey(group[0]) == commentGroup {
		return LoadGuide(hookRoot, Diagnostic{RuleName: commentGroup})
	}
	if len(group) == 0 {
		return LoadGuide(hookRoot, Diagnostic{})
	}
	return LoadGuide(hookRoot, group[0])
}

func diagnosticLocation(diagnostic Diagnostic) string {
	position := diagnostic.Position.Start
	path := diagnosticPath(diagnostic)
	if position.Line == 0 {
		return path
	}
	return fmt.Sprintf("%s:%d:%d", path, position.Line, position.Column)
}

// FormatDiagnosticFeedback renders one selected group and its remediation guide.
func FormatDiagnosticFeedback(selected []Diagnostic, total int, guide Guide, autoFixedFiles []string) string {
	rules := uniqueDiagnosticRules(selected)
	repeated := len(selected) > 1
	severity := selected[0].Severity
	if severity == "" {
		severity = "warning"
	}
	parts := []string{
		feedbackPreamble(repeated, len(rules)),
		"Do not batch diagnostics from other files or rules. Do not suppress or weaken the rule unless the guide explicitly permits it.",
		fmt.Sprintf("Remaining diagnostics in this run: %d", total),
	}
	if len(autoFixedFiles) > 0 {
		parts = append(parts, "Auto-fixed by gofmt in this pass:\n- "+strings.Join(autoFixedFiles, "\n- "))
	}
	issueLabel := "Issue"
	if repeated {
		issueLabel = "Issues"
	}
	parts = append(parts,
		"",
		fmt.Sprintf("%s: %s (%s)", issueLabel, strings.Join(rules, ", "), severity),
		diagnosticDetails(selected),
		fmt.Sprintf("Guide: hooks/guides/%s.md", guide.Name),
		"",
		strings.TrimSpace(guide.Content),
	)
	return strings.Join(parts, "\n")
}

func uniqueDiagnosticRules(diagnostics []Diagnostic) []string {
	rules := make([]string, 0, len(diagnostics))
	seen := make(map[string]struct{})
	for _, diagnostic := range diagnostics {
		rule := diagnosticRule(diagnostic)
		if _, exists := seen[rule]; exists {
			continue
		}
		seen[rule] = struct{}{}
		rules = append(rules, rule)
	}
	return rules
}

func feedbackPreamble(repeated bool, ruleCount int) string {
	if !repeated {
		return "revive reported a violation. Resolve only this issue, then stop so the hook can rerun and provide the next issue."
	}
	if ruleCount > 1 {
		return "revive reported multiple comment-related violations in one file. Resolve all of them together in one pass, then stop so the hook can rerun and provide the next issue."
	}
	return "revive reported multiple violations of the same rule in one file. Resolve only these issues, then stop so the hook can rerun and provide the next issue."
}

func diagnosticDetails(diagnostics []Diagnostic) string {
	locations := make([]string, len(diagnostics))
	for index, diagnostic := range diagnostics {
		locations[index] = diagnosticLocation(diagnostic)
	}
	message := diagnostics[0].Failure
	if message == "" {
		message = defaultMessage
	}
	if len(diagnostics) == 1 {
		return "Location: " + locations[0] + "\nMessage: " + message
	}
	if diagnosticsShareMessage(diagnostics) {
		return "Locations:\n- " + strings.Join(locations, "\n- ") + "\nMessage: " + message
	}
	lines := make([]string, len(diagnostics))
	for index, diagnostic := range diagnostics {
		itemMessage := diagnostic.Failure
		if itemMessage == "" {
			itemMessage = defaultMessage
		}
		lines[index] = fmt.Sprintf("- %s — %s", locations[index], itemMessage)
	}
	return "Diagnostics:\n" + strings.Join(lines, "\n")
}

func diagnosticsShareMessage(diagnostics []Diagnostic) bool {
	message := diagnostics[0].Failure
	for _, diagnostic := range diagnostics[1:] {
		if diagnostic.Failure != message {
			return false
		}
	}
	return true
}
