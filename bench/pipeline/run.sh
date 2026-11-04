#!/usr/bin/env bash
# Story 4.4 — end-to-end pipeline: gateway -> NATS JetStream WAL -> writers ->
# ClickHouse (logs/traces) and VictoriaMetrics (metrics).
#
# The harness starts disposable NATS, ClickHouse and VictoriaMetrics
# containers, applies the ClickHouse migrations, runs the gateway, the writer
# and two OTLP load generators, then verifies:
#   * accepted events == WAL events == writer accepted events
#   * writer written + deduped + failed == accepted
#   * ClickHouse row counts match the writer's per-signal written counts
#   * VictoriaMetrics sample counts match the metrics writer (within the ms
#     timestamp coalescing bound)
#   * end-to-end lag p95 < 10 s
#   * a transient ClickHouse/VictoriaMetrics outage is retried without a loss
#
# This is a local single-node proxy. AC1 asks for 1M/s for one hour on the T1
# fleet and remains blocked by P-07; this harness never claims G1 evidence.
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
GATEWAY_BIN="$ROOT/bin/observaki"
LOADGEN_BIN="$ROOT/bin/otlploadgen"
MIGRATIONS="$ROOT/internal/adapters/clickhouse/migrations"
CH_CONFIG="$ROOT/bench/clickhouse/config/bench.xml"
CH_USERS="$ROOT/bench/clickhouse/ingest/config/users.xml"

RATE=20000
DURATION=20s
BATCH=200
WORKERS=4
MIX=34,33,33
COMPRESSION=gzip
METRIC_PADDING=832
LOG_BODY=820
TRACE_PADDING=2400
RUNS=1
CHAOS=0
CHAOS_CH_SECONDS=3
CHAOS_VM_SECONDS=2
DEDUPE=1
DEDUPE_DURATION=1s
RESULTS_DIR=""
SMOKE=0
NATS_IMAGE="${NATS_IMAGE:-nats:2.15.2-alpine}"
CH_IMAGE="${OBSERVAKI_PIPELINE_CH_IMAGE:-clickhouse/clickhouse-server:26.8}"
VM_IMAGE="${OBSERVAKI_PIPELINE_VM_IMAGE:-victoriametrics/victoria-metrics:v1.148.0}"

usage() {
	cat <<'EOF'
Usage: bench/pipeline/run.sh [options]

Options:
  --rate N           total events/s across both tenants (default: 20000)
  --duration T       measured duration per run (default: 20s)
  --batch-size N     events per OTLP request (default: 200)
  --workers N        workers per tenant load generator (default: 4)
  --mix a,b,c        metrics,logs,traces mix in events per 100 (default: 34,33,33)
  --compression C    gzip or none (default: gzip)
  --runs N           measured repetitions (default: 1)
  --chaos            stop ClickHouse and VictoriaMetrics briefly and verify retry
  --no-dedupe        skip the identical-replay dedupe phase
  --results-dir P    report destination
  --smoke            short harness check (5s, 5k/s, chaos on)
  --help             show this help

Requires Docker, jq and curl. The writer only acks a WAL message after
ClickHouse or VictoriaMetrics confirmed the flush.
EOF
}

log() {
	printf '[%s] %s\n' "$(date -u '+%H:%M:%S')" "$*" >&2
}

die() {
	log "ERROR: $*"
	exit 1
}

while [ $# -gt 0 ]; do
	case "$1" in
	--rate) RATE=$2; shift 2 ;;
	--duration) DURATION=$2; shift 2 ;;
	--batch-size) BATCH=$2; shift 2 ;;
	--workers) WORKERS=$2; shift 2 ;;
	--mix) MIX=$2; shift 2 ;;
	--compression) COMPRESSION=$2; shift 2 ;;
	--runs) RUNS=$2; shift 2 ;;
	--chaos) CHAOS=1; shift ;;
	--no-dedupe) DEDUPE=0; shift ;;
	--results-dir) RESULTS_DIR=$2; shift 2 ;;
	--smoke) SMOKE=1; shift ;;
	--help|-h) usage; exit 0 ;;
	*) die "unknown option $1 (see --help)" ;;
	esac
done

if [ "$SMOKE" = 1 ]; then
	RATE=5000
	DURATION=5s
	BATCH=100
	WORKERS=2
	RUNS=1
	CHAOS=1
	DEDUPE_DURATION=1s
fi

if [ "$RUNS" -lt 1 ]; then
	RUNS=1
fi
case "$COMPRESSION" in
gzip|none) ;;
*) die "--compression must be gzip or none" ;;
esac
[ "$CHAOS_CH_SECONDS" -ge 1 ] && [ "$CHAOS_VM_SECONDS" -ge 1 ] || die "chaos durations must be >= 1s"

