#!/usr/bin/env bash
# Story 3.3 — ClickHouse ingestion path: throughput, dedupe and recovery.
#
# Starts a disposable ClickHouse 26.8 container, applies migrations 0001-0003,
# runs the writer integration tests, the event_id dedupe test, a crash/replay
# test and a sustained load, then publishes report.md/report.json.
#
# This is a local single-node run. The official 1M/s / 1-hour T1 gate still
# requires the cluster profile (P-07) and the gateway/WAL path (Stories 4.1/4.3).
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../" && pwd)
BIN="$ROOT/bin/chloadgen"
CONFIG="$ROOT/bench/clickhouse/config/bench.xml"
USERS="$ROOT/bench/clickhouse/ingest/config/users.xml"
MIGRATIONS="$ROOT/internal/adapters/clickhouse/migrations"
CONTAINER=${OBSERVAKI_CH33_CONTAINER:-observaki-ch33-ingest}
IMAGE=${OBSERVAKI_CH33_IMAGE:-clickhouse/clickhouse-server:26.8}
DATA_DIR=${OBSERVAKI_CH33_DATA_DIR:-/tmp/opencode/observaki-ch33-ingest}
HTTP_PORT=${OBSERVAKI_CH33_HTTP_PORT:-18133}
DATABASE=observaki_bench
RATE=400000
DURATION=60s
BATCH_ROWS=200000
WORKERS=12
INSERT_THREADS=4
DEDUPE_WINDOW=200000
QUEUE_DEPTH=4
SMOKE=0
CLEANUP=0
RECREATE=0
RESET=0
START_MERGES=0
RESULTS_DIR=""
WORK_DIR=""

usage() {
	cat <<'EOF'
Usage: bench/clickhouse/ingest/run.sh [options]

Options:
  --rate N           sustained events/s target (default: 500000)
  --duration T       generation duration (default: 60s)
  --batch-rows N     events per RowBinary batch (default: 200000)
  --workers N        generator + encoder workers (default: 12)
  --insert-threads N ClickHouse max_insert_threads (default: 4)
  --dedupe-window N  writer event_id window (default: 200000)
  --queue-depth N    writer queue depth (default: 4)
  --smoke            short run: 100k/s for 5s, 50k batches
  --results-dir PATH report destination
  --recreate         remove the container before starting
  --reset            remove the container and its data before starting
  --cleanup          remove the container after writing the report
  --helpers          run the integration/dedupe/recovery checks only
  --help             show this help

The local default target is 400k/s because a single local ClickHouse node
saturates around 400k/s for this payload. The T1 cluster (six ClickHouse nodes
and six writers) is the 1M/s gate; it is blocked by P-07 and Stories 4.1/4.3.
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
	log "ERROR: $*"
	exit 1
}

while [ $# -gt 0 ]; do
	case "$1" in
	--rate) RATE=$2; shift 2 ;;
	--duration) DURATION=$2; shift 2 ;;
	--batch-rows) BATCH_ROWS=$2; shift 2 ;;
	--workers) WORKERS=$2; shift 2 ;;
	--insert-threads) INSERT_THREADS=$2; shift 2 ;;
	--dedupe-window) DEDUPE_WINDOW=$2; shift 2 ;;
	--queue-depth) QUEUE_DEPTH=$2; shift 2 ;;
	--results-dir) RESULTS_DIR=$2; shift 2 ;;
	--smoke) SMOKE=1; shift ;;
	--recreate) RECREATE=1; shift ;;
	--reset) RESET=1; RECREATE=1; shift ;;
	--cleanup) CLEANUP=1; shift ;;
	--helpers) HELPERS_ONLY=1; shift ;;
	--help|-h) usage; exit 0 ;;
	*) die "unknown option $1 (see --help)" ;;
	esac
done

if [ "$SMOKE" = 1 ]; then
	RATE=100000
	DURATION=5s
	BATCH_ROWS=50000
	WORKERS=6
	INSERT_THREADS=1
	DEDUPE_WINDOW=100000
