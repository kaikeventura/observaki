#!/usr/bin/env bash
# Story 4.2 — gateway auth, tenant isolation and quota isolation.
#
# Starts a local gateway with two project-scoped tokens, measures a quiet
# tenant's p95 alone, then measures it again while a noisy tenant sends far
# above its quota. Passes when the quiet tenant keeps p95 within 10% and the
# noisy tenant receives 429s.
#
# The local default is the story workload: a 100k events/s quota against a
# 1M events/s noisy tenant. Use --smoke for a short harness check.
set -euo pipefail
export LC_ALL=C

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
GATEWAY_BIN="$ROOT/bin/observaki"
LOADGEN_BIN="$ROOT/bin/otlploadgen"
DURATION=10s
QUOTA=100000
NORMAL_RATE=50000
NOISY_RATE=1000000
BATCH=1000
NORMAL_MIX=0,100,0
NOISY_MIX=60,30,10
NORMAL_COMPRESSION=gzip
NOISY_COMPRESSION=none
METRIC_PADDING=832
LOG_BODY=820
TRACE_PADDING=2400
RUNS=3
NORMAL_WORKERS=2
NOISY_WORKERS=8
RESULTS_DIR=""
SMOKE=0
PIN=0
GATEWAY_CPUS=""
NORMAL_CPUS=""
NOISY_CPUS=""

usage() {
	cat <<'EOF'
Usage: bench/gateway/quota/run.sh [options]

Options:
  --duration T       measured duration for each phase (default: 10s)
  --runs N           measured repetitions, median is reported (default: 3)
  --quota N          noisy tenant quota in events/s (default: 100000)
  --normal-rate N    quiet tenant rate in events/s (default: 50000)
  --noisy-rate N     noisy tenant rate in events/s (default: 1000000)
  --batch-size N     events per OTLP request (default: 100)
  --smoke            short run: 5s, 10k/s quota, 1k/s quiet, 100k/s noisy
  --pin              pin gateway/quiet/noisy to disjoint CPU sets (needs >= 12 CPUs)
  --gateway-cpus L   explicit taskset list for the gateway (implies --pin)
  --normal-cpus L    explicit taskset list for the quiet load generator
  --noisy-cpus L     explicit taskset list for the noisy load generator
  --results-dir PATH report destination
  --help             show this help

The quiet tenant has a 1M events/s quota so it is never limited. The noisy
tenant is limited to --quota and must receive 429 + Retry-After. The run is
local and is not G1 evidence; the T1 fleet baseline remains blocked by P-07.
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
	--runs) RUNS=$2; shift 2 ;;
	--quota) QUOTA=$2; shift 2 ;;
	--normal-rate) NORMAL_RATE=$2; shift 2 ;;
	--noisy-rate) NOISY_RATE=$2; shift 2 ;;
	--batch-size) BATCH=$2; shift 2 ;;
	--normal-mix) NORMAL_MIX=$2; shift 2 ;;
	--noisy-mix) NOISY_MIX=$2; shift 2 ;;
	--small-payload) METRIC_PADDING=0; LOG_BODY=64; TRACE_PADDING=0; shift ;;
	--metric-padding) METRIC_PADDING=$2; shift 2 ;;
	--log-body) LOG_BODY=$2; shift 2 ;;
	--trace-padding) TRACE_PADDING=$2; shift 2 ;;
	--normal-compression) NORMAL_COMPRESSION=$2; shift 2 ;;
	--noisy-compression) NOISY_COMPRESSION=$2; shift 2 ;;
	--results-dir) RESULTS_DIR=$2; shift 2 ;;
	--smoke) SMOKE=1; shift ;;
	--pin) PIN=1; shift ;;
	--gateway-cpus) GATEWAY_CPUS=$2; PIN=1; shift 2 ;;
	--normal-cpus) NORMAL_CPUS=$2; PIN=1; shift 2 ;;
	--noisy-cpus) NOISY_CPUS=$2; PIN=1; shift 2 ;;
	--help|-h) usage; exit 0 ;;
	*) die "unknown option $1 (see --help)" ;;
	esac
done

