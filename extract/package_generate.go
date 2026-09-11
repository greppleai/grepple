package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	codeparser "github.com/greppleai/grepple/parser"
)

// GeneratePackageDiagram discovers the non-test Go files in directory and
// generates a complete, exact schema for the one package declared there.
func GeneratePackageDiagram(directory string) (string, error) {
	sources, err := loadPackageSources(directory)
	if err != nil {
		return "", err
	}
	if err := validateSelectedPackage(sources); err != nil {
		return "", err
	}
	analysis, err := Analyze(sources)
	if err != nil {
		return "", err
	}

	packageIDs := sortedKeys(analysis.GoPackageNames)
	if len(packageIDs) != 1 {
		return "", fmt.Errorf("selected directory must resolve to exactly one Go package; found %d", len(packageIDs))
	}
	packageID := packageIDs[0]
	importPath := analysis.GoPackagePaths[packageID]
	if importPath == "" {
		return "", fmt.Errorf("cannot resolve Go import path for selected directory %s (no enclosing go.mod with a module path)", directory)
	}

	declarations := make(map[string]*Declaration)
	for _, declaration := range analysis.GoDeclarations {
		if declaration.PackageID == packageID {
			declarations[declaration.Name] = declaration
		}
	}
	inferGoImplementations(declarations)

	lines := []string{"classDiagram", "    %% grepple:complete-package " + importPath}
	names := sortedKeys(declarations)
	for _, name := range names {
		rendered, renderErr := renderPackageDeclaration(name, declarations[name], analysis)
		if renderErr != nil {
			return "", renderErr
		}
		lines = append(lines, rendered...)
	}

	functionNames := packageFunctionNames(analysis, packageID)
	for _, name := range functionNames {
		functions := analysis.GoFunctions[packageID+":"+name]
		if len(functions) != 1 {
			return "", fmt.Errorf("exported package function %q is ambiguous", name)
		}
		lines = append(lines, renderPackageFunction(functions[0], importPath)...)
	}
	lines = append(lines, packageRelations(names, functionNames, declarations, analysis, packageID)...)

	diagram := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	return validateGeneratedClass(diagram, sources)
}

func loadPackageSources(directory string) ([]Source, error) {
	absolute := absolutePath(directory)
	entries, err := packageDirectoryEntries(directory, absolute)
	if err != nil {
		return nil, err
	}

	sources, typeScript := []Source{}, false
	for _, entry := range entries {
		name := entry.Name()
		typeScript = typeScript || isPackageTypeScriptEntry(entry)
		if !isPackageGoEntry(entry) {
			continue
		}
		path := filepath.Join(absolute, name)
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, readErr
		}
		sources = append(sources, Source{Path: path, Text: string(content)})
	}
	if len(sources) != 0 {
		return sources, nil
	}
	if typeScript {
		return nil, fmt.Errorf("selected directory is TypeScript-only; package generation requires non-test Go source files: %s", directory)
	}
	return nil, fmt.Errorf("no non-test Go source files found in selected directory: %s", directory)
}