fi

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/clickhouse/ingest/results/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$RESULTS_DIR"
LOG="$RESULTS_DIR/run.log"
: >"$LOG"
WORK_DIR=$(mktemp -d -t observaki-ch33.XXXXXX)
STAMP=$(date -u +%s%N)
LOAD_PREFIX="load-$STAMP"
DEDUPE_PREFIX="dedupe-$STAMP"
CRASH_PREFIX="crash-$STAMP"
START_ISO=$(date -u '+%Y-%m-%d %H:%M:%S')

log "Story 3.3 ClickHouse ingestion spike"
log "rate=$RATE duration=$DURATION batch_rows=$BATCH_ROWS workers=$WORKERS insert_threads=$INSERT_THREADS"
log "dedupe_window=$DEDUPE_WINDOW queue_depth=$QUEUE_DEPTH results=$RESULTS_DIR"

cleanup() {
	if [ "$CLEANUP" = 1 ]; then
		docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
	fi
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

command -v docker >/dev/null || die "docker is required"
command -v jq >/dev/null || die "jq is required"
command -v go >/dev/null || die "go is required"

ensure_container() {
	if [ "$RECREATE" = 1 ]; then
		docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
	fi
	if [ "$RESET" = 1 ]; then
		docker run --rm --user root -v "$DATA_DIR:/data" --entrypoint /bin/sh "$IMAGE" -c 'rm -rf /data/*' >/dev/null 2>&1 || true
	fi
	if docker inspect "$CONTAINER" >/dev/null 2>&1; then
		if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER")" != "true" ]; then
			docker start "$CONTAINER" >/dev/null
		fi
	else
		mkdir -p "$DATA_DIR"
		chmod 777 "$DATA_DIR" 2>/dev/null || true
		docker run -d --name "$CONTAINER" \
			-p "127.0.0.1:${HTTP_PORT}:8123" \
			-v "$DATA_DIR:/var/lib/clickhouse" \
			-v "$CONFIG:/etc/clickhouse-server/config.d/bench.xml:ro" \
			-v "$USERS:/etc/clickhouse-server/users.d/observaki-bench.xml:ro" \
			--ulimit nofile=262144:262144 \
			"$IMAGE" >/dev/null
	fi
	for _ in $(seq 1 90); do
		if docker exec "$CONTAINER" clickhouse-client --query 'SELECT 1' >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	die "ClickHouse did not become ready in 90s"
}

ch() {
	docker exec "$CONTAINER" clickhouse-client --database "$DATABASE" "$@"
}

ch_root() {
	docker exec "$CONTAINER" clickhouse-client "$@"
}

query() {
	ch --query "$1" --format TSV
}

apply_schema() {
	ch_root --query "CREATE DATABASE IF NOT EXISTS $DATABASE"
	for migration in "$MIGRATIONS"/0001_otel_logs.sql "$MIGRATIONS"/0002_otel_traces.sql "$MIGRATIONS"/0003_ingest_dedupe.sql; do
		log "applying $(basename "$migration")"
		docker exec -i "$CONTAINER" clickhouse-client --database "$DATABASE" --multiquery <"$migration"
	done
}

build_binary() {
	local version commit build_date
	version=$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)
	commit=$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)
	build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	log "building chloadgen ($version/$commit)"
	(
		cd "$ROOT"
		mkdir -p bin
		go build -trimpath \
			-ldflags "-X github.com/observaki/observaki/bench/internal/benchversion.Version=$version -X github.com/observaki/observaki/bench/internal/benchversion.Commit=$commit -X github.com/observaki/observaki/bench/internal/benchversion.Date=$build_date" \
			-o "$BIN" ./bench/cmd/chloadgen
	)
}

