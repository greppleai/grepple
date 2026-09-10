package extract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/build/constraint"
	goparser "go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// PackageBundleFormatVersion is the canonical package manifest format version.
const PackageBundleFormatVersion = 1

var packageBundleFiles = []string{"manifest.json", "overview.mmd", "structure.mmd"}

// PackageIR is the canonical, syntax-derived semantic model shared by every
// artifact in a generated package bundle.
type PackageIR struct {
	FormatVersion       int                  `json:"formatVersion"`
	Package             PackageIdentity      `json:"package"`
	Scope               PackageScope         `json:"scope"`
	Summary             PackageSummary       `json:"summary"`
	SemanticModelDigest string               `json:"semanticModelDigest"`
	SourceFiles         []PackageSourceFile  `json:"sourceFiles"`
	Declarations        []PackageDeclaration `json:"declarations"`
	ExportedFunctions   []PackageMember      `json:"exportedFunctions"`
	Routes              []PackageRoute       `json:"fiberRoutes"`
	Relations           []PackageRelation    `json:"relations"`
}

// PackageIdentity identifies the package represented by a bundle.
type PackageIdentity struct {
	Name            string `json:"name"`
	ImportPath      string `json:"importPath"`
	SourceDirectory string `json:"sourceDirectory"`
	Documentation   string `json:"documentation,omitempty"`
}

// PackageScope records which source constructs and build variants are represented.
type PackageScope struct {
	Types     string   `json:"types"`
	Functions string   `json:"functions"`
	Variables string   `json:"variables"`
	Constants string   `json:"constants"`
	Tests     string   `json:"tests"`
	BuildTags string   `json:"buildTags"`
	TagUnion  []string `json:"syntacticBuildTagUnion"`
}

// PackageSourceFile records one source file and its syntactic build constraints.
type PackageSourceFile struct {
	Path      string   `json:"path"`
	BuildTags []string `json:"buildTags,omitempty"`
}

// PackageDeclaration is one named type and its exact source location.
type PackageDeclaration struct {
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	File       string          `json:"file"`
	Line       int             `json:"line,omitempty"`
	EndLine    int             `json:"endLine,omitempty"`
	FileLocal  bool            `json:"fileLocal,omitempty"`
	Underlying string          `json:"underlyingType"`
	Members    []PackageMember `json:"members,omitempty"`
}

// PackageMember is one field, method, or package function.
type PackageMember struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	File       string            `json:"file,omitempty"`
	Line       int               `json:"line,omitempty"`
	EndLine    int               `json:"endLine,omitempty"`
	Parameters []string          `json:"parameters,omitempty"`
	Result     string            `json:"result,omitempty"`
	StructTag  *PackageStructTag `json:"structTag,omitempty"`
}

// PackageStructTag preserves an explicitly present Go struct tag.
type PackageStructTag struct {
	Value string `json:"value"`
}

