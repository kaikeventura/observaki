#!/usr/bin/env bash
# Story 4.5 — chaos and go/no-go for the ingestion stack.
#
# Runs the full local pipeline (gateway -> NATS JetStream WAL -> writers ->
# ClickHouse/VictoriaMetrics) under a continuous OTLP load and injects one of:
#
#   storage   ClickHouse is stopped for the configured outage window. The WAL
#             absorbs the backlog; after recovery the writer drains it and the
#             harness proves that zero confirmed events were lost.
#   nats      The NATS node is stopped and started again. The gateway returns
#             5xx while disconnected, reconnects automatically and resumes.
#             Redelivered WAL messages must be absorbed by event_id dedupe.
#   gateway   The gateway is SIGKILLed and restarted. Confirmed events stay in
#             the WAL; ingestion resumes when the new process is ready.
#   capacity  The WAL streams are created with a tiny MaxBytes and ClickHouse
#             is stopped, so DiscardNew rejects new envelopes. The gateway must
#             answer 429 + Retry-After (wal_backpressure), then recover.
#   all       storage, then NATS, then gateway in one continuous load.
#
# The AC1 target is steady ingestion at 1M/s with ClickHouse down for 15
# minutes on the T1 fleet. That run remains blocked by P-07; this harness
# publishes a local proxy and never claims G1 evidence.
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
GATEWAY_BIN="$ROOT/bin/observaki"
LOADGEN_BIN="$ROOT/bin/otlploadgen"
MIGRATIONS="$ROOT/internal/adapters/clickhouse/migrations"
CH_CONFIG="$ROOT/bench/clickhouse/config/bench.xml"
CH_USERS="$ROOT/bench/clickhouse/ingest/config/users.xml"

SCENARIO=storage
RATE=5000
BATCH=200
WORKERS=2
MIX=34,33,33
COMPRESSION=gzip
METRIC_PADDING=832
LOG_BODY=820
TRACE_PADDING=2400
PRE=60s
OUTAGE=15m
NATS_OUTAGE=30s
GATEWAY_OUTAGE=15s
SETTLE=15s
POST=120s
OUTAGE_SET=0
CAPACITY_MAX_BYTES=1048576
DEDUPE=1
DEDUPE_DURATION=1s
RESULTS_DIR=""
SMOKE=0
NATS_IMAGE="${NATS_IMAGE:-nats:2.15.2-alpine}"
CH_IMAGE="${OBSERVAKI_CHAOS_CH_IMAGE:-clickhouse/clickhouse-server:26.8}"
VM_IMAGE="${OBSERVAKI_CHAOS_VM_IMAGE:-victoriametrics/victoria-metrics:v1.148.0}"

usage() {
	cat <<'EOF'
Usage: bench/chaos/run.sh [options]

Options:
  --scenario S          storage | nats | gateway | capacity | all (default: storage)
  --rate N              total events/s across both tenants (default: 5000)
  --pre T               steady load before the first failure (default: 60s)
  --outage T            ClickHouse outage (default: 15m; capacity: 90s)
  --nats-outage T       NATS outage for --scenario nats|all (default: 30s)
  --gateway-outage T    gateway outage for --scenario gateway|all (default: 15s)
  --settle T            quiet gap between failures in --scenario all (default: 15s)
  --post T              load after the last recovery (default: 120s)
  --batch-size N        events per OTLP request (default: 200)
  --workers N           workers per tenant load generator (default: 2)
  --mix a,b,c           metrics,logs,traces mix in events per 100 (default: 34,33,33)
  --compression C       gzip or none (default: gzip)
  --capacity-bytes N    per-stream MaxBytes for --scenario capacity (default: 1048576)
  --no-dedupe           skip the post-recovery identical-replay check
  --results-dir P       report destination
  --smoke               short harness check (2k/s, seconds-long windows, all)
  --help                show this help

Requires Docker, jq and curl. The harness never deletes ClickHouse or NATS
data during a scenario; only disposable containers created by this run.
EOF
}

log() {
	printf '[%s] %s\n' "$(date -u '+%H:%M:%S')" "$*" >&2
}

trap 'log "ERROR at line ${LINENO}: ${BASH_COMMAND}"' ERR

die() {
	log "ERROR: $*"
	exit 1
}

while [ $# -gt 0 ]; do
	case "$1" in
	--scenario) SCENARIO=$2; shift 2 ;;
	--rate) RATE=$2; shift 2 ;;
	--pre) PRE=$2; shift 2 ;;
	--outage) OUTAGE=$2; OUTAGE_SET=1; shift 2 ;;
	--nats-outage) NATS_OUTAGE=$2; shift 2 ;;
	--gateway-outage) GATEWAY_OUTAGE=$2; shift 2 ;;
	--settle) SETTLE=$2; shift 2 ;;
	--post) POST=$2; shift 2 ;;
	--batch-size) BATCH=$2; shift 2 ;;
	--workers) WORKERS=$2; shift 2 ;;
	--mix) MIX=$2; shift 2 ;;
	--compression) COMPRESSION=$2; shift 2 ;;
	--capacity-bytes) CAPACITY_MAX_BYTES=$2; shift 2 ;;
	--no-dedupe) DEDUPE=0; shift ;;
	--results-dir) RESULTS_DIR=$2; shift 2 ;;
	--smoke) SMOKE=1; shift ;;
	--help|-h) usage; exit 0 ;;
	*) die "unknown option $1 (see --help)" ;;
	esac
done

case "$SCENARIO" in
storage|nats|gateway|capacity|all) ;;
*) die "--scenario must be storage, nats, gateway, capacity or all" ;;
esac
case "$COMPRESSION" in
gzip|none) ;;
*) die "--compression must be gzip or none" ;;
esac

to_seconds() {
	local value=$1
	case "$value" in
	*ms) echo $(( ${value%ms} / 1000 )) ;;
	*s) echo "${value%s}" ;;
	*m) echo $(( ${value%m} * 60 )) ;;
	*h) echo $(( ${value%h} * 3600 )) ;;
	*) echo "$value" ;;
	esac
}

if [ "$SMOKE" = 1 ]; then
	SCENARIO=${OBSERVAKI_CHAOS_SMOKE_SCENARIO:-all}
	RATE=2000
	BATCH=100
	WORKERS=2
	PRE=5s
	[ "$OUTAGE_SET" = 1 ] || OUTAGE=10s
	NATS_OUTAGE=5s
	GATEWAY_OUTAGE=5s
	SETTLE=3s
	POST=15s
	CAPACITY_MAX_BYTES=1048576
	DEDUPE_DURATION=1s
	if [ "$SCENARIO" = capacity ]; then
		# The gateway config rejects streams below 1 MiB; push enough load to
		# fill the stream inside the short outage window.
		RATE=20000
		BATCH=200
		WORKERS=4
		PRE=5s
		POST=30s
	fi
fi
if [ "$SCENARIO" = capacity ] && [ "$OUTAGE_SET" = 0 ] && [ "$SMOKE" = 0 ]; then
	OUTAGE=90s
