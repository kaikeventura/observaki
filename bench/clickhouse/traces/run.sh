#!/usr/bin/env bash
# Story 3.2 — ClickHouse traces storage spike.
#
# Starts a disposable ClickHouse 26.8 container, applies the embedded logs and
# traces migrations, loads a deterministic synthetic span dataset, measures
# trace-ID lookup, service+status+24h search, compression, the basic service
# map and log<->trace correlation, then publishes report.md/report.json.
#
# The default is the 1B-span local run. Use --rows 10000000 for a smoke test.
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
MIGRATIONS=(
	"$ROOT/internal/adapters/clickhouse/migrations/0001_otel_logs.sql"
	"$ROOT/internal/adapters/clickhouse/migrations/0002_otel_traces.sql"
)
TRACES_MIGRATION="$ROOT/internal/adapters/clickhouse/migrations/0002_otel_traces.sql"
LOAD_SQL="$ROOT/bench/clickhouse/traces/load.sql"
QUERY_DIR="$ROOT/bench/clickhouse/traces/queries"
CONFIG="$ROOT/bench/clickhouse/config/bench.xml"

ROWS=1000000000
CHUNK_ROWS=50000000
RUNS=20
BASE=${OBSERVAKI_CH_BASE:-$(date -u '+%Y-%m-%d %H:%M:%S')}
CONTAINER=${OBSERVAKI_CH_TRACES_CONTAINER:-observaki-ch32}
IMAGE=${OBSERVAKI_CH_IMAGE:-clickhouse/clickhouse-server:26.8}
DATA_DIR=${OBSERVAKI_CH_TRACES_DATA_DIR:-/tmp/opencode/observaki-ch32}
DATABASE=observaki_trace_bench
TENANT=tenant-0
PROJECT=project-0
SERVICE=checkout
STATUS=ERROR
MAP_WINDOW_MINUTES=60
TRACE_INDEX=""
MIN_FREE_GIB=15
REUSE=0
SKIP_LOAD=0
CLEANUP=0
RECREATE=0
OPTIMIZE=0
STOP_MERGES=0
RESULTS_DIR=""

usage() {
	cat <<'EOF'
Usage: bench/clickhouse/traces/run.sh [options]

Options:
  --rows N                 total synthetic spans (default: 1000000000; must be a multiple of 10)
  --chunk-rows N           spans per INSERT (default: 50000000; must be a multiple of 10)
  --runs N                 measured query repetitions, plus one warm-up (default: 20)
  --base "YYYY-MM-DD HH:MM:SS"
                           end of the 24h event-time window (default: now UTC)
  --trace-index N          global trace index used for the trace-ID lookup
                           (default: rows / 10 / 2)
  --map-window-minutes N   service-map window ending at --base (default: 60)
  --container NAME         Docker container name (default: observaki-ch32)
  --image IMAGE            ClickHouse image (default: clickhouse/clickhouse-server:26.8)
  --data-dir PATH          host directory for ClickHouse data
                           (default: /tmp/opencode/observaki-ch32)
  --results-dir PATH       report destination
  --reuse                  reuse an existing table with exactly --rows spans
  --skip-load              measure the existing table; do not load
  --optimize               run OPTIMIZE TABLE ... FINAL before measuring
                           (only for small datasets)
  --stop-merges            stop background merges for the traces table during
                           the load; required for the local 1B run because the
                           merge overhead otherwise exceeds the free disk
  --recreate               remove the container before starting
  --cleanup                remove the container after writing the report
  --help                   show this help

The script needs docker, jq, awk and GNU date. It preserves the data directory
so --reuse can be used for a second measurement.
EOF
}

log() {
	local message
	message="[$(date -u '+%H:%M:%S')] $*"
	if [ -n "${LOG:-}" ]; then
		printf '%s\n' "$message" | tee -a "$LOG" >&2
	else
		printf '%s\n' "$message" >&2
	fi
}

die() {
	log "error: $*"
	exit 1
}

require() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

while [ $# -gt 0 ]; do
	case "$1" in
	--rows) ROWS="$2"; shift 2 ;;
	--chunk-rows) CHUNK_ROWS="$2"; shift 2 ;;
	--runs) RUNS="$2"; shift 2 ;;
	--base) BASE="$2"; shift 2 ;;
	--trace-index) TRACE_INDEX="$2"; shift 2 ;;
	--map-window-minutes) MAP_WINDOW_MINUTES="$2"; shift 2 ;;
	--container) CONTAINER="$2"; shift 2 ;;
	--image) IMAGE="$2"; shift 2 ;;
	--data-dir) DATA_DIR="$2"; shift 2 ;;
	--results-dir) RESULTS_DIR="$2"; shift 2 ;;
	--reuse) REUSE=1; shift ;;
	--skip-load) SKIP_LOAD=1; shift ;;
	--optimize) OPTIMIZE=1; shift ;;
	--stop-merges) STOP_MERGES=1; shift ;;
	--recreate) RECREATE=1; shift ;;
	--cleanup) CLEANUP=1; shift ;;
	--help | -h) usage; exit 0 ;;
	*) die "unknown argument: $1 (use --help)" ;;
	esac
done

