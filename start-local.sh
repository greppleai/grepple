#!/usr/bin/env bash
# Start the local grepple cluster (router + two shards).
#
#   ./start-local.sh            # build and start in the background
#   ./start-local.sh logs -f    # follow logs
#   ./start-local.sh down       # stop containers, preserve repositories
#   ./start-local.sh down -v    # stop and delete repository volumes
set -euo pipefail

cd "$(dirname "$0")"

ENV_FILE="${ENV_FILE:-.env}"
if [[ ! -f "$ENV_FILE" ]]; then
  echo "No $ENV_FILE found. Create one from the template:" >&2
  echo "  cp .env.example $ENV_FILE" >&2
  echo "then configure GITHUB_TOKEN, WATCH_REPOS, and WATCH_ORGS." >&2
  exit 1
fi

compose() {
  docker compose --env-file "$ENV_FILE" "$@"
}

if [[ $# -gt 0 ]]; then
  compose "$@"
  exit
fi

echo "Starting grepple cluster using $ENV_FILE..."
compose up --build -d --wait --wait-timeout 240

router_port="$(compose port router 8080 | awk -F: 'END {print $NF}')"
shard_a_port="$(compose port shard-a 8787 | awk -F: 'END {print $NF}')"
shard_b_port="$(compose port shard-b 8787 | awk -F: 'END {print $NF}')"
router_port="${router_port:-8080}"
shard_a_port="${shard_a_port:-8787}"
shard_b_port="${shard_b_port:-8788}"
cat <<EOF

grepple cluster is up:
  router   http://localhost:${router_port}/health
  shards http://127.0.0.1:${shard_a_port}/health
          http://127.0.0.1:${shard_b_port}/health

Try:
  curl http://localhost:${router_port}/index
  curl -X POST http://localhost:${router_port}/search \
    -H 'content-type: application/json' \
    -d '{"query":"needle","regex":false}'

Use from the host CLI:
  GREPPLE_SERVER=http://localhost:${router_port} grepple needle

Follow logs: ./start-local.sh logs -f
Stop:        ./start-local.sh down
Delete data: ./start-local.sh down -v
EOF
