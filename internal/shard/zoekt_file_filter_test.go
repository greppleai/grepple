package shard

import (
	"grepple/internal/api"
	"regexp"
	"strings"
	"testing"
)

// TestGlobToPathRegexpIsCompilableSuperset checks that each translated glob is a
// valid RE2 regexp and matches (at least) the repo-relative paths the glob is
// meant to select — the file: atom must never be narrower than the glob.
func TestGlobToPathRegexpIsCompilableSuperset(t *testing.T) {
	cases := []struct {
		glob      string
		matches   []string // repo-relative paths the regexp must match
		noMatches []string // paths it should exclude (best-effort narrowing)
	}{
		{"**/package.json", []string{"package.json", "app/package.json", "a/b/c/package.json"}, []string{"readme.md", "package.jsonx"}},
		{"**/*.tsx", []string{"x.tsx", "src/x.tsx", "a/b/x.tsx"}, []string{"x.ts", "x.tsxx"}},
		{"src/*.go", []string{"src/main.go", "pkg/src/main.go"}, []string{"src/main.rs"}},
		{"Dockerfile", []string{"Dockerfile", "deploy/Dockerfile"}, []string{"Dockerfile.dev"}},
	}
	for _, c := range cases {
		re, ok := globToPathRegexp(c.glob)
		if !ok {
			t.Fatalf("glob %q should translate", c.glob)
		}
		rx, err := regexp.Compile(re)
		if err != nil {
			t.Fatalf("glob %q -> %q is not valid RE2: %v", c.glob, re, err)
		}
		for _, p := range c.matches {
			if !rx.MatchString(p) {
				t.Errorf("glob %q -> %q must match %q (would drop a real match)", c.glob, re, p)
			}
		}
		for _, p := range c.noMatches {
			if rx.MatchString(p) {
				t.Errorf("glob %q -> %q unexpectedly matched %q", c.glob, re, p)
			}
		}
	}
}

func TestGlobToPathRegexpRejectsUnsafe(t *testing.T) {
	for _, g := range []string{"src /*.go", "a\t.go", "bad[class"} {
		if _, ok := globToPathRegexp(g); ok {
			t.Errorf("glob %q must be rejected as unsafe to translate", g)
		}
	}
}

func TestZoektFileFilterFormatAndAllOrNothing(t *testing.T) {
	// A single glob produces one quoted file: atom whose contents compile.
	f := zoektFileFilter([]string{"**/package.json"})
	if !strings.HasPrefix(f, ` file:"`) || !strings.HasSuffix(f, `"`) {
		t.Fatalf("unexpected atom shape: %q", f)
	}
	// No globs => no atom.
	if zoektFileFilter(nil) != "" {
		t.Fatal("no globs must yield no file: atom")
	}
	// If any glob is untranslatable, the whole pushdown is skipped so the OR atom
	// can't wrongly exclude that glob's files (post-filter still enforces it).
	if zoektFileFilter([]string{"**/*.tsx", "bad glob"}) != "" {
		t.Fatal("one untranslatable glob must disable pushdown entirely")
	}
}

// TestZoektRepoCountsAppliesGlobs verifies the count path applies the
// authoritative glob matcher to the index response, not just the file: atom.
func TestZoektRepoCountsAppliesGlobs(t *testing.T) {
	body := `{"Result":{"Files":[
		{"Repository":"owner/a","FileName":"app/package.json","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":1}}]}]},
		{"Repository":"owner/a","FileName":"src/index.ts","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":2}}]}]},
		{"Repository":"owner/b","FileName":"package.json","ChunkMatches":[{"Ranges":[{"Start":{"LineNumber":3}}]}]}
	]}}`
	port := serveZoekt(t, body)
	allow := []api.RepoInfo{{Repo: "owner/a"}, {Repo: "owner/b"}}
	counts, _, err := zoektRepoCounts(zoektOptions{port: port}, "x", allow, []string{"**/package.json"})
	if err != nil {
		t.Fatal(err)
	}
	// src/index.ts must be excluded by the glob; owner/a and owner/b keep one
	// package.json each.
	if len(counts) != 2 {
		t.Fatalf("expected 2 repos, got %#v", counts)
	}
	for _, c := range counts {
		if c.Files != 1 || c.Matches != 1 {
			t.Fatalf("glob should leave exactly one package.json per repo, got %#v", c)
		}
	}
}