for value in "$ROWS" "$CHUNK_ROWS" "$RUNS" "$MAP_WINDOW_MINUTES"; do
	[[ "$value" =~ ^[1-9][0-9]*$ ]] || die "expected a positive integer, got $value"
done
if [ $((ROWS % 10)) -ne 0 ] || [ $((CHUNK_ROWS % 10)) -ne 0 ]; then
	die "--rows and --chunk-rows must be multiples of 10 (one trace = ten spans)"
fi
if [ "$ROWS" -lt 10 ] || [ "$CHUNK_ROWS" -lt 10 ]; then
	die "--rows and --chunk-rows must be at least 10"
fi
if [ "$MAP_WINDOW_MINUTES" -gt 1440 ]; then
	die "--map-window-minutes must be <= 1440"
fi

require docker
require jq
require awk
require sha256sum
require date

BASE=$(date -u -d "$BASE" '+%Y-%m-%d %H:%M:%S') || die "invalid --base value"
BASE_EPOCH=$(date -u -d "$BASE" +%s)
START=$(date -u -d "@$((BASE_EPOCH - 86400))" '+%Y-%m-%d %H:%M:%S')
END="$BASE"
MAP_START=$(date -u -d "@$((BASE_EPOCH - MAP_WINDOW_MINUTES * 60))" '+%Y-%m-%d %H:%M:%S')
MAP_END="$BASE"

TOTAL_TRACES=$((ROWS / 10))
if [ -z "$TRACE_INDEX" ]; then
	TRACE_INDEX=$((TOTAL_TRACES / 2))
fi
if [ "$TRACE_INDEX" -lt 0 ] || [ "$TRACE_INDEX" -ge "$TOTAL_TRACES" ]; then
	die "--trace-index must be in [0, $((TOTAL_TRACES - 1))]"
fi
TRACE_TENANT="tenant-$((TRACE_INDEX % 2))"
TRACE_PROJECT="project-$((TRACE_INDEX % 4))"

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/clickhouse/traces/results/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$RESULTS_DIR"
LOG="$RESULTS_DIR/run.log"
if [ "$REUSE" = 1 ] || [ "$SKIP_LOAD" = 1 ]; then
	touch "$LOG"
else
	: >"$LOG"
fi

log "Story 3.2 ClickHouse traces spike"
log "rows=$ROWS traces=$TOTAL_TRACES chunk_rows=$CHUNK_ROWS runs=$RUNS base=$BASE"
log "trace_index=$TRACE_INDEX tenant=$TRACE_TENANT project=$TRACE_PROJECT"
log "container=$CONTAINER image=$IMAGE data_dir=$DATA_DIR results=$RESULTS_DIR"

ch() {
	docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" "$@"
}

ch_stdin() {
	docker exec -i "$CONTAINER" clickhouse-client --database "$DATABASE" "$@"
}

ch_root() {
	docker exec "$CONTAINER" clickhouse-client "$@"
}

wait_ready() {
	for _ in $(seq 1 60); do
		if docker exec "$CONTAINER" clickhouse-client --query 'SELECT 1' >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	die "ClickHouse did not become ready in 60s"
}

ensure_container() {
	if [ "$RECREATE" = 1 ]; then
		docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
	fi
	if docker inspect "$CONTAINER" >/dev/null 2>&1; then
		if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER")" != "true" ]; then
			docker start "$CONTAINER" >/dev/null
		fi
	else
		mkdir -p "$DATA_DIR"
		chmod 777 "$DATA_DIR" 2>/dev/null || true
		docker run -d --name "$CONTAINER" \
			-v "$DATA_DIR:/var/lib/clickhouse" \
			-v "$CONFIG:/etc/clickhouse-server/config.d/bench.xml:ro" \
			--ulimit nofile=262144:262144 \
			"$IMAGE" >/dev/null
	fi
	wait_ready
}

create_database() {
	ch_root --query "CREATE DATABASE IF NOT EXISTS $DATABASE"
}

apply_schema() {
	local migration
	for migration in "${MIGRATIONS[@]}"; do
		docker exec -i "$CONTAINER" clickhouse-client --database "$DATABASE" --multiquery <"$migration"
	done
}

stop_merges() {
	if [ "$STOP_MERGES" != 1 ]; then
		return
	fi
	log "stopping background merges for otel_traces (local 1B disk bound)"
	ch --query "SYSTEM STOP MERGES $DATABASE.otel_traces"
	ch --query "ALTER TABLE otel_traces MODIFY SETTING parts_to_delay_insert = 5000, parts_to_throw_insert = 6000"
}

table_count() {
	ch --query "SELECT count() FROM otel_traces"
}

free_gib() {
	df -Pk "$DATA_DIR" | awk 'NR == 2 { printf "%.1f", $4 / 1024 / 1024 }'
}

