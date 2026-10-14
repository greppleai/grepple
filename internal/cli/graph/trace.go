package graph

import (
	"context"

	"github.com/greppleai/grepple/internal/cliruntime"
	"github.com/greppleai/grepple/internal/clitelemetry"
	"github.com/greppleai/grepple/internal/search"
	"go.opentelemetry.io/otel/attribute"
)

// resolveCLIPaths measures the complete local graph source-selection operation.
// It does not precollect candidates or claim that source policy is glob time.
func resolveCLIPaths(application cliruntime.Context, globs []string) (paths []string, err error) {
	ctx, phase := clitelemetry.StartPhase(application.Invocation(), "cli.sources.resolve")
	defer func() {
		if err == nil && phase.IsRecording() {
			phase.SetAttributes(attribute.Int("cli.sources.count", len(paths)))
		}
		clitelemetry.EndPhase(phase, ctx, err)
	}()
	return resolveInputPathsContext(ctx, globs, search.SourcePolicyConfigurer(application.Repository()))
}

// lookupDaemon observes exactly one actual daemon lookup, not a snapshot open.
// A failed, absent or invalid response is a miss. It does not time fallback builds.
func lookupDaemon(ctx context.Context, lookup func() bool) {
	lookupCtx, phase := clitelemetry.StartPhase(ctx, "cli.navigation.cache_lookup")
	hit := lookup()
	if phase.IsRecording() {
		state := "miss"
		if hit {
			state = "hit"
		}
		phase.SetAttributes(attribute.String("cli.cache.kind", "daemon"), attribute.String("cli.cache.state", state))
	}
	clitelemetry.EndPhase(phase, lookupCtx, nil)
}
