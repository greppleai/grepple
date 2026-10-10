package navigation

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

const maxSnapshotDetailBlockBytes = 128 << 20
const snapshotBlockHeaderBytes = 9

type snapshotBlockEncoder struct {
	buffer bytes.Buffer
	writer *flate.Writer
}

func newSnapshotBlockEncoder() (*snapshotBlockEncoder, error) {
	encoder := &snapshotBlockEncoder{}
	writer, err := flate.NewWriter(&encoder.buffer, flate.BestSpeed)
	if err != nil {
		return nil, err
	}
	encoder.writer = writer
	return encoder, nil
}

func (encoder *snapshotBlockEncoder) encode(data []byte) ([]byte, error) {
	if len(data) > maxSnapshotDetailBlockBytes {
		return nil, fmt.Errorf("navigation detail block exceeds size limit")
	}
	payload := data
	flag := byte(0)
	// Small blocks already use native interning; avoid decompression overhead.
	if len(data) > 4096 {
		encoder.buffer.Reset()
		encoder.writer.Reset(&encoder.buffer)
		if _, err := encoder.writer.Write(data); err != nil {
			return nil, err
		}
		if err := encoder.writer.Close(); err != nil {
			return nil, err
		}
		if encoder.buffer.Len() < len(data) {
			payload = encoder.buffer.Bytes()
			flag = 1
		}
	}
	result := make([]byte, snapshotBlockHeaderBytes+len(payload))
	result[0] = flag
	binary.LittleEndian.PutUint32(result[1:5], uint32(len(data)))
	binary.LittleEndian.PutUint32(result[5:9], crc32.ChecksumIEEE(payload))
	copy(result[snapshotBlockHeaderBytes:], payload)
	return result, nil
}

func decodeSnapshotBlock(block []byte) ([]byte, error) {
	if len(block) < snapshotBlockHeaderBytes {
		return nil, fmt.Errorf("truncated navigation detail block")
	}
	size := binary.LittleEndian.Uint32(block[1:5])
	if size > maxSnapshotDetailBlockBytes {
		return nil, fmt.Errorf("navigation detail block length exceeds limit")
	}
	payload := block[snapshotBlockHeaderBytes:]
	if crc32.ChecksumIEEE(payload) != binary.LittleEndian.Uint32(block[5:9]) {
		return nil, fmt.Errorf("navigation detail compressed checksum mismatch")
	}
	if block[0] == 0 {
		if uint32(len(payload)) != size {
			return nil, fmt.Errorf("navigation detail block length mismatch")
		}
		return payload, nil
	}
	if block[0] != 1 {
		return nil, fmt.Errorf("unsupported navigation detail block encoding")
	}
	reader := flate.NewReader(bytes.NewReader(payload))
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, int64(size)+1))
	if err != nil {
		return nil, err
	}
	if uint32(len(data)) != size {
		return nil, fmt.Errorf("navigation detail decompressed length mismatch")
	}
	return data, nil
}
