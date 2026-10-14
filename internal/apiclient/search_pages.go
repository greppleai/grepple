package apiclient

import (
	"context"
	"fmt"
	"github.com/greppleai/grepple/internal/wire"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SearchPager is the stateless current-index content-search transport capability.
type SearchPager interface {
	SearchPage(context.Context, string, SearchPageRequest) (SearchPage, error)
}

// SearchPageRequest resends the query and options on every continuation.
type SearchPageRequest struct {
	Search *wire.SearchRequest `json:"search,omitempty"`
	Cursor string              `json:"cursor,omitempty"`
}

// SearchPage contains current-index results and a query-bound offset cursor.
type SearchPage struct {
	Results  []wire.FileResult `json:"results"`
	Cursor   string            `json:"cursor"`
	Complete bool              `json:"complete"`
	PageID   string            `json:"pageId"`
}

var searchCursorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}\.(0|[1-9][0-9]{0,4})$`)

// ValidSearchCursor validates bounded offset syntax; the token carries no authority.
func ValidSearchCursor(value string) bool {
	if !searchCursorPattern.MatchString(value) {
		return false
	}
	skip, err := strconv.Atoi(strings.Split(value, ".")[1])
	return err == nil && skip <= 10000
}
func (client *apiClient) SearchPage(ctx context.Context, server string, request SearchPageRequest) (SearchPage, error) {
	var page SearchPage
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if request.Search == nil || (request.Cursor != "" && !ValidSearchCursor(request.Cursor)) {
		return page, fmt.Errorf("provide the original query/options and an optional valid cursor")
	}
	if err := client.json(ctx, http.MethodPost, endpoint(server, "/public/search/pages"), request, &page, false); err != nil {
		return page, err
	}
	if !ValidSearchCursor(page.PageID) || len(page.Results) > 100 || (!page.Complete && !ValidSearchCursor(page.Cursor)) || (page.Complete && page.Cursor != "") {
		return SearchPage{}, fmt.Errorf("invalid cursor search response")
	}
	return page, nil
}
