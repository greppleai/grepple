package mermaidcode

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	goparser "go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// WorkspaceBundleFormatVersion identifies the canonical workspace manifest schema.
const WorkspaceBundleFormatVersion = 1

var workspaceBundleFiles = []string{"manifest.json", "overview.mmd"}

// WorkspaceIR is a deterministic, syntax-derived inventory of the Go workspace.
type WorkspaceIR struct {
	FormatVersion       int                       `json:"formatVersion"`
	Workspace           WorkspaceIdentity         `json:"workspace"`
	Scope               WorkspaceScope            `json:"scope"`
	Summary             WorkspaceSummary          `json:"summary"`
	SemanticModelDigest string                    `json:"semanticModelDigest"`
	Modules             []WorkspaceModule         `json:"modules"`
	ExternalModules     []WorkspaceExternalModule `json:"externalModules"`
}

// WorkspaceIdentity stores the self-locating root anchor.
type WorkspaceIdentity struct {
	RootDirectory    string `json:"rootDirectory"`
	AnchorModulePath string `json:"anchorModulePath"`
}

// WorkspaceScope records deterministic discovery exclusions and build handling.
type WorkspaceScope struct {
	Packages    string   `json:"packages"`
	Types       string   `json:"types"`
	Functions   string   `json:"functions"`
	Variables   string   `json:"variables"`
	Constants   string   `json:"constants"`
	Tests       string   `json:"tests"`
	BuildTags   string   `json:"buildTags"`
	Directories []string `json:"excludedDirectories"`
	Symlinks    string   `json:"symlinks"`
}

// WorkspaceSummary contains machine-counted workspace totals.
type WorkspaceSummary struct {
	Modules             int `json:"modules"`
	Packages            int `json:"packages"`
	SourceFiles         int `json:"sourceFiles"`
	Declarations        int `json:"declarations"`
	ExportedFunctions   int `json:"exportedFunctions"`
	FiberRoutes         int `json:"fiberRoutes"`
	LocalImports        int `json:"localImports"`
	StandardImports     int `json:"standardLibraryImports"`
	ExternalModuleEdges int `json:"externalModuleEdges"`
}

// WorkspaceModule describes one discovered go.mod and its packages.
type WorkspaceModule struct {
	Path      string             `json:"path"`
	Directory string             `json:"directory"`
	Packages  []WorkspacePackage `json:"packages"`
	Require   []string           `json:"requiredModules"`
}

// WorkspacePackage contains syntax-derived package facts and imports.
type WorkspacePackage struct {
	ImportPath             string                    `json:"importPath"`
	Directory              string                    `json:"directory"`
	Name                   string                    `json:"name"`
	Documentation          string                    `json:"documentation,omitempty"`
	Main                   bool                      `json:"main"`
	SourceFiles            int                       `json:"sourceFiles"`
	Declarations           int                       `json:"declarations"`
	ExportedFunctions      int                       `json:"exportedFunctions"`
	FiberRoutes            int                       `json:"fiberRoutes"`
	Routes                 []PackageRoute            `json:"routes"`
	LocalImports           []string                  `json:"localImports"`
	StandardLibraryImports []string                  `json:"standardLibraryImports"`
	ExternalImports        []WorkspaceExternalImport `json:"externalImports"`
	UnmappedImports        []string                  `json:"unmappedExternalImports"`
}

// WorkspaceExternalImport maps an imported package to a required module.
type WorkspaceExternalImport struct {
	ImportPath string `json:"importPath"`
	ModulePath string `json:"modulePath"`
}

// WorkspaceExternalModule is a collapsed external dependency projection.
type WorkspaceExternalModule struct {
	Path       string   `json:"path"`
	ImportedBy []string `json:"importedBy"`
	Packages   []string `json:"packages"`
}

// WorkspaceBundle contains the two canonical generated artifacts.
type WorkspaceBundle struct {
	Manifest []byte
	Overview []byte
}

