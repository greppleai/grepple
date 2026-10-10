package navigation

// SnapshotObserver observes fixed logical operations only. Callbacks must be
// nonblocking and must not panic. No paths, source or identity are passed.
type SnapshotObserver interface{ Start(string) func() }

// SnapshotErrorObserver optionally records the error of a logical operation.
type SnapshotErrorObserver interface{ StartError(string) func(error) }

// NewObservedSnapshotStore uses the same immutable reader with optional timing.
func NewObservedSnapshotStore(observer SnapshotObserver) SnapshotStore {
	return snapshotStore{observer: observer}
}

// BeginSnapshotStage is shared by immutable storage and result attachment.
func beginSnapshotStage(observer SnapshotObserver, operation string) func() {
	if observer != nil {
		return observer.Start(operation)
	}
	return func() {}
}
