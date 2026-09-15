# Releases

Grepple releases are coordinated by Google's Release Please and start at `v0.0.1`. Conventional commits on `main` update a release pull request. Merging that pull request creates the GitHub release and tag, then invokes the native CGO build workflow to attach binaries.

## Release targets

| Operating system | Architecture | Archive |
| --- | --- | --- |
| Linux | amd64 | `.tar.gz` |
| Linux | arm64 | `.tar.gz` |
| macOS | amd64 | `.tar.gz` |
| macOS | arm64 | `.tar.gz` |
| Windows | amd64 | `.zip` |

Tree-sitter and the bundled grammars require CGO. Every target is therefore compiled and smoke-tested on a native GitHub-hosted runner rather than pretending that `CGO_ENABLED=0` or an unverified cross-compiler produces a supported binary. Linux release binaries use static external linking; macOS and Windows use their native system toolchains.

Every release includes `checksums.txt` with SHA-256 checksums and a GitHub build-provenance attestation. Artifact names follow `grepple_VERSION_GOOS_GOARCH.EXT`.

## Publishing

1. Merge conventional commits to `main`.
2. Review and merge the Release Please pull request.
3. Release Please creates the next `vX.Y.Z` tag and GitHub release.
4. `.github/workflows/release.yml` builds, smoke-tests, packages, attests, and uploads all five target archives.
5. Verify the release workflow and download one archive to confirm its version:

   ```bash
   grepple version
   ```

The release workflow is also triggered by a manually pushed semantic-version tag. Do not upload locally cross-compiled CGO binaries as supported release artifacts.

## Local validation

Install GoReleaser v2, then validate its build configuration:

```bash
make release-check
GREPPLE_BUILD_DATE="$(git show -s --format=%cI HEAD)" \
  goreleaser build --single-target --snapshot --clean --id grepple-linux --output dist/grepple
./dist/grepple version
```

Use `grepple-darwin` or `grepple-windows` only on the matching native operating system. The GitHub workflow pins the GoReleaser tool version used for official artifacts.

## Dependency updates

Dependabot checks the root Go module, the hooks Go module, and GitHub Actions weekly. Grouped pull requests still require normal test, race, vet, lint, schema, and benchmark review; automated updates do not bypass repository gates.
