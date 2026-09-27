package grit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/greppleai/grepple/internal/gritql"
	"github.com/greppleai/grepple/internal/gritqlapi"
	"github.com/greppleai/grepple/internal/search"
	"github.com/greppleai/grepple/internal/wire"

	"github.com/alexflint/go-arg"
)

// DefaultGritResultLimit caps local structural findings unless --limit changes it.
const DefaultGritResultLimit = 20

type gritArgs struct {
	Local  bool `arg:"--local" help:"search only the local working directory (the default)"`
	Remote bool `arg:"-R,--remote" help:"also query the remote shard/router and merge with local findings"`
	commonArgs
	QueryFile            string   `arg:"-f,--query-file" placeholder:"PATH" help:"read the GritQL query from PATH (- for stdin)"`
	PatternID            string   `arg:"--pattern-id" placeholder:"ID" help:"attach a pattern identifier to findings"`
	Message              string   `arg:"--message" placeholder:"TEXT" help:"attach a message to findings"`
	JSON                 bool     `arg:"--json" help:"print the complete structural response as JSON"`
	MaxOutputBytes       int      `arg:"--max-output-bytes" default:"16384" placeholder:"N" help:"cap human-readable output (default 16384; 0 = unlimited; JSON is uncapped)"`
	ExcludeGlobs         []string `arg:"--exclude-glob,separate" placeholder:"GLOB" help:"exclude a source path; repeatable"`
	Repositories         []string `arg:"--repo,separate" placeholder:"PATTERN" help:"restrict remote repositories; repeatable"`
	ExcludeRepositories  []string `arg:"--exclude-repo,separate" placeholder:"PATTERN" help:"exclude remote repositories; repeatable"`
	Skip                 int      `arg:"--skip" placeholder:"N" help:"skip the first N ordered findings"`
	Limit                int      `arg:"--limit" default:"20" placeholder:"N" help:"return at most N findings (default 20; 0 = all local)"`
	MaxFiles             int      `arg:"--max-files" placeholder:"N" help:"bound eligible source files"`
	MaxTotalBytes        int64    `arg:"--max-total-bytes" placeholder:"N" help:"bound aggregate source bytes"`
	MaxPatternBytes      int      `arg:"--max-pattern-bytes" placeholder:"N" help:"bound query source bytes during compilation"`
	MaxRegexBytes        int      `arg:"--max-regex-bytes" placeholder:"N" help:"bound one regex constraint"`
	MaxRegexInstructions int      `arg:"--max-regex-instructions" placeholder:"N" help:"bound compiled regex instructions"`
	MaxParseDepth        int      `arg:"--max-parse-depth" placeholder:"N" help:"bound query and structural recursion depth"`
	MaxSourceBytes       int      `arg:"--max-source-bytes" placeholder:"N" help:"bound one source file"`
	MaxCandidates        int      `arg:"--max-candidates" placeholder:"N" help:"bound structural candidates per file"`
	MaxASTSteps          int      `arg:"--max-ast-steps" placeholder:"N" help:"bound structural evaluation steps per file"`
	MaxFindings          int      `arg:"--max-findings" placeholder:"N" help:"bound retained findings before paging"`
	MaxMemoryBytes       int      `arg:"--max-memory-bytes" placeholder:"N" help:"bound live scanner memory"`
	MaxFileMilliseconds  int64    `arg:"--max-file-time-ms" placeholder:"N" help:"bound evaluation time per file"`
	Workers              int      `arg:"--workers" placeholder:"N" help:"bound concurrent file evaluations"`
	TimeoutMilliseconds  int64    `arg:"--timeout-ms" placeholder:"N" help:"bound the complete structural scan in milliseconds"`
	Query                string   `arg:"positional" placeholder:"QUERY"`
	Globs                []string `arg:"positional" placeholder:"GLOB"`
}

func (gritArgs) Description() string {
	return "Run native GritQL structural search over every local Tree-sitter-backed language; use grit explain for compile-only inspection or add --remote to merge findings."
}

