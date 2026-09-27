package analysis

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/greppleai/grepple/internal/navigation"
	"github.com/greppleai/grepple/internal/parser"
)

// GraphSchema identifies the normalized navigation graph contract.
const GraphSchema = "grepple-navigation-graph-v7"

// GraphReport is the complete normalized graph or one deterministic queried projection.
type GraphReport struct {
	Schema           string                               `json:"schema"`
	Files            int                                  `json:"files"`
	Sources          SourceSummary                        `json:"sources"`
	Declarations     []parser.NavigationDeclaration       `json:"declarations"`
	TypeDeclarations []parser.NavigationTypeDeclaration   `json:"typeDeclarations,omitempty"`
	Imports          []parser.NavigationImport            `json:"imports,omitempty"`
	Calls            []parser.NavigationCall              `json:"calls"`
	Exports          []parser.NavigationExport            `json:"exports,omitempty"`
	Fields           []parser.NavigationField             `json:"fields,omitempty"`
	Resolution       navigation.NavigationResolutionStats `json:"resolution"`
	TypeUsages       []parser.NavigationTypeUsage         `json:"typeUsages,omitempty"`
	MemberAccesses   []parser.NavigationMemberAccess      `json:"memberAccesses,omitempty"`
	RepositoryRoots  []string                             `json:"repositoryRoots,omitempty"`
	Query            *GraphQuery                          `json:"query,omitempty"`
	Truncation       *Truncation                          `json:"truncation,omitempty"`
}

// GraphQuery records the normalized traversal that produced a graph report.
type GraphQuery struct {
	Direction    string   `json:"direction"`
	Depth        int      `json:"depth"`
	Symbol       string   `json:"symbol,omitempty"`
	At           string   `json:"at,omitempty"`
	Package      string   `json:"package,omitempty"`
	Module       string   `json:"module,omitempty"`
	RootPath     string   `json:"rootPath,omitempty"`
	RootIDs      []string `json:"rootIds,omitempty"`
	Languages    []string `json:"languages,omitempty"`
	Confidences  []string `json:"confidences,omitempty"`
	Visibilities []string `json:"visibilities,omitempty"`
}

// BuildGraph returns a complete graph or applies the optional validated query.
func BuildGraph(universe *Universe, request *GraphQuery) (GraphReport, error) {
	if universe == nil {
		return GraphReport{}, fmt.Errorf("analysis universe is required")
	}
	graph := universe.Graph()
	report := graphReport(graph, universe)
	if request == nil {
		return report, nil
	}
	if err := validateGraphQuery(*request); err != nil {
		return GraphReport{}, err
	}
	filter, err := navigation.NormalizeNavigationGraphFilter(navigation.NavigationGraphFilter{Languages: request.Languages, Confidences: request.Confidences, Visibilities: request.Visibilities})
	if err != nil {
		return GraphReport{}, err
	}
	filtered, err := navigation.FilterNavigationGraph(graph, filter)
	if err != nil {
		return GraphReport{}, err
	}
	roots, err := selectGraphRoots(filtered.Declarations, *request)
	if err != nil {
		return GraphReport{}, err
	}
	rootIDs := make([]string, 0, len(roots))
	for _, root := range roots {
		rootIDs = append(rootIDs, root.ID)
	}
	queried, err := navigation.QueryNavigationGraph(filtered, rootIDs, navigation.NavigationQueryDirection(request.Direction), request.Depth)
	if err != nil {
		return GraphReport{}, err
	}
	report = graphReport(queried, universe)
	report.Query = &GraphQuery{Direction: request.Direction, Depth: request.Depth, RootIDs: rootIDs, Languages: filter.Languages, Confidences: filter.Confidences, Visibilities: filter.Visibilities}
	return report, nil
}

func graphReport(graph parser.NavigationGraph, universe *Universe) GraphReport {
	declarations := graph.Declarations
	if declarations == nil {
		declarations = []parser.NavigationDeclaration{}
	}
	calls := graph.Calls
	if calls == nil {
		calls = []parser.NavigationCall{}
	}
	return GraphReport{Schema: GraphSchema, Files: universe.summary.Selected, Sources: universe.Summary(), Declarations: declarations, TypeDeclarations: graph.TypeDeclarations, Imports: graph.Imports, Calls: calls, Exports: graph.Exports, Fields: graph.Fields, Resolution: navigation.MeasureNavigationResolution(graph), TypeUsages: graph.TypeUsages, MemberAccesses: graph.MemberAccesses, RepositoryRoots: graph.RepositoryRoots, Truncation: universe.Truncation()}
}

