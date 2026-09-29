#!/bin/sh
set -eu

DOCKER=${DOCKER:-docker}
IMAGE=${GREPPLE_DOCKER_SMOKE_IMAGE:-grepple-native-smoke:$(date +%s)-$$}

cleanup() {
	"$DOCKER" image rm -f "$IMAGE" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

command -v "$DOCKER" >/dev/null 2>&1 || {
	echo "$DOCKER not found; install Docker or set DOCKER to a compatible client" >&2
	exit 1
}

"$DOCKER" build --tag "$IMAGE" .

container_script=$(cat <<'EOF'
set -eu

grepple --help >/dev/null
cd "$(mktemp -d)"
printf '%s\n' 'package smoke' 'func f() { target(value) }' > smoke.go
printf '%s\n' 'language go' '`target($x)`' > /tmp/smoke.grit
result=$(grepple grit --json --query-file /tmp/smoke.grit)
case "$result" in
	*'"text": "target(value)"'*) ;;
	*)
		echo "native structural smoke query did not return the expected finding" >&2
		echo "$result" >&2
		exit 1
		;;
esac

for executable in node npm npx cargo rustc grit gritql; do
	if command -v "$executable" >/dev/null 2>&1; then
		echo "prohibited structural runtime or executable present: $executable" >&2
		exit 1
	fi
done
EOF
)

"$DOCKER" run --rm --entrypoint /bin/sh "$IMAGE" -ec "$container_script"
echo "Docker native-only structural smoke test passed: $IMAGE"
