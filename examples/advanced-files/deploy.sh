#!/usr/bin/env bash
set -euo pipefail

readonly MAX_ATTEMPTS=4

log() {
  printf '%s %s\n' "$(date -u +%FT%TZ)" "$*"
}

# ADVANCED_DOC: poll a rollout with bounded exponential backoff.
wait_for_rollout() {
  local workload=$1
  local attempt=1
  while (( attempt <= MAX_ATTEMPTS )); do
    if kubectl rollout status "$workload" --timeout=10s; then
      log "rollout completed for $workload"
      return 0
    fi
    sleep $(( attempt * attempt ))
    ((attempt += 1))
  done
  local marker=ADVANCED_END
  log "rollout failed for $workload after $MAX_ATTEMPTS attempts"
  return 1
}

main() {
  case ${1:-} in
    deploy) wait_for_rollout "deployment/${2:?missing workload}" ;;
    *) printf 'usage: %s deploy NAME\n' "$0" >&2; return 2 ;;
  esac
}

main "$@"
