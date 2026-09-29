package languages

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/greppleai/grepple/internal/wire"
)

func TestLanguagesJSONReportsRegisteredFeatureParity(t *testing.T) {
	output := captureStdout(t, func() {
		if err := New(Dependencies{}).Run([]string{"--json"}); err != nil {
			t.Fatal(err)
		}
	})
	var capabilities []wire.LanguageCapabilities
	if err := json.Unmarshal([]byte(output), &capabilities); err != nil {
		t.Fatal(err)
	}
	byLanguage := make(map[string]wire.LanguageCapabilities, len(capabilities))
	for _, capability := range capabilities {
		byLanguage[capability.Language] = capability
	}
	goLanguage := byLanguage["go"]
	if goLanguage.GritQL != wire.FeatureProduction || goLanguage.FocusedFlow != wire.FeatureProduction || goLanguage.DirectoryArchitecture != wire.FeatureProduction || goLanguage.ImportRelations != wire.FeatureProduction || goLanguage.Entrypoints != wire.FeatureProduction {
		t.Fatalf("go capabilities=%#v", goLanguage)
	}
	tsx := byLanguage["tsx"]
	if tsx.GritQL != wire.FeatureProduction || tsx.FocusedStructure != wire.FeatureProduction || tsx.DirectoryArchitecture != wire.FeatureProduction || tsx.ImportRelations != wire.FeatureProduction || tsx.Entrypoints != wire.FeatureProduction {
		t.Fatalf("tsx capabilities=%#v", tsx)
	}
	javascript := byLanguage["javascript"]
	if javascript.Navigation != wire.FeatureProduction || javascript.GritQL != wire.FeatureProduction || javascript.FocusedStructure != wire.FeatureProduction || javascript.FocusedFlow != wire.FeatureProduction || javascript.ImportRelations != wire.FeatureProduction || javascript.Entrypoints != wire.FeatureProduction {
		t.Fatalf("javascript capabilities=%#v", javascript)
	}
	swift := byLanguage["swift"]
	if swift.Navigation != wire.FeatureProduction || swift.GritQL != wire.FeatureProduction || swift.FocusedStructure != wire.FeatureProduction || swift.FocusedFlow != wire.FeatureProduction || swift.DirectoryArchitecture != wire.FeatureProduction || swift.Entrypoints != wire.FeatureUnsupported || swift.NavigationFacts.MemberAccess != wire.FeatureUnsupported {
		t.Fatalf("Swift capabilities=%#v", swift)
	}
	assertImportRelationCapabilities(t, byLanguage, "java", "kotlin", "csharp", "rust")
	assertEntrypointCapabilities(t, byLanguage, "java", "javascript", "kotlin", "csharp", "rust", "tsx", "typescript")
	assertRustLanguageCapabilities(t, byLanguage["rust"])
	markdown := byLanguage["markdown"]
	if markdown.StructuralGrep != wire.FeatureSpecialized || markdown.Outline != wire.FeatureSpecialized || markdown.Navigation != wire.FeatureNotApplicable || markdown.FocusedStructure != wire.FeatureNotApplicable || markdown.FocusedFlow != wire.FeatureNotApplicable || markdown.GritQL != wire.FeatureNotApplicable || markdown.DirectoryArchitecture != wire.FeatureNotApplicable || markdown.ImportRelations != wire.FeatureNotApplicable || markdown.Entrypoints != wire.FeatureNotApplicable || markdown.NavigationFacts.Calls != wire.FeatureNotApplicable {
		t.Fatalf("markdown capabilities=%#v", markdown)
	}
	if !strings.Contains(output, `"language": "text"`) || !strings.Contains(output, `"extensions": []`) {
		t.Fatalf("JSON collections or text fallback missing: %s", output)
	}
}

func assertImportRelationCapabilities(t *testing.T, capabilities map[string]wire.LanguageCapabilities, ids ...string) {
	t.Helper()
	for _, id := range ids {
		language := capabilities[id]
		if language.Navigation != wire.FeatureProduction || language.DirectoryArchitecture != wire.FeatureProduction || language.ImportRelations != wire.FeatureProduction {
			t.Fatalf("%s capabilities=%#v", id, language)
		}
	}
}

func assertEntrypointCapabilities(t *testing.T, capabilities map[string]wire.LanguageCapabilities, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if language := capabilities[id]; language.Entrypoints != wire.FeatureProduction {
			t.Fatalf("%s entrypoint capability=%#v", id, language)
		}
	}
}

func assertRustLanguageCapabilities(t *testing.T, rust wire.LanguageCapabilities) {
	t.Helper()
	if rust.Navigation != wire.FeatureProduction || rust.GritQL != wire.FeatureProduction || rust.FocusedStructure != wire.FeatureProduction || rust.FocusedFlow != wire.FeatureProduction || rust.ImportRelations != wire.FeatureProduction || rust.Entrypoints != wire.FeatureProduction {
		t.Fatalf("rust capabilities=%#v", rust)
	}
}

