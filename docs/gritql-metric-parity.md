# GritQL cognitive complexity versus Revive

Reproduce this checkout's audit (requires `grepple` and Revive **v1.12.0** in `PATH`):

```sh
go run ./scripts/cognitive-parity -show 20
go run ./scripts/cognitive-parity -include-generated -show 20
```

The audit compiles [`examples/go-cognitive.yaml`](../examples/go-cognitive.yaml), selects eligible Go sources with `grepple --files '**/*.go' --limit 0 --max-output-bytes 0`, and has Revive report **every** named function at threshold -1. It joins function declarations by repository-relative path and byte offset and then compares scores and the configured threshold **15**. It does not commit files or modify source. Revive directives still apply; `-include-generated` changes only Revive's generated-file policy.

A prior 705-file evaluation produced the following historical baseline (rerun the commands above for current results):

| Revive scope | Paired functions | Different scores | GritQL-only functions | >15 Revive | >15 GritQL | GritQL-only >15 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Normal Revive policy | 5,802 | 51 | 134 | 82 | 104 | 22 |
| Include generated files | 5,931 | 51 | 5 | 96 | 104 | 8 |

No Revive-only functions or Revive-only >15 warnings occurred in this selected source universe. All **51** score differences had a *higher* GritQL score: 30 by 1, five by 2, three by 3, seven by 4, and one each by 6, 10, 12, 15, 16, and 19. Normal Revive skips 129 generated functions and five `//revive:disable-next-line:cognitive-complexity`-suppressed functions that GritQL scores. Including generated files leaves the five directive-suppressed functions unpaired; those five are above 15. The other three extra >15 warnings are paired functions where scores cross the threshold.

Known non-parity mechanisms include lexical self-call matching rather than Go object identity and visiting expression/initializer subtrees that Revive's Go AST visitor deliberately skips. For example, an `if` initializer holding a closure can make the GritQL score substantially higher. A `//revive:disable...` directive and generated-file handling are also not implemented by the metric hook. **Keep Revive enabled; do not treat the cognitive example as a drop-in replacement.** Counts change as the source universe changes; rerun the script before considering migration.