command -v docker >/dev/null || die "docker is required"
command -v jq >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"
[ -x "$GATEWAY_BIN" ] || die "$GATEWAY_BIN is missing (run make build)"
[ -x "$LOADGEN_BIN" ] || die "$LOADGEN_BIN is missing (run make build)"

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/pipeline/results/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$RESULTS_DIR"
WORK_DIR=$(mktemp -d -t observaki-pipeline.XXXXXX)
LOG="$RESULTS_DIR/run.log"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

nats_container=""
ch_container=""
vm_container=""
gateway_pid=""
writer_pid=""
load_pids=()
chaos_pid=""
chaos_ch_status="not_run"
chaos_vm_status="not_run"
chaos_ch_retries_before=0
chaos_vm_retries_before=0

cleanup() {
	if [ -n "${chaos_pid:-}" ]; then
		kill "$chaos_pid" 2>/dev/null || true
		wait "$chaos_pid" 2>/dev/null || true
	fi
	for p in "${load_pids[@]:-}"; do
		kill -TERM "$p" 2>/dev/null || true
		wait "$p" 2>/dev/null || true
	done
	for pid in "$gateway_pid" "$writer_pid"; do
		if [ -n "$pid" ]; then
			kill -TERM "$pid" 2>/dev/null || true
			wait "$pid" 2>/dev/null || true
		fi
	done
	for container in "$nats_container" "$ch_container" "$vm_container"; do
		if [ -n "$container" ]; then
			if [ "${KEEP_CONTAINERS:-0}" = 1 ]; then
				log "keeping container $container"
			else
				docker rm -f "$container" >/dev/null 2>&1 || true
			fi
		fi
	done
	if [ "${KEEP_WORK:-0}" != 1 ]; then
		rm -rf "$WORK_DIR" 2>/dev/null || true
	fi
}
trap cleanup EXIT

find_free_port() {
	local start="${1:-18500}"
	local end="${2:-18600}"
	local port
	for port in $(seq "$start" "$end"); do
		if ! (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
			echo "$port"
			return 0
		fi
	done
	return 1
}

nats_port=$(find_free_port 18500 18530) || die "no free NATS port"
nats_monitor_port=$(find_free_port 18531 18560) || die "no free NATS monitor port"
grpc_port=$(find_free_port 18561 18590) || die "no free gRPC port"
http_port=$(find_free_port 18591 18620) || die "no free gateway HTTP port"
writer_port=$(find_free_port 18621 18650) || die "no free writer port"
ch_port=$(find_free_port 18651 18700) || die "no free ClickHouse port"
vm_port=$(find_free_port 18701 18750) || die "no free VictoriaMetrics port"

nats_url="nats://127.0.0.1:${nats_port}"

# --- NATS -------------------------------------------------------------------
mkdir -p "$WORK_DIR/nats/js"
cat >"$WORK_DIR/nats/nats.conf" <<'EOF'
port: 4222
http_port: 8222
max_payload: 16777216
jetstream {
  store_dir: "/data/js"
}
EOF
nats_container="observaki-pipeline-nats-$$"
log "starting NATS JetStream on 127.0.0.1:${nats_port}"
docker run -d --rm --user "$(id -u):$(id -g)" --name "$nats_container" \
	-p "${nats_port}:4222" -p "${nats_monitor_port}:8222" \
	-v "$WORK_DIR/nats:/data" "$NATS_IMAGE" -c /data/nats.conf >/dev/null
ready=0
for _ in $(seq 1 60); do
	if curl -fsS "http://127.0.0.1:${nats_monitor_port}/varz" >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 0.2
done
[ "$ready" = 1 ] || die "NATS did not become ready"

# --- ClickHouse -------------------------------------------------------------
mkdir -p "$WORK_DIR/clickhouse"
chmod 777 "$WORK_DIR/clickhouse" 2>/dev/null || true
ch_container="observaki-pipeline-ch-$$"
log "starting ClickHouse on 127.0.0.1:${ch_port}"
docker run -d --name "$ch_container" \
	-p "127.0.0.1:${ch_port}:8123" \
	-v "$WORK_DIR/clickhouse:/var/lib/clickhouse" \
	-v "$CH_CONFIG:/etc/clickhouse-server/config.d/bench.xml:ro" \
	-v "$CH_USERS:/etc/clickhouse-server/users.d/observaki-bench.xml:ro" \
	--ulimit nofile=262144:262144 \
	"$CH_IMAGE" >/dev/null

wait_for_clickhouse() {
	for _ in $(seq 1 90); do
		if docker exec "$ch_container" clickhouse-client --query 'SELECT 1' >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	return 1
}
wait_for_clickhouse || die "ClickHouse did not become ready"

ch() {
	docker exec "$ch_container" clickhouse-client --database observaki "$@"
}
ch_root() {
	docker exec "$ch_container" clickhouse-client "$@"
}
query() {
	ch --query "$1" --format TSV
}

ch_root --query "CREATE DATABASE IF NOT EXISTS observaki"
for migration in "$MIGRATIONS"/0001_otel_logs.sql "$MIGRATIONS"/0002_otel_traces.sql "$MIGRATIONS"/0003_ingest_dedupe.sql "$MIGRATIONS"/0004_tenancy.sql; do
	log "applying $(basename "$migration")"
	docker exec -i "$ch_container" clickhouse-client --database observaki --multiquery <"$migration"
done

# --- VictoriaMetrics --------------------------------------------------------
mkdir -p "$WORK_DIR/vm"
vm_container="observaki-pipeline-vm-$$"
log "starting VictoriaMetrics on 127.0.0.1:${vm_port}"
docker run -d --name "$vm_container" \
	-p "127.0.0.1:${vm_port}:8428" \
	-v "$WORK_DIR/vm:/victoria-metrics-data" \
	"$VM_IMAGE" \
	-storageDataPath=/victoria-metrics-data \
	-retentionPeriod=1d \
	-httpListenAddr=:8428 \
	-memory.allowedBytes=2GiB \
	-search.maxQueryDuration=60s \
	-loggerLevel=WARN >/dev/null
ready=0
for _ in $(seq 1 60); do
	if curl -fsS "http://127.0.0.1:${vm_port}/health" >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 0.2
done
[ "$ready" = 1 ] || die "VictoriaMetrics did not become ready"

# --- Auth snapshot ----------------------------------------------------------
token_a="pipeline-a-$(date -u +%s%N)-${RANDOM}"
token_b="pipeline-b-$(date -u +%s%N)-${RANDOM}"
hash_a=$(printf '%s' "$token_a" | sha256sum | cut -d' ' -f1)
hash_b=$(printf '%s' "$token_b" | sha256sum | cut -d' ' -f1)
issued_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
expires_at=$(date -u -d '+4 hours' +%Y-%m-%dT%H:%M:%SZ)
snapshot="$WORK_DIR/ingest-auth.json"
jq -n \
	--arg version "pipeline-$(date -u +%s)" \
	--arg issued "$issued_at" \
	--arg expires "$expires_at" \
	--arg hash_a "$hash_a" \
	--arg hash_b "$hash_b" \
	--argjson quota "$RATE" \
	'{
		config_version: $version,
		generation: 1,
		issued_at: $issued,
		expires_at: $expires,
		tenants: [
			{
				tenant_id: "tenant-a",
				ingest_quota: {events_per_second: $quota, burst_events: $quota},
				projects: [{
					project_id: "project-a",
					tokens: [{token_id: "token-a", sha256: $hash_a, scopes: ["ingest"]}]
				}]
			},
			{
				tenant_id: "tenant-b",
				ingest_quota: {events_per_second: $quota, burst_events: $quota},
				projects: [{
					project_id: "project-b",
					tokens: [{token_id: "token-b", sha256: $hash_b, scopes: ["ingest"]}]
				}]
			}
		]
	}' >"$snapshot"

