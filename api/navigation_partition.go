package api

import (
	"fmt"
	"github.com/greppleai/grepple/internal/navigation"
)

// NavigationPartition retains exact semantic links, including cross-language
// candidates and shared physical-file context. Artifact handles are read-only.
type NavigationPartition struct {
	Languages []string
	Artifact  IndexedNavigation
}

// PartitionNavigationArtifact delegates coherent language grouping to the shared engine.
func PartitionNavigationArtifact(artifact IndexedNavigation) ([]NavigationPartition, error) {
	value, ok := artifact.(indexedNavigation)
	if !ok || !value.complete {
		return nil, fmt.Errorf("unsupported navigation artifact")
	}
	parts := navigation.NewGraphOperations().PartitionLanguages(value.graph)
	result := make([]NavigationPartition, 0, len(parts))
	for _, part := range parts {
		result = append(result, NavigationPartition{Languages: part.Languages, Artifact: indexedNavigation{graph: part.Graph, complete: true}})
	}
	return result, nil
}
func (store navigationSnapshotStore) SavePaged(path, key string, artifact IndexedNavigation, metadata []byte) error {
	value, ok := artifact.(indexedNavigation)
	if !ok || !value.complete {
		return fmt.Errorf("unsupported navigation artifact")
	}
	snapshot, err := navigation.NewGraphOperations().PrepareQuery(value.graph)
	if err != nil {
		return err
	}
	return store.store.SavePaged(path, key, snapshot, metadata)
}
func (store navigationSnapshotStore) OpenPaged(path, key string) (NavigationSnapshot, []byte, error) {
	snapshot, metadata, err := store.store.OpenPaged(path, key)
	if err != nil {
		return nil, nil, err
	}
	return &navigationSnapshot{snapshot: snapshot}, metadata, nil
}
