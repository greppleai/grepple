package parser

import "testing"

// find returns the first symbol in the tree (depth-first) matching kind+name.
func find(symbols []Symbol, kind, name string) (Symbol, bool) {
	for _, s := range symbols {
		if s.Kind == kind && s.Name == name {
			return s, true
		}
		if child, ok := find(s.Children, kind, name); ok {
			return child, true
		}
	}
	return Symbol{}, false
}

func TestOutlineFromDocumentDoesNotReparse(t *testing.T) {
	content := "package sample\ntype Item struct{}\nfunc Build() Item { return Item{} }\n"
	before := parseInvocations.Load()
	document, err := ParseDocument("go", content)
	if err != nil {
		t.Fatal(err)
	}
	defer document.Close()
	afterParse := parseInvocations.Load()
	outline := OutlineFromDocument("sample.go", document)
	if afterParse != before+1 || parseInvocations.Load() != afterParse {
		t.Fatalf("parse counts before=%d after=%d final=%d", before, afterParse, parseInvocations.Load())
	}
	if len(outline.Symbols) != 2 || outline.Symbols[0].Name != "Item" || outline.Symbols[1].Name != "Build" {
		t.Fatalf("outline=%#v", outline)
	}
}

func mustFind(t *testing.T, symbols []Symbol, kind, name string) Symbol {
	t.Helper()
	s, ok := find(symbols, kind, name)
	if !ok {
		t.Fatalf("expected a %s named %q in outline", kind, name)
	}
	return s
}

func TestOutlineGo(t *testing.T) {
	src := `package p

type Rule struct {
	ID string
}

type Store interface {
	Get(id string) (Rule, error)
}

func New() *Store { return nil }

func (s *Store) Get(id string) (Rule, error) {
	return Rule{}, nil
}
`
	o := OutlineFile("registry.go", src)
	if o.Language != "go" {
		t.Fatalf("language = %q", o.Language)
	}
	s := mustFind(t, o.Symbols, "struct", "Rule")
	if s.Start != 3 || s.End != 5 {
		t.Fatalf("Rule bounds = %d-%d, want 3-5", s.Start, s.End)
	}
	iface := mustFind(t, o.Symbols, "interface", "Store")
	if _, ok := find(iface.Children, "method", "Get"); !ok {
		t.Fatal("interface method Get should be a child of Store")
	}
	mustFind(t, o.Symbols, "func", "New")
	// Methods carry a receiver prefix and stay at top level (Go declares them at
	// file scope).
	m := mustFind(t, o.Symbols, "method", "(*Store).Get")
	if m.Start != 13 {
		t.Fatalf("method start = %d, want 13", m.Start)
	}
}

func TestOutlineTypeScript(t *testing.T) {
	src := `export interface RuleResult { repo: string }

export class RuleRegistry {
  constructor(path: string) {}
  upsert(rule: Rule): Rule { return rule; }
}

export function loadRules(dir: string): Rule[] { return []; }
const handler = (x: number) => x + 1;
`
	o := OutlineFile("registry.ts", src)
	if o.Language != "typescript" {
		t.Fatalf("language = %q", o.Language)
	}
	mustFind(t, o.Symbols, "interface", "RuleResult")
	cls := mustFind(t, o.Symbols, "class", "RuleRegistry")
	mustFind(t, cls.Children, "method", "upsert")
	mustFind(t, o.Symbols, "function", "loadRules")
	// An arrow function assigned to a const is reported as a function.
	mustFind(t, o.Symbols, "function", "handler")
}

func TestOutlineJava(t *testing.T) {
	src := `package p;

public interface Sink {
	void accept(String s);
}

public class Registry {
	private int gen;
	public Registry() {}
	public Rule upsert(Rule r) { return r; }
}
`
	o := OutlineFile("Registry.java", src)
	if o.Language != "java" {
		t.Fatalf("language = %q", o.Language)
	}
	mustFind(t, o.Symbols, "interface", "Sink")
	cls := mustFind(t, o.Symbols, "class", "Registry")
	mustFind(t, cls.Children, "constructor", "Registry")
	mustFind(t, cls.Children, "method", "upsert")
	mustFind(t, cls.Children, "field", "gen")
}

func TestOutlineMarkdown(t *testing.T) {
	src := "# Title\n\nintro\n\n## Levels\n\n```\n## not a heading\n```\n\n### Debug\n\n## Rules\n"
	o := OutlineFile("doc.md", src)
	if o.Language != "markdown" {
		t.Fatalf("language = %q", o.Language)
	}
	h1 := mustFind(t, o.Symbols, "h1", "Title")
	levels := mustFind(t, h1.Children, "h2", "Levels")
	// The fenced "## not a heading" must be ignored.
	if _, ok := find(o.Symbols, "h2", "not a heading"); ok {
		t.Fatal("fenced code should not produce a heading")
	}
	// Debug (h3) nests under Levels and ends before Rules.
	mustFind(t, levels.Children, "h3", "Debug")
	rules := mustFind(t, o.Symbols, "h2", "Rules")
	if rules.Start != 13 {
		t.Fatalf("Rules start = %d, want 13", rules.Start)
	}
}

func TestOutlineUnsupportedLanguage(t *testing.T) {
	o := OutlineFile("data.rb", "class Foo\nend\n")
	if o.Language != "text" {
		t.Fatalf("language = %q", o.Language)
	}
	if len(o.Symbols) != 0 {
		t.Fatalf("expected no symbols for unsupported language, got %d", len(o.Symbols))
	}
	if o.Symbols == nil {
		t.Fatal("Symbols should be non-nil (empty slice) for stable JSON")
	}
}
