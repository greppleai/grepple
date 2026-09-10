package extract

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageBundleCanonicalFactsAndCheck(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/bundle\n")
	directory := filepath.Join(root, "api")
	writeScopeFile(t, filepath.Join(directory, "b.go"), `// Package api exposes records. Additional package details.
package api
func Build(ID) *Record { return nil }
func Ping() {}
`)
	writeScopeFile(t, filepath.Join(directory, "a.go"), `//go:build linux
package api
import "github.com/gofiber/fiber/v2"
type ID string
type Alias = ID
type Contract interface { Find(ID) (*Record, error) }
type Record struct { ID ID `+"`json:\"id\"`"+`; Untagged ID }
type handler struct { Record *Record }
func (h *handler) Register(app *fiber.App) { app.Post("/records", h.Create); app.Get("/records", h.List) }
func (h *handler) List(*fiber.Ctx) error { return nil }
func (h *handler) Create(*fiber.Ctx) error { return nil }
`)

	first := mustGeneratePackageBundle(t, directory)
	second := mustGeneratePackageBundle(t, directory)
	assertDeterministicPackageBundles(t, first, second)
	manifest := decodePackageManifest(t, first.Manifest)
	assertPackageManifestFacts(t, manifest)
	assertPackageBundleRelations(t, first)
	assertPackageBundleMetadata(t, first)
	assertPackageBundleCheckAndDrift(t, root, directory, first)
}

func mustGeneratePackageBundle(t *testing.T, directory string) *PackageBundle {
	t.Helper()
	bundle, err := GeneratePackageBundle(directory)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func assertDeterministicPackageBundles(t *testing.T, first, second *PackageBundle) {
	t.Helper()
	if string(first.Manifest) != string(second.Manifest) || string(first.Overview) != string(second.Overview) || string(first.Structure) != string(second.Structure) {
		t.Fatal("bundle is not deterministic")
	}
}

func decodePackageManifest(t *testing.T, content []byte) PackageIR {
	t.Helper()
	var manifest PackageIR
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func assertPackageManifestFacts(t *testing.T, manifest PackageIR) {
	t.Helper()
	if manifest.Package.ImportPath != "example.com/bundle/api" || manifest.Package.SourceDirectory != "api" || manifest.Package.Documentation != "Package api exposes records." || manifest.Summary.Declarations != 5 || manifest.Summary.ExportedFunctions != 2 || manifest.Summary.FiberRoutes != 2 {
		t.Fatalf("manifest facts: %+v", manifest)
	}
	if !strings.HasPrefix(manifest.SemanticModelDigest, "sha256:") || len(manifest.SemanticModelDigest) != 71 {
		t.Fatalf("digest = %q", manifest.SemanticModelDigest)
	}
}

func assertPackageBundleRelations(t *testing.T, bundle *PackageBundle) {
	t.Helper()
	structure := string(bundle.Structure)
	if !strings.Contains(structure, `Record "1" --> "1" ID : field ID`) || !strings.Contains(structure, `Build "1" ..> "1" ID : parameter 1`) {
		t.Fatalf("missing evidenced relations:\n%s", bundle.Structure)
	}
}

func assertPackageBundleMetadata(t *testing.T, bundle *PackageBundle) {
	t.Helper()
	overview := string(bundle.Overview)
	if !strings.Contains(overview, "direction LR") || !strings.Contains(overview, "namespace transport") || !strings.Contains(overview, "namespace data_contracts") {
		t.Fatalf("overview projection:\n%s", bundle.Overview)
	}
	for name, artifact := range map[string]string{"overview": overview, "structure": string(bundle.Structure)} {
		assertPackageArtifactDefaults(t, name, artifact)
	}
	if strings.Count(string(bundle.Structure), "%% grepple:exact-default") != 1 || strings.Contains(overview, "%% grepple:exact-default") {
		t.Fatalf("exact-default must occur only once in structure")
	}
	visible := []string{
		`note "Package api exposes records. | package example.com/bundle/api | scope:`,
		`class Alias["Alias = ID · api/a.go:5"]`,
		`class ID["ID = string · api/a.go:4"]`,
		`note for handler "routes: GET /records -&gt; handler.List; POST /records -&gt; handler.Create"`,
		`+Build(ID): *Record`,
		`+Ping()`,
	}
	for _, fragment := range visible {
		if !strings.Contains(overview, fragment) {
			t.Errorf("overview missing %q:\n%s", fragment, overview)
		}
	}
}

func assertPackageArtifactDefaults(t *testing.T, name, artifact string) {
	t.Helper()
	if strings.Count(artifact, "%% grepple:package-default example.com/bundle/api") != 1 || strings.Count(artifact, "%% grepple:language-default go") != 1 {
		t.Fatalf("%s defaults missing or duplicated:\n%s", name, artifact)
	}
	for _, redundant := range []string{"%% grepple:package Alias ", "<<go>>", "<<export>>"} {
		if strings.Contains(artifact, redundant) {
			t.Fatalf("%s contains redundant metadata %q:\n%s", name, redundant, artifact)
		}
	}
}

func assertPackageBundleCheckAndDrift(t *testing.T, root, directory string, bundle *PackageBundle) {
	t.Helper()
	output := filepath.Join(root, "bundle")
	if err := WritePackageBundle(output, bundle); err != nil {
		t.Fatal(err)
	}
	if err := CheckPackageBundle(output, directory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "manifest.json"), append(bundle.Manifest, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckPackageBundle(output, directory); err == nil || !strings.Contains(err.Error(), "manifest.json") {
		t.Fatalf("manifest drift error = %v", err)
	}
}

func TestPackageBundleSelfLocationAndExplicitAgreement(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/selflocating\n")
	source := filepath.Join(root, "internal", "model")
	writeScopeFile(t, filepath.Join(source, "model.go"), "package model\ntype Item struct{}\n")
	bundle, err := GeneratePackageBundle(source)
	if err != nil {
		t.Fatal(err)
	}
	bundleDirectory := filepath.Join(root, "docs", "model.package")
	if err := WritePackageBundle(bundleDirectory, bundle); err != nil {
		t.Fatal(err)
	}
	oldWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.Chdir(other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWorkingDirectory) })
	if err := CheckPackageBundle(bundleDirectory, ""); err != nil {
		t.Fatalf("self-located check from %s: %v", other, err)
	}
	wrong := filepath.Join(root, "wrong")
	writeScopeFile(t, filepath.Join(wrong, "wrong.go"), "package wrong\ntype Item struct{}\n")
	if err := CheckPackageBundle(bundleDirectory, wrong); err == nil {
		t.Fatal("explicit source unexpectedly agreed with canonical bundle")
	}
}

func TestPackageBundleRejectsUnexpectedAndMissingFiles(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "bundle")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WritePackageBundle(output, &PackageBundle{}); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unexpected-file error = %v", err)
	}
}

