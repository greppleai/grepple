package search

import (
	"reflect"
	"sort"
	"strings"

	"github.com/greppleai/grepple/parser"
)

// NavigationDiffSchema identifies the semantic graph-diff wire contract.
const NavigationDiffSchema = "grepple-navigation-diff-v5"

// NavigationDeclarationDelta pairs a declaration before and after a semantic change.
type NavigationDeclarationDelta struct {
	Before *parser.NavigationDeclaration `json:"before,omitempty"`
	After  *parser.NavigationDeclaration `json:"after,omitempty"`
}

// NavigationCallDelta pairs a call before and after a semantic change.
type NavigationCallDelta struct {
	Before *parser.NavigationCall `json:"before,omitempty"`
	After  *parser.NavigationCall `json:"after,omitempty"`
}

// NavigationGraphDiff classifies semantic declaration and call-edge changes.
type NavigationGraphDiff struct {
	Schema              string                         `json:"schema"`
	AddedDeclarations   []parser.NavigationDeclaration `json:"addedDeclarations,omitempty"`
	RemovedDeclarations []parser.NavigationDeclaration `json:"removedDeclarations,omitempty"`
	MovedDeclarations   []NavigationDeclarationDelta   `json:"movedDeclarations,omitempty"`
	ChangedDeclarations []NavigationDeclarationDelta   `json:"changedDeclarations,omitempty"`
	AddedCalls          []parser.NavigationCall        `json:"addedCalls,omitempty"`
	RemovedCalls        []parser.NavigationCall        `json:"removedCalls,omitempty"`
	ChangedCalls        []NavigationCallDelta          `json:"changedCalls,omitempty"`
}

// DiffNavigationGraphs compares normalized graphs while ignoring position-only shifts.
func DiffNavigationGraphs(before, after parser.NavigationGraph) NavigationGraphDiff {
	diff := NavigationGraphDiff{Schema: NavigationDiffSchema}
	beforeKeys := navigationDeclarationSemanticKeys(before.Declarations)
	afterKeys := navigationDeclarationSemanticKeys(after.Declarations)
	beforeByKey := groupNavigationDeclarations(before.Declarations, beforeKeys)
	afterByKey := groupNavigationDeclarations(after.Declarations, afterKeys)
	for _, key := range sortedStringUnion(beforeByKey, afterByKey) {
		diffNavigationDeclarationGroup(&diff, beforeByKey[key], afterByKey[key])
	}
	diffNavigationCalls(&diff, before, after, beforeKeys, afterKeys)
	return diff
}

func navigationDeclarationSemanticKeys(declarations []parser.NavigationDeclaration) map[string]string {
	keys := make(map[string]string, len(declarations))
	for _, declaration := range declarations {
		scope := declaration.PackageID
		if scope == "" {
			scope = declaration.ModuleID
		}
		keys[declaration.ID] = strings.Join([]string{declaration.Language, scope, declaration.Scope, declaration.Kind, declaration.Name}, "\x00")
	}
	return keys
}

func groupNavigationDeclarations(declarations []parser.NavigationDeclaration, keys map[string]string) map[string][]parser.NavigationDeclaration {
	grouped := make(map[string][]parser.NavigationDeclaration)
	for _, declaration := range declarations {
		key := keys[declaration.ID]
		grouped[key] = append(grouped[key], declaration)
	}
	for key := range grouped {
		sort.SliceStable(grouped[key], func(i, j int) bool {
			left, right := grouped[key][i], grouped[key][j]
			return left.Path < right.Path || left.Path == right.Path && left.Start < right.Start
		})
	}
	return grouped
}

func diffNavigationDeclarationGroup(diff *NavigationGraphDiff, before, after []parser.NavigationDeclaration) {
	paired := min(len(before), len(after))
	for index := range paired {
		oldDeclaration, newDeclaration := before[index], after[index]
		if oldDeclaration.Path != newDeclaration.Path {
			diff.MovedDeclarations = append(diff.MovedDeclarations, NavigationDeclarationDelta{Before: declarationPointer(oldDeclaration), After: declarationPointer(newDeclaration)})
		}
		if navigationDeclarationChanged(oldDeclaration, newDeclaration) {
			diff.ChangedDeclarations = append(diff.ChangedDeclarations, NavigationDeclarationDelta{Before: declarationPointer(oldDeclaration), After: declarationPointer(newDeclaration)})
		}
	}
	diff.RemovedDeclarations = append(diff.RemovedDeclarations, before[paired:]...)
	diff.AddedDeclarations = append(diff.AddedDeclarations, after[paired:]...)
}

func navigationDeclarationChanged(before, after parser.NavigationDeclaration) bool {
	before.ID, after.ID = "", ""
	before.Path, after.Path = "", ""
	before.Start, before.End, after.Start, after.End = 0, 0, 0, 0
	return !reflect.DeepEqual(before, after)
}

func diffNavigationCalls(diff *NavigationGraphDiff, before, after parser.NavigationGraph, beforeKeys, afterKeys map[string]string) {
	beforeGroups := groupNavigationCalls(before.Calls, beforeKeys)
	afterGroups := groupNavigationCalls(after.Calls, afterKeys)
	for _, key := range sortedStringUnion(beforeGroups, afterGroups) {
		oldCalls, newCalls := beforeGroups[key], afterGroups[key]
		paired := min(len(oldCalls), len(newCalls))
		for index := range paired {
			if navigationCallSemanticValue(oldCalls[index], beforeKeys) != navigationCallSemanticValue(newCalls[index], afterKeys) {
				diff.ChangedCalls = append(diff.ChangedCalls, NavigationCallDelta{Before: callPointer(oldCalls[index]), After: callPointer(newCalls[index])})
			}
		}
		diff.RemovedCalls = append(diff.RemovedCalls, oldCalls[paired:]...)
		diff.AddedCalls = append(diff.AddedCalls, newCalls[paired:]...)
	}
}

func groupNavigationCalls(calls []parser.NavigationCall, declarationKeys map[string]string) map[string][]parser.NavigationCall {
	grouped := make(map[string][]parser.NavigationCall)
	for _, call := range calls {
		key := strings.Join([]string{declarationKeys[call.CallerID], call.Name, call.Display}, "\x00")
		grouped[key] = append(grouped[key], call)
	}
	for key := range grouped {
		sort.SliceStable(grouped[key], func(i, j int) bool {
			left, right := grouped[key][i], grouped[key][j]
			return left.Line < right.Line || left.Line == right.Line && left.ID < right.ID
		})
	}
	return grouped
}

func navigationCallSemanticValue(call parser.NavigationCall, declarationKeys map[string]string) string {
	target := declarationKeys[call.TargetID]
	if target == "" {
		target = "unresolved:" + call.ResolvedName
		if target == "unresolved:" {
			target += call.Name
		}
	}
	candidates := make([]string, 0, len(call.CandidateTargetIDs))
	for _, id := range call.CandidateTargetIDs {
		candidates = append(candidates, declarationKeys[id])
	}
	sort.Strings(candidates)
	return strings.Join([]string{target, call.Confidence, strings.Join(candidates, ",")}, "\x00")
}

func sortedStringUnion[T any](left, right map[string]T) []string {
	seen := make(map[string]bool, len(left)+len(right))
	for key := range left {
		seen[key] = true
	}
	for key := range right {
		seen[key] = true
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func declarationPointer(value parser.NavigationDeclaration) *parser.NavigationDeclaration {
	return &value
}
func callPointer(value parser.NavigationCall) *parser.NavigationCall { return &value }
