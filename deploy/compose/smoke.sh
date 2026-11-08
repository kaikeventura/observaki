#!/usr/bin/env bash
# End-to-end check for the single-node stack (Story 5.1).
#
#   ./deploy/compose/smoke.sh               # full run, writes a report
#   ./deploy/compose/smoke.sh --no-build    # reuse the local observaki image
#
# It verifies:
#   AC1  every service becomes healthy and the API/UI answers in < 10 minutes
#   AC2  the sample app lands metrics, logs and traces in ClickHouse/VM
#   AC3  down/up keeps the data and needs no manual migration
#   Story 5.5: /openapi.json plus token-scoped projects, tokens and sample query
#
# Requires: docker, docker compose v2+, curl and jq.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

BUILD=1
PERSISTENCE=1
HEALTH_LIMIT=${OBSERVAKI_COMPOSE_HEALTH_LIMIT:-600}
DEADLINE=120
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
REPORT_DIR="$ROOT/deploy/compose/results/$STAMP"

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }
die() {
	log "FAIL: $*"
	log "hint: docker compose logs --tail 100"
	exit 1
}

usage() {
	cat <<'EOF'
Usage: deploy/compose/smoke.sh [options]

Options:
  --no-build         do not rebuild the observaki image
  --skip-persistence skip the docker compose down/up data check (AC3)
  --health-limit N   maximum seconds from `up` to healthy (default: 600)
  --help             show this help

The report is written to deploy/compose/results/<UTC timestamp>/report.md.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--no-build)
		BUILD=0
		shift
		;;
	--skip-persistence)
		PERSISTENCE=0
		shift
		;;
	--health-limit)
		HEALTH_LIMIT=${2:?--health-limit requires a value}
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

service_state() {
	docker compose ps -a --format '{{.Service}}|{{.State}}|{{.Health}}|{{.ExitCode}}'
}

require_healthy() {
	local output=$1 service
	for service in \
		clickhouse victoriametrics postgres nats \
		observaki-api observaki-gateway observaki-writer; do
		if ! printf '%s\n' "$output" | awk -F'|' -v s="$service" \
			'$1 == s && $2 == "running" && $3 == "healthy" { found = 1 } END { exit found ? 0 : 1 }'; then
			printf '%s\n' "$output" >&2
			die "${service} is not running and healthy"
		fi
	done
	if ! printf '%s\n' "$output" | awk -F'|' \
		'$1 == "clickhouse-migrate" && $2 == "exited" && $4 == "0" { found = 1 } END { exit found ? 0 : 1 }'; then
		printf '%s\n' "$output" >&2
		die "clickhouse-migrate did not exit 0"
	fi
}

ch() {
	docker compose exec -T clickhouse clickhouse-client --database observaki --query "$1"
}

vm_query() {
	curl -fsS --get 'http://127.0.0.1:8428/api/v1/query' --data-urlencode "query=$1"
}

# vm_has_series reports whether a range query over the last 10 minutes returns
# data. Range queries with end=now only see samples older than the VM
# -search.latencyOffset (30s by default), so the caller polls.
vm_has_series() {
	local now
	now=$(date +%s)
	curl -fsS --get 'http://127.0.0.1:8428/api/v1/query_range' \
		--data-urlencode "query=$1" \
		--data-urlencode "start=$((now - 600))" \
		--data-urlencode "end=$now" \
		--data-urlencode 'step=30' |
		jq -e '.data.result | length > 0' >/dev/null
}

