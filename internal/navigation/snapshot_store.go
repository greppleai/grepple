package navigation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/greppleai/grepple/internal/parser"
)

// SnapshotStore publishes immutable files. Open handles pin the same inode until
// Close; replacing a cache pathname cannot truncate an active generation.
type SnapshotStore interface {
	Save(string, string, QuerySnapshot, []byte) error
	SavePaged(string, string, QuerySnapshot, []byte) error
	OpenPaged(string, string) (PagedSnapshot, []byte, error)
	Open(string, string) (StoredSnapshot, []byte, error)
	OpenDeclarations(string, string) (StoredSnapshot, []byte, error)
}

// StoredSnapshot owns a query-ready core and selective detail reader.
type StoredSnapshot interface {
	QuerySnapshot
	Declarations() ([]parser.NavigationDeclaration, error)
	TypeDeclarations() ([]parser.NavigationTypeDeclaration, error)
	RelatedProjection([]string, string, int) (parser.NavigationGraph, error)
	Close() error
}

// PagedSnapshot provides scoped traversal and explicit full projection without
// claiming backend-specific related enrichment or eager declaration lookup.
type PagedSnapshot interface {
	StoredSnapshot
	ReadStats() PagedReadStats
}

// SnapshotLease is the common request-owned graph-reader lifetime contract.
type SnapshotLease interface {
	QuerySnapshot
	Close() error
}
type snapshotStore struct{ observer SnapshotObserver }

// SnapshotStore returns the immutable navigation storage capability.
func (graphOperations) SnapshotStore() SnapshotStore { return snapshotStore{} }

const snapshotStoreMagic = "GNSTO001"
const snapshotStoreHeaderSize = 52
const maxSnapshotManifestBytes = 32 << 20

type snapshotStoredTable struct {
	Count      uint32
	NonNil     bool
	References []snapshotStoredReference
}
type snapshotStoredReference struct {
	Offset uint64
	Length uint32
	Digest [32]byte
}
type snapshotStoredManifest struct {
	Key        string
	Metadata   []byte
	CoreDigest [32]byte
	Tables     []snapshotStoredTable
}
type storedSnapshot struct {
	mu       sync.RWMutex
	snapshot *querySnapshot
	file     *os.File
}

func (s *storedSnapshot) Query(roots []string, direction NavigationQueryDirection, depth int) (parser.NavigationGraph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return parser.NavigationGraph{}, fmt.Errorf("navigation generation closed")
	}
	if s.snapshot.ids == nil {
		return parser.NavigationGraph{}, fmt.Errorf("declaration-only navigation lease")
	}
	return s.snapshot.Query(roots, direction, depth)
}
func (s *storedSnapshot) Graph() (parser.NavigationGraph, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return parser.NavigationGraph{}, fmt.Errorf("navigation generation closed")
	}
	if s.snapshot.ids == nil {
		return parser.NavigationGraph{}, fmt.Errorf("declaration-only navigation lease")
	}
	return s.snapshot.Graph()
}
func (s *storedSnapshot) Declarations() ([]parser.NavigationDeclaration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return nil, fmt.Errorf("navigation generation closed")
	}
	return s.snapshot.details.declarations.readAll()
}
func (s *storedSnapshot) TypeDeclarations() ([]parser.NavigationTypeDeclaration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.file == nil {
		return nil, fmt.Errorf("navigation generation closed")
	}
	return s.snapshot.details.types.readAll()
}
func (s *storedSnapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	s.snapshot = nil
	return err
}