# --- Gateway ----------------------------------------------------------------
log "starting gateway on grpc=127.0.0.1:${grpc_port} http=127.0.0.1:${http_port}"
OBSERVAKI_ENV=development \
	OBSERVAKI_OTLP_GRPC_ADDR="127.0.0.1:${grpc_port}" \
	OBSERVAKI_OTLP_HTTP_ADDR="127.0.0.1:${http_port}" \
	OBSERVAKI_INGEST_AUTH_FILE="$snapshot" \
	OBSERVAKI_INGEST_AUDIT_FILE="$WORK_DIR/audit.jsonl" \
	OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL=5s \
	OBSERVAKI_NATS_URL="$nats_url" \
	OBSERVAKI_NATS_SHARDS=32 \
	OBSERVAKI_NATS_BATCH_LINGER=5ms \
	OBSERVAKI_NATS_BATCH_MAX_EVENTS=100000 \
	OBSERVAKI_NATS_BATCH_MAX_BYTES=4194304 \
	"$GATEWAY_BIN" gateway >"$RESULTS_DIR/gateway.log" 2>&1 &
gateway_pid=$!

ready=0
for _ in $(seq 1 50); do
	if curl -fsS "http://127.0.0.1:${http_port}/readyz" >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 0.2
done
[ "$ready" = 1 ] || die "gateway did not become ready (see $RESULTS_DIR/gateway.log)"