// PackageRoute is one syntax-derived Fiber route registration.
type PackageRoute struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler"`
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"endLine,omitempty"`
}

// PackageRelation records one evidenced relationship between declarations.
type PackageRelation struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Kind        string `json:"kind"`
	Via         string `json:"via"`
	Cardinality string `json:"cardinality"`
}

// PackageSummary contains compact bundle counts.
type PackageSummary struct {
	SourceFiles       int `json:"sourceFiles"`
	Declarations      int `json:"declarations"`
	ExportedTypes     int `json:"exportedTypes"`
	InternalTypes     int `json:"internalTypes"`
	Members           int `json:"members"`
	ExportedFunctions int `json:"exportedFunctions"`
	FiberRoutes       int `json:"fiberRoutes"`
	Relations         int `json:"relations"`
}

// PackageBundle contains the three canonical generated artifacts.
type PackageBundle struct {
	Manifest  []byte
	Overview  []byte
	Structure []byte
}

// BuildPackageIR analyzes exactly one non-test Go package.
func BuildPackageIR(directory string) (*PackageIR, []Source, error) {
	sources, analysis, packageID, declarations, err := analyzePackageDirectory(directory)
	if err != nil {
		return nil, nil, err
	}
	ir, err := newPackageIR(directory, sources, analysis, packageID)
	if err != nil {
		return nil, nil, err
	}
	if err := populatePackageSourceFiles(ir, sources, analysis); err != nil {
		return nil, nil, err
	}
	populatePackageDeclarations(ir, declarations)
	if err := populatePackageFunctions(ir, analysis, packageID); err != nil {
		return nil, nil, err
	}
	populatePackageRoutes(ir, analysis, packageID)
	ir.Relations = packageIRRelations(ir, declarations)
	ir.Summary = packageSummary(ir)
	if err := setPackageDigest(ir); err != nil {
		return nil, nil, err
	}
	return ir, sources, nil
}

func analyzePackageDirectory(directory string) ([]Source, *Analysis, string, map[string]*Declaration, error) {
	sources, err := loadPackageSources(directory)
	if err != nil {
		return nil, nil, "", nil, err
	}
	if err := validateSelectedPackage(sources); err != nil {
		return nil, nil, "", nil, err
	}
	analysis, err := Analyze(sources)
	if err != nil {
		return nil, nil, "", nil, err
	}
	ids := sortedKeys(analysis.GoPackageNames)
	if len(ids) != 1 {
		return nil, nil, "", nil, fmt.Errorf("selected directory must resolve to exactly one Go package; found %d", len(ids))
	}
	packageID := ids[0]
	if analysis.GoPackagePaths[packageID] == "" {
		return nil, nil, "", nil, fmt.Errorf("cannot resolve Go import path for selected directory %s (no enclosing go.mod with a module path)", directory)
	}
	declarations := packageDeclarationsForID(analysis, packageID)
	inferGoImplementations(declarations)
	return sources, analysis, packageID, declarations, nil
}

func packageDeclarationsForID(analysis *Analysis, packageID string) map[string]*Declaration {
	declarations := map[string]*Declaration{}
	for _, declaration := range analysis.GoDeclarations {
		if declaration.PackageID == packageID {
			declarations[declaration.Name] = declaration
		}
	}
	return declarations
}

func newPackageIR(directory string, sources []Source, analysis *Analysis, packageID string) (*PackageIR, error) {
	sourceDirectory, err := normalizedPackageSourceDirectory(directory)
	if err != nil {
		return nil, err
	}
	packageName := analysis.GoPackageNames[packageID]
	return &PackageIR{
		FormatVersion: PackageBundleFormatVersion,
		Package:       PackageIdentity{Name: packageName, ImportPath: analysis.GoPackagePaths[packageID], SourceDirectory: sourceDirectory, Documentation: packageDocumentation(sources, packageName)},
		Scope:         PackageScope{Types: "all", Functions: "exported", Variables: "excluded", Constants: "excluded", Tests: "excluded", BuildTags: "syntactic-union-conflicts-rejected", TagUnion: []string{}},
		SourceFiles:   []PackageSourceFile{}, Declarations: []PackageDeclaration{}, ExportedFunctions: []PackageMember{}, Routes: []PackageRoute{}, Relations: []PackageRelation{},
	}, nil
}

func populatePackageSourceFiles(ir *PackageIR, sources []Source, analysis *Analysis) error {
	allTags := map[string]bool{}
	for _, source := range sources {
		tags, err := syntacticBuildConstraints(source)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			allTags[tag] = true
		}
		file := analysis.SourcePaths[absolutePath(source.Path)]
		ir.SourceFiles = append(ir.SourceFiles, PackageSourceFile{Path: file, BuildTags: tags})
	}
	sort.Slice(ir.SourceFiles, func(i, j int) bool { return ir.SourceFiles[i].Path < ir.SourceFiles[j].Path })
	ir.Scope.TagUnion = sortedKeys(allTags)
	return nil
}

func populatePackageDeclarations(ir *PackageIR, declarations map[string]*Declaration) {
	for _, name := range sortedKeys(declarations) {
		ir.Declarations = append(ir.Declarations, packageDeclaration(declarations[name]))
	}
}

func packageDeclaration(declaration *Declaration) PackageDeclaration {
	item := PackageDeclaration{Name: declaration.Name, Kind: declaration.Kind, File: declaration.File, Line: declaration.Location.Line, EndLine: declaration.Location.EndLine, FileLocal: declaration.FileLocal, Underlying: declaration.Underlying, Members: []PackageMember{}}
	for _, member := range declaration.Members {
		converted := packageMember(member)
		if converted.File == item.File {
			converted.File = ""
		}
		if member.Kind == "property" {
			tag := declaration.StructTags[member.Name]
			if tag.Present {
				converted.StructTag = &PackageStructTag{Value: tag.Value}
			}
		}
		item.Members = append(item.Members, converted)
	}
	sortPackageMembers(item.Members)
	return item
}

func populatePackageFunctions(ir *PackageIR, analysis *Analysis, packageID string) error {
	for _, name := range packageFunctionNames(analysis, packageID) {
		functions := analysis.GoFunctions[packageID+":"+name]
		if len(functions) != 1 {
			return fmt.Errorf("exported package function %q is ambiguous", name)
		}
		converted := packageMember(functions[0])
		converted.Kind = "function"
		ir.ExportedFunctions = append(ir.ExportedFunctions, converted)
	}
	return nil
}

func populatePackageRoutes(ir *PackageIR, analysis *Analysis, packageID string) {
	for _, route := range analysis.GoFiberRoutes {
		if route.PackageID != packageID {
			continue
		}
		ir.Routes = append(ir.Routes, PackageRoute{Method: route.Method, Path: route.Path, Handler: route.Handler, File: normalizeBundleFile(route.Location.Path, analysis), Line: route.Location.Line, EndLine: route.Location.EndLine})
	}
	sort.Slice(ir.Routes, func(i, j int) bool { return routeSortKey(ir.Routes[i]) < routeSortKey(ir.Routes[j]) })
	ir.Routes = uniqueRoutes(ir.Routes)
}

func setPackageDigest(ir *PackageIR) error {
	digestModel := semanticPackageIR(ir)
	digestModel.SemanticModelDigest = ""
	digestBytes, err := json.Marshal(digestModel)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(digestBytes)
	ir.SemanticModelDigest = "sha256:" + hex.EncodeToString(sum[:])
	return nil
}

func packageMember(member Member) PackageMember {
	parameters := append([]string(nil), member.Parameters...)
	return PackageMember{Name: member.Name, Kind: member.Kind, File: member.File, Line: member.Location.Line, EndLine: member.Location.EndLine, Parameters: parameters, Result: member.Type}
}

func semanticPackageIR(ir *PackageIR) PackageIR {
	result := *ir
	result.Declarations = append([]PackageDeclaration(nil), ir.Declarations...)
	for index := range result.Declarations {
		result.Declarations[index].Line, result.Declarations[index].EndLine = 0, 0
		result.Declarations[index].Members = append([]PackageMember(nil), ir.Declarations[index].Members...)
		for member := range result.Declarations[index].Members {
			result.Declarations[index].Members[member].Line, result.Declarations[index].Members[member].EndLine = 0, 0
		}
	}
	result.ExportedFunctions = append([]PackageMember(nil), ir.ExportedFunctions...)
	for index := range result.ExportedFunctions {
		result.ExportedFunctions[index].Line, result.ExportedFunctions[index].EndLine = 0, 0
	}
	result.Routes = append([]PackageRoute(nil), ir.Routes...)
	for index := range result.Routes {
		result.Routes[index].Line, result.Routes[index].EndLine = 0, 0
	}
	return result
}

func sortPackageMembers(members []PackageMember) {
	sort.SliceStable(members, func(i, j int) bool {
		return memberSortKey(members[i]) < memberSortKey(members[j])
	})
}
func memberSortKey(m PackageMember) string {
	return m.File + "\x00" + m.Kind + "\x00" + m.Name + "\x00" + strings.Join(m.Parameters, "\x00") + "\x00" + m.Result
}
func routeSortKey(r PackageRoute) string {
	return r.Method + "\x00" + r.Path + "\x00" + r.Handler + "\x00" + r.File
}
func uniqueRoutes(in []PackageRoute) []PackageRoute {
	out := []PackageRoute{}
	seen := map[string]bool{}
	for _, r := range in {
		k := r.Method + "\x00" + r.Path + "\x00" + r.Handler
		if !seen[k] {
			seen[k] = true
			out = append(out, r)
		}
	}
	return out
}

func normalizeBundleFile(path string, analysis *Analysis) string {
	if value := analysis.SourcePaths[absolutePath(path)]; value != "" {
		return value
	}
	return filepath.ToSlash(path)
}

// syntacticBuildConstraints returns only constraints that Go recognizes in the
// leading file header, plus the GOOS/GOARCH constraints implied by the name.
func syntacticBuildConstraints(source Source) ([]string, error) {
	set := map[string]bool{}
	header, goBuild, err := goBuildHeader(source.Text)
	if err != nil {
		return nil, fmt.Errorf("parse build constraints in %s: %w", source.Path, err)
	}
	if goBuild != "" {
		expression, parseErr := constraint.Parse(goBuild)
		if parseErr != nil {
			return nil, fmt.Errorf("parse build constraints in %s: %w", source.Path, parseErr)
		}
		set[expression.String()] = true
	} else {
		for _, line := range strings.Split(header, "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
			if !constraint.IsPlusBuild(line) {
				continue
			}
			expression, parseErr := constraint.Parse(line)
			if parseErr == nil { // go/build also ignores malformed legacy +build lines.
				set[expression.String()] = true
			}
		}
	}
	for _, expression := range goFilenameConstraints(filepath.Base(source.Path)) {
		set[expression] = true
	}
	return sortedKeys(set), nil
}

// goBuildHeader follows go/build's header boundaries: //go:build is accepted in
// the leading comment run, while legacy +build lines must precede the final
// blank line separating that run from the package clause.
func goBuildHeader(text string) (string, string, error) {
	state := goBuildHeaderState{content: strings.ReplaceAll(text, "\r\n", "\n")}
	for _, raw := range strings.SplitAfter(state.content, "\n") {
		done, err := state.consume(raw)
		if err != nil {
			return "", "", err
		}
		if done {
			return state.content[:state.headerEnd], state.goBuild, nil
		}
	}
	return state.content[:state.headerEnd], state.goBuild, nil
}

type goBuildHeaderState struct {
	content           string
	headerEnd, offset int
	inBlock, ended    bool
	goBuild           string
}

func (state *goBuildHeaderState) consume(raw string) (bool, error) {
	line := strings.TrimSpace(strings.TrimSuffix(raw, "\n"))
	if line == "" && !state.ended {
		state.headerEnd = state.offset + len(raw)
		state.offset += len(raw)
		return false, nil
	}
	if !strings.HasPrefix(line, "//") {
		state.ended = true
	}
	if !state.inBlock && constraint.IsGoBuild(line) {
		if state.goBuild != "" {
			return false, fmt.Errorf("multiple //go:build comments")
		}
		state.goBuild = line
	}
	done := state.consumeCommentText(line)
	state.offset += len(raw)
	return done, nil
}

func (state *goBuildHeaderState) consumeCommentText(remaining string) bool {
	for remaining != "" {
		if state.inBlock {
			end := strings.Index(remaining, "*/")
			if end < 0 {
				return false
			}
			state.inBlock = false
			remaining = strings.TrimSpace(remaining[end+2:])
			continue
		}
		if strings.HasPrefix(remaining, "//") {
			return false
		}
		if strings.HasPrefix(remaining, "/*") {
			state.inBlock = true
			remaining = strings.TrimSpace(remaining[2:])
			continue
		}
		return true
	}
	return false
}

var knownFilenameGOOS = map[string]bool{
	"aix": true, "android": true, "darwin": true, "dragonfly": true,
	"freebsd": true, "hurd": true, "illumos": true, "ios": true,
	"js": true, "linux": true, "nacl": true, "netbsd": true,
	"openbsd": true, "plan9": true, "solaris": true, "wasip1": true,
	"windows": true, "zos": true,
}

var knownFilenameGOARCH = map[string]bool{
	"386": true, "amd64": true, "amd64p32": true, "arm": true,
	"armbe": true, "arm64": true, "arm64be": true, "loong64": true,
	"mips": true, "mipsle": true, "mips64": true, "mips64le": true,
	"mips64p32": true, "mips64p32le": true, "ppc": true, "ppc64": true,
	"ppc64le": true, "riscv": true, "riscv64": true, "s390": true,
	"s390x": true, "sparc": true, "sparc64": true, "wasm": true,
}

func goFilenameConstraints(name string) []string {
	stem := strings.SplitN(name, ".", 2)[0]
	firstUnderscore := strings.Index(stem, "_")
	if firstUnderscore <= 0 {
		return nil
	}
	parts := strings.Split(stem[firstUnderscore:], "_")
	if len(parts) > 0 && parts[len(parts)-1] == "test" {
		parts = parts[:len(parts)-1]
	}
	count := len(parts)
	if count >= 2 && knownFilenameGOOS[parts[count-2]] && knownFilenameGOARCH[parts[count-1]] {
		return []string{parts[count-2], parts[count-1]}
	}
	if count >= 1 && (knownFilenameGOOS[parts[count-1]] || knownFilenameGOARCH[parts[count-1]]) {
		return []string{parts[count-1]}
	}
	return nil
}

func packageDocumentation(sources []Source, packageName string) string {
	type candidate struct {
		path, text string
	}
	candidates := []candidate{}
	for _, source := range sources {
		file, err := goparser.ParseFile(token.NewFileSet(), source.Path, source.Text, goparser.ParseComments|goparser.PackageClauseOnly)
		if err != nil || file.Doc == nil {
			continue
		}
		text := strings.Join(strings.Fields(file.Doc.Text()), " ")
		if !strings.HasPrefix(text, "Package "+packageName+" ") {
			continue
		}
		candidates = append(candidates, candidate{path: filepath.ToSlash(source.Path), text: firstDocumentationSentence(text)})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].path < candidates[j].path })
	if len(candidates) == 0 {
		return ""
	}
	return candidates[0].text
}

func firstDocumentationSentence(text string) string {
	for index := 0; index < len(text); index++ {
		if text[index] != '.' && text[index] != '!' && text[index] != '?' {
			continue
		}
		if index+1 == len(text) || text[index+1] == ' ' || text[index+1] == '\t' || text[index+1] == '\n' {
			return text[:index+1]
		}
	}
	return text
}

func packageIRRelations(ir *PackageIR, declarations map[string]*Declaration) []PackageRelation {
	local := map[string]bool{}
	for _, declaration := range ir.Declarations {
		local[declaration.Name] = true
	}
	result := []PackageRelation{}
	for _, declaration := range ir.Declarations {
		appendDeclarationRelations(&result, local, declaration, declarations[declaration.Name])
	}
	for _, function := range ir.ExportedFunctions {
		appendFunctionRelations(&result, local, function)
	}
	return sortedUniquePackageRelations(result)
}

func appendDeclarationRelations(result *[]PackageRelation, local map[string]bool, declaration PackageDeclaration, raw *Declaration) {
	for _, parent := range sortedKeys(raw.Extends) {
		appendPackageRelation(result, local, declaration.Name, parent, "inheritance", "embedded type", "one")
	}
	for _, parent := range sortedKeys(raw.Implements) {
		appendPackageRelation(result, local, declaration.Name, parent, "implementation", "implements", "one")
	}
	appendUnderlyingRelations(result, local, declaration)
	for _, member := range declaration.Members {
		appendPackageMemberRelations(result, local, declaration.Name, member)
	}
}

func appendUnderlyingRelations(result *[]PackageRelation, local map[string]bool, declaration PackageDeclaration) {
	if declaration.Kind != "alias" && declaration.Kind != "type" {
		return
	}
	for _, target := range sortedKeys(local) {
		if cardinality, found := packageGoTypeCardinality(declaration.Underlying, target); found {
			appendPackageRelation(result, local, declaration.Name, target, "dependency", "underlying type", cardinality)
		}
	}
}

func appendPackageMemberRelations(result *[]PackageRelation, local map[string]bool, owner string, member PackageMember) {
	if member.Kind == "property" {
		addTypeRelations(result, local, owner, member.Result, "association", "field "+member.Name)
		return
	}
	for index, parameter := range member.Parameters {
		addTypeRelations(result, local, owner, parameter, "dependency", fmt.Sprintf("method %s parameter %d", member.Name, index+1))
	}
	for index, value := range resultTypes(member.Result) {
		addTypeRelations(result, local, owner, value, "dependency", fmt.Sprintf("method %s result %d", member.Name, index+1))
	}
}

func appendFunctionRelations(result *[]PackageRelation, local map[string]bool, function PackageMember) {
	for index, parameter := range function.Parameters {
		addTypeRelations(result, local, function.Name, parameter, "dependency", fmt.Sprintf("parameter %d", index+1))
	}
	for index, value := range resultTypes(function.Result) {
		addTypeRelations(result, local, function.Name, value, "dependency", fmt.Sprintf("result %d", index+1))
	}
}

func appendPackageRelation(result *[]PackageRelation, local map[string]bool, from, to, kind, via, cardinality string) {
	if from == to || !local[to] {
		return
	}
	*result = append(*result, PackageRelation{From: from, To: to, Kind: kind, Via: via, Cardinality: cardinality})
}

func sortedUniquePackageRelations(result []PackageRelation) []PackageRelation {
	sort.Slice(result, func(i, j int) bool { return relationSortKey(result[i]) < relationSortKey(result[j]) })
	out := result[:0]
	last := ""
	for _, relation := range result {
		key := relationSortKey(relation)
		if key != last {
			out = append(out, relation)
			last = key
		}
	}
	return out
}

func addTypeRelations(result *[]PackageRelation, local map[string]bool, from, value, kind, via string) {
	for _, target := range sortedKeys(local) {
		if cardinality, found := packageGoTypeCardinality(value, target); from != target && found {
			*result = append(*result, PackageRelation{From: from, To: target, Kind: kind, Via: via, Cardinality: cardinality})
		}
	}
}

func relationSortKey(r PackageRelation) string {
	return r.From + "\x00" + r.To + "\x00" + r.Kind + "\x00" + r.Via + "\x00" + r.Cardinality
}
func resultTypes(value string) []string {
	if strings.HasPrefix(value, "tuple<") && strings.HasSuffix(value, ">") {
		return splitParameters(value[6 : len(value)-1])
	}
	if value == "" {
		return nil
	}
	return []string{value}
}

func packageSummary(ir *PackageIR) PackageSummary {
	s := PackageSummary{SourceFiles: len(ir.SourceFiles), Declarations: len(ir.Declarations), ExportedFunctions: len(ir.ExportedFunctions), FiberRoutes: len(ir.Routes), Relations: len(ir.Relations)}
	for _, declaration := range ir.Declarations {
		s.Members += len(declaration.Members)
		if exportedGoName(declaration.Name) {
			s.ExportedTypes++
		} else {
			s.InternalTypes++
		}
	}
	return s
}

// GeneratePackageBundle creates and self-validates all canonical artifacts.
func GeneratePackageBundle(directory string) (*PackageBundle, error) {
	ir, sources, err := BuildPackageIR(directory)
	if err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(ir, "", "  ")
	if err != nil {
		return nil, err
	}
	manifest = append(manifest, '\n')
	bundle := &PackageBundle{Manifest: manifest, Structure: []byte(renderBundleStructure(ir)), Overview: []byte(renderBundleOverview(ir))}
	for name, content := range map[string][]byte{"overview.mmd": bundle.Overview, "structure.mmd": bundle.Structure} {
		// Artifacts are standalone schemas; canonical checks add bundle-only cardinality.
		diagnostics, checkErr := CheckClassDiagram(string(content), sources)
		if checkErr != nil {
			return nil, fmt.Errorf("self-validate %s: %w", name, checkErr)
		}
		if len(diagnostics) > 0 {
			return nil, fmt.Errorf("self-validate %s: %s", name, diagnostics[0].Message)
		}
		diagnostics, checkErr = checkPackageBundleDiagram(string(content), sources)
		if checkErr != nil {
			return nil, fmt.Errorf("self-validate canonical %s: %w", name, checkErr)
		}
		if len(diagnostics) > 0 {
			return nil, fmt.Errorf("self-validate canonical %s: %s", name, diagnostics[0].Message)
		}
	}
	return bundle, nil
}

func renderBundleStructure(ir *PackageIR) string {
	lines := []string{
		"classDiagram",
		"    %% grepple:complete-package " + ir.Package.ImportPath,
		"    %% grepple:package-default " + ir.Package.ImportPath,
		"    %% grepple:language-default go",
		"    %% grepple:exact-default",
	}
	for _, declaration := range ir.Declarations {
		lines = append(lines, renderIRDeclaration(declaration, ir, true)...)
	}
	for _, function := range ir.ExportedFunctions {
		lines = append(lines, renderIRFunction(function, ir.Package.ImportPath)...)
	}
	for _, relation := range ir.Relations {
		lines = append(lines, renderIRRelation(relation))
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderBundleOverview(ir *PackageIR) string {
	groups := map[string][]string{"transport": {}, "interfaces": {}, "data_contracts": {}, "exported_types": {}, "internal_types": {}, "operations": {}}
	routeOwners := map[string]bool{}
	for _, route := range ir.Routes {
		routeOwners[strings.SplitN(route.Handler, ".", 2)[0]] = true
	}
	for _, declaration := range ir.Declarations {
		group := "internal_types"
		switch {
		case routeOwners[declaration.Name]:
			group = "transport"
		case declaration.Kind == "interface":
			group = "interfaces"
		case hasPresentTag(declaration):
			group = "data_contracts"
		case exportedGoName(declaration.Name):
			group = "exported_types"
		}
		groups[group] = append(groups[group], renderIRDeclaration(declaration, ir, false)...)
	}
	for _, function := range ir.ExportedFunctions {
		groups["operations"] = append(groups["operations"], renderIRFunctionOverview(function, ir.Package.ImportPath)...)
	}
	lines := []string{
		"classDiagram",
		"    %% grepple:package-default " + ir.Package.ImportPath,
		"    %% grepple:language-default go",
		"    direction LR",
		"    note \"" + mermaidNoteText(packageSummaryNote(ir)) + "\"",
	}
	for _, name := range []string{"transport", "interfaces", "data_contracts", "exported_types", "internal_types", "operations"} {
		if len(groups[name]) == 0 {
			continue
		}
		lines = append(lines, "    namespace "+name+" {")
		lines = append(lines, groups[name]...)
		lines = append(lines, "    }")
	}
	for _, relation := range collapseOverviewRelations(ir.Relations) {
		lines = append(lines, renderIRRelation(relation))
	}
	lines = append(lines, renderOverviewNotes(ir)...)
	return strings.Join(lines, "\n") + "\n"
}

func hasPresentTag(declaration PackageDeclaration) bool {
	for _, member := range declaration.Members {
		if member.StructTag != nil {
			return true
		}
	}
	return false
}

func renderIRDeclaration(declaration PackageDeclaration, ir *PackageIR, exact bool) []string {
	indent := packageDeclarationIndent(exact)
	members := packageDeclarationMembers(declaration, exact)
	lines := renderPackageDeclarationClass(declaration, members, indent, exact)
	lines = append(lines, renderPackageDeclarationStereotype(declaration, indent))
	if exact {
		lines = append(lines, renderExactPackageDeclarationMetadata(declaration, ir, indent)...)
	}
	return lines
}

func packageDeclarationIndent(exact bool) string {
	if exact {
		return "    "
	}
	return "        "
}

func packageDeclarationMembers(declaration PackageDeclaration, exact bool) []PackageMember {
	if exact {
		return declaration.Members
	}
	members := []PackageMember{}
	if declaration.Kind == "interface" {
		for _, member := range declaration.Members {
			if member.Kind != "property" {
				members = append(members, member)
			}
		}
		return members
	}
	if declaration.Kind == "struct" && hasPresentTag(declaration) {
		for _, member := range declaration.Members {
			if member.Kind == "property" {
				members = append(members, member)
			}
		}
	}
	return members
}

func renderPackageDeclarationClass(declaration PackageDeclaration, members []PackageMember, indent string, exact bool) []string {
	classSyntax := packageDeclarationClassSyntax(declaration, exact)
	if len(members) == 0 {
		return []string{indent + classSyntax}
	}
	lines := []string{indent + classSyntax + " {"}
	for _, member := range members {
		lines = append(lines, indent+"    "+renderPackageIRMember(member))
	}
	return append(lines, indent+"}")
}

func packageDeclarationClassSyntax(declaration PackageDeclaration, exact bool) string {
	classSyntax := "class " + declaration.Name
	label := declaration.Name
	if declaration.Kind == "alias" || declaration.Kind == "type" {
		label += " = " + declaration.Underlying
	}
	if !exact && declaration.Line > 0 {
		label += " · " + packageRange(declaration.File, declaration.Line, declaration.EndLine)
	}
	if label != declaration.Name {
		classSyntax += "[\"" + mermaidDisplayLabel(label) + "\"]"
	}
	return classSyntax
}

func renderPackageDeclarationStereotype(declaration PackageDeclaration, indent string) string {
	if declaration.Kind == "interface" {
		return indent + "<<interface>> " + declaration.Name
	}
	return indent + "<<" + declaration.Kind + ">> " + declaration.Name
}

func renderExactPackageDeclarationMetadata(declaration PackageDeclaration, ir *PackageIR, indent string) []string {
	lines := []string{}
	if declaration.Kind == "alias" || declaration.Kind == "type" {
		lines = append(lines, indent+"%% grepple:underlying "+declaration.Name+" "+declaration.Underlying)
	}
	lines = append(lines, indent+"%% grepple:file "+declaration.Name+" "+declaration.File)
	lines = append(lines, renderPackageStructTags(declaration, indent)...)
	if declaration.FileLocal {
		lines = append(lines, indent+"%% grepple:filelocal "+declaration.Name)
	}
	lines = append(lines, renderPackageRoutes(declaration, ir, indent)...)
	if note := packageDeclarationLocationNote(declaration); note != "" {
		lines = append(lines, indent+"note for "+declaration.Name+" \""+mermaidNoteText(note)+"\"")
	}
	return lines
}

func renderPackageStructTags(declaration PackageDeclaration, indent string) []string {
	if declaration.Kind != "struct" {
		return nil
	}
	lines := []string{}
	for _, member := range declaration.Members {
		if member.StructTag != nil {
			lines = append(lines, fmt.Sprintf("%s%%%% grepple:struct-tag %s %s %q", indent, declaration.Name, member.Name, member.StructTag.Value))
		}
	}
	return lines
}

func renderPackageRoutes(declaration PackageDeclaration, ir *PackageIR, indent string) []string {
	lines := []string{}
	for _, route := range ir.Routes {
		if strings.HasPrefix(route.Handler, declaration.Name+".") {
			lines = append(lines, fmt.Sprintf("%s%%%% grepple:route %s %s %s", indent, route.Method, route.Path, route.Handler))
		}
	}
	return lines
}

func packageDeclarationLocationNote(declaration PackageDeclaration) string {
	if declaration.Line == 0 {
		return ""
	}
	parts := []string{"defined: " + packageRange(declaration.File, declaration.Line, declaration.EndLine)}
	for _, member := range declaration.Members {
		if member.Line > 0 {
			parts = append(parts, member.Name+"@"+packageMemberRange(member, declaration.File))
		}
	}
	return strings.Join(parts, "; ")
}

func packageMemberRange(member PackageMember, inheritedFile string) string {
	file := member.File
	if file == "" {
		file = inheritedFile
	}
	return packageRange(file, member.Line, member.EndLine)
}

func packageRange(path string, start, end int) string {
	value := path + ":" + strconv.Itoa(start)
	if end > start {
		value += "-" + strconv.Itoa(end)
	}
	return value
}
func renderPackageIRMember(member PackageMember) string {
	visibility := goVisibility(member.Name)
	return renderMember(Member{Kind: member.Kind, Name: member.Name, Visibility: visibility, Type: member.Result, Parameters: member.Parameters})
}
func renderIRFunction(function PackageMember, _ string) []string {
	lines := []string{"    class " + function.Name + " {", "        " + renderPackageIRMember(function), "    }", "    <<function>> " + function.Name, "    %% grepple:file " + function.Name + " " + function.File}
	if function.Line > 0 {
		lines = append(lines, "    note for "+function.Name+" \"defined: "+mermaidNoteText(packageMemberRange(function, ""))+"\"")
	}
	return lines
}
func renderIRFunctionOverview(function PackageMember, _ string) []string {
	classSyntax := "class " + function.Name
	if function.Line > 0 {
		label := function.Name + " · " + packageMemberRange(function, "")
		classSyntax += "[\"" + mermaidDisplayLabel(label) + "\"]"
	}
	return []string{"        " + classSyntax + " {", "            " + renderPackageIRMember(function), "        }", "        <<function>> " + function.Name}
}

func renderOverviewNotes(ir *PackageIR) []string {
	lines := []string{}
	routesByOwner := map[string][]string{}
	for _, route := range ir.Routes {
		owner := strings.SplitN(route.Handler, ".", 2)[0]
		routesByOwner[owner] = append(routesByOwner[owner], route.Method+" "+route.Path+" -> "+route.Handler)
	}
	for _, owner := range sortedKeys(routesByOwner) {
		if !hasIRDeclaration(ir, owner) {
			continue
		}
		entries := routesByOwner[owner]
		sort.Strings(entries)
		lines = append(lines, "    note for "+owner+" \""+mermaidNoteText("routes: "+strings.Join(entries, "; "))+"\"")
	}
	return lines
}

func packageSummaryNote(ir *PackageIR) string {
	scope := ir.Scope
	summary := ir.Summary
	details := fmt.Sprintf("package %s | scope: types=%s, functions=%s, variables=%s, constants=%s, tests=%s, buildTags=%s | counts: files=%d, declarations=%d, exportedTypes=%d, internalTypes=%d, members=%d, exportedFunctions=%d, fiberRoutes=%d, relations=%d", ir.Package.ImportPath, scope.Types, scope.Functions, scope.Variables, scope.Constants, scope.Tests, scope.BuildTags, summary.SourceFiles, summary.Declarations, summary.ExportedTypes, summary.InternalTypes, summary.Members, summary.ExportedFunctions, summary.FiberRoutes, summary.Relations)
	if ir.Package.Documentation != "" {
		return ir.Package.Documentation + " | " + details
	}
	return details
}

func hasIRDeclaration(ir *PackageIR, name string) bool {
	for _, declaration := range ir.Declarations {
		if declaration.Name == name {
			return true
		}
	}
	return false
}

// mermaidNoteText keeps generated notes single-line and inert. Mermaid decodes
// character entities for display without treating their contents as syntax.
func mermaidNoteText(value string) string {
	var result strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			result.WriteString("&amp;")
		case '"':
			result.WriteString("&quot;")
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		case '\\':
			result.WriteString("&#92;")
		case '\n':
			result.WriteString("&#10;")
		case '\r':
			result.WriteString("&#13;")
		default:
			if r < 0x20 || r == 0x7f || r == '\u2028' || r == '\u2029' {
				fmt.Fprintf(&result, "&#%d;", r)
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}
func mermaidDisplayLabel(value string) string {
	var result strings.Builder
	for _, r := range value {
		switch r {
		case '&':
			result.WriteString("&amp;")
		case '"':
			result.WriteString("&quot;")
		case '<':
			result.WriteString("&lt;")
		case '>':
			result.WriteString("&gt;")
		case '\\':
			result.WriteString("&#92;")
		case '[':
			result.WriteString("&#91;")
		case ']':
			result.WriteString("&#93;")
		case '{':
			result.WriteString("&#123;")
		case '}':
			result.WriteString("&#125;")
		case '\n':
			result.WriteString("&#10;")
		case '\r':
			result.WriteString("&#13;")
		default:
			if r < 0x20 || r == 0x7f || r == '\u2028' || r == '\u2029' {
				fmt.Fprintf(&result, "&#%d;", r)
			} else {
				result.WriteRune(r)
			}
		}
	}
	return result.String()
}

func collapseOverviewRelations(relations []PackageRelation) []PackageRelation {
	type relationGroup struct {
		relation PackageRelation
		vias     []string
	}
	groups := map[string]*relationGroup{}
	for _, relation := range relations {
		key := relation.From + "\x00" + relation.To + "\x00" + relation.Kind + "\x00" + relation.Cardinality
		group := groups[key]
		if group == nil {
			groupRelation := relation
			groupRelation.Via = ""
			group = &relationGroup{relation: groupRelation}
			groups[key] = group
		}
		group.vias = append(group.vias, relation.Via)
	}
	result := make([]PackageRelation, 0, len(groups))
	for _, key := range sortedKeys(groups) {
		group := groups[key]
		sort.Strings(group.vias)
		if len(group.vias) <= 3 {
			group.relation.Via = strings.Join(group.vias, "; ")
		} else {
			counts := map[string]int{}
			for _, via := range group.vias {
				counts[relationEvidenceCategory(via)]++
			}
			parts := []string{}
			for _, category := range sortedKeys(counts) {
				parts = append(parts, fmt.Sprintf("%s=%d", category, counts[category]))
			}
			group.relation.Via = strings.Join(parts, "; ")
		}
		result = append(result, group.relation)
	}
	return result
}

func relationEvidenceCategory(via string) string {
	switch {
	case strings.HasPrefix(via, "field "):
		return "field"
	case strings.HasPrefix(via, "method ") && strings.Contains(via, " parameter "):
		return "method-parameter"
	case strings.HasPrefix(via, "method ") && strings.Contains(via, " result "):
		return "method-result"
	case strings.HasPrefix(via, "parameter "):
		return "parameter"
	case strings.HasPrefix(via, "result "):
		return "result"
	default:
		return strings.ReplaceAll(via, " ", "-")
	}
}

func renderIRRelation(relation PackageRelation) string {
	operator := "..>"
	if relation.Kind == "association" {
		operator = "-->"
	}
	if relation.Kind == "inheritance" {
		return fmt.Sprintf("    %s <|-- %s : %s", relation.To, relation.From, mermaidDisplayLabel(relation.Via))
	}
	if relation.Kind == "implementation" {
		return fmt.Sprintf("    %s <|.. %s : %s", relation.To, relation.From, mermaidDisplayLabel(relation.Via))
	}
	multiplicity := "1"
	if relation.Cardinality == "many" {
		multiplicity = "*"
	}
	return fmt.Sprintf("    %s \"1\" %s \"%s\" %s : %s", relation.From, operator, multiplicity, relation.To, mermaidDisplayLabel(relation.Via))
}

var (
	bundleWriteFile = os.WriteFile
	bundleRename    = os.Rename
)

// WritePackageBundle transactionally replaces only the three generated files.
func WritePackageBundle(output string, bundle *PackageBundle) error {
	output, parent, base, exists, err := preparePackageBundleOutput(output)
	if err != nil {
		return err
	}
	temporary, err := writeTemporaryPackageBundle(parent, base, bundle)
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	return commitPackageBundle(temporary, output, exists)
}

func preparePackageBundleOutput(output string) (string, string, string, bool, error) {
	if output == "" {
		return "", "", "", false, fmt.Errorf("bundle output directory is required")
	}
	output = filepath.Clean(output)
	parent, base := filepath.Dir(output), filepath.Base(output)
	if base == "." || base == string(filepath.Separator) {
		return "", "", "", false, fmt.Errorf("bundle output directory is invalid")
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", "", "", false, err
	}
	exists, err := existingPackageBundle(output)
	return output, parent, base, exists, err
}

func existingPackageBundle(output string) (bool, error) {
	info, err := os.Lstat(output)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("bundle output path is not a directory")
	}
	if err := validateWritableBundleDirectory(output); err != nil {
		return false, err
	}
	return true, nil
}

func writeTemporaryPackageBundle(parent, base string, bundle *PackageBundle) (string, error) {
	temporary, err := os.MkdirTemp(parent, "."+base+".tmp-")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(temporary, 0o755); err != nil {
		_ = os.RemoveAll(temporary)
		return "", err
	}
	files := map[string][]byte{"manifest.json": bundle.Manifest, "overview.mmd": bundle.Overview, "structure.mmd": bundle.Structure}
	for _, name := range packageBundleFiles {
		if err := bundleWriteFile(filepath.Join(temporary, name), files[name], 0o644); err != nil {
			_ = os.RemoveAll(temporary)
			return "", fmt.Errorf("write temporary bundle file %s: %w", name, err)
		}
	}
	return temporary, nil
}

func commitPackageBundle(temporary, output string, exists bool) error {
	if !exists {
		if err := bundleRename(temporary, output); err != nil {
			return fmt.Errorf("commit package bundle: %w", err)
		}
		return nil
	}
	backup := temporary + "-previous"
	if err := bundleRename(output, backup); err != nil {
		return fmt.Errorf("prepare package bundle replacement: %w", err)
	}
	if err := bundleRename(temporary, output); err != nil {
		return rollbackPackageBundle(output, backup, err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove replaced package bundle backup: %w", err)
	}
	return nil
}

func rollbackPackageBundle(output, backup string, commitErr error) error {
	if rollbackErr := bundleRename(backup, output); rollbackErr != nil {
		return fmt.Errorf("commit package bundle: %w (rollback failed: %v; previous bundle remains at %s)", commitErr, rollbackErr, backup)
	}
	return fmt.Errorf("commit package bundle: %w", commitErr)
}

func validateWritableBundleDirectory(output string) error {
	entries, err := os.ReadDir(output)
	if err != nil {
		return err
	}
	expected := map[string]bool{}
	for _, name := range packageBundleFiles {
		expected[name] = true
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			return fmt.Errorf("bundle directory contains unexpected pre-existing entry: %s", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("bundle generated path is not a regular file: %s", entry.Name())
		}
	}
	return nil
}

func normalizedPackageSourceDirectory(directory string) (string, error) {
	absolute := absolutePath(directory)
	root := nearestSourceRoot("go", absolute)
	if root == "" {
		return "", fmt.Errorf("cannot resolve project-relative source directory for %s (no enclosing go.mod)", directory)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("selected package directory %s is outside its Go module root", directory)
	}
	return path.Clean(filepath.ToSlash(relative)), nil
}

// ResolvePackageBundleSource safely resolves package.sourceDirectory from a bundle's
// strict manifest metadata against the bundle's (or current directory's) Go module root.
func ResolvePackageBundleSource(bundleDirectory string) (string, error) {
	content, err := os.ReadFile(filepath.Join(bundleDirectory, "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("read package bundle manifest: %w", err)
	}
	if !json.Valid(content) {
		return "", fmt.Errorf("invalid package bundle manifest JSON")
	}
	var manifest PackageIR
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return "", fmt.Errorf("invalid package bundle manifest metadata: %w", err)
	}
	if manifest.FormatVersion != PackageBundleFormatVersion {
		return "", fmt.Errorf("unsupported package bundle formatVersion: %d", manifest.FormatVersion)
	}
	relative := manifest.Package.SourceDirectory
	if relative == "" || strings.Contains(relative, `\`) || filepath.IsAbs(relative) || path.Clean(relative) != relative || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("invalid package.sourceDirectory %q: must be a normalized project-relative path", relative)
	}
	bundleAbsolute := absolutePath(bundleDirectory)
	root := nearestSourceRoot("go", bundleAbsolute)
	if root == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr == nil {
			root = nearestSourceRoot("go", cwd)
		}
	}
	if root == "" {
		return "", fmt.Errorf("cannot resolve package.sourceDirectory %q: no project go.mod found", relative)
	}
	resolved := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	if !pathWithin(root, resolved) {
		return "", fmt.Errorf("package.sourceDirectory %q escapes project root", relative)
	}
	realRoot, rootErr := filepath.EvalSymlinks(root)
	realResolved, resolvedErr := filepath.EvalSymlinks(resolved)
	if rootErr != nil || resolvedErr != nil || !pathWithin(realRoot, realResolved) {
		return "", fmt.Errorf("package.sourceDirectory %q does not safely resolve within the project root", relative)
	}
	actualRelative, err := normalizedPackageSourceDirectory(resolved)
	if err != nil || actualRelative != relative {
		return "", fmt.Errorf("package.sourceDirectory %q does not identify that path relative to the project root", relative)
	}
	return resolved, nil
}

// CheckPackageBundle regenerates, validates, and byte-compares a bundle. When
// sourceDirectory is empty, strict manifest metadata locates it from the Go module root.
func CheckPackageBundle(bundleDirectory, sourceDirectory string) error {
	if sourceDirectory == "" {
		var err error
		sourceDirectory, err = ResolvePackageBundleSource(bundleDirectory)
		if err != nil {
			return err
		}
	}
	expected, err := GeneratePackageBundle(sourceDirectory)
	if err != nil {
		return err
	}
	sources, err := loadPackageSources(sourceDirectory)
	if err != nil {
		return err
	}
	if err := validatePackageBundleNames(bundleDirectory); err != nil {
		return err
	}
	actual, err := readPackageBundleFiles(bundleDirectory)
	if err != nil {
		return err
	}
	if err := validatePackageBundleDiagrams(actual, sources); err != nil {
		return err
	}
	return comparePackageBundleFiles(actual, expected)
}

func validatePackageBundleNames(bundleDirectory string) error {
	entries, err := os.ReadDir(bundleDirectory)
	if err != nil {
		return fmt.Errorf("read package bundle: %w", err)
	}
	actualNames := map[string]bool{}
	for _, entry := range entries {
		actualNames[entry.Name()] = true
	}
	for _, name := range packageBundleFiles {
		if !actualNames[name] {
			return fmt.Errorf("package bundle missing required file: %s", name)
		}
	}
	extra := []string{}
	for name := range actualNames {
		if name != "manifest.json" && name != "overview.mmd" && name != "structure.mmd" {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		return fmt.Errorf("package bundle contains unexpected file: %s", extra[0])
	}
	return nil
}

func readPackageBundleFiles(bundleDirectory string) (map[string][]byte, error) {
	actual := map[string][]byte{}
	for _, name := range packageBundleFiles {
		content, err := os.ReadFile(filepath.Join(bundleDirectory, name))
		if err != nil {
			return nil, fmt.Errorf("read package bundle file %s: %w", name, err)
		}
		actual[name] = content
	}
	return actual, nil
}

func validatePackageBundleDiagrams(actual map[string][]byte, sources []Source) error {
	for _, name := range []string{"overview.mmd", "structure.mmd"} {
		diagnostics, err := checkPackageBundleDiagram(string(actual[name]), sources)
		if err != nil {
			return fmt.Errorf("validate package bundle %s: %w", name, err)
		}
		if len(diagnostics) > 0 {
			return fmt.Errorf("validate package bundle %s: %s", name, diagnostics[0].Message)
		}
	}
	return nil
}

func comparePackageBundleFiles(actual map[string][]byte, expected *PackageBundle) error {
	wanted := map[string][]byte{"manifest.json": expected.Manifest, "overview.mmd": expected.Overview, "structure.mmd": expected.Structure}
	for _, name := range packageBundleFiles {
		if !bytes.Equal(actual[name], wanted[name]) {
			return fmt.Errorf("package bundle artifact differs from canonical generated content: %s", name)
		}
	}
	return nil
}
