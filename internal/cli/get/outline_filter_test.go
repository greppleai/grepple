package get

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/parser"
)

func TestGetOutlineKindFiltersHumanAndJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("format") == "json" {
			http.Error(writer, "outline should fetch raw source", http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte("package p\n\ntype Widget interface { Run() }\nfunc Build() {}\nvar Count int\n"))
	}))
	defer server.Close()
	var output, diagnostics bytes.Buffer
	exit := 0
	application := getTestApplication(&output, &diagnostics, &exit)
	if err := New(application).Run([]string{"--server", server.URL, "--outline", "--kind", "functions", "owner/repo", "widget.go"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "method\tRun") || !strings.Contains(output.String(), "func\tBuild") || strings.Contains(output.String(), "interface\tWidget") || strings.Contains(output.String(), "package p") {
		t.Fatalf("filtered remote outline=%q", output.String())
	}
	output.Reset()
	if err := New(application).Run([]string{"--server", server.URL, "--outline", "--json", "--kind", "types", "owner/repo", "widget.go"}); err != nil {
		t.Fatal(err)
	}
	var outline parser.FileOutline
	if err := json.Unmarshal(output.Bytes(), &outline); err != nil || len(outline.Symbols) != 1 || outline.Symbols[0].Name != "Widget" || len(outline.Symbols[0].Children) != 0 {
		t.Fatalf("remote JSON=%s err=%v", output.String(), err)
	}
	for _, args := range [][]string{
		{"--kind", "types", "owner/repo", "widget.go"},
		{"--outline", "--kind", "bogus", "owner/repo", "widget.go"},
	} {
		if err := New(application).Run(args); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}