# --- Writer -----------------------------------------------------------------
log "starting writer on 127.0.0.1:${writer_port} (spool $WORK_DIR/spool)"
OBSERVAKI_ENV=development \
	OBSERVAKI_WRITER_ADDR="127.0.0.1:${writer_port}" \
	OBSERVAKI_WRITER_SPOOL_DIR="$WORK_DIR/spool" \
	OBSERVAKI_WRITER_BATCH_MAX_EVENTS=20000 \
	OBSERVAKI_WRITER_BATCH_LINGER=200ms \
	OBSERVAKI_WRITER_DEDUPE_WINDOW=1000000 \
	OBSERVAKI_WRITER_ACK_WAIT=10m \
	OBSERVAKI_WRITER_RETENTION_DAYS=15 \
	OBSERVAKI_WRITER_MAX_RETRY_BACKOFF=5s \
	OBSERVAKI_CLICKHOUSE_URL="http://127.0.0.1:${ch_port}" \
	OBSERVAKI_CLICKHOUSE_DATABASE=observaki \
	OBSERVAKI_CLICKHOUSE_USER=observaki \
	OBSERVAKI_CLICKHOUSE_TIMEOUT=10s \
	OBSERVAKI_CLICKHOUSE_COMPRESSION=gzip \
	OBSERVAKI_CLICKHOUSE_MAX_RETRIES=0 \
	OBSERVAKI_CLICKHOUSE_RETRY_BACKOFF=500ms \
	OBSERVAKI_VICTORIAMETRICS_URL="http://127.0.0.1:${vm_port}" \
	OBSERVAKI_VICTORIAMETRICS_TIMEOUT=10s \
	OBSERVAKI_VICTORIAMETRICS_MAX_RETRIES=0 \
	OBSERVAKI_VICTORIAMETRICS_RETRY_BACKOFF=500ms \
	OBSERVAKI_NATS_URL="$nats_url" \
	"$GATEWAY_BIN" writer >"$RESULTS_DIR/writer.log" 2>&1 &
writer_pid=$!

ready=0
for _ in $(seq 1 50); do
	if curl -fsS "http://127.0.0.1:${writer_port}/readyz" >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 0.2
done
[ "$ready" = 1 ] || die "writer did not become ready (see $RESULTS_DIR/writer.log)"

writer_metrics() {
	curl -fsS "http://127.0.0.1:${writer_port}/metrics"
}
gateway_metrics() {
	curl -fsS "http://127.0.0.1:${http_port}/metrics"
}

metric_sum() {
	local name=$1
	local signal=$2
	writer_metrics | awk -v name="$name" -v signal="$signal" '
		index($1, name "{") == 1 && index($1, "signal=\"" signal "\"") > 0 {sum += $2}
		END {printf "%.0f", sum}
	'
}

metric_max() {
	local name=$1
	local filter=${2:-}
	writer_metrics | awk -v name="$name" -v filter="$filter" '
		index($1, name) == 1 && (filter == "" || index($1, filter) > 0) {
			if ($2 > max) max = $2
		}
		END {printf "%.6f", max}
	'
}

metric_value() {
	local name=$1
	writer_metrics | awk -v name="$name" '$1 == name {print $2; exit}'
}

gateway_metric_value() {
	local name=$1
	gateway_metrics | awk -v name="$name" '$1 == name {print $2; exit}'
}

