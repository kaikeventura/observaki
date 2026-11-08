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
auth_dir="$(mktemp -d -t observaki-auth.XXXXXX)"
pid=""
gateway_pid=""
nats_container=""
api_body=""

cleanup() {
	if [ -n "$pid" ]; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	if [ -n "$gateway_pid" ]; then
		kill "$gateway_pid" 2>/dev/null || true
		wait "$gateway_pid" 2>/dev/null || true
	fi
	if [ -n "$nats_container" ]; then
		docker rm -f "$nats_container" >/dev/null 2>&1 || true
	fi
	rm -f "$log" "$gateway_log" "$api_body"
	rm -rf "$bench_tmp" "$auth_dir" 2>/dev/null || true
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

api_body="$(mktemp -t observaki-api-smoke.XXXXXX.json)"

health="$(curl -fsS "http://127.0.0.1:${port}/api/v1/health")"
case "$health" in
*'"status":"ok"'*) ;;
*)
	echo "Smoke: unexpected /api/v1/health body: ${health}" >&2
	exit 1
	;;
esac

# Story 5.5 AC1: the generated contract is served and documents the MVP routes.
openapi="$(curl -fsS "http://127.0.0.1:${port}/openapi.json")"
case "$openapi" in
*'"openapi": "3.1.0"'*) ;;
*)
	echo "Smoke: /openapi.json is not an OpenAPI 3.1 document" >&2
	exit 1
	;;
esac
for route in '/api/v1/projects' '/api/v1/projects/{projectID}/ingest-tokens' '/api/v1/query/sample' '/api/v1/health/overview'; do
	case "$openapi" in
	*"\"${route}\""*) ;;
	*)
		echo "Smoke: /openapi.json does not document ${route}" >&2
		exit 1
		;;
	esac
done

# Story 5.5 AC2: invalid/unauthorized input returns the AD-15 envelope with a
# request id, a stable code and no stack trace.
api_status="$(curl -s -o "$api_body" -w '%{http_code}' "http://127.0.0.1:${port}/api/v1/projects")"
[ "$api_status" = "401" ] || {
	echo "Smoke: unauthenticated /api/v1/projects returned ${api_status}, want 401" >&2
	exit 1
}
grep -q '"code":"unauthorized"' "$api_body" || {
	echo "Smoke: authorization error is not structured: $(cat "$api_body")" >&2
	exit 1
}
grep -q '"request_id":"' "$api_body" || {
	echo "Smoke: authorization error has no request_id: $(cat "$api_body")" >&2
	exit 1
}
grep -qi 'goroutine\|panic' "$api_body" && {
	echo "Smoke: authorization error leaks internal detail" >&2
	exit 1
}
curl -s -D - -o /dev/null "http://127.0.0.1:${port}/api/v1/projects" | grep -qi '^x-request-id:' || {
	echo "Smoke: /api/v1/projects did not set X-Request-Id" >&2
	exit 1
}

api_status="$(curl -s -o "$api_body" -w '%{http_code}' -X DELETE "http://127.0.0.1:${port}/api/v1/projects")"
[ "$api_status" = "405" ] || {
	echo "Smoke: DELETE /api/v1/projects returned ${api_status}, want 405" >&2
	exit 1
}
grep -q '"code":"method_not_allowed"' "$api_body" || {
	echo "Smoke: 405 is not structured: $(cat "$api_body")" >&2
	exit 1
}

api_status="$(curl -s -o "$api_body" -w '%{http_code}' "http://127.0.0.1:${port}/api/v1/nope")"
[ "$api_status" = "404" ] || {
	echo "Smoke: unknown API route returned ${api_status}, want 404" >&2
	exit 1
}
grep -q '"code":"not_found"' "$api_body" || {
	echo "Smoke: 404 is not structured: $(cat "$api_body")" >&2
	exit 1
}

gateway_grpc_port="${SMOKE_GATEWAY_GRPC_PORT:-$(find_free_port 18121 18160)}"
gateway_http_port="${SMOKE_GATEWAY_HTTP_PORT:-$(find_free_port 18161 18200)}"

