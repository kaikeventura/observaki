#!/usr/bin/env bash
# Story 4.3 — gateway batching, zstd compression, sharding and the NATS
# JetStream WAL.
#
# Starts a local JetStream server, a gateway with two tenant tokens, and two
# OTLP load generators. The gateway only acknowledges after the JetStream ack.
# The run then reads every WAL message back with walcheck and proves that no
# acknowledged event was lost, that event IDs are unique (the dedupe input) and
# that one tenant spreads across shards.
#
# The local default is a 100k events/s proxy. The AC1 target is 1M/s for one
# hour on the T1 fleet and remains blocked by P-07; this harness never claims
# G1 evidence.
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
GATEWAY_BIN="$ROOT/bin/observaki"
LOADGEN_BIN="$ROOT/bin/otlploadgen"
WALCHECK_BIN="$ROOT/bin/walcheck"

DURATION=10s
RATE=100000
BATCH=500
WORKERS=4
MIX=34,33,33
COMPRESSION=gzip
RUNS=1
SHARDS=32
LINGER=5ms
METRIC_PADDING=832
LOG_BODY=820
TRACE_PADDING=2400
RESULTS_DIR=""
SMOKE=0
NATS_IMAGE="${NATS_IMAGE:-nats:2.15.2-alpine}"

usage() {
	cat <<'EOF'
Usage: bench/gateway/wal/run.sh [options]

Options:
  --duration T       measured duration per run (default: 10s)
  --rate N           total events/s across both tenants (default: 100000)
  --batch-size N     events per OTLP request (default: 500)
  --workers N        workers per tenant load generator (default: 4)
  --mix a,b,c        metrics,logs,traces mix in events per 100 (default: 34,33,33)
  --compression C    gzip or none (default: gzip)
  --runs N           repetitions (default: 1)
  --shards N         WAL shards per tenant/project/epoch/signal (default: 32)
  --linger T         gateway batch linger (default: 5ms)
  --small-payload    minimal payloads; useful on a shared local host
  --smoke            short harness check (2s, 5k/s, 1 run)
  --results-dir P    report destination
  --help             show this help

Requires Docker (or NATS_IMAGE with a built-in nats-server config) and jq.
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
	--duration) DURATION=$2; shift 2 ;;
	--rate) RATE=$2; shift 2 ;;
	--batch-size) BATCH=$2; shift 2 ;;
	--workers) WORKERS=$2; shift 2 ;;
	--mix) MIX=$2; shift 2 ;;
	--compression) COMPRESSION=$2; shift 2 ;;
	--runs) RUNS=$2; shift 2 ;;
	--shards) SHARDS=$2; shift 2 ;;
	--linger) LINGER=$2; shift 2 ;;
	--small-payload) METRIC_PADDING=0; LOG_BODY=64; TRACE_PADDING=0; shift ;;
	--results-dir) RESULTS_DIR=$2; shift 2 ;;
	--smoke) SMOKE=1; shift ;;
	--help|-h) usage; exit 0 ;;
	*) die "unknown option $1 (see --help)" ;;
	esac
done

if [ "$SMOKE" = 1 ]; then
	DURATION=2s
	RATE=5000
	BATCH=100
	WORKERS=2
	RUNS=1
	SHARDS=8
fi

case "$COMPRESSION" in
gzip|none) ;;
*) die "--compression must be gzip or none" ;;
esac
[ "$RUNS" -ge 1 ] || die "--runs must be at least 1"

command -v jq >/dev/null || die "jq is required"
command -v docker >/dev/null || die "docker is required for the local NATS server"
command -v curl >/dev/null || die "curl is required"
[ -x "$GATEWAY_BIN" ] || die "$GATEWAY_BIN is missing (run make build)"
[ -x "$LOADGEN_BIN" ] || die "$LOADGEN_BIN is missing (run make build)"
[ -x "$WALCHECK_BIN" ] || die "$WALCHECK_BIN is missing (run make build)"

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/gateway/wal/results/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$RESULTS_DIR"
WORK_DIR=$(mktemp -d -t observaki-gateway-wal.XXXXXX)
LOG="$RESULTS_DIR/run.log"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

nats_container=""
gateway_pid=""
load_pids=()

