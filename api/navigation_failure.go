package api

import (
	"errors"
	"github.com/greppleai/grepple/internal/navigation"
)

// NavigationFailureCode reports fixed diagnostic identifiers, never error text.
func NavigationFailureCode(err error) string {
	if errors.Is(err, navigation.ErrSnapshotReadBudget) {
		return "navigation_read_budget_exceeded"
	}
	if errors.Is(err, navigation.ErrSnapshotBudget) {
		return "navigation_budget_exceeded"
	}
	return ""
}