load() {
	local offset=0 count started elapsed free
	if [ "$SKIP_LOAD" = 1 ]; then
		log "skipping load; measuring the existing table"
		return
	fi
	if [ "$REUSE" = 1 ]; then
		local existing
		existing=$(table_count)
		if [ "$existing" = "$ROWS" ]; then
			log "reusing $existing spans"
			return
		fi
		die "--reuse requested but the table has $existing spans, want $ROWS"
	fi

	log "creating a fresh traces table"
	ch --query "DROP TABLE IF EXISTS otel_traces"
	apply_schema
	stop_merges

	while [ "$offset" -lt "$ROWS" ]; do
		count=$((ROWS - offset))
		if [ "$count" -gt "$CHUNK_ROWS" ]; then
			count=$CHUNK_ROWS
		fi
		started=$(date +%s)
		docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" \
			--param_base="$BASE" --param_offset="$offset" --param_count="$count" --param_rows="$ROWS" \
			--query "$(cat "$LOAD_SQL")"
		elapsed=$(( $(date +%s) - started ))
		offset=$((offset + count))
		free=$(free_gib)
		log "loaded $offset/$ROWS spans (chunk $count in ${elapsed}s, free ${free} GiB)"
		if awk -v f="$free" -v m="$MIN_FREE_GIB" 'BEGIN { exit !(f < m) }'; then
			die "free space ${free} GiB is below the ${MIN_FREE_GIB} GiB guard; load stopped at $offset spans"
		fi
	done
}

optimize_table() {
	if [ "$OPTIMIZE" != 1 ]; then
		return
	fi
	# OPTIMIZE needs the merge pool even when the load stopped merges to bound
	# disk use. Starting merges again is idempotent and only used for small
	# smoke datasets.
	ch --query "SYSTEM START MERGES $DATABASE.otel_traces" >/dev/null 2>&1 || true
	log "OPTIMIZE TABLE otel_traces FINAL (small-dataset option)"
	ch --query "OPTIMIZE TABLE otel_traces FINAL"
}

collect_storage() {
	local rows parts compressed uncompressed ratio bytes_per_row
	rows=$(table_count)
	parts=$(ch --query "SELECT count() FROM system.parts WHERE database = '$DATABASE' AND table = 'otel_traces' AND active")
	compressed=$(ch --query "SELECT sum(data_compressed_bytes) FROM system.parts WHERE database = '$DATABASE' AND table = 'otel_traces' AND active")
	uncompressed=$(ch --query "SELECT sum(data_uncompressed_bytes) FROM system.parts WHERE database = '$DATABASE' AND table = 'otel_traces' AND active")
	ratio=$(awk -v u="$uncompressed" -v c="$compressed" 'BEGIN { printf "%.2f", u / c }')
	bytes_per_row=$(awk -v c="$compressed" -v r="$rows" 'BEGIN { printf "%.1f", c / r }')
	log "storage: spans=$rows parts=$parts compressed=$compressed uncompressed=$uncompressed ratio=${ratio}x bytes_per_row=$bytes_per_row"
	printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$rows" "$parts" "$compressed" "$uncompressed" "$ratio" "$bytes_per_row" >"$RESULTS_DIR/storage.tsv"
}

run_trace_query() {
	docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" --format JSON \
		--param_tenant="$TRACE_TENANT" --param_project="$TRACE_PROJECT" --param_trace_id="$TRACE_ID" \
		--query "$(cat "$QUERY_DIR/q_trace_by_id.sql")"
}

run_service_query() {
	docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" --format JSON \
		--param_tenant="$TENANT" --param_project="$PROJECT" --param_service="$SERVICE" \
		--param_status="$STATUS" --param_start="$START" --param_end="$END" \
		--query "$(cat "$QUERY_DIR/q_service_status_24h.sql")"
}

measure() {
	local out=$1
	shift
	local i json returned
	log "query warm-up (not counted): $out"
	"$@" >/dev/null

	: >"$out"
	for i in $(seq 1 "$RUNS"); do
		json=$("$@")
		returned=$(printf '%s' "$json" | jq '.data | length')
		printf '%s' "$json" | jq -c --argjson run "$i" --argjson returned "$returned" \
			'{run: $run, elapsed_ms: (.statistics.elapsed * 1000), rows_read: .statistics.rows_read, bytes_read: .statistics.bytes_read, returned: $returned}' >>"$out"
	done
	log "measured $RUNS runs into $out"
}

percentile() {
	local file=$1 p=$2
	sort -n "$file" | awk -v p="$p" '{ a[NR] = $1 } END {
		idx = int((p / 100) * NR + 0.999)
		if (idx < 1) idx = 1
		if (idx > NR) idx = NR
		printf "%.3f", a[idx]
	}'
}

resolve_ids() {
	local ids
	ids=$(ch --format TSV --param_trace_index="$TRACE_INDEX" --query "$(cat "$QUERY_DIR/q_ids_for_index.sql")")
	TRACE_ID=$(printf '%s' "$ids" | cut -f1)
	ROOT_SPAN_ID=$(printf '%s' "$ids" | cut -f2)
	if [ -z "$TRACE_ID" ] || [ -z "$ROOT_SPAN_ID" ]; then
		die "could not resolve the trace_id/root span_id for trace index $TRACE_INDEX"
	fi
	log "trace lookup target: trace_index=$TRACE_INDEX trace_id=$TRACE_ID root_span_id=$ROOT_SPAN_ID"
}

