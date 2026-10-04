package parser

import "strconv"

type svelteNavigation struct{ navigationAdapter }

func (*svelteNavigation) FactCapabilities() NavigationFactCapabilities {
	// No process entrypoints are inferred from scripts inside a component.
	facts := adapterForLanguage("typescript").Navigation().FactCapabilities()
	facts.Entrypoints = false
	return facts
}

func svelteScopeGraph(graph *NavigationGraph, root *syntaxNode) {
	prefix := "svelte-body@" + strconv.Itoa(int(root.tree.offset))
	ids := make(map[string]string, len(graph.Declarations))
	for i := range graph.Declarations {
		declaration := &graph.Declarations[i]
		previous := declaration.ID
		declaration.Scope = prefix + "/" + declaration.Scope
		declaration.Entrypoint = ""
		declaration.ID = navigationDeclarationStableID(*declaration)
		ids[previous] = declaration.ID
	}
	for i := range graph.Calls {
		call := &graph.Calls[i]
		call.CallerID = ids[call.CallerID]
	}
	for i := range graph.Imports {
		graph.Imports[i].Scope = prefix + "/" + graph.Imports[i].Scope
	}
	for i := range graph.TypeUsages {
		graph.TypeUsages[i].CallerID = ids[graph.TypeUsages[i].CallerID]
	}
	for i := range graph.MemberAccesses {
		graph.MemberAccesses[i].CallerID = ids[graph.MemberAccesses[i].CallerID]
	}
	// Instance exports are component props, not JavaScript module exports. Module
	// exports/component import resolution are intentionally not inferred either.
	graph.Exports = nil
}
