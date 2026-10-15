package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func testScopeSpan(kind string, start, end int) *parser.ScopeLineRange {
	return &parser.ScopeLineRange{Kind: kind, StartLine: start, EndLine: end}
}

func TestAnnotatedLineOutputPreservesSourceAndDeduplicatesSpans(t *testing.T) {
	source := " runtime.Start() │ café @func:1-99 @if:3-4  "
	sourceMatch := wire.ResultMatch{Line: 3, Text: source, Nearest: testScopeSpan("if", 3, 4), Scope: testScopeSpan("func", 2, 5)}
	same := testScopeSpan("func", 2, 5)
	results := []wire.FileResult{{Path: "call.go", Matches: []wire.ResultMatch{sourceMatch, {Line: 2, Text: "func run() {", Nearest: same, Scope: same}, {Line: 4, Text: "unclassified  ", Nearest: testScopeSpan("identifier", 4, 4), Scope: testScopeSpan("func", 0, 9)}}}}
	for _, tc := range []struct {
		name    string
		anchors AnchorLookup
		want    string
	}{
		{"plain", nil, fmt.Sprintf("call.go:3:%s @if:3-4 @func:2-5\ncall.go:2:func run() { @func:2-5\ncall.go:4:unclassified  \n", source)},
		{"anchored", AnchorLookup{"call.go": {2: "AAA", 3: "BBB", 4: "CCC"}}, fmt.Sprintf("call.go\n\nAAA│2│func run() { @func:2-5\nBBB│3│%s @if:3-4 @func:2-5\nCCC│4│unclassified  \n", source)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			_, err := Render(Options{JSON: "off", LineOnly: true, Anchors: tc.anchors, Params: search.Params{AnnotateScopes: true}}, results, &output)
			if err != nil {
				t.Fatal(err)
			}
			if output.String() != tc.want {
				t.Fatalf("rendered=%q\nwant=%q", output.String(), tc.want)
			}
		})
	}
	// Neither sort order nor decorations are added to the old plain format.
	var old bytes.Buffer
	_, err := Render(Options{JSON: "off", LineOnly: true}, results, &old)
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("call.go:3:%s\ncall.go:2:func run() {\ncall.go:4:unclassified  \n", source); old.String() != want {
		t.Fatalf("unannotated output changed: %q, want %q", old.String(), want)
	}
}

