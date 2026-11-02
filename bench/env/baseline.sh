#!/usr/bin/env bash
# Run the published T1 baseline protocol: warm-up, ramp and a measured run.
#
# The warm-up and ramp are recorded in the result folder for transparency but
# are not part of the go/no-go number. The measured run is the last phase.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BENCHCTL="$ROOT/bin/benchctl"
SCENARIO="${OBSERVAKI_BENCH_BASELINE_SCENARIO:-throughput-1m}"
ENDPOINT="${OBSERVAKI_BENCH_ENDPOINT:-http://127.0.0.1:4318}"
RESULTS="${OBSERVAKI_BENCH_RESULTS:-}"
DURATION="1h"
WARMUP="10m"
RAMP_STEP="1m"
RAMP_STEPS=5
RATE=""
DRY_RUN=0
PLAN=0

usage() {
	cat <<'EOF'
baseline.sh — run the T1 benchmark baseline protocol

Usage:
  ./bench/env/baseline.sh [flags]

Flags:
  --scenario NAME     throughput scenario (default: throughput-1m)
  --endpoint URL      OTLP endpoint (default: $OBSERVAKI_BENCH_ENDPOINT or http://127.0.0.1:4318)
  --results-dir DIR   results root (default: bench/results/<UTC>-baseline-<scenario>)
  --duration DURATION measured run (default: 1h)
  --warmup DURATION   warm-up at full rate, not measured (default: 10m)
  --ramp-step DUR    duration of each ramp step (default: 1m)
  --ramp-steps N      number of ramp steps, 1-5 (default: 5)
  --rate N            target events/s (default: the scenario's SCENARIO_RATE)
  --dry-run           run the load generator without sending traffic
  --plan              print the protocol without executing it
  -h, --help          show this help

Protocol:
  warm-up  full rate for the warm-up duration
  ramp     10%, 25%, 50%, 75%, 100% of the target for one step each
  measured full rate for the measured duration

The report includes every phase. The measured run is the one with the full
duration; the warm-up and ramp are excluded from the gate number.
EOF
}

die() {
	echo "baseline: $*" >&2
	exit 1
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--scenario)
		[ "$#" -ge 2 ] || die "--scenario requires a value"
		SCENARIO="$2"
		shift 2
		;;
	--endpoint)
		[ "$#" -ge 2 ] || die "--endpoint requires a value"
		ENDPOINT="$2"
		shift 2
		;;
	--results-dir)
		[ "$#" -ge 2 ] || die "--results-dir requires a value"
		RESULTS="$2"
		shift 2
		;;
	--duration)
		[ "$#" -ge 2 ] || die "--duration requires a value"
		DURATION="$2"
		shift 2
		;;
	--warmup)
		[ "$#" -ge 2 ] || die "--warmup requires a value"
		WARMUP="$2"
		shift 2
		;;
	--ramp-step)
		[ "$#" -ge 2 ] || die "--ramp-step requires a value"
		RAMP_STEP="$2"
		shift 2
		;;
	--ramp-steps)
		[ "$#" -ge 2 ] || die "--ramp-steps requires a value"
		RAMP_STEPS="$2"
		shift 2
		;;
	--rate)
		[ "$#" -ge 2 ] || die "--rate requires a value"
		RATE="$2"
		shift 2
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	--plan)
		PLAN=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		die "unknown flag: $1"
		;;
	esac
done

[ -x "$BENCHCTL" ] || die "benchctl not found; run make build"

scenario_file="$ROOT/bench/scenarios/$SCENARIO.env"
[ -f "$scenario_file" ] || die "unknown scenario: $SCENARIO"
kind="$(sed -n 's/^SCENARIO_KIND=//p' "$scenario_file")"
[ "$kind" = "throughput" ] || die "baseline.sh supports throughput scenarios only, got ${kind:-unknown}"

if [ -z "$RATE" ]; then
	RATE="$(sed -n 's/^SCENARIO_RATE=//p' "$scenario_file")"
fi
case "$RATE" in
'' | 0 | *[!0-9]*) die "scenario has no valid SCENARIO_RATE; pass --rate" ;;
esac

case "$RAMP_STEPS" in
1) percentages=(100) ;;
2) percentages=(50 100) ;;
3) percentages=(33 66 100) ;;
4) percentages=(25 50 75 100) ;;
5) percentages=(10 25 50 75 100) ;;
*) die "--ramp-steps must be between 1 and 5" ;;
esac

if [ -z "$RESULTS" ]; then
	RESULTS="$ROOT/bench/results/$(date -u +%Y%m%dT%H%M%SZ)-baseline-$SCENARIO"
fi
REPORT_DIR="$ROOT/bench/reports/$(date -u +%Y%m%dT%H%M%SZ)-baseline-$SCENARIO"

run_phase() {
	local label="$1"
	local rate="$2"
	local duration="$3"
	local args=(run "$SCENARIO" --duration "$duration" --rate "$rate" --results-dir "$RESULTS")
	if [ "$DRY_RUN" = "1" ]; then
		args+=(--dry-run)
	fi
	if [ -n "$ENDPOINT" ]; then
		args+=(--endpoint "$ENDPOINT")
	fi
	echo "==> ${label}: ${rate} events/s for ${duration}"
	"$BENCHCTL" "${args[@]}"
}

if [ "$PLAN" = "1" ]; then
	echo "scenario: $SCENARIO (${RATE} events/s)"
	echo "results:  $RESULTS"
	if [ "$DRY_RUN" = "1" ]; then
		echo "mode:     dry-run (no traffic)"
	else
		echo "mode:     run against $ENDPOINT"
	fi
	echo "warm-up:  ${RATE} events/s for ${WARMUP} (not measured)"
	echo "ramp:     ${percentages[*]}% for ${RAMP_STEP} each (not measured)"
	echo "measured: ${RATE} events/s for ${DURATION}"
	echo "report:   $REPORT_DIR"
	exit 0
fi

run_phase "warm-up" "$RATE" "$WARMUP"
for percentage in "${percentages[@]}"; do
	rate=$((RATE * percentage / 100))
	[ "$rate" -gt 0 ] || rate=1
	run_phase "ramp ${percentage}%" "$rate" "$RAMP_STEP"
done
run_phase "measured" "$RATE" "$DURATION"

"$BENCHCTL" report --results-dir "$RESULTS" --scenario "$SCENARIO" --out "$REPORT_DIR"
echo "baseline report: $REPORT_DIR/report.md"
echo "baseline json:   $REPORT_DIR/report.json"
echo "measured run:    the last run with ${DURATION} at ${RATE} events/s"