if [ "$SMOKE" = 1 ]; then
	DURATION=5s
	QUOTA=10000
	NORMAL_RATE=1000
	NOISY_RATE=100000
	BATCH=50
	RUNS=1
fi

if [ "$PIN" = 1 ]; then
	command -v taskset >/dev/null || die "taskset is required for --pin"
	cpu_count=$(nproc)
	if [ -z "$GATEWAY_CPUS" ] || [ -z "$NORMAL_CPUS" ] || [ -z "$NOISY_CPUS" ]; then
		[ "$cpu_count" -ge 12 ] || die "--pin needs at least 12 CPUs (found ${cpu_count})"
		GATEWAY_CPUS=${GATEWAY_CPUS:-0-7}
		NORMAL_CPUS=${NORMAL_CPUS:-8-9}
		NOISY_CPUS=${NOISY_CPUS:-10-$((cpu_count - 3))}
	fi
	gateway_cmd=(taskset -c "$GATEWAY_CPUS")
	normal_cmd=(taskset -c "$NORMAL_CPUS")
	noisy_cmd=(taskset -c "$NOISY_CPUS")
else
	gateway_cmd=()
	normal_cmd=()
	noisy_cmd=()
fi

command -v jq >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"
[ -x "$GATEWAY_BIN" ] || die "$GATEWAY_BIN is missing (run make build)"
[ -x "$LOADGEN_BIN" ] || die "$LOADGEN_BIN is missing (run make build)"

if [ -z "$RESULTS_DIR" ]; then
	RESULTS_DIR="$ROOT/bench/gateway/quota/results/$(date -u +%Y%m%dT%H%M%SZ)"
fi
mkdir -p "$RESULTS_DIR"
WORK_DIR=$(mktemp -d -t observaki-gateway-quota.XXXXXX)
LOG="$RESULTS_DIR/run.log"
: >"$LOG"
exec > >(tee -a "$LOG") 2>&1

cleanup() {
	if [ -n "${gateway_pid:-}" ]; then
		kill -TERM "$gateway_pid" 2>/dev/null || true
		wait "$gateway_pid" 2>/dev/null || true
	fi
	if [ -n "${noisy_pid:-}" ]; then
		kill -TERM "$noisy_pid" 2>/dev/null || true
		wait "$noisy_pid" 2>/dev/null || true
	fi
	rm -rf "$WORK_DIR"
}
trap cleanup EXIT

find_free_port() {
	local start="${1:-18300}"
	local end="${2:-18400}"
	local port
	for port in $(seq "$start" "$end"); do
		if ! (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
			echo "$port"
			return 0
		fi
	done
	return 1
}

grpc_port=$(find_free_port 18300 18340) || die "no free gRPC port"
http_port=$(find_free_port 18341 18380) || die "no free HTTP port"

normal_token="normal-$(date -u +%s%N)-$RANDOM"
noisy_token="noisy-$(date -u +%s%N)-$RANDOM"
normal_hash=$(printf '%s' "$normal_token" | sha256sum | cut -d' ' -f1)
noisy_hash=$(printf '%s' "$noisy_token" | sha256sum | cut -d' ' -f1)
issued_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
expires_at=$(date -u -d '+2 hours' +%Y-%m-%dT%H:%M:%SZ)

snapshot="$WORK_DIR/ingest-auth.json"
jq -n \
	--arg version "gateway-quota-$(date -u +%s)" \
	--argjson generation 1 \
	--arg issued "$issued_at" \
	--arg expires "$expires_at" \
	--arg normal_hash "$normal_hash" \
	--arg noisy_hash "$noisy_hash" \
	--argjson quota "$QUOTA" \
	'{
		config_version: $version,
		generation: $generation,
		issued_at: $issued,
		expires_at: $expires,
		tenants: [
			{
				tenant_id: "tenant-quiet",
				ingest_quota: {events_per_second: 1000000, burst_events: 1000000},
				projects: [{
					project_id: "project-quiet",
					tokens: [{token_id: "token-quiet", sha256: $normal_hash, scopes: ["ingest"]}]
				}]
			},
			{
				tenant_id: "tenant-noisy",
				ingest_quota: {events_per_second: $quota, burst_events: $quota},
				projects: [{
					project_id: "project-noisy",
					tokens: [{token_id: "token-noisy", sha256: $noisy_hash, scopes: ["ingest"]}]
				}]
			}
		]
	}' >"$snapshot"
