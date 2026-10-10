package api

// OpenArtifact is an explicit cold migration operation, never a query fallback.
// It reconstructs the pinned active generation, preserving native empty shapes
// that a compatibility protobuf round trip cannot faithfully reproduce.
func (store navigationSnapshotStore) OpenArtifact(path, key string) (IndexedNavigation, []byte, error) {
	snapshot, metadata, err := store.store.Open(path, key)
	if err != nil {
		return nil, nil, err
	}
	defer snapshot.Close()
	graph, err := snapshot.Graph()
	if err != nil {
		return nil, nil, err
	}
	return indexedNavigation{graph: graph, complete: true}, metadata, nil
}
