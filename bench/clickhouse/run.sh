#!/usr/bin/env bash
# Story 3.1 — ClickHouse logs storage spike.
#
# Starts a disposable ClickHouse 26.8 container, applies the embedded logs
# migration, loads a deterministic synthetic dataset, runs q03, measures
# compression, exercises the 7/15-day TTL and publishes report.md/report.json.
#
# The default is the 1B-row local run. Use --rows 10000000 for a smoke test.
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
MIGRATION="$ROOT/internal/adapters/clickhouse/migrations/0001_otel_logs.sql"
LOAD_SQL="$ROOT/bench/clickhouse/load.sql"
QUERY_SQL="$ROOT/bench/clickhouse/queries/q03_filtered_logs.sql"
CONFIG="$ROOT/bench/clickhouse/config/bench.xml"

ROWS=1000000000
CHUNK_ROWS=50000000
RUNS=20
BASE=${OBSERVAKI_CH_BASE:-$(date -u '+%Y-%m-%d %H:%M:%S')}
CONTAINER=${OBSERVAKI_CH_CONTAINER:-observaki-ch31}
IMAGE=${OBSERVAKI_CH_IMAGE:-clickhouse/clickhouse-server:26.8}
DATA_DIR=${OBSERVAKI_CH_DATA_DIR:-/tmp/opencode/observaki-ch31}
DATABASE=observaki_bench
TENANT=tenant-0
PROJECT=project-0
SERVICE=checkout
SEVERITY=ERROR
MIN_FREE_GIB=15
REUSE=0
SKIP_LOAD=0
TTL_ONLY=0
CLEANUP=0
RECREATE=0
OPTIMIZE=0
STOP_MERGES=0
RESULTS_DIR=""

usage() {
	cat <<'EOF'
Usage: bench/clickhouse/run.sh [options]

Options:
  --rows N           total synthetic rows (default: 1000000000)
  --chunk-rows N     rows per INSERT (default: 50000000)
  --runs N           measured query repetitions, plus one warm-up (default: 20)
  --base "YYYY-MM-DD HH:MM:SS"
                     end of the 24h event-time window (default: now UTC)
  --container NAME   Docker container name (default: observaki-ch31)
  --image IMAGE      ClickHouse image (default: clickhouse/clickhouse-server:26.8)
  --data-dir PATH    host directory for ClickHouse data
                     (default: /tmp/opencode/observaki-ch31)
  --results-dir PATH report destination
  --reuse            reuse an existing table with exactly --rows rows
  --skip-load        measure the existing table; do not load
  --ttl-only         run only the 7/15-day TTL check and exit
  --optimize         run OPTIMIZE TABLE ... FINAL before measuring
                     (only for small datasets)
  --stop-merges      stop background merges for the main table during the
                     load; required for the local 1B run because the merge
                     overhead otherwise exceeds the free disk
  --recreate         remove the container before starting
  --cleanup          remove the container after writing the report
  --help             show this help

The script needs docker, jq, awk and GNU date. It never leaves a running
cluster behind unless --cleanup is passed; the data directory is preserved
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
	--container) CONTAINER="$2"; shift 2 ;;
	--image) IMAGE="$2"; shift 2 ;;
	--data-dir) DATA_DIR="$2"; shift 2 ;;
	--results-dir) RESULTS_DIR="$2"; shift 2 ;;
	--reuse) REUSE=1; shift ;;
	--skip-load) SKIP_LOAD=1; shift ;;
	--ttl-only) TTL_ONLY=1; shift ;;
	--optimize) OPTIMIZE=1; shift ;;
	--stop-merges) STOP_MERGES=1; shift ;;
	--recreate) RECREATE=1; shift ;;
	--cleanup) CLEANUP=1; shift ;;
	--help | -h) usage; exit 0 ;;
	*) die "unknown argument: $1 (use --help)" ;;
	esac
done

