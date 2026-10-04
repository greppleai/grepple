package navigation

import (
	"github.com/greppleai/grepple/internal/parser"
	"path/filepath"
	"testing"
)

func TestSvelteEmbeddedCallsStayInTheirScriptAndResolveImports(t *testing.T) {
	root := t.TempDir()
	component := filepath.Join(root, "App.svelte")
	utility := filepath.Join(root, "utils.ts")
	source := `<script lang="ts" module>
function helper() { return 1; }
function moduleRun() { helper(); }
</script>
<script lang="ts">
import { external } from './utils';
function helper() { return 2; }
function instanceRun() { helper(); external(); }
</script>
<style>.button { color: red; }</style>`
	analysis, stats := NewGraphEngine(BuildOptions{DisableCache: true}).BuildTextSources([]TextSource{
		{Path: component, Text: source},
		{Path: utility, Text: "export function external() { return 3; }"},
	})
	if stats.Failed != 0 || stats.Recovered != 0 {
		t.Fatalf("parse stats: %+v", stats)
	}
	graph := analysis.Graph()
	declarations := map[string]string{}
	for _, declaration := range graph.Declarations {
		declarations[declaration.ID] = declaration.Scope
	}
	local, imported := assertSvelteResolvedCalls(t, graph.Calls, component, declarations)
	if local != 2 || imported != 1 {
		t.Fatalf("unexpected calls: %+v", graph.Calls)
	}
}

func assertSvelteResolvedCalls(t *testing.T, calls []parser.NavigationCall, component string, declarations map[string]string) (int, int) {
	t.Helper()
	var local, imported int
	for _, call := range calls {
		if call.Path != component {
			continue
		}
		switch call.Name {
		case "helper":
			if call.TargetID == "" || declarations[call.TargetID] != declarations[call.CallerID] {
				t.Fatalf("call crossed script scopes: %+v", call)
			}
			local++
		case "external":
			if call.TargetID == "" {
				t.Fatalf("exact script import unresolved: %+v", call)
			}
			imported++
		}
	}
	return local, imported
}
