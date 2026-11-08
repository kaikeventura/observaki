#!/usr/bin/env bash
# End-to-end check for the Observaki Helm chart (Story 5.2).
#
#   ./deploy/helm/smoke.sh              # build, install on kind, upgrade, rollback
#   ./deploy/helm/smoke.sh --no-build   # reuse the local observaki images
#   ./deploy/helm/smoke.sh --keep       # leave the kind cluster running
#
# It verifies:
#   AC1  a 3-node kind cluster installs the chart, every workload and the
#        bundled dependencies become ready, and telemetry flows end to end
#   AC2  a chart upgrade followed by a rollback returns the pods to the previous
#        image and keeps the data in the persistent volumes
#   AC3  the chart README is present and documents prerequisites, values,
#        sizing, upgrade and troubleshooting
#
# The report is written to deploy/helm/results/<UTC timestamp>/report.md.
#
# Requires: docker, kind, kubectl, helm 4.3+, curl and jq. It creates (and by
# default deletes) a kind cluster named observaki-helm and pulls the ClickHouse,
# VictoriaMetrics, PostgreSQL and NATS images into it.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

CLUSTER=observaki-helm
RELEASE=${OBSERVAKI_HELM_RELEASE:-observaki}
NAMESPACE=${OBSERVAKI_HELM_NAMESPACE:-observaki}
CHART=${OBSERVAKI_HELM_CHART:-$ROOT/deploy/helm/observaki}
KIND_CONFIG=$ROOT/deploy/helm/kind.yaml
TIMEOUT=${OBSERVAKI_HELM_TIMEOUT:-600s}
HEALTH_LIMIT=${OBSERVAKI_HELM_HEALTH_LIMIT:-600}
API_PORT=${OBSERVAKI_HELM_API_PORT:-18080}
GATEWAY_PORT=${OBSERVAKI_HELM_GATEWAY_PORT:-14318}
VM_PORT=${OBSERVAKI_HELM_VM_PORT:-18428}
BUILD=1
KEEP=0
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
REPORT_DIR="$ROOT/deploy/helm/results/$STAMP"
REPORT="$REPORT_DIR/report.md"

PASS=0
FAIL=0
declare -a RESULTS=()
PF_PIDS=()
WORK_DIR=$(mktemp -d -t observaki-helm-smoke.XXXXXX)

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }

cleanup() {
	for pid in "${PF_PIDS[@]:-}"; do
		kill "$pid" >/dev/null 2>&1 || true
	done
}
trap 'cleanup; rm -rf "$WORK_DIR"' EXIT

die() {
	log "FAIL: $*"
	RESULTS+=("FAIL|$*")
	FAIL=$((FAIL + 1))
	write_report
	exit 1
}

usage() {
	cat <<'EOF'
Usage: deploy/helm/smoke.sh [options]

Options:
  --no-build       reuse the local observaki:local and observaki:local-v2 images
  --keep           keep the kind cluster and the Helm release for debugging
  --timeout VALUE  helm/kubectl wait timeout (default: 600s)
  --help           show this help

The report is written to deploy/helm/results/<UTC timestamp>/report.md.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--no-build)
		BUILD=0
		shift
		;;
	--keep)
		KEEP=1
		shift
		;;
	--timeout)
		TIMEOUT=${2:?--timeout requires a value}
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

for cmd in docker kind kubectl helm curl jq; do
	command -v "$cmd" >/dev/null || die "$cmd is required"
done

mkdir -p "$REPORT_DIR"

record() {
	local status=$1 message=$2
	RESULTS+=("$status|$message")
	if [ "$status" = PASS ]; then
		PASS=$((PASS + 1))
		log "PASS: $message"
	else
		FAIL=$((FAIL + 1))
		log "FAIL: $message"
	fi
}

check() {
	local message=$1
	shift
	if "$@" >/dev/null 2>&1; then
		record PASS "$message"
	else
		record FAIL "$message"
	fi
}

