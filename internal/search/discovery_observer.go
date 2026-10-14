package search

import "context"

// CandidateDiscoveryObserver observes only one whole candidate collection call.
// Implementations must be nonblocking and must not inspect or export source paths.
// The finish callback receives an aggregate count and error, never file names.
type CandidateDiscoveryObserver func(mode string) func(count int, err error)

type candidateDiscoveryKey struct{}

// WithCandidateDiscoveryObserver explicitly opts one request into observation.
// Core consumers that do not supply the observer retain the original behavior.
func WithCandidateDiscoveryObserver(ctx context.Context, observer CandidateDiscoveryObserver) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, candidateDiscoveryKey{}, observer)
}

func beginCandidateDiscovery(ctx context.Context, mode string) func(int, error) {
	observer, _ := ctx.Value(candidateDiscoveryKey{}).(CandidateDiscoveryObserver)
	if observer == nil {
		return nil
	}
	return observer(mode)
}