fi
if [ "$SCENARIO" = capacity ] && [ "$CAPACITY_MAX_BYTES" -lt 1048576 ]; then
	die "--capacity-bytes must be >= 1048576 (gateway config minimum)"
fi

RATE=$((RATE / 2 * 2))
[ "$RATE" -ge 2 ] || die "--rate must be >= 2"
PRE_S=$(to_seconds "$PRE")
OUTAGE_S=$(to_seconds "$OUTAGE")
NATS_OUTAGE_S=$(to_seconds "$NATS_OUTAGE")
GATEWAY_OUTAGE_S=$(to_seconds "$GATEWAY_OUTAGE")
SETTLE_S=$(to_seconds "$SETTLE")
POST_S=$(to_seconds "$POST")
for window in "$PRE_S" "$POST_S"; do
	[ "$window" -ge 1 ] || die "pre/post windows must be >= 1s"
done

storage_end=$((PRE_S + OUTAGE_S))
nats_start=$((storage_end + SETTLE_S))
nats_end=$((nats_start + NATS_OUTAGE_S))
gateway_start=$((nats_end + SETTLE_S))
gateway_end=$((gateway_start + GATEWAY_OUTAGE_S))

case "$SCENARIO" in
storage|capacity) TOTAL_S=$((PRE_S + OUTAGE_S + POST_S)) ;;
nats) TOTAL_S=$((PRE_S + NATS_OUTAGE_S + POST_S)) ;;
gateway) TOTAL_S=$((PRE_S + GATEWAY_OUTAGE_S + POST_S)) ;;
all) TOTAL_S=$((PRE_S + OUTAGE_S + SETTLE_S + NATS_OUTAGE_S + SETTLE_S + GATEWAY_OUTAGE_S + POST_S)) ;;
esac
TOTAL="${TOTAL_S}s"

command -v docker >/dev/null || die "docker is required"
command -v jq >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"
[ -x "$GATEWAY_BIN" ] || die "$GATEWAY_BIN is missing (run make build)"
[ -x "$LOADGEN_BIN" ] || die "$LOADGEN_BIN is missing (run make build)"

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/chaos/results/$(date -u +%Y%m%dT%H%M%SZ)-${SCENARIO}"
fi
mkdir -p "$RESULTS_DIR"
WORK_DIR=$(mktemp -d -t observaki-chaos.XXXXXX)
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
sampler_pid=""
gateway_restarts=0
ch_outages=0
nats_outages=0
gateway_outages=0
current_phase="pre"

gateway_pid_file() {
	echo "$WORK_DIR/gateway.pid"
}

