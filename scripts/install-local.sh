#!/usr/bin/env sh
set -eu

bin_dir="${GREPPLE_BIN_DIR:-$HOME/.local/bin}"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT HUP INT TERM

CGO_ENABLED=1 go build -tags='netgo osusergo' -trimpath \
  -ldflags='-s -w -linkmode external -extldflags -static' \
  -o "$stage/grepple" ./cmd/grepple

mkdir -p "$bin_dir"
install -m 0755 "$stage/grepple" "$bin_dir/grepple"
printf 'installed %s\n' "$bin_dir/grepple"