func parseGritArgs(args []string, outputs ...io.Writer) (gritArgs, error) {
	var output io.Writer
	if len(outputs) > 0 {
		output = outputs[0]
	}
	values := gritArgs{Limit: DefaultGritResultLimit, MaxOutputBytes: DefaultTextOutputBytes}
	argumentParser, err := arg.NewParser(arg.Config{Program: "grepple grit"}, &values)
	if err != nil {
		return values, err
	}
	if err := argumentParser.Parse(args); err != nil {
		if errors.Is(err, arg.ErrHelp) {
			if output == nil {
				output = os.Stdout
			}
			argumentParser.WriteHelp(output)
			return values, nil
		}
		return values, err
	}
	if err := validateGritArgs(values); err != nil {
		return values, err
	}
	return values, nil
}

func validateGritArgs(values gritArgs) error {
	if err := validateGritQuerySelection(values); err != nil {
		return err
	}
	if err := validateGritScope(values); err != nil {
		return err
	}
	return validateGritLimits(values)
}

func validateGritQuerySelection(values gritArgs) error {
	if values.Query != "" && values.QueryFile != "" {
		return fmt.Errorf("grit query text and query file cannot be used together")
	}
	if values.Query == "" && values.QueryFile == "" {
		return fmt.Errorf("grit requires query text or --query-file")
	}
	if values.Local && (values.Remote || values.Server != "") {
		return fmt.Errorf("--local cannot be combined with --remote or --server")
	}
	return nil
}

func validateGritScope(values gritArgs) error {
	if len(values.Repositories)+len(values.ExcludeRepositories) > 0 && !values.Remote && values.Server == "" {
		return fmt.Errorf("structural repository selectors require --remote or --server")
	}
	if len(values.Globs)+len(values.ExcludeGlobs) > wire.MaxGritGlobs || len(values.Repositories)+len(values.ExcludeRepositories) > wire.MaxGritRepositories {
		return fmt.Errorf("structural scope contains too many values")
	}
	if err := validateGritValueLengths(values.Globs, values.ExcludeGlobs, wire.MaxGritGlobBytes, "structural glob exceeds its maximum size"); err != nil {
		return err
	}
	return validateGritValueLengths(values.Repositories, values.ExcludeRepositories, wire.MaxGritRepositoryBytes, "repository identifier exceeds its maximum size")
}

func validateGritValueLengths(includes, excludes []string, maximum int, message string) error {
	for _, value := range append(append([]string{}, includes...), excludes...) {
		if len(value) > maximum {
			return fmt.Errorf("%s", message)
		}
	}
	return nil
}

func validateGritLimits(values gritArgs) error {
	if values.Skip < 0 || values.Limit < 0 {
		return fmt.Errorf("--skip and --limit must not be negative")
	}
	if !values.Local && (values.Remote || values.Server != "") && values.Limit > wire.MaxGritPageLimit {
		return fmt.Errorf("remote structural --limit must not exceed %d", wire.MaxGritPageLimit)
	}
	if gritHasNegativeLimit(values) {
		return fmt.Errorf("structural resource limits must not be negative")
	}
	if values.TimeoutMilliseconds > 300_000 || values.MaxFileMilliseconds > 10_000 {
		return fmt.Errorf("structural time limit exceeds its maximum")
	}
	if len(values.PatternID) > wire.MaxGritPatternIDBytes || len(values.Message) > wire.MaxGritMessageBytes {
		return fmt.Errorf("structural pattern identifier or message exceeds its maximum size")
	}
	return nil
}

func gritHasNegativeLimit(values gritArgs) bool {
	limits := []int64{
		int64(values.MaxFiles), values.MaxTotalBytes, int64(values.MaxPatternBytes), int64(values.MaxRegexBytes),
		int64(values.MaxRegexInstructions), int64(values.MaxParseDepth), int64(values.MaxSourceBytes),
		int64(values.MaxCandidates), int64(values.MaxASTSteps), int64(values.MaxFindings),
		int64(values.MaxMemoryBytes), int64(values.MaxOutputBytes), values.MaxFileMilliseconds, int64(values.Workers), values.TimeoutMilliseconds,
	}
	for _, limit := range limits {
		if limit < 0 {
			return true
		}
	}
	return false
}

