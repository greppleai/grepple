package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/sourcekind"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

const directoryArchitectureSchema = "grepple-directory-architecture-v1"

type architectureArgs struct {
	JSON           bool     `arg:"--json" help:"emit complete directory architecture JSON"`
	Compact        bool     `arg:"--compact" help:"emit a bounded agent-facing directory summary"`
	Depth          int      `arg:"--depth" placeholder:"N" help:"maximum displayed directory depth (default 3; 0 = unlimited)"`
	MaxNodes       int      `arg:"--max-nodes" placeholder:"N" help:"maximum directories in compact output (default 200; 0 = unlimited)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

type architectureResolveArgs struct {
	JSON           bool     `arg:"--json" help:"emit complete matching declarations as JSON"`
	Compact        bool     `arg:"--compact" help:"emit bounded source-linked matches"`
	Symbol         string   `arg:"--symbol,required" placeholder:"NAME" help:"exact or terminal declaration name"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
}

type architectureWhyArgs struct {
	JSON           bool     `arg:"--json" help:"emit complete source-linked relation evidence as JSON"`
	Compact        bool     `arg:"--compact" help:"emit bounded source-linked relation evidence"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" placeholder:"N" help:"cap compact output (default 16384; 0 = unlimited)"`
	From           string   `arg:"positional,required" placeholder:"FROM"`
	To             string   `arg:"positional,required" placeholder:"TO"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
}

type directoryArchitecture struct {
	Schema      string                     `json:"schema"`
	Root        string                     `json:"root"`
	Files       int                        `json:"files"`
	Sources     navigationSourceSummary    `json:"sources"`
	Directories []architectureDirectory    `json:"directories"`
	Symbols     []architectureSymbol       `json:"symbols"`
	Relations   []architectureRelation     `json:"relations"`
	Truncation  *navigationGraphTruncation `json:"truncation,omitempty"`
}

type architectureDirectory struct {
	Path            string              `json:"path"`
	Files           int                 `json:"files"`
	Classifications []architectureCount `json:"classifications"`
	Languages       []architectureCount `json:"languages"`
	Declarations    []architectureCount `json:"declarations"`
	PublicCallables int                 `json:"publicCallables"`
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
	Start          int                         `json:"startLine"`
	End            int                         `json:"endLine"`
}

type architectureRelation struct {
	From     string                         `json:"from"`
	To       string                         `json:"to"`
	Kind     string                         `json:"kind"`
	Count    int                            `json:"count"`
	Evidence []architectureRelationEvidence `json:"evidence"`
}

type architectureRelationEvidence struct {
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Caller     string `json:"caller"`
	Target     string `json:"target"`
	Confidence string `json:"confidence"`
}

type architectureResolveOutput struct {
	Schema  string                  `json:"schema"`
	Symbol  string                  `json:"symbol"`
	Sources navigationSourceSummary `json:"sources"`
	Matches []architectureSymbol    `json:"matches"`
}

type architectureWhyOutput struct {
	Schema   string                         `json:"schema"`
	From     string                         `json:"from"`
	To       string                         `json:"to"`
	Relation string                         `json:"relation"`
	Sources  navigationSourceSummary        `json:"sources"`
	Evidence []architectureRelationEvidence `json:"evidence"`
}

type directoryAccumulator struct {
	files           int
	classifications map[string]int
	languages       map[string]int
	declarations    map[string]int
	public          int
}

func runArchitecture(args []string) error {
	if len(args) == 0 || isExtractHelp(args[0]) {
		return stdoutWriter().writeString("Inspect language-neutral directory architecture.\nUsage:\n  grepple architecture directory (--compact | --json) [PATH ...]\n  grepple architecture resolve --symbol NAME (--compact | --json) [PATH ...]\n  grepple architecture why FROM TO (--compact | --json) [PATH ...]\n")
	}
	switch args[0] {
	case "directory":
		return runArchitectureDirectory(args[1:])
	case "resolve":
		return runArchitectureResolve(args[1:])
	case "why":
		return runArchitectureWhy(args[1:])
	default:
		return fmt.Errorf("unknown architecture command %q", args[0])
	}
}

var errArchitectureHelp = errors.New("architecture help displayed")

func runArchitectureDirectory(args []string) error {
	values := architectureArgs{Depth: 3, MaxNodes: 200, MaxOutputBytes: DefaultTextOutputBytes}
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
	architecture, err := buildDirectoryArchitecture(values.Paths, values.MaxFiles)
	if err != nil {
		return err
	}
	if values.JSON {
		return stdoutWriter().writeJSON(architecture)
	}
	return renderDirectoryArchitecture(architecture, values)
}

func runArchitectureResolve(args []string) error {
	values := architectureResolveArgs{MaxOutputBytes: DefaultTextOutputBytes}
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
	architecture, err := buildDirectoryArchitecture(values.Paths, values.MaxFiles)
	if err != nil {
		return err
	}
	matches := resolveArchitectureSymbols(architecture.Symbols, values.Symbol)
	output := architectureResolveOutput{Schema: "grepple-architecture-resolve-v1", Symbol: values.Symbol, Sources: architecture.Sources, Matches: matches}
	if values.JSON {
		if err := stdoutWriter().writeJSON(output); err != nil {
			return err
		}
	} else if err := renderArchitectureResolve(output, values.MaxOutputBytes); err != nil {
		return err
	}
	if len(matches) == 0 {
		requestExit(1)
	}
	return nil
}

func runArchitectureWhy(args []string) error {
	values := architectureWhyArgs{MaxOutputBytes: DefaultTextOutputBytes}
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
	architecture, err := buildDirectoryArchitecture(values.Paths, values.MaxFiles)
	if err != nil {
		return err
	}
	evidence := architectureRelationEvidenceFor(architecture.Relations, values.From, values.To)
	output := architectureWhyOutput{Schema: "grepple-architecture-why-v1", From: cleanArchitectureDirectory(values.From), To: cleanArchitectureDirectory(values.To), Relation: "resolved-call", Sources: architecture.Sources, Evidence: evidence}
	if values.JSON {
		if err := stdoutWriter().writeJSON(output); err != nil {
			return err
		}
	} else if err := renderArchitectureWhy(output, values.MaxOutputBytes); err != nil {
		return err
	}
	if len(evidence) == 0 {
		requestExit(1)
	}
	return nil
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

func buildDirectoryArchitecture(globs []string, maxFiles int) (directoryArchitecture, error) {
	params := search.Params{Files: true, Globs: globs}
	if err := applyRepositorySourceConfig(&params); err != nil {
		return directoryArchitecture{}, err
	}
	paths, err := search.ListFilePaths(params, nil)
	if err != nil {
		return directoryArchitecture{}, err
	}
	discovered := len(paths)
	paths = navigationSourcePaths(paths)
	supported := len(paths)
	var truncation *navigationGraphTruncation
	if maxFiles > 0 && len(paths) > maxFiles {
		truncation = &navigationGraphTruncation{Reason: "max_files", Limit: maxFiles, Skipped: len(paths) - maxFiles}
		paths = paths[:maxFiles]
	}
	graph, graphStats := search.BuildNavigationGraphWithStats(paths)
	visibility := architectureVisibilityIndex(graph.Declarations)
	directories := make(map[string]*directoryAccumulator)
	symbols := make([]architectureSymbol, 0)
	for _, sourcePath := range paths {
		content, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			continue
		}
		outline := parser.OutlineFile(sourcePath, string(content))
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
	relations := buildArchitectureRelations(graph)
	sources := navigationSourceSummary{Discovered: discovered, Selected: graphStats.Attempted, Parsed: graphStats.Parsed, Skipped: discovered - supported + graphStats.Skipped, Failed: graphStats.Failed, Recovered: graphStats.Recovered}
	return directoryArchitecture{Schema: directoryArchitectureSchema, Root: ".", Files: len(paths), Sources: sources, Directories: buildArchitectureDirectories(directories), Symbols: symbols, Relations: relations, Truncation: truncation}, nil
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
		item.Visibility = architectureSymbolVisibility(visibility, item)
		*target = append(*target, item)
		for _, ancestor := range architectureDirectoryAncestors(directoryPath) {
			directory := getDirectoryAccumulator(directories, ancestor)
			directory.declarations[item.Kind]++
			if item.Visibility == parser.NavigationVisibilityPublic {
				directory.public++
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

func architectureSymbolVisibility(index map[string][]parser.NavigationDeclaration, symbol architectureSymbol) parser.NavigationVisibility {
	candidates := index[architectureSymbolKey(symbol.Path, symbol.Name)]
	bestSpan := 0
	found := false
	var visibility parser.NavigationVisibility
	for _, candidate := range candidates {
		if symbol.Start > candidate.End || candidate.Start > symbol.End {
			continue
		}
		span := candidate.End - candidate.Start
		if !found || span < bestSpan {
			visibility = candidate.Visibility
			bestSpan = span
			found = true
		}
	}
	return visibility
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
		result = append(result, architectureDirectory{Path: path, Files: entry.files, Classifications: architectureCounts(entry.classifications), Languages: architectureCounts(entry.languages), Declarations: architectureCounts(entry.declarations), PublicCallables: entry.public})
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
	evidence := architectureRelationEvidence{Path: call.Path, Line: call.Line, Caller: caller.Name, Target: target.Name, Confidence: call.Confidence}
	return from, to, evidence, true
}

func buildArchitectureRelations(graph parser.NavigationGraph) []architectureRelation {
	declarations := make(map[string]parser.NavigationDeclaration, len(graph.Declarations))
	for _, declaration := range graph.Declarations {
		declarations[declaration.ID] = declaration
	}
	grouped := make(map[string]*architectureRelation)
	for _, call := range graph.Calls {
		from, to, evidence, ok := architectureRelationForCall(call, declarations)
		if !ok {
			continue
		}
		key := from + "\x00" + to
		relation := grouped[key]
		if relation == nil {
			relation = &architectureRelation{From: from, To: to, Kind: "resolved-call", Evidence: []architectureRelationEvidence{}}
			grouped[key] = relation
		}
		relation.Count++
		relation.Evidence = append(relation.Evidence, evidence)
	}
	result := make([]architectureRelation, 0, len(grouped))
	for _, relation := range grouped {
		sort.Slice(relation.Evidence, func(i, j int) bool {
			if relation.Evidence[i].Path != relation.Evidence[j].Path {
				return relation.Evidence[i].Path < relation.Evidence[j].Path
			}
			return relation.Evidence[i].Line < relation.Evidence[j].Line
		})
		result = append(result, *relation)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].From != result[j].From {
			return result[i].From < result[j].From
		}
		return result[i].To < result[j].To
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
	for _, relation := range relations {
		if relation.From == from && relation.To == to {
			return relation.Evidence
		}
	}
	return []architectureRelationEvidence{}
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
	if !write("architecture %s root=%s files=%d directories=%d shown=%d relations=%d sources=%s", architecture.Schema, architecture.Root, architecture.Files, len(directories), shown, len(architecture.Relations), compactNavigationSourceSummary(architecture.Sources)) {
		return nil
	}
	for _, directory := range directories[:shown] {
		if !write("D %s files=%d classes=%s languages=%s declarations=%s public-callables=%d", directory.Path, directory.Files, formatArchitectureCounts(directory.Classifications), formatArchitectureCounts(directory.Languages), formatArchitectureCounts(directory.Declarations), directory.PublicCallables) {
			return nil
		}
	}
	for _, relation := range architecture.Relations {
		if !write("R %s -> %s kind=%s count=%d at=%s", relation.From, relation.To, relation.Kind, relation.Count, architectureEvidenceLocation(relation.Evidence)) {
			return nil
		}
	}
	if shown < len(directories) {
		_ = write("! omitted directories=%d; raise --max-nodes or narrow PATH", len(directories)-shown)
	}
	return nil
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
		if err := writer.writeString(fmt.Sprintf("S %s %s %s %s visibility=%s class=%s directory=%s\n", match.Language, match.Kind, match.Name, architectureSymbolLocation(match), visibility, match.Classification, match.Directory)); err != nil {
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
		if err := writer.writeString(fmt.Sprintf("E %s:%d %s -> %s [%s]\n", evidence.Path, evidence.Line, evidence.Caller, evidence.Target, evidence.Confidence)); err != nil {
			return nil
		}
	}
	return nil
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
