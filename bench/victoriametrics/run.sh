#!/usr/bin/env bash
# Story 3.4 — VictoriaMetrics ingestion, MetricsQL and cardinality.
#
# Starts a disposable VictoriaMetrics 1.148 LTS container, verifies rate /
# histogram_quantile / label aggregation, runs the 1M-series / 1M-sample/s
# load, executes the 20 representative MetricsQL queries over 24h windows and
# exercises per-tenant series admission with a noisy-neighbor check.
#
# This is a local single-node run. The T1 baseline still requires P-07 and the
# gateway/WAL/Query Service stories (4.1/4.3/4.5); it is not the G1 gate.
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../" && pwd)
BIN_DIR="$ROOT/bin"
VM_LOADGEN="$BIN_DIR/vmloadgen"
VM_QUERYBENCH="$BIN_DIR/vmquerybench"
VM_ADMISSION="$BIN_DIR/vmadmission"
QUERIES="$ROOT/bench/victoriametrics/queries/catalog.tsv"
CONTAINER=${OBSERVAKI_VM34_CONTAINER:-observaki-vm34}
IMAGE=${OBSERVAKI_VM34_IMAGE:-victoriametrics/victoria-metrics:v1.148.0}
DATA_DIR=${OBSERVAKI_VM34_DATA_DIR:-/tmp/opencode/observaki-vm34}
HTTP_PORT=${OBSERVAKI_VM34_HTTP_PORT:-18428}
ADMISSION_PORT=${OBSERVAKI_VM34_ADMISSION_PORT:-18429}
MEMORY_LIMIT=${OBSERVAKI_VM34_MEMORY_LIMIT:-4GiB}
SERIES=1000000
RATE=1000000
DURATION=1h
WORKERS=16
BATCH=5000
RUNS=5
WARMUP=1
CARD_SERIES=250000
CARD_RATE=10000
CARD_DURATION=30s
CARD_LIMIT_A=150000
CARD_LIMIT_B=100000
CARD_NEIGHBOR_SERIES=50000
CARD_NEIGHBOR_RATE=50000
CARD_NEIGHBOR_DURATION=60s
CARD_QUERY_RUNS=10
SMOKE=0
CLEANUP=0
RECREATE=0
RESET=0
SKIP_LOAD=0
SKIP_QUERIES=0
SKIP_CARDINALITY=0
RESULTS_DIR=""
WORK_DIR=""
POLLER_PID=""
ADMISSION_PID=""

usage() {
	cat <<'EOF'
Usage: bench/victoriametrics/run.sh [options]

Options:
  --series N              active series (default: 1000000)
  --rate N                samples/s (default: 1000000)
  --duration T            load duration (default: 1h)
  --workers N             sender workers (default: 16)
  --batch-series N        series per remote write request (default: 5000)
  --runs N                measured runs per query (default: 5)
  --warmup N              warmup runs per query (default: 1)
  --smoke                 short run: 50k series, 50k/s, 10s, 2 query runs
  --results-dir PATH      report destination
  --recreate              remove the container before starting
  --reset                 remove the container and its data before starting
  --cleanup               remove the container after writing the report
  --skip-load             reuse the existing dataset
  --skip-queries          skip the 20-query benchmark
  --skip-cardinality      skip the admission/noisy-neighbor test
  --help                  show this help

The published run is local single-node. The T1 baseline needs P-07 and the
gateway/WAL/Query Service path (Stories 4.1/4.3/4.5); it is not the G1 gate.
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
	--series) SERIES=$2; shift 2 ;;
	--rate) RATE=$2; shift 2 ;;
	--duration) DURATION=$2; shift 2 ;;
	--workers) WORKERS=$2; shift 2 ;;
	--batch-series) BATCH=$2; shift 2 ;;
	--runs) RUNS=$2; shift 2 ;;
	--warmup) WARMUP=$2; shift 2 ;;
	--results-dir) RESULTS_DIR=$2; shift 2 ;;
	--smoke) SMOKE=1; shift ;;
	--recreate) RECREATE=1; shift ;;
	--reset) RESET=1; RECREATE=1; shift ;;
	--cleanup) CLEANUP=1; shift ;;
	--skip-load) SKIP_LOAD=1; shift ;;
	--skip-queries) SKIP_QUERIES=1; shift ;;
	--skip-cardinality) SKIP_CARDINALITY=1; shift ;;
	--help|-h) usage; exit 0 ;;
	*) die "unknown option $1 (see --help)" ;;
	esac
