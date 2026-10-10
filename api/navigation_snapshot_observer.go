package api

import (
	"fmt"
	"github.com/greppleai/grepple/internal/navigation"
)

// NavigationObserver times fixed logical phases without exposing private data.
// Implementations must not block or panic; default APIs remain unobserved.
type NavigationObserver interface{ Start(string) func() }

// NewObservedNavigationSnapshotStore adds optional timing to the existing reader.
func NewObservedNavigationSnapshotStore(observer NavigationObserver) NavigationSnapshotStore {
	return navigationSnapshotStore{store: navigation.NewObservedSnapshotStore(observer)}
}

// NavigationSnapshotReadStats excludes native details, output and physical I/O.
type NavigationSnapshotReadStats struct {
	Paged                                        bool
	DirectoryBytes, CoreBytesRead, CorePagesRead uint64
}

// ReadNavigationSnapshotStats reports successful paged reads for this lease.
func ReadNavigationSnapshotStats(snapshot NavigationSnapshot) NavigationSnapshotReadStats {
	value, ok := snapshot.(*navigationSnapshot)
	if !ok {
		return NavigationSnapshotReadStats{}
	}
	paged, ok := value.snapshot.(navigation.PagedSnapshot)
	if !ok {
		return NavigationSnapshotReadStats{}
	}
	stats := paged.ReadStats()
	return NavigationSnapshotReadStats{Paged: true, DirectoryBytes: stats.DirectoryBytes, CoreBytesRead: stats.CoreBytesRead, CorePagesRead: stats.CorePagesRead}
}

// AttachNavigationSnapshotObserved uses the same result engine with phase timing.
func AttachNavigationSnapshotObserved(snapshot NavigationSnapshot, plan SearchPlan, batch SearchBatch, root, buildCWD string, observer NavigationObserver) (SearchBatch, error) {
	value, ok := snapshot.(*navigationSnapshot)
	if !ok {
		return nil, fmt.Errorf("unsupported navigation snapshot")
	}
	return value.attach(plan, batch, root, buildCWD, observer)
}
