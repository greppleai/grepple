package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"

	architecturecommand "github.com/greppleai/grepple/internal/cli/architecture"
	graphcommand "github.com/greppleai/grepple/internal/cli/graph"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type localResearchUniversePlan struct {
	paths       []string
	discovered  int
	unsupported int
	truncation  *graphcommand.Truncation
	key         string
}

type localResearchUniverse struct {
	once sync.Once
	plan localResearchUniversePlan

	err        error
	sources    []architecturecommand.ParsedSource
	documents  map[string]*parser.Document
	analysis   *search.NavigationAnalysis
	graph      parser.NavigationGraph
	parseStats search.NavigationSourceStats
	graphStats search.NavigationSourceStats
	closeOnce  sync.Once
}

var researchUniverseBuilds atomic.Int64

func planLocalResearchUniverse(globs []string, maxFiles int) (localResearchUniversePlan, error) {
	paths, err := graphcommand.ResolveInputPaths(globs, applyRepositorySourceConfig)
	if err != nil {
		return localResearchUniversePlan{}, err
	}
	plan := localResearchUniversePlan{discovered: len(paths)}
	plan.paths = graphcommand.SourcePaths(paths)
	if maxFiles > 0 && len(plan.paths) > maxFiles {
		plan.truncation = &graphcommand.Truncation{Reason: "max_files", Limit: maxFiles, Skipped: len(plan.paths) - maxFiles}
		plan.paths = plan.paths[:maxFiles]
	}
	encoded, _ := json.Marshal(struct {
		Paths      []string `json:"paths"`
		Discovered int      `json:"discovered"`
		MaxFiles   int      `json:"maxFiles"`
	}{plan.paths, plan.discovered, maxFiles})
	digest := sha256.Sum256(encoded)
	plan.key = hex.EncodeToString(digest[:])
	return plan, nil
}

func (universe *localResearchUniverse) load() {
	researchUniverseBuilds.Add(1)
	universe.sources, universe.parseStats = architecturecommand.LoadDocuments(universe.plan.paths)
	universe.analysis, universe.graphStats = search.BuildNavigationAnalysisFromDocuments(architecturecommand.NavigationDocuments(universe.sources), search.NavigationBuildOptions{})
	universe.graph = universe.analysis.Graph()
	universe.documents = make(map[string]*parser.Document, len(universe.sources))
	for _, source := range universe.sources {
		absolute, err := filepath.Abs(source.Path)
		if err != nil {
			continue
		}
		universe.documents[filepath.Clean(absolute)] = source.Document
	}
}

func (universe *localResearchUniverse) document(path string) *parser.Document {
	if universe == nil {
		return nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	return universe.documents[filepath.Clean(absolute)]
}

func (universe *localResearchUniverse) architecture() architecturecommand.Report {
	var truncation *architecturecommand.Truncation
	if universe.plan.truncation != nil {
		truncation = &architecturecommand.Truncation{Reason: universe.plan.truncation.Reason, Limit: universe.plan.truncation.Limit, Skipped: universe.plan.truncation.Skipped}
	}
	return architecturecommand.BuildFromParts(universe.plan.paths, universe.plan.discovered, universe.plan.discovered-universe.plan.unsupported, truncation, universe.sources, universe.parseStats, universe.graph, universe.graphStats)
}

func (universe *localResearchUniverse) navigationOutput() graphcommand.Output {
	output := graphcommand.FromParts(universe.plan.paths, universe.plan.discovered, universe.plan.unsupported, universe.plan.truncation, universe.graph, universe.graphStats)
	output.Sources = graphcommand.SourceSummary{
		Discovered: universe.plan.discovered,
		Selected:   len(universe.plan.paths),
		Parsed:     universe.graphStats.Parsed,
		Skipped:    universe.plan.unsupported + universe.parseStats.Skipped + universe.graphStats.Skipped,
		Failed:     universe.parseStats.Failed + universe.graphStats.Failed,
		Recovered:  universe.graphStats.Recovered,
	}
	return output
}

func (universe *localResearchUniverse) close() {
	if universe == nil {
		return
	}
	universe.closeOnce.Do(func() {
		for _, source := range universe.sources {
			source.Document.Close()
		}
	})
}