done

if [ "$SMOKE" = 1 ]; then
	SERIES=100000
	RATE=100000
	DURATION=10s
	WORKERS=8
	BATCH=5000
	RUNS=2
	WARMUP=0
	CARD_SERIES=30000
	CARD_RATE=30000
	CARD_DURATION=15s
	CARD_LIMIT_A=20000
	CARD_LIMIT_B=60000
	CARD_NEIGHBOR_SERIES=30000
	CARD_NEIGHBOR_RATE=30000
	CARD_NEIGHBOR_DURATION=40s
	CARD_QUERY_RUNS=30
fi

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/victoriametrics/results/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$RESULTS_DIR"
LOG="$RESULTS_DIR/run.log"
: >"$LOG"
WORK_DIR=$(mktemp -d -t observaki-vm34.XXXXXX)
VM_URL="http://127.0.0.1:${HTTP_PORT}"
ADMISSION_URL="http://127.0.0.1:${ADMISSION_PORT}"
START_ISO=$(date -u '+%Y-%m-%d %H:%M:%S')

log "Story 3.4 VictoriaMetrics spike"
log "series=$SERIES rate=$RATE duration=$DURATION workers=$WORKERS batch=$BATCH runs=$RUNS"
log "results=$RESULTS_DIR"

cleanup() {
	if [ -n "$POLLER_PID" ]; then
		kill "$POLLER_PID" 2>/dev/null || true
		wait "$POLLER_PID" 2>/dev/null || true
	fi
	if [ -n "$ADMISSION_PID" ]; then
		kill "$ADMISSION_PID" 2>/dev/null || true
		wait "$ADMISSION_PID" 2>/dev/null || true
	fi
	if [ "$CLEANUP" = 1 ]; then
		docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
	fi
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

command -v docker >/dev/null || die "docker is required"
command -v jq >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"
command -v go >/dev/null || die "go is required"

remove_data() {
	docker run --rm --user root -v "$DATA_DIR:/data" --entrypoint /bin/sh "$IMAGE" -c 'rm -rf /data/*' >/dev/null 2>&1 || true
}

ensure_container() {
	if [ "$RECREATE" = 1 ]; then
		docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
	fi
	if [ "$RESET" = 1 ]; then
		remove_data
	fi
	if docker inspect "$CONTAINER" >/dev/null 2>&1; then
		if [ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER")" != "true" ]; then
			docker start "$CONTAINER" >/dev/null
		fi
	else
		mkdir -p "$DATA_DIR"
		chmod 777 "$DATA_DIR" 2>/dev/null || true
		docker run -d --name "$CONTAINER" \
			-p "127.0.0.1:${HTTP_PORT}:8428" \
			-v "$DATA_DIR:/victoria-metrics-data" \
			"$IMAGE" \
			-storageDataPath=/victoria-metrics-data \
			-retentionPeriod=30d \
			-httpListenAddr=:8428 \
			-memory.allowedBytes="$MEMORY_LIMIT" \
			-search.maxSamplesPerQuery=10000000000 \
			-search.maxUniqueTimeseries=2000000 \
			-search.maxQueryDuration=60s \
			-loggerLevel=WARN >/dev/null
	fi
	for _ in $(seq 1 90); do
		if curl -fsS "$VM_URL/health" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	die "VictoriaMetrics did not become ready in 90s"
}

build_binaries() {
	local version commit build_date
	version=$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)
	commit=$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)
	build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	log "building vmloadgen/vmquerybench/vmadmission ($version/$commit)"
	(
		cd "$ROOT"
		mkdir -p bin
		go build -trimpath \
			-ldflags "-X github.com/observaki/observaki/bench/internal/benchversion.Version=$version -X github.com/observaki/observaki/bench/internal/benchversion.Commit=$commit -X github.com/observaki/observaki/bench/internal/benchversion.Date=$build_date" \
			-o "$VM_LOADGEN" ./bench/cmd/vmloadgen
		go build -trimpath \
			-ldflags "-X github.com/observaki/observaki/bench/internal/benchversion.Version=$version -X github.com/observaki/observaki/bench/internal/benchversion.Commit=$commit -X github.com/observaki/observaki/bench/internal/benchversion.Date=$build_date" \
			-o "$VM_QUERYBENCH" ./bench/cmd/vmquerybench
		go build -trimpath \
			-ldflags "-X github.com/observaki/observaki/bench/internal/benchversion.Version=$version -X github.com/observaki/observaki/bench/internal/benchversion.Commit=$commit -X github.com/observaki/observaki/bench/internal/benchversion.Date=$build_date" \
			-o "$VM_ADMISSION" ./bench/cmd/vmadmission
	)
}