write_report() {
	{
		echo "# Helm chart smoke - ${STAMP}"
		echo
		echo "- Cluster: kind \`${CLUSTER}\` (3 nodes)"
		echo "- Chart: \`deploy/helm/observaki\` ($(helm show chart "$CHART" | awk '/^version:/ {print $2}'))"
		echo "- Release: \`${RELEASE}\` in namespace \`${NAMESPACE}\`"
		echo "- Helm: $(helm version --template '{{ .Version }}')"
		echo "- Kubernetes: $(kubectl version -o json 2>/dev/null | jq -r '.serverVersion.gitVersion // "unknown"')"
		echo "- Host: $(uname -srm)"
		echo
		echo "## Install"
		echo
		echo "| Metric | Value |"
		echo "|---|---|"
		echo "| Install wall clock | ${INSTALL_SECONDS:-n/a} s |"
		echo "| Healthy limit | ${HEALTH_LIMIT} s |"
		echo "| API reachable | ${API_STATUS:-n/a} |"
		echo "| Gateway ready | ${GATEWAY_STATUS:-n/a} |"
		echo "| Nodes Ready | ${NODES_READY:-n/a} |"
		echo
		echo "### Pods"
		echo
		echo '```'
		kubectl get pods -n "$NAMESPACE" -o wide 2>/dev/null || true
		echo '```'
		echo
		echo "### Persistent volumes"
		echo
		echo '```'
		kubectl get pvc -n "$NAMESPACE" 2>/dev/null || true
		echo '```'
		echo
		echo "## AC1 - Install"
		echo
		echo "- Workloads and bundled dependencies ready: ${AC1_READY:-n/a}"
		echo "- Sample telemetry: ${AC1_SAMPLE:-n/a}"
		echo "- ClickHouse logs/traces: ${AC1_COUNTS:-n/a}"
		echo "- VictoriaMetrics sample present: ${AC1_VM:-n/a}"
		echo
		echo "## AC2 - Upgrade and rollback"
		echo
		echo "| Step | Revision | Chart | Image tags | Logs | Traces |"
		echo "|---|---|---|---|---|---|"
		echo "| Installed | ${REV_INSTALL:-n/a} | ${CHART_INSTALL:-n/a} | ${IMAGES_INSTALL:-n/a} | ${LOGS_INSTALL:-n/a} | ${TRACES_INSTALL:-n/a} |"
		echo "| Upgraded | ${REV_UPGRADE:-n/a} | ${CHART_UPGRADE:-n/a} | ${IMAGES_UPGRADE:-n/a} | ${LOGS_UPGRADE:-n/a} | ${TRACES_UPGRADE:-n/a} |"
		echo "| Rolled back | ${REV_ROLLBACK:-n/a} | ${CHART_ROLLBACK:-n/a} | ${IMAGES_ROLLBACK:-n/a} | ${LOGS_ROLLBACK:-n/a} | ${TRACES_ROLLBACK:-n/a} |"
		echo
		echo "- Upgrade wall clock: ${UPGRADE_SECONDS:-n/a} s"
		echo "- Rollback wall clock: ${ROLLBACK_SECONDS:-n/a} s"
		echo "- Chart version bumped and restored: ${AC2_CHART:-n/a}"
		echo "- Data preserved across upgrade and rollback: ${AC2_DATA:-n/a}"
		echo "- Rollback restored the previous image: ${AC2_IMAGE:-n/a}"
		echo
		echo "## AC3 - Documentation"
		echo
		echo "- Chart README sections: ${AC3_README:-n/a}"
		echo
		echo "## Checks"
		echo
		echo "| Result | Check |"
		echo "|---|---|"
		for result in "${RESULTS[@]}"; do
			printf '| %s | %s |\n' "${result%%|*}" "${result#*|}"
		done
		echo
		echo "**Summary:** ${PASS} passed, ${FAIL} failed."
	} >"$REPORT"
	log "report: $REPORT"
}

# --- helpers -----------------------------------------------------------------

helm_revision() {
	helm list -n "$NAMESPACE" -o json | jq -r --arg r "$RELEASE" '.[] | select(.name == $r) | .revision'
}

helm_chart_version() {
	helm list -n "$NAMESPACE" -o json | jq -r --arg r "$RELEASE" '.[] | select(.name == $r) | .chart'
}

pod_images() {
	kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/instance=${RELEASE}" -o json |
		jq -r '[.items[] | select(.status.phase == "Running") | .spec.containers[0].image | select(startswith("observaki:"))] | unique | join(",")'
}

wait_http() {
	local url=$1 deadline=$2
	local start
	start=$(date +%s)
	until curl -fsS "$url" >/dev/null 2>&1; do
		if [ $(( $(date +%s) - start )) -ge "$deadline" ]; then
			return 1
		fi
		sleep 2
	done
}

clickhouse_pod() {
	kubectl get pods -n "$NAMESPACE" -l "app.kubernetes.io/instance=${RELEASE},app.kubernetes.io/component=clickhouse" \
		-o jsonpath='{.items[0].metadata.name}'
}