for value in "$ROWS" "$CHUNK_ROWS" "$RUNS"; do
	[[ "$value" =~ ^[1-9][0-9]*$ ]] || die "expected a positive integer, got $value"
done

require docker
require jq
require awk
require sha256sum
require date

BASE=$(date -u -d "$BASE" '+%Y-%m-%d %H:%M:%S') || die "invalid --base value"
BASE_EPOCH=$(date -u -d "$BASE" +%s)
START=$(date -u -d "@$((BASE_EPOCH - 3600))" '+%Y-%m-%d %H:%M:%S')
END="$BASE"

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/clickhouse/results/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$RESULTS_DIR"
LOG="$RESULTS_DIR/run.log"
if [ "$REUSE" = 1 ] || [ "$SKIP_LOAD" = 1 ]; then
	touch "$LOG"
else
	: >"$LOG"
fi

log "Story 3.1 ClickHouse logs spike"
log "rows=$ROWS chunk_rows=$CHUNK_ROWS runs=$RUNS base=$BASE"
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
	docker exec -i "$CONTAINER" clickhouse-client --database "$DATABASE" --multiquery <"$MIGRATION"
}

stop_merges() {
	if [ "$STOP_MERGES" != 1 ]; then
		return
	fi
	log "stopping background merges for otel_logs (local 1B disk bound)"
	ch --query "SYSTEM STOP MERGES observaki_bench.otel_logs"
	ch --query "ALTER TABLE otel_logs MODIFY SETTING parts_to_delay_insert = 5000, parts_to_throw_insert = 6000"
}

table_count() {
	ch --query "SELECT count() FROM otel_logs"
}

free_gib() {
	df -Pk "$DATA_DIR" | awk 'NR == 2 { printf "%.1f", $4 / 1024 / 1024 }'
}

