// Package architecture implements source-linked directory architecture commands.
package architecture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/sourcekind"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const directoryArchitectureSchema = "grepple-directory-architecture-v5"

type architectureArgs struct {
	JSON bool `arg:"--json" help:"emit complete directory architecture JSON"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	Compact        bool     `arg:"--compact" help:"emit a bounded agent-facing directory summary"`
	Depth          int      `arg:"--depth" placeholder:"N" help:"maximum displayed directory depth (default 3; 0 = unlimited)"`
	MaxNodes       int      `arg:"--max-nodes" placeholder:"N" help:"maximum directories in compact output (default 200; 0 = unlimited)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

type architectureResolveArgs struct {
	JSON bool `arg:"--json" help:"emit complete matching declarations as JSON"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	Compact        bool     `arg:"--compact" help:"emit bounded source-linked matches"`
	Symbol         string   `arg:"--symbol,required" placeholder:"NAME" help:"exact or terminal declaration name"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
}

type architectureWhyArgs struct {
	JSON bool `arg:"--json" help:"emit complete source-linked relation evidence as JSON"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	Compact        bool     `arg:"--compact" help:"emit bounded source-linked relation evidence"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	From           string   `arg:"positional,required" placeholder:"FROM"`
	To             string   `arg:"positional,required" placeholder:"TO"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
}

// Report is a complete source-linked directory architecture.
type Report struct {
	Schema           string                       `json:"schema"`
	Root             string                       `json:"root"`
	Files            int                          `json:"files"`
	Sources          SourceSummary                `json:"sources"`
	SourceFiles      []architectureSourceFile     `json:"sourceFiles"`
	Directories      []architectureDirectory      `json:"directories"`
	Symbols          []architectureSymbol         `json:"symbols"`
	Relations        []architectureRelation       `json:"relations"`
	RepositoryRoots  []string                     `json:"repositoryRoots,omitempty"`
	RelationCoverage architectureRelationCoverage `json:"relationCoverage"`
	Truncation       *Truncation                  `json:"truncation,omitempty"`
}

type architectureSourceFile struct {
	Path           string `json:"path"`
	Language       string `json:"language"`
	Classification string `json:"classification"`
}

type architectureDirectory struct {
	Path            string              `json:"path"`
	Files           int                 `json:"files"`
	Classifications []architectureCount `json:"classifications"`
	Languages       []architectureCount `json:"languages"`
	Declarations    []architectureCount `json:"declarations"`
	PublicCallables int                 `json:"publicCallables"`
	Entrypoints     int                 `json:"entrypoints"`
}

type architectureCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type architectureSymbol struct {
	Name           string                      `json:"name"`
	Kind           string                      `json:"kind"`
	Language       string                      `json:"language"`
	Classification string                      `json:"classification"`
	Path           string                      `json:"path"`
	Directory      string                      `json:"directory"`
	Container      string                      `json:"container,omitempty"`
	Visibility     parser.NavigationVisibility `json:"visibility,omitempty"`
	Entrypoint     string                      `json:"entrypoint,omitempty"`
	Start          int                         `json:"startLine"`
	End            int                         `json:"endLine"`
}

type architectureRelation struct {
	From            string                         `json:"from"`
	To              string                         `json:"to"`
	Kind            string                         `json:"kind"`
	Count           int                            `json:"count"`
	Classifications []architectureCount            `json:"classifications"`
	Evidence        []architectureRelationEvidence `json:"evidence"`
}

type architectureRelationEvidence struct {
	Path           string `json:"path"`
	Line           int    `json:"line"`
	Caller         string `json:"caller"`
	Target         string `json:"target"`
	Kind           string `json:"kind"`
	Classification string `json:"classification"`
	Confidence     string `json:"confidence"`
	ImportPath     string `json:"importPath,omitempty"`
	Role           string `json:"role,omitempty"`
}

type architectureRelationCoverage struct {
	ImportFacts                int      `json:"importFacts"`
	ResolvedImports            int      `json:"resolvedImports"`
	AmbiguousImports           int      `json:"ambiguousImports"`
	UnresolvedImports          int      `json:"unresolvedImports"`
	TypeReferences             int      `json:"typeReferences"`
	ResolvedTypeReferences     int      `json:"resolvedTypeReferences"`
	AmbiguousTypeReferences    int      `json:"ambiguousTypeReferences"`
	UnresolvedTypeReferences   int      `json:"unresolvedTypeReferences"`
	UnqualifiedTypeReferences  int      `json:"unqualifiedTypeReferences"`
	UnsupportedImportLanguages []string `json:"unsupportedImportLanguages"`
}

type architectureResolveOutput struct {
	Schema  string               `json:"schema"`
	Symbol  string               `json:"symbol"`
	Sources SourceSummary        `json:"sources"`
	Matches []architectureSymbol `json:"matches"`
}

type architectureWhyOutput struct {
	Schema   string                         `json:"schema"`
	From     string                         `json:"from"`
	To       string                         `json:"to"`
	Relation string                         `json:"relation"`
	Sources  SourceSummary                  `json:"sources"`
	Evidence []architectureRelationEvidence `json:"evidence"`
}

// ParsedSource owns one parsed architecture source document.
type ParsedSource struct {
	Path     string
	Document *parser.Document
	Outline  parser.FileOutline
}

// LoadDocuments parses source paths for architecture and reusable research analysis.
func LoadDocuments(paths []string) ([]ParsedSource, search.NavigationSourceStats) {
	stats := search.NavigationSourceStats{Attempted: len(paths)}
	result := make([]ParsedSource, 0, len(paths))
	for _, sourcePath := range paths {
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			stats.Failed++
			continue
		}
		if bytes.IndexByte(content, 0) >= 0 {
			stats.Skipped++
			continue
		}
		document, err := parser.ParseDocument(parser.LanguageFor(sourcePath), string(content))
		if err != nil {
			stats.Failed++
			continue
		}
		result = append(result, ParsedSource{Path: sourcePath, Document: document, Outline: parser.OutlineFromDocument(sourcePath, document)})
	}
	return result, stats
}

// NavigationDocuments projects parsed architecture sources for graph construction.
func NavigationDocuments(sources []ParsedSource) []search.NavigationDocumentSource {
	result := make([]search.NavigationDocumentSource, 0, len(sources))
	for _, source := range sources {
		result = append(result, search.NavigationDocumentSource{Path: source.Path, Document: source.Document})
	}
	return result
}

type directoryAccumulator struct {
	files           int
	classifications map[string]int
	languages       map[string]int
	declarations    map[string]int
	public          int
	entrypoints     int
}

// Run executes architecture commands. Deprecated: construct the command with New.
func Run(args []string, dependencies Dependencies) error { return New(dependencies).Run(args) }

// Run executes the architecture command family.
func (command *command) Run(args []string) error {
	if len(args) == 0 || isExtractHelp(args[0]) {
		return stdoutWriter().writeString("Inspect language-neutral directory architecture. Local checkout is the default; --repo selects one exact indexed remote repository for directory, resolve, why, or responsibilities. Compare reads two local JSON snapshots.\nUsage:\n  grepple architecture directory (--compact | --json) [PATH ...]\n  grepple architecture resolve --symbol NAME (--compact | --json) [PATH ...]\n  grepple architecture why FROM TO (--compact | --json) [PATH ...]\n  grepple architecture responsibilities (--compact | --json) [PATH ...]\n  grepple architecture compare (--compact | --json) BEFORE.json AFTER.json\n")
	}
	switch args[0] {
	case "directory":
		return command.runDirectory(args[1:])
	case "resolve":
		return command.runResolve(args[1:])
	case "why":
		return command.runWhy(args[1:])
	case "responsibilities":
		return command.runResponsibilities(args[1:])
	case "compare":
		return command.runCompare(args[1:])
	default:
		return fmt.Errorf("unknown architecture command %q", args[0])
	}
}

func (command *command) runDirectory(args []string) error {
	return runArchitectureDirectory(args, command.dependencies)
}
func (command *command) runResolve(args []string) error {
	return runArchitectureResolve(args, command.dependencies)
}
func (command *command) runWhy(args []string) error {
	return runArchitectureWhy(args, command.dependencies)
}
func (command *command) runResponsibilities(args []string) error {
	return runArchitectureResponsibilities(args, command.dependencies)
}
func (command *command) runCompare(args []string) error {
	return runArchitectureCompare(args, command.dependencies)
}

var errArchitectureHelp = errors.New("architecture help displayed")

func runArchitectureDirectory(args []string, dependencies Dependencies) error {
	values := architectureArgs{Depth: 3, MaxNodes: 200, MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture directory", args, &values); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	if values.JSON == values.Compact {
		return fmt.Errorf("architecture directory requires exactly one of --json or --compact")
	}
	if values.Depth < 0 || values.MaxNodes < 0 || values.MaxFiles < 0 || values.MaxOutputBytes < 0 {
		return fmt.Errorf("architecture limits must be non-negative")
	}
	architecture, remote, err := loadDirectoryArchitecture(context.Background(), values.Paths, values.MaxFiles, values.Repository, values.Server, dependencies)
	if err != nil {
		return err
	}
	if values.JSON {
		if remote != nil {
			return stdoutWriter().writeJSON(remote)
		}
		return stdoutWriter().writeJSON(architecture)
	}
	return renderDirectoryArchitecture(architecture, values)
}

func runArchitectureResolve(args []string, dependencies Dependencies) error {
	values := architectureResolveArgs{MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture resolve", args, &values); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	if values.JSON == values.Compact {
		return fmt.Errorf("architecture resolve requires exactly one of --json or --compact")
	}
	if err := validateArchitectureOutputLimits(values.MaxFiles, values.MaxOutputBytes); err != nil {
		return err
	}
	architecture, remote, err := loadDirectoryArchitecture(context.Background(), values.Paths, values.MaxFiles, values.Repository, values.Server, dependencies)
	if err != nil {
		return err
	}
	matches := resolveArchitectureSymbols(architecture.Symbols, values.Symbol)
	output := architectureResolveOutput{Schema: "grepple-architecture-resolve-v1", Symbol: values.Symbol, Sources: architecture.Sources, Matches: matches}
	if values.JSON {
		if err := writeArchitectureProjection(remote, output); err != nil {
			return err
		}
	} else if err := renderArchitectureResolve(output, values.MaxOutputBytes); err != nil {
		return err
	}
	if len(matches) == 0 {
		dependencies.requestExit(1)
	}
	return nil
}

func runArchitectureWhy(args []string, dependencies Dependencies) error {
	values := architectureWhyArgs{MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture why", args, &values); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	if values.JSON == values.Compact {
		return fmt.Errorf("architecture why requires exactly one of --json or --compact")
	}
	if err := validateArchitectureOutputLimits(values.MaxFiles, values.MaxOutputBytes); err != nil {
		return err
	}
	architecture, remote, err := loadDirectoryArchitecture(context.Background(), values.Paths, values.MaxFiles, values.Repository, values.Server, dependencies)
	if err != nil {
		return err
	}
	evidence := architectureRelationEvidenceFor(architecture.Relations, values.From, values.To)
	output := architectureWhyOutput{Schema: "grepple-architecture-why-v2", From: cleanArchitectureDirectory(values.From), To: cleanArchitectureDirectory(values.To), Relation: architectureEvidenceRelation(evidence), Sources: architecture.Sources, Evidence: evidence}
	if values.JSON {
		if err := writeArchitectureProjection(remote, output); err != nil {
			return err
		}
	} else if err := renderArchitectureWhy(output, values.MaxOutputBytes); err != nil {
		return err
	}
	if len(evidence) == 0 {
		dependencies.requestExit(1)
	}
	return nil
}

func writeArchitectureProjection(remote *api.AnalysisResponse, output any) error {
	if remote == nil {
		return stdoutWriter().writeJSON(output)
	}
	result, err := json.Marshal(output)
	if err != nil {
		return err
	}
	remote.Result = result
	return stdoutWriter().writeJSON(remote)
}

func loadDirectoryArchitecture(ctx context.Context, paths []string, maxFiles int, repository, server string, dependencies Dependencies) (Report, *api.AnalysisResponse, error) {
	if repository == "" {
		result, err := Build(paths, maxFiles, dependencies)
		return result, nil, err
	}
	response, err := dependencies.remote(ctx, api.AnalysisRequest{Operation: api.AnalysisArchitecture, Repository: repository, Paths: paths, MaxFiles: maxFiles}, dependencies.serverDefault(server))
	if err != nil {
		return Report{}, nil, err
	}
	var result Report
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return Report{}, nil, fmt.Errorf("decode remote architecture: %w", err)
	}
	return result, &response, nil
}

func validateArchitectureOutputLimits(maxFiles, maxOutputBytes int) error {
	if maxFiles < 0 || maxOutputBytes < 0 {
		return fmt.Errorf("architecture limits must be non-negative")
	}
	return nil
}

func parseArchitectureArgs(program string, args []string, values any) error {
	parser, err := arg.NewParser(arg.Config{Program: program}, values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			parser.WriteHelp(os.Stdout)
			return errArchitectureHelp
		}
		return err
	}
	return nil
}

// Build constructs a directory architecture from local sources.
func Build(globs []string, maxFiles int, dependencies Dependencies) (Report, error) {
	params := search.Params{Files: true, Globs: globs}
	if err := dependencies.applySourceConfig(&params); err != nil {
		return Report{}, err
	}
	paths, err := search.ListFilePaths(params, nil)
	if err != nil {
		return Report{}, err
	}
	discovered := len(paths)
	paths = navigationSourcePaths(paths)
	supported := len(paths)
	var truncation *navigationGraphTruncation
	if maxFiles > 0 && len(paths) > maxFiles {
		truncation = &navigationGraphTruncation{Reason: "max_files", Limit: maxFiles, Skipped: len(paths) - maxFiles}
		paths = paths[:maxFiles]
	}
	parsedSources, parseStats := LoadDocuments(paths)
	graph, graphStats := search.BuildNavigationGraphFromDocuments(NavigationDocuments(parsedSources), search.NavigationBuildOptions{})
	architecture := BuildFromParts(paths, discovered, supported, truncation, parsedSources, parseStats, graph, graphStats)
	for _, source := range parsedSources {
		source.Document.Close()
	}
	return architecture, nil
}

// BuildFromParts projects an already parsed navigation universe into directory architecture.
func BuildFromParts(paths []string, discovered, supported int, truncation *Truncation, parsedSources []ParsedSource, parseStats search.NavigationSourceStats, graph parser.NavigationGraph, graphStats search.NavigationSourceStats) Report {
	visibility := architectureVisibilityIndex(graph.Declarations)
	directories := make(map[string]*directoryAccumulator)
	symbols := make([]architectureSymbol, 0)
	for _, source := range parsedSources {
		sourcePath := source.Path
		outline := source.Outline
		directory := cleanArchitectureDirectory(filepath.Dir(sourcePath))
		classification := string(sourcekind.Classify(sourcePath, "."))
		for _, ancestor := range architectureDirectoryAncestors(directory) {
			entry := getDirectoryAccumulator(directories, ancestor)
			entry.files++
			entry.languages[outline.Language]++
			entry.classifications[classification]++
		}
		appendArchitectureSymbols(&symbols, directories, outline.Symbols, outline.Language, sourcePath, directory, "", visibility)
	}
	sortArchitectureSymbols(symbols)
	relations, relationCoverage := buildArchitectureRelations(graph, paths)
	sources := navigationSourceSummary{Discovered: discovered, Selected: len(paths), Parsed: graphStats.Parsed, Skipped: discovered - supported + parseStats.Skipped + graphStats.Skipped, Failed: parseStats.Failed + graphStats.Failed, Recovered: graphStats.Recovered}
	return directoryArchitecture{Schema: directoryArchitectureSchema, Root: ".", Files: len(paths), Sources: sources, SourceFiles: buildArchitectureSourceFiles(paths), Directories: buildArchitectureDirectories(directories), Symbols: symbols, Relations: relations, RepositoryRoots: graph.RepositoryRoots, RelationCoverage: relationCoverage, Truncation: truncation}
}

func buildArchitectureSourceFiles(paths []string) []architectureSourceFile {
	files := make([]architectureSourceFile, 0, len(paths))
	for _, path := range paths {
		files = append(files, architectureSourceFile{
			Path:           filepath.ToSlash(path),
			Language:       parser.LanguageFor(path),
			Classification: string(sourcekind.Classify(path, ".")),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}
func architectureVisibilityIndex(declarations []parser.NavigationDeclaration) map[string][]parser.NavigationDeclaration {
	result := make(map[string][]parser.NavigationDeclaration, len(declarations))
	for _, declaration := range declarations {
		key := architectureSymbolKey(declaration.Path, declaration.Name)
		result[key] = append(result[key], declaration)
	}
	return result
}

func appendArchitectureSymbols(target *[]architectureSymbol, directories map[string]*directoryAccumulator, symbols []parser.Symbol, language, sourcePath, directoryPath, container string, visibility map[string][]parser.NavigationDeclaration) {
	for _, symbol := range symbols {
		item := architectureSymbol{Name: symbol.Name, Kind: symbol.Kind, Language: language, Classification: string(sourcekind.Classify(sourcePath, ".")), Path: filepath.ToSlash(sourcePath), Directory: directoryPath, Container: container, Start: symbol.Start, End: symbol.End}
		item.Visibility, item.Entrypoint = architectureSymbolNavigation(visibility, item)
		*target = append(*target, item)
		for _, ancestor := range architectureDirectoryAncestors(directoryPath) {
			directory := getDirectoryAccumulator(directories, ancestor)
			directory.declarations[item.Kind]++
			if item.Visibility == parser.NavigationVisibilityPublic {
				directory.public++
			}
			if item.Entrypoint != "" {
				directory.entrypoints++
			}
		}
		nextContainer := item.Name
		if container != "" {
			nextContainer = container + "." + item.Name
		}
		appendArchitectureSymbols(target, directories, symbol.Children, language, sourcePath, directoryPath, nextContainer, visibility)
	}
}

func architectureSymbolKey(path, name string) string {
	return filepath.ToSlash(filepath.Clean(path)) + "\x00" + name
}

func architectureSymbolNavigation(index map[string][]parser.NavigationDeclaration, symbol architectureSymbol) (parser.NavigationVisibility, string) {
	candidates := index[architectureSymbolKey(symbol.Path, symbol.Name)]
	bestSpan := 0
	found := false
	var visibility parser.NavigationVisibility
	entrypoint := ""
	for _, candidate := range candidates {
		if symbol.Start > candidate.End || candidate.Start > symbol.End {
			continue
		}
		span := candidate.End - candidate.Start
		if !found || span < bestSpan {
			visibility = candidate.Visibility
			entrypoint = candidate.Entrypoint
			bestSpan = span
			found = true
		}
	}
	return visibility, entrypoint
}

func architectureDirectoryAncestors(directory string) []string {
	directory = cleanArchitectureDirectory(directory)
	if directory == "." {
		return []string{"."}
	}
	parts := strings.Split(directory, "/")
	result := []string{"."}
	for index := range parts {
		result = append(result, strings.Join(parts[:index+1], "/"))
	}
	return result
}

func getDirectoryAccumulator(directories map[string]*directoryAccumulator, path string) *directoryAccumulator {
	entry := directories[path]
	if entry == nil {
		entry = &directoryAccumulator{languages: make(map[string]int), declarations: make(map[string]int), classifications: make(map[string]int)}
		directories[path] = entry
	}
	return entry
}

func buildArchitectureDirectories(index map[string]*directoryAccumulator) []architectureDirectory {
	paths := make([]string, 0, len(index))
	for path := range index {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]architectureDirectory, 0, len(paths))
	for _, path := range paths {
		entry := index[path]
		result = append(result, architectureDirectory{Path: path, Files: entry.files, Classifications: architectureCounts(entry.classifications), Languages: architectureCounts(entry.languages), Declarations: architectureCounts(entry.declarations), PublicCallables: entry.public, Entrypoints: entry.entrypoints})
	}
	return result
}

func architectureCounts(values map[string]int) []architectureCount {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]architectureCount, 0, len(names))
	for _, name := range names {
		result = append(result, architectureCount{Name: name, Count: values[name]})
	}
	return result
}

func architectureRelationConfidence(confidence string) bool {
	switch confidence {
	case "exact", "import-resolved", "context-resolved":
		return true
	default:
		return false
	}
}

func architectureRelationForCall(call parser.NavigationCall, declarations map[string]parser.NavigationDeclaration) (string, string, architectureRelationEvidence, bool) {
	target, ok := declarations[call.TargetID]
	caller, callerOK := declarations[call.CallerID]
	if !ok || !callerOK || !architectureRelationConfidence(call.Confidence) {
		return "", "", architectureRelationEvidence{}, false
	}
	from := cleanArchitectureDirectory(filepath.Dir(caller.Path))
	to := cleanArchitectureDirectory(filepath.Dir(target.Path))
	if from == to {
		return "", "", architectureRelationEvidence{}, false
	}
	evidence := architectureRelationEvidence{Path: call.Path, Line: call.Line, Caller: caller.Name, Target: target.Name, Kind: "resolved-call", Classification: string(sourcekind.Classify(call.Path, ".")), Confidence: call.Confidence}
	return from, to, evidence, true
}

func buildArchitectureRelations(graph parser.NavigationGraph, paths []string) ([]architectureRelation, architectureRelationCoverage) {
	declarations := make(map[string]parser.NavigationDeclaration, len(graph.Declarations))
	packageFiles := make(map[string][]string)
	for _, declaration := range graph.Declarations {
		declarations[declaration.ID] = declaration
		if declaration.PackageID != "" {
			packageFiles[declaration.PackageID] = append(packageFiles[declaration.PackageID], declaration.Path)
		}
	}
	grouped := make(map[string]*architectureRelation)
	seen := make(map[string]bool)
	for _, call := range graph.Calls {
		from, to, evidence, ok := architectureRelationForCall(call, declarations)
		if ok {
			addArchitectureRelation(grouped, seen, from, to, evidence)
		}
	}
	coverage := architectureRelationCoverage{UnsupportedImportLanguages: unsupportedArchitectureImportLanguages(paths)}
	importTargets := addArchitectureImportRelations(grouped, seen, graph.Imports, &coverage)
	typeUsages := append([]parser.NavigationTypeUsage(nil), graph.TypeUsages...)
	for _, field := range graph.Fields {
		typeUsages = append(typeUsages, parser.NavigationTypeUsage{CallerID: field.OwnerType + "." + field.Name, Type: field.Type, ImportPath: field.ImportPath, Role: "field", Language: field.Language, Path: field.Path, Line: field.Line})
	}
	addArchitectureTypeRelations(grouped, seen, typeUsages, declarations, packageFiles, importTargets, &coverage)
	return finalizeArchitectureRelations(grouped), coverage
}

func unsupportedArchitectureImportLanguages(paths []string) []string {
	languages := map[string]bool{}
	for _, sourcePath := range paths {
		language := parser.LanguageFor(sourcePath)
		capability, ok := parser.CapabilitiesForLanguage(language)
		if ok && capability.Navigation && !capability.ImportNavigation {
			languages[language] = true
		}
	}
	result := make([]string, 0, len(languages))
	for language := range languages {
		result = append(result, language)
	}
	sort.Strings(result)
	return result
}

func addArchitectureImportRelations(grouped map[string]*architectureRelation, seen map[string]bool, imports []parser.NavigationImport, coverage *architectureRelationCoverage) map[string][]string {
	targetIndex := make(map[string][]string)
	counted := make(map[string]bool)
	for _, item := range imports {
		indexKey := item.Path + "\x00" + item.ImportPath
		targetIndex[indexKey] = append(targetIndex[indexKey], item.TargetPaths...)
		statementKey := fmt.Sprintf("%s\x00%d\x00%s", item.Path, item.Line, item.ImportPath)
		if counted[statementKey] {
			continue
		}
		counted[statementKey] = true
		coverage.ImportFacts++
		targetDirectories := architectureTargetDirectories(item.TargetPaths)
		switch len(targetDirectories) {
		case 0:
			coverage.UnresolvedImports++
		case 1:
			coverage.ResolvedImports++
			from := cleanArchitectureDirectory(filepath.Dir(item.Path))
			evidence := architectureRelationEvidence{Path: item.Path, Line: item.Line, Caller: item.Alias, Target: item.ImportPath, Kind: "import", Classification: string(sourcekind.Classify(item.Path, ".")), Confidence: "local-import-resolved", ImportPath: item.ImportPath}
			addArchitectureRelation(grouped, seen, from, targetDirectories[0], evidence)
		default:
			coverage.AmbiguousImports++
		}
	}
	for key, targets := range targetIndex {
		sort.Strings(targets)
		targetIndex[key] = compactArchitectureStrings(targets)
	}
	return targetIndex
}

func addArchitectureTypeRelations(grouped map[string]*architectureRelation, seen map[string]bool, usages []parser.NavigationTypeUsage, declarations map[string]parser.NavigationDeclaration, packageFiles, importTargets map[string][]string, coverage *architectureRelationCoverage) {
	for _, usage := range usages {
		coverage.TypeReferences++
		if usage.ImportPath == "" {
			coverage.UnqualifiedTypeReferences++
			continue
		}
		targets := append([]string(nil), packageFiles[usage.ImportPath]...)
		targets = append(targets, importTargets[usage.Path+"\x00"+usage.ImportPath]...)
		targetDirectories := architectureTargetDirectories(targets)
		switch len(targetDirectories) {
		case 0:
			coverage.UnresolvedTypeReferences++
		case 1:
			coverage.ResolvedTypeReferences++
			caller := declarations[usage.CallerID].Name
			if caller == "" {
				caller = usage.CallerID
			}
			evidence := architectureRelationEvidence{Path: usage.Path, Line: usage.Line, Caller: caller, Target: usage.Type, Kind: "type-reference", Classification: string(sourcekind.Classify(usage.Path, ".")), Confidence: "local-import-resolved", ImportPath: usage.ImportPath, Role: usage.Role}
			addArchitectureRelation(grouped, seen, cleanArchitectureDirectory(filepath.Dir(usage.Path)), targetDirectories[0], evidence)
		default:
			coverage.AmbiguousTypeReferences++
		}
	}
}

func projectArchitectureRelations(relations []architectureRelation, directories []architectureDirectory) []architectureRelation {
	visible := make([]string, 0, len(directories))
	for _, directory := range directories {
		visible = append(visible, directory.Path)
	}
	grouped := make(map[string]*architectureRelation)
	seen := make(map[string]bool)
	for _, relation := range relations {
		from := nearestVisibleArchitectureDirectory(relation.From, visible)
		to := nearestVisibleArchitectureDirectory(relation.To, visible)
		for _, evidence := range relation.Evidence {
			addArchitectureRelation(grouped, seen, from, to, evidence)
		}
	}
	return finalizeArchitectureRelations(grouped)
}

func nearestVisibleArchitectureDirectory(path string, visible []string) string {
	best := ""
	for _, candidate := range visible {
		if candidate == "." || path == candidate || strings.HasPrefix(path, candidate+"/") {
			if len(candidate) > len(best) {
				best = candidate
			}
		}
	}
	return best
}

func architectureTargetDirectories(paths []string) []string {
	directories := make([]string, 0, len(paths))
	for _, path := range paths {
		directories = append(directories, cleanArchitectureDirectory(filepath.Dir(path)))
	}
	sort.Strings(directories)
	return compactArchitectureStrings(directories)
}

func compactArchitectureStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func addArchitectureRelation(grouped map[string]*architectureRelation, seen map[string]bool, from, to string, evidence architectureRelationEvidence) {
	if from == "" || to == "" || from == to {
		return
	}
	key := from + "\x00" + to + "\x00" + evidence.Kind
	evidenceKey := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s", key, evidence.Path, evidence.Line, evidence.Caller, evidence.Target, evidence.Role)
	if seen[evidenceKey] {
		return
	}
	seen[evidenceKey] = true
	relation := grouped[key]
	if relation == nil {
		relation = &architectureRelation{From: from, To: to, Kind: evidence.Kind, Evidence: []architectureRelationEvidence{}}
		grouped[key] = relation
	}
	relation.Count++
	relation.Evidence = append(relation.Evidence, evidence)
}

func finalizeArchitectureRelations(grouped map[string]*architectureRelation) []architectureRelation {
	result := make([]architectureRelation, 0, len(grouped))
	for _, relation := range grouped {
		classifications := make(map[string]int)
		for _, evidence := range relation.Evidence {
			classifications[evidence.Classification]++
		}
		relation.Classifications = architectureCounts(classifications)
		sort.Slice(relation.Evidence, func(i, j int) bool {
			if relation.Evidence[i].Path != relation.Evidence[j].Path {
				return relation.Evidence[i].Path < relation.Evidence[j].Path
			}
			if relation.Evidence[i].Line != relation.Evidence[j].Line {
				return relation.Evidence[i].Line < relation.Evidence[j].Line
			}
			return relation.Evidence[i].Target < relation.Evidence[j].Target
		})
		result = append(result, *relation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].From != result[j].From {
			return result[i].From < result[j].From
		}
		if result[i].To != result[j].To {
			return result[i].To < result[j].To
		}
		return result[i].Kind < result[j].Kind
	})
	return result
}

func resolveArchitectureSymbols(symbols []architectureSymbol, query string) []architectureSymbol {
	matches := make([]architectureSymbol, 0)
	for _, symbol := range symbols {
		terminal := symbol.Name
		if separator := strings.LastIndexAny(terminal, ".:#"); separator >= 0 {
			terminal = terminal[separator+1:]
		}
		if symbol.Name == query || terminal == query {
			matches = append(matches, symbol)
		}
	}
	return matches
}

func architectureRelationEvidenceFor(relations []architectureRelation, from, to string) []architectureRelationEvidence {
	from = cleanArchitectureDirectory(from)
	to = cleanArchitectureDirectory(to)
	evidence := []architectureRelationEvidence{}
	for _, relation := range relations {
		if relation.From == from && relation.To == to {
			evidence = append(evidence, relation.Evidence...)
		}
	}
	return evidence
}

func architectureEvidenceRelation(evidence []architectureRelationEvidence) string {
	kinds := make([]string, 0)
	seen := make(map[string]bool)
	for _, item := range evidence {
		if !seen[item.Kind] {
			seen[item.Kind] = true
			kinds = append(kinds, item.Kind)
		}
	}
	sort.Strings(kinds)
	if len(kinds) == 0 {
		return "none"
	}
	return strings.Join(kinds, ",")
}

func sortArchitectureSymbols(symbols []architectureSymbol) {
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Path != symbols[j].Path {
			return symbols[i].Path < symbols[j].Path
		}
		if symbols[i].Start != symbols[j].Start {
			return symbols[i].Start < symbols[j].Start
		}
		return symbols[i].Name < symbols[j].Name
	})
}

func cleanArchitectureDirectory(path string) string {
	clean := filepath.ToSlash(filepath.Clean(path))
	if clean == "" || clean == "./" {
		return "."
	}
	return strings.TrimPrefix(clean, "./")
}

func renderDirectoryArchitecture(architecture directoryArchitecture, values architectureArgs) error {
	output := architectureOutputWriter(values.MaxOutputBytes)
	write := func(format string, arguments ...any) bool {
		return output.writeString(fmt.Sprintf(format, arguments...)+"\n") == nil
	}
	directories := filterArchitectureDirectories(architecture.Directories, values.Depth)
	shown := len(directories)
	if values.MaxNodes > 0 && shown > values.MaxNodes {
		shown = values.MaxNodes
	}
	visibleRelations := projectArchitectureRelations(architecture.Relations, directories[:shown])
	entrypoints := countArchitectureEntrypoints(architecture.Symbols)
	if !write("architecture %s root=%s files=%d directories=%d shown=%d relations=%d shown-relations=%d entrypoints=%d sources=%s", architecture.Schema, architecture.Root, architecture.Files, len(directories), shown, len(architecture.Relations), len(visibleRelations), entrypoints, compactNavigationSourceSummary(architecture.Sources)) {
		return nil
	}
	if !write("repository-roots %s", formatArchitectureStrings(architecture.RepositoryRoots)) {
		return nil
	}
	coverage := architecture.RelationCoverage
	if !write("coverage imports=%d resolved=%d ambiguous=%d unresolved=%d types=%d type-resolved=%d type-ambiguous=%d type-unresolved=%d type-unqualified=%d import-unsupported=%s", coverage.ImportFacts, coverage.ResolvedImports, coverage.AmbiguousImports, coverage.UnresolvedImports, coverage.TypeReferences, coverage.ResolvedTypeReferences, coverage.AmbiguousTypeReferences, coverage.UnresolvedTypeReferences, coverage.UnqualifiedTypeReferences, formatArchitectureStrings(coverage.UnsupportedImportLanguages)) {
		return nil
	}
	if !renderArchitectureDirectories(write, directories[:shown]) {
		return nil
	}
	if !renderArchitectureFacts(write, architecture.Symbols) {
		return nil
	}
	if !renderArchitectureRelations(write, visibleRelations) {
		return nil
	}
	if len(visibleRelations) < len(architecture.Relations) {
		if !write("! omitted relation-groups=%d; raise --depth/--max-nodes or narrow PATH", len(architecture.Relations)-len(visibleRelations)) {
			return nil
		}
	}
	if shown < len(directories) {
		_ = write("! omitted directories=%d; raise --max-nodes or narrow PATH", len(directories)-shown)
	}
	return nil
}

func countArchitectureEntrypoints(symbols []architectureSymbol) int {
	count := 0
	for _, symbol := range symbols {
		if symbol.Entrypoint != "" {
			count++
		}
	}
	return count
}

func renderArchitectureDirectories(write func(string, ...any) bool, directories []architectureDirectory) bool {
	for _, directory := range directories {
		if !write("D %s files=%d classes=%s languages=%s declarations=%s public-callables=%d entrypoints=%d", directory.Path, directory.Files, formatArchitectureCounts(directory.Classifications), formatArchitectureCounts(directory.Languages), formatArchitectureCounts(directory.Declarations), directory.PublicCallables, directory.Entrypoints) {
			return false
		}
	}
	return true
}

func renderArchitectureFacts(write func(string, ...any) bool, symbols []architectureSymbol) bool {
	for _, symbol := range symbols {
		if symbol.Entrypoint != "" && !write("E %s %s %s class=%s directory=%s", symbol.Entrypoint, symbol.Name, architectureSymbolLocation(symbol), symbol.Classification, symbol.Directory) {
			return false
		}
	}
	return true
}

func renderArchitectureRelations(write func(string, ...any) bool, relations []architectureRelation) bool {
	for _, relation := range relations {
		if !write("R %s -> %s kind=%s count=%d classes=%s at=%s", relation.From, relation.To, relation.Kind, relation.Count, formatArchitectureCounts(relation.Classifications), architectureEvidenceLocation(relation.Evidence)) {
			return false
		}
	}
	return true
}
func renderArchitectureResolve(output architectureResolveOutput, maxBytes int) error {
	writer := architectureOutputWriter(maxBytes)
	if err := writer.writeString(fmt.Sprintf("architecture resolve symbol=%s matches=%d sources=%s\n", output.Symbol, len(output.Matches), compactNavigationSourceSummary(output.Sources))); err != nil {
		return nil
	}
	for _, match := range output.Matches {
		visibility := string(match.Visibility)
		if visibility == "" {
			visibility = "unknown"
		}
		if err := writer.writeString(fmt.Sprintf("S %s %s %s %s visibility=%s entrypoint=%s class=%s directory=%s\n", match.Language, match.Kind, match.Name, architectureSymbolLocation(match), visibility, emptyArchitectureValue(match.Entrypoint), match.Classification, match.Directory)); err != nil {
			return nil
		}
	}
	return nil
}

func renderArchitectureWhy(output architectureWhyOutput, maxBytes int) error {
	writer := architectureOutputWriter(maxBytes)
	if err := writer.writeString(fmt.Sprintf("architecture why %s -> %s relation=%s evidence=%d sources=%s\n", output.From, output.To, output.Relation, len(output.Evidence), compactNavigationSourceSummary(output.Sources))); err != nil {
		return nil
	}
	for _, evidence := range output.Evidence {
		if err := writer.writeString(fmt.Sprintf("E %s:%d %s -> %s kind=%s class=%s confidence=%s\n", evidence.Path, evidence.Line, evidence.Caller, evidence.Target, evidence.Kind, evidence.Classification, evidence.Confidence)); err != nil {
			return nil
		}
	}
	return nil
}

func emptyArchitectureValue(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func architectureOutputWriter(maxBytes int) *outputWriter {
	if maxBytes > 0 {
		return newBoundedOutputWriter(os.Stdout, maxBytes)
	}
	return stdoutWriter()
}

func filterArchitectureDirectories(directories []architectureDirectory, depth int) []architectureDirectory {
	if depth == 0 {
		return directories
	}
	result := make([]architectureDirectory, 0, len(directories))
	for _, directory := range directories {
		if architectureDirectoryDepth(directory.Path) <= depth {
			result = append(result, directory)
		}
	}
	return result
}

func architectureDirectoryDepth(path string) int {
	if path == "." {
		return 0
	}
	return len(strings.Split(filepath.ToSlash(path), "/"))
}

func formatArchitectureStrings(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
}

func formatArchitectureCounts(counts []architectureCount) string {
	parts := make([]string, 0, len(counts))
	for _, count := range counts {
		parts = append(parts, fmt.Sprintf("%s:%d", count.Name, count.Count))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func architectureEvidenceLocation(evidence []architectureRelationEvidence) string {
	if len(evidence) == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", evidence[0].Path, evidence[0].Line)
}

func architectureSymbolLocation(symbol architectureSymbol) string {
	if symbol.Start == symbol.End {
		return fmt.Sprintf("%s:%d", symbol.Path, symbol.Start)
	}
	return fmt.Sprintf("%s:%d-%d", symbol.Path, symbol.Start, symbol.End)
}
