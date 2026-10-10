package navigation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
)

const pagedCacheBytes = 4 << 20
const pagedReadBudget = 128 << 20
const pagedSelectionLimit = 1 << 20

// Scratch belongs to a single query; there is no unbounded shared page cache.
type pagedQueryReader struct {
	s       *pagedStoredSnapshot
	cache   *pagedCache[[]byte]
	read    int
	decoded *pagedCache[any]
	err     error
}
type pagedPage struct {
	section string
	block   uint32
}

func newPagedQueryReader(s *pagedStoredSnapshot) *pagedQueryReader {
	return &pagedQueryReader{s: s, cache: newPagedCache[[]byte](pagedCacheBytes), decoded: newPagedCache[any](pagedCacheBytes)}
}
func (r *pagedQueryReader) block(name string, index uint32) []byte {
	if r.err != nil {
		return nil
	}
	key := pagedPage{name, index}
	if data, ok := r.cache.get(key); ok {
		return data
	}
	section, ok := r.s.manifest.Sections[name]
	if !ok || uint64(index) >= uint64(len(section.Blocks)) {
		r.err = fmt.Errorf("paged block outside section")
		return nil
	}
	ref := section.Blocks[index]
	if r.read+int(ref.Length) > pagedReadBudget {
		r.err = ErrSnapshotReadBudget
		return nil
	}
	data := make([]byte, ref.Length)
	r.read += len(data)
	count, err := r.s.file.ReadAt(data, r.s.offset+int64(ref.Offset))
	r.s.coreBytes.Add(uint64(count))
	r.s.corePages.Add(1)
	if err != nil {
		r.err = err
		return nil
	}
	if sha256.Sum256(data) != ref.Digest {
		r.err = fmt.Errorf("paged block checksum mismatch")
		return nil
	}
	r.cache.put(key, data, len(data))
	return data
}
func (r *pagedQueryReader) numbers(name string, start, end uint32) []uint32 {
	count := r.s.manifest.Sections[name].Count
	if start > end || end > count {
		r.err = fmt.Errorf("paged numeric range outside section")
		return nil
	}
	if uint64(end-start) > pagedSelectionLimit {
		r.err = fmt.Errorf("%w: paged numeric range limit", ErrSnapshotBudget)
		return nil
	}
	values := make([]uint32, 0, end-start)
	for position := start; position < end; {
		block := position / pagedRows
		data := r.block(name, block)
		expected := int(min(uint32(pagedRows), count-block*pagedRows)) * 4
		if len(data) != expected {
			if r.err == nil {
				r.err = fmt.Errorf("paged numeric block size mismatch")
			}
			return nil
		}
		stop := min(end, (block+1)*pagedRows)
		for i := position; i < stop; i++ {
			values = append(values, binary.LittleEndian.Uint32(data[(i%pagedRows)*4:]))
		}
		position = stop
	}
	return values
}
func (r *pagedQueryReader) number(name string, index uint32) uint32 {
	values := r.numbers(name, index, index+1)
	if len(values) != 1 {
		return 0
	}
	return values[0]
}
func (r *pagedQueryReader) rows(prefix string, node uint32) []uint32 {
	offsets := r.numbers(prefix+"Offsets", node, node+2)
	if len(offsets) != 2 {
		return nil
	}
	return r.numbers(prefix+"Rows", offsets[0], offsets[1])
}
func (r *pagedQueryReader) name(node uint32) string {
	section := r.s.manifest.Sections["names"]
	if node >= section.Count {
		r.err = fmt.Errorf("paged name outside section")
		return ""
	}
	block := node / pagedLocatorRows
	key := pagedPage{"names", block}
	cached, _ := r.decoded.get(key)
	names, ok := cached.([]string)
	if !ok {
		data := r.block("names", block)
		if r.err == nil {
			r.err = json.Unmarshal(data, &names)
		}
		if r.err == nil {
			r.remember(key, names, len(data)*3)
		}
	}
	if r.err != nil {
		return ""
	}
	if len(names) != int(min(uint32(pagedLocatorRows), section.Count-block*pagedLocatorRows)) {
		r.err = fmt.Errorf("paged name count mismatch")
		return ""
	}
	return names[node%pagedLocatorRows]
}
func (r *pagedQueryReader) keyRows(section, key string) []uint32 {
	directory := r.s.manifest.Sections[section]
	index := sort.Search(len(directory.First), func(i int) bool { return directory.First[i] > key }) - 1
	if index < 0 {
		return nil
	}
	page := pagedPage{section, uint32(index)}
	cached, _ := r.decoded.get(page)
	rows, ok := cached.([]pagedKeyRows)
	if !ok {
		data := r.block(section, uint32(index))
		if r.err == nil {
			r.err = json.Unmarshal(data, &rows)
		}
		if r.err == nil {
			r.remember(page, rows, len(data)*4)
		}
	}
	if r.err != nil {
		return nil
	}
	expected := min(uint32(pagedLocatorRows), directory.Count-uint32(index)*pagedLocatorRows)
	if len(rows) != int(expected) || rows[0].Key != directory.First[index] {
		r.err = fmt.Errorf("paged key count mismatch")
		return nil
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].Key <= rows[i-1].Key {
			r.err = fmt.Errorf("unordered paged key block")
			return nil
		}
	}
	found := sort.Search(len(rows), func(i int) bool { return rows[i].Key >= key })
	if found < len(rows) && rows[found].Key == key {
		return r.checkedRows(rows[found].Rows)
	}
	return nil
}
func (r *pagedQueryReader) checkedRows(rows []uint32) []uint32 {
	if len(rows) > pagedSelectionLimit {
		r.err = fmt.Errorf("%w: paged keyed row limit", ErrSnapshotBudget)
		return nil
	}
	return rows
}
func (r *pagedQueryReader) remember(key pagedPage, value any, size int) {
	r.decoded.put(key, value, size)
}

func (r *pagedQueryReader) call(row uint32) (snapshotCall, []uint32) {
	if row >= r.s.manifest.Sections["calls"].Count/4 {
		r.err = fmt.Errorf("paged call outside section")
		return snapshotCall{}, nil
	}
	values := r.numbers("calls", row*4, row*4+4)
	if len(values) != 4 {
		return snapshotCall{}, nil
	}
	if values[3] > 3 || values[0] >= r.s.manifest.Sections["names"].Count {
		r.err = fmt.Errorf("invalid paged call record")
		return snapshotCall{}, nil
	}
	c := snapshotCall{caller: values[0], targets: values[1], count: values[2], direct: values[3]&1 != 0, candidatesNonNil: values[3]&2 != 0}
	if uint64(c.targets)+uint64(c.count) > uint64(r.s.manifest.Sections["targets"].Count) || c.direct && c.count == 0 {
		r.err = fmt.Errorf("invalid paged targets")
		return c, nil
	}
	targets := r.numbers("targets", c.targets, c.targets+c.count)
	for _, node := range targets {
		if node >= r.s.manifest.Sections["names"].Count {
			r.err = fmt.Errorf("invalid paged target node")
		}
	}
	return c, targets
}