clickhouse_query() {
	local pod user password
	pod=$(clickhouse_pod)
	user=$(kubectl get secret "${RELEASE}-clickhouse" -n "$NAMESPACE" -o jsonpath='{.data.user}' | base64 -d)
	password=$(kubectl get secret "${RELEASE}-clickhouse" -n "$NAMESPACE" -o jsonpath='{.data.password}' | base64 -d)
	kubectl exec -n "$NAMESPACE" "$pod" -c clickhouse -- \
		clickhouse-client --user "$user" --password "$password" --database observaki --query "$1"
}

wait_for_counts() {
	local deadline=$1 expected_logs=$2 expected_traces=$3
	local start logs traces
	start=$(date +%s)
	while :; do
		logs=$(clickhouse_query 'SELECT count() FROM otel_logs' 2>/dev/null || echo 0)
		traces=$(clickhouse_query 'SELECT count() FROM otel_traces' 2>/dev/null || echo 0)
		if [ "$logs" -ge "$expected_logs" ] && [ "$traces" -ge "$expected_traces" ]; then
			printf '%s %s' "$logs" "$traces"
			return 0
		fi
		if [ $(( $(date +%s) - start )) -ge "$deadline" ]; then
			printf '%s %s' "$logs" "$traces"
			return 1
		fi
		sleep 3
	done
}

send_sample() {
	OBSERVAKI_OTLP_HTTP_URL="http://127.0.0.1:${GATEWAY_PORT}" \
		OBSERVAKI_INGEST_TOKEN=observaki-local-dev-token \
		"$ROOT/deploy/compose/sample.sh" >/dev/null
}

# A port-forward is pinned to the pod it connected to; a rollout replaces that
# pod and kills the tunnel. Restart the forwards after every install/upgrade/
# rollback before sending traffic again.
stop_port_forwards() {
	for pid in "${PF_PIDS[@]:-}"; do
		kill "$pid" >/dev/null 2>&1 || true
		wait "$pid" 2>/dev/null || true
	done
	PF_PIDS=()
}

start_port_forwards() {
	stop_port_forwards
	kubectl port-forward -n "$NAMESPACE" "svc/${RELEASE}-api" "${API_PORT}:80" >"$REPORT_DIR/port-forward-api.log" 2>&1 &
	PF_PIDS+=($!)
	kubectl port-forward -n "$NAMESPACE" "svc/${RELEASE}-gateway" "${GATEWAY_PORT}:4318" >"$REPORT_DIR/port-forward-gateway.log" 2>&1 &
	PF_PIDS+=($!)
	kubectl port-forward -n "$NAMESPACE" "svc/${RELEASE}-victoriametrics" "${VM_PORT}:8428" >"$REPORT_DIR/port-forward-vm.log" 2>&1 &
	PF_PIDS+=($!)
}

# --- build and cluster -------------------------------------------------------

if [ "$BUILD" = 1 ]; then
	commit=$(git rev-parse --short HEAD 2>/dev/null || echo none)
	build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	log "building observaki:local and observaki:local-v2"
	docker build -f deploy/compose/Dockerfile \
		--build-arg VERSION=helm-local --build-arg COMMIT="$commit" --build-arg BUILD_DATE="$build_date" \
		-t observaki:local . >/dev/null
	docker build -f deploy/compose/Dockerfile \
		--build-arg VERSION=helm-local-v2 --build-arg COMMIT="$commit" --build-arg BUILD_DATE="$build_date" \
		-t observaki:local-v2 . >/dev/null
	record PASS "built observaki:local and observaki:local-v2"
fi

if kind get clusters | grep -qx "$CLUSTER"; then
	log "reusing kind cluster $CLUSTER"
else
	log "creating 3-node kind cluster $CLUSTER"
	kind create cluster --config "$KIND_CONFIG" --wait 300s >/dev/null
	record PASS "created 3-node kind cluster"
fi

kind load docker-image observaki:local --name "$CLUSTER" >/dev/null
kind load docker-image observaki:local-v2 --name "$CLUSTER" >/dev/null
record PASS "loaded images into the kind nodes"

kubectl config use-context "kind-${CLUSTER}" >/dev/null

# --- AC1: install ------------------------------------------------------------

