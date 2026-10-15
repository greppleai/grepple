package parser

import "strings"

// ScopeCandidate is an inclusive source-line range a presentation wants to make selectable.
type ScopeCandidate struct {
	StartLine int
	EndLine   int
}

// VerifiedScopeSelectors returns only parser-verified public scopes whose exact
// range matches one of candidates. It parses code at most once per call; callers
// should batch a file's presentation candidates.
func VerifiedScopeSelectors(content, language string, candidates []ScopeCandidate) map[ScopeCandidate]ScopeLineRange {
	wanted := make(map[ScopeCandidate]bool, len(candidates))
	for _, candidate := range candidates {
		if candidate.StartLine > 0 && candidate.EndLine >= candidate.StartLine {
			wanted[candidate] = true
		}
	}
	if len(wanted) == 0 || strings.IndexByte(content, 0) >= 0 {
		return nil
	}
	if languageAdapters[language] != nil {
		document, err := parseDocument(language, content)
		if err != nil {
			return nil
		}
		defer document.Close()
		return verifiedCodeScopeSelectors(document, wanted)
	}
	result := make(map[ScopeCandidate]ScopeLineRange)
	for candidate := range wanted {
		ambiguous := false
		for kind := range publicScopeKinds {
			selector := ScopeSelector{Kind: kind, StartLine: candidate.StartLine, EndLine: candidate.EndLine, Exact: true}
			if span, status := ResolveScopeSelector(content, language, selector, nil); status == ScopeVerified {
				if prior, found := result[candidate]; found && prior != span {
					ambiguous = true
				} else {
					result[candidate] = span
				}
			}
		}
		if ambiguous {
			delete(result, candidate)
		}
	}
	return result
}

func verifiedCodeScopeSelectors(document *Document, wanted map[ScopeCandidate]bool) map[ScopeCandidate]ScopeLineRange {
	result := make(map[ScopeCandidate]ScopeLineRange)
	ambiguous := make(map[ScopeCandidate]bool)
	_ = document.Read(func(view DocumentView) error {
		stack := []scopeWalkItem{{viewWalkItem: viewWalkItem{node: view.Root(), depth: 1}}}
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			node := current.node
			if !node.Valid() {
				continue
			}
			r := node.Range()
			candidate := ScopeCandidate{StartLine: r.Start.Line, EndLine: inclusiveEndLine(r)}
			if wanted[candidate] && current.depth > 1 && !node.IsError() && !node.IsMissing() && !node.HasError() {
				if kind := scopeKindForNode(node); kind != "" {
					span := ScopeLineRange{Kind: nativeScopeKind(effectiveScopeLanguage(node), kind), StartLine: candidate.StartLine, EndLine: candidate.EndLine}
					if prior, found := result[candidate]; found && prior != span {
						ambiguous[candidate] = true
					} else {
						result[candidate] = span
					}
				}
			}
			if injected := node.raw.injectionRoot(); injected != nil {
				stack = append(stack, scopeWalkItem{viewWalkItem: viewWalkItem{node: ViewNode{view: node.view, raw: injected}, depth: current.depth + 1}, injected: true})
			}
			children := node.NamedChildren()
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, scopeWalkItem{viewWalkItem: viewWalkItem{node: children[i], depth: current.depth + 1}, injected: current.injected})
			}
		}
		return nil
	})
	for candidate := range ambiguous {
		delete(result, candidate)
	}
	return result
}