func validateGraphQuery(request GraphQuery) error {
	direction := navigation.NavigationQueryDirection(request.Direction)
	switch direction {
	case navigation.NavigationQueryCallers, navigation.NavigationQueryCallees, navigation.NavigationQueryDependencies, navigation.NavigationQueryDependents, navigation.NavigationQueryImpact:
	default:
		return fmt.Errorf("unsupported graph direction %q", request.Direction)
	}
	if request.Depth < 1 || request.Depth > 10 {
		return fmt.Errorf("graph depth must be between 1 and 10")
	}
	selectors := 0
	for _, value := range []string{request.Symbol, request.At, request.Package, request.Module, request.RootPath} {
		if value != "" {
			selectors++
		}
	}
	if selectors != 1 {
		return fmt.Errorf("graph query requires exactly one root selector")
	}
	return nil
}

func selectGraphRoots(declarations []parser.NavigationDeclaration, request GraphQuery) ([]parser.NavigationDeclaration, error) {
	if request.Symbol != "" || request.At != "" {
		root, err := selectSingleGraphRoot(declarations, request.Symbol, request.At)
		if err != nil {
			return nil, err
		}
		return []parser.NavigationDeclaration{root}, nil
	}
	matches := make([]parser.NavigationDeclaration, 0)
	for _, declaration := range declarations {
		matched := false
		switch {
		case request.Package != "":
			matched = declaration.Package == request.Package || declaration.PackageID == request.Package
		case request.Module != "":
			matched = declaration.ModuleID == request.Module
		default:
			root := cleanPath(request.RootPath)
			path := cleanPath(declaration.Path)
			matched = root == "." || path == root || strings.HasPrefix(path, root+"/")
		}
		if matched {
			matches = append(matches, declaration)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no navigation declarations match graph root")
	}
	return matches, nil
}

//revive:disable-next-line:cognitive-complexity
func selectSingleGraphRoot(declarations []parser.NavigationDeclaration, symbol, at string) (parser.NavigationDeclaration, error) {
	matches := make([]parser.NavigationDeclaration, 0)
	selector := "symbol " + strconv.Quote(symbol)
	if symbol != "" {
		for _, declaration := range declarations {
			if declaration.Name == symbol {
				matches = append(matches, declaration)
			}
		}
	} else {
		path, line, err := parseAt(at)
		if err != nil {
			return parser.NavigationDeclaration{}, err
		}
		selector = "location " + at
		for _, declaration := range declarations {
			if cleanPath(declaration.Path) == cleanPath(path) && line >= declaration.Start && line <= declaration.End {
				matches = append(matches, declaration)
			}
		}
		if len(matches) > 1 {
			width := matches[0].End - matches[0].Start
			for _, match := range matches[1:] {
				if match.End-match.Start < width {
					width = match.End - match.Start
				}
			}
			narrowed := matches[:0]
			for _, match := range matches {
				if match.End-match.Start == width {
					narrowed = append(narrowed, match)
				}
			}
			matches = narrowed
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return parser.NavigationDeclaration{}, fmt.Errorf("no navigation declaration matches %s", selector)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].ID < matches[j].ID })
	locations := make([]string, 0, len(matches))
	for _, match := range matches {
		locations = append(locations, fmt.Sprintf("--at %s:%d", match.Path, match.Start))
	}
	return parser.NavigationDeclaration{}, fmt.Errorf("navigation declaration %s is ambiguous; try %s", selector, strings.Join(locations, " or "))
}

func parseAt(value string) (string, int, error) {
	separator := strings.LastIndex(value, ":")
	if separator <= 0 {
		return "", 0, fmt.Errorf("location requires PATH:LINE")
	}
	lineText := value[separator+1:]
	if dash := strings.IndexByte(lineText, '-'); dash >= 0 {
		lineText = lineText[:dash]
	}
	line, err := strconv.Atoi(lineText)
	if err != nil || line < 1 {
		return "", 0, fmt.Errorf("location requires a positive line")
	}
	return value[:separator], line, nil
}

func cleanPath(value string) string {
	cleaned := filepath.Clean(value)
	if absolute, err := filepath.Abs(cleaned); err == nil {
		if workingDirectory, cwdErr := filepath.Abs("."); cwdErr == nil {
			if relative, relativeErr := filepath.Rel(workingDirectory, absolute); relativeErr == nil {
				cleaned = relative
			}
		}
	}
	cleaned = filepath.ToSlash(cleaned)
	cleaned = strings.TrimPrefix(cleaned, "./")
	if cleaned == "" {
		return "."
	}
	return cleaned
}
