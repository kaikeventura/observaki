#!/usr/bin/env bash
# Check the Story 5.3 sample apps.
#
#   ./examples/check.sh            # syntax/formatting only (no network, no deps)
#   ./examples/check.sh --build    # build Go, install Python/Node dependencies
#   ./examples/check.sh --live     # run all three apps against the Compose stack
#
# --live requires the single-node stack to be healthy and verifies that each
# language produced 2 log records, 2 spans and the sample metric.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

MODE=static
case "${1:-}" in
--static | "") MODE=static ;;
--build) MODE=build ;;
--live) MODE=live ;;
--help | -h)
	cat <<'EOF'
Usage: examples/check.sh [--static|--build|--live]

  --static  gofmt, Python compile, Node syntax and package.json checks
  --build   additionally build Go and install the Python/Node dependencies
  --live    build and run the three apps against the running Compose stack
EOF
	exit 0
	;;
*)
	echo "unknown option: $1" >&2
	exit 2
	;;
esac

log() { printf '[examples] %s\n' "$*" >&2; }
die() {
	log "FAIL: $*"
	exit 1
}

TMP=$(mktemp -d -t observaki-examples.XXXXXX)
trap 'rm -rf "$TMP"' EXIT

static_check() {
	log "checking formatting and syntax"
	(cd examples/go && [ -z "$(gofmt -l .)" ]) || die "examples/go is not gofmt-clean"
	python3 - <<'PY' || die "examples/python/app.py does not compile"
compile(open("examples/python/app.py", encoding="utf-8").read(), "app.py", "exec")
PY
	node --check examples/node/app.js || die "examples/node/app.js has a syntax error"
	python3 - <<'PY' || die "examples/node/package.json is not valid JSON"
import json
json.load(open("examples/node/package.json", encoding="utf-8"))
PY
	bash -n examples/check.sh || die "examples/check.sh has a syntax error"
	log "static checks passed"
}

build_check() {
	static_check
	log "building the Go example"
	(cd examples/go && go build -o "$TMP/quickstart-go" . && go vet ./...)

	log "installing the Python dependencies in a temporary venv"
	python3 -m venv "$TMP/venv"
	"$TMP/venv/bin/pip" install --quiet --disable-pip-version-check \
		-r examples/python/requirements.txt

	log "installing the Node dependencies"
	(cd examples/node && npm ci --no-audit --no-fund >/dev/null)
}

ch() {
	docker compose exec -T clickhouse clickhouse-client --database observaki --query "$1"
}

wait_for_count() {
	local table=$1 service=$2 minimum=$3 elapsed=0 count
	until [ "${count:-0}" -ge "$minimum" ]; do
		count=$(ch "SELECT count() FROM ${table} WHERE service_name = '${service}'")
		if [ "$elapsed" -ge 120 ]; then
			die "${table} for ${service}: ${count} rows, want >= ${minimum}"
		fi
		elapsed=$((elapsed + 2))
		sleep 2
	done
	log "${table} for ${service}: ${count} rows"
}

wait_for_metric() {
	local service=$1 elapsed=0 found=0 now
	while [ "$found" -eq 0 ]; do
		now=$(date +%s)
		if curl -fsS --get 'http://127.0.0.1:8428/api/v1/query_range' \
			--data-urlencode "query=observaki_quickstart_requests_total{project_id=\"project-dev\",service_name=\"${service}\"}" \
			--data-urlencode "start=$((now - 600))" \
			--data-urlencode "end=$now" \
			--data-urlencode 'step=30' |
			python3 -c 'import json,sys; sys.exit(0 if json.load(sys.stdin)["data"]["result"] else 1)'; then
			found=1
			break
		fi
		if [ "$elapsed" -ge 120 ]; then
			die "metric for ${service} did not appear in VictoriaMetrics"
		fi
		elapsed=$((elapsed + 2))
		sleep 2
	done
	log "metric for ${service}: present"
}

live_check() {
	command -v docker >/dev/null || die "docker is required for --live"
	command -v curl >/dev/null || die "curl is required for --live"
	docker compose version >/dev/null 2>&1 || die "docker compose v2+ is required"
	curl -fsS http://127.0.0.1:4318/readyz >/dev/null ||
		die "the gateway is not ready; run 'docker compose up -d --build --wait' first"
	curl -fsS http://127.0.0.1:8082/readyz >/dev/null ||
		die "the writer is not ready; run 'docker compose up -d --build --wait' first"

	build_check

	local stamp service
	stamp=$(date -u +%Y%m%d%H%M%S)

	service="observaki-quickstart-go-${stamp}"
	log "running the Go example (service ${service})"
	(cd examples/go && OTEL_SERVICE_NAME="$service" go run .)
	wait_for_count otel_logs "$service" 2
	wait_for_count otel_traces "$service" 2
	wait_for_metric "$service"

	service="observaki-quickstart-python-${stamp}"
	log "running the Python example (service ${service})"
	(cd examples/python && OTEL_SERVICE_NAME="$service" "$TMP/venv/bin/python" app.py)
	wait_for_count otel_logs "$service" 2
	wait_for_count otel_traces "$service" 2
	wait_for_metric "$service"

	service="observaki-quickstart-node-${stamp}"
	log "running the Node.js example (service ${service})"
	(cd examples/node && OTEL_SERVICE_NAME="$service" node app.js)
	wait_for_count otel_logs "$service" 2
	wait_for_count otel_traces "$service" 2
	wait_for_metric "$service"

	log "PASS: Go, Python and Node.js sent logs, traces and metrics"
}

case "$MODE" in
static) static_check ;;
build) build_check ;;
live) live_check ;;
esac
