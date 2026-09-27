// Package architecture implements source-linked directory architecture commands.
package architecture

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
	"github.com/greppleai/grepple/internal/analysis"
	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/archdaemon"
	"github.com/greppleai/grepple/internal/search"
)

const directoryArchitectureSchema = analysis.ArchitectureSchema

type architectureArgs struct {
	JSON bool `arg:"--json" help:"emit complete directory architecture JSON"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	Relations      bool     `arg:"--relations" help:"emit one compressed relation per directory pair"`
	Mermaid        bool     `arg:"--mermaid" help:"emit a Mermaid directory dependency flowchart with source-colored links"`
	Output         string   `arg:"--output" placeholder:"PATH" help:"write Mermaid output instead of stdout"`
	Depth          int      `arg:"--depth" default:"3" placeholder:"N" help:"maximum displayed directory depth (default 3; 0 = unlimited)"`
	MaxNodes       int      `arg:"--max-nodes" default:"200" placeholder:"N" help:"maximum directories in text or Mermaid output (default 200; 0 = unlimited)"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap text output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"file, directory, or glob to include; defaults to the working directory"`
}

type architectureResolveArgs struct {
	JSON bool `arg:"--json" help:"emit complete matching declarations as JSON"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	Symbol         string   `arg:"--symbol,required" placeholder:"NAME" help:"exact or terminal declaration name"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited)"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
}

type architectureWhyArgs struct {
	JSON bool `arg:"--json" help:"emit complete source-linked relation evidence as JSON"`
	commonArgs
	Repository     string   `arg:"--repo" placeholder:"OWNER/REPO[@REF]" help:"analyze one exact indexed repository"`
	MaxFiles       int      `arg:"--max-files" placeholder:"N" help:"analyze at most N supported files (0 = unlimited)"`
	MaxOutputBytes int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited)"`
	From           string   `arg:"positional,required" placeholder:"FROM"`
	To             string   `arg:"positional,required" placeholder:"TO"`
	Paths          []string `arg:"positional" placeholder:"PATH" help:"source universe; defaults to the working directory"`
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

// Run executes the architecture command family.
func (command *command) Run(args []string) error {
	dependencies := command.services()
	if len(args) == 0 || isExtractHelp(args[0]) {
		return stdoutWriter(dependencies).writeString("Inspect language-neutral directory architecture. Local checkout is the default; --repo selects one exact indexed remote repository. Human output is the default; use --json for complete machine output.\nUsage:\n  grepple architecture directory [--relations | --mermaid | --json] [PATH ...]\n")
	}
	switch args[0] {
	case "directory":
		return command.runDirectory(args[1:])
	default:
		return fmt.Errorf("unknown architecture command %q", args[0])
	}
}

func (command *command) runDirectory(args []string) error {
	return runArchitectureDirectory(args, command.services())
}

var errArchitectureHelp = errors.New("architecture help displayed")

func runArchitectureDirectory(args []string, dependencies Dependencies) error {
	values := architectureArgs{Depth: 3, MaxNodes: 200, MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture directory", args, &values, dependencies.Stdout); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	return executeArchitectureDirectory(&values, dependencies)
}

func executeArchitectureDirectory(values *DirectoryArgs, dependencies Dependencies) error {
	outputModes := 0
	for _, enabled := range []bool{values.JSON, values.Relations, values.Mermaid} {
		if enabled {
			outputModes++
		}
	}
	if outputModes > 1 {
		return fmt.Errorf("architecture directory output modes --json, --relations, and --mermaid cannot be combined")
	}
	if values.Output != "" && !values.Mermaid {
		return fmt.Errorf("architecture directory --output requires --mermaid")
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
			return stdoutWriter(dependencies).writeJSON(remote)
		}
		return stdoutWriter(dependencies).writeJSON(architecture)
	}
	if values.Mermaid {
		return renderDirectoryArchitectureMermaid(architecture, *values, dependencies)
	}
	if values.Relations {
		return renderDirectoryArchitectureRelations(architecture, *values, dependencies)
	}
	return renderDirectoryArchitecture(architecture, *values, dependencies)
}

func runArchitectureResolve(args []string, dependencies Dependencies) error {
	values := architectureResolveArgs{MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture resolve", args, &values, dependencies.Stdout); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	return executeArchitectureResolve(&values, dependencies)
}

func executeArchitectureResolve(values *ResolveArgs, dependencies Dependencies) error {
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
		if err := writeArchitectureProjection(remote, output, dependencies); err != nil {
			return err
		}
	} else if err := renderArchitectureResolve(output, values.MaxOutputBytes, dependencies); err != nil {
		return err
	}
	if len(matches) == 0 {
		dependencies.requestExit(1)
	}
	return nil
}

func runArchitectureWhy(args []string, dependencies Dependencies) error {
	values := architectureWhyArgs{MaxOutputBytes: defaultTextOutputBytes}
	if err := parseArchitectureArgs("grepple architecture why", args, &values, dependencies.Stdout); err != nil {
		if errors.Is(err, errArchitectureHelp) {
			return nil
		}
		return err
	}
	return executeArchitectureWhy(&values, dependencies)
}

func executeArchitectureWhy(values *WhyArgs, dependencies Dependencies) error {
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
		if err := writeArchitectureProjection(remote, output, dependencies); err != nil {
			return err
		}
	} else if err := renderArchitectureWhy(output, values.MaxOutputBytes, dependencies); err != nil {
		return err
	}
	if len(evidence) == 0 {
		dependencies.requestExit(1)
	}
	return nil
}