func TestLanguageCapabilityMatrixReportsPythonStructuralParity(t *testing.T) {
	for _, capability := range languageCapabilityMatrix() {
		if capability.Language != "python" {
			continue
		}
		if capability.Navigation != wire.FeatureProduction || capability.GritQL != wire.FeatureProduction || capability.FocusedStructure != wire.FeatureProduction || capability.FocusedFlow != wire.FeatureProduction || capability.ImportRelations != wire.FeatureProduction || capability.Entrypoints != wire.FeatureProduction {
			t.Fatalf("python capabilities=%#v", capability)
		}
		return
	}
	t.Fatal("python capabilities missing")
}

func TestLanguageCapabilityMatrixReportsNavigationFacts(t *testing.T) {
	byLanguage := make(map[string]wire.LanguageCapabilities)
	for _, capability := range languageCapabilityMatrix() {
		byLanguage[capability.Language] = capability
	}
	production := wire.FeatureProduction
	unsupported := wire.FeatureUnsupported
	goFacts := byLanguage["go"].NavigationFacts
	if goFacts.Declarations != production || goFacts.Calls != production || goFacts.Imports != production || goFacts.TypeReferences != production || goFacts.Fields != production || goFacts.MemberAccess != production || goFacts.Entrypoints != production {
		t.Fatalf("Go facts=%#v", goFacts)
	}
	assertECMANavigationFacts(t, byLanguage)
	java := byLanguage["java"].NavigationFacts
	if java.Imports != production || java.TypeReferences != production || java.Fields != production || java.MemberAccess != production || java.Entrypoints != production {
		t.Fatalf("Java facts=%#v", java)
	}
	for _, id := range []string{"python", "kotlin", "csharp", "rust"} {
		facts := byLanguage[id].NavigationFacts
		if facts.TypeReferences != production || facts.Fields != production {
			t.Fatalf("%s typed facts=%#v", id, facts)
		}
	}
	shell := byLanguage["shell"].NavigationFacts
	if shell.Declarations != production || shell.Calls != production || shell.Imports != unsupported || shell.MemberAccess != unsupported {
		t.Fatalf("Shell facts=%#v", shell)
	}
	if jsonFacts := byLanguage["json"].NavigationFacts; jsonFacts.Declarations != unsupported || jsonFacts.Calls != unsupported {
		t.Fatalf("JSON facts=%#v", jsonFacts)
	}
}

func assertECMANavigationFacts(t *testing.T, byLanguage map[string]wire.LanguageCapabilities) {
	t.Helper()
	production := wire.FeatureProduction
	unsupported := wire.FeatureUnsupported
	javascript := byLanguage["javascript"].NavigationFacts
	if javascript.TypeReferences != production || javascript.Fields != unsupported || javascript.MemberAccess != production || javascript.Entrypoints != production {
		t.Fatalf("JavaScript facts=%#v", javascript)
	}
	typescript := byLanguage["typescript"].NavigationFacts
	if typescript.Entrypoints != production || byLanguage["tsx"].NavigationFacts.Entrypoints != production {
		t.Fatalf("TypeScript facts=%#v TSX facts=%#v", typescript, byLanguage["tsx"].NavigationFacts)
	}
}

func TestLanguagesHumanOutputUsesCapabilityIcons(t *testing.T) {
	output := captureStdout(t, func() {
		if err := New(Dependencies{}).Run(nil); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"LANGUAGE", "STRUCTURAL", "GRITQL", "NAVIGATION FACT SUPPORT", "TYPE-REFS", "individual facts may remain ambiguous or unresolved", "javascript", "markdown", "✓ production", "~ specialized production"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output %q does not contain %q", output, expected)
		}
	}
}

func TestLanguagesRejectsUnexpectedArguments(t *testing.T) {
	if err := New(Dependencies{}).Run([]string{"extra"}); err == nil {
		t.Fatal("expected unexpected argument to fail")
	}
}

func TestLanguageCapabilityMatrixGritQLCoversTreeSitterLanguages(t *testing.T) {
	for _, capability := range languageCapabilityMatrix() {
		if capability.StructuralGrep == wire.FeatureProduction && capability.Navigation == wire.FeatureProduction && capability.GritQL != wire.FeatureProduction {
			t.Fatalf("Tree-sitter language %q lacks production GritQL: %#v", capability.Language, capability)
		}
	}
}

func TestLanguageCapabilityDocumentationIsGeneratedFromRegistrations(t *testing.T) {
	content, err := os.ReadFile("../../../docs/file-type-support.md")
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
	if err := New(Dependencies{}).Run([]string{"--json", "--markdown"}); err == nil {
		t.Fatal("expected conflicting output formats to fail")
	}
}
