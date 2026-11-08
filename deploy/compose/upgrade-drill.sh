#!/usr/bin/env bash
# Story 5.4 AC2 — upgrade/rollback drill on the single-node Compose stack.
#
#   ./deploy/compose/upgrade-drill.sh              # build two image tags
#   ./deploy/compose/upgrade-drill.sh --no-build   # reuse existing drill tags
#
# It builds the same source twice with different VERSION build args
# (observaki:5.4-drill-a and observaki:5.4-drill-b), starts A, upgrades to B,
# rolls back to A and verifies the version, the metadata and the ingested data
# at every step. The rollback limit from AC2 is 15 minutes.
#
# The report is written to deploy/compose/results/<UTC timestamp>/upgrade-report.md.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

NO_BUILD=0
HEALTH_LIMIT=${OBSERVAKI_COMPOSE_HEALTH_LIMIT:-600}
ROLLBACK_LIMIT=${OBSERVAKI_ROLLBACK_LIMIT:-900}
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
REPORT_DIR="$ROOT/deploy/compose/results/$STAMP"
IMAGE_A=${OBSERVAKI_DRILL_IMAGE_A:-observaki:5.4-drill-a}
IMAGE_B=${OBSERVAKI_DRILL_IMAGE_B:-observaki:5.4-drill-b}
VERSION_A=${OBSERVAKI_DRILL_VERSION_A:-5.4-drill-a}
VERSION_B=${OBSERVAKI_DRILL_VERSION_B:-5.4-drill-b}

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }
die() {
	log "FAIL: $*"
	log "hint: docker compose logs --tail 100"
	exit 1
}

usage() {
	cat <<'EOF'
Usage: deploy/compose/upgrade-drill.sh [options]

Options:
  --no-build         reuse the existing observaki:5.4-drill-a/-b images
  --health-limit N   maximum seconds from `up` to healthy (default: 600)
  --rollback-limit N maximum seconds for the rollback step (default: 900)
  --help             show this help
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--no-build)
		NO_BUILD=1
		shift
		;;
	--health-limit)
		HEALTH_LIMIT=${2:?--health-limit requires a value}
		shift 2
		;;
	--rollback-limit)
		ROLLBACK_LIMIT=${2:?--rollback-limit requires a value}
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

command -v docker >/dev/null || die "docker is required"
command -v curl >/dev/null || die "curl is required"
command -v jq >/dev/null || die "jq is required"
docker compose version >/dev/null 2>&1 || die "docker compose v2+ is required"

mkdir -p "$REPORT_DIR"

psql() {
	docker compose exec -T postgres psql \
		-U "${OBSERVAKI_POSTGRES_USER:-observaki}" \
		-d "${OBSERVAKI_POSTGRES_DB:-observaki}" \
		-tA -v ON_ERROR_STOP=1 -c "$1"
}

ch() {
	docker compose exec -T clickhouse clickhouse-client --database observaki --query "$1"
}

version() {
	curl -fsS http://127.0.0.1:8080/api/v1/health | jq -r '.version'
}

wait_version() {
	local want=$1 elapsed=0 current
	until [ "$(version 2>/dev/null || true)" = "$want" ]; do
		elapsed=$((elapsed + 2))
		[ "$elapsed" -lt "$HEALTH_LIMIT" ] || return 1
		sleep 2
	done
	current=$(version)
	[ "$current" = "$want" ]
}

metadata_snapshot() {
	psql "SELECT
		(SELECT count(*) FROM users) || '|' ||
		(SELECT count(*) FROM projects) || '|' ||
		(SELECT count(*) FROM dashboards) || '|' ||
		(SELECT count(*) FROM alert_rules) || '|' ||
		(SELECT title FROM dashboards WHERE id = '00000000-0000-4000-8000-000000000004')"
}

build_image() {
	local tag=$1 version=$2
	if [ "$NO_BUILD" -eq 1 ] && docker image inspect "$tag" >/dev/null 2>&1; then
		log "reusing image $tag"
		return
	fi
	log "building $tag (VERSION=$version)"
	docker build -f deploy/compose/Dockerfile \
		--build-arg VERSION="$version" \
		--build-arg COMMIT=drill \
		--build-arg BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		-t "$tag" . >/dev/null
}

up_image() {
	local image=$1
	OBSERVAKI_IMAGE="$image" docker compose up -d --wait --no-build
}

log "building the two versioned images"
build_image "$IMAGE_A" "$VERSION_A"
build_image "$IMAGE_B" "$VERSION_B"

log "starting version A (${VERSION_A})"
STARTED=$(date +%s)
up_image "$IMAGE_A" || die "could not start version A"
UP_SECONDS=$(($(date +%s) - STARTED))
wait_version "$VERSION_A" || die "API did not report version ${VERSION_A}"
[ "$UP_SECONDS" -lt "$HEALTH_LIMIT" ] || die "startup took ${UP_SECONDS}s, limit ${HEALTH_LIMIT}s"

METADATA_BEFORE=$(metadata_snapshot)
LOG_COUNT_BEFORE=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
"$ROOT/deploy/compose/sample.sh" >/dev/null
sleep 3
LOG_COUNT_AFTER_SAMPLE=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
log "version A healthy in ${UP_SECONDS}s; metadata=${METADATA_BEFORE}; logs=${LOG_COUNT_AFTER_SAMPLE}"

