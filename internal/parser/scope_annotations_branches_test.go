package parser

import "testing"

func TestCodeLineScopeAnnotationsControlFlowAcrossLanguages(t *testing.T) {
	tests := []struct {
		language, content                                    string
		line, nearestStart, nearestEnd, scopeStart, scopeEnd int
		nearest, scope                                       string
	}{
		{"go", "package p\nfunc run() {\n if true {\n  work()\n }\n}\n", 4, 3, 5, 2, 6, "if", "func"},
		{"java", "class A {\n void run() {\n  if (true) {\n   work();\n  }\n }\n}\n", 4, 3, 5, 2, 6, "if", "method"},
		{"kotlin", "fun run() {\n if (true) {\n  println(1)\n }\n}\n", 3, 2, 4, 1, 5, "if", "func"},
		{"dart", "void run() {\n if (true) {\n  print(1);\n }\n}\n", 3, 2, 4, 1, 5, "if", "func"},
		{"swift", "func run() {\n if true {\n  print(1)\n }\n}\n", 3, 2, 4, 1, 5, "if", "func"},
		{"svelte", "{#if ready}\n  <p>yes</p>\n{/if}\n", 2, 1, 3, 0, 0, "if", ""},
		{"javascript", "function run() {\n if (true) {\n  work();\n }\n}\n", 3, 2, 4, 1, 5, "if", "func"},
		{"typescript", "function run(): void {\n if (true) {\n  work();\n }\n}\n", 3, 2, 4, 1, 5, "if", "func"},
		{"tsx", "function Run() {\n if (true) {\n  return <div />;\n }\n}\n", 3, 2, 4, 1, 5, "if", "func"},
		{"python", "def run():\n    if True:\n        work()\n    done()\n", 3, 2, 3, 1, 4, "if", "func"},
		{"csharp", "class A {\n void Run() {\n  if (true) {\n   work();\n  }\n }\n}\n", 4, 3, 5, 2, 6, "if", "method"},
		{"c", "int run(void) {\n if (1) {\n  work();\n }\n return 1;\n}\n", 3, 2, 4, 1, 6, "if", "func"},
		{"cpp", "int run() {\n if (true) {\n  work();\n }\n return 1;\n}\n", 3, 2, 4, 1, 6, "if", "func"},
		{"rust", "fn run() {\n if true {\n  work();\n }\n}\n", 3, 2, 4, 1, 5, "if", "func"},
		{"php", "<?php\nfunction run() {\n if (true) {\n  work();\n }\n}\n", 4, 3, 5, 2, 6, "if", "func"},
		{"shell", "run() {\n if true; then\n  echo yes\n fi\n}\n", 3, 2, 4, 1, 5, "if", "func"},
	}
	for _, tt := range tests {
		t.Run(tt.language, func(t *testing.T) {
			annotation := CodeLineScopeAnnotations(tt.content, tt.language, map[int]bool{tt.line: true})[tt.line]
			wantNearest := ScopeLineRange{tt.nearest, tt.nearestStart, tt.nearestEnd}
			if annotation.Nearest == nil || *annotation.Nearest != wantNearest {
				t.Errorf("nearest = %+v, want %+v", annotation.Nearest, wantNearest)
			}
			if tt.scope == "" {
				if annotation.Scope != nil {
					t.Errorf("unexpected scope = %+v", annotation.Scope)
				}
				return
			}
			wantScope := ScopeLineRange{tt.scope, tt.scopeStart, tt.scopeEnd}
			if annotation.Scope == nil || *annotation.Scope != wantScope {
				t.Errorf("scope = %+v, want %+v", annotation.Scope, wantScope)
			}
		})
	}
}