type discoveredModule struct {
	directory, relative, modulePath string
	requires                        []string
}

// BuildWorkspaceIR discovers every Go module and direct non-test package below root.
func BuildWorkspaceIR(root string) (*WorkspaceIR, error) {
	rootAbs, err := safeWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	modules, err := discoverWorkspaceModules(rootAbs)
	if err != nil {
		return nil, err
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no go.mod found at or below workspace root: %s", root)
	}
	anchor := nearestSourceRoot("go", rootAbs)
	if anchor == "" {
		return nil, fmt.Errorf("workspace root must be within a Go module so it can be self-locating: %s", root)
	}
	rootRelative, err := filepath.Rel(anchor, rootAbs)
	if err != nil || !safeRelative(filepath.ToSlash(rootRelative)) {
		return nil, fmt.Errorf("cannot normalize workspace root: %s", root)
	}
	anchorContent, err := os.ReadFile(filepath.Join(anchor, "go.mod"))
	if err != nil {
		return nil, err
	}
	ir := &WorkspaceIR{
		FormatVersion: WorkspaceBundleFormatVersion,
		Workspace:     WorkspaceIdentity{RootDirectory: path.Clean(filepath.ToSlash(rootRelative)), AnchorModulePath: goModulePath(string(anchorContent))},
		Scope:         WorkspaceScope{Packages: "direct-non-test-go-packages", Types: "all", Functions: "exported", Variables: "excluded", Constants: "excluded", Tests: "excluded", BuildTags: "syntactic-union-conflicts-rejected", Directories: sortedKeys(excludedDirectories), Symlinks: "excluded"},
		Modules:       []WorkspaceModule{}, ExternalModules: []WorkspaceExternalModule{},
	}
	moduleByDirectory := map[string]discoveredModule{}
	for _, module := range modules {
		moduleByDirectory[module.directory] = module
	}
	for _, module := range modules {
		packageDirectories, walkErr := discoverModulePackages(module, moduleByDirectory)
		if walkErr != nil {
			return nil, walkErr
		}
		item := WorkspaceModule{Path: module.modulePath, Directory: module.relative, Require: append([]string{}, module.requires...), Packages: []WorkspacePackage{}}
		for _, directory := range packageDirectories {
			packageIR, sources, buildErr := BuildPackageIR(directory)
			if buildErr != nil {
				return nil, fmt.Errorf("analyze workspace package %s: %w", directory, buildErr)
			}
			relative, _ := filepath.Rel(rootAbs, directory)
			pkg := WorkspacePackage{
				ImportPath: packageIR.Package.ImportPath, Directory: path.Clean(filepath.ToSlash(relative)), Name: packageIR.Package.Name,
				Documentation: packageIR.Package.Documentation, Main: packageIR.Package.Name == "main", SourceFiles: packageIR.Summary.SourceFiles,
				Declarations: packageIR.Summary.Declarations, ExportedFunctions: packageIR.Summary.ExportedFunctions, FiberRoutes: packageIR.Summary.FiberRoutes,
				Routes: append([]PackageRoute{}, packageIR.Routes...), LocalImports: []string{}, StandardLibraryImports: []string{}, ExternalImports: []WorkspaceExternalImport{}, UnmappedImports: []string{},
			}
			pkg.UnmappedImports, err = packageImports(sources)
			if err != nil {
				return nil, err
			}
			item.Packages = append(item.Packages, pkg)
		}
		ir.Modules = append(ir.Modules, item)
	}
	classifyWorkspaceImports(ir)
	workspaceSummaryAndExternal(ir)
	digest := *ir
	digest.SemanticModelDigest = ""
	encoded, err := json.Marshal(digest)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	ir.SemanticModelDigest = "sha256:" + hex.EncodeToString(sum[:])
	return ir, nil
}

func safeWorkspaceRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	absolute := absolutePath(root)
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", fmt.Errorf("select workspace root %s: %w", root, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("workspace root must be a real directory: %s", root)
	}
	return absolute, nil
}

func discoverWorkspaceModules(root string) ([]discoveredModule, error) {
	result := []discoveredModule{}
	err := filepath.WalkDir(root, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current != root && entry.IsDir() && (entry.Type()&os.ModeSymlink != 0 || excludedDirectories[entry.Name()]) {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || entry.Name() != "go.mod" {
			return nil
		}
		content, readErr := os.ReadFile(current)
		if readErr != nil {
			return readErr
		}
		modulePath := goModulePath(string(content))
		if modulePath == "" {
			return fmt.Errorf("go.mod has no module path: %s", current)
		}
		directory := filepath.Dir(current)
		relative, _ := filepath.Rel(root, directory)
		result = append(result, discoveredModule{directory: directory, relative: path.Clean(filepath.ToSlash(relative)), modulePath: modulePath, requires: goRequiredModules(string(content))})
		return nil
	})
	sort.Slice(result, func(i, j int) bool { return result[i].relative < result[j].relative })
	return result, err
}

func goRequiredModules(content string) []string {
	set := map[string]bool{}
	inBlock := false
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "//", 2)[0])
		if line == "" {
			continue
		}
		if line == "require (" {
			inBlock = true
			continue
		}
		if inBlock && line == ")" {
			inBlock = false
			continue
		}
		fields := strings.Fields(line)
		if inBlock && len(fields) >= 2 {
			set[fields[0]] = true
		}
		if !inBlock && len(fields) >= 3 && fields[0] == "require" {
			set[fields[1]] = true
		}
	}
	return sortedKeys(set)
}

func discoverModulePackages(module discoveredModule, all map[string]discoveredModule) ([]string, error) {
	result := []string{}
	err := filepath.WalkDir(module.directory, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if current != module.directory {
				if entry.Type()&os.ModeSymlink != 0 || excludedDirectories[entry.Name()] {
					return filepath.SkipDir
				}
				if _, nested := all[current]; nested {
					return filepath.SkipDir
				}
			}
			entries, readErr := os.ReadDir(current)
			if readErr != nil {
				return readErr
			}
			for _, child := range entries {
				if isPackageGoEntry(child) {
					result = append(result, current)
					break
				}
			}
		}
		return nil
	})
	sort.Strings(result)
	return result, err
}

func packageImports(sources []Source) ([]string, error) {
	set := map[string]bool{}
	for _, source := range sources {
		file, err := goparser.ParseFile(token.NewFileSet(), source.Path, source.Text, goparser.ImportsOnly)
		if err != nil {
			return nil, fmt.Errorf("parse imports in %s: %w", source.Path, err)
		}
		for _, spec := range file.Imports {
			value, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr == nil {
				set[value] = true
			}
		}
	}
	return sortedKeys(set), nil
}

func classifyWorkspaceImports(ir *WorkspaceIR) {
	locals := map[string]bool{}
	for _, module := range ir.Modules {
		for _, pkg := range module.Packages {
			locals[pkg.ImportPath] = true
		}
	}
	for mi := range ir.Modules {
		for pi := range ir.Modules[mi].Packages {
			pkg := &ir.Modules[mi].Packages[pi]
			imports := pkg.UnmappedImports
			pkg.UnmappedImports = []string{}
			for _, imported := range imports {
				switch {
				case locals[imported]:
					pkg.LocalImports = append(pkg.LocalImports, imported)
				case standardGoImport(imported):
					pkg.StandardLibraryImports = append(pkg.StandardLibraryImports, imported)
				default:
					modulePath := longestModulePrefix(imported, ir.Modules[mi].Require)
					if modulePath == "" {
						pkg.UnmappedImports = append(pkg.UnmappedImports, imported)
					} else {
						pkg.ExternalImports = append(pkg.ExternalImports, WorkspaceExternalImport{ImportPath: imported, ModulePath: modulePath})
					}
				}
			}
		}
	}
}

