#!/usr/bin/env bash
# Run the Go API and the SvelteKit dev server side by side.
set -euo pipefail
cd "$(dirname "$0")/.."

# Load .env without overriding variables already exported in the shell, so
# one-off overrides like `OBSERVAKI_HTTP_ADDR=:18080 make dev` still work.
load_env_file() {
	local line key value
	while IFS= read -r line || [ -n "$line" ]; do
		case "$line" in
		'' | '#'*) continue ;;
		esac
		key="${line%%=*}"
		value="${line#*=}"
		key="${key//[[:space:]]/}"
		[ -n "$key" ] || continue
		if [ -z "${!key+x}" ]; then
			export "$key=$value"
		fi
	done <"$1"
}

if [ -f .env ]; then
	load_env_file .env
fi

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

api_url="http://localhost${OBSERVAKI_HTTP_ADDR}"
case "$OBSERVAKI_HTTP_ADDR" in
:* ) ;;
* ) api_url="http://${OBSERVAKI_HTTP_ADDR}" ;;
esac

echo "API: ${api_url}"
echo "Web: http://localhost:${WEB_PORT}"

go run ./cmd/observaki serve &
pids+=("$!")

pnpm --dir web dev &
pids+=("$!")

# Stop everything as soon as one process exits.
wait -n "${pids[@]}"
