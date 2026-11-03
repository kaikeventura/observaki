#!/usr/bin/env bash
# Run the local CI suite and smoke-test the built binary.
set -euo pipefail
cd "$(dirname "$0")/.."

# In GitHub Actions the quality job already ran `make ci`.
if [ "${SMOKE_SKIP_CI:-0}" != "1" ]; then
	make ci
fi

find_free_port() {
	local start="${1:-18080}"
	local end="${2:-18120}"
	local port
	for port in $(seq "$start" "$end"); do
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
gateway_log="$(mktemp -t observaki-gateway-smoke.XXXXXX.log)"
pid=""
gateway_pid=""

cleanup() {
	if [ -n "$pid" ]; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	if [ -n "$gateway_pid" ]; then
		kill "$gateway_pid" 2>/dev/null || true
		wait "$gateway_pid" 2>/dev/null || true
	fi
	rm -f "$log" "$gateway_log"
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

gateway_grpc_port="${SMOKE_GATEWAY_GRPC_PORT:-$(find_free_port 18121 18160)}"
gateway_http_port="${SMOKE_GATEWAY_HTTP_PORT:-$(find_free_port 18161 18200)}"

echo "Smoke: starting ./bin/observaki gateway on grpc=127.0.0.1:${gateway_grpc_port} http=127.0.0.1:${gateway_http_port}"
OBSERVAKI_OTLP_GRPC_ADDR="127.0.0.1:${gateway_grpc_port}" \
	OBSERVAKI_OTLP_HTTP_ADDR="127.0.0.1:${gateway_http_port}" \
	./bin/observaki gateway >"$gateway_log" 2>&1 &
gateway_pid=$!

gateway_ready=""
for _ in $(seq 1 50); do
	if curl -fsS "http://127.0.0.1:${gateway_http_port}/readyz" >/dev/null 2>&1; then
		gateway_ready="yes"
		break
	fi
	sleep 0.2
done
if [ -z "$gateway_ready" ]; then
	echo "Smoke: gateway /readyz did not respond; gateway log:" >&2
	cat "$gateway_log" >&2
	exit 1
fi

./bin/otlploadgen --protocol http --endpoint "http://127.0.0.1:${gateway_http_port}" \
	--rate 60 --duration 1s --batch-size 3 --workers 1 --mix 34,33,33 --compression gzip \
	--metrics-addr "" >/dev/null
./bin/otlploadgen --protocol grpc --endpoint "127.0.0.1:${gateway_grpc_port}" \
	--rate 60 --duration 1s --batch-size 3 --workers 1 --mix 34,33,33 --compression gzip \
	--metrics-addr "" >/dev/null

gateway_metrics="$(curl -fsS "http://127.0.0.1:${gateway_http_port}/metrics")"
for metric in observaki_gateway_requests_total observaki_gateway_events_total observaki_gateway_up; do
	case "$gateway_metrics" in
	*"$metric"*) ;;
	*)
		echo "Smoke: gateway /metrics does not expose ${metric}" >&2
		exit 1
		;;
	esac
done

kill -TERM "$gateway_pid"
if ! wait "$gateway_pid"; then
	echo "Smoke: gateway did not exit cleanly after SIGTERM; gateway log:" >&2
	cat "$gateway_log" >&2
	exit 1
fi
gateway_pid=""

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

./bin/chloadgen --version >/dev/null

./bin/vmloadgen --version >/dev/null
./bin/vmquerybench --version >/dev/null
./bin/vmadmission --version >/dev/null

./bin/benchctl run throughput-100k --plan --results-dir "$bench_tmp/results" >/dev/null
./bin/benchctl report --results-dir "$bench_tmp/results" --out "$bench_tmp/report" >/dev/null
for artifact in report.md report.json; do
	[ -f "$bench_tmp/report/$artifact" ] || {
		echo "Smoke: benchmark $artifact is missing" >&2
		exit 1
	}
done

echo "Smoke: OK (healthz, api health, gateway OTLP HTTP/gRPC, gateway metrics/SIGTERM, web build, version, loadgen payload, chloadgen, bench scenarios, benchmark report)"