func TestAnnotatedHumanIsDisplayOnlyWhenSourceLooksLikeASuffix(t *testing.T) {
	const literal = "work() @func:2-5"
	results := []wire.FileResult{{Path: "call.go", Matches: []wire.ResultMatch{{Line: 3, Text: literal}}}}
	var human bytes.Buffer
	if _, err := Render(Options{JSON: "off", LineOnly: true, Params: search.Params{AnnotateScopes: true}}, results, &human); err != nil {
		t.Fatal(err)
	}
	if human.String() != "call.go:3:"+literal+"\n" {
		t.Fatalf("source-shaped suffix was changed: %q", human.String())
	}
	var structured bytes.Buffer
	if _, err := Render(Options{JSON: "matches", LineOnly: true, Params: search.Params{AnnotateScopes: true}}, results, &structured); err != nil {
		t.Fatal(err)
	}
	var data struct {
		Matches []struct {
			Text    string                 `json:"text"`
			Nearest *parser.ScopeLineRange `json:"nearest"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(structured.Bytes(), &data); err != nil || len(data.Matches) != 1 || data.Matches[0].Text != literal || data.Matches[0].Nearest != nil {
		t.Fatalf("JSON must disambiguate fake suffix: %s (%v)", structured.String(), err)
	}
}

func TestVerifiedRelatedSelectorRequiresCurrentExactScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.go")
	if err := os.WriteFile(path, []byte("// doc\nfunc Run() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := verifiedRelatedSelector(path, 1, 2); got != "" {
		t.Fatalf("comment-inclusive graph span got %q", got)
	}
	if got := verifiedRelatedSelector(path, 2, 2); got != " @func:2-2" {
		t.Fatalf("verified local selector got %q", got)
	}
}

func TestAnnotatedLineValidationCRLFAndOutputCap(t *testing.T) {
	match := wire.ResultMatch{Line: 2, Text: "é @while:1-3\r\n", Nearest: testScopeSpan("type_identifier", 1, 3), Scope: testScopeSpan("method", 1, 1)}
	results := []wire.FileResult{{Path: "x.go", Matches: []wire.ResultMatch{match}}}
	var output bytes.Buffer
	_, err := Render(Options{JSON: "off", LineOnly: true, Params: search.Params{AnnotateScopes: true}}, results, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "x.go:2:é @while:1-3\n" {
		t.Fatalf("invalid spans or CRLF normalization: %q", output.String())
	}
	output.Reset()
	results[0].Matches[0].Text = strings.Repeat("é", 200) + " @func:1-4"
	_, err = Render(Options{JSON: "off", LineOnly: true, MaxOutputBytes: 160, Params: search.Params{AnnotateScopes: true}}, results, &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Len() > 160 || !strings.Contains(output.String(), "truncated") || strings.Contains(output.String(), "⟦source-bytes:") {
		t.Fatalf("partial annotated row: %q", output.String())
	}
}

func TestScopeAnnotationsInBothJSONFormatsAndUnoptedCompatibility(t *testing.T) {
	nearest := testScopeSpan("if", 2, 3)
	result := []wire.FileResult{{Path: "x.go", Matches: []wire.ResultMatch{{Line: 2, Text: "work()", Nearest: nearest, Scope: testScopeSpan("func", 1, 4), StartLine: 1, EndLine: 4}, {Line: 3, Text: "work()", Nearest: testScopeSpan("raw_node", 2, 3), Scope: testScopeSpan("func", 9, 10)}}}}
	unchanged := append([]wire.ResultMatch(nil), result[0].Matches...)
	for _, mode := range []string{"full", "matches"} {
		for _, enabled := range []bool{true, false} {
			var out bytes.Buffer
			_, err := Render(Options{JSON: mode, LineOnly: mode == "matches", Params: search.Params{AnnotateScopes: enabled}}, result, &out)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]json.RawMessage
			if err = json.Unmarshal(out.Bytes(), &decoded); err != nil {
				t.Fatal(err)
			}
			var matches []map[string]any
			if mode == "full" {
				var files []struct {
					Matches []map[string]any `json:"matches"`
				}
				if err = json.Unmarshal(decoded["results"], &files); err != nil || len(files) != 1 {
					t.Fatalf("bad full JSON: %s (%v)", out.String(), err)
				}
				matches = files[0].Matches
			} else if err = json.Unmarshal(decoded["matches"], &matches); err != nil {
				t.Fatal(err)
			}
			if len(matches) != 2 || matches[0]["text"] != "work()" || matches[0]["startLine"] != float64(1) || matches[0]["endLine"] != float64(4) {
				t.Fatalf("mutated source/ranges: %s", out.String())
			}
			if enabled {
				first, ok := matches[0]["nearest"].(map[string]any)
				if !ok || first["kind"] != "if" {
					t.Fatalf("missing nearest: %s", out.String())
				}
				if _, ok := matches[0]["scope"].(map[string]any); !ok {
					t.Fatalf("missing scope: %s", out.String())
				}
			} else if _, ok := matches[0]["nearest"]; ok {
				t.Fatalf("unopted roles leaked: %s", out.String())
			}
			if _, ok := matches[1]["nearest"]; ok {
				t.Fatalf("unverified role leaked: %s", out.String())
			}
			if _, ok := matches[1]["scope"]; ok {
				t.Fatalf("out-of-bounds role leaked: %s", out.String())
			}
		}
	}
	if !reflect.DeepEqual(result[0].Matches, unchanged) {
		t.Fatalf("render mutated source results: %+v", result[0].Matches)
	}
}
