package navigation

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sort"
)

// The core codec stores explicit little-endian scalar fields, not native Go
// struct layouts. Publication/identity and detail indexes belong to the owner.
const snapshotCoreMagic = "GNCOR001"
const maxSnapshotCoreBytes = int64(512 << 20)

type snapshotCoreEncoder struct {
	writer io.Writer
	err    error
}

func (encoder *snapshotCoreEncoder) number(value uint32) {
	if encoder.err != nil {
		return
	}
	var data [4]byte
	binary.LittleEndian.PutUint32(data[:], value)
	_, encoder.err = encoder.writer.Write(data[:])
}
func (encoder *snapshotCoreEncoder) count(length int) {
	if uint64(length) > math.MaxUint32 {
		encoder.err = fmt.Errorf("snapshot core capacity exceeded")
		return
	}
	encoder.number(uint32(length))
}
func (encoder *snapshotCoreEncoder) text(value string) {
	encoder.count(len(value))
	if encoder.err == nil {
		_, encoder.err = io.WriteString(encoder.writer, value)
	}
}
func (encoder *snapshotCoreEncoder) strings(values []string) {
	encoder.count(len(values))
	for _, value := range values {
		encoder.text(value)
	}
}
func (encoder *snapshotCoreEncoder) numbers(values []uint32) {
	encoder.count(len(values))
	for _, value := range values {
		encoder.number(value)
	}
}
func (encoder *snapshotCoreEncoder) nodeRows(values map[uint32][]uint32) {
	keys := make([]uint32, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	encoder.count(len(keys))
	for _, key := range keys {
		encoder.number(key)
		encoder.numbers(values[key])
	}
}
func (encoder *snapshotCoreEncoder) textRows(values map[string][]uint32) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	encoder.count(len(keys))
	for _, key := range keys {
		encoder.text(key)
		encoder.numbers(values[key])
	}
}
func writeSnapshotCore(writer io.Writer, snapshot *querySnapshot) error {
	buffer := bufio.NewWriter(writer)
	if _, err := io.WriteString(buffer, snapshotCoreMagic); err != nil {
		return err
	}
	encoder := &snapshotCoreEncoder{writer: buffer}
	encoder.strings(snapshot.roots)
	encoder.strings(snapshot.names)
	encoder.nodeRows(snapshot.declarations)
	encoder.count(len(snapshot.calls))
	for _, call := range snapshot.calls {
		encoder.number(call.caller)
		encoder.number(call.targets)
		encoder.number(call.count)
		flags := uint32(0)
		if call.direct {
			flags |= 1
		}
		if call.candidatesNonNil {
			flags |= 2
		}
		encoder.number(flags)
	}
	for _, values := range [][]uint32{snapshot.targets, snapshot.outgoing.offsets, snapshot.outgoing.edges, snapshot.incoming.offsets, snapshot.incoming.edges, snapshot.groups} {
		encoder.numbers(values)
	}
	encoder.nodeRows(snapshot.duplicates)
	for _, values := range []map[string][]uint32{snapshot.types, snapshot.imports, snapshot.exports, snapshot.fields} {
		encoder.textRows(values)
	}
	encoder.nodeRows(snapshot.usages)
	encoder.nodeRows(snapshot.accesses)
	if encoder.err != nil {
		return encoder.err
	}
	return buffer.Flush()
}

type snapshotCoreDecoder struct {
	reader           *bufio.Reader
	remaining        int64
	allocationBudget int64
	err              error
}

