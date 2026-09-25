package navigation

import (
	"path/filepath"
	"testing"
)

func TestQualifyExternalDependenciesFromComposerLock(t *testing.T) {
	root := t.TempDir()
	mustWriteDependencyFile(t, filepath.Join(root, "composer.json"), `{"require":{"acme/widgets":"^2","acme/ui":"^1"}}`)
	mustWriteDependencyFile(t, filepath.Join(root, "composer.lock"), `{"packages":[{"name":"acme/widgets","version":"2.4.0","autoload":{"psr-4":{"Acme\\":"src/"}}},{"name":"acme/ui","version":"1.2.0","autoload":{"psr-4":{"Acme\\UI\\":"src/"}}}]}`)
	mustWriteDependencyFile(t, filepath.Join(root, "main.php"), "<?php\n")
	results := []externalDependencyTestResult{{Path: "main.php", Language: "php", Related: []RelatedSymbol{
		{External: &ExternalReference{ID: "ui", Language: "php", ImportPath: "Acme\\UI\\Button", Symbol: "Button", Kind: "type"}},
		{External: &ExternalReference{ID: "other", Language: "php", ImportPath: "Other\\Unknown", Symbol: "Unknown", Kind: "type"}},
	}}}
	if err := QualifyExternalDependencies(results, root); err != nil {
		t.Fatal(err)
	}
	ui := results[0].Related[0].External
	if ui.Module != "acme/ui" || ui.Version != "1.2.0" || ui.Package != "Acme\\UI\\Button" {
		t.Fatalf("qualified Composer reference = %+v", ui)
	}
	if other := results[0].Related[1].External; other.Module != "" || other.Version != "" {
		t.Fatalf("unmatched reference = %+v", other)
	}
	if references := ExternalDependencyReferences(results); len(references) != 1 {
		t.Fatalf("artifact references = %+v", references)
	}
}
