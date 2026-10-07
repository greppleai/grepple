package apiclient

import (
	"context"
	"fmt"
	"github.com/greppleai/grepple/internal/wire"
	"net/http"
	"regexp"
	"time"
)

// SearchPager is the opt-in snapshot-bound content-search transport capability.
type SearchPager interface {
	SearchPage(context.Context, string, SearchPageRequest) (SearchPage, error)
}

// SearchPageRequest starts a search or resumes one opaque page identity.
type SearchPageRequest struct {
	Search *wire.SearchRequest `json:"search,omitempty"`
	Cursor string              `json:"cursor,omitempty"`
}

// SearchPage contains one retry-stable page and its continuation identity.
type SearchPage struct {
	Results   []wire.FileResult `json:"results"`
	Cursor    string            `json:"cursor"`
	Complete  bool              `json:"complete"`
	PageID    string            `json:"pageId"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

var searchCursorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}\.[A-Za-z0-9_-]{43}$`)

// ValidSearchCursor validates transport syntax, not authority or expiration.
func ValidSearchCursor(value string) bool { return searchCursorPattern.MatchString(value) }
func (client *apiClient) SearchPage(ctx context.Context, server string, request SearchPageRequest) (SearchPage, error) {
	var page SearchPage
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if (request.Search == nil) == (request.Cursor == "") || (request.Cursor != "" && !ValidSearchCursor(request.Cursor)) {
		return page, fmt.Errorf("provide a search or a valid cursor, not both")
	}
	if err := client.json(ctx, http.MethodPost, endpoint(server, "/public/search/pages"), request, &page, false); err != nil {
		return page, err
	}
	if !ValidSearchCursor(page.PageID) || page.ExpiresAt.IsZero() || len(page.Results) > 100 || (!page.Complete && !ValidSearchCursor(page.Cursor)) || (page.Complete && page.Cursor != "") {
		return SearchPage{}, fmt.Errorf("invalid cursor search response")
	}
	return page, nil
}
