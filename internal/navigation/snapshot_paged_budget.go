package navigation

import (
	"errors"
	"fmt"
)

// ErrSnapshotBudget denotes explicit incomplete-query failure, not corruption.
// Callers must not bypass it by rebuilding an unbounded rich graph.
var ErrSnapshotBudget = errors.New("navigation query budget exceeded")

// ErrSnapshotReadBudget identifies cumulative encoded page reads specifically.
var ErrSnapshotReadBudget = fmt.Errorf("%w: paged query read limit", ErrSnapshotBudget)

// Admission limits simultaneous paged traversals; generation directories and
// returned output remain owned by their callers, outside the page cache budget.
var pagedQueryAdmission = make(chan struct{}, 2)