cleanup() {
	rm -f "$WORK_DIR/stop-sampler" 2>/dev/null || true
	for p in "$chaos_pid" "$sampler_pid"; do
		if [ -n "${p:-}" ]; then
			kill "$p" 2>/dev/null || true
			wait "$p" 2>/dev/null || true
		fi
	done
	for p in "${load_pids[@]:-}"; do
		kill -TERM "$p" 2>/dev/null || true
		wait "$p" 2>/dev/null || true
	done
	local pidfile_pid=""
	if [ -f "$(gateway_pid_file)" ]; then
		pidfile_pid=$(cat "$(gateway_pid_file)" 2>/dev/null || true)
	fi
	for pid in "$gateway_pid" "$pidfile_pid" "$writer_pid"; do
		if [ -n "${pid:-}" ]; then
			kill -TERM "$pid" 2>/dev/null || true
			kill -9 "$pid" 2>/dev/null || true
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
	local start="${1:-18800}"
	local end="${2:-18900}"
	local port
	for port in $(seq "$start" "$end"); do
		if ! (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
			echo "$port"
			return 0
		fi
	done
	return 1
}

record_event() {
	local name=$1
	local phase=$2
	local now elapsed
	now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
	elapsed=$(( $(date +%s) - load_start_epoch ))
	jq -nc --arg ts "$now" --arg event "$name" --arg phase "$phase" --argjson elapsed "$elapsed" \
		'{ts: $ts, elapsed_seconds: $elapsed, event: $event, phase: $phase}' >>"$RESULTS_DIR/events.jsonl"
	printf '%s\n' "$phase" >"$WORK_DIR/phase"
}

nats_port=$(find_free_port 18800 18830) || die "no free NATS port"
nats_monitor_port=$(find_free_port 18831 18860) || die "no free NATS monitor port"
grpc_port=$(find_free_port 18861 18890) || die "no free gRPC port"
http_port=$(find_free_port 18891 18920) || die "no free gateway HTTP port"
writer_port=$(find_free_port 18921 18950) || die "no free writer port"
ch_port=$(find_free_port 18951 19000) || die "no free ClickHouse port"
vm_port=$(find_free_port 19001 19050) || die "no free VictoriaMetrics port"

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
nats_container="observaki-chaos-nats-$$"
log "starting NATS JetStream on 127.0.0.1:${nats_port}"
docker run -d --user "$(id -u):$(id -g)" --name "$nats_container" \
	-p "${nats_port}:4222" -p "${nats_monitor_port}:8222" \
	-v "$WORK_DIR/nats:/data" "$NATS_IMAGE" -c /data/nats.conf >/dev/null

wait_for_nats() {
	for _ in $(seq 1 90); do
		if curl -fsS "http://127.0.0.1:${nats_monitor_port}/varz" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.2
	done
	return 1
}
wait_for_nats || die "NATS did not become ready"

# --- ClickHouse -------------------------------------------------------------
mkdir -p "$WORK_DIR/clickhouse"
chmod 777 "$WORK_DIR/clickhouse" 2>/dev/null || true
ch_container="observaki-chaos-ch-$$"
log "starting ClickHouse on 127.0.0.1:${ch_port}"
docker run -d --name "$ch_container" \
	-p "127.0.0.1:${ch_port}:8123" \
	-v "$WORK_DIR/clickhouse:/var/lib/clickhouse" \
	-v "$CH_CONFIG:/etc/clickhouse-server/config.d/bench.xml:ro" \
	-v "$CH_USERS:/etc/clickhouse-server/users.d/observaki-bench.xml:ro" \
	--ulimit nofile=262144:262144 \
	"$CH_IMAGE" >/dev/null

wait_for_clickhouse() {
	for _ in $(seq 1 180); do
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
vm_container="observaki-chaos-vm-$$"
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

wait_for_vm() {
	for _ in $(seq 1 90); do
		if curl -fsS "http://127.0.0.1:${vm_port}/health" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.2
	done
	return 1
}
wait_for_vm || die "VictoriaMetrics did not become ready"

# --- Auth snapshot ----------------------------------------------------------
token_a="chaos-a-$(date -u +%s%N)-${RANDOM}"
token_b="chaos-b-$(date -u +%s%N)-${RANDOM}"
hash_a=$(printf '%s' "$token_a" | sha256sum | cut -d' ' -f1)
hash_b=$(printf '%s' "$token_b" | sha256sum | cut -d' ' -f1)
issued_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
expires_at=$(date -u -d '+6 hours' +%Y-%m-%dT%H:%M:%SZ)
snapshot="$WORK_DIR/ingest-auth.json"
jq -n \
	--arg version "chaos-$(date -u +%s)" \
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
stream_max_bytes=8589934592
if [ "$SCENARIO" = capacity ]; then
	stream_max_bytes=$CAPACITY_MAX_BYTES
fi

start_gateway() {
	local tag=${1:-gateway}
	OBSERVAKI_ENV=development \
		OBSERVAKI_OTLP_GRPC_ADDR="127.0.0.1:${grpc_port}" \
		OBSERVAKI_OTLP_HTTP_ADDR="127.0.0.1:${http_port}" \
		OBSERVAKI_INGEST_AUTH_FILE="$snapshot" \
		OBSERVAKI_INGEST_AUDIT_FILE="$WORK_DIR/audit.jsonl" \
		OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL=5s \
		OBSERVAKI_NATS_URL="$nats_url" \
		OBSERVAKI_NATS_SHARDS=32 \
		OBSERVAKI_NATS_STREAM_MAX_BYTES="$stream_max_bytes" \
		OBSERVAKI_NATS_BATCH_LINGER=5ms \
		OBSERVAKI_NATS_BATCH_MAX_EVENTS=100000 \
		OBSERVAKI_NATS_BATCH_MAX_BYTES=4194304 \
		"$GATEWAY_BIN" gateway >"$RESULTS_DIR/${tag}.log" 2>&1 &
	gateway_pid=$!
	echo "$gateway_pid" >"$(gateway_pid_file)"
}

wait_for_gateway() {
	for _ in $(seq 1 100); do
		if curl -fsS "http://127.0.0.1:${http_port}/readyz" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.2
	done
	return 1
}

log "starting gateway on grpc=127.0.0.1:${grpc_port} http=127.0.0.1:${http_port}"
start_gateway gateway
wait_for_gateway || die "gateway did not become ready (see $RESULTS_DIR/gateway.log)"

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

for _ in $(seq 1 100); do
	if curl -fsS "http://127.0.0.1:${writer_port}/readyz" >/dev/null 2>&1; then
		break
	fi
	sleep 0.2
done
curl -fsS "http://127.0.0.1:${writer_port}/readyz" >/dev/null 2>&1 || die "writer did not become ready (see $RESULTS_DIR/writer.log)"

writer_metrics() {
	if [ -n "${WRITER_METRICS_CACHE+x}" ]; then
		printf '%s\n' "$WRITER_METRICS_CACHE"
		return 0
	fi
	curl -fsS --max-time 2 "http://127.0.0.1:${writer_port}/metrics"
}
gateway_metrics() {
	if [ -n "${GATEWAY_METRICS_CACHE+x}" ]; then
		printf '%s\n' "$GATEWAY_METRICS_CACHE"
		return 0
	fi
	curl -fsS --max-time 2 "http://127.0.0.1:${http_port}/metrics"
}

metric_sum() {
	local name=$1
	local signal=$2
	writer_metrics 2>/dev/null | awk -v name="$name" -v signal="$signal" '
		index($1, name "{") == 1 && index($1, "signal=\"" signal "\"") > 0 {sum += $2}
		END {printf "%.0f", sum}
	'
}

metric_max() {
	local name=$1
	local filter=${2:-}
	writer_metrics 2>/dev/null | awk -v name="$name" -v filter="$filter" '
		index($1, name) == 1 && (filter == "" || index($1, filter) > 0) {
			if ($2 > max) max = $2
		}
		END {printf "%.6f", max}
	'
}

metric_value() {
	local name=$1
	writer_metrics 2>/dev/null | awk -v name="$name" '$1 == name {print $2; exit}'
}

gateway_metric_value() {
	local name=$1
	gateway_metrics 2>/dev/null | awk -v name="$name" '$1 == name {print $2; exit}'
}

# A restarted gateway resets its counters. Every instance is scraped right
# before it is killed into gateway-metrics-killed-N.txt, and the final instance
# is scraped live, so the report can sum confirmed WAL events across restarts.
sum_gateway_metric() {
	local name=$1
	local total=0
	local file value
	for file in "$WORK_DIR"/gateway-metrics-killed-*.txt; do
		[ -f "$file" ] || continue
		value=$(awk -v name="$name" '$1 == name {print $2; exit}' "$file")
		total=$((total + ${value:-0}))
	done
	value=$(gateway_metric_value "$name")
	total=$((total + ${value:-0}))
	echo "$total"
}

signal_sum() {
	local name=$1
	local total=0
	for signal in traces logs metrics; do
		total=$((total + $(metric_sum "$name" "$signal")))
	done
	echo "$total"
}

drain_pipeline() {
	local timeout=${1:-300}
	local started
	started=$(date +%s)
	while true; do
		local in_flight pending accepted written deduped failed skipped
		in_flight=$(metric_value "observaki_writer_in_flight_events")
		in_flight=${in_flight:-0}
		pending=0
		for signal in traces logs metrics; do
			local value
			value=$(writer_metrics 2>/dev/null | awk -v signal="$signal" '
				index($1, "observaki_writer_consumer_pending_messages{signal=\"" signal "\"}") == 1 {print $2; exit}
			')
			pending=$((pending + ${value:-0}))
		done
		accepted=$(signal_sum "observaki_writer_accepted_events_total")
		written=$(signal_sum "observaki_writer_written_events_total")
		deduped=$(signal_sum "observaki_writer_deduped_events_total")
		failed=$(signal_sum "observaki_writer_failed_events_total")
		skipped=$(signal_sum "observaki_writer_skipped_events_total")
		if [ "$in_flight" = "0" ] && [ "$pending" = "0" ] && [ "$accepted" = "$((written + deduped + failed + skipped))" ]; then
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
	local rate=$1
	local token=$2
	local output=$3
	local duration=$4
	local start_time=$5
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

# --- Chaos plan -------------------------------------------------------------
stop_clickhouse() {
	log "chaos: stopping ClickHouse for ${OUTAGE}"
	ch_outages=$((ch_outages + 1))
	record_event "clickhouse_stopped" "storage-outage"
	docker stop -t 10 "$ch_container" >/dev/null
}

start_clickhouse() {
	log "chaos: starting ClickHouse"
	record_event "clickhouse_started" "storage-recovery"
	docker start "$ch_container" >/dev/null
	wait_for_clickhouse || die "ClickHouse did not recover"
	log "chaos: ClickHouse recovered"
	record_event "clickhouse_recovered" "storage-recovery"
}

stop_nats() {
	log "chaos: stopping NATS for ${NATS_OUTAGE}"
	nats_outages=$((nats_outages + 1))
	record_event "nats_stopped" "nats-outage"
	docker stop -t 5 "$nats_container" >/dev/null
}

start_nats() {
	log "chaos: starting NATS"
	record_event "nats_started" "nats-recovery"
	docker start "$nats_container" >/dev/null
	wait_for_nats || die "NATS did not recover"
	log "chaos: NATS recovered"
	record_event "nats_recovered" "nats-recovery"
}

kill_gateway() {
	log "chaos: SIGKILL gateway for ${GATEWAY_OUTAGE}"
	gateway_outages=$((gateway_outages + 1))
	record_event "gateway_killed" "gateway-outage"
	# Capture the counter of the instance that is about to die so the report
	# can sum WAL publishes across restarts.
	gateway_metrics >"$WORK_DIR/gateway-metrics-killed-${gateway_outages}.txt" 2>/dev/null || true
	local pid
	pid=$(cat "$(gateway_pid_file)" 2>/dev/null || true)
	if [ -n "$pid" ]; then
		kill -9 "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	rm -f "$(gateway_pid_file)"
	gateway_pid=""
}

restart_gateway() {
	log "chaos: restarting gateway"
	gateway_restarts=$((gateway_restarts + 1))
	record_event "gateway_started" "gateway-recovery"
	start_gateway "gateway-restart-${gateway_restarts}"
	wait_for_gateway || die "gateway did not recover"
	log "chaos: gateway recovered"
	record_event "gateway_recovered" "gateway-recovery"
}

chaos_plan() {
	sleep "$PRE_S"
	case "$SCENARIO" in
	storage)
		stop_clickhouse
		sleep "$OUTAGE_S"
		start_clickhouse
		;;
	capacity)
		stop_clickhouse
		sleep "$OUTAGE_S"
		start_clickhouse
		;;
	nats)
		stop_nats
		sleep "$NATS_OUTAGE_S"
		start_nats
		;;
	gateway)
		kill_gateway
		sleep "$GATEWAY_OUTAGE_S"
		restart_gateway
		;;
	all)
		stop_clickhouse
		sleep "$OUTAGE_S"
		start_clickhouse
		sleep "$SETTLE_S"
		stop_nats
		sleep "$NATS_OUTAGE_S"
		start_nats
		sleep "$SETTLE_S"
		kill_gateway
		sleep "$GATEWAY_OUTAGE_S"
		restart_gateway
		;;
	esac
	record_event "chaos_complete" "post"
}