func standardGoImport(importPath string) bool {
	first := strings.SplitN(importPath, "/", 2)[0]
	return first != "" && !strings.Contains(first, ".")
}

func longestModulePrefix(importPath string, modules []string) string {
	best := ""
	for _, module := range modules {
		if (importPath == module || strings.HasPrefix(importPath, module+"/")) && len(module) > len(best) {
			best = module
		}
	}
	return best
}

func workspaceSummaryAndExternal(ir *WorkspaceIR) {
	external := map[string]*WorkspaceExternalModule{}
	for _, module := range ir.Modules {
		for _, pkg := range module.Packages {
			ir.Summary.Packages++
			ir.Summary.SourceFiles += pkg.SourceFiles
			ir.Summary.Declarations += pkg.Declarations
			ir.Summary.ExportedFunctions += pkg.ExportedFunctions
			ir.Summary.FiberRoutes += pkg.FiberRoutes
			ir.Summary.LocalImports += len(pkg.LocalImports)
			ir.Summary.StandardImports += len(pkg.StandardLibraryImports)
			seen := map[string]bool{}
			for _, dependency := range pkg.ExternalImports {
				item := external[dependency.ModulePath]
				if item == nil {
					item = &WorkspaceExternalModule{Path: dependency.ModulePath}
					external[dependency.ModulePath] = item
				}
				item.Packages = appendUnique(item.Packages, dependency.ImportPath)
				item.ImportedBy = appendUnique(item.ImportedBy, pkg.ImportPath)
				if !seen[dependency.ModulePath] {
					ir.Summary.ExternalModuleEdges++
					seen[dependency.ModulePath] = true
				}
			}
		}
	}
	ir.Summary.Modules = len(ir.Modules)
	for _, key := range sortedKeys(external) {
		item := external[key]
		sort.Strings(item.Packages)
		sort.Strings(item.ImportedBy)
		ir.ExternalModules = append(ir.ExternalModules, *item)
	}
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// GenerateWorkspaceBundle creates the two canonical workspace artifacts.
func GenerateWorkspaceBundle(root string) (*WorkspaceBundle, error) {
	ir, err := BuildWorkspaceIR(root)
	if err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(ir, "", "  ")
	if err != nil {
		return nil, err
	}
	return &WorkspaceBundle{Manifest: append(manifest, '\n'), Overview: []byte(renderWorkspaceOverview(ir))}, nil
}

func renderWorkspaceOverview(ir *WorkspaceIR) string {
	lines := []string{"flowchart LR", "    classDef entrypoint fill:#fff4cc,stroke:#8a6d00,stroke-width:3px"}
	packageIDs := map[string]string{}
	index := 0
	for moduleIndex, module := range ir.Modules {
		lines = append(lines, fmt.Sprintf("    subgraph module_%d[\"%s\"]", moduleIndex, mermaidDisplayLabel(module.Path)))
		for _, pkg := range module.Packages {
			id := fmt.Sprintf("package_%d", index)
			index++
			packageIDs[pkg.ImportPath] = id
			label := fmt.Sprintf("%s<br/>files=%d declarations=%d exportedFunctions=%d routes=%d", pkg.ImportPath, pkg.SourceFiles, pkg.Declarations, pkg.ExportedFunctions, pkg.FiberRoutes)
			if pkg.Main {
				lines = append(lines, fmt.Sprintf("        %s([\"%s\"])", id, mermaidDisplayLabel(label)), "        class "+id+" entrypoint")
			} else {
				lines = append(lines, fmt.Sprintf("        %s[\"%s\"]", id, mermaidDisplayLabel(label)))
			}
		}
		lines = append(lines, "    end")
	}
	for _, module := range ir.Modules {
		for _, pkg := range module.Packages {
			for _, target := range pkg.LocalImports {
				if packageIDs[target] != "" {
					lines = append(lines, "    "+packageIDs[pkg.ImportPath]+" --> "+packageIDs[target])
				}
			}
		}
	}
	if len(ir.ExternalModules) > 0 {
		lines = append(lines, "    subgraph external_dependencies[\"external modules\"]")
		for i, external := range ir.ExternalModules {
			lines = append(lines, fmt.Sprintf("        external_%d[\"%s\"]", i, mermaidDisplayLabel(external.Path)))
		}
		lines = append(lines, "    end")
		for i, external := range ir.ExternalModules {
			for _, source := range external.ImportedBy {
				lines = append(lines, fmt.Sprintf("    %s -.-> external_%d", packageIDs[source], i))
			}
		}
	}
	routeIndex := 0
	for _, module := range ir.Modules {
		for _, pkg := range module.Packages {
			if len(pkg.Routes) == 0 {
				continue
			}
			entries := []string{}
			limit := len(pkg.Routes)
			if limit > 5 {
				limit = 5
			}
			for _, route := range pkg.Routes[:limit] {
				entries = append(entries, route.Method+" "+route.Path+" -> "+route.Handler)
			}
			if len(pkg.Routes) > limit {
				entries = append(entries, fmt.Sprintf("+%d more", len(pkg.Routes)-limit))
			}
			id := fmt.Sprintf("routes_%d", routeIndex)
			routeIndex++
			lines = append(lines, fmt.Sprintf("    %s[\"%s\"]", id, mermaidDisplayLabel(strings.Join(entries, "<br/>"))), "    "+packageIDs[pkg.ImportPath]+" --- "+id)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

var (
	workspaceWriteFile = os.WriteFile
	workspaceRename    = os.Rename
)

// WriteWorkspaceBundle transactionally replaces exactly manifest.json and overview.mmd.
func WriteWorkspaceBundle(output string, bundle *WorkspaceBundle) error {
	if output == "" {
		return fmt.Errorf("workspace bundle output directory is required")
	}
	output = filepath.Clean(output)
	parent, base := filepath.Dir(output), filepath.Base(output)
	if base == "." || base == string(filepath.Separator) {
		return fmt.Errorf("workspace bundle output directory is invalid")
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	exists := false
	if info, err := os.Lstat(output); err == nil {
		exists = true
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("workspace bundle output path is not a directory")
		}
		if err := validateWorkspaceBundleDirectory(output); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.MkdirTemp(parent, "."+base+".tmp-")
	if err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o755); err != nil {
		_ = os.RemoveAll(temporary)
		return err
	}
	remove := true
	defer func() {
		if remove {
			_ = os.RemoveAll(temporary)
		}
	}()
	files := map[string][]byte{"manifest.json": bundle.Manifest, "overview.mmd": bundle.Overview}
	for _, name := range workspaceBundleFiles {
		if err := workspaceWriteFile(filepath.Join(temporary, name), files[name], 0o644); err != nil {
			return fmt.Errorf("write temporary workspace bundle file %s: %w", name, err)
		}
	}
	if !exists {
		if err := workspaceRename(temporary, output); err != nil {
			return fmt.Errorf("commit workspace bundle: %w", err)
		}
		remove = false
		return nil
	}
	backup := temporary + "-previous"
	if err := workspaceRename(output, backup); err != nil {
		return fmt.Errorf("prepare workspace bundle replacement: %w", err)
	}
	if err := workspaceRename(temporary, output); err != nil {
		if rollbackErr := workspaceRename(backup, output); rollbackErr != nil {
			return fmt.Errorf("commit workspace bundle: %w (rollback failed: %v; previous bundle remains at %s)", err, rollbackErr, backup)
		}
		return fmt.Errorf("commit workspace bundle: %w", err)
	}
	remove = false
	return os.RemoveAll(backup)
}

func validateWorkspaceBundleDirectory(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	expected := map[string]bool{"manifest.json": true, "overview.mmd": true}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			return fmt.Errorf("workspace bundle contains unexpected file: %s", entry.Name())
		}
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("workspace bundle generated path is not a regular file: %s", entry.Name())
		}
	}
	return nil
}

func safeRelative(value string) bool {
	return value != "" && !strings.Contains(value, `\`) && !filepath.IsAbs(value) && path.Clean(value) == value && value != ".." && !strings.HasPrefix(value, "../")
}

// ResolveWorkspaceBundleRoot resolves strict manifest metadata relative to its anchor module.
func ResolveWorkspaceBundleRoot(bundleDirectory string) (string, error) {
	if err := validateWorkspaceBundlePath(bundleDirectory); err != nil {
		return "", err
	}
	content, err := os.ReadFile(filepath.Join(bundleDirectory, "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("read workspace bundle manifest: %w", err)
	}
	if !json.Valid(content) {
		return "", fmt.Errorf("invalid workspace bundle manifest JSON")
	}
	var manifest WorkspaceIR
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return "", fmt.Errorf("invalid workspace bundle manifest: %w", err)
	}
	if manifest.FormatVersion != WorkspaceBundleFormatVersion || !safeRelative(manifest.Workspace.RootDirectory) {
		return "", fmt.Errorf("invalid workspace bundle root metadata")
	}
	candidates := []string{absolutePath(bundleDirectory)}
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		candidates = append(candidates, cwd)
	}
	for _, candidate := range candidates {
		anchor := nearestSourceRoot("go", candidate)
		if anchor == "" {
			continue
		}
		mod, readErr := os.ReadFile(filepath.Join(anchor, "go.mod"))
		if readErr != nil || goModulePath(string(mod)) != manifest.Workspace.AnchorModulePath {
			continue
		}
		resolved := filepath.Clean(filepath.Join(anchor, filepath.FromSlash(manifest.Workspace.RootDirectory)))
		if !pathWithin(anchor, resolved) {
			continue
		}
		realAnchor, e1 := filepath.EvalSymlinks(anchor)
		realResolved, e2 := filepath.EvalSymlinks(resolved)
		if e1 == nil && e2 == nil && pathWithin(realAnchor, realResolved) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("cannot safely self-locate workspace root")
}

// CheckWorkspaceBundle regenerates and byte-compares exactly both canonical files.
func CheckWorkspaceBundle(bundleDirectory, root string) error {
	if err := validateWorkspaceBundlePath(bundleDirectory); err != nil {
		return err
	}
	if root == "" {
		var err error
		root, err = ResolveWorkspaceBundleRoot(bundleDirectory)
		if err != nil {
			return err
		}
	}
	expected, err := GenerateWorkspaceBundle(root)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(bundleDirectory)
	if err != nil {
		return fmt.Errorf("read workspace bundle: %w", err)
	}
	actualNames := map[string]bool{}
	for _, entry := range entries {
		actualNames[entry.Name()] = true
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("workspace bundle entry is not a regular file: %s", entry.Name())
		}
	}
	for _, name := range workspaceBundleFiles {
		if !actualNames[name] {
			return fmt.Errorf("workspace bundle missing required file: %s", name)
		}
	}
	extra := []string{}
	for name := range actualNames {
		if name != "manifest.json" && name != "overview.mmd" {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		return fmt.Errorf("workspace bundle contains unexpected file: %s", extra[0])
	}
	wanted := map[string][]byte{"manifest.json": expected.Manifest, "overview.mmd": expected.Overview}
	for _, name := range workspaceBundleFiles {
		actual, readErr := os.ReadFile(filepath.Join(bundleDirectory, name))
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(actual, wanted[name]) {
			return fmt.Errorf("workspace bundle artifact differs from canonical generated content: %s", name)
		}
	}
	return nil
}

func validateWorkspaceBundlePath(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("select workspace bundle directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("workspace bundle path is not a real directory")
	}
	return nil
}