service_map() {
	local nodes_json edges_json nodes edges nodes_ms edges_ms
	log "service map: nodes and edges in the last ${MAP_WINDOW_MINUTES}m"
	nodes_json=$(docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" --format JSON \
		--param_tenant="$TENANT" --param_project="$PROJECT" --param_start="$MAP_START" --param_end="$MAP_END" \
		--query "$(cat "$QUERY_DIR/q_service_map_nodes.sql")")
	edges_json=$(docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" --format JSON \
		--param_tenant="$TENANT" --param_project="$PROJECT" --param_start="$MAP_START" --param_end="$MAP_END" \
		--query "$(cat "$QUERY_DIR/q_service_map_edges.sql")")
	printf '%s' "$nodes_json" >"$RESULTS_DIR/service_map_nodes.json"
	printf '%s' "$edges_json" >"$RESULTS_DIR/service_map_edges.json"
	nodes=$(printf '%s' "$nodes_json" | jq '.data | length')
	edges=$(printf '%s' "$edges_json" | jq '.data | length')
	nodes_ms=$(printf '%s' "$nodes_json" | jq '.statistics.elapsed * 1000')
	edges_ms=$(printf '%s' "$edges_json" | jq '.statistics.elapsed * 1000')
	log "service map: nodes=$nodes edges=$edges nodes_ms=$nodes_ms edges_ms=$edges_ms"
}

correlation() {
	local log_row log_trace_id log_span_id trace_result trace_spans root_found logs_result logs_count log_event
	local log_to_trace=false trace_to_log=false

	log "correlation: insert one log for trace $TRACE_ID"
	ch --query "TRUNCATE TABLE otel_logs"
	ch --param_trace_id="$TRACE_ID" --param_span_id="$ROOT_SPAN_ID" --query "INSERT INTO otel_logs (tenant_id, project_id, timestamp, ingested_at, observed_timestamp, severity, severity_number, service_name, body, attributes, resource_attributes, trace_id, span_id, event_id, retention_days) VALUES ('$TRACE_TENANT', '$TRACE_PROJECT', now64(9), now64(9), now64(9), 'ERROR', 17, 'checkout', 'correlated log for trace lookup', map('corr', 'log-to-trace'), map('service.name', 'checkout'), {trace_id:String}, {span_id:String}, 'corr-log-1', 15)"

	log_row=$(ch --format JSON --param_tenant="$TRACE_TENANT" --param_project="$TRACE_PROJECT" --param_event_id='corr-log-1' --query "$(cat "$QUERY_DIR/q_log_by_event_id.sql")")
	log_trace_id=$(printf '%s' "$log_row" | jq -r '.data[0].trace_id // empty')
	log_span_id=$(printf '%s' "$log_row" | jq -r '.data[0].span_id // empty')
	if [ -n "$log_trace_id" ] && [ -n "$log_span_id" ]; then
		trace_result=$(docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" --format JSON \
			--param_tenant="$TRACE_TENANT" --param_project="$TRACE_PROJECT" --param_trace_id="$log_trace_id" \
			--query "$(cat "$QUERY_DIR/q_trace_by_id.sql")")
		trace_spans=$(printf '%s' "$trace_result" | jq '.data | length')
		root_found=$(printf '%s' "$trace_result" | jq --arg span "$log_span_id" '[.data[] | select(.span_id == $span)] | length')
		if [ "$log_trace_id" = "$TRACE_ID" ] && [ "$trace_spans" = "10" ] && [ "$root_found" -ge 1 ]; then
			log_to_trace=true
		fi
	else
		trace_spans=0
		root_found=0
	fi

	logs_result=$(ch --format JSON --param_tenant="$TRACE_TENANT" --param_project="$TRACE_PROJECT" --param_trace_id="$TRACE_ID" --query "$(cat "$QUERY_DIR/q_logs_by_trace_id.sql")")
	logs_count=$(printf '%s' "$logs_result" | jq '.data | length')
	log_event=$(printf '%s' "$logs_result" | jq -r '.data[0].event_id // empty')
	if [ "$logs_count" -ge 1 ] && [ "$log_event" = "corr-log-1" ]; then
		trace_to_log=true
	fi

	log "correlation: log_to_trace=$log_to_trace trace_to_log=$trace_to_log trace_spans=$trace_spans logs=$logs_count"
	jq -n \
		--argjson trace_index "$TRACE_INDEX" \
		--arg trace_id "$TRACE_ID" \
		--arg root_span_id "$ROOT_SPAN_ID" \
		--argjson log_to_trace "$log_to_trace" \
		--argjson trace_to_log "$trace_to_log" \
		--argjson trace_spans "$trace_spans" \
		--argjson logs "$logs_count" \
		'{trace_index: $trace_index, trace_id: $trace_id, root_span_id: $root_span_id, log_to_trace: $log_to_trace, trace_to_log: $trace_to_log, trace_spans: $trace_spans, logs: $logs}' \
		>"$RESULTS_DIR/correlation.json"
}

red_check() {
	local red_tables mv_tables red_in_clickhouse=false
	red_tables=$(ch --query "SELECT count() FROM system.tables WHERE database = '$DATABASE' AND (name LIKE '%span_metrics%' OR name LIKE '%red%')")
	mv_tables=$(ch --query "SELECT count() FROM system.tables WHERE database = '$DATABASE' AND engine = 'MaterializedView'")
	if [ "$red_tables" != "0" ] || [ "$mv_tables" != "0" ]; then
		red_in_clickhouse=true
	fi
	log "AD-21 check: span_metrics/RED tables=$red_tables materialized_views=$mv_tables red_in_clickhouse=$red_in_clickhouse"
	jq -n --argjson red_tables "$red_tables" --argjson mv_tables "$mv_tables" --argjson red_in_clickhouse "$red_in_clickhouse" \
		'{span_metrics_tables: $red_tables, materialized_views: $mv_tables, red_in_clickhouse: $red_in_clickhouse}' \
		>"$RESULTS_DIR/red_check.json"
}

write_report() {
	local rows parts compressed uncompressed ratio bytes_per_row
	local trace_p50 trace_p95 trace_p99 trace_rows_read trace_bytes_read trace_returned
	local service_p50 service_p95 service_p99 service_rows_read service_bytes_read service_returned
	local compression_pass trace_pass service_pass trace_rows_pass service_rows_pass
	local map_pass correlation_pass red_pass status verdict_reason failures
	local schema_sha commit dirty ch_version generated_at hostname cpus memory_bytes disk_total disk_free
	local limitations reproduction merges_stopped optimized

	IFS=$'\t' read -r rows parts compressed uncompressed ratio bytes_per_row <"$RESULTS_DIR/storage.tsv"

	jq -r '.elapsed_ms' "$RESULTS_DIR/trace_runs.jsonl" >"$RESULTS_DIR/trace_elapsed_ms.txt"
	trace_p50=$(percentile "$RESULTS_DIR/trace_elapsed_ms.txt" 50)
	trace_p95=$(percentile "$RESULTS_DIR/trace_elapsed_ms.txt" 95)
	trace_p99=$(percentile "$RESULTS_DIR/trace_elapsed_ms.txt" 99)
	trace_rows_read=$(jq -s 'map(.rows_read) | add / length' "$RESULTS_DIR/trace_runs.jsonl")
	trace_bytes_read=$(jq -s 'map(.bytes_read) | add / length' "$RESULTS_DIR/trace_runs.jsonl")
	trace_returned=$(jq -s 'map(.returned) | add / length' "$RESULTS_DIR/trace_runs.jsonl")

	jq -r '.elapsed_ms' "$RESULTS_DIR/service_runs.jsonl" >"$RESULTS_DIR/service_elapsed_ms.txt"
	service_p50=$(percentile "$RESULTS_DIR/service_elapsed_ms.txt" 50)
	service_p95=$(percentile "$RESULTS_DIR/service_elapsed_ms.txt" 95)
	service_p99=$(percentile "$RESULTS_DIR/service_elapsed_ms.txt" 99)
	service_rows_read=$(jq -s 'map(.rows_read) | add / length' "$RESULTS_DIR/service_runs.jsonl")
	service_bytes_read=$(jq -s 'map(.bytes_read) | add / length' "$RESULTS_DIR/service_runs.jsonl")
	service_returned=$(jq -s 'map(.returned) | add / length' "$RESULTS_DIR/service_runs.jsonl")

	compression_pass=false
	if awk -v r="$ratio" 'BEGIN { exit !(r >= 5) }'; then compression_pass=true; fi
	trace_pass=false
	if awk -v p="$trace_p95" 'BEGIN { exit !(p < 1000) }'; then trace_pass=true; fi
	service_pass=false
	if awk -v p="$service_p95" 'BEGIN { exit !(p < 2000) }'; then service_pass=true; fi
	trace_rows_pass=false
	if awk -v r="$trace_returned" 'BEGIN { exit !(r > 0) }'; then trace_rows_pass=true; fi
	service_rows_pass=false
	if awk -v r="$service_returned" 'BEGIN { exit !(r > 0) }'; then service_rows_pass=true; fi
	map_pass=false
	if [ "$(jq '.data | length' "$RESULTS_DIR/service_map_nodes.json")" -ge 1 ] && [ "$(jq '.data | length' "$RESULTS_DIR/service_map_edges.json")" -ge 1 ]; then
		map_pass=true
	fi
	correlation_pass=false
	if [ "$(jq -r '.log_to_trace' "$RESULTS_DIR/correlation.json")" = "true" ] && [ "$(jq -r '.trace_to_log' "$RESULTS_DIR/correlation.json")" = "true" ]; then
		correlation_pass=true
	fi
	red_pass=false
	if [ "$(jq -r '.red_in_clickhouse' "$RESULTS_DIR/red_check.json")" = "false" ]; then
		red_pass=true
	fi

	failures=""
	if [ "$compression_pass" != "true" ]; then failures="$failures compression"; fi
	if [ "$trace_pass" != "true" ]; then failures="$failures trace_p95"; fi
	if [ "$service_pass" != "true" ]; then failures="$failures service_p95"; fi
	if [ "$trace_rows_pass" != "true" ]; then failures="$failures trace_rows"; fi
	if [ "$service_rows_pass" != "true" ]; then failures="$failures service_rows"; fi
	if [ "$map_pass" != "true" ]; then failures="$failures service_map"; fi
	if [ "$correlation_pass" != "true" ]; then failures="$failures correlation"; fi
	if [ "$red_pass" != "true" ]; then failures="$failures red_in_clickhouse"; fi
	if [ -z "$failures" ]; then
		status=pass
		verdict_reason="compression >= 5x, trace lookup p95 < 1000 ms, service+status p95 < 2000 ms, service map and correlation pass"
	else
		status=fail
		verdict_reason="missed:${failures}"
	fi

	schema_sha=$(sha256sum "$TRACES_MIGRATION" | awk '{ print $1 }')
	commit=$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)
	if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no 2>/dev/null)" ]; then dirty=true; else dirty=false; fi
	ch_version=$(ch --query "SELECT version()")
	generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	hostname=$(hostname)
	cpus=$(nproc)
	memory_bytes=$(awk '/MemTotal/ { printf "%.0f", $2 * 1024 }' /proc/meminfo)
	disk_total=$(df -B1 --output=size "$DATA_DIR" | tail -1 | tr -d ' ')
	disk_free=$(df -B1 --output=avail "$DATA_DIR" | tail -1 | tr -d ' ')
	merges_stopped=false
	if [ "$STOP_MERGES" = 1 ]; then merges_stopped=true; fi
	optimized=false
	if [ "$OPTIMIZE" = 1 ]; then optimized=true; fi
	limitations='[
		"single-node local ClickHouse, not the T1 cluster (Story 2.4 profile requires P-07)",
		"the 1B-span dataset is generated with ClickHouse SQL, not through the OTLP gateway (Story 4.1/3.3)",
		"the trace-ID lookup uses one known trace; the OS page cache is warm after the warm-up run",
		"the service map is derived from raw spans with a 1% deterministic trace sample; production RED and the production service map come from the gateway and VictoriaMetrics (AD-21)",
		"the local host is smaller than the query-1b scenario reference (16 vCPU/64 GB); the report records the real hardware",
		"no replication, NATS, Query Service, row policies or chaos; those are Stories 3.3/3.5/4.5"
	]'
	if [ "$STOP_MERGES" = 1 ]; then
		limitations=$(printf '%s' "$limitations" | jq '. + ["background merges are stopped for the traces table during the local load to bound disk use; the table is measured with unmerged parts"]')
	fi
	if [ "$OPTIMIZE" = 1 ]; then
		limitations=$(printf '%s' "$limitations" | jq '. + ["OPTIMIZE TABLE FINAL was run before measuring; this is only for small smoke datasets"]')
	fi
	reproduction="./bench/clickhouse/traces/run.sh --rows $ROWS --chunk-rows $CHUNK_ROWS --runs $RUNS --trace-index $TRACE_INDEX --map-window-minutes $MAP_WINDOW_MINUTES"
	if [ "$STOP_MERGES" = 1 ]; then reproduction="$reproduction --stop-merges"; fi
	if [ "$OPTIMIZE" = 1 ]; then reproduction="$reproduction --optimize"; fi
	if [ "$REUSE" = 1 ]; then reproduction="$reproduction --reuse"; fi
	if [ "$SKIP_LOAD" = 1 ]; then reproduction="$reproduction --skip-load"; fi
	reproduction="$reproduction --base \"$BASE\""

	jq -n \
		--arg generated_at "$generated_at" \
		--arg commit "$commit" \
		--argjson dirty "$dirty" \
		--arg clickhouse_version "$ch_version" \
		--arg image "$IMAGE" \
		--arg container "$CONTAINER" \
		--arg hostname "$hostname" \
		--argjson cpus "$cpus" \
		--argjson memory_bytes "$memory_bytes" \
		--argjson disk_total_bytes "$disk_total" \
		--argjson disk_free_bytes "$disk_free" \
		--arg data_dir "$DATA_DIR" \
		--arg base "$BASE" \
		--arg start "$START" \
		--arg end "$END" \
		--arg map_start "$MAP_START" \
		--arg map_end "$MAP_END" \
		--argjson map_window_minutes "$MAP_WINDOW_MINUTES" \
		--argjson rows "$rows" \
		--argjson traces "$((rows / 10))" \
		--argjson chunk_rows "$CHUNK_ROWS" \
		--argjson parts "$parts" \
		--argjson merges_stopped "$merges_stopped" \
		--argjson optimized "$optimized" \
		--argjson compressed "$compressed" \
		--argjson uncompressed "$uncompressed" \
		--argjson ratio "$ratio" \
		--argjson bytes_per_row "$bytes_per_row" \
		--argjson trace_p50 "$trace_p50" \
		--argjson trace_p95 "$trace_p95" \
		--argjson trace_p99 "$trace_p99" \
		--argjson trace_rows_read "$trace_rows_read" \
		--argjson trace_bytes_read "$trace_bytes_read" \
		--argjson trace_returned "$trace_returned" \
		--argjson service_p50 "$service_p50" \
		--argjson service_p95 "$service_p95" \
		--argjson service_p99 "$service_p99" \
		--argjson service_rows_read "$service_rows_read" \
		--argjson service_bytes_read "$service_bytes_read" \
		--argjson service_returned "$service_returned" \
		--argjson runs "$RUNS" \
		--argjson compression_pass "$compression_pass" \
		--argjson trace_pass "$trace_pass" \
		--argjson service_pass "$service_pass" \
		--argjson trace_rows_pass "$trace_rows_pass" \
		--argjson service_rows_pass "$service_rows_pass" \
		--argjson map_pass "$map_pass" \
		--argjson correlation_pass "$correlation_pass" \
		--argjson red_pass "$red_pass" \
		--arg status "$status" \
		--arg verdict_reason "$verdict_reason" \
		--arg schema_sha "$schema_sha" \
		--arg reproduction "$reproduction" \
		--arg tenant "$TENANT" \
		--arg project "$PROJECT" \
		--arg service "$SERVICE" \
		--arg status_filter "$STATUS" \
		--arg trace_tenant "$TRACE_TENANT" \
		--arg trace_project "$TRACE_PROJECT" \
		--arg trace_id "$TRACE_ID" \
		--arg root_span_id "$ROOT_SPAN_ID" \
		--argjson limitations "$limitations" \
		--slurpfile map_nodes "$RESULTS_DIR/service_map_nodes.json" \
		--slurpfile map_edges "$RESULTS_DIR/service_map_edges.json" \
		--slurpfile correlation "$RESULTS_DIR/correlation.json" \
		--slurpfile red_check "$RESULTS_DIR/red_check.json" \
		'{
			schema_version: 1,
			generated_at: $generated_at,
			commit: $commit,
			dirty: $dirty,
			clickhouse: {version: $clickhouse_version, image: $image, container: $container},
			host: {hostname: $hostname, cpus: $cpus, memory_bytes: $memory_bytes, disk_total_bytes: $disk_total_bytes, disk_free_bytes: $disk_free_bytes, data_dir: $data_dir},
			schema: {migration: "0002_otel_traces.sql", sha256: $schema_sha, partition_by: "toDate(ingested_at)", order_by: "(tenant_id, project_id, service_name, status_code, timestamp)", ttl: "ingested_at + INTERVAL retention_days DAY", indexes: ["idx_trace bloom_filter", "idx_span bloom_filter", "idx_parent bloom_filter", "idx_service set(1000)"]},
			dataset: {rows: $rows, traces: $traces, chunk_rows: $chunk_rows, chunks: (($rows / $chunk_rows) | ceil), base: $base, window_hours: 24, services: 9, operations: 10, tenants: 2, projects: 4, status_mix: "97% OK / 3% ERROR on non-root spans, 99% OK / 1% ERROR on roots", events_percent: 10, links_percent: 5, retention_days: [7, 15], ids: "CityHash64, random-looking and deterministic"},
			storage: {parts: $parts, merges_stopped: $merges_stopped, optimized: $optimized, compressed_bytes: $compressed, uncompressed_bytes: $uncompressed, ratio: $ratio, compressed_bytes_per_row: $bytes_per_row},
			queries: {
				trace_by_id: {id: "trace_by_id", description: "Full trace by trace_id", runs: $runs, p50_ms: $trace_p50, p95_ms: $trace_p95, p99_ms: $trace_p99, rows_read_mean: $trace_rows_read, bytes_read_mean: $trace_bytes_read, returned_mean: $trace_returned, target_ms: 1000, tenant: $trace_tenant, project: $trace_project, trace_id: $trace_id},
				service_status_24h: {id: "service_status_24h", description: "Spans by service, status and 24h window", runs: $runs, p50_ms: $service_p50, p95_ms: $service_p95, p99_ms: $service_p99, rows_read_mean: $service_rows_read, bytes_read_mean: $service_bytes_read, returned_mean: $service_returned, target_ms: 2000, tenant: $tenant, project: $project, service: $service, status: $status_filter, start: $start, end: $end}
			},
			service_map: {window_minutes: $map_window_minutes, start: $map_start, end: $map_end, sample_percent: 1, nodes: ($map_nodes[0].data | length), edges: ($map_edges[0].data | length), nodes_elapsed_ms: ($map_nodes[0].statistics.elapsed * 1000), edges_elapsed_ms: ($map_edges[0].statistics.elapsed * 1000), nodes_data: $map_nodes[0].data, edges_data: $map_edges[0].data},
			correlation: $correlation[0],
			red_check: $red_check[0],
			verdict: {status: $status, reason: $verdict_reason, criteria: {compression_ge_5x: $compression_pass, trace_lookup_p95_lt_1s: $trace_pass, service_query_p95_lt_2s: $service_pass, trace_lookup_returned_spans: $trace_rows_pass, service_query_returned_spans: $service_rows_pass, service_map_nodes_and_edges: $map_pass, correlation_both_directions: $correlation_pass, red_not_in_clickhouse: $red_pass}},
			reproduction: $reproduction,
			limitations: $limitations
		}' >"$RESULTS_DIR/report.json"

	cat >"$RESULTS_DIR/report.md" <<EOF
