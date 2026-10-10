package api

import (
	"fmt"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

// NavigationSnapshotStore persists full semantic artifacts as query-ready
// immutable generations. Identity and source revision fencing remain host-owned.
type NavigationSnapshotStore interface {
	Save(string, string, IndexedNavigation, []byte) error
	Open(string, string) (NavigationSnapshot, []byte, error)
	OpenDeclarations(string, string) (NavigationSnapshot, []byte, error)
	OpenArtifact(string, string) (IndexedNavigation, []byte, error)
	SavePaged(string, string, IndexedNavigation, []byte) error
	OpenPaged(string, string) (NavigationSnapshot, []byte, error)
}

// NavigationSnapshot pins a file generation; callers must close request leases.
type NavigationSnapshot interface {
	DeclarationIndex() (IndexedNavigation, error)
	Attach(SearchPlan, SearchBatch, string, string) (SearchBatch, error)
	Close() error
}
type navigationSnapshotStore struct{ store navigation.SnapshotStore }
type navigationSnapshot struct{ snapshot navigation.StoredSnapshot }

// NewNavigationSnapshotStore exposes immutable storage without parser layouts.
func NewNavigationSnapshotStore() NavigationSnapshotStore {
	return navigationSnapshotStore{store: navigation.NewGraphOperations().SnapshotStore()}
}
func (store navigationSnapshotStore) Save(path, key string, artifact IndexedNavigation, metadata []byte) error {
	value, ok := artifact.(indexedNavigation)
	if !ok || !value.complete {
		return fmt.Errorf("unsupported navigation artifact")
	}
	snapshot, err := navigation.NewGraphOperations().PrepareQuery(value.graph)
	if err != nil {
		return err
	}
	return store.store.Save(path, key, snapshot, metadata)
}
func (store navigationSnapshotStore) Open(path, key string) (NavigationSnapshot, []byte, error) {
	snapshot, metadata, err := store.store.Open(path, key)
	if err != nil {
		return nil, nil, err
	}
	return &navigationSnapshot{snapshot: snapshot}, metadata, nil
}
func (snapshot *navigationSnapshot) DeclarationIndex() (IndexedNavigation, error) {
	declarations, err := snapshot.snapshot.Declarations()
	if err != nil {
		return nil, err
	}
	types, err := snapshot.snapshot.TypeDeclarations()
	if err != nil {
		return nil, err
	}
	return indexedNavigation{graph: parser.NavigationGraph{Declarations: declarations, TypeDeclarations: types}}, nil
}
func (snapshot *navigationSnapshot) Close() error { return snapshot.snapshot.Close() }
func (store navigationSnapshotStore) OpenDeclarations(path, key string) (NavigationSnapshot, []byte, error) {
	snapshot, metadata, err := store.store.OpenDeclarations(path, key)
	if err != nil {
		return nil, nil, err
	}
	return &navigationSnapshot{snapshot: snapshot}, metadata, nil
}