func loadGritQuery(values gritArgs) (string, error) {
	if values.QueryFile == "" {
		if len(values.Query) > wire.MaxGritQueryBytes {
			return "", fmt.Errorf("structural query exceeds the %d-byte maximum", wire.MaxGritQueryBytes)
		}
		return values.Query, nil
	}
	if values.QueryFile == "-" {
		return readGritQuery(os.Stdin)
	}
	file, err := os.Open(values.QueryFile)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return readGritQuery(file)
}

func readGritQuery(reader io.Reader) (string, error) {
	content, err := io.ReadAll(io.LimitReader(reader, wire.MaxGritQueryBytes+1))
	if err != nil {
		return "", err
	}
	if len(content) > wire.MaxGritQueryBytes {
		return "", fmt.Errorf("structural query exceeds the %d-byte maximum", wire.MaxGritQueryBytes)
	}
	return string(content), nil
}

// Run executes structural query commands.
func (command *command) Run(args []string) error {
	dependencies := command.services()
	if len(args) > 0 && args[0] == "explain" {
		return runGritExplain(args[1:], dependencies)
	}
	values, err := parseGritArgs(args, dependencies.Stdout)
	if err != nil {
		return err
	}
	return command.execute(&values)
}

func (command *command) execute(values *Arguments) error {
	dependencies := command.services()
	if values.Query == "" && values.QueryFile == "" {
		return nil
	}
	if err := validateGritArgs(*values); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !values.Local && (values.Remote || values.Server != "") {
		return runGritRemote(ctx, *values, dependencies)
	}
	return runGritLocal(ctx, *values, dependencies)
}

func runGritLocal(ctx context.Context, values gritArgs, supplied ...Dependencies) error {
	dependencies := Dependencies{}
	if len(supplied) > 0 {
		dependencies = supplied[0]
	}
	return executeGrit(ctx, values, false, dependencies)
}

func runGritRemote(ctx context.Context, values gritArgs, dependencies Dependencies) error {
	return executeGrit(ctx, values, true, dependencies)
}

func executeGrit(ctx context.Context, values gritArgs, includeRemote bool, dependencies Dependencies) error {
	query, program, err := compileGritQuery(values)
	if err != nil {
		return err
	}
	local, err := acquireGritLocal(ctx, values, program, dependencies)
	if err != nil {
		return err
	}
	response := local
	if includeRemote {
		currentRepo := dependencies.currentRepository()
		request := gritRequest(values, query)
		if currentRepo != "" {
			request.ExcludeRepositories = appendUnique(request.ExcludeRepositories, currentRepo)
		}
		remote, requestErr := collectGritRemote(ctx, request, dependencies.serverDefault(values.Server), requiredGritRemoteFindings(values.Skip, values.Limit), dependencies)
		if requestErr != nil {
			return requestErr
		}
		response, err = mergeGritResponses(local, remote, currentRepo)
		if err != nil {
			return err
		}
	}
	response.Findings = windowGritFindings(response.Findings, values.Skip, values.Limit)
	response.ResultMetadata = dependencies.metadata(values, response, includeRemote)
	return outputGritResponse(values, response, dependencies)
}

func compileGritQuery(values gritArgs) (string, *gritql.Program, error) {
	query, err := loadGritQuery(values)
	if err != nil {
		return "", nil, err
	}
	program, err := gritql.Compile([]byte(query), gritql.CompileOptions{
		MaxPatternBytes: values.MaxPatternBytes, MaxRegexBytes: values.MaxRegexBytes,
		MaxRegexInstructions: values.MaxRegexInstructions, MaxDepth: values.MaxParseDepth,
	})
	if err != nil {
		return "", nil, formatGritCompileError(err)
	}
	return query, program, nil
}

