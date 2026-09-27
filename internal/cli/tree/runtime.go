// Package tree implements local and indexed-repository tree rendering.
package tree

import (
	"github.com/greppleai/grepple/internal/wire"
	"github.com/greppleai/grepple/internal/cliruntime"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type localTree func(path string, depth int, kind sourcedomain.Kind, areas []string) (wire.TreeResponse, error)

func newLocal(context cliruntime.Context) localTree {
	return func(path string, depth int, kind sourcedomain.Kind, areas []string) (wire.TreeResponse, error) {
		return buildLocal(path, depth, kind, areas, context.Repository())
	}
}
