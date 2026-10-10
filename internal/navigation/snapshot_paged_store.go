package navigation

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/greppleai/grepple/internal/parser"
)

const pagedSnapshotMagic = "GNSTO005"
const pagedLocatorRows = 128
const pagedRows = 4096
const maxPagedBlockBytes = 8 << 20

// A page directory holds locators, never decoded adjacency or native facts.
// Offsets are relative to the core. Each independently read page is checked.
type pagedSection struct {
	Count  uint32
	First  []string
	Blocks []snapshotStoredReference
}
type pagedManifest struct {
	Key      string
	Metadata []byte
	Roots    []string
	Sections map[string]pagedSection
	Tables   []snapshotStoredTable
}
type pagedStoredSnapshot struct {
	mu                   sync.RWMutex
	file                 *os.File
	offset               int64
	manifest             pagedManifest
	details              *snapshotDetails
	directoryBytes       uint64
	coreBytes, corePages atomic.Uint64
}

func (snapshotStore) SavePaged(path, key string, prepared QuerySnapshot, metadata []byte) error {
	s, ok := prepared.(*querySnapshot)
	if !ok || s == nil || len(key) != 64 || len(metadata) > 1<<20 {
		return fmt.Errorf("invalid paged snapshot identity")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	core, err := os.CreateTemp(filepath.Dir(path), ".navigation-paged-core-*")
	if err != nil {
		return err
	}
	defer core.Close()
	defer os.Remove(core.Name())
	manifest := pagedManifest{Key: key, Metadata: metadata, Roots: s.roots, Sections: map[string]pagedSection{}}
	if err := writePagedCore(core, s, &manifest); err != nil {
		return err
	}
	size, err := core.Seek(0, io.SeekCurrent)
	if err != nil || size > maxSnapshotCoreBytes {
		return fmt.Errorf("invalid paged core size")
	}
	compatibility := snapshotStoredManifest{}
	blocks, err := collectSnapshotStoredTables(s.details, &compatibility)
	if err != nil {
		return err
	}
	manifest.Tables = compatibility.Tables
	encoded, err := encodePagedManifest(manifest)
	if err != nil {
		return err
	}
	if len(encoded) > maxSnapshotManifestBytes {
		return fmt.Errorf("paged directory too large")
	}
	return installSnapshotGenerationMagic(path, core, size, encoded, blocks, pagedSnapshotMagic)
}
func (store snapshotStore) OpenPaged(path, key string) (PagedSnapshot, []byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	snapshot, err := openPagedSnapshot(file, key, store.observer)
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return snapshot, snapshot.manifest.Metadata, nil
}
func openPagedSnapshot(file *os.File, key string, observers ...SnapshotObserver) (*pagedStoredSnapshot, error) {
	var observer SnapshotObserver
	if len(observers) > 0 {
		observer = observers[0]
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid paged generation")
	}
	var header [snapshotStoreHeaderSize]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint64(header[8:])
	length := binary.LittleEndian.Uint32(header[16:])
	offset := uint64(snapshotStoreHeaderSize) + uint64(length)
	if string(header[:8]) != pagedSnapshotMagic || size > uint64(maxSnapshotCoreBytes) || length > maxSnapshotManifestBytes || offset+size > uint64(info.Size()) {
		return nil, fmt.Errorf("invalid paged header")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, err
	}
	var digest [32]byte
	copy(digest[:], header[20:])
	if sha256.Sum256(data) != digest {
		return nil, fmt.Errorf("paged directory checksum mismatch")
	}
	directoryStage := beginSnapshotStage(observer, "navigation.decode_directory")
	manifest, err := decodePagedManifest(data)
	directoryStage()
	if err != nil {
		return nil, err
	}
	if manifest.Key != key || len(manifest.Metadata) > 1<<20 {
		return nil, fmt.Errorf("paged generation identity mismatch")
	}
	if err := validatePagedSections(manifest.Sections, size); err != nil {
		return nil, err
	}
	holder := &querySnapshot{}
	detailStage := beginSnapshotStage(observer, "navigation.detail_directory")
	err = attachSnapshotStoredDetails(holder, file, int64(offset+size), info.Size()-int64(offset+size), manifest.Tables)
	detailStage()
	if err != nil {
		return nil, err
	}
	return &pagedStoredSnapshot{file: file, offset: int64(offset), manifest: manifest, details: holder.details, directoryBytes: uint64(snapshotStoreHeaderSize) + uint64(length)}, nil
}
func (s *pagedStoredSnapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	s.details = nil
	return err
}
func (s *pagedStoredSnapshot) Query(ids []string, d NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	pagedQueryAdmission <- struct{}{}
	defer func() { <-pagedQueryAdmission }()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return parser.NavigationGraph{}, fmt.Errorf("paged generation closed")
	}
	return queryPagedSnapshot(s, ids, d, depth)
}
func (s *pagedStoredSnapshot) Graph() (parser.NavigationGraph, error) {
	pagedQueryAdmission <- struct{}{}
	defer func() { <-pagedQueryAdmission }()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return parser.NavigationGraph{}, fmt.Errorf("paged generation closed")
	}
	return graphPagedSnapshot(s)
}
func (s *pagedStoredSnapshot) ReadStats() PagedReadStats {
	return PagedReadStats{DirectoryBytes: s.directoryBytes, CoreBytesRead: s.coreBytes.Load(), CorePagesRead: s.corePages.Load()}
}
