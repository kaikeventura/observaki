#!/usr/bin/env bash
# Reproduce the Story 5.3 quickstart and time it (AC1).
#
#   ./deploy/compose/quickstart-check.sh --fresh
#   ./deploy/compose/quickstart-check.sh --no-build
#
# The check follows the documented quickstart: start the single-node stack,
# wait for every healthcheck, send the sample telemetry and verify that logs,
# traces and the sample metric landed. It fails if the whole path takes more
# than 15 minutes (900 s).
#
# --fresh removes the Compose volumes and the local observaki image first, so
# the run matches a first-time clone. It destroys local evaluation data.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

FRESH=0
BUILD=1
LIMIT=${OBSERVAKI_QUICKSTART_LIMIT:-900}
DEADLINE=120
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
REPORT_DIR="$ROOT/deploy/compose/results/$STAMP"

log() { printf '[quickstart] %s\n' "$*" >&2; }
die() {
	log "FAIL: $*"
	log "hint: docker compose logs --tail 100"
	exit 1
}

usage() {
	cat <<'EOF'
Usage: deploy/compose/quickstart-check.sh [options]

Options:
  --fresh        remove the Compose volumes and the observaki image first
  --no-build     reuse the local observaki image
  --limit N      maximum seconds for the whole quickstart (default: 900)
  --help         show this help

The report is written to deploy/compose/results/<UTC timestamp>/quickstart-report.md.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--fresh)
		FRESH=1
		shift
		;;
	--no-build)
		BUILD=0
		shift
		;;
	--limit)
		LIMIT=${2:?--limit requires a value}
		shift 2
		;;
	--help | -h)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

command -v docker >/dev/null || die "docker is required"
command -v curl >/dev/null || die "curl is required"
command -v jq >/dev/null || die "jq is required"
docker compose version >/dev/null 2>&1 || die "docker compose v2+ is required"

mkdir -p "$REPORT_DIR"

ch() {
	docker compose exec -T clickhouse clickhouse-client --database observaki --query "$1"
}

wait_for() {
	local description=$1 elapsed=0
	shift
	until "$@"; do
		elapsed=$((elapsed + 2))
		[ "$elapsed" -lt "$DEADLINE" ] || die "timed out after ${DEADLINE}s waiting for ${description}"
		sleep 2
	done
}

logs_at_least() {
	local before=$1
	[ "$(( $(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'") - before ))" -ge 2 ]
}

traces_at_least() {
	local before=$1
	[ "$(( $(ch "SELECT count() FROM otel_traces WHERE project_id = 'project-dev'") - before ))" -ge 2 ]
}

metric_present() {
	local now
	now=$(date +%s)
	curl -fsS --get 'http://127.0.0.1:8428/api/v1/query_range' \
		--data-urlencode 'query=observaki_sample_requests_total{project_id="project-dev"}' \
		--data-urlencode "start=$((now - 600))" \
		--data-urlencode "end=$now" \
		--data-urlencode 'step=30' |
		jq -e '.data.result | length > 0' >/dev/null
}

if [ "$FRESH" -eq 1 ]; then
	log "removing the Compose volumes and the local image (fresh-machine run)"
	docker compose down -v --remove-orphans >/dev/null 2>&1 || true
	docker image rm -f observaki:local >/dev/null 2>&1 || true
fi

started=$(date +%s)
up_args=(up -d --wait)
if [ "$BUILD" -eq 1 ]; then
	up_args+=(--build)
fi
log "docker compose ${up_args[*]}"
docker compose "${up_args[@]}" || die "docker compose up failed"
up_seconds=$(($(date +%s) - started))
log "stack healthy in ${up_seconds}s"

curl -fsS http://127.0.0.1:8080/healthz >/dev/null || die "API /healthz is not reachable"
curl -fsS http://127.0.0.1:4318/readyz >/dev/null || die "gateway /readyz is not reachable"
curl -fsS http://127.0.0.1:8082/readyz >/dev/null || die "writer /readyz is not reachable"

logs_before=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
traces_before=$(ch "SELECT count() FROM otel_traces WHERE project_id = 'project-dev'")

log "sending the sample telemetry (deploy/compose/sample.sh)"
sample_output=$("$ROOT/deploy/compose/sample.sh")
printf '%s\n' "$sample_output" >&2

wait_for "2 sample logs in ClickHouse" logs_at_least "$logs_before"
wait_for "2 sample spans in ClickHouse" traces_at_least "$traces_before"
wait_for "the sample metric in VictoriaMetrics" metric_present

total_seconds=$(($(date +%s) - started))
[ "$total_seconds" -lt "$LIMIT" ] || die "quickstart took ${total_seconds}s, limit is ${LIMIT}s"

docker_version=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)
compose_version=$(docker compose version --short 2>/dev/null || docker compose version)

{
	echo "# Story 5.3 — quickstart check"
	echo
	echo "- Date (UTC): \`$STAMP\`"
	echo "- Host: \`$(uname -srm)\`"
	echo "- Docker: \`$docker_version\`; Compose: \`$compose_version\`"
	echo "- Driver: \`deploy/compose/quickstart-check.sh\` (fresh=$FRESH, build=$BUILD)"
	echo
	echo "## AC1 — quickstart under 15 minutes"
	echo
	echo "- \`docker compose up -d --wait\` (build=$BUILD): **${up_seconds}s** to all-healthy."
	echo "- Sample telemetry accepted and visible in ClickHouse and VictoriaMetrics."
	echo "- Total from \`up\` to verified data: **${total_seconds}s** (limit ${LIMIT}s)."
	echo "- Result: **pass**."
	echo
	echo "## Sample output"
	echo
	echo '```'
	printf '%s\n' "$sample_output"
	echo '```'
	echo
	echo "## Notes"
	echo
	echo "- This is the single-node evaluation stack, not a benchmark and not G1 evidence."
	echo "- The quickstart doc is \`docs/quickstart.md\`; the per-language apps are in \`examples/\`."
} >"$REPORT_DIR/quickstart-report.md"

log "report written to deploy/compose/results/$STAMP/quickstart-report.md"
log "PASS: quickstart completed in ${total_seconds}s (limit ${LIMIT}s)"