ui_reachable() {
	local body
	body=$(curl -fsS http://127.0.0.1:8080/)
	printf '%s' "$body" | grep -q 'Observaki' || return 1
	curl -fsS http://127.0.0.1:8080/api/v1/health | jq -e '.status == "ok"' >/dev/null
	curl -fsS http://127.0.0.1:8080/healthz >/dev/null
	curl -fsS http://127.0.0.1:4318/readyz >/dev/null
	curl -fsS http://127.0.0.1:8082/readyz >/dev/null
}

# Story 5.5: the generated contract is served and the seeded API token works
# against the real control plane; unauthenticated reads fail closed.
api_contract() {
	local status body
	curl -fsS http://127.0.0.1:8080/openapi.json | jq -e '.openapi == "3.1.0"' >/dev/null || return 1
	curl -fsS http://127.0.0.1:8080/openapi.json |
		jq -e '.paths["/api/v1/projects"] and .paths["/api/v1/projects/{projectID}/ingest-tokens"] and .paths["/api/v1/query/sample"]' >/dev/null || return 1

	status=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/v1/projects)
	[ "$status" = "401" ] || return 1

	body=$(curl -fsS -H 'Authorization: Bearer observaki-local-dev-token' http://127.0.0.1:8080/api/v1/projects)
	printf '%s' "$body" | jq -e '.projects[0].id == "00000000-0000-4000-8000-000000000002"' >/dev/null || return 1

	body=$(curl -fsS -H 'Authorization: Bearer observaki-local-dev-token' \
		http://127.0.0.1:8080/api/v1/projects/00000000-0000-4000-8000-000000000002/ingest-tokens)
	printf '%s' "$body" | jq -e '.tokens | length >= 1' >/dev/null || return 1
	if printf '%s' "$body" | grep -qi 'sha256'; then
		return 1
	fi

	body=$(curl -fsS -H 'Authorization: Bearer observaki-local-dev-token' \
		'http://127.0.0.1:8080/api/v1/query/sample?signal=metrics')
	printf '%s' "$body" | jq -e '.sample == true and .project_id == "00000000-0000-4000-8000-000000000002"' >/dev/null || return 1
}

wait_for() {
	local description=$1 elapsed=0
	shift
	until "$@"; do
		elapsed=$((elapsed + 2))
		if [ "$elapsed" -ge "$DEADLINE" ]; then
			die "timed out after ${DEADLINE}s waiting for ${description}"
		fi
		sleep 2
	done
}

logs_at_least() {
	local before=$1 min=$2
	local now
	now=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
	[ "$((now - before))" -ge "$min" ]
}

traces_at_least() {
	local before=$1 min=$2
	local now
	now=$(ch "SELECT count() FROM otel_traces WHERE project_id = 'project-dev'")
	[ "$((now - before))" -ge "$min" ]
}

metrics_present() {
	vm_has_series 'observaki_sample_requests_total{project_id="project-dev"}'
}

log "starting the single-node stack (build=${BUILD})"
up_args=(up -d --wait)
if [ "$BUILD" -eq 1 ]; then
	up_args+=(--build)
fi

started=$(date +%s)
docker compose "${up_args[@]}" || die "docker compose up failed"
up_seconds=$(($(date +%s) - started))
log "stack healthy in ${up_seconds}s"

state=$(service_state)
require_healthy "$state"
[ "$up_seconds" -lt "$HEALTH_LIMIT" ] || die "stack took ${up_seconds}s, limit is ${HEALTH_LIMIT}s"
ui_reachable || die "API/UI health endpoints are not reachable"
api_contract || die "Story 5.5 API contract check failed"
log "API, UI, gateway and writer are reachable; OpenAPI and token auth verified"

log "recording baseline row counts"
logs_before=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
traces_before=$(ch "SELECT count() FROM otel_traces WHERE project_id = 'project-dev'")
log "otel_logs=${logs_before} otel_traces=${traces_before}"

log "sending the sample telemetry"
sample_output=$("$ROOT/deploy/compose/sample.sh")
printf '%s\n' "$sample_output" >&2

wait_for "2 sample log records in ClickHouse" logs_at_least "$logs_before" 2
wait_for "2 sample spans in ClickHouse" traces_at_least "$traces_before" 2
wait_for "the sample metric in VictoriaMetrics" metrics_present
logs_after=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
traces_after=$(ch "SELECT count() FROM otel_traces WHERE project_id = 'project-dev'")
log "logs ${logs_before} -> ${logs_after}, traces ${traces_before} -> ${traces_after}, metric series present"

persistence_result="skipped"
down_seconds=0
restart_seconds=0
if [ "$PERSISTENCE" -eq 1 ]; then
	log "stopping the stack to verify the volumes (AC3)"
	started=$(date +%s)
	docker compose down || die "docker compose down failed"
	down_seconds=$(($(date +%s) - started))

	log "starting the stack again without touching the schema"
	started=$(date +%s)
	docker compose up -d --wait || die "docker compose up after down failed"
	restart_seconds=$(($(date +%s) - started))

	state=$(service_state)
	require_healthy "$state"
	ui_reachable || die "API/UI health endpoints are not reachable after restart"

	logs_restored=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
	traces_restored=$(ch "SELECT count() FROM otel_traces WHERE project_id = 'project-dev'")
	persistence_result="pass"
	[ "$logs_restored" -ge "$logs_after" ] || {
		persistence_result="fail"
		die "logs were lost across restart: ${logs_after} -> ${logs_restored}"
	}
	[ "$traces_restored" -ge "$traces_after" ] || {
		persistence_result="fail"
		die "traces were lost across restart: ${traces_after} -> ${traces_restored}"
	}
	vm_has_series 'observaki_sample_requests_total{project_id="project-dev"}' ||
		die "the metric series disappeared across restart"
	log "data persisted: logs=${logs_restored} traces=${traces_restored} (down ${down_seconds}s, up ${restart_seconds}s)"
fi

compose_version=$(docker compose version --short 2>/dev/null || docker compose version)
docker_version=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)

{
	echo "# Story 5.1 — single-node Compose smoke"
	echo
	echo "- Date (UTC): \`$STAMP\`"
	echo "- Host: \`$(uname -srm)\`"
	echo "- Docker: \`$docker_version\`; Compose: \`$compose_version\`"
	echo "- Driver: \`deploy/compose/smoke.sh\`; sample: \`deploy/compose/sample.sh\`"
	echo
	echo "## AC1 — startup and health"
	echo
	echo "- \`docker compose up -d --wait\` (build=$BUILD): **${up_seconds}s** to all-healthy (limit ${HEALTH_LIMIT}s)."
	echo "- Healthy: clickhouse, victoriametrics, postgres, nats, observaki-api, observaki-gateway, observaki-writer; clickhouse-migrate exited 0."
	echo "- UI \`http://127.0.0.1:8080/\` and \`/api/v1/health\` reachable; gateway \`/readyz\` and writer \`/readyz\` return 200."
	echo "- Story 5.5: \`/openapi.json\` is OpenAPI 3.1 and documents projects, ingestion tokens and the sample query; \`/api/v1/projects\` returns 401 without a token and the seeded token reads the development project without exposing a digest."
	echo
	echo "## AC2 — sample telemetry"
	echo
	echo '```'
	printf '%s\n' "$sample_output"
	echo '```'
	echo
	echo "- \`otel_logs\` rows for project-dev: ${logs_before} -> ${logs_after}."
	echo "- \`otel_traces\` rows for project-dev: ${traces_before} -> ${traces_after}."
	echo "- VictoriaMetrics: the range query for \`observaki_sample_requests_total{project_id=\"project-dev\"}\` returns data (the 30s latency offset is awaited)."
	echo "- Linux validated here; the macOS run is still pending (see the handoff)."
	echo
	echo "## AC3 — lifecycle"
	echo
	echo "- Persistence check: **${persistence_result}** (down ${down_seconds}s, up ${restart_seconds}s)."
	if [ "$PERSISTENCE" -eq 1 ]; then
		echo "- After \`docker compose down\` / \`up\`, rows: logs=${logs_restored} traces=${traces_restored}; the metric series is still present."
		echo "- \`clickhouse-migrate\` re-applied the idempotent migrations automatically; no manual step."
	fi
	echo
	echo "## Notes"
	echo
	echo "- This is a single-node evaluation stack, not a benchmark run and not G1 evidence."
	echo "- The landing page has no Explore view yet (Stories 6.1-6.4); data is verified in storage and through the API."
} >"$REPORT_DIR/report.md"

log "report written to deploy/compose/results/$STAMP/report.md"
log "PASS: single-node stack started, ingested metrics/logs/traces and kept the data across restart"