func TestPackageBundleRealPackageShapedFixture(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/searchfixture\n")
	directory := filepath.Join(root, "search")
	writeScopeFile(t, filepath.Join(directory, "model.go"), `package search
type Params struct { Query string; Repository string; Limit int; Offset int; CaseSensitive bool }
type FileMatch struct { File string; Display string; Lines []string; MatchCount int; Repository string }
type Matches = []FileMatch
`)
	writeScopeFile(t, filepath.Join(directory, "search.go"), `package search
func (p Params) Validate() error { return nil }
func (m FileMatch) Count() int { return len(m.Lines) }
func (m FileMatch) Empty() bool { return len(m.Lines) == 0 }
func (m FileMatch) Display() string { return m.File }
func Files(p Params, candidates []string) ([]FileMatch, error) { return nil, nil }
`)
	bundle, err := GeneratePackageBundle(directory)
	if err != nil {
		t.Fatal(err)
	}
	overview := string(bundle.Overview)
	if !strings.Contains(overview, "+Files(Params, []string): tuple~[]FileMatch&#44;error~") {
		t.Fatalf("overview does not contain exact Files signature:\n%s", overview)
	}
	if len(bundle.Overview) >= len(bundle.Structure) {
		t.Fatalf("overview is not smaller: overview=%d structure=%d", len(bundle.Overview), len(bundle.Structure))
	}
	if _, err := ParseClassDiagram(overview); err != nil {
		t.Fatalf("overview does not parse: %v", err)
	}
}