func writeArchitectureProjection(remote *wire.AnalysisResponse, output any, dependencies Dependencies) error {
	if remote == nil {
		return stdoutWriter(dependencies).writeJSON(output)
	}
	result, err := json.Marshal(output)
	if err != nil {
		return err
	}
	remote.Result = result
	return stdoutWriter(dependencies).writeJSON(remote)
}

func loadDirectoryArchitecture(ctx context.Context, paths []string, maxFiles int, repository, server string, dependencies Dependencies) (Report, *wire.AnalysisResponse, error) {
	if repository == "" {
		result, err := Build(paths, maxFiles, dependencies)
		return result, nil, err
	}
	response, err := dependencies.remote(ctx, wire.AnalysisRequest{Operation: wire.AnalysisArchitecture, Repository: repository, Paths: paths, MaxFiles: maxFiles}, dependencies.serverDefault(server))
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

func parseArchitectureArgs(program string, args []string, values any, outputs ...io.Writer) error {
	parser, err := arg.NewParser(arg.Config{Program: program}, values)
	if err != nil {
		return err
	}
	if err := parser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			output := io.Writer(os.Stdout)
			if len(outputs) > 0 && outputs[0] != nil {
				output = outputs[0]
			}
			parser.WriteHelp(output)
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
	if dependencies.Daemon {
		if report, ok := archdaemon.Query(paths, maxFiles); ok {
			return report, nil
		}
	}
	sources := analysis.ReadSources(paths)
	key := ""
	if dependencies.Daemon {
		key, _ = archdaemon.Key(paths, maxFiles, sources)
	}
	universe, err := analysis.NewUniverse(sources, maxFiles)
	if err != nil {
		return Report{}, err
	}
	defer universe.Close()
	report := analysis.BuildArchitecture(universe)
	if key != "" {
		_ = archdaemon.Store(paths, maxFiles, key, report)
	}
	return report, nil
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
			addProjectedArchitectureRelation(grouped, seen, from, to, evidence)
		}
	}
	return finalizeProjectedArchitectureRelations(grouped)
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

