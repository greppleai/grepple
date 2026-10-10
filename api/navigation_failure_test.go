package api

import (
	"errors"
	"fmt"
	"github.com/greppleai/grepple/internal/navigation"
	"testing"
)

func TestNavigationFailureCode(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{{nil, ""}, {errors.New("secret: paged query read limit"), ""}, {navigation.ErrSnapshotReadBudget, "navigation_read_budget_exceeded"}, {fmt.Errorf("private: %w", navigation.ErrSnapshotReadBudget), "navigation_read_budget_exceeded"}, {navigation.ErrSnapshotBudget, "navigation_budget_exceeded"}} {
		if got := NavigationFailureCode(test.err); got != test.code {
			t.Fatalf("code %q want %q", got, test.code)
		}
	}
}