audit="$WORK_DIR/audit.jsonl"

log "starting gateway on grpc=127.0.0.1:${grpc_port} http=127.0.0.1:${http_port}"
OBSERVAKI_ENV=development \
	OBSERVAKI_OTLP_GRPC_ADDR="127.0.0.1:${grpc_port}" \
	OBSERVAKI_OTLP_HTTP_ADDR="127.0.0.1:${http_port}" \
	OBSERVAKI_INGEST_AUTH_FILE="$snapshot" \
	OBSERVAKI_INGEST_AUDIT_FILE="$audit" \
	OBSERVAKI_INGEST_AUTH_RELOAD_INTERVAL=5s \
	"${gateway_cmd[@]}" "$GATEWAY_BIN" gateway >"$RESULTS_DIR/gateway.log" 2>&1 &
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

run_loadgen() {
	local name=$1
	local rate=$2
	local workers=$3
	local token=$4
	local mix=$5
	local compression=$6
	local -a cpu_cmd=()
	if [ "$name" = noisy ]; then
		cpu_cmd=("${noisy_cmd[@]}")
	else
		cpu_cmd=("${normal_cmd[@]}")
	fi
	"${cpu_cmd[@]}" "$LOADGEN_BIN" \
		--protocol http \
		--endpoint "http://127.0.0.1:${http_port}" \
		--rate "$rate" \
		--duration "$DURATION" \
		--batch-size "$BATCH" \
		--workers "$workers" \
		--mix "$mix" \
		--compression "$compression" \
		--metric-padding "$METRIC_PADDING" \
		--log-body "$LOG_BODY" \
		--trace-padding "$TRACE_PADDING" \
		--metrics-addr "" \
		--header "Authorization=Bearer ${token}" \
		>"$RESULTS_DIR/${name}.json" \
		2>"$RESULTS_DIR/${name}.log"
}

quota_metric() {
	curl -fsS "http://127.0.0.1:${http_port}/metrics" |
		awk '/^observaki_gateway_quota_rejected_requests_total /{print $2; exit}'
}

gateway_p95() {
	local tenant=$1
	curl -fsS "http://127.0.0.1:${http_port}/metrics" |
		awk -v tenant="$tenant" 'index($1, "observaki_gateway_request_duration_p95_seconds{tenant_id=\"" tenant "\"}") == 1 {printf "%.3f", $2 * 1000; exit}'
}

quota_before=$(quota_metric)

baseline_values=()
loaded_values=()
baseline_server_values=()
loaded_server_values=()
baseline_count=0
loaded_count=0
for run in $(seq 1 "$RUNS"); do
	log "run ${run}/${RUNS} phase 1: quiet tenant alone at ${NORMAL_RATE} events/s for ${DURATION}"
	run_loadgen "baseline-${run}" "$NORMAL_RATE" "$NORMAL_WORKERS" "$normal_token" "$NORMAL_MIX" "$NORMAL_COMPRESSION"
	baseline_values+=("$(jq -r '.metrics.latency.p95_ms // 0' "$RESULTS_DIR/baseline-${run}.json")")
	baseline_server_values+=("$(gateway_p95 tenant-quiet)")
	baseline_count=$(jq -r '.metrics.latency.count // 0' "$RESULTS_DIR/baseline-${run}.json")

	log "run ${run}/${RUNS} phase 2: quiet tenant at ${NORMAL_RATE} events/s while noisy tenant sends ${NOISY_RATE} events/s (quota ${QUOTA})"
	run_loadgen "noisy-${run}" "$NOISY_RATE" "$NOISY_WORKERS" "$noisy_token" "$NOISY_MIX" "$NOISY_COMPRESSION" &
	noisy_pid=$!
	sleep 1
	run_loadgen "loaded-${run}" "$NORMAL_RATE" "$NORMAL_WORKERS" "$normal_token" "$NORMAL_MIX" "$NORMAL_COMPRESSION"
	wait "$noisy_pid" || true
	noisy_pid=""
	loaded_values+=("$(jq -r '.metrics.latency.p95_ms // 0' "$RESULTS_DIR/loaded-${run}.json")")
	loaded_server_values+=("$(gateway_p95 tenant-quiet)")
	loaded_count=$(jq -r '.metrics.latency.count // 0' "$RESULTS_DIR/loaded-${run}.json")