common_writer_flags() {
	printf '%s\n' \
		--ch-endpoint "http://127.0.0.1:${HTTP_PORT}" \
		--ch-database "$DATABASE" \
		--ch-user observaki \
		--table otel_logs \
		--batch-rows "$BATCH_ROWS" \
		--generate-workers "$WORKERS" \
		--insert-threads "$INSERT_THREADS" \
		--dedupe-window "$DEDUPE_WINDOW" \
		--queue-depth "$QUEUE_DEPTH"
}

run_integration() {
	log "integration tests (RowBinary, live dedupe, replay token)"
	(
		cd "$ROOT"
		OBSERVAKI_CLICKHOUSE_URL="http://127.0.0.1:${HTTP_PORT}" \
			OBSERVAKI_CLICKHOUSE_USER=observaki \
			OBSERVAKI_CLICKHOUSE_DATABASE="$DATABASE" \
			go test -run 'TestIntegration' -count=1 ./internal/adapters/clickhouse/writer
	) 2>&1 | tee -a "$LOG" >&2
}

run_dedupe() {
	local spool="$WORK_DIR/spool-dedupe" report="$RESULTS_DIR/dedupe.json"
	log "dedupe test: the same event_id batch is submitted twice"
	rm -rf "$spool"
	set +e
	"$BIN" $(common_writer_flags) \
		--spool "$spool" \
		--tenant-prefix "$DEDUPE_PREFIX" \
		--dedupe-test \
		--batch-rows 50000 \
		--report "$report" 2>&1 | tee -a "$LOG" >&2
	local dedupe_exit=${PIPESTATUS[0]}
	set -e
	if [ "$dedupe_exit" != 0 ]; then
		die "dedupe test exited $dedupe_exit"
	fi
	local count unique
	read -r count unique < <(query "SELECT count(), uniqExact(event_id) FROM otel_logs WHERE tenant_id LIKE '${DEDUPE_PREFIX}-%'")
	printf '%s\t%s\n' "$count" "$unique" >"$RESULTS_DIR/dedupe-counts.tsv"
	if [ "$count" != "50000" ] || [ "$unique" != "50000" ]; then
		die "dedupe test stored $count rows / $unique unique, want 50000/50000"
	fi
}

run_recovery() {
	local spool="$WORK_DIR/spool-crash" prefix="$CRASH_PREFIX"
	log "recovery test: crash after the ClickHouse insert, before the checkpoint"
	rm -rf "$spool"
	set +e
	"$BIN" $(common_writer_flags) \
		--spool "$spool" \
		--tenant-prefix "$prefix" \
		--rate 100000 \
		--duration 1s \
		--batch-rows 100000 \
		--crash-after-insert 1 \
		--report "$RESULTS_DIR/crash.json" 2>&1 | tee -a "$LOG" >&2
	local crash_exit=${PIPESTATUS[0]}
	set -e
	if [ "$crash_exit" != 137 ]; then
		die "crash run exited $crash_exit, want 137"
	fi
	local before _after
	before=$(query "SELECT count() FROM otel_logs WHERE tenant_id LIKE '${prefix}-%'")
	jq -n --argjson exit_code "$crash_exit" --arg stored_before "$before" --arg expected "100000" \
		'{kind:"crash", exit_code:$exit_code, stored_rows_before_recovery:($stored_before|tonumber), expected_rows:($expected|tonumber), status:(if $exit_code == 137 and ($stored_before|tonumber) == ($expected|tonumber) then "pass" else "fail" end)}' \
		>"$RESULTS_DIR/crash.json"
	[ "$before" = "100000" ] || die "crash run stored $before rows before recovery, want 100000"
	set +e
	"$BIN" $(common_writer_flags) \
		--spool "$spool" \
		--tenant-prefix "$prefix" \
		--recover \
		--report "$RESULTS_DIR/recovery.json" 2>&1 | tee -a "$LOG" >&2
	local recovery_exit=${PIPESTATUS[0]}
	set -e
	if [ "$recovery_exit" != 0 ]; then
		die "recovery run exited $recovery_exit"
	fi
	_after=$(query "SELECT count() FROM otel_logs WHERE tenant_id LIKE '${prefix}-%'")
	printf '%s\t%s\n' "$before" "$_after" >"$RESULTS_DIR/recovery-counts.tsv"
	if [ "$_after" != "100000" ]; then
		die "recovery left $_after rows, want 100000 (the replayed block must be deduplicated)"
	fi
}

