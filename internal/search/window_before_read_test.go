package search

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestWindowBeforeReadEquivalence verifies the bounded (early-stop) read path
// returns exactly the same windowed results as an unbounded scan windowed by
// hand — the whole point being that it may read fewer files, not different ones.
func TestWindowBeforeReadEquivalence(t *testing.T) {
	root := t.TempDir()
	candidates := writeNeedleFixture(t, root)
	previous, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	base := Params{Query: "needle", Root: root}
	reference := referenceOrder(t, base, candidates)

	for _, tc := range []struct{ skip, limit int }{{0, 5}, {2, 3}, {5, 100}, {0, 1}, {18, 5}} {
		verifyWindowCase(t, base, candidates, reference, tc.skip, tc.limit)
	}
}

// writeNeedleFixture writes 30 files (every third lacks the needle, so
// early-stop must read past non-matching candidates to fill the window) and
// returns them in reverse order to prove ordering is enforced.
func writeNeedleFixture(t *testing.T, root string) []string {
	t.Helper()
	var candidates []string
	for i := 0; i < 30; i++ {
		abs := filepath.Join(root, "owner/repo", fmt.Sprintf("file%02d.go", i))
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatal(err)
		}
		body := "package x\n"
		if i%3 != 0 {
			body += "// needle\n"
		}
		if err := os.WriteFile(abs, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, abs)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))
	return candidates
}

// referenceOrder runs the unbounded scan and returns its display paths,
// asserting they are emitted in sorted order.
func referenceOrder(t *testing.T, base Params, candidates []string) []string {
	t.Helper()
	full, err := Files(base, candidates)
	if err != nil {
		t.Fatal(err)
	}
	reference := make([]string, len(full))
	for i, m := range full {
		reference[i] = m.DisplayPath
	}
	if !sort.StringsAreSorted(reference) {
		t.Fatalf("unbounded results not in display order: %v", reference)
	}
	return reference
}

// verifyWindowCase checks one skip/limit window against the same window
// sliced out of the unbounded reference by hand.
func verifyWindowCase(t *testing.T, base Params, candidates, reference []string, skip, limit int) {
	t.Helper()
	p := base
	p.Skip, p.Limit = skip, limit
	got, err := Files(p, candidates)
	if err != nil {
		t.Fatal(err)
	}
	// Expected window over the reference set.
	lo := min(skip, len(reference))
	hi := lo
	if limit > 0 {
		hi = min(lo+limit, len(reference))
	}
	want := reference[lo:hi]
	if len(got) != len(want) {
		t.Fatalf("skip=%d limit=%d: got %d files, want %d", skip, limit, len(got), len(want))
	}
	for i := range got {
		if got[i].DisplayPath != want[i] {
			t.Fatalf("skip=%d limit=%d pos %d: got %q want %q", skip, limit, i, got[i].DisplayPath, want[i])
		}
	}
}

// TestWindowBeforeReadStopsEarly proves a bounded query reads far fewer files
// than the candidate set when matches are plentiful, rather than reading them
// all like the old code.
func TestWindowBeforeReadStopsEarly(t *testing.T) {
	root := t.TempDir()
	const n = 2000
	var candidates []string
	for i := 0; i < n; i++ {
		abs := filepath.Join(root, "owner/repo", fmt.Sprintf("file%04d.go", i))
		if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("package x\n// needle\n"), 0644); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, abs)
	}
	previous, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	readInvocations.Store(0)
	got, err := Files(Params{Query: "needle", Root: root, Limit: 5}, candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 windowed files, got %d", len(got))
	}
	if reads := readInvocations.Load(); reads >= n {
		t.Fatalf("bounded query read %d of %d candidates — early-stop not working", reads, n)
	}
}
