#!/usr/bin/env bash
# Run the local CI suite and smoke-test the built binary.
set -euo pipefail
cd "$(dirname "$0")/.."

# In GitHub Actions the quality job already ran `make ci`.
if [ "${SMOKE_SKIP_CI:-0}" != "1" ]; then
	make ci
fi

find_free_port() {
	local port
	for port in $(seq 18080 18120); do
		if ! (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
			echo "$port"
			return 0
		fi
	done
	return 1
}

port="${SMOKE_PORT:-$(find_free_port)}"
log="$(mktemp -t observaki-smoke.XXXXXX.log)"
bench_tmp="$(mktemp -d -t observaki-bench.XXXXXX)"
pid=""

cleanup() {
	if [ -n "$pid" ]; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	rm -f "$log"
	rm -rf "$bench_tmp"
}
trap cleanup EXIT

echo "Smoke: starting ./bin/observaki serve on 127.0.0.1:${port}"
OBSERVAKI_HTTP_ADDR="127.0.0.1:${port}" ./bin/observaki serve >"$log" 2>&1 &
pid=$!

ready=""
for _ in $(seq 1 50); do
	if curl -fsS "http://127.0.0.1:${port}/healthz" >/dev/null 2>&1; then
		ready="yes"
		break
	fi
	sleep 0.2
done

if [ -z "$ready" ]; then
	echo "Smoke: /healthz did not respond; server log:" >&2
	cat "$log" >&2
	exit 1
fi

healthz="$(curl -fsS "http://127.0.0.1:${port}/healthz")"
[ "$healthz" = '{"status":"ok"}' ] || {
	echo "Smoke: unexpected /healthz body: ${healthz}" >&2
	exit 1
}

health="$(curl -fsS "http://127.0.0.1:${port}/api/v1/health")"
case "$health" in
*'"status":"ok"'*) ;;
*)
	echo "Smoke: unexpected /api/v1/health body: ${health}" >&2
	exit 1
	;;
esac

[ -f web/build/index.html ] || {
	echo "Smoke: web/build/index.html is missing" >&2
	exit 1
}
grep -q "Observaki" web/build/index.html || {
	echo "Smoke: web/build/index.html does not contain the app markup" >&2
	exit 1
}

./bin/observaki version

./bin/otlploadgen --check-payload >/dev/null

./bin/benchctl check >/dev/null

./bin/benchctl run throughput-100k --plan --results-dir "$bench_tmp/results" >/dev/null
./bin/benchctl report --results-dir "$bench_tmp/results" --out "$bench_tmp/report" >/dev/null
for artifact in report.md report.json; do
	[ -f "$bench_tmp/report/$artifact" ] || {
		echo "Smoke: benchmark $artifact is missing" >&2
		exit 1
	}
done

echo "Smoke: OK (healthz, api health, web build, version, loadgen payload, bench scenarios, benchmark report)"