# Story 4.2: a temp snapshot with hashed tokens. The smoke token and the
# limited token are generated per run and never committed.
smoke_token="smoke-$(date -u +%s)-${RANDOM}${RANDOM}"
limited_token="limited-$(date -u +%s)-${RANDOM}${RANDOM}"
smoke_hash=$(printf '%s' "$smoke_token" | sha256sum | cut -d' ' -f1)
limited_hash=$(printf '%s' "$limited_token" | sha256sum | cut -d' ' -f1)
cat >"$auth_dir/ingest-auth.json" <<EOF
{
  "config_version": "smoke-$(date -u +%s)",
  "generation": 1,
  "issued_at": "2026-01-01T00:00:00Z",
  "expires_at": "2030-01-01T00:00:00Z",
  "tenants": [
    {
      "tenant_id": "tenant-smoke",
      "ingest_quota": {"events_per_second": 1000000, "burst_events": 1000000},
      "projects": [{
        "project_id": "project-smoke",
        "tokens": [{"token_id": "token-smoke", "sha256": "${smoke_hash}", "scopes": ["ingest"]}]
      }]
    },
    {
      "tenant_id": "tenant-limited",
      "ingest_quota": {"events_per_second": 1, "burst_events": 1},
      "projects": [{
        "project_id": "project-limited",
        "tokens": [{"token_id": "token-limited", "sha256": "${limited_hash}", "scopes": ["ingest"]}]
      }]
    }
  ]
}
EOF

echo "Smoke: starting ./bin/observaki gateway on grpc=127.0.0.1:${gateway_grpc_port} http=127.0.0.1:${gateway_http_port}"
nats_url="${SMOKE_NATS_URL:-}"
if [ -z "$nats_url" ]; then
	command -v docker >/dev/null || {
		echo "Smoke: docker is required to start NATS (or set SMOKE_NATS_URL to a JetStream server)" >&2
		exit 1
	}
	nats_port="${SMOKE_NATS_PORT:-$(find_free_port 18201 18240)}"
	nats_monitor_port="${SMOKE_NATS_MONITOR_PORT:-$(find_free_port 18241 18280)}"
	mkdir -p "$auth_dir/nats/js"
	cat >"$auth_dir/nats/nats.conf" <<EOF
port: 4222
http_port: 8222
max_payload: 16777216
jetstream {
  store_dir: "/data/js"
}
EOF
	nats_container="observaki-smoke-nats-$$"
	docker run -d --rm --user "$(id -u):$(id -g)" --name "$nats_container" \
		-p "${nats_port}:4222" -p "${nats_monitor_port}:8222" \
		-v "$auth_dir/nats:/data" nats:2.15.2-alpine -c /data/nats.conf >/dev/null
	nats_url="nats://127.0.0.1:${nats_port}"
	nats_ready=""
	for _ in $(seq 1 50); do
		if curl -fsS "http://127.0.0.1:${nats_monitor_port}/varz" >/dev/null 2>&1; then
			nats_ready="yes"
			break
		fi
		sleep 0.2
	done
	[ "$nats_ready" = "yes" ] || {
		echo "Smoke: NATS did not become ready" >&2
		docker logs "$nats_container" >&2 || true
		exit 1
	}
fi

OBSERVAKI_OTLP_GRPC_ADDR="127.0.0.1:${gateway_grpc_port}" \
	OBSERVAKI_OTLP_HTTP_ADDR="127.0.0.1:${gateway_http_port}" \
	OBSERVAKI_INGEST_AUTH_FILE="$auth_dir/ingest-auth.json" \
	OBSERVAKI_INGEST_AUDIT_FILE="$auth_dir/audit.jsonl" \
	OBSERVAKI_NATS_URL="$nats_url" \
	OBSERVAKI_NATS_SHARDS=8 \
	OBSERVAKI_NATS_BATCH_LINGER=2ms \
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

# Story 4.2 AC1/AC2: auth failures never reach the sink, and a cross-project
# assertion is rejected and audited.
unauthorized_status=$(curl -s -o /dev/null -w '%{http_code}' -X POST \
	-H 'Content-Type: application/x-protobuf' --data-binary '' \
	"http://127.0.0.1:${gateway_http_port}/v1/logs")
[ "$unauthorized_status" = "401" ] || {
	echo "Smoke: unauthenticated OTLP request returned ${unauthorized_status}, want 401" >&2
	exit 1
}
forbidden_status=$(curl -s -o /dev/null -w '%{http_code}' -X POST \
	-H 'Content-Type: application/x-protobuf' \
	-H "Authorization: Bearer ${smoke_token}" \
	-H 'X-Observaki-Project-Id: project-other' \
	--data-binary '' \
	"http://127.0.0.1:${gateway_http_port}/v1/logs")