func packageDirectoryEntries(directory, absolute string) ([]os.DirEntry, error) {
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, fmt.Errorf("select Go package directory %s: %w", directory, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("Go package selector must be an unambiguous directory: %s", directory)
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return nil, fmt.Errorf("read Go package directory %s: %w", directory, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func isPackageGoEntry(entry os.DirEntry) bool {
	name := entry.Name()
	return !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

func isPackageTypeScriptEntry(entry os.DirEntry) bool {
	return !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 && isTypeScriptPackageFile(entry.Name())
}

func validateSelectedPackage(sources []Source) error {
	packages := map[string]bool{}
	for _, source := range sources {
		document, err := codeparser.ParseDocument("go", source.Text)
		if err != nil {
			return err
		}
		root := document.Root()
		if root.HasError() {
			document.Close()
			return malformedSourceError(source.Path, root)
		}
		name := goPackageName(root, []byte(source.Text))
		document.Close()
		if name == "" {
			return fmt.Errorf("no Go package declaration found in %s", source.Path)
		}
		packages[name] = true
	}
	if len(packages) != 1 {
		return fmt.Errorf("selected directory must resolve to exactly one Go package; found packages: %s", strings.Join(sortedKeys(packages), ", "))
	}
	return nil
}

func isTypeScriptPackageFile(name string) bool {
	for _, extension := range typeScriptExtensions {
		if strings.HasSuffix(name, extension) && !isTypeScriptDeclaration(name) {
			return true
		}
	}
	return false
}

func renderPackageDeclaration(name string, declaration *Declaration, analysis *Analysis) ([]string, error) {
	lines, err := renderClass(name, declaration, analysis)
	if err != nil {
		return nil, err
	}
	lines = lines[:len(lines)-1]
	if declaration.Kind == "struct" || declaration.Kind == "interface" {
		lines = append(lines, "    <<exact>> "+name)
	}
	for _, route := range sortedPackageRoutes(analysis.GoFiberRoutes, declaration.PackageID, name) {
		lines = append(lines, fmt.Sprintf("    %%%% grepple:route %s %s %s", route.Method, route.Path, route.Handler))
	}
	return append(lines, ""), nil
}

func sortedPackageRoutes(routes []FiberRoute, packageID, owner string) []FiberRoute {
	result := make([]FiberRoute, 0)
	prefix := owner + "."
	seen := map[string]bool{}
	for _, route := range routes {
		key := route.Method + "\x00" + route.Path + "\x00" + route.Handler
		if route.PackageID == packageID && strings.HasPrefix(route.Handler, prefix) && !seen[key] {
			seen[key] = true
			result = append(result, route)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := result[i].Method + "\x00" + result[i].Path + "\x00" + result[i].Handler
		right := result[j].Method + "\x00" + result[j].Path + "\x00" + result[j].Handler
		return left < right
	})
	return result
}

func packageFunctionNames(analysis *Analysis, packageID string) []string {
	set := map[string]bool{}
	for key, functions := range analysis.GoFunctions {
		if !strings.HasPrefix(key, packageID+":") {
			continue
		}
		for _, function := range functions {
			if exportedGoName(function.Name) {
				set[function.Name] = true
			}
		}
	}
	return sortedKeys(set)
}

func renderPackageFunction(function Member, importPath string) []string {
	member := renderMember(function)
	lines := []string{"    class " + function.Name + " {", "        " + member, "    }", "    <<function>> " + function.Name, "    <<go>> " + function.Name}
	if function.Location.Line > 0 {
		lines = append(lines, "    note for "+function.Name+" \"defined: "+mermaidNoteText(sourceRange(function.Location))+"\"")
	}
	if exportedGoName(function.Name) {
		lines = append(lines, "    <<export>> "+function.Name)
	}
	lines = append(lines, "    %% grepple:package "+function.Name+" "+importPath)
	if function.File != "" {
		lines = append(lines, "    %% grepple:file "+function.Name+" "+function.File)
	}
	return append(lines, "")
}

func packageRelations(typeNames, functionNames []string, declarations map[string]*Declaration, analysis *Analysis, packageID string) []string {
	chosen := stringSet(typeNames)
	relations := map[string]bool{}
	for _, owner := range typeNames {
		declaration := declarations[owner]
		addHeritageRelations(owner, declaration, chosen, relations)
		addMemberRelations(owner, declaration.Members, typeNames, declaration.Extends, declaration.Implements, relations)
	}
	for _, owner := range functionNames {
		functions := analysis.GoFunctions[packageID+":"+owner]
		if len(functions) == 1 {
			addMemberRelations(owner, []Member{functions[0]}, typeNames, nil, nil, relations)
		}
	}
	return sortedKeys(relations)
}

func addMemberRelations(owner string, members []Member, targets []string, extends, implements map[string]bool, relations map[string]bool) {
	for _, target := range targets {
		if owner == target || extends[target] || implements[target] {
			continue
		}
		found, many := memberReferenceCardinality(members, target)
		if found {
			relations[associationRelation(owner, target, many)] = true
		}
	}
}

func memberReferenceCardinality(members []Member, target string) (bool, bool) {
	found, many := false, false
	for _, member := range members {
		for _, memberType := range append([]string{member.Type}, member.Parameters...) {
			if !typeReferences(memberType, target) {
				continue
			}
			found = true
			many = many || collectionReferences(memberType, target)
		}
	}
	return found, many
}
