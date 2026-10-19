# Documentation

Start with the [README](../README.md) for installation and a first search. Command-specific `--help` is authoritative for flags; `grepple examples` prints short, runnable workflows.

## Guides

| Task | Guide |
| --- | --- |
| Scope files and interpret results | [Source selection](source-scope.md) · [Output contracts](output-contracts.md) · [File types](file-type-support.md) |
| Find and follow code | [Verified selectors](at-verified-scope-selectors.md) · [Line-only scopes](line-only-scope-annotations.md) · [Directory architecture](directory-architecture.md) |
| Change code safely | [Transactional writes](write.md) · [Edit-anchor providers](anchor-providers.md) |
| Keep local research notes | [Repository memories](repository-memories.md) |
| Search patterns and rules | [GritQL compatibility](gritql-compatibility.md) · [Hook assertions](hook-string-assertions.md) |
| Use an indexed service | [Versioned indexing](versioned-indexing.md) |
| Configure integrations | [Agent skills](agent-setup.md) · [Ask research](ask.md) · [CLI telemetry](cli-telemetry.md) |
| Build and ship | [Releases](releases.md) · [Changelog](CHANGELOG.md) |

## Engineering records

[Backend navigation rollout](backend-scoped-navigation.md) · [Navigation storage investigation](navigation-snapshot-storage.md) · [Architecture benchmarks](architecture-performance-benchmarks.md)

Other files include contracts, measured benchmarks, rollout checkpoints, and dated design investigations. Treat these as **context**, not current CLI instructions; verify commands against `--help` and implementation claims against source. Historical changelog entries are intentionally preserved.
