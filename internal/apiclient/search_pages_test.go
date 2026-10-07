package apiclient

import (
	"context"
	"encoding/json"
	"github.com/greppleai/grepple/internal/wire"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func pageToken(letter string) string {
	return strings.Repeat("S", 43) + "." + strings.Repeat(letter, 43)
}
func TestSearchPageWireContinuationReplay(t *testing.T) {
	var requests []SearchPageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/public/search/pages" {
			t.Error("wrong endpoint")
		}
		var body SearchPageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests = append(requests, body)
		_ = json.NewEncoder(w).Encode(SearchPage{Results: []wire.FileResult{{Repo: "owner/repo", Path: "owner/repo/source.go"}}, PageID: pageToken("A"), Cursor: pageToken("B"), ExpiresAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)})
	}))
	defer server.Close()
	pager := New().(SearchPager)
	query := "Needle"
	first, err := pager.SearchPage(context.Background(), server.URL, SearchPageRequest{Search: &wire.SearchRequest{Query: &query}})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := pager.SearchPage(context.Background(), server.URL, SearchPageRequest{Cursor: first.PageID})
	if err != nil || !reflect.DeepEqual(first, replay) {
		t.Fatal("replay changed")
	}
	if requests[1].Search != nil || requests[1].Cursor != first.PageID {
		t.Fatal("query was resent with continuation")
	}
}
func TestSearchPageDoesNotFallbackOnErrorsOrMalformedResponses(t *testing.T) {
	for _, status := range []int{http.StatusGone, http.StatusForbidden, http.StatusServiceUnavailable, http.StatusNotFound, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/public/search/pages" {
					t.Error("legacy fallback attempted")
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"results":[]}`))
			}))
			defer server.Close()
			query := "Needle"
			_, err := New().(SearchPager).SearchPage(context.Background(), server.URL, SearchPageRequest{Search: &wire.SearchRequest{Query: &query}})
			if err == nil || calls != 1 {
				t.Fatal("invalid page accepted or silently retried")
			}
		})
	}
}
