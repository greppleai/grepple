package navigation

import (
	"bufio"
	"bytes"
	"fmt"
)

const pagedManifestMagic = "GNPDIR01"

func pagedSectionOrder() []string {
	return append(append(append([]string{}, pagedNumericSections...), "names"), pagedKeySections...)
}
func encodePagedManifest(manifest pagedManifest) ([]byte, error) {
	var output bytes.Buffer
	encoder := &snapshotCoreEncoder{writer: &output}
	output.WriteString(pagedManifestMagic)
	encoder.text(manifest.Key)
	encoder.text(string(manifest.Metadata))
	encoder.strings(manifest.Roots)
	for _, name := range pagedSectionOrder() {
		section := manifest.Sections[name]
		encoder.number(section.Count)
		encoder.strings(section.First)
		encodePagedReferences(encoder, section.Blocks)
	}
	encoder.count(len(manifest.Tables))
	for _, table := range manifest.Tables {
		encoder.number(table.Count)
		flags := uint32(0)
		if table.NonNil {
			flags = 1
		}
		encoder.number(flags)
		encodePagedReferences(encoder, table.References)
	}
	return output.Bytes(), encoder.err
}
func encodePagedReferences(encoder *snapshotCoreEncoder, refs []snapshotStoredReference) {
	encoder.count(len(refs))
	for _, ref := range refs {
		encoder.number(uint32(ref.Offset))
		encoder.number(uint32(ref.Offset >> 32))
		encoder.number(ref.Length)
		if encoder.err == nil {
			_, encoder.err = encoder.writer.Write(ref.Digest[:])
		}
	}
}
func decodePagedManifest(data []byte) (pagedManifest, error) {
	decoder := &snapshotCoreDecoder{reader: bufio.NewReader(bytes.NewReader(data)), remaining: int64(len(data)), allocationBudget: 64 << 20}
	var magic [8]byte
	decoder.read(magic[:])
	if string(magic[:]) != pagedManifestMagic {
		return pagedManifest{}, fmt.Errorf("invalid paged directory schema")
	}
	manifest := pagedManifest{Key: decoder.text(), Metadata: []byte(decoder.text()), Roots: decoder.strings(), Sections: map[string]pagedSection{}}
	for _, name := range pagedSectionOrder() {
		manifest.Sections[name] = pagedSection{Count: decoder.number(), First: decoder.strings(), Blocks: decodePagedReferences(decoder)}
	}
	count := decoder.number()
	if count != 8 {
		return pagedManifest{}, fmt.Errorf("invalid paged detail table count")
	}
	manifest.Tables = make([]snapshotStoredTable, 8)
	for i := range manifest.Tables {
		table := &manifest.Tables[i]
		table.Count = decoder.number()
		flags := decoder.number()
		if flags > 1 {
			return pagedManifest{}, fmt.Errorf("invalid paged detail flags")
		}
		table.NonNil = flags == 1
		table.References = decodePagedReferences(decoder)
	}
	if decoder.err != nil {
		return pagedManifest{}, decoder.err
	}
	if decoder.remaining != 0 {
		return pagedManifest{}, fmt.Errorf("trailing paged directory bytes")
	}
	return manifest, nil
}
func decodePagedReferences(decoder *snapshotCoreDecoder) []snapshotStoredReference {
	refs := make([]snapshotStoredReference, decoder.count(44, 48))
	for i := range refs {
		low, high := decoder.number(), decoder.number()
		refs[i].Offset = uint64(low) | uint64(high)<<32
		refs[i].Length = decoder.number()
		decoder.read(refs[i].Digest[:])
	}
	return refs
}