run_verify() {
	log "correctness: rate, histogram_quantile and label aggregation"
	set +e
	"$VM_QUERYBENCH" verify --vm-url "$VM_URL" --out "$RESULTS_DIR/verify.json" 2>&1 | tee -a "$LOG" >&2
	local exit_code=${PIPESTATUS[0]}
	set -e
	if [ "$exit_code" != 0 ]; then
		die "verification failed (exit $exit_code)"
	fi
}

start_resource_poller() {
	: >"$RESULTS_DIR/resources.tsv"
	(
		while true; do
			ts=$(date -u +%s)
			metrics=$(curl -fsS "$VM_URL/metrics" 2>/dev/null || true)
			rss=$(printf '%s\n' "$metrics" | awk '/^process_resident_memory_bytes / {print $2; exit}')
			inserted=$(printf '%s\n' "$metrics" | awk '/^vm_rows_inserted_total\{type="promremotewrite"\}/ {print $2; exit}')
			printf '%s\t%s\t%s\n' "$ts" "${rss:-0}" "${inserted:-0}" >>"$RESULTS_DIR/resources.tsv"
			sleep 10
		done
	) &
	POLLER_PID=$!
}

stop_resource_poller() {
	if [ -n "$POLLER_PID" ]; then
		kill "$POLLER_PID" 2>/dev/null || true
		wait "$POLLER_PID" 2>/dev/null || true
		POLLER_PID=""
	fi
}

run_load() {
	log "sustained load: ${RATE}/s for ${DURATION} with ${SERIES} series"
	start_resource_poller
	set +e
	"$VM_LOADGEN" \
		--vm-url "$VM_URL" \
		--series "$SERIES" \
		--rate "$RATE" \
		--duration "$DURATION" \
		--workers "$WORKERS" \
		--batch-series "$BATCH" \
		--report "$RESULTS_DIR/load.json" 2>&1 | tee -a "$LOG" >&2
	local exit_code=${PIPESTATUS[0]}
	set -e
	stop_resource_poller
	if [ "$exit_code" != 0 ]; then
		log "load exited $exit_code"
	fi
	return 0
}

wait_for_series() {
	local want=$1
	local deadline=$((SECONDS + 300))
	while [ "$SECONDS" -lt "$deadline" ]; do
		local count
		count=$(curl -fsS "$VM_URL/api/v1/series/count" 2>/dev/null | jq -r '.data[0] // 0' 2>/dev/null || echo 0)
		if [ "${count:-0}" -ge "$want" ]; then
			log "series visible: $count (target $want)"
			return 0
		fi
		sleep 2
	done
	die "series did not reach $want within 300s"
}

