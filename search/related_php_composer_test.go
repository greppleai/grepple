package search

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelatedPHPExposesComposerImportForQualification(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"composer.json": `{"require":{"acme/widgets":"^2"}}`,
		"composer.lock": `{"packages":[{"name":"acme/widgets","version":"2.4.0","autoload":{"psr-4":{"Acme\\":"src/"}}}]}`,
		"main.php": `<?php
use Acme\Widget;
function execute(Widget $widget): void { $widget->send(); // needle
}
`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	matches, err := Files(Params{Query: "needle", Related: true}, []string{filepath.Join(root, "main.php")})
	if err != nil || len(matches) != 1 {
		t.Fatalf("matches=%+v err=%v", matches, err)
	}
	for _, point := range matches[0].Related {
		if point.External != nil && point.External.ImportPath == `Acme\Widget` && point.Confidence == "dependency-unresolved" {
			return
		}
	}
	t.Fatalf("missing Composer external reference: %+v", matches[0].Related)
}
