package navigation

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestSnapshotDiskDetailsRichParityAndConcurrentReaders(t *testing.T) {
	graph := navigationQueryFixture()
	prepared, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	snapshotTestDiskDetails(t, prepared.(*querySnapshot))
	for _, direction := range []NavigationQueryDirection{NavigationQueryCallers, NavigationQueryCallees, NavigationQueryDependencies, NavigationQueryDependents, NavigationQueryImpact} {
		for depth := 1; depth <= 3; depth++ {
			assertSnapshotQueryParity(t, graph, prepared, []string{"root"}, direction, depth)
		}
	}
	var wait sync.WaitGroup
	for n := 0; n < 8; n++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			assertSnapshotQueryParity(t, graph, prepared, []string{"root"}, NavigationQueryImpact, 2)
		}()
	}
	wait.Wait()
}

func TestSnapshotReadAtPinsOpenGenerationAndFailsAfterTruncation(t *testing.T) {
	encoder, err := newSnapshotBlockEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.writer.Close()
	block, err := encoder.encode([]byte("generation one"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "generation.bin")
	if err := os.WriteFile(path, block, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reference := snapshotDetailBlockReference{length: uint32(len(block)), digest: sha256.Sum256(block)}
	source, err := newSnapshotDetailReadAt(file, int64(len(block)), []snapshotDetailBlockReference{reference})
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(directory, "replacement.bin")
	if err := os.WriteFile(replacement, []byte("not the original generation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	actual, err := source.ReadBlock(0)
	if err != nil || !reflect.DeepEqual(actual, block) {
		t.Fatalf("reader switched generations: %v", err)
	}
	if err := file.Truncate(1); err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReadBlock(0); err == nil {
		t.Fatal("truncated generation produced partial output")
	}
}

type snapshotCountingReadAt struct {
	reader       io.ReaderAt
	reads, bytes atomic.Uint64
}

func (reader *snapshotCountingReadAt) ReadAt(data []byte, offset int64) (int, error) {
	n, err := reader.reader.ReadAt(data, offset)
	reader.reads.Add(1)
	reader.bytes.Add(uint64(n))
	return n, err
}

func snapshotTestDiskDetails(t *testing.T, snapshot *querySnapshot) *snapshotCountingReadAt {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(t.TempDir(), "details.bin"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	reader := &snapshotCountingReadAt{reader: file}
	snapshotTestDiskTable(t, &snapshot.details.declarations, file, reader)
	snapshotTestDiskTable(t, &snapshot.details.calls, file, reader)
	snapshotTestDiskTable(t, &snapshot.details.types, file, reader)
	snapshotTestDiskTable(t, &snapshot.details.imports, file, reader)
	snapshotTestDiskTable(t, &snapshot.details.exports, file, reader)
	snapshotTestDiskTable(t, &snapshot.details.fields, file, reader)
	snapshotTestDiskTable(t, &snapshot.details.usages, file, reader)
	snapshotTestDiskTable(t, &snapshot.details.accesses, file, reader)
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("detail_disk_bytes=%d", info.Size())
	return reader
}

func snapshotTestDiskTable[T any](t *testing.T, table *snapshotDetailTable[T], file *os.File, reader io.ReaderAt) {
	t.Helper()
	references := make([]snapshotDetailBlockReference, 0, len(table.blocks))
	for _, block := range table.blocks {
		offset, err := file.Seek(0, io.SeekCurrent)
		if err != nil {
			t.Fatal(err)
		}
		n, err := file.Write(block)
		if err != nil || n != len(block) {
			t.Fatalf("write test detail block: %v", err)
		}
		references = append(references, snapshotDetailBlockReference{offset: uint64(offset), length: uint32(len(block)), digest: sha256.Sum256(block)})
	}
	size, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	source, err := newSnapshotDetailReadAt(reader, size, references)
	if err != nil {
		t.Fatal(err)
	}
	table.source = source
	table.blocks = nil
}

func TestSnapshotDiskDetailsParityAndSelectiveReads(t *testing.T) {
	graph := snapshotNeighborhoodFixture(10000)
	snapshot, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	prepared := snapshot.(*querySnapshot)
	reader := snapshotTestDiskDetails(t, prepared)
	if reader.reads.Load() != 0 {
		t.Fatal("opening details read record payloads")
	}
	actual, err := snapshot.Query([]string{"0"}, NavigationQueryCallees, 2)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := queryNavigationGraph(graph, []string{"0"}, NavigationQueryCallees, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("disk hydration changed query facts")
	}
	if reader.reads.Load() != 2 || reader.bytes.Load() > 16384 {
		t.Fatalf("unbounded reads: blocks=%d bytes=%d", reader.reads.Load(), reader.bytes.Load())
	}
	t.Logf("selected_detail_reads=%d selected_detail_bytes=%d", reader.reads.Load(), reader.bytes.Load())
}

func TestSnapshotReadAtRejectsInvalidReferencesAndCorruption(t *testing.T) {
	block := []byte("123456789abc")
	reference := snapshotDetailBlockReference{length: uint32(len(block)), digest: sha256.Sum256(block)}
	reader := &snapshotCountingReadAt{reader: bytesReaderAt(block)}
	source, err := newSnapshotDetailReadAt(reader, int64(len(block)), []snapshotDetailBlockReference{reference})
	if err != nil {
		t.Fatal(err)
	}
	block[0] ^= 1
	if _, err := source.ReadBlock(0); err == nil {
		t.Fatal("corrupt detail accepted")
	}
	if _, err := source.ReadBlock(1); err == nil {
		t.Fatal("invalid block accepted")
	}
	for _, bad := range []snapshotDetailBlockReference{{offset: ^uint64(0), length: 9}, {length: ^uint32(0)}, {offset: 4, length: 9}} {
		if _, err := newSnapshotDetailReadAt(reader, int64(len(block)), []snapshotDetailBlockReference{bad}); err == nil {
			t.Fatal("invalid range accepted")
		}
	}
}

type bytesReaderAt []byte

func (reader bytesReaderAt) ReadAt(data []byte, offset int64) (int, error) {
	if offset < 0 || offset >= int64(len(reader)) {
		return 0, io.EOF
	}
	n := copy(data, reader[offset:])
	if n != len(data) {
		return n, io.EOF
	}
	return n, nil
}
