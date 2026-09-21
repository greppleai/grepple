package cli

import (
	"encoding/json"
	"github.com/greppleai/grepple/api"
	"strings"
	"testing"
)

func TestSearchJSONIncludesStandardPagingMetadata(t *testing.T) {
	dir := chdirTemp(t)
	writeGraphSource(t, dir, "a.txt", "needle\n")
	writeGraphSource(t, dir, "b.txt", "needle\n")
	output := captureStdout(t, func() {
		if err := Run([]string{"search", "--json", "--limit", "1", "needle", "."}); err != nil {
			t.Fatal(err)
		}
	})
	var response api.SearchResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Fatal(err)
	}
	metadata := response.Metadata
	if metadata == nil || metadata.Page.Returned != 1 || metadata.Page.Limit != 1 || metadata.Page.Complete || metadata.NextCommand == "" {
		t.Fatalf("search metadata=%#v", metadata)
	}
	if !strings.Contains(metadata.NextCommand, "grepple search --skip 1 --limit 1 --json") {
		t.Fatalf("search continuation is not copyable: %q", metadata.NextCommand)
	}
}