func addProjectedArchitectureRelation(grouped map[string]*architectureRelation, seen map[string]bool, from, to string, evidence architectureRelationEvidence) {
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

func finalizeProjectedArchitectureRelations(grouped map[string]*architectureRelation) []architectureRelation {
	result := make([]architectureRelation, 0, len(grouped))
	for _, relation := range grouped {
		classifications := make(map[string]int)
		for _, evidence := range relation.Evidence {
			classifications[evidence.Classification]++
		}
		relation.Classifications = projectedArchitectureCounts(classifications)
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

func projectedArchitectureCounts(values map[string]int) []architectureCount {
	result := make([]architectureCount, 0, len(values))
	for name, count := range values {
		result = append(result, architectureCount{Name: name, Count: count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func resolveArchitectureSymbols(symbols []architectureSymbol, query string) []architectureSymbol {
	return analysis.ResolveArchitectureSymbols(symbols, query)
}

func architectureRelationEvidenceFor(relations []architectureRelation, from, to string) []architectureRelationEvidence {
	return analysis.ArchitectureRelationEvidenceFor(relations, from, to)
}

func architectureEvidenceRelation(evidence []architectureRelationEvidence) string {
	return analysis.ArchitectureEvidenceRelation(evidence)
}

func cleanArchitectureDirectory(path string) string {
	return analysis.CleanArchitectureDirectory(path)
}

func renderDirectoryArchitecture(architecture directoryArchitecture, values architectureArgs, dependencies Dependencies) error {
	output := architectureOutputWriter(values.MaxOutputBytes, dependencies)
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
	if !renderArchitectureFacts(write, architecture.Symbols) {
		return nil
	}
	if len(visibleRelations) > 0 && !write("") {
		return nil
	}
	if !renderArchitectureRelations(write, visibleRelations) || !renderUnconnectedArchitectureDirectories(write, directories[:shown], architecture.Directories, visibleRelations) {
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

func renderDirectoryArchitectureRelations(architecture directoryArchitecture, values architectureArgs, dependencies Dependencies) error {
	output := architectureOutputWriter(values.MaxOutputBytes, dependencies)
	write := func(format string, arguments ...any) bool {
		return output.writeString(fmt.Sprintf(format, arguments...)+"\n") == nil
	}
	directories := filterArchitectureDirectories(architecture.Directories, values.Depth)
	shown := len(directories)
	if values.MaxNodes > 0 && shown > values.MaxNodes {
		shown = values.MaxNodes
	}
	return writeCompressedArchitectureRelations(write, compressedArchitectureRelations(projectArchitectureRelations(architecture.Relations, directories[:shown])))
}

func renderDirectoryArchitectureMermaid(architecture directoryArchitecture, values architectureArgs, dependencies Dependencies) error {
	directories := filterArchitectureDirectories(architecture.Directories, values.Depth)
	shown := len(directories)
	if values.MaxNodes > 0 && shown > values.MaxNodes {
		shown = values.MaxNodes
	}
	relations := compressedArchitectureRelations(projectArchitectureRelations(architecture.Relations, directories[:shown]))
	content := architectureRelationsMermaid(relations, shown < len(directories), values.MaxNodes)
	if values.Output == "" {
		return stdoutWriter(dependencies).writeString(content)
	}
	if err := os.MkdirAll(filepath.Dir(values.Output), 0o755); err != nil {
		return err
	}
	return os.WriteFile(values.Output, []byte(content), 0o644)
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

// renderUnconnectedArchitectureDirectories reports visible leaf directories that
// would otherwise vanish from the relation-first human projection. Use the full
// inventory to avoid mislabeling ancestors when depth or node limits hide children.
func renderUnconnectedArchitectureDirectories(write func(string, ...any) bool, directories, allDirectories []architectureDirectory, relations []architectureRelation) bool {
	connected := make(map[string]bool, len(relations)*2)
	for _, relation := range relations {
		connected[relation.From], connected[relation.To] = true, true
	}
	wrote := false
	for _, directory := range directories {
		if connected[directory.Path] {
			continue
		}
		ancestor := false
		for _, other := range allDirectories {
			if other.Path != directory.Path && (directory.Path == "." || strings.HasPrefix(other.Path, directory.Path+"/")) {
				ancestor = true
				break
			}
		}
		if ancestor {
			continue
		}
		if !wrote && len(relations) > 0 && !write("") {
			return false
		}
		if !write("unconnected %s", directory.Path) {
			return false
		}
		wrote = true
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
	from, to := "", ""
	for _, relation := range relations {
		if relation.From != from || relation.To != to {
			if from != "" && !write("") {
				return false
			}
			from, to = relation.From, relation.To
			if !write("%s -> %s", from, to) {
				return false
			}
		}
		if !write("  %-15s count=%d classes=%s at=%s", relation.Kind, relation.Count, formatArchitectureCounts(relation.Classifications), architectureEvidenceLocation(relation.Evidence)) {
			return false
		}
	}
	return true
}

type compressedArchitectureRelation struct {
	From  string
	To    string
	Kinds []string
}

func compressedArchitectureRelations(relations []architectureRelation) []compressedArchitectureRelation {
	compressed := make([]compressedArchitectureRelation, 0, len(relations))
	for index := 0; index < len(relations); {
		from, to := relations[index].From, relations[index].To
		kinds := make([]string, 0, 3)
		for index < len(relations) && relations[index].From == from && relations[index].To == to {
			kinds = append(kinds, relations[index].Kind)
			index++
		}
		compressed = append(compressed, compressedArchitectureRelation{From: from, To: to, Kinds: kinds})
	}
	return compressed
}

func writeCompressedArchitectureRelations(write func(string, ...any) bool, relations []compressedArchitectureRelation) error {
	for _, relation := range relations {
		if !write("R %s -> %s kinds=%s", relation.From, relation.To, strings.Join(relation.Kinds, ",")) {
			return nil
		}
	}
	return nil
}

func architectureRelationsMermaid(relations []compressedArchitectureRelation, truncated bool, maxNodes int) string {
	nodeSet := make(map[string]bool)
	for _, relation := range relations {
		nodeSet[relation.From] = true
		nodeSet[relation.To] = true
	}
	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	identifiers := make(map[string]string, len(nodes))
	colors := make(map[string]string, len(nodes))
	lines := []string{"flowchart LR"}
	if truncated {
		lines = append(lines, fmt.Sprintf("    %%%% grepple:truncated max-nodes %d", maxNodes))
	}
	for index, node := range nodes {
		identifier := fmt.Sprintf("d%d", index)
		identifiers[node] = identifier
		colors[node] = architectureMermaidColor(index)
		label := html.EscapeString(strings.NewReplacer("\r", "", "\n", `\\n`).Replace(node))
		lines = append(lines, fmt.Sprintf("    %s[\"%s\"]", identifier, label))
	}
	if len(nodes) > 0 {
		lines = append(lines, "", "    %% Link strokes use the source directory color; node borders are the legend.")
		for _, node := range nodes {
			lines = append(lines, fmt.Sprintf("    style %s stroke:%s,stroke-width:2px", identifiers[node], colors[node]))
		}
	}
	if len(relations) > 0 {
		lines = append(lines, "")
	}
	for _, relation := range relations {
		lines = append(lines, fmt.Sprintf("    %s -->|%s| %s", identifiers[relation.From], strings.Join(relation.Kinds, ", "), identifiers[relation.To]))
	}
	if len(relations) > 0 {
		lines = append(lines, "")
	}
	for index, relation := range relations {
		lines = append(lines, fmt.Sprintf("    linkStyle %d stroke:%s,stroke-width:2px", index, colors[relation.From]))
	}
	return strings.Join(lines, "\n") + "\n"
}
func renderArchitectureResolve(output architectureResolveOutput, maxBytes int, dependencies Dependencies) error {
	writer := architectureOutputWriter(maxBytes, dependencies)
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

func renderArchitectureWhy(output architectureWhyOutput, maxBytes int, dependencies Dependencies) error {
	writer := architectureOutputWriter(maxBytes, dependencies)
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

func architectureOutputWriter(maxBytes int, dependencies Dependencies) *outputWriter {
	if maxBytes > 0 {
		return newBoundedOutputWriter(outputDestination(dependencies), maxBytes)
	}
	return stdoutWriter(dependencies)
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
