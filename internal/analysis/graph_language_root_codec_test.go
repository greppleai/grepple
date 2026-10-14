package analysis

import (
	"reflect"
	"testing"
)

func TestLanguageRootCatalogRoundTripAndBounds(t *testing.T) {
	manifest := languageManifest{Roots: []languageRootRow{{ID: "id", Name: "ñame", Source: 0, Start: 1, End: 30}, {ID: "id", Name: "ñame", Source: 0, Start: 2, End: 20}}, RootSources: []languageRootSource{{Path: "test.py", Language: "python", Package: "package", ModuleID: "module"}}}
	encoded, err := encodeLanguageRootCatalog(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded := languageManifest{RootCatalog: encoded}
	if err := decodeLanguageRootCatalog(&decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.Roots, manifest.Roots) || !reflect.DeepEqual(decoded.RootSources, manifest.RootSources) {
		t.Fatal("root catalog facts changed")
	}
	for end := 0; end < len(encoded); end++ {
		truncated := languageManifest{RootCatalog: encoded[:end]}
		if decodeLanguageRootCatalog(&truncated) == nil {
			t.Fatalf("truncation accepted at %d", end)
		}
	}
	trailing := languageManifest{RootCatalog: append(encoded, 0)}
	if decodeLanguageRootCatalog(&trailing) == nil {
		t.Fatal("trailing catalog bytes accepted")
	}
}
