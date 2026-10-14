package navigation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

var pagedNumericSections = []string{"calls", "targets", "groups", "declOffsets", "declRows", "outOffsets", "outRows", "inOffsets", "inRows", "dupOffsets", "dupRows", "usageOffsets", "usageRows", "accessOffsets", "accessRows"}
var pagedKeySections = []string{"lookup", "types", "imports", "exports", "fields", "files"}

type pagedKeyRows struct {
	Key  string
	Rows []uint32
}

func writePagedCore(file *os.File, s *querySnapshot, m *pagedManifest) error {
	calls := make([]uint32, 0, len(s.calls)*4)
	for _, c := range s.calls {
		flags := uint32(0)
		if c.direct {
			flags |= 1
		}
		if c.candidatesNonNil {
			flags |= 2
		}
		calls = append(calls, c.caller, c.targets, c.count, flags)
	}
	values := map[string][]uint32{"calls": calls, "targets": s.targets, "groups": s.groups, "outOffsets": s.outgoing.offsets, "outRows": s.outgoing.edges, "inOffsets": s.incoming.offsets, "inRows": s.incoming.edges}
	for _, entry := range []struct {
		name  string
		rows  map[uint32][]uint32
		count int
	}{{"decl", s.declarations, len(s.names)}, {"dup", s.duplicates, len(s.calls)}, {"usage", s.usages, len(s.names)}, {"access", s.accesses, len(s.names)}} {
		offsets, rows := flattenPagedRows(entry.rows, entry.count)
		values[entry.name+"Offsets"] = offsets
		values[entry.name+"Rows"] = rows
	}
	for _, name := range pagedNumericSections {
		if err := writePagedNumbers(file, m, name, values[name]); err != nil {
			return err
		}
	}
	if err := writePagedJSON(file, m, "names", len(s.names), func(start, end int) (any, string) { return s.names[start:end], "" }); err != nil {
		return err
	}
	lookup := map[string][]uint32{}
	for id, node := range s.ids {
		lookup[id] = []uint32{node}
	}
	files, err := pagedFileRows(s)
	if err != nil {
		return err
	}
	for _, entry := range []struct {
		name string
		rows map[string][]uint32
	}{{"lookup", lookup}, {"types", s.types}, {"imports", s.imports}, {"exports", s.exports}, {"fields", s.fields}, {"files", files}} {
		if err := writePagedKeys(file, m, entry.name, entry.rows); err != nil {
			return err
		}
	}
	return nil
}
func flattenPagedRows(mapping map[uint32][]uint32, count int) ([]uint32, []uint32) {
	offsets := make([]uint32, count+1)
	rows := []uint32{}
	for i := range count {
		rows = append(rows, mapping[uint32(i)]...)
		offsets[i+1] = uint32(len(rows))
	}
	return offsets, rows
}
func appendPagedBlock(file *os.File, section *pagedSection, data []byte) error {
	if len(data) == 0 || len(data) > maxPagedBlockBytes {
		return fmt.Errorf("paged block exceeds bound")
	}
	offset, err := file.Seek(0, 1)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	section.Blocks = append(section.Blocks, snapshotStoredReference{Offset: uint64(offset), Length: uint32(len(data)), Digest: sha256.Sum256(data)})
	return nil
}
func writePagedNumbers(file *os.File, m *pagedManifest, name string, values []uint32) error {
	section := pagedSection{Count: uint32(len(values))}
	for start := 0; start < len(values); start += pagedRows {
		end := min(start+pagedRows, len(values))
		data := make([]byte, (end-start)*4)
		for i, v := range values[start:end] {
			binary.LittleEndian.PutUint32(data[i*4:], v)
		}
		if err := appendPagedBlock(file, &section, data); err != nil {
			return err
		}
	}
	m.Sections[name] = section
	return nil
}
func writePagedJSON(file *os.File, m *pagedManifest, name string, count int, rows func(int, int) (any, string)) error {
	section := pagedSection{Count: uint32(count)}
	for start := 0; start < count; start += pagedLocatorRows {
		values, first := rows(start, min(start+pagedLocatorRows, count))
		data, err := json.Marshal(values)
		if err != nil {
			return err
		}
		if err := appendPagedBlock(file, &section, data); err != nil {
			return err
		}
		if name != "names" {
			section.First = append(section.First, first)
		}
	}
	m.Sections[name] = section
	return nil
}
func writePagedKeys(file *os.File, m *pagedManifest, name string, mapping map[string][]uint32) error {
	keys := make([]string, 0, len(mapping))
	for key := range mapping {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return writePagedJSON(file, m, name, len(keys), func(start, end int) (any, string) {
		rows := make([]pagedKeyRows, 0, end-start)
		for _, key := range keys[start:end] {
			rows = append(rows, pagedKeyRows{Key: key, Rows: mapping[key]})
		}
		return rows, keys[start]
	})
}
func validatePagedSections(sections map[string]pagedSection, size uint64) error {
	if len(sections) != len(pagedNumericSections)+len(pagedKeySections)+1 {
		return fmt.Errorf("invalid paged section directory")
	}
	position := uint64(0)
	for _, name := range append(append(append([]string{}, pagedNumericSections...), "names"), pagedKeySections...) {
		section, ok := sections[name]
		width := uint64(pagedRows)
		if name == "names" || len(section.First) > 0 {
			width = pagedLocatorRows
		}
		if !ok || uint64(len(section.Blocks)) != (uint64(section.Count)+width-1)/width {
			return fmt.Errorf("invalid paged section count")
		}
		next, err := validatePagedRanges(section, position, size)
		if err != nil {
			return err
		}
		position = next
		if err := validatePagedFirst(name, section); err != nil {
			return err
		}
	}
	if position != size {
		return fmt.Errorf("trailing paged core bytes")
	}
	return validatePagedCounts(sections)
}
func validatePagedCounts(sections map[string]pagedSection) error {
	nodes := sections["names"].Count
	calls := sections["calls"].Count / 4
	if sections["calls"].Count%4 != 0 || sections["groups"].Count != calls {
		return fmt.Errorf("invalid paged call count")
	}
	for _, name := range []string{"declOffsets", "outOffsets", "inOffsets", "usageOffsets", "accessOffsets"} {
		if uint64(sections[name].Count) != uint64(nodes)+1 {
			return fmt.Errorf("invalid paged node offsets")
		}
	}
	if uint64(sections["dupOffsets"].Count) != uint64(calls)+1 {
		return fmt.Errorf("invalid paged duplicate offsets")
	}
	return nil
}
func validatePagedFirst(name string, section pagedSection) error {
	keyed := false
	for _, key := range pagedKeySections {
		keyed = keyed || name == key
	}
	if keyed {
		if len(section.First) != len(section.Blocks) {
			return fmt.Errorf("invalid paged key directory")
		}
		for i := 1; i < len(section.First); i++ {
			if section.First[i] <= section.First[i-1] {
				return fmt.Errorf("unordered paged keys")
			}
		}
	} else if len(section.First) != 0 {
		return fmt.Errorf("unexpected paged keys")
	}
	return nil
}
