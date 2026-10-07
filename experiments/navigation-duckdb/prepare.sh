#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
DATA="${1:-/workspace/.tmp/navigation-duckdb}"
REPOS="${REPOS:-/workspace/grepple,/workspace/grepple-backend}"
mkdir -p "$DATA"
chmod 700 "$DATA"
# -mod=mod avoids an existing upstream Swift/Dart dependency-test tidy failure.
# No production go.mod or go.sum is changed by this nested experiment module.
go build -mod=mod -o "$DATA/go-index" .
go build -mod=mod -tags duckdb -o "$DATA/duck-index" .
for pair in base:1 scale10:10; do
  name="${pair%:*}"
  count="${pair#*:}"
  if [[ -e "$DATA/$name.duckdb" || -e "$DATA/$name.column.duckdb" ]]; then
    echo "Refusing to overwrite $DATA/$name.duckdb; choose a fresh data directory" >&2
    exit 1
  fi
  "$DATA/go-index" -mode export -repos "$REPOS" -replicas "$count" -out "$DATA/$name.json"
  "$DATA/go-index" -mode pack -input "$DATA/$name.json" -out "$DATA/$name.pb" > "$DATA/$name.pack.json"
  "$DATA/duck-index" -mode build -input "$DATA/$name.json" -out "$DATA/$name.duckdb" > "$DATA/$name.build.json"
  "$DATA/duck-index" -mode build -backend duckdb-col -input "$DATA/$name.json" -out "$DATA/$name.column.duckdb" > "$DATA/$name.column.build.json"
  chmod 600 "$DATA/$name.duckdb" "$DATA/$name.column.duckdb"
done
python3 run.py --data "$DATA"