cleanup() {
	for p in "${load_pids[@]:-}"; do
		kill -TERM "$p" 2>/dev/null || true
		wait "$p" 2>/dev/null || true
	done
	if [ -n "$gateway_pid" ]; then
		kill -TERM "$gateway_pid" 2>/dev/null || true
		wait "$gateway_pid" 2>/dev/null || true
	fi
	if [ -n "$nats_container" ]; then
		docker rm -f "$nats_container" >/dev/null 2>&1 || true
	fi
	rm -rf "$WORK_DIR" 2>/dev/null || true
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

nats_port=$(find_free_port 18500 18540) || die "no free NATS port"
nats_monitor_port=$(find_free_port 18541 18580) || die "no free NATS monitor port"
grpc_port=$(find_free_port 18581 18620) || die "no free gRPC port"
http_port=$(find_free_port 18621 18660) || die "no free HTTP port"

mkdir -p "$WORK_DIR/nats/js"
cat >"$WORK_DIR/nats/nats.conf" <<'EOF'
port: 4222
http_port: 8222
max_payload: 16777216
jetstream {
  store_dir: "/data/js"
}
EOF
nats_container="observaki-wal-nats-$$"
log "starting NATS JetStream on 127.0.0.1:${nats_port}"
docker run -d --rm --user "$(id -u):$(id -g)" --name "$nats_container" \
	-p "${nats_port}:4222" -p "${nats_monitor_port}:8222" \
	-v "$WORK_DIR/nats:/data" "$NATS_IMAGE" -c /data/nats.conf >/dev/null

nats_url="nats://127.0.0.1:${nats_port}"
ready=0
for _ in $(seq 1 60); do
	if curl -fsS "http://127.0.0.1:${nats_monitor_port}/varz" >/dev/null 2>&1; then
		ready=1
		break
	fi
	sleep 0.2
done
[ "$ready" = 1 ] || die "NATS did not become ready"

token_a="wal-a-$(date -u +%s%N)-${RANDOM}"
token_b="wal-b-$(date -u +%s%N)-${RANDOM}"
hash_a=$(printf '%s' "$token_a" | sha256sum | cut -d' ' -f1)
hash_b=$(printf '%s' "$token_b" | sha256sum | cut -d' ' -f1)
issued_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
expires_at=$(date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ)
snapshot="$WORK_DIR/ingest-auth.json"
jq -n \
	--arg version "gateway-wal-$(date -u +%s)" \
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

log "starting gateway on grpc=127.0.0.1:${grpc_port} http=127.0.0.1:${http_port} (shards=${SHARDS}, linger=${LINGER})"
OBSERVAKI_ENV=development \
	OBSERVAKI_OTLP_GRPC_ADDR="127.0.0.1:${grpc_port}" \
	OBSERVAKI_OTLP_HTTP_ADDR="127.0.0.1:${http_port}" \
	OBSERVAKI_INGEST_AUTH_FILE="$snapshot" \
	OBSERVAKI_INGEST_AUDIT_FILE="$WORK_DIR/audit.jsonl" \
	OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL=5s \
	OBSERVAKI_NATS_URL="$nats_url" \
	OBSERVAKI_NATS_SHARDS="$SHARDS" \
	OBSERVAKI_NATS_BATCH_LINGER="$LINGER" \
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

metric_value() {
	local name=$1
	curl -fsS "http://127.0.0.1:${http_port}/metrics" |
		awk -v name="$name" '$1 == name {print $2; exit}'
}

tenant_metric() {
	local name=$1
	local tenant=$2
	curl -fsS "http://127.0.0.1:${http_port}/metrics" |
		awk -v name="$name" -v tenant="$tenant" 'index($1, name "{tenant_id=\"" tenant "\"}") == 1 {print $2; exit}'
}

run_loadgen() {
	local name=$1
	local rate=$2
	local token=$3
	local output=$4
	"$LOADGEN_BIN" \
		--protocol http \
		--endpoint "http://127.0.0.1:${http_port}" \
		--rate "$rate" \
		--duration "$DURATION" \
		--batch-size "$BATCH" \
		--workers "$WORKERS" \
		--mix "$MIX" \
		--compression "$COMPRESSION" \
		--metric-padding "$METRIC_PADDING" \
		--log-body "$LOG_BODY" \
		--trace-padding "$TRACE_PADDING" \
		--metrics-addr "" \
		--header "Authorization=Bearer ${token}" \
		>"$output" 2>"${output%.json}.log"
}

cumulative_accepted=0
run_reports=()
for run in $(seq 1 "$RUNS"); do
	log "run ${run}/${RUNS}: two tenants at $((RATE / 2)) events/s each for ${DURATION}"
	wal_events_before=$(metric_value "observaki_gateway_wal_published_events_total")
	wal_batches_before=$(metric_value "observaki_gateway_wal_published_batches_total")
	wal_events_before=${wal_events_before:-0}
	wal_batches_before=${wal_batches_before:-0}

	run_loadgen "tenant-a-${run}" "$((RATE / 2))" "$token_a" "$RESULTS_DIR/tenant-a-${run}.json" &
	load_pids+=("$!")
	run_loadgen "tenant-b-${run}" "$((RATE / 2))" "$token_b" "$RESULTS_DIR/tenant-b-${run}.json" &
	load_pids+=("$!")
	for pid in "${load_pids[@]}"; do
		wait "$pid"
	done
	load_pids=()

	sent_a=$(jq -r '.metrics.sent' "$RESULTS_DIR/tenant-a-${run}.json")
	sent_b=$(jq -r '.metrics.sent' "$RESULTS_DIR/tenant-b-${run}.json")
	failed_a=$(jq -r '.metrics.failed' "$RESULTS_DIR/tenant-a-${run}.json")
	failed_b=$(jq -r '.metrics.failed' "$RESULTS_DIR/tenant-b-${run}.json")
	rejected_a=$(jq -r '.metrics.rejected' "$RESULTS_DIR/tenant-a-${run}.json")
	rejected_b=$(jq -r '.metrics.rejected' "$RESULTS_DIR/tenant-b-${run}.json")
	achieved_a=$(jq -r '.metrics.achieved_rate' "$RESULTS_DIR/tenant-a-${run}.json")
	achieved_b=$(jq -r '.metrics.achieved_rate' "$RESULTS_DIR/tenant-b-${run}.json")
	client_p99_a=$(jq -r '.metrics.latency.p99_ms' "$RESULTS_DIR/tenant-a-${run}.json")
	client_p99_b=$(jq -r '.metrics.latency.p99_ms' "$RESULTS_DIR/tenant-b-${run}.json")

	for _ in $(seq 1 100); do
		wal_pending=$(metric_value "observaki_gateway_wal_pending_bytes")
		[ "${wal_pending:-1}" = "0" ] && break
		sleep 0.05
	done
	wal_events_after=$(metric_value "observaki_gateway_wal_published_events_total")
	wal_batches_after=$(metric_value "observaki_gateway_wal_published_batches_total")
	wal_events_delta=$((wal_events_after - wal_events_before))
	wal_batches_delta=$((wal_batches_after - wal_batches_before))
	sent_total=$((sent_a + sent_b))
	cumulative_accepted=$((cumulative_accepted + sent_total))

	server_p99_a=$(tenant_metric "observaki_gateway_request_duration_p99_seconds" "tenant-a")
	server_p99_b=$(tenant_metric "observaki_gateway_request_duration_p99_seconds" "tenant-b")
	wal_publish_p99=$(curl -fsS "http://127.0.0.1:${http_port}/metrics" |
		awk -F' ' '/observaki_gateway_wal_publish_duration_seconds\{quantile="0.99"\}/ {print $2; exit}')

	run_reports+=("$(jq -n \
		--argjson run "$run" \
		--argjson sent_a "$sent_a" --argjson sent_b "$sent_b" \
		--argjson failed_a "$failed_a" --argjson failed_b "$failed_b" \
		--argjson rejected_a "$rejected_a" --argjson rejected_b "$rejected_b" \
		--argjson achieved_a "$achieved_a" --argjson achieved_b "$achieved_b" \
		--argjson client_p99_a "$client_p99_a" --argjson client_p99_b "$client_p99_b" \
		--argjson server_p99_a "${server_p99_a:-0}" --argjson server_p99_b "${server_p99_b:-0}" \
		--argjson wal_publish_p99 "${wal_publish_p99:-0}" \
		--argjson wal_events_delta "$wal_events_delta" \
		--argjson wal_batches_delta "$wal_batches_delta" \
		'{
			run: $run,
			sent_tenant_a: $sent_a,
			sent_tenant_b: $sent_b,
			failed: ($failed_a + $failed_b),
			rejected: ($rejected_a + $rejected_b),
			achieved_rate_tenant_a: $achieved_a,
			achieved_rate_tenant_b: $achieved_b,
			client_p99_ms_tenant_a: (($client_p99_a * 100 | round) / 100),
			client_p99_ms_tenant_b: (($client_p99_b * 100 | round) / 100),
			gateway_p99_ms_tenant_a: (($server_p99_a * 100000 | round) / 100),
			gateway_p99_ms_tenant_b: (($server_p99_b * 100000 | round) / 100),
			wal_publish_p99_ms: (($wal_publish_p99 * 100000 | round) / 100),
			wal_events_delta: $wal_events_delta,
			wal_batches_delta: $wal_batches_delta
		}')")
done

log "stopping gateway and verifying the WAL with walcheck"
kill -TERM "$gateway_pid"
if ! wait "$gateway_pid"; then
	die "gateway did not exit cleanly after SIGTERM; see $RESULTS_DIR/gateway.log"
fi
gateway_pid=""

walcheck_status=0
"$WALCHECK_BIN" --url "$nats_url" --expect-events "$cumulative_accepted" \
	--fail-on-duplicates --out "$RESULTS_DIR/wal.json" \
	|| walcheck_status=$?

run_reports_json=$(printf '%s\n' "${run_reports[@]}" | jq -s '.')

max_gateway_p99=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map([.gateway_p99_ms_tenant_a, .gateway_p99_ms_tenant_b] | max) | max')
max_client_p99=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map([.client_p99_ms_tenant_a, .client_p99_ms_tenant_b] | max) | max')
total_failed=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.failed) | add')
total_rejected=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.rejected) | add')
total_wal_delta=$(printf '%s\n' "${run_reports[@]}" | jq -s 'map(.wal_events_delta) | add')
wal_events=$(jq -r '.total_events' "$RESULTS_DIR/wal.json")
wal_unique=$(jq -r '.unique_events' "$RESULTS_DIR/wal.json")
wal_duplicates=$(jq -r '.duplicate_events' "$RESULTS_DIR/wal.json")
wal_compression=$(jq -r '.compression_ratio' "$RESULTS_DIR/wal.json")
wal_subjects=$(jq -r '.subjects | length' "$RESULTS_DIR/wal.json")
wal_shards=$(jq -r '.shards | length' "$RESULTS_DIR/wal.json")
wal_status=$(jq -r '.status' "$RESULTS_DIR/wal.json")
max_subject_share=$(jq -r '.subjects | to_entries | map(.value) | (max / (add | if . == 0 then 1 else . end)) * 100' "$RESULTS_DIR/wal.json")
wal_compression_display=$(awk -v value="$wal_compression" 'BEGIN { printf "%.2f", value }')
max_gateway_p99_display=$(awk -v value="$max_gateway_p99" 'BEGIN { printf "%.2f", value }')
max_client_p99_display=$(awk -v value="$max_client_p99" 'BEGIN { printf "%.2f", value }')
max_subject_share_display=$(awk -v value="$max_subject_share" 'BEGIN { printf "%.2f", value }')