func acquireGritLocal(ctx context.Context, values gritArgs, program *gritql.Program, dependencies Dependencies) (wire.GritResponse, error) {
	if err := ctx.Err(); err != nil {
		return wire.GritResponse{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return wire.GritResponse{}, err
	}
	candidates, err := gritCandidates(ctx, cwd, values.Globs, dependencies)
	if err != nil {
		return wire.GritResponse{}, err
	}
	result := gritql.ScanFiles(ctx, os.DirFS(cwd), program, candidates, gritScanOptions(values))
	if err := ctx.Err(); err != nil {
		return wire.GritResponse{}, err
	}
	response := gritResponse(result)
	response.Total = len(response.Findings)
	return response, nil
}

func gritCandidates(ctx context.Context, root string, globs []string, supplied ...Dependencies) ([]gritql.ScanCandidate, error) {
	dependencies := Dependencies{}
	if len(supplied) > 0 {
		dependencies = supplied[0]
	}
	params := search.Params{Files: true, Globs: globs, Root: root}
	if err := dependencies.applySourceConfig(&params); err != nil {
		return nil, err
	}
	paths, err := search.ListFilePathsContext(ctx, params, nil)
	if err != nil {
		return nil, err
	}
	candidates := make([]gritql.ScanCandidate, 0, len(paths))
	for _, candidatePath := range paths {
		normalized := filepath.ToSlash(candidatePath)
		candidates = append(candidates, gritql.ScanCandidate{ReadPath: normalized, Path: normalized})
	}
	return candidates, nil
}

func outputGritResponse(values gritArgs, response wire.GritResponse, dependencies Dependencies) error {
	if values.JSON {
		if err := stdoutWriter(dependencies).writeJSON(response); err != nil {
			return err
		}
	} else if err := renderGritHuman(response, values.MaxOutputBytes, dependencies); err != nil && !errors.Is(err, errOutputTruncated) {
		return err
	}
	if len(response.Findings) == 0 {
		dependencies.requestExit(1)
	}
	return nil
}

func gritRequest(values gritArgs, query string) wire.GritRequest {
	skip, limit := values.Skip, values.Limit
	request := wire.GritRequest{
		Query: query, Compatibility: wire.GritCompatibilityV1, PatternID: values.PatternID, Message: values.Message,
		Globs: append([]string(nil), values.Globs...), ExcludeGlobs: append([]string(nil), values.ExcludeGlobs...),
		Repositories: append([]string(nil), values.Repositories...), ExcludeRepositories: append([]string(nil), values.ExcludeRepositories...),
		Skip: &skip, Limit: &limit,
	}
	limits := wire.GritLimits{}
	setGritIntLimits(&limits, values)
	setGritInt64Limits(&limits, values)
	request.Limits = &limits
	return request
}

func setGritIntLimits(limits *wire.GritLimits, values gritArgs) {
	setPositiveInt(&limits.PatternBytes, values.MaxPatternBytes)
	setPositiveInt(&limits.RegexBytes, values.MaxRegexBytes)
	setPositiveInt(&limits.RegexInstructions, values.MaxRegexInstructions)
	setPositiveInt(&limits.ParseDepth, values.MaxParseDepth)
	setPositiveInt(&limits.SourceBytes, values.MaxSourceBytes)
	setPositiveInt(&limits.Candidates, values.MaxCandidates)
	setPositiveInt(&limits.ASTSteps, values.MaxASTSteps)
	setPositiveInt(&limits.Findings, values.MaxFindings)
	setPositiveInt(&limits.Files, values.MaxFiles)
	setPositiveInt(&limits.Workers, values.Workers)
}

func setGritInt64Limits(limits *wire.GritLimits, values gritArgs) {
	setPositiveInt64(&limits.FileTimeMillis, values.MaxFileMilliseconds)
	setPositiveInt64(&limits.BatchTimeMillis, values.TimeoutMilliseconds)
	setPositiveInt64(&limits.TotalBytes, values.MaxTotalBytes)
	memory := int64(values.MaxMemoryBytes)
	setPositiveInt64(&limits.MemoryBytes, memory)
}

func setPositiveInt(destination **int, value int) {
	if value > 0 {
		*destination = &value
	}
}

func setPositiveInt64(destination **int64, value int64) {
	if value > 0 {
		*destination = &value
	}
}

func gritScanOptions(values gritArgs) gritql.ScanOptions {
	evaluation := gritql.EvaluateOptions{
		MaxDepth:            values.MaxParseDepth,
		MaxCandidates:       values.MaxCandidates,
		MaxSteps:            values.MaxASTSteps,
		MaxFindings:         values.MaxFindings,
		MaxSourceBytes:      values.MaxSourceBytes,
		MaxMemoryBytes:      values.MaxMemoryBytes,
		DisableFileTimeout:  values.MaxFileMilliseconds == 0,
		DisableBatchTimeout: values.TimeoutMilliseconds == 0,
	}
	if values.MaxFileMilliseconds > 0 {
		evaluation.MaxElapsed = time.Duration(values.MaxFileMilliseconds) * time.Millisecond
	}
	if values.TimeoutMilliseconds > 0 {
		evaluation.MaxBatchElapsed = time.Duration(values.TimeoutMilliseconds) * time.Millisecond
	}
	return gritql.ScanOptions{
		EvaluateOptions: evaluation,
		PatternID:       values.PatternID,
		Message:         values.Message,
		ExcludeGlobs:    append([]string(nil), values.ExcludeGlobs...),
		Workers:         values.Workers,
		MaxFiles:        values.MaxFiles,
		MaxTotalBytes:   values.MaxTotalBytes,
	}
}

func formatGritCompileError(err error) error {
	var compileError *gritql.CompileError
	if !errors.As(err, &compileError) {
		return err
	}
	if compileError.Range == nil {
		return fmt.Errorf("%s: %s", compileError.Code, compileError.Message)
	}
	return fmt.Errorf("%s at %d:%d: %s", compileError.Code, compileError.Range.Start.Line, compileError.Range.Start.Column, compileError.Message)
}

func windowGritFindings(findings []wire.GritFinding, skip, limit int) []wire.GritFinding {
	if skip >= len(findings) {
		return findings[:0]
	}
	findings = findings[skip:]
	if limit > 0 && len(findings) > limit {
		findings = findings[:limit]
	}
	return findings
}

func renderGritHuman(response wire.GritResponse, maxOutputBytes int, supplied ...Dependencies) error {
	dependencies := Dependencies{}
	if len(supplied) > 0 {
		dependencies = supplied[0]
	}
	stdout, stderr := dependencies.Stdout, dependencies.Stderr
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	output := newBoundedOutputWriter(stdout, maxOutputBytes)
	for _, finding := range response.Findings {
		if err := renderGritFinding(output, finding); err != nil {
			return err
		}
	}
	return renderGritDiagnostics(newBoundedOutputWriter(stderr, maxOutputBytes), response)
}

func renderGritFinding(output *outputWriter, finding wire.GritFinding) error {
	rng := finding.Range
	path := finding.Path
	if finding.Repo != "" {
		path = finding.Repo + "/" + path
	}
	if err := output.writeString(fmt.Sprintf("%s:%d:%d-%d:%d\t%s\n", path, rng.Start.Line, rng.Start.Column, rng.End.Line, rng.End.Column, strconv.Quote(finding.Text))); err != nil {
		return err
	}
	for _, binding := range finding.Bindings {
		if err := output.writeString(fmt.Sprintf("  $%s\t%s\t%d:%d-%d:%d\n", binding.Name, binding.Kind, binding.Range.Start.Line, binding.Range.Start.Column, binding.Range.End.Line, binding.Range.End.Column)); err != nil {
			return err
		}
	}
	return nil
}

func renderGritDiagnostics(output *outputWriter, response wire.GritResponse) error {
	for _, diagnostic := range response.Diagnostics {
		if err := output.writeString(fmt.Sprintf("%s: %s\n", diagnostic.Code, diagnostic.Message)); err != nil {
			return err
		}
	}
	for _, truncation := range response.Truncations {
		if err := output.writeString(fmt.Sprintf("truncated: %s (limit %d, skipped %d)\n", truncation.Reason, truncation.Limit, truncation.Skipped)); err != nil {
			return err
		}
	}
	for _, shardError := range response.ShardErrors {
		if err := output.writeString(fmt.Sprintf("shard error: %s\n", shardError)); err != nil {
			return err
		}
	}
	if response.ResultMetadata != nil && response.ResultMetadata.NextCommand != "" {
		if err := output.writeString("continue: " + response.ResultMetadata.NextCommand + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func gritResponse(result gritql.ScanResult) wire.GritResponse {
	return gritqlapi.Response(result)
}
