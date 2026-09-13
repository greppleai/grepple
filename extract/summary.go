package extract

import (
	"fmt"
	"strings"
)

const (
	maxPackageSummarySurfaceItems = 30
	maxPackageSummaryRoutes       = 10
	maxWorkspaceSummaryModules    = 10
	maxWorkspaceSummaryPackages   = 40
	maxWorkspaceSummaryEdges      = 40
)

// GeneratePackageSummary builds a bounded Markdown orientation summary from the
// same canonical package IR used by package bundles.
func GeneratePackageSummary(directory string) (string, error) {
	ir, _, err := BuildPackageIR(directory)
	if err != nil {
		return "", err
	}
	return renderPackageSummary(ir), nil
}

// GenerateWorkspaceSummary builds a bounded Markdown orientation summary from
// the same canonical workspace IR used by workspace bundles.
func GenerateWorkspaceSummary(root string) (string, error) {
	ir, err := BuildWorkspaceIR(root)
	if err != nil {
		return "", err
	}
	return renderWorkspaceSummary(ir), nil
}

func renderPackageSummary(ir *PackageIR) string {
	identity := ir.Package.ImportPath
	if identity == "" {
		identity = ir.Package.Name
	}
	lines := []string{
		fmt.Sprintf("# Package `%s`", identity),
		"",
	}
	if ir.Package.Documentation != "" {
		lines = append(lines, ir.Package.Documentation, "")
	}
	lines = append(lines,
		fmt.Sprintf("- Source: `%s`", ir.Package.SourceDirectory),
		fmt.Sprintf("- Shape: %d files, %d declarations (%d exported, %d internal), %d members, %d exported functions", ir.Summary.SourceFiles, ir.Summary.Declarations, ir.Summary.ExportedTypes, ir.Summary.InternalTypes, ir.Summary.Members, ir.Summary.ExportedFunctions),
		fmt.Sprintf("- Integration: %d routes, %d evidenced relations", ir.Summary.FiberRoutes, ir.Summary.Relations),
	)
	lines = appendBoundedMarkdownSection(lines, "Public surface", packageSummarySurfaceItems(ir), maxPackageSummarySurfaceItems)
	lines = appendBoundedMarkdownSection(lines, "Routes", packageSummaryRouteItems(ir), maxPackageSummaryRoutes)
	return strings.Join(lines, "\n") + "\n"
}

func packageSummarySurfaceItems(ir *PackageIR) []string {
	items := make([]string, 0, ir.Summary.ExportedTypes+len(ir.ExportedFunctions))
	for _, declaration := range ir.Declarations {
		if exportedGoName(declaration.Name) {
			items = append(items, fmt.Sprintf("`%s` (%s) — `%s`", declaration.Name, declaration.Kind, summaryLocation(declaration.File, declaration.Line)))
		}
	}
	for _, function := range ir.ExportedFunctions {
		signature := function.Name + "(" + strings.Join(function.Parameters, ", ") + ")"
		if function.Result != "" {
			signature += " " + function.Result
		}
		items = append(items, fmt.Sprintf("`%s` (function) — `%s`", signature, summaryLocation(function.File, function.Line)))
	}
	return items
}

func packageSummaryRouteItems(ir *PackageIR) []string {
	items := make([]string, 0, len(ir.Routes))
	for _, route := range ir.Routes {
		items = append(items, fmt.Sprintf("`%s %s` → `%s` — `%s`", route.Method, route.Path, route.Handler, summaryLocation(route.File, route.Line)))
	}
	return items
}

func renderWorkspaceSummary(ir *WorkspaceIR) string {
	identity := ir.Workspace.AnchorModulePath
	if identity == "" {
		identity = ir.Workspace.RootDirectory
	}
	lines := []string{
		fmt.Sprintf("# Workspace `%s`", identity),
		"",
		fmt.Sprintf("- Root: `%s`", ir.Workspace.RootDirectory),
		fmt.Sprintf("- Shape: %d modules, %d packages, %d files, %d declarations, %d exported functions", ir.Summary.Modules, ir.Summary.Packages, ir.Summary.SourceFiles, ir.Summary.Declarations, ir.Summary.ExportedFunctions),
		fmt.Sprintf("- Integration: %d local imports, %d external-module edges, %d routes", ir.Summary.LocalImports, ir.Summary.ExternalModuleEdges, ir.Summary.FiberRoutes),
	}
	lines = appendBoundedMarkdownSection(lines, "Modules", workspaceSummaryModuleItems(ir), maxWorkspaceSummaryModules)
	lines = appendBoundedMarkdownSection(lines, "Packages", workspaceSummaryPackageItems(ir), maxWorkspaceSummaryPackages)
	lines = appendBoundedMarkdownSection(lines, "Local dependencies", workspaceSummaryDependencyItems(ir), maxWorkspaceSummaryEdges)
	return strings.Join(lines, "\n") + "\n"
}

func workspaceSummaryModuleItems(ir *WorkspaceIR) []string {
	items := make([]string, 0, len(ir.Modules))
	for _, module := range ir.Modules {
		items = append(items, fmt.Sprintf("`%s` — `%s` (%d packages)", module.Path, module.Directory, len(module.Packages)))
	}
	return items
}

func workspaceSummaryPackageItems(ir *WorkspaceIR) []string {
	items := make([]string, 0, ir.Summary.Packages)
	for _, module := range ir.Modules {
		for _, pkg := range module.Packages {
			details := fmt.Sprintf("%d files, %d declarations, %d exported functions", pkg.SourceFiles, pkg.Declarations, pkg.ExportedFunctions)
			if pkg.FiberRoutes > 0 {
				details += fmt.Sprintf(", %d routes", pkg.FiberRoutes)
			}
			items = append(items, fmt.Sprintf("`%s` — package `%s`; %s", pkg.Directory, pkg.Name, details))
		}
	}
	return items
}

func workspaceSummaryDependencyItems(ir *WorkspaceIR) []string {
	directories := workspacePackageDirectories(ir)
	items := make([]string, 0, ir.Summary.LocalImports)
	for _, module := range ir.Modules {
		for _, pkg := range module.Packages {
			for _, dependency := range pkg.LocalImports {
				items = append(items, fmt.Sprintf("`%s` → `%s`", pkg.Directory, directories[dependency]))
			}
		}
	}
	return items
}

func workspacePackageDirectories(ir *WorkspaceIR) map[string]string {
	directories := make(map[string]string, ir.Summary.Packages)
	for _, module := range ir.Modules {
		for _, pkg := range module.Packages {
			directories[pkg.ImportPath] = pkg.Directory
		}
	}
	return directories
}

func appendBoundedMarkdownSection(lines []string, title string, items []string, limit int) []string {
	if len(items) == 0 {
		return lines
	}
	lines = append(lines, "", "## "+title, "")
	count := len(items)
	if count > limit {
		count = limit
	}
	for _, item := range items[:count] {
		lines = append(lines, "- "+item)
	}
	if omitted := len(items) - count; omitted > 0 {
		lines = append(lines, fmt.Sprintf("- _… %d additional entries omitted; use the canonical overview or manifest for complete detail._", omitted))
	}
	return lines
}

func summaryLocation(path string, line int) string {
	if line > 0 {
		return fmt.Sprintf("%s:%d", path, line)
	}
	return path
}
