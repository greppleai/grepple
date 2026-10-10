package navigation

import (
	"reflect"
	"testing"
)

func TestPagedManifestBinaryBounds(t *testing.T) {
	manifest := pagedManifest{Key: "key", Metadata: []byte("metadata"), Roots: []string{"root"}, Sections: map[string]pagedSection{}, Tables: make([]snapshotStoredTable, 8)}
	for _, name := range pagedSectionOrder() {
		manifest.Sections[name] = pagedSection{First: []string{}, Blocks: []snapshotStoredReference{}}
	}
	for i := range manifest.Tables {
		manifest.Tables[i].References = []snapshotStoredReference{}
	}
	encoded, err := encodePagedManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodePagedManifest(encoded)
	if err != nil || !reflect.DeepEqual(manifest, decoded) {
		t.Fatalf("binary directory changed: %v", err)
	}
	for end := 0; end < len(encoded); end++ {
		if _, err := decodePagedManifest(encoded[:end]); err == nil {
			t.Fatalf("truncation accepted at %d", end)
		}
	}
	if _, err := decodePagedManifest(append(encoded, 0)); err == nil {
		t.Fatal("trailing directory accepted")
	}
}
