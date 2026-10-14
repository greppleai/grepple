package navigation

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

func TestSnapshotBlockRoundtripAndBounds(t *testing.T) {
	encoder, err := newSnapshotBlockEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.writer.Close()
	for _, data := range [][]byte{nil, []byte("small"), bytes.Repeat([]byte("large repeated names and metadata"), 4096)} {
		block, err := encoder.encode(data)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeSnapshotBlock(block)
		if err != nil || !bytes.Equal(decoded, data) {
			t.Fatalf("block roundtrip: %v", err)
		}
		if len(data) > 4096 && block[0] != 1 {
			t.Fatal("large repeated detail did not compress")
		}
	}
}

func TestSnapshotBlockRejectsMalformedLengthsAndData(t *testing.T) {
	encoder, err := newSnapshotBlockEncoder()
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.writer.Close()
	good, err := encoder.encode(bytes.Repeat([]byte("repeat"), 1024))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"short", "limit", "length", "checksum", "encoding", "compressed"} {
		t.Run(kind, func(t *testing.T) { assertMalformedSnapshotBlock(t, good, kind) })
	}
}

func assertMalformedSnapshotBlock(t *testing.T, good []byte, kind string) {
	t.Helper()
	bad := append([]byte(nil), good...)
	switch kind {
	case "short":
		bad = bad[:4]
	case "limit":
		binary.LittleEndian.PutUint32(bad[1:5], ^uint32(0))
	case "length":
		binary.LittleEndian.PutUint32(bad[1:5], 1)
	case "checksum":
		bad[len(bad)-1] ^= 1
	case "encoding":
		bad[0] = 2
	case "compressed":
		bad[snapshotBlockHeaderBytes] = 0xff
		binary.LittleEndian.PutUint32(bad[5:9], crc32.ChecksumIEEE(bad[snapshotBlockHeaderBytes:]))
	}
	if _, err := decodeSnapshotBlock(bad); err == nil {
		t.Fatal("accepted malformed block")
	}
}
