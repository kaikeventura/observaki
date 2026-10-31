#!/usr/bin/env bash
# Run the Go API and the SvelteKit dev server side by side.
set -euo pipefail
cd "$(dirname "$0")/.."

: "${OBSERVAKI_HTTP_ADDR:=:8080}"
: "${WEB_PORT:=5173}"
export OBSERVAKI_HTTP_ADDR WEB_PORT

pids=()

cleanup() {
	local status=$?
	trap - EXIT INT TERM
	for pid in "${pids[@]}"; do
		kill "$pid" 2>/dev/null || true
	done
	wait 2>/dev/null || true
	exit "$status"
}
trap cleanup EXIT INT TERM

echo "API: http://localhost${OBSERVAKI_HTTP_ADDR}"
echo "Web: http://localhost${WEB_PORT}"

go run ./cmd/observaki serve &
pids+=("$!")

pnpm --dir web dev &
pids+=("$!")

# Stop everything as soon as one process exits.
wait -n "${pids[@]}"