ttl_test() {
	local ttl_table=otel_logs_ttl_test
	local remaining keep7 keep15mid keep15new expired7 expired15 ttl_pass=true

	log "TTL check: 7-day and 15-day buckets"
	ch --query "DROP TABLE IF EXISTS $ttl_table"
	sed "s/CREATE TABLE IF NOT EXISTS otel_logs/CREATE TABLE IF NOT EXISTS $ttl_table/" "$MIGRATION" |
		ch_stdin --multiquery

	ch_stdin --multiquery <<'SQL'
INSERT INTO otel_logs_ttl_test
    (tenant_id, project_id, timestamp, ingested_at, observed_timestamp, severity, severity_number, service_name, body, attributes, resource_attributes, trace_id, span_id, event_id, retention_days)
VALUES
    ('tenant-0', 'project-0', now64(9) - INTERVAL 8 DAY,  now64(9) - INTERVAL 8 DAY,  now64(9) - INTERVAL 8 DAY,  'INFO', 9, 'checkout', 'expired-7d',   map('k', 'v'), map('service.name', 'checkout'), 't1', 's1', 'e1', 7),
    ('tenant-0', 'project-0', now64(9) - INTERVAL 1 DAY,  now64(9) - INTERVAL 1 DAY,  now64(9) - INTERVAL 1 DAY,  'INFO', 9, 'checkout', 'keep-7d',      map('k', 'v'), map('service.name', 'checkout'), 't2', 's2', 'e2', 7),
    ('tenant-0', 'project-2', now64(9) - INTERVAL 16 DAY, now64(9) - INTERVAL 16 DAY, now64(9) - INTERVAL 16 DAY, 'INFO', 9, 'checkout', 'expired-15d',  map('k', 'v'), map('service.name', 'checkout'), 't3', 's3', 'e3', 15),
    ('tenant-0', 'project-2', now64(9) - INTERVAL 8 DAY,  now64(9) - INTERVAL 8 DAY,  now64(9) - INTERVAL 8 DAY,  'INFO', 9, 'checkout', 'keep-15d',     map('k', 'v'), map('service.name', 'checkout'), 't4', 's4', 'e4', 15),
    ('tenant-0', 'project-2', now64(9) - INTERVAL 1 DAY,  now64(9) - INTERVAL 1 DAY,  now64(9) - INTERVAL 1 DAY,  'INFO', 9, 'checkout', 'keep-15d-new', map('k', 'v'), map('service.name', 'checkout'), 't5', 's5', 'e5', 15);
SQL

	ch --query "OPTIMIZE TABLE $ttl_table FINAL"
	remaining=$(ch --query "SELECT count() FROM $ttl_table")
	keep7=$(ch --query "SELECT count() FROM $ttl_table WHERE body = 'keep-7d'")
	keep15mid=$(ch --query "SELECT count() FROM $ttl_table WHERE body = 'keep-15d'")
	keep15new=$(ch --query "SELECT count() FROM $ttl_table WHERE body = 'keep-15d-new'")
	expired7=$(ch --query "SELECT count() FROM $ttl_table WHERE body = 'expired-7d'")
	expired15=$(ch --query "SELECT count() FROM $ttl_table WHERE body = 'expired-15d'")

	if [ "$remaining" != "3" ] || [ "$keep7" != "1" ] || [ "$keep15mid" != "1" ] || [ "$keep15new" != "1" ] ||
		[ "$expired7" != "0" ] || [ "$expired15" != "0" ]; then
		ttl_pass=false
	fi
	log "TTL result: remaining=$remaining keep7=$keep7 keep15mid=$keep15mid keep15new=$keep15new expired7=$expired7 expired15=$expired15 pass=$ttl_pass"

	jq -n \
		--argjson inserted 5 \
		--argjson remaining "$remaining" \
		--argjson keep_7d "$keep7" \
		--argjson keep_15d_8d_old "$keep15mid" \
		--argjson keep_15d_1d_old "$keep15new" \
		--argjson expired_7d "$expired7" \
		--argjson expired_15d "$expired15" \
		--argjson passed "$ttl_pass" \
		'{inserted: $inserted, remaining: $remaining, expected_remaining: 3, keep_7d: $keep_7d, keep_15d_8d_old: $keep_15d_8d_old, keep_15d_1d_old: $keep_15d_1d_old, expired_7d: $expired_7d, expired_15d: $expired_15d, passed: $passed}' \
		>"$RESULTS_DIR/ttl.json"

	ch --query "DROP TABLE IF EXISTS $ttl_table"
	if [ "$ttl_pass" != "true" ]; then
		die "TTL check failed; see $RESULTS_DIR/ttl.json"
	fi
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
			log "reusing $existing rows"
			return
		fi
		die "--reuse requested but the table has $existing rows, want $ROWS"
	fi

	log "creating a fresh logs table"
	ch --query "DROP TABLE IF EXISTS otel_logs"
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
		log "loaded $offset/$ROWS rows (chunk $count in ${elapsed}s, free ${free} GiB)"
		if awk -v f="$free" -v m="$MIN_FREE_GIB" 'BEGIN { exit !(f < m) }'; then
			die "free space ${free} GiB is below the ${MIN_FREE_GIB} GiB guard; load stopped at $offset rows"
		fi
	done

	if [ "$OPTIMIZE" = 1 ]; then
		log "OPTIMIZE TABLE otel_logs FINAL (small-dataset option)"
		ch --query "OPTIMIZE TABLE otel_logs FINAL"
	fi
}

collect_storage() {
	local parts compressed uncompressed rows body_avg
	rows=$(table_count)
	parts=$(ch --query "SELECT count() FROM system.parts WHERE database = '$DATABASE' AND table = 'otel_logs' AND active")
	compressed=$(ch --query "SELECT sum(data_compressed_bytes) FROM system.parts WHERE database = '$DATABASE' AND table = 'otel_logs' AND active")
	uncompressed=$(ch --query "SELECT sum(data_uncompressed_bytes) FROM system.parts WHERE database = '$DATABASE' AND table = 'otel_logs' AND active")
	body_avg=$(ch --query "SELECT round(avg(length(body)), 1) FROM otel_logs")
	log "storage: rows=$rows parts=$parts compressed=$compressed uncompressed=$uncompressed body_avg=$body_avg"
	printf '%s\t%s\t%s\t%s\t%s\n' "$rows" "$parts" "$compressed" "$uncompressed" "$body_avg" >"$RESULTS_DIR/storage.tsv"
}

