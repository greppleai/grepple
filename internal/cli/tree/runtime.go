// Package tree implements local and indexed-repository tree rendering.
package tree

import (
	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/directorymeta"
	"github.com/greppleai/grepple/internal/wire"
)

type localTree func(path string, depth int, selector directorymeta.SelectorV1) (wire.TreeResponse, error)

func newLocal(context cliruntime.Context) localTree {
	return func(path string, depth int, selector directorymeta.SelectorV1) (wire.TreeResponse, error) {
		return buildLocalSelected(path, depth, selector, context.Repository())
	}
}