run_queries() {
	log "20 MetricsQL queries over 24h windows"
	set +e
	"$VM_QUERYBENCH" run \
		--vm-url "$VM_URL" \
		--catalog "$QUERIES" \
		--runs "$RUNS" \
		--warmup "$WARMUP" \
		--nocache \
		--out "$RESULTS_DIR/queries.json" 2>&1 | tee -a "$LOG" >&2
	local exit_code=${PIPESTATUS[0]}
	set -e
	if [ "$exit_code" != 0 ]; then
		log "query benchmark exited $exit_code"
	fi
	return 0
}

wait_for_proxy() {
	for _ in $(seq 1 30); do
		if curl -fsS "$ADMISSION_URL/healthz" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	die "admission proxy did not become ready"
}

run_cardinality() {
	log "cardinality: tenant-a limit=$CARD_LIMIT_A, tenant-b limit=$CARD_LIMIT_B"
	"$VM_ADMISSION" \
		--listen "127.0.0.1:${ADMISSION_PORT}" \
		--upstream "$VM_URL" \
		--limit "tenant-a=${CARD_LIMIT_A}" \
		--limit "tenant-b=${CARD_LIMIT_B}" \
		--default-limit 1000000 >"$RESULTS_DIR/admission.log" 2>&1 &
	ADMISSION_PID=$!
	wait_for_proxy

	log "neighbor load: tenant-b ${CARD_NEIGHBOR_SERIES} series at ${CARD_NEIGHBOR_RATE}/s"
	"$VM_LOADGEN" \
		--vm-url "$ADMISSION_URL" \
		--mode probe \
		--probe-tenant tenant-b \
		--series "$CARD_NEIGHBOR_SERIES" \
		--rate "$CARD_NEIGHBOR_RATE" \
		--duration "$CARD_NEIGHBOR_DURATION" \
		--workers 8 \
		--batch-series 100 \
		--allow-rejections \
		--report "$RESULTS_DIR/neighbor-load.json" 2>&1 | tee -a "$LOG" >&2 &
	local neighbor_pid=$!

	local active=0
	for _ in $(seq 1 60); do
		active=$(curl -fsS "$ADMISSION_URL/stats" 2>/dev/null | jq -r '.tenants["tenant-b"].active_series // 0' 2>/dev/null || echo 0)
		if [ "${active:-0}" -gt 0 ]; then
			break
		fi
		sleep 1
	done
	[ "${active:-0}" -gt 0 ] || die "tenant-b did not become active"
	# Give each neighbor series at least two samples before the baseline.
	sleep 8

	log "neighbor baseline query"
	set +e
	"$VM_QUERYBENCH" query \
		--vm-url "$VM_URL" \
		--expr 'sum by (series_id) (rate(observaki_cardinality_probe_total{tenant_id="tenant-b"}[5m]))' \
		--window 24h --step 5m --runs "$CARD_QUERY_RUNS" --warmup 2 --nocache \
		--out "$RESULTS_DIR/neighbor-baseline.json" 2>&1 | tee -a "$LOG" >&2
	set -e

	log "tenant-a violation: ${CARD_SERIES} series at ${CARD_RATE}/s for ${CARD_DURATION}"
	"$VM_LOADGEN" \
		--vm-url "$ADMISSION_URL" \
		--mode probe \
		--probe-tenant tenant-a \
		--series "$CARD_SERIES" \
		--rate "$CARD_RATE" \
		--duration "$CARD_DURATION" \
		--workers 8 \
		--batch-series 100 \
		--allow-rejections \
		--report "$RESULTS_DIR/cardinality-a.json" 2>&1 | tee -a "$LOG" >&2 &
	local violation_pid=$!

	local rejected=0
	for _ in $(seq 1 30); do
		rejected=$(curl -fsS "$ADMISSION_URL/stats" 2>/dev/null | jq -r '.tenants["tenant-a"].rejected_unique_series // 0' 2>/dev/null || echo 0)
		if [ "${rejected:-0}" -gt 0 ]; then
			break
		fi
		sleep 1
	done

	log "neighbor query during the violation"
	set +e
	"$VM_QUERYBENCH" query \
		--vm-url "$VM_URL" \
		--expr 'sum by (series_id) (rate(observaki_cardinality_probe_total{tenant_id="tenant-b"}[5m]))' \
		--window 24h --step 5m --runs "$CARD_QUERY_RUNS" --warmup 2 --nocache \
		--out "$RESULTS_DIR/neighbor-during.json" 2>&1 | tee -a "$LOG" >&2
	set -e

	wait "$violation_pid" 2>/dev/null || true
	wait "$neighbor_pid" 2>/dev/null || true

	curl -fsS "$ADMISSION_URL/stats" >"$RESULTS_DIR/cardinality-stats.json"
	curl -fsS "$ADMISSION_URL/metrics" >"$RESULTS_DIR/cardinality-metrics.txt"
	kill "$ADMISSION_PID" 2>/dev/null || true
	wait "$ADMISSION_PID" 2>/dev/null || true
	ADMISSION_PID=""
}