func TestPackageBundleRouteDigestIgnoresSourceLineShifts(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/routes\n")
	directory := filepath.Join(root, "api")
	sourcePath := filepath.Join(directory, "api.go")
	source := `package api
import "github.com/gofiber/fiber/v2"
type Handler struct{}
func (h *Handler) Register(app *fiber.App) { app.Get("/items", h.List) }
func (h *Handler) List(*fiber.Ctx) error { return nil }
`
	writeScopeFile(t, sourcePath, source)
	first, _, err := BuildPackageIR(directory)
	if err != nil {
		t.Fatal(err)
	}
	writeScopeFile(t, sourcePath, "// comment-only line shift\n"+source)
	second, _, err := BuildPackageIR(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.SemanticModelDigest != second.SemanticModelDigest {
		t.Fatalf("digest drifted after comment-only line shift: %s != %s", first.SemanticModelDigest, second.SemanticModelDigest)
	}
	if len(first.Routes) != 1 || len(second.Routes) != 1 || first.Routes[0].Line+1 != second.Routes[0].Line {
		t.Fatalf("route locations did not track line shift: %+v %+v", first.Routes, second.Routes)
	}
	manifest, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `"line"`) {
		t.Fatalf("canonical route facts omit source line: %s", manifest)
	}
}

func TestPackageBundleManifestPutsSummaryBeforeArrays(t *testing.T) {
	ir := PackageIR{Package: PackageIdentity{}, Scope: PackageScope{}, SourceFiles: []PackageSourceFile{}, Declarations: []PackageDeclaration{}, ExportedFunctions: []PackageMember{}, Routes: []PackageRoute{}, Relations: []PackageRelation{}}
	encoded, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	summary := strings.Index(text, `"summary"`)
	digest := strings.Index(text, `"semanticModelDigest"`)
	arrays := strings.Index(text, `"sourceFiles":[]`)
	if summary < 0 || digest < summary || arrays < digest {
		t.Fatalf("manifest top-level order is not identity/summary-first: %s", text)
	}
}

func TestWritePackageBundleCommitFailureRollsBack(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	old := &PackageBundle{Manifest: []byte("old manifest"), Overview: []byte("old overview"), Structure: []byte("old structure")}
	if err := WritePackageBundle(output, old); err != nil {
		t.Fatal(err)
	}
	originalRename := bundleRename
	t.Cleanup(func() { bundleRename = originalRename })
	calls := 0
	bundleRename = func(oldPath, newPath string) error {
		calls++
		if calls == 2 {
			return errors.New("injected commit failure")
		}
		return os.Rename(oldPath, newPath)
	}
	if err := WritePackageBundle(output, &PackageBundle{}); err == nil || !strings.Contains(err.Error(), "injected commit failure") {
		t.Fatalf("commit failure = %v", err)
	}
	if calls != 3 {
		t.Fatalf("rename calls = %d, want prepare, commit, rollback", calls)
	}
	content, err := os.ReadFile(filepath.Join(output, "manifest.json"))
	if err != nil || string(content) != "old manifest" {
		t.Fatalf("old bundle not restored: %q, %v", content, err)
	}
}

func TestOverviewProjectionDetailsAndCollapsedRelations(t *testing.T) {
	ir := &PackageIR{
		Package: PackageIdentity{ImportPath: "example.com/view"},
		Scope:   PackageScope{},
		Declarations: []PackageDeclaration{
			{Name: "Target", Kind: "struct", File: "model.go"},
			{Name: "Contract", Kind: "interface", File: "model.go", Members: []PackageMember{{Name: "First", Kind: "method"}, {Name: "Second", Kind: "method", Result: "error"}}},
			{Name: "Payload", Kind: "struct", File: "model.go", Members: []PackageMember{{Name: "One", Kind: "property", Result: "Target", StructTag: &PackageStructTag{Value: `json:"one"`}}, {Name: "Two", Kind: "property", Result: "Target"}}},
		},
		ExportedFunctions: []PackageMember{{Name: "Unrelated", Kind: "function"}},
		Relations: []PackageRelation{
			{From: "Payload", To: "Target", Kind: "association", Via: "field One", Cardinality: "one"},
			{From: "Payload", To: "Target", Kind: "association", Via: "field Two", Cardinality: "one"},
			{From: "Payload", To: "Target", Kind: "association", Via: "field Three", Cardinality: "one"},
			{From: "Payload", To: "Target", Kind: "association", Via: "field Four", Cardinality: "one"},
		},
	}
	overview := renderBundleOverview(ir)
	if !strings.HasPrefix(overview, "classDiagram\n    %% grepple:package-default example.com/view\n    %% grepple:language-default go\n    direction LR\n    note \"") {
		t.Fatalf("defaults, direction, and summary note are not in canonical order:\n%s", overview)
	}
	for _, want := range []string{"+First()", "+Second(): error", "+One: Target", "+Two: Target", "+Unrelated()", "field=4"} {
		if !strings.Contains(overview, want) {
			t.Errorf("overview missing %q:\n%s", want, overview)
		}
	}
	if strings.Count(overview, `Payload "1" --> "1" Target`) != 1 || strings.Contains(overview, "field One") {
		t.Fatalf("overview relations were not collapsed by semantic endpoints:\n%s", overview)
	}
	if got := len(ir.Relations); got != 4 {
		t.Fatalf("manifest relation facts were modified: %d", got)
	}
	exact := collapseOverviewRelations(ir.Relations[:3])
	if len(exact) != 1 || exact[0].Via != "field One; field Three; field Two" {
		t.Fatalf("small relation group did not retain sorted exact evidence: %+v", exact)
	}
}