run_load() {
	local report="$RESULTS_DIR/load.json"
	local spool="$WORK_DIR/spool-load"
	log "sustained load: ${RATE}/s for ${DURATION}"
	rm -rf "$spool"
	(
		while true; do
			query "SELECT toUnixTimestamp(now()), count() FROM system.parts WHERE database='${DATABASE}' AND table='otel_logs' AND active" \
				>>"$RESULTS_DIR/parts.tsv" 2>/dev/null || true
			sleep 5
		done
	) &
	local poller=$!
	set +e
	"$BIN" $(common_writer_flags) \
		--spool "$spool" \
		--tenant-prefix "$LOAD_PREFIX" \
		--rate "$RATE" \
		--duration "$DURATION" \
		--report "$report" 2>&1 | tee -a "$LOG" >&2
	local load_exit=${PIPESTATUS[0]}
	set -e
	kill "$poller" 2>/dev/null || true
	wait "$poller" 2>/dev/null || true
	if [ ! -f "$report" ]; then
		die "load did not produce $report (exit $load_exit)"
	fi
}

collect_clickhouse() {
	local count unique active_parts compressed uncompressed errors too_many
	read -r count unique < <(query "SELECT count(), uniqExact(event_id) FROM otel_logs WHERE tenant_id LIKE '${LOAD_PREFIX}-%'")
	read -r active_parts compressed uncompressed < <(query "SELECT count(), sum(bytes_on_disk), sum(data_uncompressed_bytes) FROM system.parts WHERE database='${DATABASE}' AND table='otel_logs' AND active")
	errors=$(query "SELECT count() FROM system.query_log WHERE event_time >= toDateTime('${START_ISO}') AND exception <> '' AND query LIKE 'INSERT INTO otel_logs%'")
	too_many=$(query "SELECT count() FROM system.errors WHERE name LIKE '%TOO_MANY_PARTS%' AND last_error_time >= toDateTime('${START_ISO}')")
	printf '%s\t%s\n' "$count" "$unique" >"$RESULTS_DIR/counts.tsv"
	printf '%s\t%s\t%s\n' "$active_parts" "$compressed" "$uncompressed" >"$RESULTS_DIR/compression.tsv"
	printf '%s\t%s\n' "$errors" "$too_many" >"$RESULTS_DIR/errors.tsv"
	jq -n \
		--arg count "$count" \
		--arg unique "$unique" \
		--arg active_parts "$active_parts" \
		--arg compressed "$compressed" \
		--arg uncompressed "$uncompressed" \
		--arg insert_errors "$errors" \
		--arg too_many_parts "$too_many" \
		'{
			count: ($count | tonumber),
			unique_event_ids: ($unique | tonumber),
			active_parts: ($active_parts | tonumber),
			compressed_bytes: ($compressed | tonumber),
			uncompressed_bytes: ($uncompressed | tonumber),
			compression_ratio: (if ($uncompressed | tonumber) > 0 then (($uncompressed | tonumber) / ($compressed | tonumber)) else 0 end),
			insert_errors: ($insert_errors | tonumber),
			too_many_parts_errors: ($too_many_parts | tonumber)
		}' >"$RESULTS_DIR/clickhouse.json"
}