run_query() {
	docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" --format JSON \
		--param_tenant="$TENANT" --param_project="$PROJECT" --param_service="$SERVICE" \
		--param_severity="$SEVERITY" --param_start="$START" --param_end="$END" \
		--query "$(cat "$QUERY_SQL")"
}

measure_query() {
	local i json returned
	log "query warm-up (not counted)"
	run_query >/dev/null

	: >"$RESULTS_DIR/runs.jsonl"
	for i in $(seq 1 "$RUNS"); do
		json=$(run_query)
		returned=$(printf '%s' "$json" | jq '.data | length')
		printf '%s' "$json" | jq -c --argjson run "$i" --argjson returned "$returned" \
			'{run: $run, elapsed_ms: (.statistics.elapsed * 1000), rows_read: .statistics.rows_read, bytes_read: .statistics.bytes_read, returned: $returned}' >>"$RESULTS_DIR/runs.jsonl"
	done
	log "measured $RUNS query runs"
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

write_report() {
	local storage rows parts compressed uncompressed body_avg ratio compressed_per_row
	local p50 p95 p99 rows_read_mean bytes_read_mean returned_mean
	local compression_pass query_pass query_rows_pass status schema_sha commit dirty
	local ch_version generated_at hostname cpus memory_bytes disk_total disk_free
	local limitations verdict_reason merges_stopped reproduction

	IFS=$'\t' read -r rows parts compressed uncompressed body_avg <"$RESULTS_DIR/storage.tsv"
	ratio=$(awk -v u="$uncompressed" -v c="$compressed" 'BEGIN { printf "%.2f", u / c }')
	compressed_per_row=$(awk -v c="$compressed" -v r="$rows" 'BEGIN { printf "%.1f", c / r }')

	jq -r '.elapsed_ms' "$RESULTS_DIR/runs.jsonl" >"$RESULTS_DIR/elapsed_ms.txt"
	p50=$(percentile "$RESULTS_DIR/elapsed_ms.txt" 50)
	p95=$(percentile "$RESULTS_DIR/elapsed_ms.txt" 95)
	p99=$(percentile "$RESULTS_DIR/elapsed_ms.txt" 99)
	rows_read_mean=$(jq -s 'map(.rows_read) | add / length' "$RESULTS_DIR/runs.jsonl")
	bytes_read_mean=$(jq -s 'map(.bytes_read) | add / length' "$RESULTS_DIR/runs.jsonl")
	returned_mean=$(jq -s 'map(.returned) | add / length' "$RESULTS_DIR/runs.jsonl")

	compression_pass=false
	if awk -v r="$ratio" 'BEGIN { exit !(r >= 5) }'; then compression_pass=true; fi
	query_pass=false
	if awk -v p="$p95" 'BEGIN { exit !(p < 1000) }'; then query_pass=true; fi
	query_rows_pass=false
	if awk -v r="$returned_mean" 'BEGIN { exit !(r > 0) }'; then query_rows_pass=true; fi
	if [ "$compression_pass" = "true" ] && [ "$query_pass" = "true" ] && [ "$query_rows_pass" = "true" ]; then
		status=pass
		verdict_reason="compression >= 5x, q03 p95 < 1000 ms and q03 returned rows"
	else
		status=fail
		verdict_reason="compression, q03 p95 or q03 returned rows missed the target"
	fi

	schema_sha=$(sha256sum "$MIGRATION" | awk '{ print $1 }')
	commit=$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)
	if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no 2>/dev/null)" ]; then dirty=true; else dirty=false; fi
	ch_version=$(ch --query "SELECT version()")
	generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	hostname=$(hostname)
	cpus=$(nproc)
	memory_bytes=$(awk '/MemTotal/ { printf "%.0f", $2 * 1024 }' /proc/meminfo)
	disk_total=$(df -B1 --output=size "$DATA_DIR" | tail -1 | tr -d ' ')
	disk_free=$(df -B1 --output=avail "$DATA_DIR" | tail -1 | tr -d ' ')
	limitations='[
		"single-node local ClickHouse, not the T1 cluster (Story 2.4 profile requires P-07)",
		"the 1B dataset is generated with ClickHouse SQL, not through the OTLP gateway (Story 4.1/3.3)",
		"q03 uses the last hour of the synthetic 24h window; the OS page cache is warm after the warm-up run",
		"the local host is smaller than the query-1b scenario reference (16 vCPU/64 GB); the report records the real hardware",
		"no replication, NATS, Query Service, row policies or chaos; those are Stories 3.3/3.5/4.5"
	]'
	merges_stopped=false
	if [ "$STOP_MERGES" = 1 ]; then
		merges_stopped=true
		limitations=$(printf '%s' "$limitations" | jq '. + ["background merges are stopped for the main table during the local load to bound disk use; the table is measured with unmerged parts"]')
	fi
	reproduction="./bench/clickhouse/run.sh --rows $ROWS --chunk-rows $CHUNK_ROWS --runs $RUNS"
	if [ "$STOP_MERGES" = 1 ]; then
		reproduction="$reproduction --stop-merges"
	fi
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
		--argjson rows "$rows" \
		--argjson chunk_rows "$CHUNK_ROWS" \
		--argjson parts "$parts" \
		--argjson merges_stopped "$merges_stopped" \
		--argjson compressed "$compressed" \
		--argjson uncompressed "$uncompressed" \
		--argjson ratio "$ratio" \
		--argjson compressed_per_row "$compressed_per_row" \
		--argjson body_avg "$body_avg" \
		--argjson p50 "$p50" \
		--argjson p95 "$p95" \
		--argjson p99 "$p99" \
		--argjson rows_read_mean "$rows_read_mean" \
		--argjson bytes_read_mean "$bytes_read_mean" \
		--argjson returned_mean "$returned_mean" \
		--argjson runs "$RUNS" \
		--argjson compression_pass "$compression_pass" \
		--argjson query_pass "$query_pass" \
		--argjson query_rows_pass "$query_rows_pass" \
		--arg status "$status" \
		--arg verdict_reason "$verdict_reason" \
		--arg schema_sha "$schema_sha" \
		--arg reproduction "$reproduction" \
		--arg tenant "$TENANT" \
		--arg project "$PROJECT" \
		--arg service "$SERVICE" \
		--arg severity "$SEVERITY" \
		--argjson limitations "$limitations" \
		--slurpfile ttl "$RESULTS_DIR/ttl.json" \
		'{
			schema_version: 1,
			generated_at: $generated_at,
			commit: $commit,
			dirty: $dirty,
			clickhouse: {version: $clickhouse_version, image: $image, container: $container},
			host: {hostname: $hostname, cpus: $cpus, memory_bytes: $memory_bytes, disk_total_bytes: $disk_total_bytes, disk_free_bytes: $disk_free_bytes, data_dir: $data_dir},
			schema: {migration: "0001_otel_logs.sql", sha256: $schema_sha, partition_by: "toDate(ingested_at)", order_by: "(tenant_id, project_id, service_name, timestamp)", ttl: "ingested_at + INTERVAL retention_days DAY"},
			dataset: {rows: $rows, chunk_rows: $chunk_rows, chunks: (($rows / $chunk_rows) | ceil), base: $base, window_hours: 24, body_avg_bytes: $body_avg, tenants: 2, projects: 4, services: 8, severity_mix: "90% INFO / 7% WARN / 3% ERROR", retention_days: [7, 15]},
			storage: {parts: $parts, merges_stopped: $merges_stopped, compressed_bytes: $compressed, uncompressed_bytes: $uncompressed, ratio: $ratio, compressed_bytes_per_row: $compressed_per_row},
			query: {id: "q03", description: "Filtered log search by service and severity, 1h window", runs: $runs, p50_ms: $p50, p95_ms: $p95, p99_ms: $p99, rows_read_mean: $rows_read_mean, bytes_read_mean: $bytes_read_mean, returned_mean: $returned_mean, tenant: $tenant, project: $project, service: $service, severity: $severity, start: $start, end: $end},
			ttl: $ttl[0],
			verdict: {status: $status, reason: $verdict_reason, criteria: {compression_ge_5x: $compression_pass, query_p95_lt_1s: $query_pass, query_returned_rows: $query_rows_pass, ttl_removes_expired: $ttl[0].passed}},
			reproduction: $reproduction,
			limitations: $limitations
		}' >"$RESULTS_DIR/report.json"

	cat >"$RESULTS_DIR/report.md" <<EOF
