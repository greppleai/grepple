#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/mock" "$work/fixture" "$work/home"
export INSTALL_TEST_ROOT="$root" INSTALL_TEST_WORK="$work" HOME="$work/home"
export PATH="$work/mock:$PATH"
export GREPPLE_BIN_DIR="$work/bin"

cat > "$work/mock/uname" <<'SH'
#!/bin/sh
case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) exit 1 ;; esac
SH
cat > "$work/mock/curl" <<'SH'
#!/bin/sh
set -eu
output=
url=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --output) output=$2; shift 2 ;;
        --write-out) shift 2 ;;
        https://*) url=$1; shift ;;
        *) shift ;;
    esac
done
case "$url" in
    https://raw.githubusercontent.com/greppleai/grepple/main/install.sh) source="$INSTALL_TEST_ROOT/install.sh" ;;
    https://github.com/greppleai/grepple/releases/latest)
        printf '%s' 'https://github.com/greppleai/grepple/releases/tag/v9.9.9'; exit 0 ;;
    https://github.com/greppleai/grepple/releases/download/v9.9.9/checksums.txt) source="$INSTALL_TEST_WORK/checksums.txt" ;;
    https://github.com/greppleai/grepple/releases/download/v9.9.9/grepple_9.9.9_linux_amd64.tar.gz)
        source="$INSTALL_TEST_WORK/grepple_9.9.9_linux_amd64.tar.gz" ;;
    *) echo "unexpected URL: $url" >&2; exit 1 ;;
esac
if [ -n "$output" ]; then cp "$source" "$output"; else cat "$source"; fi
SH
chmod +x "$work/mock/curl" "$work/mock/uname"

printf '%s\n' '#!/bin/sh' 'echo "grepple fixture"' > "$work/fixture/grepple"
archive=grepple_9.9.9_linux_amd64.tar.gz
tar -czf "$work/$archive" -C "$work/fixture" grepple
if command -v sha256sum >/dev/null 2>&1; then
    digest=$(sha256sum "$work/$archive" | awk '{print $1}')
else
    digest=$(shasum -a 256 "$work/$archive" | awk '{print $1}')
fi
printf '%s  %s\n' "$digest" "$archive" > "$work/checksums.txt"

url=https://raw.githubusercontent.com/greppleai/grepple/main/install.sh
curl -fsSL "$url" | sh
[ "$("$GREPPLE_BIN_DIR/grepple" --version)" = 'grepple fixture' ]
curl -fsSL "$url" | sh -s -- --bin-dir "$work/custom"
[ "$("$work/custom/grepple" --version)" = 'grepple fixture' ]

# Failed verification must preserve the previously installed executable.
printf '%064d  %s\n' 0 "$archive" > "$work/checksums.txt"
if curl -fsSL "$url" | sh -s -- --bin-dir "$work/custom" >/dev/null 2>&1; then
    echo 'installer accepted a bad checksum' >&2
    exit 1
fi
[ "$("$work/custom/grepple" --version)" = 'grepple fixture' ]
printf '%s\n' 'streamed installer and checksum rollback passed'