# Story 3.2 — ClickHouse traces storage spike

**Verdict: ${status^^}** — ${verdict_reason}.

This is a single-node local spike, not the T1 cluster and not Gate G1 evidence.
The report records the exact commit, schema hash, hardware and limitations.

## Environment

| Field | Value |
|---|---|
| Commit | \`${commit}\` (dirty: ${dirty}) |
| Host | ${hostname}, ${cpus} CPUs, $(awk -v b="$memory_bytes" 'BEGIN { printf "%.1f GiB", b / 1073741824 }') RAM |
| Disk | $(awk -v t="$disk_total" -v f="$disk_free" 'BEGIN { printf "%.1f GiB total, %.1f GiB free", t / 1073741824, f / 1073741824 }') |
| ClickHouse | ${ch_version} (\`${IMAGE}\`) |
| Schema | \`0002_otel_traces.sql\` sha256 \`${schema_sha}\` |

## Dataset

| Field | Value |
|---|---:|
| Spans | ${rows} |
| Traces | $((rows / 10)) (10 spans each) |
| Chunks | $(( (ROWS + CHUNK_ROWS - 1) / CHUNK_ROWS )) × ${CHUNK_ROWS} spans |
| Event-time window | 24 h ending ${BASE} |
| Dimensions | 2 tenants × 4 projects × 9 services × 10 operations |
| Status mix | 97% OK / 3% ERROR on non-root spans; 99% OK / 1% ERROR on roots |
| Events / links | 10% of spans have one event; 5% have one link |
| Retention | 7 and 15 days per row |
| IDs | CityHash64, random-looking and deterministic |

## Compression

| Metric | Value |
|---|---:|
| Compressed | $(awk -v b="$compressed" 'BEGIN { printf "%.2f GiB", b / 1073741824 }') |
| Uncompressed | $(awk -v b="$uncompressed" 'BEGIN { printf "%.2f GiB", b / 1073741824 }') |
| Ratio | **${ratio}x** (target ≥ 5x) |
| Compressed bytes/span | ${bytes_per_row} B |
| Active parts | ${parts} |

## Trace lookup — q_trace_by_id

\`\`\`sql
$(cat "$QUERY_DIR/q_trace_by_id.sql")
\`\`\`

| Metric | Value |
|---|---:|
| Runs (after warm-up) | ${RUNS} |
| p50 | ${trace_p50} ms |
| p95 | ${trace_p95} ms (target < 1000 ms) |
| p99 | ${trace_p99} ms |
| Rows read (mean) | ${trace_rows_read} |
| Bytes read (mean) | ${trace_bytes_read} |
| Spans returned (mean) | ${trace_returned} |
| Trace | \`${TRACE_ID}\` (${TRACE_TENANT}/${TRACE_PROJECT}) |

## Service + status + 24h — q_service_status_24h

\`\`\`sql
$(cat "$QUERY_DIR/q_service_status_24h.sql")
\`\`\`

| Metric | Value |
|---|---:|
| Runs (after warm-up) | ${RUNS} |
| p50 | ${service_p50} ms |
| p95 | ${service_p95} ms (target < 2000 ms) |
| p99 | ${service_p99} ms |
| Rows read (mean) | ${service_rows_read} |
| Bytes read (mean) | ${service_bytes_read} |
| Spans returned (mean) | ${service_returned} |
| Filter | \`${SERVICE}\` + \`${STATUS}\` in the 24h window ending ${BASE} |

## Service map

Window: ${MAP_WINDOW_MINUTES} minutes ending ${BASE}. Edges use a 1%
deterministic trace sample and scale the counts by 100; the report labels them
as estimates. The map is derived from raw spans for spike validation only;
production RED and the production service map come from the gateway and
VictoriaMetrics (AD-21).

### Nodes

| Service | Spans | Errors | Avg ms | p95 ms |
|---|---:|---:|---:|---:|
$(jq -r '.data[] | "| \(.service_name) | \(.spans) | \(.errors) | \(.avg_duration_ms) | \(.p95_duration_ms) |"' "$RESULTS_DIR/service_map_nodes.json")

### Edges (estimated)

| Caller | Callee | Spans | Errors |
|---|---|---:|---:|
$(jq -r '.data[] | "| \(.caller) | \(.callee) | \(.spans_estimated) | \(.errors_estimated) |"' "$RESULTS_DIR/service_map_edges.json")

## Correlation

| Direction | Result |
|---|---|
| Log → trace | $(jq -r '.log_to_trace' "$RESULTS_DIR/correlation.json") (spans: $(jq -r '.trace_spans' "$RESULTS_DIR/correlation.json")) |
| Trace → log | $(jq -r '.trace_to_log' "$RESULTS_DIR/correlation.json") (logs: $(jq -r '.logs' "$RESULTS_DIR/correlation.json")) |

The log row stores the trace's \`trace_id\` and the root \`span_id\`; the trace
query returns all ten spans, including that root span.

## AD-21 check

\`\`\`json
$(jq -c . "$RESULTS_DIR/red_check.json")
\`\`\`

No \`span_metrics\` table, RED table or materialized view exists in ClickHouse.
RED stays in VictoriaMetrics (AD-21).

## Limitations

$(printf '%s\n' "$limitations" | jq -r '.[] | "- " + .')

## Reproduction

\`\`\`bash
${reproduction}
\`\`\`
EOF

	log "report: $RESULTS_DIR/report.md"
	log "json:   $RESULTS_DIR/report.json"
	if [ "$status" != "pass" ]; then
		exit 1
	fi
}

cleanup() {
	if [ "$CLEANUP" = 1 ]; then
		docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
		log "container removed; data directory preserved at $DATA_DIR"
	fi
}

main() {
	ensure_container
	create_database
	apply_schema
	stop_merges
	load
	optimize_table
	resolve_ids
	collect_storage
	measure "$RESULTS_DIR/trace_runs.jsonl" run_trace_query
	measure "$RESULTS_DIR/service_runs.jsonl" run_service_query
	service_map
	correlation
	red_check
	write_report
	cleanup
}

main "$@"
