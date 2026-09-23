// Package tree implements local and indexed-repository tree rendering.
package tree

import (
	"github.com/greppleai/grepple/api"
	"github.com/greppleai/grepple/internal/cliruntime"
	sourcedomain "github.com/greppleai/grepple/internal/sources"
)

type localTree func(path string, depth int, kind sourcedomain.Kind) (api.TreeResponse, error)

func newLocal(context cliruntime.Context) localTree {
	return func(path string, depth int, kind sourcedomain.Kind) (api.TreeResponse, error) {
		return buildLocal(path, depth, kind, context.Repository())
	}
}