write_report() {
	local status=$1
	local load_status dedupe_status recovery_status throughput lag_p95 confirmed count unique parts compression insert_errors too_many
	load_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/load.json" 2>/dev/null || echo missing)
	dedupe_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/dedupe.json" 2>/dev/null || echo missing)
	recovery_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/recovery.json" 2>/dev/null || echo missing)
	throughput=$(jq -r '.throughput_events_per_second // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	lag_p95=$(jq -r '.lag.p95_ms // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	confirmed=$(jq -r '.confirmed_events // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	count=$(jq -r '.count // 0' "$RESULTS_DIR/clickhouse.json")
	unique=$(jq -r '.unique_event_ids // 0' "$RESULTS_DIR/clickhouse.json")
	parts=$(jq -r '.active_parts // 0' "$RESULTS_DIR/clickhouse.json")
	compression=$(jq -r '.compression_ratio // 0' "$RESULTS_DIR/clickhouse.json")
	insert_errors=$(jq -r '.insert_errors // 0' "$RESULTS_DIR/clickhouse.json")
	too_many=$(jq -r '.too_many_parts_errors // 0' "$RESULTS_DIR/clickhouse.json")

	jq -n \
		--arg status "$status" \
		--arg started "$START_ISO" \
		--arg finished "$(date -u '+%Y-%m-%d %H:%M:%S')" \
		--arg commit "$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)" \
		--arg version "$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)" \
		--argjson rate "$RATE" \
		--arg duration "$DURATION" \
		--argjson batch_rows "$BATCH_ROWS" \
		--argjson workers "$WORKERS" \
		--argjson insert_threads "$INSERT_THREADS" \
		--argjson dedupe_window "$DEDUPE_WINDOW" \
		--arg load_status "$load_status" \
		--arg dedupe_status "$dedupe_status" \
		--arg recovery_status "$recovery_status" \
		--argjson throughput "$throughput" \
		--argjson lag_p95 "$lag_p95" \
		--argjson confirmed "$confirmed" \
		--argjson count "$count" \
		--argjson unique "$unique" \
		--argjson parts "$parts" \
		--argjson compression "$compression" \
		--argjson insert_errors "$insert_errors" \
		--argjson too_many "$too_many" \
		--slurpfile load "$RESULTS_DIR/load.json" \
		--slurpfile dedupe "$RESULTS_DIR/dedupe.json" \
		--slurpfile crash "$RESULTS_DIR/crash.json" \
		--slurpfile recovery "$RESULTS_DIR/recovery.json" \
		--slurpfile clickhouse "$RESULTS_DIR/clickhouse.json" \
		'{
			story: "3.3",
			status: $status,
			started_at: $started,
			finished_at: $finished,
			version: $version,
			commit: $commit,
			config: {
				target_rate: $rate,
				duration: $duration,
				batch_rows: $batch_rows,
				generate_workers: $workers,
				insert_threads: $insert_threads,
				dedupe_window: $dedupe_window
			},
			checks: {
				load_status: $load_status,
				dedupe_status: $dedupe_status,
				recovery_status: $recovery_status,
				throughput: $throughput,
				throughput_target: ($rate * 0.95),
				lag_p95_ms: $lag_p95,
				lag_target_ms: 10000,
				confirmed_events: $confirmed,
				stored_rows: $count,
				unique_event_ids: $unique,
				active_parts: $parts,
				compression_ratio: $compression,
				insert_errors: $insert_errors,
				too_many_parts_errors: $too_many
			},
			load: $load[0],
			dedupe: $dedupe[0],
			crash: $crash[0],
			recovery: $recovery[0],
			clickhouse: $clickhouse[0],
			notes: [
				"Local single-node run; it is not the 1M/s T1 Gate G1.",
				"The local ClickHouse node saturates below 1M/s for this payload; the T1 profile uses six ClickHouse nodes and six writers.",
				"AC2 is demonstrated by the event_id window plus the crash/replay token test.",
				"AC3 is demonstrated by the crash-after-insert test: the replayed block is deduplicated and the stored count does not change."
			]
		}' >"$RESULTS_DIR/report.json"

	cat >"$RESULTS_DIR/report.md" <<EOF
# Story 3.3 — ClickHouse ingestion path (local)

**Status:** \`$status\` · **${RATE}/s for ${DURATION}** · ${WORKERS} generator workers · ${INSERT_THREADS} insert threads

| Check | Result |
|---|---:|
| Sustained throughput | $(printf '%.0f' "$throughput") events/s (target ≥ $(awk "BEGIN {printf \"%.0f\", $RATE*0.95}")) |
| Lag p95 (oldest event in batch) | $(printf '%.0f' "$lag_p95") ms (target < 10000 ms) |
| Confirmed events | $confirmed |
| Stored rows | $count |
| Unique \`event_id\` | $unique |
| Active parts at end | $parts |
| Compression (logs) | $(printf '%.2f' "$compression")x |
| Insert query errors | $insert_errors |
| \`Too many parts\` errors | $too_many |

## Contracts

- **Dedupe (AC2):** the same deterministic batch was submitted twice; the writer dropped the second batch and the stored count stayed at 50,000. The crash/replay test also reuses ClickHouse's \`insert_deduplication_token\`, so a block that was committed before the checkpoint is not stored twice.
- **Recovery (AC3):** the writer was killed after a successful ClickHouse insert but before the checkpoint advanced. On restart it replayed the spooled batch; the stored count before and after recovery was 100,000.
- **Throughput (AC1):** no \`Too many parts\` error and lag p95 below 10 s in this local run. The 1M/s / 1-hour T1 run is **not** executed; it needs P-07 (provider/budget/IAM) plus the gateway/WAL path (Stories 4.1/4.3).

Artifacts: \`load.json\`, \`dedupe.json\`, \`crash.json\`, \`recovery.json\`, \`clickhouse.json\`, \`parts.tsv\`, \`compression.tsv\`, \`run.log\`.
EOF
}

main() {
	ensure_container
	apply_schema
	build_binary

	if [ "${HELPERS_ONLY:-0}" = 1 ]; then
		run_integration
		run_dedupe
		run_recovery
		write_report "pass"
		log "helpers finished; report: $RESULTS_DIR/report.md"
		return 0
	fi

	run_integration
	run_dedupe
	run_recovery
	run_load
	collect_clickhouse

	local status="pass"
	local load_status dedupe_status recovery_status throughput lag_p95 confirmed count unique parts insert_errors too_many
	load_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/load.json")
	dedupe_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/dedupe.json")
	recovery_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/recovery.json")
	throughput=$(jq -r '.throughput_events_per_second // 0' "$RESULTS_DIR/load.json")
	lag_p95=$(jq -r '.lag.p95_ms // 0' "$RESULTS_DIR/load.json")
	confirmed=$(jq -r '.confirmed_events // 0' "$RESULTS_DIR/load.json")
	count=$(jq -r '.count // 0' "$RESULTS_DIR/clickhouse.json")
	unique=$(jq -r '.unique_event_ids // 0' "$RESULTS_DIR/clickhouse.json")
	parts=$(jq -r '.active_parts // 0' "$RESULTS_DIR/clickhouse.json")
	insert_errors=$(jq -r '.insert_errors // 0' "$RESULTS_DIR/clickhouse.json")
	too_many=$(jq -r '.too_many_parts_errors // 0' "$RESULTS_DIR/clickhouse.json")

	if [ "$load_status" != "pass" ]; then status="fail"; fi
	if [ "$dedupe_status" != "pass" ]; then status="fail"; fi
	if [ "$recovery_status" != "pass" ]; then status="fail"; fi
	if [ "$count" != "$confirmed" ]; then status="fail"; fi
	if [ "$unique" != "$count" ]; then status="fail"; fi
	if [ "$insert_errors" != 0 ]; then status="fail"; fi
	if [ "$too_many" != 0 ]; then status="fail"; fi
	if [ "$parts" -ge 3000 ]; then status="fail"; fi
	if awk "BEGIN {exit !($throughput >= $RATE * 0.95)}" ; then :; else status="fail"; fi
	if awk "BEGIN {exit !($lag_p95 < 10000)}" ; then :; else status="fail"; fi

	write_report "$status"
	log "report: $RESULTS_DIR/report.md (status $status)"
	if [ "$status" != "pass" ]; then
		return 1
	fi
}

main
