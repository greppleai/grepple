package navigation

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPHPComposerManifestParticipatesInRepositoryContext(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "composer.json")
	if err := os.WriteFile(manifest, []byte(`{"autoload":{"psr-4":{"App\\":"src/"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []string{filepath.Join(root, "src", "Widget.php"), filepath.Join(root, "src", "Another.php")}
	if got := RepositoryContextFiles(files); !reflect.DeepEqual(got, []string{manifest}) {
		t.Fatalf("repository context = %v", got)
	}
}
