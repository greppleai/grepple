package navigation

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/greppleai/grepple/api"
	"golang.org/x/mod/modfile"
)

// QualifyExternalDependencies adds exact local manifest evidence to unresolved
// dependency references. References without an exact version remain unchanged.
func QualifyExternalDependencies(results []api.FileResult, workingDirectory string) error {
	if workingDirectory == "" {
		workingDirectory, _ = os.Getwd()
	}
	modules := make(map[string]goDependencyModule)
	for resultIndex := range results {
		if results[resultIndex].Repo != "" {
			continue
		}
		path := results[resultIndex].Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDirectory, path)
		}
		modulePath, ok := nearestFile(filepath.Dir(path), "go.mod")
		if !ok {
			continue
		}
		module, exists := modules[modulePath]
		if !exists {
			parsed, err := readGoDependencyModule(modulePath)
			if err != nil {
				return err
			}
			module = parsed
			modules[modulePath] = module
		}
		qualifyDependencySymbols(results[resultIndex].Related, module.dependencies)
	}
	return nil
}

type goDependency struct {
	module, sourceModule, version, integrity string
}

type goDependencyModule struct {
	dependencies []goDependency
}

func readGoDependencyModule(path string) (goDependencyModule, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return goDependencyModule{}, err
	}
	parsed, err := modfile.Parse(path, content, nil)
	if err != nil {
		return goDependencyModule{}, err
	}
	integrities, err := readGoDependencySums(filepath.Join(filepath.Dir(path), "go.sum"))
	if err != nil {
		return goDependencyModule{}, err
	}
	dependencies := make([]goDependency, 0, len(parsed.Require))
	for _, required := range parsed.Require {
		dependency := goDependency{module: required.Mod.Path, sourceModule: required.Mod.Path, version: required.Mod.Version, integrity: integrities[required.Mod.Path+"@"+required.Mod.Version]}
		applyGoDependencyReplacements(&dependency, parsed.Replace)
		dependencies = append(dependencies, dependency)
	}
	if workPath, ok := nearestFile(filepath.Dir(path), "go.work"); ok {
		workContent, readErr := os.ReadFile(workPath)
		if readErr != nil {
			return goDependencyModule{}, readErr
		}
		work, parseErr := modfile.ParseWork(workPath, workContent, nil)
		if parseErr != nil {
			return goDependencyModule{}, parseErr
		}
		for index := range dependencies {
			applyGoDependencyReplacements(&dependencies[index], work.Replace)
		}
	}
	return goDependencyModule{dependencies: dependencies}, nil
}

func readGoDependencySums(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	integrities := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && !strings.HasSuffix(fields[1], "/go.mod") {
			integrities[fields[0]+"@"+fields[1]] = fields[2]
		}
	}
	return integrities, nil
}

func applyGoDependencyReplacements(dependency *goDependency, replacements []*modfile.Replace) {
	for _, replacement := range replacements {
		if replacement.Old.Path != dependency.module || replacement.Old.Version != "" && replacement.Old.Version != dependency.version {
			continue
		}
		dependency.integrity = ""
		if replacement.New.Version == "" {
			dependency.version = ""
			continue
		}
		dependency.sourceModule = replacement.New.Path
		dependency.version = replacement.New.Version
	}
}

func qualifyDependencySymbols(symbols []api.RelatedSymbol, dependencies []goDependency) {
	for index := range symbols {
		if reference := symbols[index].External; reference != nil && reference.Language == "go" {
			if dependency, ok := dependencyForImport(reference.ImportPath, dependencies); ok {
				reference.Module = dependency.sourceModule
				reference.Package = dependency.sourceModule + strings.TrimPrefix(reference.ImportPath, dependency.module)
				reference.Version = dependency.version
				reference.Integrity = dependency.integrity
			}
		}
		qualifyDependencySymbols(symbols[index].Related, dependencies)
	}
}

func dependencyForImport(importPath string, dependencies []goDependency) (goDependency, bool) {
	best := goDependency{}
	for _, dependency := range dependencies {
		if importPath != dependency.module && !strings.HasPrefix(importPath, dependency.module+"/") {
			continue
		}
		if len(dependency.module) > len(best.module) {
			best = dependency
		}
	}
	return best, best.module != "" && best.version != ""
}

func nearestFile(directory, name string) (string, bool) {
	for {
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", false
		}
		directory = parent
	}
}

// ExternalDependencyReferences returns deterministic exact references eligible
// for a server artifact lookup.
func ExternalDependencyReferences(results []api.FileResult) []api.ExternalNavigationReference {
	byID := make(map[string]api.ExternalNavigationReference)
	var collect func([]api.RelatedSymbol)
	collect = func(symbols []api.RelatedSymbol) {
		for _, symbol := range symbols {
			if reference := symbol.External; reference != nil && reference.ID != "" && reference.Module != "" && reference.Version != "" {
				byID[reference.ID] = *reference
			}
			collect(symbol.Related)
		}
	}
	for _, result := range results {
		collect(result.Related)
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	references := make([]api.ExternalNavigationReference, 0, len(ids))
	for _, id := range ids {
		references = append(references, byID[id])
	}
	return references
}

// ApplyExternalDependencyResolution replaces unresolved references with exact
// symbols returned by a navigation artifact server.
func ApplyExternalDependencyResolution(results []api.FileResult, response api.NavigationResolveResponse) []api.FileResult {
	resolved := make(map[string][]api.RelatedSymbol, len(response.Results))
	for _, result := range response.Results {
		resolved[result.ID] = append(resolved[result.ID], result.Symbols...)
	}
	for index := range results {
		results[index].Related = applyExternalDependencySymbols(results[index].Related, resolved)
	}
	return results
}

func applyExternalDependencySymbols(symbols []api.RelatedSymbol, resolved map[string][]api.RelatedSymbol) []api.RelatedSymbol {
	result := make([]api.RelatedSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		if symbol.External != nil && len(resolved[symbol.External.ID]) > 0 {
			for _, replacement := range resolved[symbol.External.ID] {
				replacement.CallLine = symbol.CallLine
				replacement.Direction = symbol.Direction
				replacement.Role = symbol.Role
				result = append(result, replacement)
			}
			continue
		}
		symbol.Related = applyExternalDependencySymbols(symbol.Related, resolved)
		result = append(result, symbol)
	}
	return result
}