collect_vm() {
	local metrics series_count main_series_count rss_max rss_mean rows_inserted data_size slow_queries ignored disk_free
	metrics=$(curl -fsS "$VM_URL/metrics" 2>/dev/null || true)
	series_count=$(curl -fsS "$VM_URL/api/v1/series/count" 2>/dev/null | jq -r '.data[0] // 0' 2>/dev/null || echo 0)
	main_series_count=$(jq -r '.data[0] // 0' "$RESULTS_DIR/series-after-load.json" 2>/dev/null || echo 0)
	rss_max=$(awk 'BEGIN {max=0} {if ($2+0 > max) max=$2+0} END {printf "%.0f", max}' "$RESULTS_DIR/resources.tsv" 2>/dev/null || echo 0)
	rss_mean=$(awk 'BEGIN {sum=0; n=0} {sum+=$2+0; n++} END {if (n>0) printf "%.0f", sum/n; else print 0}' "$RESULTS_DIR/resources.tsv" 2>/dev/null || echo 0)
	rows_inserted=$(printf '%s\n' "$metrics" | awk '/^vm_rows_inserted_total\{type="promremotewrite"\}/ {print $2; exit}')
	data_size=$(du -sb "$DATA_DIR" 2>/dev/null | awk '{print $1}' || echo 0)
	slow_queries=$(printf '%s\n' "$metrics" | awk '/^vm_slow_queries_total / {print $2; exit}')
	ignored=$(printf '%s\n' "$metrics" | awk '/^vm_rows_ignored_total\{reason="too_many_labels"\}/ {print $2; exit}')
	disk_free=$(df -B1 --output=avail "$DATA_DIR" 2>/dev/null | tail -1 | tr -d ' ' || echo 0)
	jq -n \
		--arg series_count "${series_count:-0}" \
		--arg main_series_count "${main_series_count:-0}" \
		--arg rss_max "${rss_max:-0}" \
		--arg rss_mean "${rss_mean:-0}" \
		--arg rows_inserted "${rows_inserted:-0}" \
		--arg data_size "${data_size:-0}" \
		--arg slow_queries "${slow_queries:-0}" \
		--arg ignored "${ignored:-0}" \
		--arg disk_free "${disk_free:-0}" \
		'{
			series_count: ($series_count | tonumber),
			main_series_count: ($main_series_count | tonumber),
			rss_max_bytes: ($rss_max | tonumber),
			rss_mean_bytes: ($rss_mean | tonumber),
			rows_inserted: ($rows_inserted | tonumber),
			data_size_bytes: ($data_size | tonumber),
			slow_queries: ($slow_queries | tonumber),
			rows_ignored_too_many_labels: ($ignored | tonumber),
			disk_free_bytes: ($disk_free | tonumber)
		}' >"$RESULTS_DIR/vm.json"
}

