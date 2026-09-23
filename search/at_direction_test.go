package search

import (
	"testing"
)

// Exact retrieval is an outgoing call walk, unlike bidirectional search matches.
// Depth one includes only direct calls; each higher depth exposes one more hop.
func TestAtFollowsOnlyOutgoingCallsToRequestedDepth(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	entry := writeGoFixture(t, root, "entry.go", "package related\nfunc entry() string { return target() }\nfunc inbound() string { return entry() }\n")
	writeGoFixture(t, root, "target.go", "package related\nfunc target() string { return leaf() }\nfunc sibling() string { return target() }\n")
	writeGoFixture(t, root, "leaf.go", "package related\nfunc leaf() string { return bottom() }\nfunc bottom() string { return \"done\" }\n")
	for _, depth := range []int{1, 2, 3} {
		match, err := At(Params{At: entry + ":2", Root: root, Related: true, FollowRelated: depth})
		if err != nil {
			t.Fatal(err)
		}
		if match.OmittedRelatedCallers != 0 {
			t.Fatalf("depth %d: unexpected caller omission %d", depth, match.OmittedRelatedCallers)
		}
		if len(match.Related) != 1 || match.Related[0].Name != "target" || match.Related[0].Direction != "callee" {
			t.Fatalf("depth %d: root navigation=%#v", depth, match.Related)
		}
		target := match.Related[0]
		if depth == 1 {
			if target.Preview != nil {
				t.Fatalf("depth 1 followed an extra hop: %#v", target.Preview)
			}
			continue
		}
		if target.Preview == nil || target.Preview.OmittedCallers != 0 || len(target.Preview.Related) != 1 || target.Preview.Related[0].Name != "leaf" || target.Preview.Related[0].Direction != "callee" {
			t.Fatalf("depth %d: target navigation=%#v", depth, target.Preview)
		}
		leaf := target.Preview.Related[0]
		if depth == 2 {
			if leaf.Preview != nil {
				t.Fatalf("depth 2 followed an extra hop: %#v", leaf.Preview)
			}
			continue
		}
		if leaf.Preview == nil || leaf.Preview.OmittedCallers != 0 || len(leaf.Preview.Related) != 1 || leaf.Preview.Related[0].Name != "bottom" || leaf.Preview.Related[0].Direction != "callee" {
			t.Fatalf("depth 3: leaf navigation=%#v", leaf.Preview)
		}
	}
}
