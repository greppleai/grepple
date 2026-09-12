package cli

import (
	"encoding/json"
	"os"
	"testing"
)

func TestCountRendersMatchingLinesPerFile(t *testing.T) {
	dir := chdirTemp(t)
	if err := os.WriteFile(dir+"/a.txt", []byte("needle needle\nno\nneedle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/b.txt", []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := runSearch([]string{"--count", "needle", "a.txt", "b.txt"}); err != nil {
			t.Fatal(err)
		}
	})
	if want := "a.txt\t2\nb.txt\t1\n"; out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
}

func TestCountSummaryIsCompleteIndependentOfPaging(t *testing.T) {
	dir := chdirTemp(t)
	for name, content := range map[string]string{
		"a.txt": "needle\nneedle\n",
		"b.txt": "needle\n",
		"c.txt": "needle\n",
	} {
		if err := os.WriteFile(dir+"/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out := captureStdout(t, func() {
		if err := runSearch([]string{"--count-summary", "--limit", "1", "--skip", "10", "needle", "."}); err != nil {
			t.Fatal(err)
		}
	})
	if want := "3 files\t4 matches\n"; out != want {
		t.Fatalf("expected %q, got %q", want, out)
	}
	jsonOutput := captureStdout(t, func() {
		if err := runSearch([]string{"--count-summary", "--json", "needle", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var decoded struct {
		Count struct {
			Files, Matches int
		} `json:"count"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Count.Files != 3 || decoded.Count.Matches != 4 {
		t.Fatalf("count summary=%#v", decoded.Count)
	}
}

func TestCountAndCountSummaryAreMutuallyExclusive(t *testing.T) {
	for _, alias := range []string{"--count-summary", "--count-by-repo"} {
		if _, _, _, err := parseSearchArgs([]string{"--count", alias, "needle"}); err == nil {
			t.Fatalf("expected --count with %s to fail", alias)
		}
	}
}
