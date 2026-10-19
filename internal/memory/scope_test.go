package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopesNormalizeAndMatchWithoutFilesystemExpansion(t *testing.T) {
	tests := []struct {
		scope, candidate string
		match            bool
	}{
		{"internal/cli/", "internal/cli/args.go", true},
		{"internal/cli/", "internal/cli/search/command.go", true},
		{"internal/cli/*.go", "internal/cli/args.go", true},
		{"internal/cli/*.go", "internal/cli/search/command.go", false},
		{"internal/cli/**/*.go", "internal/cli/args.go", true},
		{"internal/cli/**/*.go", "internal/cli/search/command.go", true},
		{"internal/cli/*_test.go", "internal/cli/args.go", false},
		{"internal/cli/*_test.go", "internal/cli/args_test.go", true},
		{"internal/cli/*.go", "internal/CLI/args.go", false},
	}
	for _, test := range tests {
		t.Run(test.scope+"/"+test.candidate, func(t *testing.T) {
			got, err := NormalizeScope(test.scope)
			if err != nil || got != test.scope {
				t.Fatalf("NormalizeScope(%q) = %q, %v", test.scope, got, err)
			}
			if got := ScopeMatches(test.scope, test.candidate); got != test.match {
				t.Fatalf("ScopeMatches(%q, %q) = %v", test.scope, test.candidate, got)
			}
		})
	}
}

func TestScopeValidationAndStorePrefix(t *testing.T) {
	root := fixture(t)
	if err := os.MkdirAll(filepath.Join(root, "internal", "cli"), 0700); err != nil {
		t.Fatal(err)
	}
	for input, want := range map[string]string{
		"internal/cli":         "internal/cli/",
		"internal/cli/":        "internal/cli/",
		"internal/cli/*.go":    "internal/cli/*.go",
		"internal/cli/**/*.go": "internal/cli/**/*.go",
	} {
		got, err := ScopeForStore(root, input)
		if err != nil || got != want {
			t.Fatalf("ScopeForStore(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	invalid := []string{
		"", "a.go", "/", "*", "**/*", "internal/cli/**", "internal/**/cli/*.go",
		"internal/cli/***.go", "internal/cli/file?.go", "internal/cli/[ab].go", "../outside/",
		".git/", ".grepple/", "linked/", "missing/*.go", "internal/cli/\x00",
	}
	for _, value := range invalid {
		if _, err := ScopeForStore(root, value); err == nil {
			t.Fatalf("accepted unsafe scope input %q", value)
		}
	}
	tooDeep := strings.Repeat("a/", scopePrefixLimit+1) + "*.go"
	if _, err := NormalizeScope(tooDeep); err == nil {
		t.Fatalf("accepted deep scope %q", tooDeep)
	}
	if _, err := NormalizeScope("internal/cli/" + strings.Repeat("x", scopeBytesLimit)); err == nil {
		t.Fatal("accepted oversized scope")
	}
}

func TestHeaderScopesAreCanonicalAndLegacyCompatible(t *testing.T) {
	legacy := Header{Schema: "grepple-memory-v1", Name: "legacy", Description: "Legacy", Keywords: []string{"source"}}
	if err := legacy.Normalize(); err != nil {
		t.Fatalf("legacy header: %v", err)
	}
	h := Header{Schema: "grepple-memory-v1", Name: "scoped", Description: "Scoped", Scopes: []string{"internal/cli/**/*.go", "internal/cli/"}}
	if err := h.Normalize(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(h.Scopes, ","); got != "internal/cli/,internal/cli/**/*.go" {
		t.Fatalf("canonical scopes %q", got)
	}
	if _, err := Encode(h, []byte("body")); err != nil {
		t.Fatalf("scope-only memory rejected: %v", err)
	}
	duplicate := h
	duplicate.Scopes = []string{"internal/cli/", "internal/cli/"}
	if err := duplicate.Normalize(); err == nil {
		t.Fatal("accepted duplicate scopes")
	}
}