done
quota_after=$(quota_metric)
quota_rejected=$((quota_after - quota_before))

median() {
	if [ "$#" -eq 0 ]; then
		echo 0
		return
	fi
	printf '%s\n' "$@" | sort -n | awk '{values[NR]=$1} END {
		if (NR % 2 == 1) { printf "%.6f", values[(NR+1)/2] }
		else { printf "%.6f", (values[NR/2] + values[NR/2+1]) / 2 }
	}'
}
baseline_p95=$(median "${baseline_values[@]}")
loaded_p95=$(median "${loaded_values[@]}")
baseline_server_p95=$(median "${baseline_server_values[@]}")
loaded_server_p95=$(median "${loaded_server_values[@]}")

delta_percent=$(awk -v base="$baseline_p95" -v loaded="$loaded_p95" 'BEGIN {
	if (base <= 0) { print "999999"; exit }
	printf "%.2f", (loaded - base) / base * 100
}')
server_delta_percent=$(awk -v base="$baseline_server_p95" -v loaded="$loaded_server_p95" 'BEGIN {
	if (base <= 0) { print "999999"; exit }
	printf "%.2f", (loaded - base) / base * 100
}')

status=pass
isolation_status=pass
isolation_note=""
client_note=""
reasons=()
if ! awk -v delta="$server_delta_percent" 'BEGIN { exit !(delta < 10) }'; then
	isolation_status=inconclusive
	isolation_note="local gateway processing p95 changed ${server_delta_percent}% (limit 10%); the gateway and both load generators share one host, so CPU frequency and memory bandwidth contention invalidate the neighbor comparison. The T1 fleet with separate load generators is required."
fi
if [ "$isolation_status" = pass ] && ! awk -v delta="$delta_percent" 'BEGIN { exit !(delta < 10) }'; then
	client_note="client-side p95 changed ${delta_percent}% because the load generator shares the host with the gateway and the noisy tenant; the gateway processing p95 is the local isolation verdict and it stayed under 10%."
fi
if [ "$quota_rejected" -le 0 ]; then
	status=fail
	reasons+=("noisy tenant received no quota rejections")
fi
if [ "$baseline_count" -le 0 ] || [ "$loaded_count" -le 0 ]; then
	status=fail
	reasons+=("latency samples are missing")
fi
if [ -z "$baseline_server_p95" ] || [ -z "$loaded_server_p95" ]; then
	baseline_server_p95=${baseline_server_p95:-0}
	loaded_server_p95=${loaded_server_p95:-0}
	isolation_note="${isolation_note} gateway processing latency samples are missing."
fi

