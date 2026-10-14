package analysis

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const languageRootCatalogMagic = "GNROOT01"

func encodeLanguageRootCatalog(manifest languageManifest) ([]byte, error) {
	var data bytes.Buffer
	data.WriteString(languageRootCatalogMagic)
	write := func(value uint32) { _ = binary.Write(&data, binary.LittleEndian, value) }
	text := func(value string) { write(uint32(len(value))); data.WriteString(value) }
	write(uint32(len(manifest.RootSources)))
	for _, s := range manifest.RootSources {
		for _, value := range []string{s.Path, s.Language, s.Package, s.PackageID, s.ModuleID} {
			text(value)
		}
	}
	write(uint32(len(manifest.Roots)))
	for _, root := range manifest.Roots {
		text(root.ID)
		text(root.Name)
		write(root.Source)
		_ = binary.Write(&data, binary.LittleEndian, int64(root.Start))
		_ = binary.Write(&data, binary.LittleEndian, int64(root.End))
	}
	if data.Len() > maxLanguageManifestBytes/2 {
		return nil, fmt.Errorf("root catalog exceeds bound")
	}
	return data.Bytes(), nil
}
func decodeLanguageRootCatalog(manifest *languageManifest) error {
	data := manifest.RootCatalog
	if len(data) < 8 || string(data[:8]) != languageRootCatalogMagic {
		return fmt.Errorf("invalid root catalog schema")
	}
	decoder := &languageRootDecoder{reader: bytes.NewReader(data[8:]), data: data[8:], budget: 64 << 20}
	manifest.RootSources = decoder.sources()
	manifest.Roots = decoder.rows()
	if decoder.err != nil {
		return decoder.err
	}
	if decoder.reader.Len() != 0 {
		return fmt.Errorf("trailing root catalog bytes")
	}
	manifest.RootCatalog = nil
	return nil
}

type languageRootDecoder struct {
	reader *bytes.Reader
	data   []byte
	err    error
	budget int64
}

func (d *languageRootDecoder) number() uint32 {
	if d.err != nil {
		return 0
	}
	var data [4]byte
	if d.reader.Len() < 4 {
		d.err = io.ErrUnexpectedEOF
		return 0
	}
	_, d.err = d.reader.Read(data[:])
	return binary.LittleEndian.Uint32(data[:])
}
func (d *languageRootDecoder) count(minimum, width int64) uint32 {
	count := d.number()
	if d.err != nil {
		return 0
	}
	if uint64(count) > uint64(int64(d.reader.Len())/minimum) || int64(count)*width > d.budget {
		d.err = fmt.Errorf("root catalog count exceeds bound")
		return 0
	}
	d.budget -= int64(count) * width
	return count
}
func (d *languageRootDecoder) text() string {
	length := d.count(1, 2)
	if d.err != nil {
		return ""
	}
	offset := int(d.reader.Size()) - d.reader.Len()
	value := string(d.data[offset : offset+int(length)])
	_, d.err = d.reader.Seek(int64(length), io.SeekCurrent)
	return value
}
func (d *languageRootDecoder) sources() []languageRootSource {
	result := make([]languageRootSource, d.count(20, 80))
	for i := range result {
		result[i] = languageRootSource{Path: d.text(), Language: d.text(), Package: d.text(), PackageID: d.text(), ModuleID: d.text()}
	}
	return result
}
func (d *languageRootDecoder) rows() []languageRootRow {
	result := make([]languageRootRow, d.count(28, 64))
	for i := range result {
		root := &result[i]
		root.ID = d.text()
		root.Name = d.text()
		root.Source = d.number()
		start, end := int64(uint64(d.number())|uint64(d.number())<<32), int64(uint64(d.number())|uint64(d.number())<<32)
		root.Start, root.End = int(start), int(end)
		if int64(root.Start) != start || int64(root.End) != end {
			d.err = fmt.Errorf("root line overflow")
		}
	}
	return result
}