status=pass
t1_status=pending
reasons=()
if [ "$walcheck_status" -ne 0 ] || [ "$wal_status" != "pass" ]; then
	status=fail
	reasons+=("walcheck failed: $(jq -c '.errors' "$RESULTS_DIR/wal.json")")
fi
if [ "$total_failed" -ne 0 ] || [ "$total_rejected" -ne 0 ]; then
	status=fail
	reasons+=("load generator failed/rejected events: ${total_failed}/${total_rejected}")
fi
if [ "$total_wal_delta" -ne "$cumulative_accepted" ]; then
	status=fail
	reasons+=("WAL ack count ${total_wal_delta} != accepted ${cumulative_accepted}")
fi
if [ "$wal_events" -ne "$cumulative_accepted" ]; then
	status=fail
	reasons+=("WAL events ${wal_events} != accepted ${cumulative_accepted}")
fi
if [ "$wal_duplicates" -ne 0 ]; then
	status=fail
	reasons+=("WAL has ${wal_duplicates} duplicate event ids")
fi
if ! awk -v p99="$max_gateway_p99" 'BEGIN { exit !(p99 < 50) }'; then
	status=fail
	reasons+=("gateway p99 processing ${max_gateway_p99} ms >= 50 ms")
fi
if [ "$wal_subjects" -lt 2 ] || [ "$wal_shards" -lt 2 ]; then
	status=fail
	reasons+=("sharding did not spread: ${wal_subjects} subjects / ${wal_shards} shards")