func TestPackageManifestDefaultsAreLosslessAndCompact(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/defaults\n")
	directory := filepath.Join(root, "model")
	writeScopeFile(t, filepath.Join(directory, "model.go"), "package model\ntype Item struct { Tagged string `json:\"tagged\"`; Plain string }\nfunc Ping() {}\n")
	bundle, err := GeneratePackageBundle(directory)
	if err != nil {
		t.Fatal(err)
	}
	text := string(bundle.Manifest)
	for _, noisy := range []string{`"visibility":`, `"exported":`, `"present":`} {
		if strings.Contains(text, noisy) {
			t.Errorf("manifest contains redundant field %s:\n%s", noisy, text)
		}
	}
	if !strings.Contains(text, `"kind": "function"`) {
		t.Fatalf("package function kind is not canonical:\n%s", text)
	}
	var ir PackageIR
	if err := json.Unmarshal(bundle.Manifest, &ir); err != nil {
		t.Fatal(err)
	}
	if len(ir.ExportedFunctions) != 1 || ir.ExportedFunctions[0].Parameters != nil || ir.ExportedFunctions[0].Result != "" {
		t.Fatalf("empty function defaults did not round trip: %+v", ir.ExportedFunctions)
	}
	item := ir.Declarations[0]
	if item.Members[0].File != "" || item.Members[1].File != "" {
		t.Fatalf("member file should inherit declaration file: %+v", item.Members)
	}
	tags := map[string]*PackageStructTag{}
	for _, member := range item.Members {
		tags[member.Name] = member.StructTag
	}
	if tags["Plain"] != nil || tags["Tagged"] == nil || tags["Tagged"].Value != `json:"tagged"` {
		t.Fatalf("struct tag presence was not lossless: %+v", tags)
	}
}

func TestWritePackageBundleFailurePreservesExistingBundle(t *testing.T) {
	output := filepath.Join(t.TempDir(), "bundle")
	old := &PackageBundle{Manifest: []byte("old manifest"), Overview: []byte("old overview"), Structure: []byte("old structure")}
	if err := WritePackageBundle(output, old); err != nil {
		t.Fatal(err)
	}
	originalWrite := bundleWriteFile
	t.Cleanup(func() { bundleWriteFile = originalWrite })
	bundleWriteFile = func(name string, content []byte, mode os.FileMode) error {
		if filepath.Base(name) == "overview.mmd" {
			return errors.New("injected write failure")
		}
		return os.WriteFile(name, content, mode)
	}
	updated := &PackageBundle{Manifest: []byte("new manifest"), Overview: []byte("new overview"), Structure: []byte("new structure")}
	if err := WritePackageBundle(output, updated); err == nil || !strings.Contains(err.Error(), "injected write failure") {
		t.Fatalf("write failure = %v", err)
	}
	for name, want := range map[string]string{"manifest.json": "old manifest", "overview.mmd": "old overview", "structure.mmd": "old structure"} {
		content, err := os.ReadFile(filepath.Join(output, name))
		if err != nil || string(content) != want {
			t.Fatalf("%s changed after failed transaction: %q, %v", name, content, err)
		}
	}
}

