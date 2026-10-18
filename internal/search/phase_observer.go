package search

import "context"

// LocalPhaseObserver observes a single bounded operation, not each file or worker.
// Names are fixed by the search engine and must never contain user input.
type LocalPhaseObserver func(name string) func(error)

type localPhaseKey struct{}

// WithLocalPhaseObserver attaches phase observation to one invocation only.
func WithLocalPhaseObserver(ctx context.Context, observer LocalPhaseObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, localPhaseKey{}, observer)
}

// BeginLocalPhase is also used by the streaming outline renderer to measure
// its inclusive read/parse/write batch without introducing per-file spans.
func BeginLocalPhase(ctx context.Context, name string) func(error) {
	return beginLocalPhase(ctx, name)
}

func beginLocalPhase(ctx context.Context, name string) func(error) {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(localPhaseKey{}).(LocalPhaseObserver)
	if observer == nil {
		return nil
	}
	return observer(name)
}