func (decoder *snapshotCoreDecoder) read(data []byte) {
	if decoder.err != nil {
		return
	}
	if int64(len(data)) > decoder.remaining {
		decoder.err = io.ErrUnexpectedEOF
		return
	}
	_, decoder.err = io.ReadFull(decoder.reader, data)
	decoder.remaining -= int64(len(data))
}
func (decoder *snapshotCoreDecoder) number() uint32 {
	var data [4]byte
	decoder.read(data[:])
	return binary.LittleEndian.Uint32(data[:])
}
func (decoder *snapshotCoreDecoder) count(minimum, allocation int64) int {
	count := decoder.number()
	if decoder.err != nil {
		return 0
	}
	if uint64(count) > uint64(decoder.remaining/minimum) {
		decoder.err = fmt.Errorf("snapshot core count outside section")
		return 0
	}
	required := int64(count) * allocation
	if required > decoder.allocationBudget {
		decoder.err = fmt.Errorf("snapshot core allocation budget exceeded")
		return 0
	}
	decoder.allocationBudget -= required
	return int(count)
}
func (decoder *snapshotCoreDecoder) text() string {
	length := decoder.count(1, 2)
	if decoder.err != nil {
		return ""
	}
	data := make([]byte, length)
	decoder.read(data)
	return string(data)
}
func (decoder *snapshotCoreDecoder) strings() []string {
	values := make([]string, decoder.count(4, 24))
	for i := range values {
		values[i] = decoder.text()
	}
	return values
}
func (decoder *snapshotCoreDecoder) numbers() []uint32 {
	values := make([]uint32, decoder.count(4, 4))
	for i := range values {
		values[i] = decoder.number()
	}
	return values
}
func (decoder *snapshotCoreDecoder) nodeRows() map[uint32][]uint32 {
	count := decoder.count(8, 64)
	values := make(map[uint32][]uint32, count)
	for i := 0; i < count && decoder.err == nil; i++ {
		key := decoder.number()
		if _, exists := values[key]; exists {
			decoder.err = fmt.Errorf("duplicate snapshot node key")
			break
		}
		values[key] = decoder.numbers()
	}
	return values
}
func (decoder *snapshotCoreDecoder) textRows() map[string][]uint32 {
	count := decoder.count(8, 80)
	values := make(map[string][]uint32, count)
	for i := 0; i < count && decoder.err == nil; i++ {
		key := decoder.text()
		if _, exists := values[key]; exists {
			decoder.err = fmt.Errorf("duplicate snapshot text key")
			break
		}
		values[key] = decoder.numbers()
	}
	return values
}
func readSnapshotCore(reader io.Reader, size int64) (*querySnapshot, error) {
	if size < int64(len(snapshotCoreMagic)) || size > maxSnapshotCoreBytes {
		return nil, fmt.Errorf("snapshot core section size outside bounds")
	}
	decoder := &snapshotCoreDecoder{reader: bufio.NewReader(reader), remaining: size, allocationBudget: 256 << 20}
	magic := make([]byte, len(snapshotCoreMagic))
	decoder.read(magic)
	if string(magic) != snapshotCoreMagic {
		return nil, fmt.Errorf("incompatible snapshot core schema")
	}
	snapshot := &querySnapshot{roots: decoder.strings(), names: decoder.strings()}
	snapshot.ids = make(map[string]uint32, len(snapshot.names))
	for row, name := range snapshot.names {
		if _, exists := snapshot.ids[name]; exists {
			return nil, fmt.Errorf("duplicate snapshot identity")
		}
		snapshot.ids[name] = uint32(row)
	}
	snapshot.declarations = decoder.nodeRows()
	snapshot.calls = make([]snapshotCall, decoder.count(16, 24))
	for row := range snapshot.calls {
		call := snapshotCall{caller: decoder.number(), targets: decoder.number(), count: decoder.number()}
		flags := decoder.number()
		if flags > 3 {
			return nil, fmt.Errorf("unsupported snapshot call flags")
		}
		call.direct = flags&1 != 0
		call.candidatesNonNil = flags&2 != 0
		snapshot.calls[row] = call
	}
	snapshot.targets = decoder.numbers()
	snapshot.outgoing = snapshotCSR{offsets: decoder.numbers(), edges: decoder.numbers()}
	snapshot.incoming = snapshotCSR{offsets: decoder.numbers(), edges: decoder.numbers()}
	snapshot.groups = decoder.numbers()
	snapshot.duplicates = decoder.nodeRows()
	snapshot.types = decoder.textRows()
	snapshot.imports = decoder.textRows()
	snapshot.exports = decoder.textRows()
	snapshot.fields = decoder.textRows()
	snapshot.usages = decoder.nodeRows()
	snapshot.accesses = decoder.nodeRows()
	if decoder.err != nil {
		return nil, decoder.err
	}
	if decoder.remaining != 0 {
		return nil, fmt.Errorf("unexpected snapshot core trailing bytes")
	}
	if err := validateSnapshotCore(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}