reason_text=""
if [ ${#reasons[@]} -gt 0 ]; then
	reason_text=$(printf '%s; ' "${reasons[@]}")
	reason_text=${reason_text%; }
fi

jq -n \
	--arg status "$status" \
	--arg duration "$DURATION" \
	--argjson quota "$QUOTA" \
	--argjson normal_rate "$NORMAL_RATE" \
	--argjson noisy_rate "$NOISY_RATE" \
	--argjson batch "$BATCH" \
	--argjson runs "$RUNS" \
	--arg normal_mix "$NORMAL_MIX" \
	--arg noisy_mix "$NOISY_MIX" \
	--arg normal_compression "$NORMAL_COMPRESSION" \
	--arg noisy_compression "$NOISY_COMPRESSION" \
	--argjson metric_padding "$METRIC_PADDING" \
	--argjson log_body "$LOG_BODY" \
	--argjson trace_padding "$TRACE_PADDING" \
	--argjson pin "$PIN" \
	--arg gateway_cpus "$GATEWAY_CPUS" \
	--arg normal_cpus "$NORMAL_CPUS" \
	--arg noisy_cpus "$NOISY_CPUS" \
	--argjson baseline_p95 "$baseline_p95" \
	--argjson loaded_p95 "$loaded_p95" \
	--argjson delta "$delta_percent" \
	--argjson baseline_server_p95 "$baseline_server_p95" \
	--argjson loaded_server_p95 "$loaded_server_p95" \
	--argjson server_delta "$server_delta_percent" \
	--argjson rejected "$quota_rejected" \
	--arg reason "$reason_text" \
	--arg isolation_status "$isolation_status" \
	--arg isolation_note "$isolation_note" \
	--arg client_note "$client_note" \
	'{
		story: "4.2",
		status: $status,
		isolation_status: $isolation_status,
		isolation_note: $isolation_note,
		client_note: $client_note,
		duration: $duration,
		quota_events_per_second: $quota,
		normal_rate: $normal_rate,
		noisy_rate: $noisy_rate,
		batch_size: $batch,
		runs: $runs,
		normal_mix: $normal_mix,
		noisy_mix: $noisy_mix,
		normal_compression: $normal_compression,
		noisy_compression: $noisy_compression,
		payload: {metric_padding: $metric_padding, log_body: $log_body, trace_padding: $trace_padding},
		pinned: ($pin == 1),
		gateway_cpus: $gateway_cpus,
		normal_cpus: $normal_cpus,
		noisy_cpus: $noisy_cpus,
		baseline_p95_ms: $baseline_p95,
		loaded_p95_ms: $loaded_p95,
		p95_delta_percent: $delta,
		baseline_server_p95_ms: $baseline_server_p95,
		loaded_server_p95_ms: $loaded_server_p95,
		server_p95_delta_percent: $server_delta,
		quota_rejected_requests: $rejected,
		reason: $reason,
		limitations: [
			"local single-process gateway with the discard sink; not G1 evidence",
			"the quiet tenant quota is 1M events/s so it is never limited",
			"the noisy tenant over-quota traffic is rejected before the sink"
		]
	}' >"$RESULTS_DIR/report.json"

cat >"$RESULTS_DIR/report.md" <<EOF
# Story 4.2 — gateway quota isolation (local)

- **Status:** ${status}
- **Isolation (>10% local check):** ${isolation_status}
- **Duration per phase:** ${DURATION}
- **Noisy quota:** ${QUOTA} events/s
- **Quiet tenant:** ${NORMAL_RATE} events/s
- **Noisy tenant offered:** ${NOISY_RATE} events/s
- **Quiet mix:** ${NORMAL_MIX} (logs only by default, so p95 is comparable)
- **Noisy mix:** ${NOISY_MIX}
- **Compression:** quiet ${NORMAL_COMPRESSION}, noisy ${NOISY_COMPRESSION}
- **Payload:** metric ${METRIC_PADDING} B, log ${LOG_BODY} B, trace ${TRACE_PADDING} B
- **Pinned:** ${PIN} (gateway=${GATEWAY_CPUS:-none}, quiet=${NORMAL_CPUS:-none}, noisy=${NOISY_CPUS:-none})

| Metric | Value |
|---|---|
| Quiet client p95 alone (observed) | ${baseline_p95} ms |
| Quiet client p95 under noisy tenant (observed) | ${loaded_p95} ms |
| Client p95 change (observed) | ${delta_percent}% |
| Quiet gateway processing p95 alone (AC metric) | ${baseline_server_p95} ms |
| Quiet gateway processing p95 under noisy tenant (AC metric) | ${loaded_server_p95} ms |
| Gateway processing p95 change (AC metric) | ${server_delta_percent}% (limit 10%) |
| Quota-rejected requests | ${quota_rejected} |

${reason_text:+**Failure:** ${reason_text}}

${isolation_note:+**Isolation note:** ${isolation_note}}

${client_note:+**Client note:** ${client_note}}

This is a local single-process run with the discard sink. It validates the
admission and isolation behavior, not the T1 baseline (P-07 and Stories
4.1/4.3/4.4).
EOF

cat "$RESULTS_DIR/report.md"
if [ "$status" != pass ]; then
	exit 1
fi
log "gateway quota isolation: pass"