fi
reason_text=""
if [ ${#reasons[@]} -gt 0 ]; then
	reason_text=$(printf '%s; ' "${reasons[@]}")
	reason_text=${reason_text%; }
fi

jq -n \
	--arg status "$status" \
	--arg t1_status "$t1_status" \
	--arg duration "$DURATION" \
	--argjson rate "$RATE" \
	--argjson batch "$BATCH" \
	--argjson runs "$RUNS" \
	--argjson shards "$SHARDS" \
	--arg linger "$LINGER" \
	--arg mix "$MIX" \
	--arg compression "$COMPRESSION" \
	--argjson sent "$cumulative_accepted" \
	--argjson wal_events "$wal_events" \
	--argjson wal_unique "$wal_unique" \
	--argjson wal_duplicates "$wal_duplicates" \
	--argjson wal_compression "$wal_compression" \
	--argjson wal_subjects "$wal_subjects" \
	--argjson wal_shards "$wal_shards" \
	--argjson max_subject_share "$max_subject_share" \
	--argjson max_gateway_p99 "$max_gateway_p99" \
	--argjson max_client_p99 "$max_client_p99" \
	--argjson failed "$total_failed" \
	--argjson rejected "$total_rejected" \
	--argjson run_reports "$run_reports_json" \
	--arg reason "$reason_text" \
	'{
		story: "4.3",
		status: $status,
		t1_status: $t1_status,
		t1_note: "AC1 requires 1M/s for one hour on the T1 fleet; P-07 and the T1 baseline remain pending. This local run is a proxy, not G1 evidence.",
		duration: $duration,
		rate: $rate,
		batch_size: $batch,
		runs: $runs,
		shards: $shards,
		batch_linger: $linger,
		mix: $mix,
		compression: $compression,
		accepted_events: $sent,
		wal_events: $wal_events,
		unique_event_ids: $wal_unique,
		duplicate_event_ids: $wal_duplicates,
		wal_compression_ratio: (($wal_compression * 100 | round) / 100),
		wal_subjects: $wal_subjects,
		wal_shards_used: $wal_shards,
		max_subject_share_percent: (($max_subject_share * 100 | round) / 100),
		max_gateway_p99_ms: (($max_gateway_p99 * 100 | round) / 100),
		max_client_p99_ms: (($max_client_p99 * 100 | round) / 100),
		failed_events: $failed,
		rejected_events: $rejected,
		reason: $reason,
		runs_detail: $run_reports
	}' >"$RESULTS_DIR/report.json"

cat >"$RESULTS_DIR/report.md" <<EOF
# Story 4.3 — gateway WAL (local)

- **Status:** ${status}
- **T1 target:** ${t1_status} (1M/s for 1h; P-07 and the T1 baseline remain pending)
- **Duration per run:** ${DURATION}
- **Offered rate:** ${RATE} events/s total across two tenants
- **Batch:** ${BATCH} events/request; linger ${LINGER}
- **Runs:** ${RUNS}
- **Shards per tenant/project/epoch/signal:** ${SHARDS}
- **Mix:** ${MIX}
- **Compression:** ${COMPRESSION}

| Metric | Value |
|---|---:|
| Accepted events | ${cumulative_accepted} |
| WAL events | ${wal_events} |
| Unique event IDs | ${wal_unique} |
| Duplicate event IDs | ${wal_duplicates} |
| WAL compression ratio | ${wal_compression_display}x |
| Subjects | ${wal_subjects} |
| Shards used | ${wal_shards} |
| Largest subject share | ${max_subject_share_display}% |
| Gateway p99 processing | ${max_gateway_p99_display} ms (AC1 target < 50 ms) |
| Client p99 | ${max_client_p99_display} ms |
| Failed / rejected events | ${total_failed} / ${total_rejected} |

${reason_text:+**Failure:** ${reason_text}}

The gateway acknowledged only after the JetStream publish ack. \`walcheck\`
read every message back without acking and confirmed the event count, unique
\`event_id\` values and shard distribution. This is a local proxy: it does not
prove the T1 throughput target or a 15-minute ClickHouse outage.
EOF

cat "$RESULTS_DIR/report.md"
if [ "$status" != pass ]; then
	exit 1
fi
log "gateway WAL: pass"