drain_pipeline() {
	local timeout=${1:-120}
	local started
	started=$(date +%s)
	while true; do
		local in_flight pending accepted written deduped failed
		in_flight=$(metric_value "observaki_writer_in_flight_events")
		in_flight=${in_flight:-0}
		pending=0
		for signal in traces logs metrics; do
			local value
			value=$(writer_metrics | awk -v signal="$signal" '
				index($1, "observaki_writer_consumer_pending_messages{signal=\"" signal "\"}") == 1 {print $2; exit}
			')
			pending=$((pending + ${value:-0}))
		done
		accepted=0
		written=0
		deduped=0
		failed=0
		for signal in traces logs metrics; do
			accepted=$((accepted + $(metric_sum "observaki_writer_accepted_events_total" "$signal")))
			written=$((written + $(metric_sum "observaki_writer_written_events_total" "$signal")))
			deduped=$((deduped + $(metric_sum "observaki_writer_deduped_events_total" "$signal")))
			failed=$((failed + $(metric_sum "observaki_writer_failed_events_total" "$signal")))
		done
		if [ "$in_flight" = "0" ] && [ "$pending" = "0" ] && [ "$accepted" = "$((written + deduped + failed))" ]; then
			return 0
		fi
		if [ $(( $(date +%s) - started )) -ge "$timeout" ]; then
			log "drain timeout: in_flight=${in_flight} pending=${pending} accepted=${accepted} written=${written} deduped=${deduped} failed=${failed}"
			return 1
		fi
		sleep 0.5
	done
}

run_loadgen() {
	local name=$1
	local rate=$2
	local token=$3
	local output=$4
	local duration=$5
	local start_time=$6
	"$LOADGEN_BIN" \
		--protocol http \
		--endpoint "http://127.0.0.1:${http_port}" \
		--rate "$rate" \
		--duration "$duration" \
		--batch-size "$BATCH" \
		--workers "$WORKERS" \
		--mix "$MIX" \
		--compression "$COMPRESSION" \
		--metric-padding "$METRIC_PADDING" \
		--log-body "$LOG_BODY" \
		--trace-padding "$TRACE_PADDING" \
		--metrics-addr "" \
		--seed 1 \
		--start-time "$start_time" \
		--header "Authorization=Bearer ${token}" \
		>"$output" 2>"${output%.json}.log"
}

# --- Dedupe phase -----------------------------------------------------------
dedupe_events=0
dedupe_status="skipped"
if [ "$DEDUPE" = 1 ]; then
	log "dedupe phase: two identical deterministic 1s loads (same seed and start time)"
	dedupe_start=$(date -u -d '-2 seconds' '+%Y-%m-%dT%H:%M:%SZ')
	dedupe_rate=$((RATE / 2))
	run_loadgen "dedupe-a" "$dedupe_rate" "$token_a" "$RESULTS_DIR/dedupe-a.json" "$DEDUPE_DURATION" "$dedupe_start"
	dedupe_accepted=$(jq -r '.metrics.sent' "$RESULTS_DIR/dedupe-a.json")
	drain_pipeline 60 || die "pipeline did not drain after the first dedupe load"
	deduped_before=$(metric_sum "observaki_writer_deduped_events_total" "logs")
	deduped_before=$((deduped_before + $(metric_sum "observaki_writer_deduped_events_total" "metrics")))
	deduped_before=$((deduped_before + $(metric_sum "observaki_writer_deduped_events_total" "traces")))
	run_loadgen "dedupe-b" "$dedupe_rate" "$token_a" "$RESULTS_DIR/dedupe-b.json" "$DEDUPE_DURATION" "$dedupe_start"
	dedupe_accepted=$((dedupe_accepted + $(jq -r '.metrics.sent' "$RESULTS_DIR/dedupe-b.json")))
	drain_pipeline 60 || die "pipeline did not drain after the second dedupe load"
	deduped_after=$(metric_sum "observaki_writer_deduped_events_total" "logs")
	deduped_after=$((deduped_after + $(metric_sum "observaki_writer_deduped_events_total" "metrics")))
	deduped_after=$((deduped_after + $(metric_sum "observaki_writer_deduped_events_total" "traces")))
	dedupe_events=$((deduped_after - deduped_before))
	if [ "$dedupe_events" -eq 0 ]; then
		dedupe_status="fail"
	else
		dedupe_status="pass"
	fi
	log "dedupe phase: ${dedupe_events} events deduplicated (${dedupe_status})"
fi

# --- Measured runs ----------------------------------------------------------
run_reports=()
measure_start=$(date -u '+%Y-%m-%dT%H:%M:%S')
for run in $(seq 1 "$RUNS"); do
	log "run ${run}/${RUNS}: two tenants at $((RATE / 2)) events/s each for ${DURATION}"
	start_time=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
	wal_before=$(gateway_metric_value "observaki_gateway_wal_published_events_total")
	wal_before=${wal_before:-0}
	accepted_before=$(metric_sum "observaki_writer_accepted_events_total" "logs")
	accepted_before=$((accepted_before + $(metric_sum "observaki_writer_accepted_events_total" "metrics")))
	accepted_before=$((accepted_before + $(metric_sum "observaki_writer_accepted_events_total" "traces")))

	run_loadgen "tenant-a-${run}" "$((RATE / 2))" "$token_a" "$RESULTS_DIR/tenant-a-${run}.json" "$DURATION" "$start_time" &
	load_pids+=("$!")
	run_loadgen "tenant-b-${run}" "$((RATE / 2))" "$token_b" "$RESULTS_DIR/tenant-b-${run}.json" "$DURATION" "$start_time" &
	load_pids+=("$!")

	if [ "$CHAOS" = 1 ]; then
		(
			sleep "$(awk -v d="$DURATION" 'BEGIN {printf "%.1f", d * 0.35}')"
			log "chaos: stopping ClickHouse for ${CHAOS_CH_SECONDS}s"
			docker stop "$ch_container" >/dev/null
			sleep "$CHAOS_CH_SECONDS"
			docker start "$ch_container" >/dev/null
			wait_for_clickhouse || exit 1
			log "chaos: ClickHouse recovered"
			sleep 1
			log "chaos: stopping VictoriaMetrics for ${CHAOS_VM_SECONDS}s"
			docker stop "$vm_container" >/dev/null
			sleep "$CHAOS_VM_SECONDS"
			docker start "$vm_container" >/dev/null
			vm_ready=0
			for _ in $(seq 1 60); do
				if curl -fsS "http://127.0.0.1:${vm_port}/health" >/dev/null 2>&1; then
					vm_ready=1
					break
				fi
				sleep 0.2
			done
			[ "$vm_ready" = 1 ] || exit 1
			log "chaos: VictoriaMetrics recovered"
		) &
		chaos_pid=$!
	fi

	for pid in "${load_pids[@]}"; do
		wait "$pid"
	done
	load_pids=()
	if [ "$CHAOS" = 1 ]; then
		wait "$chaos_pid" || die "chaos injection failed; see $RESULTS_DIR/run.log"
		chaos_pid=""
	fi

	sent_a=$(jq -r '.metrics.sent' "$RESULTS_DIR/tenant-a-${run}.json")
	sent_b=$(jq -r '.metrics.sent' "$RESULTS_DIR/tenant-b-${run}.json")
	failed_a=$(jq -r '.metrics.failed' "$RESULTS_DIR/tenant-a-${run}.json")
	failed_b=$(jq -r '.metrics.failed' "$RESULTS_DIR/tenant-b-${run}.json")
	rejected_a=$(jq -r '.metrics.rejected' "$RESULTS_DIR/tenant-a-${run}.json")
	rejected_b=$(jq -r '.metrics.rejected' "$RESULTS_DIR/tenant-b-${run}.json")
	client_p99_a=$(jq -r '.metrics.latency.p99_ms' "$RESULTS_DIR/tenant-a-${run}.json")
	client_p99_b=$(jq -r '.metrics.latency.p99_ms' "$RESULTS_DIR/tenant-b-${run}.json")

	if ! drain_pipeline 180; then
		die "pipeline did not drain after run ${run}"
	fi
	wal_after=$(gateway_metric_value "observaki_gateway_wal_published_events_total")
	wal_after=${wal_after:-0}
	accepted_after=$(metric_sum "observaki_writer_accepted_events_total" "logs")
	accepted_after=$((accepted_after + $(metric_sum "observaki_writer_accepted_events_total" "metrics")))
	accepted_after=$((accepted_after + $(metric_sum "observaki_writer_accepted_events_total" "traces")))

	run_reports+=("$(jq -n \
		--argjson run "$run" \
		--argjson sent_a "$sent_a" --argjson sent_b "$sent_b" \
		--argjson failed_a "$failed_a" --argjson failed_b "$failed_b" \
		--argjson rejected_a "$rejected_a" --argjson rejected_b "$rejected_b" \
		--argjson client_p99_a "$client_p99_a" --argjson client_p99_b "$client_p99_b" \
		--argjson wal_delta "$((wal_after - wal_before))" \
		--argjson accepted_delta "$((accepted_after - accepted_before))" \
		'{
			run: $run,
			sent_tenant_a: $sent_a,
			sent_tenant_b: $sent_b,
			failed: ($failed_a + $failed_b),
			rejected: ($rejected_a + $rejected_b),
			client_p99_ms_tenant_a: (($client_p99_a * 100 | round) / 100),
			client_p99_ms_tenant_b: (($client_p99_b * 100 | round) / 100),
			wal_events_delta: $wal_delta,
			writer_accepted_delta: $accepted_delta
		}')")
done

# --- Final scrape -----------------------------------------------------------
writer_metrics >"$RESULTS_DIR/writer-metrics.txt"
gateway_metrics >"$RESULTS_DIR/gateway-metrics.txt"

total_sent=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.sent_tenant_a + .sent_tenant_b) | add')
total_wal=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.wal_events_delta) | add')
total_accepted=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.writer_accepted_delta) | add')
total_failed=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.failed) | add')
total_rejected=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.rejected) | add')

