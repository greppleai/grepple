#!/usr/bin/env sh
set -eu

bin_dir="${GREPPLE_BIN_DIR:-$HOME/.local/bin}"
zoekt_version="${GREPPLE_ZOEKT_VERSION:-v0.0.0-20260814112500-b0de0bb820f5}"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT HUP INT TERM

for command in grepple shard router; do
  CGO_ENABLED=1 go build -tags='netgo osusergo' -trimpath \
    -ldflags='-s -w -linkmode external -extldflags -static' \
    -o "$stage/$command" "./cmd/$command"
done

CGO_ENABLED=0 GOBIN="$stage" go install -trimpath -ldflags='-s -w' \
  "github.com/sourcegraph/zoekt/cmd/zoekt-git-index@$zoekt_version" \
  "github.com/sourcegraph/zoekt/cmd/zoekt-webserver@$zoekt_version"

mkdir -p "$bin_dir"
for command in grepple shard router zoekt-git-index zoekt-webserver; do
  install -m 0755 "$stage/$command" "$bin_dir/$command"
  printf 'installed %s\n' "$bin_dir/$command"
done