write_report() {
	for name in load queries verify vm cardinality-stats; do
		if [ ! -f "$RESULTS_DIR/$name.json" ]; then
			printf '{}\n' >"$RESULTS_DIR/$name.json"
		fi
	done
	local load_status query_status verify_status card_status drift baseline_p95 during_p95
	load_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/load.json" 2>/dev/null || echo missing)
	query_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/queries.json" 2>/dev/null || echo missing)
	verify_status=$(jq -r '.status // "missing"' "$RESULTS_DIR/verify.json" 2>/dev/null || echo missing)
	baseline_p95=$(jq -r '.latency_ms.p95_ms // 0' "$RESULTS_DIR/neighbor-baseline.json" 2>/dev/null || echo 0)
	during_p95=$(jq -r '.latency_ms.p95_ms // 0' "$RESULTS_DIR/neighbor-during.json" 2>/dev/null || echo 0)
	drift=$(awk "BEGIN { if ($baseline_p95 > 0) printf \"%.2f\", (($during_p95 - $baseline_p95) / $baseline_p95) * 100; else print 0 }")
	card_status="pass"
	local a_rejected b_rejected neighbor_series
	a_rejected=$(jq -r '.tenants["tenant-a"].rejected_unique_series // 0' "$RESULTS_DIR/cardinality-stats.json" 2>/dev/null || echo 0)
	b_rejected=$(jq -r '.tenants["tenant-b"].rejected_unique_series // 0' "$RESULTS_DIR/cardinality-stats.json" 2>/dev/null || echo 0)
	neighbor_series=$(jq -r '.result_series // 0' "$RESULTS_DIR/neighbor-during.json" 2>/dev/null || echo 0)
	if [ "$a_rejected" -le 0 ] || [ "$b_rejected" -ne 0 ] || [ "$neighbor_series" -le 0 ] || awk "BEGIN {exit !($drift >= 10)}"; then
		card_status="fail"
	fi

	if [ "$SKIP_LOAD" = 1 ]; then load_status="skipped"; fi
	if [ "$SKIP_QUERIES" = 1 ]; then query_status="skipped"; fi
	if [ "$SKIP_CARDINALITY" = 1 ]; then card_status="skipped"; fi

	local status="pass"
	for check in "$load_status" "$query_status" "$verify_status" "$card_status"; do
		if [ "$check" != "pass" ] && [ "$check" != "skipped" ]; then
			status="fail"
		fi
	done

	local throughput sent errors rejected p95 series rss_max data_size rows_inserted series_count main_series_count
	throughput=$(jq -r '.achieved_rate // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	sent=$(jq -r '.sent_samples // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	errors=$(jq -r '.errors // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	rejected=$(jq -r '.rejected_samples // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	p95=$(jq -r '.latency_ms.p95_ms // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	series=$(jq -r '.series // 0' "$RESULTS_DIR/load.json" 2>/dev/null || echo 0)
	rss_max=$(jq -r '.rss_max_bytes // 0' "$RESULTS_DIR/vm.json" 2>/dev/null || echo 0)
	data_size=$(jq -r '.data_size_bytes // 0' "$RESULTS_DIR/vm.json" 2>/dev/null || echo 0)
	rows_inserted=$(jq -r '.rows_inserted // 0' "$RESULTS_DIR/vm.json" 2>/dev/null || echo 0)
	series_count=$(jq -r '.series_count // 0' "$RESULTS_DIR/vm.json" 2>/dev/null || echo 0)
	main_series_count=$(jq -r '.main_series_count // 0' "$RESULTS_DIR/vm.json" 2>/dev/null || echo 0)

	local query_p50 query_p95 query_p99 query_errors slowest_query_id slowest_query_p95
	query_p50=$(jq -r '.overall_latency_ms.p50_ms // 0' "$RESULTS_DIR/queries.json" 2>/dev/null || echo 0)
	query_p95=$(jq -r '.overall_latency_ms.p95_ms // 0' "$RESULTS_DIR/queries.json" 2>/dev/null || echo 0)
	query_p99=$(jq -r '.overall_latency_ms.p99_ms // 0' "$RESULTS_DIR/queries.json" 2>/dev/null || echo 0)
	query_errors=$(jq -r '.errors // 0' "$RESULTS_DIR/queries.json" 2>/dev/null || echo 0)
	slowest_query_id=$(jq -r '[.queries[]? | {id: .id, p95: (.latency_ms.p95_ms // 0)}] | sort_by(.p95) | last | .id // "none"' "$RESULTS_DIR/queries.json" 2>/dev/null || echo none)
	slowest_query_p95=$(jq -r '[.queries[]? | {id: .id, p95: (.latency_ms.p95_ms // 0)}] | sort_by(.p95) | last | .p95 // 0' "$RESULTS_DIR/queries.json" 2>/dev/null || echo 0)

	jq -n \
		--arg status "$status" \
		--arg started "$START_ISO" \
		--arg finished "$(date -u '+%Y-%m-%d %H:%M:%S')" \
		--arg commit "$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)" \
		--arg version "$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)" \
		--argjson series "$SERIES" \
		--argjson rate "$RATE" \
		--arg duration "$DURATION" \
		--argjson workers "$WORKERS" \
		--argjson batch "$BATCH" \
		--argjson runs "$RUNS" \
		--arg load_status "$load_status" \
		--arg query_status "$query_status" \
		--arg verify_status "$verify_status" \
		--arg card_status "$card_status" \
		--argjson throughput "$throughput" \
		--argjson sent "$sent" \
		--argjson errors "$errors" \
		--argjson rejected "$rejected" \
		--argjson load_p95 "$p95" \
		--argjson query_p50 "$query_p50" \
		--argjson query_p95 "$query_p95" \
		--argjson query_p99 "$query_p99" \
		--argjson query_errors "$query_errors" \
		--arg slowest_query_id "$slowest_query_id" \
		--argjson slowest_query_p95 "$slowest_query_p95" \
		--argjson baseline_p95 "$baseline_p95" \
		--argjson during_p95 "$during_p95" \
		--argjson drift "$drift" \
		--argjson a_rejected "$a_rejected" \
		--argjson b_rejected "$b_rejected" \
		--argjson neighbor_series "$neighbor_series" \
		--slurpfile load "$RESULTS_DIR/load.json" \
		--slurpfile queries "$RESULTS_DIR/queries.json" \
		--slurpfile verify "$RESULTS_DIR/verify.json" \
		--slurpfile vm "$RESULTS_DIR/vm.json" \
		--slurpfile stats "$RESULTS_DIR/cardinality-stats.json" \
		'{
			story: "3.4",
			status: $status,
			started_at: $started,
			finished_at: $finished,
			version: $version,
			commit: $commit,
			config: {
				target_series: $series,
				target_rate: $rate,
				duration: $duration,
				workers: $workers,
				batch_series: $batch,
				query_runs: $runs
			},
			checks: {
				load_status: $load_status,
				query_status: $query_status,
				verify_status: $verify_status,
				cardinality_status: $card_status,
				throughput: $throughput,
				throughput_target: ($rate * 0.95),
				sent_samples: $sent,
				load_errors: $errors,
				load_rejected_samples: $rejected,
				load_latency_p95_ms: $load_p95,
				query_p50_ms: $query_p50,
				query_p95_ms: $query_p95,
				query_p99_ms: $query_p99,
				query_errors: $query_errors,
				slowest_query_id: $slowest_query_id,
				slowest_query_p95_ms: $slowest_query_p95,
				neighbor_baseline_p95_ms: $baseline_p95,
				neighbor_during_p95_ms: $during_p95,
				neighbor_drift_percent: $drift,
				tenant_a_rejected_series: $a_rejected,
				tenant_b_rejected_series: $b_rejected,
				neighbor_result_series: $neighbor_series
			},
			load: $load[0],
			queries: $queries[0],
			verify: $verify[0],
			vm: $vm[0],
			cardinality: $stats[0],
			notes: [
				"Local single-node run; it is not the 1M/s T1 Gate G1.",
				"The 20 queries are tenant/project scoped and use nocache=1, so the reported latency is the raw query path without the rollup cache.",
				"Cardinality is enforced by bench/cmd/vmadmission, a spike proxy for AD-33; the production gateway owns this decision in Stories 4.1/4.2.",
				"The cardinality test uses a fresh probe metric namespace so the proxy can count series from zero.",
				"The T1 profile has 16 vCPU and 64 GiB; the local host is smaller in RAM and is not the reference hardware."
			]
		}' >"$RESULTS_DIR/report.json"

	cat >"$RESULTS_DIR/report.md" <<EOF
# Story 3.4 — VictoriaMetrics, MetricsQL and cardinality (local)

**Status:** \`$status\` · **${SERIES} series at ${RATE}/s for ${DURATION}** · ${WORKERS} workers · ${BATCH} series/request

| Check | Result |
|---|---:|
| Sustained throughput | $(printf '%.0f' "$throughput") samples/s (target ≥ $(awk "BEGIN {printf \"%.0f\", $RATE*0.95}")) |
| Sent samples | $sent |
| Load errors / rejected | $errors / $rejected |
| Load latency p95 | $(printf '%.1f' "$p95") ms |
| Active series after load | $main_series_count (target $SERIES) |
| Active series after cardinality | $series_count |
| VM RSS max / mean | $(awk "BEGIN {printf \"%.1f\", $rss_max/1073741824}") GiB / $(awk "BEGIN {printf \"%.1f\", $(jq -r '.rss_mean_bytes // 0' "$RESULTS_DIR/vm.json")/1073741824}") GiB |
| Data size | $(awk "BEGIN {printf \"%.1f\", $data_size/1073741824}") GiB |
| Rows inserted (remote write) | $rows_inserted |
| 20 queries p50 / p95 / p99 | $(printf '%.1f' "$query_p50") / **$(printf '%.1f' "$query_p95")** / $(printf '%.1f' "$query_p99") ms |
| Query errors | $query_errors |
| Slowest query p95 ($slowest_query_id) | $(printf '%.1f' "$slowest_query_p95") ms |
| Correctness checks | $verify_status |
| Tenant-a rejected series | $a_rejected (limit ${CARD_LIMIT_A}) |
| Tenant-b rejected series | $b_rejected (limit ${CARD_LIMIT_B}) |
| Neighbor p95 baseline / during | $(printf '%.1f' "$baseline_p95") / $(printf '%.1f' "$during_p95") ms |
| Neighbor drift | $(printf '%.2f' "$drift")% (target < 10%) |

## Contracts

- **Ingestion (AC1):** ${SERIES} active series, ${RATE} samples/s for ${DURATION}. The load generator uses Prometheus remote write and reports the achieved rate, request latency and errors.
- **Query (AC2):** 20 representative MetricsQL queries over 24h windows with \`nocache=1\`; p95 target < 1000 ms. Correctness is checked separately for \`rate\`, \`increase\`, \`histogram_quantile\` and label aggregation on a deterministic dataset.
- **Cardinality (AC3):** a per-tenant admission proxy enforces the series limit before forwarding to VictoriaMetrics. Tenant-a exceeds its limit and receives \`429 cardinality_limit_exceeded\` plus a rejection metric; tenant-b stays under its limit and its query p95 drift is measured against the baseline.

Artifacts: \`load.json\`, \`queries.json\`, \`verify.json\`, \`cardinality-stats.json\`, \`cardinality-metrics.txt\`, \`neighbor-baseline.json\`, \`neighbor-during.json\`, \`vm.json\`, \`resources.tsv\`, \`run.log\`.
EOF
}

main() {
	ensure_container
	build_binaries

	run_verify
	if [ "$SKIP_LOAD" != 1 ]; then
		run_load
	fi
	wait_for_series "$SERIES"
	curl -fsS "$VM_URL/api/v1/series/count" >"$RESULTS_DIR/series-after-load.json"
	if [ "$SKIP_QUERIES" != 1 ]; then
		run_queries
	fi
	if [ "$SKIP_CARDINALITY" != 1 ]; then
		run_cardinality
	fi
	collect_vm
	write_report

	local status
	status=$(jq -r '.status' "$RESULTS_DIR/report.json")
	log "report: $RESULTS_DIR/report.md (status $status)"
	if [ "$status" != "pass" ]; then
		return 1
	fi
}

main