accepted_logs=$(metric_sum "observaki_writer_accepted_events_total" "logs")
accepted_traces=$(metric_sum "observaki_writer_accepted_events_total" "traces")
accepted_metrics=$(metric_sum "observaki_writer_accepted_events_total" "metrics")
written_logs=$(metric_sum "observaki_writer_written_events_total" "logs")
written_traces=$(metric_sum "observaki_writer_written_events_total" "traces")
written_metrics=$(metric_sum "observaki_writer_written_events_total" "metrics")
deduped_total=0
failed_total=0
skipped_total=0
retries_total=0
for signal in logs traces metrics; do
	deduped_total=$((deduped_total + $(metric_sum "observaki_writer_deduped_events_total" "$signal")))
	failed_total=$((failed_total + $(metric_sum "observaki_writer_failed_events_total" "$signal")))
	skipped_total=$((skipped_total + $(metric_sum "observaki_writer_skipped_events_total" "$signal")))
	retries_total=$((retries_total + $(metric_sum "observaki_writer_retries_total" "$signal")))
done
accepted_total=$((accepted_logs + accepted_traces + accepted_metrics))
written_total=$((written_logs + written_traces + written_metrics))

lag_p95=$(metric_max "observaki_writer_lag_seconds" 'quantile="0.95"')
lag_p50=$(metric_max "observaki_writer_lag_seconds" 'quantile="0.5"')
lag_p99=$(metric_max "observaki_writer_lag_seconds" 'quantile="0.99"')

log "stopping gateway and writer"
kill -TERM "$gateway_pid"
gateway_exit=0
wait "$gateway_pid" || gateway_exit=$?
gateway_pid=""
kill -TERM "$writer_pid"
writer_exit=0
wait "$writer_pid" || writer_exit=$?
writer_pid=""

