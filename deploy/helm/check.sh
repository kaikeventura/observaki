#!/usr/bin/env bash
# Static checks for the Observaki Helm chart (Story 5.2).
#
#   ./deploy/helm/check.sh
#
# Verifies that the chart copies of the ClickHouse migrations and the
# development ingest snapshot have not drifted from their sources, lints and
# renders the chart in bundled and external modes, exercises the fail-fast
# guards, and validates the manifests with kubectl/kubeconform when available.
#
# Requires: helm 4.3+. kubectl and kubeconform are optional.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

CHART="deploy/helm/observaki"
TMP=$(mktemp -d -t observaki-chart.XXXXXX)
trap 'rm -rf "$TMP"' EXIT

failures=0
ok() { printf '  ok    %s\n' "$1"; }
fail() {
	printf '  FAIL  %s\n' "$1"
	failures=$((failures + 1))
}

require() {
	command -v "$1" >/dev/null 2>&1 || {
		echo "missing required command: $1" >&2
		exit 1
	}
}

require helm
require diff

helm_version=$(helm version --template '{{ .Version }}' 2>/dev/null || true)
echo "Checking the Observaki Helm chart (helm ${helm_version:-unknown})..."

# 1. Files copied into the chart must match their canonical sources. Helm
#    cannot read files outside the chart directory, so the copies are checked
#    in CI to catch drift.
if diff -r "internal/adapters/clickhouse/migrations" "$CHART/files/migrations" >/dev/null; then
	ok "ClickHouse migrations match internal/adapters/clickhouse/migrations"
else
	fail "ClickHouse migration copies drifted; run deploy/helm/sync-files.sh"
	diff -r "internal/adapters/clickhouse/migrations" "$CHART/files/migrations" || true
fi

if diff -q "deploy/compose/config/ingest-auth.json" "$CHART/files/ingest-auth-dev.json" >/dev/null; then
	ok "development ingest snapshot matches the Compose stack"
else
	fail "ingest-auth-dev.json drifted from deploy/compose/config/ingest-auth.json; run deploy/helm/sync-files.sh"
fi

# 2. Lint and render.
if helm lint --strict "$CHART" >"$TMP/lint.txt" 2>&1; then
	ok "helm lint --strict"
else
	fail "helm lint --strict"
	cat "$TMP/lint.txt"
fi

render() {
	local name=$1
	shift
	helm template observaki "$CHART" --namespace observaki "$@"
}

if render bundled >"$TMP/bundled.yaml" 2>"$TMP/bundled.err"; then
	ok "renders the default (bundled dependencies) profile"
	if grep -Eq '[0-9]e\+[0-9]' "$TMP/bundled.yaml"; then
		# The YAML parser turns large integers into float64 and renders them as
		# 1.048576e+07, which the gateway/ClickHouse reject at runtime.
		fail "rendered manifests contain scientific notation (quote large values in values.yaml)"
		grep -nE '[0-9]e\+[0-9]' "$TMP/bundled.yaml" || true
	else
		ok "no scientific notation in the rendered manifests"
	fi
else
	fail "default profile does not render"
	cat "$TMP/bundled.err"
fi

external_args=(
	--set dependencies.mode=external
	--set external.clickhouse.url=http://clickhouse.example:8123
	--set external.clickhouse.nativeAddress=clickhouse.example:9000
	--set external.victoriametrics.url=http://victoriametrics.example:8428
	--set external.postgres.urlSecret=observaki-postgres
	--set external.nats.url=nats://nats.example:4222
)
if render external "${external_args[@]}" >"$TMP/external.yaml" 2>"$TMP/external.err"; then
	ok "renders the external dependencies profile"
	if grep -q '^kind: StatefulSet' "$TMP/external.yaml"; then
		# Only the writer StatefulSet (spool PVC) may remain in external mode.
		statefulsets=$(grep -c '^kind: StatefulSet' "$TMP/external.yaml")
		if [ "$statefulsets" -eq 1 ]; then
			ok "external mode installs only the writer StatefulSet"
		else
			fail "external mode rendered ${statefulsets} StatefulSets, want 1"
		fi
	else
		fail "external mode did not render the writer StatefulSet"
	fi
else
	fail "external profile does not render"
	cat "$TMP/external.err"
fi

# 3. Fail-fast guards: each case must refuse to render with the expected hint.
expect_failure() {
	local label=$1 expected=$2
	shift 2
	if helm template observaki "$CHART" "$@" >/dev/null 2>"$TMP/guard.err"; then
		fail "${label} should not render"
		return
	fi
	if grep -q "$expected" "$TMP/guard.err"; then
		ok "${label} is rejected"
	else
		fail "${label} failed for an unexpected reason"
		cat "$TMP/guard.err"
	fi
}

expect_failure "missing external ClickHouse URL" "external.clickhouse.url is required" \
	--set dependencies.mode=external \
	--set external.victoriametrics.url=http://vm:8428 \
	--set external.postgres.urlSecret=pg \
	--set external.nats.url=nats://nats:4222

expect_failure "external mode without a native ClickHouse address" "external.clickhouse.nativeAddress is required" \
	--set dependencies.mode=external \
	--set external.clickhouse.url=http://ch:8123 \
	--set external.victoriametrics.url=http://vm:8428 \
	--set external.postgres.urlSecret=pg \
	--set external.nats.url=nats://nats:4222

expect_failure "production with the development auth snapshot" "gateway.auth.devSnapshot" \
	--set environment=production

expect_failure "production without the signed snapshot public key" "gateway.auth.publicKey" \
	--set environment=production \
	--set gateway.auth.devSnapshot=false \
	--set gateway.auth.existingSecret=observaki-ingest-auth

expect_failure "production with the bundled development ClickHouse credential" "clickhouse.auth.existingSecret" \
	--set environment=production \
	--set gateway.auth.devSnapshot=false \
	--set gateway.auth.existingSecret=observaki-ingest-auth \
	--set gateway.auth.publicKey=test-public-key

expect_failure "invalid dependencies mode" "dependencies.mode" \
	--set dependencies.mode=whatever

# 4. Optional manifest validation.
if command -v kubectl >/dev/null 2>&1 && kubectl cluster-info >/dev/null 2>&1; then
	if kubectl apply --dry-run=client --validate=false -f "$TMP/bundled.yaml" >/dev/null 2>"$TMP/kubectl.err"; then
		ok "kubectl client dry-run accepts the rendered manifests"
	else
		fail "kubectl client dry-run rejected the rendered manifests"
		cat "$TMP/kubectl.err"
	fi
else
	echo "  skip  kubectl has no reachable cluster (client dry-run skipped)"
fi

if command -v kubeconform >/dev/null 2>&1; then
	if kubeconform -strict -summary "$TMP/bundled.yaml" >/dev/null 2>"$TMP/kubeconform.err"; then
		ok "kubeconform validates the rendered manifests"
	else
		fail "kubeconform rejected the rendered manifests"
		cat "$TMP/kubeconform.err"
	fi
else
	echo "  skip  kubeconform not installed (schema validation skipped)"
fi

echo
if [ "$failures" -gt 0 ]; then
	echo "Helm chart check failed: ${failures} problem(s)."
	exit 1
fi
echo "Helm chart check passed."