# --- Timeline sampler -------------------------------------------------------
sample_timeline() {
	local now elapsed ready writer_up accepted written deduped failed retries pending in_flight
	local wal_published wal_backpressure wal_errors nats_msgs nats_bytes lag_p95 phase
	while [ ! -f "$WORK_DIR/stop-sampler" ]; do
		now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
		elapsed=$(( $(date +%s) - load_start_epoch ))
		phase=$(cat "$WORK_DIR/phase" 2>/dev/null || echo "$current_phase")
		ready=$(curl -s --max-time 2 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${http_port}/readyz" 2>/dev/null || true)
		ready=${ready:-000}
		# One scrape per iteration: the helpers below read the cache instead
		# of issuing a request per metric, so a sample stays well under 1 s.
		WRITER_METRICS_CACHE=$(curl -fsS --max-time 2 "http://127.0.0.1:${writer_port}/metrics" 2>/dev/null || true)
		GATEWAY_METRICS_CACHE=$(curl -fsS --max-time 2 "http://127.0.0.1:${http_port}/metrics" 2>/dev/null || true)
		writer_up=$(metric_value "observaki_writer_up")
		writer_up=${writer_up:-0}
		wal_published=$(gateway_metric_value "observaki_gateway_wal_published_events_total")
		wal_published=${wal_published:-0}
		wal_backpressure=$(gateway_metric_value "observaki_gateway_wal_backpressure_total")
		wal_backpressure=${wal_backpressure:-0}
		wal_errors=$(gateway_metric_value "observaki_gateway_wal_publish_errors_total")
		wal_errors=${wal_errors:-0}
		nats_json=$(curl -fsS --max-time 2 "http://127.0.0.1:${nats_monitor_port}/jsz?streams=true" 2>/dev/null || echo '{}')
		nats_msgs=$(printf '%s' "$nats_json" | jq '[.account_details[]?.stream_detail[]?.state.messages] | add // 0' 2>/dev/null || echo 0)
		nats_bytes=$(printf '%s' "$nats_json" | jq '[.account_details[]?.stream_detail[]?.state.bytes] | add // 0' 2>/dev/null || echo 0)
		accepted=$(signal_sum "observaki_writer_accepted_events_total")
		written=$(signal_sum "observaki_writer_written_events_total")
		deduped=$(signal_sum "observaki_writer_deduped_events_total")
		failed=$(signal_sum "observaki_writer_failed_events_total")
		retries=$(signal_sum "observaki_writer_retries_total")
		pending=0
		for signal in traces logs metrics; do
			value=$(writer_metrics 2>/dev/null | awk -v signal="$signal" '
				index($1, "observaki_writer_consumer_pending_messages{signal=\"" signal "\"}") == 1 {print $2; exit}
			')
			pending=$((pending + ${value:-0}))
		done
		in_flight=$(metric_value "observaki_writer_in_flight_events")
		in_flight=${in_flight:-0}
		lag_p95=$(metric_max "observaki_writer_lag_seconds" 'quantile="0.95"')
		jq -nc \
			--arg ts "$now" --argjson elapsed "$elapsed" --arg phase "$phase" \
			--arg ready "$ready" --argjson writer_up "$writer_up" \
			--argjson wal_published "$wal_published" --argjson wal_backpressure "$wal_backpressure" \
			--argjson wal_errors "$wal_errors" --argjson nats_msgs "$nats_msgs" --argjson nats_bytes "$nats_bytes" \
			--argjson accepted "$accepted" --argjson written "$written" --argjson deduped "$deduped" \
			--argjson failed "$failed" --argjson retries "$retries" --argjson pending "$pending" \
			--argjson in_flight "$in_flight" --argjson lag_p95 "$lag_p95" \
			'{ts: $ts, elapsed_seconds: $elapsed, phase: $phase, gateway_ready: $ready,
			  writer_up: $writer_up, wal_published_events: $wal_published,
			  wal_backpressure: $wal_backpressure, wal_publish_errors: $wal_errors,
			  nats_stream_messages: $nats_msgs, nats_stream_bytes: $nats_bytes,
			  writer_accepted: $accepted, writer_written: $written, writer_deduped: $deduped,
			  writer_failed: $failed, writer_retries: $retries, writer_pending: $pending,
			  writer_in_flight_events: $in_flight, lag_p95_seconds: $lag_p95}' \
			>>"$RESULTS_DIR/timeline.jsonl"
		sleep 1
	done
}

# --- Load -------------------------------------------------------------------
printf 'pre\n' >"$WORK_DIR/phase"
: >"$RESULTS_DIR/events.jsonl"
: >"$RESULTS_DIR/timeline.jsonl"
load_start_epoch=$(date +%s)
load_start_time=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
log "scenario=${SCENARIO} rate=${RATE}/s total=${TOTAL} (pre=${PRE} outage=${OUTAGE} nats=${NATS_OUTAGE} gateway=${GATEWAY_OUTAGE} post=${POST})"
log "results: $RESULTS_DIR"
(sample_timeline) &
sampler_pid=$!

run_loadgen "$((RATE / 2))" "$token_a" "$RESULTS_DIR/tenant-a.json" "$TOTAL" "$load_start_time" &
load_pids+=("$!")
run_loadgen "$((RATE / 2))" "$token_b" "$RESULTS_DIR/tenant-b.json" "$TOTAL" "$load_start_time" &
load_pids+=("$!")

chaos_plan &
chaos_pid=$!

for pid in "${load_pids[@]}"; do
	# A chaos run makes the load generator fail requests by design; its exit
	# code is not a harness failure. The counts are verified below.
	wait "$pid" || log "load generator exited non-zero (client failures are expected during chaos)"
done
load_pids=()
record_event "load_complete" "post"
if [ -n "$chaos_pid" ]; then
	chaos_exit=0
	wait "$chaos_pid" || chaos_exit=$?
	chaos_pid=""
	if [ "$chaos_exit" -ne 0 ]; then
		die "chaos plan failed with exit ${chaos_exit}; see $RESULTS_DIR/run.log"
	fi
fi
sleep 1
: >"$WORK_DIR/stop-sampler"
wait "$sampler_pid" 2>/dev/null || true
sampler_pid=""
record_event "draining" "post"

# Counters are written by the chaos subshell, so derive them from the
# append-only event log instead of parent-shell variables.
ch_outages=$(jq -s '[.[] | select(.event == "clickhouse_stopped")] | length' "$RESULTS_DIR/events.jsonl")
nats_outages=$(jq -s '[.[] | select(.event == "nats_stopped")] | length' "$RESULTS_DIR/events.jsonl")
gateway_outages=$(jq -s '[.[] | select(.event == "gateway_killed")] | length' "$RESULTS_DIR/events.jsonl")
gateway_restarts=$(jq -s '[.[] | select(.event == "gateway_started")] | length' "$RESULTS_DIR/events.jsonl")

sent_a=$(jq -r '.metrics.sent' "$RESULTS_DIR/tenant-a.json")
sent_b=$(jq -r '.metrics.sent' "$RESULTS_DIR/tenant-b.json")
failed_a=$(jq -r '.metrics.failed' "$RESULTS_DIR/tenant-a.json")
failed_b=$(jq -r '.metrics.failed' "$RESULTS_DIR/tenant-b.json")
rejected_a=$(jq -r '.metrics.rejected' "$RESULTS_DIR/tenant-a.json")
rejected_b=$(jq -r '.metrics.rejected' "$RESULTS_DIR/tenant-b.json")
sent=$((sent_a + sent_b))
client_failed=$((failed_a + failed_b))
client_rejected=$((rejected_a + rejected_b))
signal_sent() {
	local signal=$1
	local total=0
	total=$(jq -r --arg signal "$signal" '[.metrics.signals[] | select(.signal == $signal) | .sent] | add // 0' "$RESULTS_DIR/tenant-a.json")
	total=$((total + $(jq -r --arg signal "$signal" '[.metrics.signals[] | select(.signal == $signal) | .sent] | add // 0' "$RESULTS_DIR/tenant-b.json")))
	echo "$total"
}
sent_logs=$(signal_sent logs)
sent_traces=$(signal_sent traces)
sent_metrics=$(signal_sent metrics)


if ! drain_pipeline 900; then
	die "pipeline did not drain after the chaos run"
fi
record_event "drained" "post"

# --- Post-recovery replay check --------------------------------------------
dedupe_status="skipped"
dedupe_events=0
if [ "$DEDUPE" = 1 ]; then
	log "replay check: two identical deterministic loads (same seed and start time)"
	replay_start=$(date -u -d '-2 seconds' '+%Y-%m-%dT%H:%M:%SZ')
	replay_rate=$((RATE / 2))
	run_loadgen "$replay_rate" "$token_a" "$RESULTS_DIR/replay-a.json" "$DEDUPE_DURATION" "$replay_start" ||
		log "replay load reported client failures (expected under WAL pressure)"
	drain_pipeline 120 || die "pipeline did not drain after the first replay load"
	deduped_before=$(signal_sum "observaki_writer_deduped_events_total")
	run_loadgen "$replay_rate" "$token_a" "$RESULTS_DIR/replay-b.json" "$DEDUPE_DURATION" "$replay_start" ||
		log "replay load reported client failures (expected under WAL pressure)"
	drain_pipeline 120 || die "pipeline did not drain after the second replay load"
	deduped_after=$(signal_sum "observaki_writer_deduped_events_total")
	dedupe_events=$((deduped_after - deduped_before))
	if [ "$dedupe_events" -gt 0 ]; then
		dedupe_status="pass"
	else
		dedupe_status="fail"
	fi
	log "replay check: ${dedupe_events} events deduplicated (${dedupe_status})"
fi

# --- Final scrape -----------------------------------------------------------
writer_metrics >"$RESULTS_DIR/writer-metrics.txt" 2>/dev/null || true
gateway_metrics >"$RESULTS_DIR/gateway-metrics.txt" 2>/dev/null || true
curl -fsS --max-time 2 "http://127.0.0.1:${nats_monitor_port}/jsz?streams=true" >"$RESULTS_DIR/nats-streams-final.json" 2>/dev/null || echo '{}' >"$RESULTS_DIR/nats-streams-final.json"

accepted_logs=$(metric_sum "observaki_writer_accepted_events_total" "logs")
accepted_traces=$(metric_sum "observaki_writer_accepted_events_total" "traces")
accepted_metrics=$(metric_sum "observaki_writer_accepted_events_total" "metrics")
written_logs=$(metric_sum "observaki_writer_written_events_total" "logs")
written_traces=$(metric_sum "observaki_writer_written_events_total" "traces")
written_metrics=$(metric_sum "observaki_writer_written_events_total" "metrics")
accepted_total=$((accepted_logs + accepted_traces + accepted_metrics))
written_total=$((written_logs + written_traces + written_metrics))
deduped_total=$(signal_sum "observaki_writer_deduped_events_total")
failed_total=$(signal_sum "observaki_writer_failed_events_total")
skipped_total=$(signal_sum "observaki_writer_skipped_events_total")
retries_total=$(signal_sum "observaki_writer_retries_total")
wal_published=$(sum_gateway_metric "observaki_gateway_wal_published_events_total")
wal_backpressure=$(sum_gateway_metric "observaki_gateway_wal_backpressure_total")
wal_publish_errors=$(sum_gateway_metric "observaki_gateway_wal_publish_errors_total")
lag_p95=$(metric_max "observaki_writer_lag_seconds" 'quantile="0.95"')
lag_p50=$(metric_max "observaki_writer_lag_seconds" 'quantile="0.5"')
lag_p99=$(metric_max "observaki_writer_lag_seconds" 'quantile="0.99"')
peak_wal_messages=$(jq -s 'map(.nats_stream_messages) | max // 0' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo 0)
peak_wal_bytes=$(jq -s 'map(.nats_stream_bytes) | max // 0' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo 0)
wal_messages_start=$(jq -s '.[0].nats_stream_messages // 0' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo 0)
wal_growth=$((peak_wal_messages - wal_messages_start))
[ "$wal_growth" -ge 0 ] || wal_growth=0
gateway_ready_min=$(jq -rs 'map(.gateway_ready) | min // "000"' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo "000")
gateway_ready_max=$(jq -rs 'map(.gateway_ready) | max // "000"' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo "000")
gateway_ready_last=$(jq -rs '.[-1].gateway_ready // "000"' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo "000")
# Recent-window lag in the post phase. A short smoke cannot refill the 4096
# sample window, so the check is skipped when there are fewer than 3 samples.
lag_p95_post=$(jq -rs '[.[] | select(.phase == "post") | .lag_p95_seconds] | if length >= 3 then (.[-5:] | sort | .[(length / 2 | floor)]) else -1 end' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo -1)
post_wal_growth=$(jq -rs '[.[] | select(.phase == "post") | .wal_published_events] | if length >= 2 then (.[-1] - .[0]) else 0 end' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo 0)

phase_non200() {
	local phase=$1
	jq -s --arg phase "$phase" '[.[] | select(.phase == $phase and .gateway_ready != "200")] | length' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo 0
}
nats_outage_non200=$(phase_non200 "nats-outage")
gateway_outage_non200=$(phase_non200 "gateway-outage")
storage_outage_retry_samples=$(jq -s '[.[] | select(.phase == "storage-outage" and .writer_retries > 0)] | length' "$RESULTS_DIR/timeline.jsonl" 2>/dev/null || echo 0)

log "stopping gateway and writer"
if [ -f "$(gateway_pid_file)" ]; then
	gateway_pid=$(cat "$(gateway_pid_file)" 2>/dev/null || true)
fi
stop_process() {
	local pid=$1
	local state
	[ -n "$pid" ] || return 0
	kill -TERM "$pid" 2>/dev/null || true
	for _ in $(seq 1 300); do
		state=$(ps -o stat= -p "$pid" 2>/dev/null | tr -d ' ' || true)
		if [ -z "$state" ] || [ "${state#Z}" != "$state" ]; then
			return 0
		fi
		sleep 0.1
	done
	kill -9 "$pid" 2>/dev/null || true
	return 1
}
gateway_exit=0
stop_process "$gateway_pid" || gateway_exit=1
gateway_pid=""
rm -f "$(gateway_pid_file)"
kill -TERM "$writer_pid" 2>/dev/null || true
writer_exit=0
wait "$writer_pid" 2>/dev/null || writer_exit=$?
writer_pid=""

ch_logs=$(query "SELECT count() FROM otel_logs")
ch_logs_unique=$(query "SELECT uniqExact(event_id) FROM otel_logs")
ch_traces=$(query "SELECT count() FROM otel_traces")
ch_traces_unique=$(query "SELECT uniqExact(event_id) FROM otel_traces")
duplicate_rows=$(( (ch_logs - ch_logs_unique) + (ch_traces - ch_traces_unique) ))
stored_rows=$((ch_logs + ch_traces))
duplicate_rate_pct=$(awk -v dup="$duplicate_rows" -v rows="$stored_rows" 'BEGIN {if (rows == 0) print 0; else printf "%.4f", dup * 100 / rows}')
last_recovery=$(jq -rs '[.[] | select(.event | test("_recovered$")) | .elapsed_seconds] | max // -1' "$RESULTS_DIR/events.jsonl" 2>/dev/null || echo -1)
drained_at=$(jq -rs '[.[] | select(.event == "drained") | .elapsed_seconds] | .[0] // -1' "$RESULTS_DIR/events.jsonl" 2>/dev/null || echo -1)
recovery_seconds=-1
if [ "$last_recovery" -ge 0 ] && [ "$drained_at" -ge "$last_recovery" ]; then
	recovery_seconds=$((drained_at - last_recovery))
fi
vm_query='sum(count_over_time({__name__="bench.http.server.duration"}[2h]))'
vm_samples=0
vm_series=0
metrics_tolerance=$(awk -v written="$written_metrics" 'BEGIN {printf "%.0f", written * 0.99}')
for _ in $(seq 1 60); do
	vm_now=$(date +%s)
	vm_samples=$(curl -fsS --get "http://127.0.0.1:${vm_port}/api/v1/query" --data-urlencode "query=${vm_query}" --data-urlencode "time=${vm_now}" 2>/dev/null | jq -r '.data.result[0].value[1] // "0"' | awk '{printf "%.0f", $1}')
	vm_series=$(curl -fsS --get "http://127.0.0.1:${vm_port}/api/v1/query" --data-urlencode 'query=count({__name__="bench.http.server.duration"})' --data-urlencode "time=${vm_now}" 2>/dev/null | jq -r '.data.result[0].value[1] // "0"' | awk '{printf "%.0f", $1}')
	if [ "$vm_samples" -ge "$metrics_tolerance" ] 2>/dev/null || { [ "$written_metrics" -eq 0 ] && [ "$vm_series" -gt 0 ]; }; then
		break
	fi
	sleep 1
done

# --- Checks -----------------------------------------------------------------
status=pass
t1_status=pending
checks=()
fail_reasons=()

check() {
	local name=$1
	local result=$2
	local detail=$3
	checks+=("$(jq -nc --arg name "$name" --arg status "$result" --arg detail "$detail" '{name: $name, status: $status, detail: $detail}')")
	if [ "$result" = fail ]; then
		status=fail
		fail_reasons+=("$name: $detail")
	fi
}

check "gateway_confirmed_events_reached_wal" "$([ "$wal_published" -ge "$sent" ] && echo pass || echo fail)" "wal=${wal_published} confirmed=${sent}"
check "wal_events_reached_writer" "$([ "$accepted_total" -ge "$sent" ] && echo pass || echo fail)" "accepted=${accepted_total} confirmed=${sent}"
check "writer_accounting_closes" "$([ "$accepted_total" -eq "$((written_total + deduped_total + failed_total + skipped_total))" ] && echo pass || echo fail)" "accepted=${accepted_total} written=${written_total} deduped=${deduped_total} failed=${failed_total} skipped=${skipped_total}"
check "zero_writer_failures" "$([ "$failed_total" -eq 0 ] && echo pass || echo fail)" "failed=${failed_total}"
check "zero_confirmed_events_lost" "$([ "$written_total" -ge "$sent" ] && echo pass || echo fail)" "written=${written_total} confirmed=${sent}"
check "confirmed_logs_stored" "$([ "$ch_logs_unique" -ge "$sent_logs" ] && echo pass || echo fail)" "unique_logs=${ch_logs_unique} confirmed_logs=${sent_logs}"
check "confirmed_traces_stored" "$([ "$ch_traces_unique" -ge "$sent_traces" ] && echo pass || echo fail)" "unique_traces=${ch_traces_unique} confirmed_traces=${sent_traces}"
metrics_sent_tolerance=$(awk -v sent="$sent_metrics" 'BEGIN {printf "%.0f", sent * 0.99}')
check "confirmed_metrics_stored" "$([ "$vm_samples" -ge "$metrics_sent_tolerance" ] && echo pass || echo fail)" "vm_samples=${vm_samples} confirmed_metrics=${sent_metrics} tolerance=${metrics_sent_tolerance}"
check "clickhouse_logs_match_writer" "$([ "$ch_logs" = "$written_logs" ] && echo pass || echo fail)" "clickhouse=${ch_logs} writer=${written_logs}"
check "clickhouse_traces_not_above_writer" "$([ "$ch_traces" -le "$written_traces" ] && echo pass || echo fail)" "clickhouse=${ch_traces} writer=${written_traces} block_deduped=$((written_traces - ch_traces))"
check "duplicate_rows_within_bound" "$(awk -v rate="$duplicate_rate_pct" 'BEGIN {exit !(rate <= 1.0)}' && echo pass || echo fail)" "duplicate_rows=${duplicate_rows} stored_rows=${stored_rows} rate=${duplicate_rate_pct}% (bound 1%)"
check "victoriametrics_samples_match_writer" "$([ "$vm_samples" -ge "$metrics_tolerance" ] && [ "$vm_samples" -le "$written_metrics" ] && echo pass || echo fail)" "samples=${vm_samples} writer=${written_metrics} tolerance=${metrics_tolerance}"
if [ "$ch_outages" -gt 0 ]; then
	check "steady_state_lag_p95_under_10s" "skip" "a clickhouse outage keeps backlog lag in the 4096-sample window; observed post p95=${lag_p95_post}s (see 4.4 stable run for the <10s gate)"
elif [ "$lag_p95_post" = "-1" ] || [ "$POST_S" -lt 60 ]; then
	check "steady_state_lag_p95_under_10s" "skip" "post window shorter than 60s cannot refill the 4096-sample lag ring"
else
	check "steady_state_lag_p95_under_10s" "$(awk -v p95="$lag_p95_post" 'BEGIN {exit !(p95 < 10)}' && echo pass || echo fail)" "post_phase_p95=${lag_p95_post}s"
fi
check "gateway_exited_cleanly" "$([ "$gateway_exit" -eq 0 ] && echo pass || echo fail)" "exit=${gateway_exit}"
check "writer_exited_cleanly" "$([ "$writer_exit" -eq 0 ] && echo pass || echo fail)" "exit=${writer_exit}"

if [ "$ch_outages" -gt 0 ]; then
	check "storage_outage_used_wal_or_backpressure" "$([ "$wal_growth" -gt 0 ] || [ "$wal_backpressure" -gt 0 ] && echo pass || echo fail)" "wal_growth_messages=${wal_growth} backpressure=${wal_backpressure}"
	if [ "$SCENARIO" != capacity ]; then
		check "writer_retried_during_storage_outage" "$([ "$retries_total" -gt 0 ] && echo pass || echo fail)" "retries=${retries_total} samples_with_retry=${storage_outage_retry_samples}"
	fi
fi
if [ "$nats_outages" -gt 0 ]; then
	check "gateway_saw_nats_outage" "$([ "$nats_outage_non200" -gt 0 ] && echo pass || echo fail)" "non_200_samples=${nats_outage_non200}"
	check "gateway_recovered_after_nats" "$([ "$gateway_ready_last" = "200" ] && echo pass || echo fail)" "last_readyz=${gateway_ready_last}"
fi
if [ "$gateway_outages" -gt 0 ]; then
	check "gateway_saw_gateway_outage" "$([ "$gateway_outage_non200" -gt 0 ] && echo pass || echo fail)" "non_200_samples=${gateway_outage_non200}"
	check "gateway_restarted" "$([ "$gateway_restarts" -gt 0 ] && [ "$gateway_exit" -eq 0 ] && echo pass || echo fail)" "restarts=${gateway_restarts} exit=${gateway_exit}"
	check "gateway_recovered_after_restart" "$([ "$gateway_ready_last" = "200" ] && echo pass || echo fail)" "last_readyz=${gateway_ready_last}"
fi
if [ "$SCENARIO" = capacity ]; then
	check "wal_capacity_returned_429" "$([ "$wal_backpressure" -gt 0 ] && echo pass || echo fail)" "backpressure_requests=${wal_backpressure} wal_errors=${wal_publish_errors}"
	check "capacity_rejected_some_client_requests" "$([ $((client_failed + client_rejected)) -gt 0 ] && echo pass || echo fail)" "client_failed=${client_failed} client_rejected=${client_rejected}"
	check "ingestion_resumed_after_wal_drained" "$([ "$post_wal_growth" -gt 0 ] && echo pass || echo fail)" "post_phase_wal_published_growth=${post_wal_growth}"
fi
if [ "$DEDUPE" = 1 ]; then
	check "identical_replay_deduped" "$([ "$dedupe_status" = pass ] && echo pass || echo fail)" "deduped=${dedupe_events}"
fi

checks_json=$(printf '%s\n' "${checks[@]}" | jq -s '.')
events_json=$(jq -s '.' "$RESULTS_DIR/events.jsonl" 2>/dev/null || echo '[]')
reason_text=""
if [ ${#fail_reasons[@]} -gt 0 ]; then
	reason_text=$(printf '%s; ' "${fail_reasons[@]}")
	reason_text=${reason_text%; }
fi

jq -n \
	--arg status "$status" \
	--arg t1_status "$t1_status" \
	--arg scenario "$SCENARIO" \
	--argjson rate "$RATE" \
	--argjson total_seconds "$TOTAL_S" \
	--argjson pre_seconds "$PRE_S" \
	--argjson outage_seconds "$OUTAGE_S" \
	--argjson nats_outage_seconds "$NATS_OUTAGE_S" \
	--argjson gateway_outage_seconds "$GATEWAY_OUTAGE_S" \
	--argjson settle_seconds "$SETTLE_S" \
	--argjson post_seconds "$POST_S" \
	--argjson ch_outages "$ch_outages" \
	--argjson nats_outages "$nats_outages" \
	--argjson gateway_outages "$gateway_outages" \
	--argjson gateway_restarts "$gateway_restarts" \
	--argjson sent "$sent" \
	--argjson sent_logs "$sent_logs" \
	--argjson sent_traces "$sent_traces" \
	--argjson sent_metrics "$sent_metrics" \
	--argjson duplicate_rows "$duplicate_rows" \
	--arg duplicate_rate_pct "$duplicate_rate_pct" \
	--argjson recovery_seconds "$recovery_seconds" \
	--argjson client_failed "$client_failed" \
	--argjson client_rejected "$client_rejected" \
	--argjson wal_published "$wal_published" \
	--argjson wal_backpressure "$wal_backpressure" \
	--argjson wal_publish_errors "$wal_publish_errors" \
	--argjson peak_wal_messages "$peak_wal_messages" \
	--argjson peak_wal_bytes "$peak_wal_bytes" \
	--argjson wal_growth "$wal_growth" \
	--arg gateway_ready_min "$gateway_ready_min" \
	--arg gateway_ready_last "$gateway_ready_last" \
	--argjson nats_outage_non200 "$nats_outage_non200" \
	--argjson gateway_outage_non200 "$gateway_outage_non200" \
	--argjson storage_outage_retry_samples "$storage_outage_retry_samples" \
	--argjson post_wal_growth "$post_wal_growth" \
	--argjson accepted "$accepted_total" \
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
	--argjson lag_p95_post "$lag_p95_post" \
	--arg dedupe_status "$dedupe_status" \
	--argjson dedupe_events "$dedupe_events" \
	--argjson gateway_exit "$gateway_exit" \
	--argjson writer_exit "$writer_exit" \
	--arg reason "$reason_text" \
	--argjson checks "$checks_json" \
	--argjson events "$events_json" \
	'{
		story: "4.5",
		status: $status,
		t1_status: $t1_status,
		t1_note: "AC1 requires steady ingestion at 1M/s with ClickHouse down for 15 minutes on the T1 fleet; P-07 and the T1 baseline remain pending. This local run is a proxy, not G1 evidence.",
		scenario: $scenario,
		load: {rate: $rate, total_seconds: $total_seconds, pre_seconds: $pre_seconds,
		       outage_seconds: $outage_seconds, nats_outage_seconds: $nats_outage_seconds,
		       gateway_outage_seconds: $gateway_outage_seconds, settle_seconds: $settle_seconds,
		       post_seconds: $post_seconds},
		faults: {clickhouse_outages: $ch_outages, nats_outages: $nats_outages,
		         gateway_outages: $gateway_outages, gateway_restarts: $gateway_restarts},
		client: {confirmed_events: $sent, failed_requests: $client_failed, rejected_events: $client_rejected,
		         confirmed_logs: $sent_logs, confirmed_traces: $sent_traces, confirmed_metrics: $sent_metrics},
		gateway: {wal_published_events: $wal_published, wal_backpressure_requests: $wal_backpressure,
		          wal_publish_errors: $wal_publish_errors, exit: $gateway_exit,
		          min_readyz: $gateway_ready_min, last_readyz: $gateway_ready_last,
		          nats_outage_non_200_samples: $nats_outage_non200,
		          gateway_outage_non_200_samples: $gateway_outage_non200},
		wal: {peak_stream_messages: $peak_wal_messages, peak_stream_bytes: $peak_wal_bytes,
		      growth_messages: $wal_growth, storage_outage_retry_samples: $storage_outage_retry_samples,
		      post_phase_published_growth: $post_wal_growth},
		writer: {accepted_events: $accepted, written_events: $written, deduped_events: $deduped,
		         failed_events: $failed, skipped_events: $skipped, retries: $retries, exit: $writer_exit},
		signals: {
			logs: {accepted: $accepted_logs, written: $written_logs},
			traces: {accepted: $accepted_traces, written: $written_traces},
			metrics: {accepted: $accepted_metrics, written: $written_metrics}
		},
		clickhouse: {logs: $ch_logs, traces: $ch_traces},
		duplicates: {rows: $duplicate_rows, rate_percent: $duplicate_rate_pct, bound_percent: 1.0},
		victoriametrics: {samples: $vm_samples, series: $vm_series},
		lag_seconds: {p50: $lag_p50, p95: $lag_p95, p99: $lag_p99, post_phase_p95: $lag_p95_post},
		recovery_seconds: $recovery_seconds,
		replay: {status: $dedupe_status, deduped_events: $dedupe_events},
		reason: $reason,
		checks: $checks,
		events: $events
	}' >"$RESULTS_DIR/report.json"

cat >"$RESULTS_DIR/report.md" <<EOF
# Story 4.5 — chaos at the ingestion stack (local)

- **Status:** ${status}
- **Scenario:** ${SCENARIO}
- **T1 target:** ${t1_status} (steady 1M/s with ClickHouse down for 15 min; P-07 and the T1 baseline remain pending)
- **Offered rate:** ${RATE} events/s total across two tenants
- **Timeline:** pre ${PRE_S}s, ClickHouse outage ${OUTAGE_S}s, NATS outage ${NATS_OUTAGE_S}s, gateway outage ${GATEWAY_OUTAGE_S}s, settle ${SETTLE_S}s, post ${POST_S}s (total ${TOTAL_S}s)
- **Faults:** ClickHouse=${ch_outages}, NATS=${nats_outages}, gateway=${gateway_outages} (${gateway_restarts} restarts)
- **Replay check:** ${dedupe_status} (${dedupe_events} events deduplicated)

| Metric | Value |
|---|---:|
| Confirmed events (load generator) | ${sent} (logs ${sent_logs}, traces ${sent_traces}, metrics ${sent_metrics}) |
| Client failed / rejected requests | ${client_failed} / ${client_rejected} |
| Gateway WAL published events (all instances) | ${wal_published} |
| Gateway 429 wal_backpressure (all instances) | ${wal_backpressure} |
| Gateway WAL publish errors (all instances) | ${wal_publish_errors} |
| Peak WAL messages / bytes | ${peak_wal_messages} / ${peak_wal_bytes} |
| WAL growth during the outage | ${wal_growth} messages |
| Gateway /readyz min / last | ${gateway_ready_min} / ${gateway_ready_last} |
| Writer accepted | ${accepted_total} |
| Writer written | ${written_total} |
| Writer deduped | ${deduped_total} |
| Writer failed | ${failed_total} |
| Writer skipped | ${skipped_total} |
| Writer retries | ${retries_total} |
| ClickHouse logs | ${ch_logs} (unique ${ch_logs_unique}) |
| ClickHouse traces | ${ch_traces} (unique ${ch_traces_unique}) |
| Duplicate rows / rate | ${duplicate_rows} / ${duplicate_rate_pct}% (bound 1%) |
| Recovery (last fault recovered → drained) | ${recovery_seconds}s |
| VictoriaMetrics samples | ${vm_samples} (${vm_series} series) |
| Lag p50 / p95 / p99 (recent window) | ${lag_p50}s / ${lag_p95}s / ${lag_p99}s |
| Post-phase lag p95 (median of last samples) | ${lag_p95_post}s |
| Exit codes gateway/writer | ${gateway_exit} / ${writer_exit} |

${reason_text:+**Failure:** ${reason_text}}

## Checks

| Check | Status | Detail |
|---|---|---|
$(printf '%s\n' "${checks[@]}" | jq -r '"| \(.name) | \(.status) | \(.detail) |"')

The gateway acknowledges a request only after the JetStream publish. The writer
acknowledges a WAL message only after ClickHouse or VictoriaMetrics confirmed
the flush (or the message reached the DLQ). During the outage the writer keeps
the messages unacked in the WAL and retries with exponential backoff. Confirmed
events are therefore not lost; the cost is lag and WAL growth. This is a local
single-node proxy and does not prove the 1M/s/15 min T1 target.
EOF

cat "$RESULTS_DIR/report.md"
if [ "$status" != pass ]; then
	log "chaos: fail — ${reason_text}"
	exit 1
fi
log "chaos: pass"