ch_logs=$(query "SELECT count() FROM otel_logs")
ch_logs_unique=$(query "SELECT uniqExact(event_id) FROM otel_logs")
ch_traces=$(query "SELECT count() FROM otel_traces")
ch_traces_unique=$(query "SELECT uniqExact(event_id) FROM otel_traces")
vm_query='sum(count_over_time({__name__="bench.http.server.duration"}[2h]))'
vm_samples=0
vm_series=0
vm_target=$(awk -v written="$written_metrics" 'BEGIN {printf "%.0f", written * 0.99}')
for _ in $(seq 1 60); do
	vm_now=$(date +%s)
	vm_samples=$(curl -fsS --get "http://127.0.0.1:${vm_port}/api/v1/query" --data-urlencode "query=${vm_query}" --data-urlencode "time=${vm_now}" | jq -r '.data.result[0].value[1] // "0"' | awk '{printf "%.0f", $1}')
	vm_series=$(curl -fsS --get "http://127.0.0.1:${vm_port}/api/v1/query" --data-urlencode 'query=count({__name__="bench.http.server.duration"})' --data-urlencode "time=${vm_now}" | jq -r '.data.result[0].value[1] // "0"' | awk '{printf "%.0f", $1}')
	if [ "$vm_samples" -ge "$vm_target" ] && [ "$vm_series" -gt 0 ]; then
		break
	fi
	sleep 1
done

metrics_tolerance=$(awk -v written="$written_metrics" 'BEGIN {printf "%.0f", written * 0.99}')
status=pass
t1_status=pending
reasons=()
if [ "$total_sent" != "$total_wal" ]; then
	status=fail
	reasons+=("gateway WAL events ${total_wal} != load generator sent ${total_sent}")
fi
if [ "$total_sent" != "$total_accepted" ]; then
	status=fail
	reasons+=("writer accepted ${total_accepted} != load generator sent ${total_sent}")
fi
if [ "$accepted_total" != "$((written_total + deduped_total + failed_total + skipped_total))" ]; then
	status=fail
	reasons+=("accepted ${accepted_total} != written ${written_total} + deduped ${deduped_total} + failed ${failed_total} + skipped ${skipped_total}")
fi
if [ "$total_failed" -ne 0 ] || [ "$total_rejected" -ne 0 ] || [ "$failed_total" -ne 0 ]; then
	status=fail
	reasons+=("load/writer failures: client ${total_failed}/${total_rejected}, writer ${failed_total}")
fi
if [ "$ch_logs" != "$written_logs" ]; then
	status=fail
	reasons+=("ClickHouse logs ${ch_logs} != writer written ${written_logs}")
fi
if [ "$ch_traces" != "$written_traces" ]; then
	status=fail
	reasons+=("ClickHouse traces ${ch_traces} != writer written ${written_traces}")
fi
if [ "$ch_logs_unique" != "$ch_logs" ] || [ "$ch_traces_unique" != "$ch_traces" ]; then
	status=fail
	reasons+=("duplicate event_id in ClickHouse: logs ${ch_logs_unique}/${ch_logs}, traces ${ch_traces_unique}/${ch_traces}")
fi
if [ "$written_metrics" -gt 0 ] && [ "$vm_series" -eq 0 ]; then
	status=fail
	reasons+=("VictoriaMetrics has no series for bench.http.server.duration")
fi
if [ "$vm_samples" -lt "$metrics_tolerance" ] || [ "$vm_samples" -gt "$written_metrics" ]; then
	status=fail
	reasons+=("VictoriaMetrics samples ${vm_samples} outside [${metrics_tolerance}, ${written_metrics}] (writer written metrics)")
fi
if ! awk -v p95="$lag_p95" 'BEGIN {exit !(p95 < 10)}'; then
	status=fail
	reasons+=("writer lag p95 ${lag_p95}s >= 10s")
fi
if [ "$CHAOS" = 1 ] && [ "$retries_total" -eq 0 ]; then
	status=fail
	reasons+=("chaos was enabled but the writer recorded no retry")
fi
if [ "$dedupe_status" = "fail" ]; then
	status=fail
	reasons+=("identical replay was not deduplicated")
fi
if [ "$gateway_exit" -ne 0 ] || [ "$writer_exit" -ne 0 ]; then
	status=fail
	reasons+=("exit codes gateway=${gateway_exit} writer=${writer_exit}")
