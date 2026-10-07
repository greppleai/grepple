package search

import (
	"context"
	"errors"
	"testing"
)

func TestImmutableNavigationBuildFailureIsNotRetained(t *testing.T) {
	cache := testImmutableCache()
	failure := errors.New("discovery failed")
	if _, err := cache.resolve(context.Background(), "failed", func() (*navigationIndex, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatal("build error lost")
	}
	if len(cache.entries) != 0 || cache.bytes != 0 {
		t.Fatal("failed lookup retained")
	}
	if _, err := cache.resolve(context.Background(), "failed", func() (*navigationIndex, error) { return newNavigationIndex(), nil }); err != nil {
		t.Fatal("retry did not recover")
	}
}
func TestImmutableRepositoryLookupPolicyIsolation(t *testing.T) {
	params := Params{Root: "/root", ImmutableNavigationRevision: "fixed"}
	roots := []string{"/root/owner/repo"}
	key := immutableRepositoryNavigationKey(params, roots)
	variants := []Params{params, params, params, params, params}
	variants[0].IgnorePaths = []string{"**/fixture/**"}
	variants[1].ProductionOnly = true
	variants[2].Repo = []string{"owner/repo"}
	variants[3].ExcludeRepo = []string{"owner/excluded"}
	variants[4].IgnoreRoot = "/different"
	for _, variant := range variants {
		if key == immutableRepositoryNavigationKey(variant, roots) {
			t.Fatal("policy changed without lookup invalidation")
		}
	}
}
