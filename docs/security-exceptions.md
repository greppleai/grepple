# Known, unfixable Trivy findings

## GO-2026-5932 — golang.org/x/crypto/openpgp

- **Where it appears**: `go.mod`, and the compiled `shard`, `router`,
  `zoekt-git-index`, and `zoekt-webserver` binaries in the runtime image.
- **Why there is no fix**: the advisory (https://vuln.go.dev/ID/GO-2026-5932.json,
  tracked upstream at https://go.dev/issue/44226) is a design-level warning —
  `golang.org/x/crypto/openpgp` and its subpackages are "unsafe by design,
  have numerous known security issues, [and are] not maintained" — there is
  no patched version to upgrade to for any subpackage.
- **Why it's safe to leave undismissed rather than ignored**: `golang.org/x/crypto`
  is present in this repo's dependency graph only as an indirect,
  graph-pruning requirement — confirmed via `go mod why golang.org/x/crypto`
  ("main module does not need package golang.org/x/crypto") and by
  reproducing a byte-identical `go.mod`/`go.sum` after `go mod tidy`. No code
  in this repository, nor in `zoekt`, imports `golang.org/x/crypto/openpgp`
  directly. Upstream `sourcegraph/zoekt` reached the identical conclusion for
  the same advisory (see commit `14ce696`, PR #1098): "this package isn't
  actually used, trivy just isn't smart enough to know that vs which modules
  we use."
- **Revisit when**: the Go vulnerability database publishes a fixed version
  for `golang.org/x/crypto/openpgp`, or govulncheck-style call-graph analysis
  is available to confirm non-reachability programmatically instead of by
  manual inspection.
- **Not added to `.trivyignore`**: per policy, unfixable findings are
  documented rather than suppressed so they remain visible in scan output.

### Re-verified 2026-08-28

- Re-ran the detection independently this pass rather than relying on the
  write-up above: `grep -rn "openpgp" --include="*.go" .` — no hits anywhere
  in this repo. `go mod graph | grep golang.org/x/crypto` — only transitive
  edges from `github.com/gofiber/fiber/v3` and `github.com/valyala/fasthttp`
  (plus their onward edges to `x/net`, `x/sys`, `x/term`, `x/text`). `go mod
  why golang.org/x/crypto` — confirms `main module does not need package
  golang.org/x/crypto`. `trivy fs --scanners vuln --format table .` —
  `GO-2026-5932` still reported with a blank Fixed Version column.
- Conclusion is unchanged: no patched version exists upstream, and the
  package remains unreachable from any code path this repo or `zoekt`
  actually exercises. `.trivyignore` was not modified — this finding remains
  intentionally undismissed rather than suppressed.
