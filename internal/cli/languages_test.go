package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/greppleai/grepple/api"
)

func TestLanguagesJSONReportsRegisteredFeatureParity(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"languages", "--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var capabilities []api.LanguageCapabilities
	if err := json.Unmarshal([]byte(output), &capabilities); err != nil {
		t.Fatal(err)
	}
	byLanguage := make(map[string]api.LanguageCapabilities, len(capabilities))
	for _, capability := range capabilities {
		byLanguage[capability.Language] = capability
	}
	goLanguage := byLanguage["go"]
	if goLanguage.GritQL != api.FeatureProduction || goLanguage.FocusedFlow != api.FeatureProduction || goLanguage.DirectoryArchitecture != api.FeatureProduction || goLanguage.ImportRelations != api.FeatureProduction || goLanguage.Entrypoints != api.FeatureProduction {
		t.Fatalf("go capabilities=%#v", goLanguage)
	}
	tsx := byLanguage["tsx"]
	if tsx.GritQL != api.FeatureProduction || tsx.FocusedStructure != api.FeatureProduction || tsx.DirectoryArchitecture != api.FeatureProduction || tsx.ImportRelations != api.FeatureProduction || tsx.Entrypoints != api.FeatureUnsupported {
		t.Fatalf("tsx capabilities=%#v", tsx)
	}
	javascript := byLanguage["javascript"]
	if javascript.Navigation != api.FeatureProduction || javascript.GritQL != api.FeatureProduction || javascript.FocusedStructure != api.FeatureProduction || javascript.FocusedFlow != api.FeatureProduction || javascript.ImportRelations != api.FeatureProduction {
		t.Fatalf("javascript capabilities=%#v", javascript)
	}
	assertImportRelationCapabilities(t, byLanguage, "java", "kotlin", "csharp")
	assertRustLanguageCapabilities(t, byLanguage["rust"])
	markdown := byLanguage["markdown"]
	if markdown.StructuralGrep != api.FeatureSpecialized || markdown.Outline != api.FeatureSpecialized || markdown.Navigation != api.FeatureUnsupported || markdown.DirectoryArchitecture != api.FeatureUnsupported || markdown.ImportRelations != api.FeatureUnsupported {
		t.Fatalf("markdown capabilities=%#v", markdown)
	}
	if !strings.Contains(output, `"language": "text"`) || !strings.Contains(output, `"extensions": []`) {
		t.Fatalf("JSON collections or text fallback missing: %s", output)
	}
}

func assertImportRelationCapabilities(t *testing.T, capabilities map[string]api.LanguageCapabilities, ids ...string) {
	t.Helper()
	for _, id := range ids {
		language := capabilities[id]
		if language.Navigation != api.FeatureProduction || language.DirectoryArchitecture != api.FeatureProduction || language.ImportRelations != api.FeatureProduction {
			t.Fatalf("%s capabilities=%#v", id, language)
		}
	}
}

func assertRustLanguageCapabilities(t *testing.T, rust api.LanguageCapabilities) {
	t.Helper()
	if rust.Navigation != api.FeatureProduction || rust.GritQL != api.FeatureProduction || rust.FocusedStructure != api.FeatureProduction || rust.FocusedFlow != api.FeatureProduction || rust.ImportRelations != api.FeatureUnsupported || rust.Entrypoints != api.FeatureUnsupported {
		t.Fatalf("rust capabilities=%#v", rust)
	}
}

func TestLanguageCapabilityMatrixReportsPythonStructuralParity(t *testing.T) {
	for _, capability := range languageCapabilityMatrix() {
		if capability.Language != "python" {
			continue
		}
		if capability.Navigation != api.FeatureProduction || capability.GritQL != api.FeatureProduction || capability.FocusedStructure != api.FeatureProduction || capability.FocusedFlow != api.FeatureProduction || capability.ImportRelations != api.FeatureProduction {
			t.Fatalf("python capabilities=%#v", capability)
		}
		return
	}
	t.Fatal("python capabilities missing")
}

func TestLanguagesHumanOutputUsesCapabilityIcons(t *testing.T) {
	output := captureStdout(t, func() {
		if err := Run([]string{"languages"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"LANGUAGE", "STRUCTURAL", "GRITQL", "javascript", "markdown", "✓ production", "~ specialized production"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output %q does not contain %q", output, expected)
		}
	}
}

func TestLanguagesRejectsUnexpectedArguments(t *testing.T) {
	if err := Run([]string{"languages", "extra"}); err == nil {
		t.Fatal("expected unexpected argument to fail")
	}
}

func TestLanguageCapabilityMatrixGritQLCoversTreeSitterLanguages(t *testing.T) {
	for _, capability := range languageCapabilityMatrix() {
		if capability.StructuralGrep == api.FeatureProduction && capability.Navigation == api.FeatureProduction && capability.GritQL != api.FeatureProduction {
			t.Fatalf("Tree-sitter language %q lacks production GritQL: %#v", capability.Language, capability)
		}
	}
}

func TestLanguageCapabilityDocumentationIsGeneratedFromRegistrations(t *testing.T) {
	content, err := os.ReadFile("../../docs/file-type-support.md")
	if err != nil {
		t.Fatal(err)
	}
	const startMarker = "<!-- grepple:language-matrix:start -->"
	const endMarker = "<!-- grepple:language-matrix:end -->"
	source := string(content)
	start := strings.Index(source, startMarker)
	end := strings.Index(source, endMarker)
	if start < 0 || end < start {
		t.Fatal("language matrix markers are missing")
	}
	actual := strings.TrimSpace(source[start+len(startMarker) : end])
	expected := strings.TrimSpace(renderLanguageCapabilitiesMarkdown(languageCapabilityMatrix()))
	if actual != expected {
		t.Fatalf("documented language matrix is stale; regenerate with grepple languages --markdown\nactual:\n%s\nexpected:\n%s", actual, expected)
	}
}

func TestLanguagesRejectsConflictingOutputFormats(t *testing.T) {
	if err := Run([]string{"languages", "--json", "--markdown"}); err == nil {
		t.Fatal("expected conflicting output formats to fail")
	}
}