log "installing the chart"
install_start=$(date +%s)
if ! helm upgrade --install "$RELEASE" "$CHART" \
	--namespace "$NAMESPACE" --create-namespace \
	--wait --timeout "$TIMEOUT" \
	--set image.repository=observaki \
	--set image.tag=local \
	--set image.pullPolicy=IfNotPresent \
	--set environment=development \
	2>"$REPORT_DIR/install.log"; then
	die "helm install failed (see $REPORT_DIR/install.log)"
fi
INSTALL_SECONDS=$(( $(date +%s) - install_start ))
record PASS "helm install completed in ${INSTALL_SECONDS}s"
if [ "$INSTALL_SECONDS" -le "$HEALTH_LIMIT" ]; then
	record PASS "install under the ${HEALTH_LIMIT}s health limit"
else
	record FAIL "install took ${INSTALL_SECONDS}s, over the ${HEALTH_LIMIT}s health limit"
fi
REV_INSTALL=$(helm_revision)
IMAGES_INSTALL=$(pod_images)
CHART_INSTALL=$(helm_chart_version)

# Every deployment and statefulset must report its desired replicas ready, and
# the four bundled dependencies must be ready.
AC1_READY=pass
for workload in deployment/${RELEASE}-api deployment/${RELEASE}-gateway statefulset/${RELEASE}-writer \
	statefulset/${RELEASE}-clickhouse statefulset/${RELEASE}-victoriametrics \
	statefulset/${RELEASE}-postgres statefulset/${RELEASE}-nats; do
	readiness=$(kubectl get "$workload" -n "$NAMESPACE" -o json |
		jq -r '[(.spec.replicas // 1), (.status.readyReplicas // 0)] | @tsv' 2>/dev/null || true)
	desired=$(printf '%s' "$readiness" | awk '{print $1}')
	ready=$(printf '%s' "$readiness" | awk '{print $2}')
	if [ "${ready:-0}" -lt "${desired:-1}" ]; then
		AC1_READY="failed at ${workload} (${ready:-0}/${desired:-1})"
		record FAIL "${workload} ready replicas ${ready:-0}/${desired:-1}"
	else
		record PASS "${workload} ready replicas ${ready}/${desired}"
	fi
done

NODES_READY=$(kubectl get nodes --no-headers | awk '$2 == "Ready"' | wc -l | tr -d ' ')
if [ "$NODES_READY" -ge 3 ]; then
	record PASS "3 cluster nodes Ready (${NODES_READY})"
else
	record FAIL "only ${NODES_READY} cluster nodes Ready"
fi

pvc_bound=$(kubectl get pvc -n "$NAMESPACE" --no-headers 2>/dev/null | awk '$2 == "Bound"' | wc -l | tr -d ' ')
if [ "$pvc_bound" -ge 5 ]; then
	record PASS "persistent volume claims bound (${pvc_bound})"
else
	record FAIL "only ${pvc_bound} PVCs bound (want >= 5)"
fi

# Port-forwards for the API, the OTLP gateway and VictoriaMetrics.
start_port_forwards

if wait_http "http://127.0.0.1:${API_PORT}/healthz" 120; then
	API_STATUS=$(curl -fsS "http://127.0.0.1:${API_PORT}/healthz")
	record PASS "API answers /healthz through the Service"
else
	API_STATUS=unreachable
	die "API is not reachable through port-forward"
fi
if wait_http "http://127.0.0.1:${GATEWAY_PORT}/readyz" 120; then
	GATEWAY_STATUS=$(curl -fsS "http://127.0.0.1:${GATEWAY_PORT}/readyz")
	record PASS "gateway answers /readyz through the Service"
else
	GATEWAY_STATUS=unreachable
	die "gateway is not reachable through port-forward"
fi
wait_http "http://127.0.0.1:${VM_PORT}/health" 120 || die "VictoriaMetrics is not reachable"

# AC1 sample data: 2 logs + 2 spans per round, plus the sample metric.
log "sending the sample OTLP round"
send_sample || die "sample OTLP send failed"

LOGS_INSTALL=0
TRACES_INSTALL=0
if counts=$(wait_for_counts 180 2 2); then
	LOGS_INSTALL=$(printf '%s' "$counts" | awk '{print $1}')
	TRACES_INSTALL=$(printf '%s' "$counts" | awk '{print $2}')
	AC1_COUNTS="${LOGS_INSTALL} logs / ${TRACES_INSTALL} traces"
	AC1_SAMPLE=pass
	record PASS "telemetry reached ClickHouse (${AC1_COUNTS})"