func (snapshotStore) Save(path, key string, prepared QuerySnapshot, metadata []byte) error {
	snapshot, ok := prepared.(*querySnapshot)
	if !ok {
		return fmt.Errorf("unsupported navigation snapshot")
	}
	if len(key) != 64 || len(metadata) > 1<<20 {
		return fmt.Errorf("invalid snapshot identity or metadata")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	core, err := os.CreateTemp(filepath.Dir(path), ".navigation-core-*")
	if err != nil {
		return err
	}
	defer os.Remove(core.Name())
	defer core.Close()
	if err = writeSnapshotCore(core, snapshot); err != nil {
		return err
	}
	size, err := core.Seek(0, io.SeekCurrent)
	if err != nil || size > maxSnapshotCoreBytes {
		return fmt.Errorf("invalid core size: %v", err)
	}
	if _, err = core.Seek(0, io.SeekStart); err != nil {
		return err
	}
	digest := sha256.New()
	if _, err = io.Copy(digest, core); err != nil {
		return err
	}
	manifest := snapshotStoredManifest{Key: key, Metadata: metadata}
	copy(manifest.CoreDigest[:], digest.Sum(nil))
	blocks, err := collectSnapshotStoredTables(snapshot.details, &manifest)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if len(encoded) > maxSnapshotManifestBytes {
		return fmt.Errorf("navigation manifest too large")
	}
	return installSnapshotGeneration(path, core, size, encoded, blocks)
}

func installSnapshotGeneration(path string, core *os.File, size int64, encoded []byte, blocks [][]byte) error {
	return installSnapshotGenerationMagic(path, core, size, encoded, blocks, snapshotStoreMagic)
}
func installSnapshotGenerationMagic(path string, core *os.File, size int64, encoded []byte, blocks [][]byte, magic string) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".navigation-generation-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	defer temporary.Close()
	var header [snapshotStoreHeaderSize]byte
	copy(header[:], magic)
	binary.LittleEndian.PutUint64(header[8:16], uint64(size))
	binary.LittleEndian.PutUint32(header[16:20], uint32(len(encoded)))
	manifestDigest := sha256.Sum256(encoded)
	copy(header[20:], manifestDigest[:])
	if _, err = temporary.Write(header[:]); err != nil {
		return err
	}
	if _, err = temporary.Write(encoded); err != nil {
		return err
	}
	if _, err = core.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if _, err = io.Copy(temporary, core); err != nil {
		return err
	}
	for _, block := range blocks {
		if _, err = temporary.Write(block); err != nil {
			return err
		}
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporary.Name(), path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func appendSnapshotStoredTable[T any](table snapshotDetailTable[T], manifest *snapshotStoredManifest, blocks *[][]byte, offset *uint64) error {
	stored := snapshotStoredTable{Count: table.count, NonNil: table.nonNil}
	count := (uint64(table.count) + snapshotDetailBlockRows - 1) / snapshotDetailBlockRows
	for block := uint64(0); block < count; block++ {
		data, err := table.encodedBlock(uint32(block))
		if err != nil {
			return err
		}
		stored.References = append(stored.References, snapshotStoredReference{Offset: *offset, Length: uint32(len(data)), Digest: sha256.Sum256(data)})
		*offset += uint64(len(data))
		*blocks = append(*blocks, data)
	}
	manifest.Tables = append(manifest.Tables, stored)
	return nil
}
func collectSnapshotStoredTables(details *snapshotDetails, manifest *snapshotStoredManifest) ([][]byte, error) {
	var blocks [][]byte
	var offset uint64
	operations := []func() error{
		func() error { return appendSnapshotStoredTable(details.declarations, manifest, &blocks, &offset) }, func() error { return appendSnapshotStoredTable(details.calls, manifest, &blocks, &offset) },
		func() error { return appendSnapshotStoredTable(details.types, manifest, &blocks, &offset) }, func() error { return appendSnapshotStoredTable(details.imports, manifest, &blocks, &offset) },
		func() error { return appendSnapshotStoredTable(details.exports, manifest, &blocks, &offset) }, func() error { return appendSnapshotStoredTable(details.fields, manifest, &blocks, &offset) },
		func() error { return appendSnapshotStoredTable(details.usages, manifest, &blocks, &offset) }, func() error { return appendSnapshotStoredTable(details.accesses, manifest, &blocks, &offset) }}
	for _, operation := range operations {
		if err := operation(); err != nil {
			return nil, err
		}
	}
	return blocks, nil
}

func (store snapshotStore) Open(path, key string) (StoredSnapshot, []byte, error) {
	return store.open(path, key, true)
}
func (store snapshotStore) OpenDeclarations(path, key string) (StoredSnapshot, []byte, error) {
	return store.open(path, key, false)
}
func (store snapshotStore) open(path, key string, core bool) (StoredSnapshot, []byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	snapshot, metadata, err := openSnapshotGeneration(file, key, core, store.observer)
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	return &storedSnapshot{snapshot: snapshot, file: file}, metadata, nil
}
func openSnapshotGeneration(file *os.File, key string, core bool, observers ...SnapshotObserver) (*querySnapshot, []byte, error) {
	var observer SnapshotObserver
	if len(observers) > 0 {
		observer = observers[0]
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("invalid navigation generation")
	}
	var header [snapshotStoreHeaderSize]byte
	if _, err = io.ReadFull(file, header[:]); err != nil {
		return nil, nil, err
	}
	coreSize := binary.LittleEndian.Uint64(header[8:16])
	manifestSize := binary.LittleEndian.Uint32(header[16:])
	if string(header[:8]) != snapshotStoreMagic || coreSize > uint64(maxSnapshotCoreBytes) || manifestSize > maxSnapshotManifestBytes {
		return nil, nil, fmt.Errorf("invalid navigation generation header")
	}
	payloadOffset := uint64(snapshotStoreHeaderSize) + uint64(manifestSize) + coreSize
	if payloadOffset > uint64(info.Size()) {
		return nil, nil, fmt.Errorf("truncated navigation generation")
	}
	encoded := make([]byte, int(manifestSize))
	if _, err = io.ReadFull(file, encoded); err != nil {
		return nil, nil, err
	}
	var manifestDigest [32]byte
	copy(manifestDigest[:], header[20:])
	if sha256.Sum256(encoded) != manifestDigest {
		return nil, nil, fmt.Errorf("navigation manifest checksum mismatch")
	}
	var manifest snapshotStoredManifest
	if err = json.Unmarshal(encoded, &manifest); err != nil {
		return nil, nil, err
	}
	if manifest.Key != key || len(manifest.Tables) != 8 || len(manifest.Metadata) > 1<<20 {
		return nil, nil, fmt.Errorf("navigation generation identity mismatch")
	}
	snapshot := &querySnapshot{}
	if core {
		coreStage := beginSnapshotStage(observer, "navigation.read_decode_core")
		snapshot, err = readStoredSnapshotCore(file, int64(snapshotStoreHeaderSize)+int64(manifestSize), int64(coreSize), manifest.CoreDigest)
		coreStage()
		if err != nil {
			return nil, nil, err
		}
	}

	detailStage := beginSnapshotStage(observer, "navigation.detail_directory")
	err = attachSnapshotStoredDetails(snapshot, file, int64(payloadOffset), info.Size()-int64(payloadOffset), manifest.Tables)
	detailStage()
	if err != nil {
		return nil, nil, err
	}
	return snapshot, manifest.Metadata, nil
}
func readStoredSnapshotCore(file *os.File, offset, size int64, expected [32]byte) (*querySnapshot, error) {
	digest := sha256.New()
	if _, err := io.Copy(digest, io.NewSectionReader(file, offset, size)); err != nil {
		return nil, err
	}
	var sum [32]byte
	copy(sum[:], digest.Sum(nil))
	if sum != expected {
		return nil, fmt.Errorf("navigation core checksum mismatch")
	}
	return readSnapshotCore(io.NewSectionReader(file, offset, size), size)
}
func attachSnapshotStoredTable[T any](stored snapshotStoredTable, reader io.ReaderAt, size int64, selectRows func(parser.NavigationGraph) []T) (snapshotDetailTable[T], error) {
	table := snapshotDetailTable[T]{count: stored.Count, nonNil: stored.NonNil, selectRows: selectRows}
	if uint64(len(stored.References)) != (uint64(stored.Count)+snapshotDetailBlockRows-1)/snapshotDetailBlockRows || !stored.NonNil && stored.Count != 0 {
		return table, fmt.Errorf("invalid stored detail count")
	}
	references := make([]snapshotDetailBlockReference, len(stored.References))
	for row, reference := range stored.References {
		references[row] = snapshotDetailBlockReference{offset: reference.Offset, length: reference.Length, digest: reference.Digest}
	}
	source, err := newSnapshotDetailReadAt(reader, size, references)
	table.source = source
	return table, err
}
func attachSnapshotStoredDetails(snapshot *querySnapshot, file *os.File, offset, size int64, tables []snapshotStoredTable) error {
	reader := io.NewSectionReader(file, offset, size)
	details := &snapshotDetails{}
	var err error
	details.declarations, err = attachSnapshotStoredTable(tables[0], reader, size, func(g parser.NavigationGraph) []parser.NavigationDeclaration { return g.Declarations })
	if err != nil {
		return err
	}
	details.calls, err = attachSnapshotStoredTable(tables[1], reader, size, func(g parser.NavigationGraph) []parser.NavigationCall { return g.Calls })
	if err != nil {
		return err
	}
	details.types, err = attachSnapshotStoredTable(tables[2], reader, size, func(g parser.NavigationGraph) []parser.NavigationTypeDeclaration { return g.TypeDeclarations })
	if err != nil {
		return err
	}
	details.imports, err = attachSnapshotStoredTable(tables[3], reader, size, func(g parser.NavigationGraph) []parser.NavigationImport { return g.Imports })
	if err != nil {
		return err
	}
	details.exports, err = attachSnapshotStoredTable(tables[4], reader, size, func(g parser.NavigationGraph) []parser.NavigationExport { return g.Exports })
	if err != nil {
		return err
	}
	details.fields, err = attachSnapshotStoredTable(tables[5], reader, size, func(g parser.NavigationGraph) []parser.NavigationField { return g.Fields })
	if err != nil {
		return err
	}
	details.usages, err = attachSnapshotStoredTable(tables[6], reader, size, func(g parser.NavigationGraph) []parser.NavigationTypeUsage { return g.TypeUsages })
	if err != nil {
		return err
	}
	details.accesses, err = attachSnapshotStoredTable(tables[7], reader, size, func(g parser.NavigationGraph) []parser.NavigationMemberAccess { return g.MemberAccesses })
	if err != nil {
		return err
	}
	snapshot.details = details
	return nil
}
