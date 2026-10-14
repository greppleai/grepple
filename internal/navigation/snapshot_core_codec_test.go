package navigation

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

func TestSnapshotCoreCodecDeterministicQueryParity(t *testing.T) {
	graph := navigationQueryFixture()
	prepared, err := newQuerySnapshot(graph)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := prepared.(*querySnapshot)
	var first, second bytes.Buffer
	if err := writeSnapshotCore(&first, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshotCore(&second, snapshot); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatal("non-deterministic core encoding")
	}
	decoded, err := readSnapshotCore(bytes.NewReader(first.Bytes()), int64(first.Len()))
	if err != nil {
		t.Fatal(err)
	}
	decoded.details = snapshot.details
	for _, direction := range []NavigationQueryDirection{NavigationQueryCallers, NavigationQueryCallees, NavigationQueryDependencies, NavigationQueryDependents, NavigationQueryImpact} {
		for depth := 1; depth <= 3; depth++ {
			assertSnapshotQueryParity(t, graph, decoded, []string{"root"}, direction, depth)
		}
	}
	if !reflect.DeepEqual(snapshot.names, decoded.names) {
		t.Fatal("stable identities changed")
	}
}

func TestSnapshotCoreCodecRejectsTruncationAndLengths(t *testing.T) {
	prepared, err := newQuerySnapshot(navigationQueryFixture())
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	if err := writeSnapshotCore(&buffer, prepared.(*querySnapshot)); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	for _, length := range []int{0, 7, len(data) - 1} {
		if _, err := readSnapshotCore(bytes.NewReader(data[:length]), int64(length)); err == nil {
			t.Fatal("truncated core accepted")
		}
	}
	broken := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(broken[len(snapshotCoreMagic):], ^uint32(0))
	if _, err := readSnapshotCore(bytes.NewReader(broken), int64(len(broken))); err == nil {
		t.Fatal("invalid count accepted")
	}
}