else
	AC1_COUNTS=$(printf '%s' "$counts" | awk '{print $1 " logs / " $2 " traces"}')
	AC1_SAMPLE=failed
	record FAIL "telemetry did not reach ClickHouse (${AC1_COUNTS})"
fi

AC1_VM=failed
vm_start=$(date +%s)
while :; do
	vm_response=$(curl -fsS --get "http://127.0.0.1:${VM_PORT}/api/v1/query_range" \
		--data-urlencode 'query=observaki_sample_requests_total' \
		--data-urlencode "start=$(( $(date +%s) - 600 ))" \
		--data-urlencode "end=$(date +%s)" \
		--data-urlencode 'step=15' 2>/dev/null || echo '{}')
	if [ "$(printf '%s' "$vm_response" | jq -r '.data.result | length' 2>/dev/null || echo 0)" -gt 0 ]; then
		AC1_VM=pass
		record PASS "metric reached VictoriaMetrics"
		break
	fi
	if [ $(( $(date +%s) - vm_start )) -ge 180 ]; then
		record FAIL "metric did not reach VictoriaMetrics"
		break
	fi
	sleep 5
done

# helm test runs the chart's own probes.
if helm test "$RELEASE" -n "$NAMESPACE" --timeout 120s >"$REPORT_DIR/helm-test.log" 2>&1; then
	record PASS "helm test"
else
	record FAIL "helm test"
fi

# --- AC2: upgrade and rollback ----------------------------------------------

CHART_VERSION=$(helm show chart "$CHART" | awk '/^version:/ {print $2}')
UPGRADE_VERSION="${CHART_VERSION}.1"
log "packaging chart ${UPGRADE_VERSION} for the upgrade"
helm package "$CHART" --version "$UPGRADE_VERSION" --destination "$WORK_DIR" >/dev/null
UPGRADE_CHART="$WORK_DIR/observaki-${UPGRADE_VERSION}.tgz"
[ -f "$UPGRADE_CHART" ] || die "packaged chart not found at $UPGRADE_CHART"

log "upgrading the release to chart ${UPGRADE_VERSION} and the v2 image"
upgrade_start=$(date +%s)
if ! helm upgrade "$RELEASE" "$UPGRADE_CHART" \
	--namespace "$NAMESPACE" \
	--wait --timeout "$TIMEOUT" \
	--set image.repository=observaki \
	--set image.tag=local-v2 \
	--set image.pullPolicy=IfNotPresent \
	--set environment=development \
	2>"$REPORT_DIR/upgrade.log"; then
	die "helm upgrade failed (see $REPORT_DIR/upgrade.log)"
fi
UPGRADE_SECONDS=$(( $(date +%s) - upgrade_start ))
REV_UPGRADE=$(helm_revision)
IMAGES_UPGRADE=$(pod_images)
CHART_UPGRADE=$(helm_chart_version)
if printf '%s' "$IMAGES_UPGRADE" | grep -q 'observaki:local-v2'; then
	record PASS "upgrade rolled the pods to observaki:local-v2"
else
	record FAIL "upgrade did not roll the pods to observaki:local-v2 (${IMAGES_UPGRADE})"
fi

# The rollout replaced the gateway/API pods and killed the tunnels.
start_port_forwards
if ! wait_http "http://127.0.0.1:${API_PORT}/healthz" 120 ||
	! wait_http "http://127.0.0.1:${GATEWAY_PORT}/readyz" 120; then
	die "API/gateway not reachable after the upgrade"
fi
record PASS "API and gateway reachable after the upgrade"

send_sample || die "sample after upgrade failed"
# Prove the upgraded pods write too: the sample adds 2 logs and 2 spans.
if counts=$(wait_for_counts 120 "$((LOGS_INSTALL + 2))" "$((TRACES_INSTALL + 2))"); then
	LOGS_UPGRADE=$(printf '%s' "$counts" | awk '{print $1}')
	TRACES_UPGRADE=$(printf '%s' "$counts" | awk '{print $2}')
	record PASS "telemetry after the upgrade (${LOGS_UPGRADE} logs / ${TRACES_UPGRADE} traces)"
else
	LOGS_UPGRADE=$(printf '%s' "$counts" | awk '{print $1}')
	TRACES_UPGRADE=$(printf '%s' "$counts" | awk '{print $2}')
	record FAIL "telemetry after the upgrade did not increase (${LOGS_UPGRADE} logs / ${TRACES_UPGRADE} traces)"
fi

