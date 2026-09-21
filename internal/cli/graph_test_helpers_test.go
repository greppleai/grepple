package cli

import graphcommand "github.com/greppleai/grepple/internal/cli/graph"

type navigationGraphOutput = graphcommand.Output

const navigationGraphSchema = graphcommand.Schema

type navigationGraphTruncation = graphcommand.Truncation
type navigationSourceSummary = graphcommand.SourceSummary
type graphDiffOutput = graphcommand.DiffOutput
type graphResolveOutput = graphcommand.ResolveOutput

const navigationResolveSchema = graphcommand.NavigationResolveSchema

func runGraph(args []string) error { return runCommand(append([]string{"graph"}, args...)) }
func buildNavigationGraphOutputFromPaths(paths []string, maxFiles int) navigationGraphOutput {
	return graphcommand.BuildFromPaths(paths, maxFiles)
}
func graphContinuationCommand(mode string, paths []string, truncation *navigationGraphTruncation) string {
	return graphcommand.ContinuationCommand(mode, paths, truncation, graphcommand.Services{ActiveScopeFlags: appendActiveRepositoryScopeFlags})
}
func shortGraphID(id string) string { return graphcommand.ShortID(id) }