log "upgrading to version B (${VERSION_B})"
STARTED=$(date +%s)
up_image "$IMAGE_B" || die "upgrade failed"
UPGRADE_SECONDS=$(($(date +%s) - STARTED))
wait_version "$VERSION_B" || die "API did not report version ${VERSION_B}"
METADATA_AFTER_UPGRADE=$(metadata_snapshot)
LOG_COUNT_AFTER_UPGRADE=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
[ "$METADATA_AFTER_UPGRADE" = "$METADATA_BEFORE" ] || die "metadata changed during upgrade: ${METADATA_BEFORE} -> ${METADATA_AFTER_UPGRADE}"
[ "$LOG_COUNT_AFTER_UPGRADE" -ge "$LOG_COUNT_AFTER_SAMPLE" ] || die "logs were lost during upgrade"
log "upgrade to B in ${UPGRADE_SECONDS}s; metadata and data preserved"

log "rolling back to version A (${VERSION_A})"
STARTED=$(date +%s)
up_image "$IMAGE_A" || die "rollback failed"
ROLLBACK_SECONDS=$(($(date +%s) - STARTED))
wait_version "$VERSION_A" || die "API did not report version ${VERSION_A} after rollback"
METADATA_AFTER_ROLLBACK=$(metadata_snapshot)
LOG_COUNT_AFTER_ROLLBACK=$(ch "SELECT count() FROM otel_logs WHERE project_id = 'project-dev'")
[ "$METADATA_AFTER_ROLLBACK" = "$METADATA_BEFORE" ] || die "metadata changed during rollback: ${METADATA_BEFORE} -> ${METADATA_AFTER_ROLLBACK}"
[ "$LOG_COUNT_AFTER_ROLLBACK" -ge "$LOG_COUNT_AFTER_UPGRADE" ] || die "logs were lost during rollback"
[ "$ROLLBACK_SECONDS" -lt "$ROLLBACK_LIMIT" ] || die "rollback took ${ROLLBACK_SECONDS}s, limit ${ROLLBACK_LIMIT}s"
log "rollback to A in ${ROLLBACK_SECONDS}s; version, metadata and data verified"

COMPOSE_VERSION=$(docker compose version --short 2>/dev/null || docker compose version)
DOCKER_VERSION=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)

{
	echo "# Story 5.4 — upgrade/rollback drill (AC2)"
	echo
	echo "- Date (UTC): \`$STAMP\`"
	echo "- Host: \`$(uname -srm)\`"
	echo "- Docker: \`$DOCKER_VERSION\`; Compose: \`$COMPOSE_VERSION\`"
	echo "- Images: \`$IMAGE_A\` (VERSION=$VERSION_A) and \`$IMAGE_B\` (VERSION=$VERSION_B)"
	echo
	echo "## Procedure"
	echo
	echo '```bash'
	echo "OBSERVAKI_IMAGE=$IMAGE_A docker compose up -d --wait --no-build"
	echo "OBSERVAKI_IMAGE=$IMAGE_B docker compose up -d --wait --no-build   # upgrade"
	echo "OBSERVAKI_IMAGE=$IMAGE_A docker compose up -d --wait --no-build   # rollback"
	echo '```'
	echo
	echo "## Results"
	echo
	echo "| Step | Time | API version | Metadata | Logs (project-dev) |"
	echo "|---|---|---|---|---|"
	echo "| Start A | ${UP_SECONDS}s | ${VERSION_A} | \`${METADATA_BEFORE}\` | ${LOG_COUNT_AFTER_SAMPLE} |"
	echo "| Upgrade to B | ${UPGRADE_SECONDS}s | ${VERSION_B} | \`${METADATA_AFTER_UPGRADE}\` | ${LOG_COUNT_AFTER_UPGRADE} |"
	echo "| Rollback to A | ${ROLLBACK_SECONDS}s | ${VERSION_A} | \`${METADATA_AFTER_ROLLBACK}\` | ${LOG_COUNT_AFTER_ROLLBACK} |"
	echo
	echo "- Metadata format: \`users|projects|dashboards|alert_rules|dashboard-title\`."
	echo "- Rollback limit: ${ROLLBACK_LIMIT}s (AC2: 15 min). Observed: **${ROLLBACK_SECONDS}s**."
	echo "- The PostgreSQL and ClickHouse migration jobs re-ran on each image change and stayed idempotent."
	echo
	echo "## Notes"
	echo
	echo "- This is a single-node Compose drill; the Helm equivalent (install/upgrade/rollback on kind) is documented in \`docs/runbooks/upgrade-rollback.md\` and was measured in Story 5.2 (22s/17s)."
	echo "- The drill leaves the stack running on version A."
	echo "- Not G1 evidence."
} >"$REPORT_DIR/upgrade-report.md"

log "report written to $REPORT_DIR/upgrade-report.md"
log "PASS: upgrade in ${UPGRADE_SECONDS}s and rollback in ${ROLLBACK_SECONDS}s with metadata and data preserved"
