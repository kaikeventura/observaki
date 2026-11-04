#!/usr/bin/env bash
# Emit one round of sample telemetry (metrics, logs and traces) to the
# single-node stack through the OTLP/HTTP gateway.
#
# This is the "sample app" behind Story 5.1 AC2: it proves that all three
# signals travel gateway -> NATS WAL -> writers -> ClickHouse/VictoriaMetrics.
# The polished sample application and the per-language instrumentation guides
# are Story 5.3.
#
#   ./deploy/compose/sample.sh                       # one round
#   ./deploy/compose/sample.sh --loop 5              # one round every 5 s
#   OBSERVAKI_OTLP_HTTP_URL=http://host:4318 ./deploy/compose/sample.sh
#
# Requires curl only. The bearer token is the public development token from
# deploy/compose/config/ingest-auth.json.
set -euo pipefail

endpoint=${OBSERVAKI_OTLP_HTTP_URL:-http://127.0.0.1:4318}
token=${OBSERVAKI_INGEST_TOKEN:-observaki-local-dev-token}
loop_seconds=0
response_file=$(mktemp -t observaki-sample.XXXXXX)
trap 'rm -f "$response_file"' EXIT

usage() {
	cat <<'EOF'
Usage: deploy/compose/sample.sh [options]

Options:
  --url URL          OTLP/HTTP endpoint (default: http://127.0.0.1:4318)
  --token TOKEN      bearer ingest token (default: the stack dev token)
  --loop SECONDS     keep emitting one round every SECONDS (default: one round)
  --help             show this help
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--url)
		endpoint=${2:?--url requires a value}
		shift 2
		;;
	--token)
		token=${2:?--token requires a value}
		shift 2
		;;
	--loop)
		loop_seconds=${2:?--loop requires a value}
		shift 2
		;;
	--help | -h)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

random_hex() {
	local bytes=$1
	od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
}

send() {
	local signal=$1 path=$2 body=$3
	local status
	status=$(curl -sS -o "$response_file" -w '%{http_code}' \
		-H "Authorization: Bearer ${token}" \
		-H 'Content-Type: application/json' \
		--data-binary "$body" \
		"${endpoint}${path}") || {
		echo "${signal}: request failed" >&2
		return 1
	}
	if [ "$status" -lt 200 ] || [ "$status" -ge 300 ]; then
		echo "${signal}: HTTP ${status}: $(cat "$response_file")" >&2
		return 1
	fi
	if grep -Eq '"(rejectedSpans|rejectedLogRecords|rejectedDataPoints)":[1-9]' "$response_file"; then
		echo "${signal}: partial success: $(cat "$response_file")" >&2
		return 1
	fi
	echo "${signal}: accepted (HTTP ${status})"
}

emit_once() {
	local now_ns run_id trace_id span_id child_span_id
	now_ns="$(date -u +%s)000000000"
	run_id="sample-$(date -u +%Y%m%dT%H%M%SZ)-${RANDOM}"
	# Spans are deduplicated by trace_id/span_id (AD-18), so every round needs
	# fresh IDs to be counted as new spans.
	trace_id=$(random_hex 16)
	span_id=$(random_hex 8)
	child_span_id=$(random_hex 8)

	local logs
	logs=$(cat <<EOF
{
  "resourceLogs": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "observaki-sample"}},
      {"key": "service.version", "value": {"stringValue": "0.0.0-dev"}},
      {"key": "deployment.environment", "value": {"stringValue": "local"}}
    ]},
    "scopeLogs": [{
      "scope": {"name": "observaki-sample", "version": "0.0.0-dev"},
      "logRecords": [
        {
          "timeUnixNano": "${now_ns}",
          "observedTimeUnixNano": "${now_ns}",
          "severityNumber": 9,
          "severityText": "INFO",
          "body": {"stringValue": "checkout completed (${run_id})"},
          "attributes": [
            {"key": "http.route", "value": {"stringValue": "/checkout"}},
            {"key": "http.status_code", "value": {"intValue": "200"}}
          ]
        },
        {
          "timeUnixNano": "${now_ns}",
          "observedTimeUnixNano": "${now_ns}",
          "severityNumber": 13,
          "severityText": "WARN",
          "body": {"stringValue": "payment retry scheduled (${run_id})"},
          "attributes": [
            {"key": "http.route", "value": {"stringValue": "/checkout"}},
            {"key": "retry.attempt", "value": {"intValue": "1"}}
          ]
        }
      ]
    }]
  }]
}
EOF
)

	local traces
	traces=$(cat <<EOF
{
  "resourceSpans": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "observaki-sample"}},
      {"key": "service.version", "value": {"stringValue": "0.0.0-dev"}},
      {"key": "deployment.environment", "value": {"stringValue": "local"}}
    ]},
    "scopeSpans": [{
      "scope": {"name": "observaki-sample", "version": "0.0.0-dev"},
      "spans": [
        {
          "traceId": "${trace_id}",
          "spanId": "${span_id}",
          "parentSpanId": "0000000000000000",
          "name": "POST /checkout",
          "kind": 2,
          "startTimeUnixNano": "${now_ns}",
          "endTimeUnixNano": "${now_ns}",
          "status": {"code": 1},
          "attributes": [
            {"key": "http.route", "value": {"stringValue": "/checkout"}},
            {"key": "sample.run_id", "value": {"stringValue": "${run_id}"}}
          ]
        },
        {
          "traceId": "${trace_id}",
          "spanId": "${child_span_id}",
          "parentSpanId": "${span_id}",
          "name": "SELECT cart_items",
          "kind": 3,
          "startTimeUnixNano": "${now_ns}",
          "endTimeUnixNano": "${now_ns}",
          "status": {"code": 1},
          "attributes": [
            {"key": "db.system", "value": {"stringValue": "postgresql"}},
            {"key": "sample.run_id", "value": {"stringValue": "${run_id}"}}
          ]
        }
      ]
    }]
  }]
}
EOF
)

	local metrics
	metrics=$(cat <<EOF
{
  "resourceMetrics": [{
    "resource": {"attributes": [
      {"key": "service.name", "value": {"stringValue": "observaki-sample"}},
      {"key": "service.version", "value": {"stringValue": "0.0.0-dev"}},
      {"key": "deployment.environment", "value": {"stringValue": "local"}}
    ]},
    "scopeMetrics": [{
      "scope": {"name": "observaki-sample", "version": "0.0.0-dev"},
      "metrics": [
        {
          "name": "observaki_sample_requests_total",
          "unit": "1",
          "sum": {
            "aggregationTemporality": 2,
            "isMonotonic": true,
            "dataPoints": [{
              "asInt": "1",
              "timeUnixNano": "${now_ns}",
              "attributes": [{"key": "http.route", "value": {"stringValue": "/checkout"}}]
            }]
          }
        },
        {
          "name": "observaki_sample_request_duration_seconds",
          "unit": "s",
          "gauge": {
            "dataPoints": [{
              "asDouble": 0.042,
              "timeUnixNano": "${now_ns}",
              "attributes": [{"key": "http.route", "value": {"stringValue": "/checkout"}}]
            }]
          }
        }
      ]
    }]
  }]
}
EOF
)

	send logs /v1/logs "$logs"
	send traces /v1/traces "$traces"
	send metrics /v1/metrics "$metrics"
}

if [ "$loop_seconds" -gt 0 ]; then
	while true; do
		emit_once
		sleep "$loop_seconds"
	done
fi

emit_once