[ "$forbidden_status" = "403" ] || {
	echo "Smoke: cross-project OTLP request returned ${forbidden_status}, want 403" >&2
	exit 1
}
grep -q "ingest.cross_project_write" "$auth_dir/audit.jsonl" || {
	echo "Smoke: cross-project attempt was not audited" >&2
	exit 1
}

./bin/otlploadgen --protocol http --endpoint "http://127.0.0.1:${gateway_http_port}" \
	--rate 60 --duration 1s --batch-size 3 --workers 1 --mix 34,33,33 --compression gzip \
	--metrics-addr "" --header "Authorization=Bearer ${smoke_token}" >/dev/null
./bin/otlploadgen --protocol grpc --endpoint "127.0.0.1:${gateway_grpc_port}" \
	--rate 60 --duration 1s --batch-size 3 --workers 1 --mix 34,33,33 --compression gzip \
	--seed 2 --metrics-addr "" --header "Authorization=Bearer ${smoke_token}" >/dev/null

# Story 4.2 AC3: a 3-event request against a 1 event/s burst is throttled.
./bin/otlploadgen --protocol http --endpoint "http://127.0.0.1:${gateway_http_port}" \
	--rate 10 --duration 1s --batch-size 3 --workers 1 --mix 0,100,0 --compression none \
	--seed 3 --metrics-addr "" --header "Authorization=Bearer ${limited_token}" >/dev/null 2>&1 || true

gateway_metrics="$(curl -fsS "http://127.0.0.1:${gateway_http_port}/metrics")"
for metric in observaki_gateway_requests_total observaki_gateway_events_total \
	observaki_gateway_auth_failures_total observaki_gateway_quota_rejected_events_total \
	observaki_gateway_config_age_seconds observaki_gateway_up \
	observaki_gateway_wal_connected observaki_gateway_wal_published_events_total \
	observaki_gateway_wal_pending_bytes; do
	case "$gateway_metrics" in
	*"$metric"*) ;;
	*)
		echo "Smoke: gateway /metrics does not expose ${metric}" >&2
		exit 1
		;;
	esac
done
case "$gateway_metrics" in
*'observaki_gateway_auth_failures_total{reason="cross_project_write"} 1'*) ;;
*)
	echo "Smoke: gateway did not count the cross-project rejection" >&2
	exit 1
	;;
esac
quota_rejected=$(printf '%s\n' "$gateway_metrics" | awk '/^observaki_gateway_quota_rejected_events_total /{print $2; exit}')
[ "${quota_rejected:-0}" -gt 0 ] || {
	echo "Smoke: the limited tenant was not throttled" >&2
	exit 1
}

wal_events=""
for _ in $(seq 1 50); do
	gateway_metrics="$(curl -fsS "http://127.0.0.1:${gateway_http_port}/metrics")"
	wal_events=$(printf '%s\n' "$gateway_metrics" | awk '/^observaki_gateway_wal_published_events_total /{print $2; exit}')
	wal_pending=$(printf '%s\n' "$gateway_metrics" | awk '/^observaki_gateway_wal_pending_bytes /{print $2; exit}')
	if [ "${wal_events:-0}" -gt 0 ] && [ "${wal_pending:-1}" = "0" ]; then
		break
	fi
	sleep 0.1
done
[ "${wal_events:-0}" -gt 0 ] || {
	echo "Smoke: no events were acknowledged by the NATS WAL" >&2
	exit 1
}
case "$gateway_metrics" in
*'observaki_gateway_wal_connected 1'*) ;;
*)
	echo "Smoke: the NATS WAL is not connected" >&2
	exit 1
	;;
esac

kill -TERM "$gateway_pid"
if ! wait "$gateway_pid"; then
	echo "Smoke: gateway did not exit cleanly after SIGTERM; gateway log:" >&2
	cat "$gateway_log" >&2
	exit 1
fi
gateway_pid=""

./bin/walcheck --url "$nats_url" --expect-events "$wal_events" \
	--fail-on-duplicates --out "$auth_dir/wal.json" >/dev/null
walcheck_status=$?
if [ "$walcheck_status" -ne 0 ]; then
	echo "Smoke: WAL verification failed" >&2
	cat "$auth_dir/wal.json" >&2 || true
	exit 1
fi

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

echo "Smoke: OK (healthz, api health, OpenAPI + structured 401/404/405, gateway OTLP HTTP/gRPC, auth 401/403 + audit, quota 429, gateway metrics/SIGTERM, web build, version, loadgen payload, chloadgen, bench scenarios, benchmark report)"
