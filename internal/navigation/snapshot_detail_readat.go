package navigation

import (
	"crypto/sha256"
	"fmt"
	"io"
)

// Explicit offsets, lengths and digests are a reader abstraction, not a persisted
// ABI. No Go structs are mapped onto file bytes. Generation ownership is external.
type snapshotDetailBlockReference struct {
	offset uint64
	length uint32
	digest [sha256.Size]byte
}

type snapshotDetailReadAt struct {
	reader     io.ReaderAt
	references []snapshotDetailBlockReference
}

func newSnapshotDetailReadAt(reader io.ReaderAt, size int64, references []snapshotDetailBlockReference) (snapshotDetailBlockSource, error) {
	if reader == nil || size < 0 {
		return nil, fmt.Errorf("invalid navigation detail reader")
	}
	for _, reference := range references {
		if reference.length < snapshotBlockHeaderBytes || uint64(reference.length) > maxSnapshotDetailBlockBytes+snapshotBlockHeaderBytes {
			return nil, fmt.Errorf("navigation detail reference length outside bounds")
		}
		if reference.offset > uint64(size) || uint64(reference.length) > uint64(size)-reference.offset {
			return nil, fmt.Errorf("navigation detail reference outside generation")
		}
	}
	return &snapshotDetailReadAt{reader: reader, references: append([]snapshotDetailBlockReference(nil), references...)}, nil
}

func (source *snapshotDetailReadAt) ReadBlock(block uint32) ([]byte, error) {
	if uint64(block) >= uint64(len(source.references)) {
		return nil, fmt.Errorf("navigation detail block outside generation")
	}
	reference := source.references[block]
	data := make([]byte, int(reference.length))
	n, err := source.reader.ReadAt(data, int64(reference.offset))
	if err != nil {
		return nil, fmt.Errorf("read navigation detail block: %w", err)
	}
	if n != len(data) {
		return nil, io.ErrUnexpectedEOF
	}
	if sha256.Sum256(data) != reference.digest {
		return nil, fmt.Errorf("navigation detail generation checksum mismatch")
	}
	return data, nil
}
