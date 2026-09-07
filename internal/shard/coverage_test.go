package shard

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"grepple/internal/repository"
)

// TestZoektSupplementalCandidatesIsPerRepo verifies that unknown Zoekt coverage
// is handled per-repo (never a full-shard scan) and, crucially, that an
// unknown-coverage repo which already has a Zoekt shard is trusted (served from
// the index) rather than scanned — only a repo with no shard is scanned.
func TestZoektSupplementalCandidatesIsPerRepo(t *testing.T) {
	root := t.TempDir()
	indexDir := t.TempDir()
	coveredDir := filepath.Join(root, "owner", "covered")
	noShardDir := filepath.Join(root, "owner", "noshard")
	withShardDir := filepath.Join(root, "owner", "withshard")

	registry := repository.NewRegistry(root)
	if err := registry.Init(); err != nil {
		t.Fatal(err)
	}
	registry.Record("owner/covered", "", "HEAD", coveredDir)
	registry.Record("owner/noshard", "", "HEAD", noShardDir)
	registry.Record("owner/withshard", "", "HEAD", withShardDir)

	// Give owner/withshard an on-disk Zoekt shard so shardExists() is true.
	shardPath := filepath.Join(indexDir, zoektShardPrefix("owner/withshard")+"_v16.00000.zoekt")
	if err := os.WriteFile(shardPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fallbackFile := filepath.Join(coveredDir, "untracked.txt")
	state := &shardImpl{
		registry: registry,
		root:     root,
		zoektService: &zoektServiceImpl{
			options: zoektOptions{indexDir: indexDir},
			coverage: map[string]zoektCoverage{
				"owner/covered": {known: true, indexed: true, fallback: []string{fallbackFile}},
				// owner/noshard and owner/withshard deliberately absent => unknown.
			},
		},
	}

	fallback, scanDirs, uncoveredRepos := state.zoektSupplementalCandidates()

	if !slices.Equal(fallback, []string{fallbackFile}) {
		t.Fatalf("covered repo fallback should be returned, got %v", fallback)
	}
	// Only the shard-less repo is scanned; the one with a shard is trusted.
	if !slices.Equal(scanDirs, []string{noShardDir}) {
		t.Fatalf("only the shard-less repo dir should be scanned, got %v", scanDirs)
	}
	if slices.Contains(scanDirs, withShardDir) {
		t.Fatal("a repo with an existing shard must NOT be scanned")
	}
	if slices.Contains(scanDirs, coveredDir) {
		t.Fatal("a covered repo must not be scanned")
	}
	// Both unknown-coverage repos are reported for diagnostics.
	slices.Sort(uncoveredRepos)
	if !slices.Equal(uncoveredRepos, []string{"owner/noshard", "owner/withshard"}) {
		t.Fatalf("uncoveredRepos should list both unknown repos, got %v", uncoveredRepos)
	}
}