func TestPackageBuildConstraintsAreHeaderOnlyAndIncludeFilenameConstraints(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/constraints\n")
	directory := filepath.Join(root, "variants")
	writeScopeFile(t, filepath.Join(directory, "base.go"), `package variants
//go:build ignored_after_package
// +build also_ignored
type Base struct{}
`)
	writeScopeFile(t, filepath.Join(directory, "block.go"), `/* license */
// +build ignored_after_block

package variants
type Block struct{}
`)
	writeScopeFile(t, filepath.Join(directory, "explicit.go"), `// +build linux darwin

package variants
type Explicit struct{}
`)
	writeScopeFile(t, filepath.Join(directory, "variant_windows_amd64.go"), `//go:build cgo && (custom || linux)
package variants
type FilenameVariant struct{}
`)

	first, _, err := BuildPackageIR(directory)
	if err != nil {
		t.Fatal(err)
	}
	if first.Scope.BuildTags != "syntactic-union-conflicts-rejected" {
		t.Fatalf("scope = %q", first.Scope.BuildTags)
	}
	wantFiles := map[string][]string{
		"variants/base.go":                  nil,
		"variants/block.go":                 nil,
		"variants/explicit.go":              {"linux || darwin"},
		"variants/variant_windows_amd64.go": {"amd64", "cgo && (custom || linux)", "windows"},
	}
	for _, file := range first.SourceFiles {
		want := wantFiles[file.Path]
		if strings.Join(file.BuildTags, "|") != strings.Join(want, "|") {
			t.Errorf("%s constraints = %#v, want %#v", file.Path, file.BuildTags, want)
		}
	}
	wantUnion := "amd64|cgo && (custom || linux)|linux || darwin|windows"
	if got := strings.Join(first.Scope.TagUnion, "|"); got != wantUnion {
		t.Fatalf("constraint union = %q, want %q", got, wantUnion)
	}

	originalGOOS, hadGOOS := os.LookupEnv("GOOS")
	t.Cleanup(func() {
		if hadGOOS {
			_ = os.Setenv("GOOS", originalGOOS)
		} else {
			_ = os.Unsetenv("GOOS")
		}
	})
	_ = os.Setenv("GOOS", "plan9")
	second, _, err := BuildPackageIR(directory)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("package IR depends on runtime GOOS")
	}
}

func TestPackageBuildConstraintsRejectMalformedLeadingGoBuild(t *testing.T) {
	root := t.TempDir()
	writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/invalidconstraint\n")
	directory := filepath.Join(root, "invalid")
	writeScopeFile(t, filepath.Join(directory, "invalid.go"), "//go:build linux &&\npackage invalid\ntype T struct{}\n")
	if _, _, err := BuildPackageIR(directory); err == nil || !strings.Contains(err.Error(), "parse build constraints") {
		t.Fatalf("malformed constraint error = %v", err)
	}
}

func TestPackageSyntacticUnionRejectsDuplicateGoDeclarations(t *testing.T) {
	tests := []struct {
		name, base, linux, windows, symbol string
	}{
		{name: "types", linux: "type Variant struct{}", windows: "type Variant string", symbol: "Variant"},
		{name: "functions", linux: "func variant() {}", windows: "func variant() {}", symbol: "variant"},
		{name: "methods", base: "type Variant struct{}", linux: "func (Variant) Run() {}", windows: "func (*Variant) Run() {}", symbol: "Variant.Run"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeScopeFile(t, filepath.Join(root, "go.mod"), "module example.com/duplicates\n")
			directory := filepath.Join(root, "variants")
			if test.base != "" {
				writeScopeFile(t, filepath.Join(directory, "base.go"), "package variants\n"+test.base+"\n")
			}
			writeScopeFile(t, filepath.Join(directory, "variant_linux.go"), "package variants\n"+test.linux+"\n")
			writeScopeFile(t, filepath.Join(directory, "variant_windows.go"), "package variants\n"+test.windows+"\n")
			_, _, err := BuildPackageIR(directory)
			if err == nil || !strings.Contains(err.Error(), test.symbol) || !strings.Contains(err.Error(), "variant_linux.go:2:") || !strings.Contains(err.Error(), "variant_windows.go:2:") {
				t.Fatalf("duplicate error = %v", err)
			}
		})
	}
}
