package gritql

import "testing"

func TestLuaAdapterRegistration(t *testing.T) {
	adapter, ok := targetLanguageByID("lua")
	if !ok || adapter.grammar != LuaGrammar || adapter.treeSitter != TreeSitterLuaGrammar || adapter.compileTemplates == nil {
		t.Fatalf("Lua adapter=%#v registered=%v", adapter, ok)
	}
}
