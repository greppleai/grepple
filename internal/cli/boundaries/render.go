package boundaries

import (
	"strings"

	"github.com/greppleai/grepple/search"
)

func writeBoundaryFacadeBypasses(write func(string, ...any) bool, bypasses []search.BoundaryFacadeBypass, limit int) bool {
	if !write("\nfacade bypass signals:") {
		return false
	}
	if len(bypasses) == 0 {
		return write("  (none)")
	}
	visible := boundaryVisibleCount(len(bypasses), limit)
	for _, bypass := range bypasses[:visible] {
		if !write("  ! %s: %s:%d %s -> %s:%d %s [%s]", bypass.Facade, bypass.Caller.Path, bypass.Caller.Line, bypass.Caller.Name, bypass.Target.Path, bypass.Target.Line, bypass.Target.Name, bypass.Confidence) {
			return false
		}
	}
	if omitted := len(bypasses) - visible; omitted > 0 {
		return write("  +%d additional facade bypass signals; use --limit 0 or --json for all", omitted)
	}
	return true
}

func writeBoundarySignals(write func(string, ...any) bool, candidate search.BoundaryCandidate) bool {
	if len(candidate.Signals) == 0 {
		return true
	}
	return write("  signals: %s", strings.Join(candidate.Signals, ", "))
}