# Story 3.1 — ClickHouse logs storage spike

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
| Schema | \`0001_otel_logs.sql\` sha256 \`${schema_sha}\` |

## Dataset

| Field | Value |
|---|---:|
| Rows | ${rows} |
| Chunks | $(( (ROWS + CHUNK_ROWS - 1) / CHUNK_ROWS )) × ${CHUNK_ROWS} rows |
| Event-time window | 24 h ending ${BASE} |
| Body average | ${body_avg} B |
| Dimensions | 2 tenants × 4 projects × 8 services; 90% INFO / 7% WARN / 3% ERROR |
| Retention | 7 and 15 days per row |

## Compression

| Metric | Value |
|---|---:|
| Compressed | $(awk -v b="$compressed" 'BEGIN { printf "%.2f GiB", b / 1073741824 }') |
| Uncompressed | $(awk -v b="$uncompressed" 'BEGIN { printf "%.2f GiB", b / 1073741824 }') |
| Ratio | **${ratio}x** (target ≥ 5x) |
| Compressed bytes/row | ${compressed_per_row} B |
| Active parts | ${parts} |

## q03 — filtered log search (1h window)

\`\`\`sql
$(cat "$QUERY_SQL")
\`\`\`

| Metric | Value |
|---|---:|
| Runs (after warm-up) | ${RUNS} |
| p50 | ${p50} ms |
| p95 | ${p95} ms (target < 1000 ms) |
| p99 | ${p99} ms |
| Rows read (mean) | ${rows_read_mean} |
| Bytes read (mean) | ${bytes_read_mean} |
| Rows returned (mean) | ${returned_mean} |

## TTL check

| Bucket | Inserted | Remaining | Expected |
|---|---:|---:|---:|
| 7-day, 8 days old | 1 | $(jq -r '.expired_7d' "$RESULTS_DIR/ttl.json") | 0 |
| 7-day, 1 day old | 1 | $(jq -r '.keep_7d' "$RESULTS_DIR/ttl.json") | 1 |
| 15-day, 16 days old | 1 | $(jq -r '.expired_15d' "$RESULTS_DIR/ttl.json") | 0 |
| 15-day, 8 days old | 1 | $(jq -r '.keep_15d_8d_old' "$RESULTS_DIR/ttl.json") | 1 |
| 15-day, 1 day old | 1 | $(jq -r '.keep_15d_1d_old' "$RESULTS_DIR/ttl.json") | 1 |

The 15-day/8-day-old row is the bucket boundary test: it must survive because
only the 7-day policy expires at 8 days. The expired rows are removed by the
TTL merge, not by the query.

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

	if [ "$TTL_ONLY" = 1 ]; then
		ttl_test
		log "TTL-only run finished"
		return
	fi

	ttl_test
	load
	collect_storage
	measure_query
	write_report
	cleanup
}

main "$@"
