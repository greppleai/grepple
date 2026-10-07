#!/usr/bin/env sh
# Install the latest published CLI; never build source or modify shell profiles.
set -eu

fail() { printf 'grepple install: %s\n' "$*" >&2; exit 1; }
usage() {
    printf '%s\n' 'Usage: sh install.sh [--bin-dir DIRECTORY]' \
        'Installs the latest GitHub release on Linux/macOS (amd64/arm64).' \
        'Default: GREPPLE_BIN_DIR, or $HOME/.local/bin. No sudo or profile edits.'
}
bin_dir=${GREPPLE_BIN_DIR:-${HOME:?HOME must be set}/.local/bin}
while [ "$#" -gt 0 ]; do
    case "$1" in
        --bin-dir) [ "$#" -ge 2 ] || fail '--bin-dir requires a directory'; bin_dir=$2; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) usage >&2; fail "unknown argument: $1" ;;
    esac
done
[ -n "$bin_dir" ] || fail 'install directory cannot be empty'
case "$bin_dir" in /*) ;; *) bin_dir="$PWD/$bin_dir" ;; esac

case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) fail 'unsupported OS; download a Windows binary from GitHub Releases instead' ;;
esac
case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) fail 'unsupported architecture; supported: amd64 and arm64' ;;
esac
for dependency in curl tar mktemp awk; do
    command -v "$dependency" >/dev/null 2>&1 || fail "missing required command: $dependency"
done
if command -v sha256sum >/dev/null 2>&1; then
    checksum_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    checksum_tool=shasum
else
    fail 'install sha256sum or shasum to verify release checksums'
fi

stage=$(mktemp -d)
install_tmp=
cleanup() {
    rm -rf "$stage"
    [ -z "$install_tmp" ] || rm -f "$install_tmp"
}
trap cleanup 0
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
fetch() {
    curl --disable --fail --silent --show-error --location --max-redirs 5 \
        --proto '=https' --proto-redir '=https' --connect-timeout 10 \
        --max-time 120 --max-filesize 134217728 "$@"
}
release_root=https://github.com/greppleai/grepple/releases
latest=$(fetch --output /dev/null --write-out '%{url_effective}' "$release_root/latest")
case "$latest" in
    "$release_root/tag/"*) tag=${latest#"$release_root/tag/"} ;;
    *) fail 'could not resolve the latest GitHub release' ;;
esac
printf '%s\n' "$tag" | awk '/^v[0-9]+\.[0-9]+\.[0-9]+([-+][A-Za-z0-9.+-]+)?$/ {ok=1} END {exit !ok}' \
    || fail 'unexpected release tag'
asset="grepple_${tag#v}_${os}_${arch}.tar.gz"
printf 'Downloading grepple %s (%s/%s)\n' "$tag" "$os" "$arch"
fetch --output "$stage/$asset" "$release_root/download/$tag/$asset"
fetch --output "$stage/checksums.txt" "$release_root/download/$tag/checksums.txt"
expected=$(awk -v name="$asset" '$2 == name {print $1; count++} END {if (count != 1) exit 1}' "$stage/checksums.txt") \
    || fail 'release manifest must contain exactly one checksum for this archive'
case "$expected" in ''|*[!0-9a-f]*) fail 'invalid release checksum' ;; esac
[ "${#expected}" -eq 64 ] || fail 'invalid release checksum length'
if [ "$checksum_tool" = sha256sum ]; then
    actual=$(sha256sum "$stage/$asset" | awk '{print $1}')
else
    actual=$(shasum -a 256 "$stage/$asset" | awk '{print $1}')
fi
[ "$actual" = "$expected" ] || fail 'checksum mismatch; existing installation left unchanged'
# Release archives contain exactly one root-level executable. Reject extra paths.
members=$(tar -tzf "$stage/$asset")
[ "$members" = grepple ] || fail 'unexpected release archive contents'
tar -xzf "$stage/$asset" -C "$stage" grepple
[ -f "$stage/grepple" ] && [ ! -L "$stage/grepple" ] || fail 'archive executable must be a regular file'
[ ! -d "$bin_dir/grepple" ] && [ ! -L "$bin_dir/grepple" ] \
    || fail 'destination grepple is a directory or symlink; refusing to replace it'
mkdir -p "$bin_dir"
install_tmp=$(mktemp "$bin_dir/.grepple-install.XXXXXX")
cp "$stage/grepple" "$install_tmp"
chmod 0755 "$install_tmp"
mv -f "$install_tmp" "$bin_dir/grepple"
install_tmp=
printf 'Installed grepple %s to %s/grepple\n' "$tag" "$bin_dir"
case ":${PATH:-}:" in
    *":$bin_dir:"*) printf '%s\n' 'Run: grepple --version' ;;
    *) printf 'Add %s to PATH in your shell profile, then run grepple --version.\n' "$bin_dir" ;;
esac
