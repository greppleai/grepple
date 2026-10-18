package search

import (
	"fmt"
	"github.com/greppleai/grepple/internal/directorymeta"
)

// compileSearchMetadataSelector validates every requested local selector before
// any router call or scan. Unsupported universes must fail, never run unfiltered.
func compileSearchMetadataSelector(values *Args) (directorymeta.SelectorV1, error) {
	selector, err := directorymeta.CompileSelectorsV1(directorymeta.SelectorsV1{
		Purposes: []string(values.Purposes), DomainAreas: []string(values.DomainAreas),
		FunctionalAreas: []string(values.FunctionalAreas), Keywords: []string(values.Keywords),
	})
	if err != nil || !selector.Requested() {
		return selector, err
	}
	for _, entry := range []struct {
		enabled bool
		name    string
	}{
		{values.Remote, "--remote"}, {values.RemoteOnly, "--remote-only"},
		{values.Server != "", "--server"}, {values.Cursor != "", "--cursor"},
		{len(values.Repos) > 0, "--repo"},
		{values.Related || values.FollowRelated > 0, "--related/--follow-related"},
	} {
		if entry.enabled {
			return directorymeta.SelectorV1{}, fmt.Errorf("local metadata selectors cannot be combined with %s", entry.name)
		}
	}
	return selector, nil
}