fi
reason_text=""
if [ ${#reasons[@]} -gt 0 ]; then
	reason_text=$(printf '%s; ' "${reasons[@]}")
	reason_text=${reason_text%; }
fi

run_reports_json=$(printf '%s\n' "${run_reports[@]}" | jq -s '.')
jq -n \
	--arg status "$status" \
	--arg t1_status "$t1_status" \
	--arg duration "$DURATION" \
	--argjson rate "$RATE" \
	--argjson batch "$BATCH" \
	--argjson runs "$RUNS" \
	--arg mix "$MIX" \
	--arg compression "$COMPRESSION" \
	--argjson chaos "$CHAOS" \
	--argjson chaos_ch_seconds "$CHAOS_CH_SECONDS" \
	--argjson chaos_vm_seconds "$CHAOS_VM_SECONDS" \
	--arg dedupe_status "$dedupe_status" \
	--argjson dedupe_events "$dedupe_events" \
	--argjson sent "$total_sent" \
	--argjson wal "$total_wal" \
	--argjson accepted "$accepted_total" \
	--argjson accepted_measured "$total_accepted" \
	--argjson written "$written_total" \
	--argjson deduped "$deduped_total" \
	--argjson failed "$failed_total" \
	--argjson skipped "$skipped_total" \
	--argjson retries "$retries_total" \
	--argjson accepted_logs "$accepted_logs" \
	--argjson accepted_traces "$accepted_traces" \
	--argjson accepted_metrics "$accepted_metrics" \
	--argjson written_logs "$written_logs" \
	--argjson written_traces "$written_traces" \
	--argjson written_metrics "$written_metrics" \
	--argjson ch_logs "$ch_logs" \
	--argjson ch_traces "$ch_traces" \
	--argjson vm_samples "$vm_samples" \
	--argjson vm_series "$vm_series" \
	--argjson lag_p50 "$lag_p50" \
	--argjson lag_p95 "$lag_p95" \
	--argjson lag_p99 "$lag_p99" \
	--argjson gateway_exit "$gateway_exit" \
	--argjson writer_exit "$writer_exit" \
	--arg reason "$reason_text" \
	--argjson run_reports "$run_reports_json" \
	'{
		story: "4.4",
		status: $status,
		t1_status: $t1_status,
		t1_note: "AC1 requires 1M/s for one hour on the T1 fleet; P-07 and the T1 baseline remain pending. This local run is a proxy, not G1 evidence.",
		duration: $duration,
		rate: $rate,
		batch_size: $batch,
		runs: $runs,
		mix: $mix,
		compression: $compression,
		chaos: {enabled: ($chaos == 1), clickhouse_seconds: $chaos_ch_seconds, victoriametrics_seconds: $chaos_vm_seconds},
		dedupe: {status: $dedupe_status, events: $dedupe_events},
		load_generator_events: $sent,
		wal_events: $wal,
		writer_accepted_events: $accepted,
		writer_accepted_measured: $accepted_measured,
		writer_written_events: $written,
		writer_deduped_events: $deduped,
		writer_failed_events: $failed,
		writer_skipped_events: $skipped,
		writer_retries: $retries,
		clickhouse: {logs: $ch_logs, traces: $ch_traces},
		victoriametrics: {samples: $vm_samples, series: $vm_series},
		signals: {
			logs: {accepted: $accepted_logs, written: $written_logs},
			traces: {accepted: $accepted_traces, written: $written_traces},
			metrics: {accepted: $accepted_metrics, written: $written_metrics}
		},
		lag_seconds: {p50: $lag_p50, p95: $lag_p95, p99: $lag_p99},
		exit_codes: {gateway: $gateway_exit, writer: $writer_exit},
		reason: $reason,
		runs_detail: $run_reports
	}' >"$RESULTS_DIR/report.json"

cat >"$RESULTS_DIR/report.md" <<EOF
# Story 4.4 — writers over the NATS WAL (local)

- **Status:** ${status}
- **T1 target:** ${t1_status} (1M/s for 1h; P-07 and the T1 baseline remain pending)
- **Duration per run:** ${DURATION}
- **Offered rate:** ${RATE} events/s total across two tenants
- **Runs:** ${RUNS}
- **Mix:** ${MIX}
- **Compression:** ${COMPRESSION}
- **Chaos:** ${CHAOS} (ClickHouse ${CHAOS_CH_SECONDS}s, VictoriaMetrics ${CHAOS_VM_SECONDS}s)
- **Dedupe phase:** ${dedupe_status} (${dedupe_events} events deduplicated)

| Metric | Value |
|---|---:|
| Load generator sent (measured runs) | ${total_sent} |
| Gateway WAL events (measured runs) | ${total_wal} |
| Writer accepted (measured runs) | ${total_accepted} |
| Writer accepted (all phases) | ${accepted_total} |
| Writer written | ${written_total} |
| Writer deduped | ${deduped_total} |
| Writer failed | ${failed_total} |
| Writer skipped | ${skipped_total} |
| Writer retries | ${retries_total} |
| ClickHouse logs | ${ch_logs} (unique ${ch_logs_unique}) |
| ClickHouse traces | ${ch_traces} (unique ${ch_traces_unique}) |
| VictoriaMetrics samples | ${vm_samples} (${vm_series} series) |
| Lag p50 / p95 / p99 | ${lag_p50}s / ${lag_p95}s / ${lag_p99}s |
| Exit codes gateway/writer | ${gateway_exit} / ${writer_exit} |

${reason_text:+**Failure:** ${reason_text}}

The gateway acknowledged only after the JetStream publish. The writer
acknowledged each WAL message only after ClickHouse or VictoriaMetrics
confirmed the flush. During the chaos window the writer retried with
exponential backoff and left the messages unacked in the WAL. This is a local
proxy: it does not prove the T1 throughput target or the 15-minute outage in
Story 4.5.
EOF

cat "$RESULTS_DIR/report.md"
if [ "$status" != pass ]; then
	exit 1
fi
log "pipeline: pass"
