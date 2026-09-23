package ask

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"sync/atomic"

	publicanalysis "github.com/greppleai/grepple/analysis"
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/parser"
	"github.com/greppleai/grepple/search"
)

type localResearchUniversePlan struct {
	paths    []string
	maxFiles int
	key      string
}

type localResearchUniverse struct {
	once sync.Once
	plan localResearchUniversePlan

	err       error
	universe  *publicanalysis.Universe
	analysis  *search.NavigationAnalysis
	closeOnce sync.Once
}

var researchUniverseBuilds atomic.Int64

func planLocalResearchUniverse(application cliruntime.Context, globs []string, maxFiles int) (localResearchUniversePlan, error) {
	params := search.Params{Files: true, Globs: globs}
	if err := search.ConfigureSourcePolicy(&params, application.Repository()); err != nil {
		return localResearchUniversePlan{}, err
	}
	paths, err := search.ListFilePaths(params, nil)
	if err != nil {
		return localResearchUniversePlan{}, err
	}
	plan := localResearchUniversePlan{paths: paths, maxFiles: maxFiles}
	encoded, _ := json.Marshal(struct {
		Paths    []string `json:"paths"`
		MaxFiles int      `json:"maxFiles"`
	}{plan.paths, maxFiles})
	digest := sha256.Sum256(encoded)
	plan.key = hex.EncodeToString(digest[:])
	return plan, nil
}

func (universe *localResearchUniverse) load() {
	researchUniverseBuilds.Add(1)
	universe.universe, universe.err = publicanalysis.NewUniverse(publicanalysis.ReadSources(universe.plan.paths), universe.plan.maxFiles)
	if universe.err != nil {
		return
	}
	universe.analysis = universe.universe.NavigationAnalysis()
}

func (universe *localResearchUniverse) document(path string) *parser.Document {
	if universe == nil || universe.universe == nil {
		return nil
	}
	return universe.universe.Document(path)
}

func (universe *localResearchUniverse) architecture() publicanalysis.ArchitectureReport {
	return publicanalysis.BuildArchitecture(universe.universe)
}

func (universe *localResearchUniverse) close() {
	if universe == nil {
		return
	}
	universe.closeOnce.Do(func() {
		if universe.universe != nil {
			universe.universe.Close()
		}
	})
}
