package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/cliruntime"
	engine "github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"
)

func TestChildWindowParamsBoundaries(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		name      string
		params    engine.Params
		mixed     bool
		wantLimit int
		wantError bool
	}{
		{"cap equality", engine.Params{Skip: 75, Limit: 25}, true, 100, false},
		{"cross cap", engine.Params{Skip: 75, Limit: 26}, true, 0, true},
		{"large skip", engine.Params{Skip: maxInt, Limit: 1}, true, 0, true},
		{"local overflow", engine.Params{Skip: maxInt, Limit: 1}, false, 0, true},
		{"local beyond cap", engine.Params{Skip: 200, Limit: 50}, false, 250, false},
		{"local unbounded", engine.Params{Skip: 200}, false, 0, false},
		{"mixed unbounded", engine.Params{}, true, 0, true},
		{"max files prefix", engine.Params{Skip: 95, Limit: 100, MaxFiles: 5}, true, 100, false},
		{"max files crosses cap", engine.Params{Skip: 96, Limit: 100, MaxFiles: 5}, true, 0, true},
		{"max files unbounded limit", engine.Params{Skip: 5, MaxFiles: 5}, false, 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := childWindowParams(tc.params, tc.mixed)
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tc.wantError)
			}
			wantMaxFiles := 0
			if tc.params.MaxFiles > 0 {
				wantMaxFiles = tc.wantLimit
			}
			if err == nil && (got.Skip != 0 || got.Limit != tc.wantLimit || got.MaxFiles != wantMaxFiles) {
				t.Fatalf("child params=%+v", got)
			}
		})
	}
}

func TestMixedWindowMergesRankedPrefixes(t *testing.T) {
	chdirTemp(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GREPPLE_TOKEN", "")
	writeGraphSource(t, ".", "a.txt", "needle\n")
	writeGraphSource(t, ".", "c.txt", "needle\n")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request wire.SearchRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Skip == nil || *request.Skip != 0 || request.Limit == nil || *request.Limit != 3 {
			t.Errorf("unexpected prefix: %+v", request)
		}
		_ = json.NewEncoder(w).Encode(wire.SearchResponse{Results: []wire.FileResult{{Path: "b.txt"}, {Path: "d.txt"}}})
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	exit := 0
	app := cliruntime.Environment{Output: &stdout, ErrorOutput: &stderr, Exit: func(code int) { exit = code }}
	err := New(app).Run([]string{"--remote", "--server", server.URL, "--files-with-matches", "-F", "needle", "--skip", "1", "--limit", "2", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Files) != 2 || output.Files[0].Path != "b.txt" || output.Files[1].Path != "c.txt" {
		t.Fatalf("wrong merged window: %s", &stdout)
	}
	if exit != 0 || calls != 1 || stderr.Len() != 0 {
		t.Fatalf("exit=%d calls=%d stderr=%s", exit, calls, &stderr)
	}

	for _, limit := range []string{"0", "100", "200"} {
		stdout.Reset()
		stderr.Reset()
		err := New(app).Run([]string{"--remote", "--server", server.URL, "--files-with-matches", "-F", "needle", "--skip", "1", "--limit", limit, "--json"})
		if err == nil || !strings.Contains(err.Error(), "--remote-only or --local") {
			t.Fatalf("limit %s: error=%v", limit, err)
		}
		if stdout.Len() != 0 || calls != 1 {
			t.Fatalf("unsupported window rendered/contacted server: %s, calls=%d", &stdout, calls)
		}
	}
}

func TestLocalWindowBeyondServerCap(t *testing.T) {
	root := chdirTemp(t)
	t.Setenv("HOME", t.TempDir())
	for i := 0; i < 130; i++ {
		writeGraphSource(t, root, fmt.Sprintf("file-%03d.txt", i), "needle\n")
	}
	var stdout, stderr bytes.Buffer
	app := cliruntime.Environment{Output: &stdout, ErrorOutput: &stderr}
	if err := New(app).Run([]string{"--local", "-F", "needle", "--files-with-matches", "--skip", "110", "--limit", "5", "--max-files", "3", "--json"}); err != nil {
		t.Fatal(err)
	}
	var output struct {
		Files []struct {
			Path string `json:"path"`
		} `json:"files"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Files) != 3 {
		t.Fatalf("wrong local window: %s", &stdout)
	}
	for i, file := range output.Files {
		if file.Path != fmt.Sprintf("file-%03d.txt", 110+i) {
			t.Errorf("wrong file: %q", file.Path)
		}
	}
}
