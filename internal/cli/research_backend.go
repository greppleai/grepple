package cli

import (
	"context"
	"encoding/json"

	"charm.land/fantasy"
	askcommand "github.com/greppleai/grepple/internal/cli/ask"
)

type cliResearchBackend struct {
	root    string
	server  string
	session *researchSession
}

func (b *cliResearchBackend) Identity() string { return b.session.identity }
func (b *cliResearchBackend) Close()           { b.session.Close() }
func (b *cliResearchBackend) Search(ctx context.Context, in askcommand.SearchInput) (any, error) {
	return runAskSearch(ctx, b.root, b.server, askSearchInput{Query: in.Query, Paths: in.Paths, Repository: in.Repository, Regex: in.Regex, IgnoreCase: in.IgnoreCase, Mode: in.Mode, Limit: in.Limit, Context: in.Context})
}

func backendToolResult(value any, err error) (fantasy.ToolResponse, error) {
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}
	return fantasy.NewTextResponse(string(encoded)), nil
}
func (b *cliResearchBackend) Navigate(ctx context.Context, in askcommand.NavigateInput) (any, error) {
	return runAskNavigateWithSession(ctx, b.session, b.root, b.server, askNavigateInput{Location: in.Location, Repository: in.Repository, FollowDepth: in.FollowDepth})
}
func (b *cliResearchBackend) Structural(ctx context.Context, in askcommand.StructuralInput) (any, error) {
	return runAskStructural(ctx, b.root, b.server, askStructuralInput{Query: in.Query, Paths: in.Paths, Repository: in.Repository, Limit: in.Limit})
}
func (b *cliResearchBackend) Architecture(_ context.Context, in askcommand.ArchitectureInput) (any, error) {
	return runAskArchitectureWithSession(b.session, b.root, askArchitectureInput{Operation: in.Operation, Paths: in.Paths, Repository: in.Repository, Symbol: in.Symbol, From: in.From, To: in.To, MaxFiles: in.MaxFiles})
}
func (b *cliResearchBackend) Graph(_ context.Context, in askcommand.GraphInput) (any, error) {
	return runAskGraphWithSession(b.session, b.root, askGraphInput{Direction: in.Direction, Symbol: in.Symbol, Location: in.Location, Paths: in.Paths, Repository: in.Repository, Depth: in.Depth, Language: in.Language, Confidence: in.Confidence})
}
func (b *cliResearchBackend) SourceScope(_ context.Context, in askcommand.SourceScopeInput) (any, error) {
	return runAskSourceScope(b.root, askSourceScopeInput{Paths: in.Paths})
}
func (b *cliResearchBackend) RepositoryRefs(ctx context.Context, in askcommand.RepositoryRefsInput) (any, error) {
	return runAskRepositoryRefs(ctx, b.server, askRepositoryRefsInput{Repository: in.Repository, Kind: in.Kind})
}
func (b *cliResearchBackend) RepositoryTree(ctx context.Context, in askcommand.RepositoryTreeInput) (any, error) {
	return runAskRepositoryTree(ctx, b.server, askRepositoryTreeInput{Repository: in.Repository, Path: in.Path, Depth: in.Depth})
}
func (b *cliResearchBackend) Read(ctx context.Context, in askcommand.ReadInput) (fantasy.ToolResponse, error) {
	files := make([]readToolFileInput, 0, len(in.Files))
	for _, file := range in.Files {
		files = append(files, readToolFileInput{Path: file.Path, StartLine: file.StartLine, EndLine: file.EndLine, Outline: file.Outline})
	}
	return runAskReadTool(ctx, b.root, b.server, readToolInput{Path: in.Path, Files: files, Repository: in.Repository, StartLine: in.StartLine, EndLine: in.EndLine, Outline: in.Outline})
}