log "rolling back the release"
rollback_start=$(date +%s)
if ! helm rollback "$RELEASE" -n "$NAMESPACE" --wait --timeout "$TIMEOUT" \
	2>"$REPORT_DIR/rollback.log"; then
	die "helm rollback failed (see $REPORT_DIR/rollback.log)"
fi
ROLLBACK_SECONDS=$(( $(date +%s) - rollback_start ))
REV_ROLLBACK=$(helm_revision)
IMAGES_ROLLBACK=$(pod_images)
CHART_ROLLBACK=$(helm_chart_version)

if [ "$CHART_UPGRADE" = "observaki-${UPGRADE_VERSION}" ] && [ "$CHART_ROLLBACK" = "observaki-${CHART_VERSION}" ]; then
	AC2_CHART=pass
	record PASS "chart version moved to ${UPGRADE_VERSION} and rolled back to ${CHART_VERSION}"
else
	AC2_CHART=failed
	record FAIL "chart versions unexpected (upgrade=${CHART_UPGRADE}, rollback=${CHART_ROLLBACK})"
fi

if printf '%s' "$IMAGES_ROLLBACK" | grep -q 'observaki:local' &&
	! printf '%s' "$IMAGES_ROLLBACK" | grep -q 'observaki:local-v2'; then
	AC2_IMAGE=pass
	record PASS "rollback restored the previous image (${IMAGES_ROLLBACK})"
else
	AC2_IMAGE=failed
	record FAIL "rollback did not restore the previous image (${IMAGES_ROLLBACK})"
fi

# The rollback replaced the pods again; re-open the tunnels before the final
# readiness check.
start_port_forwards
if ! wait_http "http://127.0.0.1:${API_PORT}/healthz" 120 ||
	! wait_http "http://127.0.0.1:${GATEWAY_PORT}/readyz" 120; then
	die "API/gateway not reachable after the rollback"
fi
record PASS "API and gateway reachable after the rollback"

counts=$(wait_for_counts 60 "$LOGS_UPGRADE" "$TRACES_UPGRADE") || true
LOGS_ROLLBACK=$(printf '%s' "$counts" | awk '{print $1}')
TRACES_ROLLBACK=$(printf '%s' "$counts" | awk '{print $2}')
if [ "$LOGS_ROLLBACK" -ge "$LOGS_UPGRADE" ] && [ "$TRACES_ROLLBACK" -ge "$TRACES_UPGRADE" ] &&
	[ "$LOGS_ROLLBACK" -ge "$LOGS_INSTALL" ] && [ "$TRACES_ROLLBACK" -ge "$TRACES_INSTALL" ]; then
	AC2_DATA=pass
	record PASS "data survived the upgrade and rollback (${LOGS_ROLLBACK} logs / ${TRACES_ROLLBACK} traces)"
else
	AC2_DATA=failed
	record FAIL "data was lost across upgrade/rollback (${LOGS_INSTALL}/${LOGS_UPGRADE}/${LOGS_ROLLBACK} logs)"
fi

# --- AC3: documentation ------------------------------------------------------

readme=$CHART/README.md
missing=""
for section in "Prerequisites" "Values" "Sizing" "Upgrade" "Troubleshooting"; do
	if ! grep -qi "^#* *${section}" "$readme" 2>/dev/null; then
		missing="${missing} ${section}"
	fi
done
if [ -z "$missing" ]; then
	AC3_README=pass
	record PASS "chart README documents prerequisites, values, sizing, upgrade and troubleshooting"
else
	AC3_README="missing:${missing}"
	record FAIL "chart README is missing sections:${missing}"
fi

# --- teardown and report -----------------------------------------------------

if [ "$KEEP" = 1 ]; then
	log "keeping the kind cluster (--keep)"
else
	log "deleting the kind cluster"
	kind delete cluster --name "$CLUSTER" >/dev/null 2>&1 || true
fi

write_report

echo
echo "AC1 install: ${AC1_READY}, sample ${AC1_SAMPLE}, ${AC1_COUNTS}"
echo "AC2 upgrade/rollback: chart ${AC2_CHART}, image ${AC2_IMAGE}, data ${AC2_DATA} (${REV_INSTALL} -> ${REV_UPGRADE} -> ${REV_ROLLBACK})"
echo "AC3 documentation: ${AC3_README}"
echo "Checks: ${PASS} passed, ${FAIL} failed"
echo "Report: ${REPORT}"

if [ "$FAIL" -gt 0 ]; then
	exit 1
fi
