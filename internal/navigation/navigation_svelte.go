package navigation

import "strings"

// Separate script bodies do not share implicit lexical bindings. Module-to-
// instance exposure and compiler-generated component bindings need real evidence.
func svelteScriptScopesMatch(source, candidate string) bool {
	if !strings.HasPrefix(source, "svelte-body@") {
		return true
	}
	sourceBody, _, _ := strings.Cut(source, "/")
	candidateBody, _, _ := strings.Cut(candidate, "/")
	return sourceBody == candidateBody
}

func filterSvelteScriptCandidates(call navigationCall, candidates []navigationDeclaration) []navigationDeclaration {
	if !strings.HasPrefix(call.moduleScope, "svelte-body@") {
		return candidates
	}
	return filterNavigationCandidates(candidates, func(candidate navigationDeclaration) bool {
		if candidate.file == call.file {
			return svelteScriptScopesMatch(call.moduleScope, candidate.moduleScope)
		}
		return call.importPath != ""
	})
}
