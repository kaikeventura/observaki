#!/usr/bin/env bash
# Story 5.4 operator drill on the single-node Compose stack.
#
#   ./deploy/compose/operator-smoke.sh              # full run (builds the image)
#   ./deploy/compose/operator-smoke.sh --no-build   # reuse observaki:local
#
# Verifies:
#   AC1  metadata backup/restore of projects, users, dashboards and alert rules
#   AC3  health page data: ingestion lag, query error rate, storage usage and
#        component health
#   AC4  threshold alert on ingestion lag: state change, dedupe, silence and a
#        webhook notification received by a local sink
#
# It requires the same tools as the Story 5.1 smoke: docker, docker compose v2+,
# curl, jq and psql (inside the postgres container, no host install). The AC2
# upgrade/rollback drill is deploy/compose/upgrade-drill.sh.
#
# The report is written to deploy/compose/results/<UTC timestamp>/operator-report.md.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

BUILD=1
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
REPORT_DIR="$ROOT/deploy/compose/results/$STAMP"
SINK_PORT=${OBSERVAKI_WEBHOOK_PORT:-19090}
SINK_OUT="$REPORT_DIR/webhooks.jsonl"
SINK_LOG="$REPORT_DIR/webhooksink.log"
DEADLINE=${OBSERVAKI_OPERATOR_DEADLINE:-120}
SINK_PID=""

DRILL_RULE_ID="00000000-0000-4000-8000-0000000000b1"
DRILL_SILENCE_ID="00000000-0000-4000-8000-0000000000b2"
SEED_RULE_ID="00000000-0000-4000-8000-000000000006"

log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*" >&2; }
die() {
	log "FAIL: $*"
	log "hint: docker compose logs --tail 100 observaki-api"
	exit 1
}

usage() {
	cat <<'EOF'
Usage: deploy/compose/operator-smoke.sh [options]

Options:
  --no-build         do not rebuild the observaki image
  --deadline N       seconds to wait for each health/data condition (default: 120)
  --help             show this help

Requires the stack to be startable with docker compose. The report is written to
deploy/compose/results/<UTC timestamp>/operator-report.md.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--no-build)
		BUILD=0
		shift
		;;
	--deadline)
		DEADLINE=${2:?--deadline requires a value}
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

cleanup() {
	if [ -n "$SINK_PID" ]; then
		kill "$SINK_PID" 2>/dev/null || true
		wait "$SINK_PID" 2>/dev/null || true
	fi
}
trap cleanup EXIT

psql() {
	docker compose exec -T postgres psql \
		-U "${OBSERVAKI_POSTGRES_USER:-observaki}" \
		-d "${OBSERVAKI_POSTGRES_DB:-observaki}" \
		-tA -v ON_ERROR_STOP=1 -c "$1"
}

# psql_file reads SQL from stdin, which keeps multi-statement setup atomic.
psql_file() {
	docker compose exec -T postgres psql \
		-U "${OBSERVAKI_POSTGRES_USER:-observaki}" \
		-d "${OBSERVAKI_POSTGRES_DB:-observaki}" \
		-tA -v ON_ERROR_STOP=1 -f -
}

overview() {
	curl -fsS http://127.0.0.1:8080/api/v1/health/overview
}

wait_for() {
	local description=$1 elapsed=0
	shift
	until "$@"; do
		elapsed=$((elapsed + 2))
		if [ "$elapsed" -ge "$DEADLINE" ]; then
			die "timed out after ${DEADLINE}s waiting for ${description}"
		fi
		sleep 2
	done
}

wait_for_overview() {
	local filter=$1
	overview | jq -e "$filter" >/dev/null 2>&1
}

infra_ready() {
	local state
	state=$(docker compose ps -a --format '{{.Service}}|{{.State}}|{{.Health}}|{{.ExitCode}}')
	for service in \
		clickhouse victoriametrics postgres nats \
		observaki-api observaki-gateway observaki-writer; do
		printf '%s\n' "$state" | awk -F'|' -v s="$service" \
			'$1 == s && $2 == "running" && $3 == "healthy" { found = 1 } END { exit found ? 0 : 1 }' ||
			return 1
	done
	printf '%s\n' "$state" | awk -F'|' \
		'$1 == "clickhouse-migrate" && $2 == "exited" && $4 == "0" { found = 1 } END { exit found ? 0 : 1 }' ||
		return 1
	printf '%s\n' "$state" | awk -F'|' \
		'$1 == "postgres-migrate" && $2 == "exited" && $4 == "0" { found = 1 } END { exit found ? 0 : 1 }'
}

metadata_counts() {
	psql "SELECT
		(SELECT count(*) FROM users) || '|' ||
		(SELECT count(*) FROM projects) || '|' ||
		(SELECT count(*) FROM dashboards) || '|' ||
		(SELECT count(*) FROM alert_rules) || '|' ||
		(SELECT count(*) FROM notification_channels) || '|' ||
		(SELECT count(*) FROM alert_rule_channels) || '|' ||
		(SELECT count(*) FROM silences) || '|' ||
		(SELECT count(*) FROM slos) || '|' ||
		(SELECT count(*) FROM api_tokens)"
}

alert_state() {
	psql "SELECT state FROM alert_states WHERE rule_id = '${DRILL_RULE_ID}'"
}

is_state() {
	[ "$(alert_state)" = "$1" ]
}

log "starting the single-node stack (build=${BUILD})"
up_args=(up -d --wait)
if [ "$BUILD" -eq 1 ]; then
	up_args+=(--build)
fi
docker compose "${up_args[@]}" || die "docker compose up failed"
infra_ready || {
	docker compose ps -a >&2
	die "the stack did not become healthy"
}
log "stack healthy"

# ---------------------------------------------------------------------------
# AC3: auto-observability
# ---------------------------------------------------------------------------
log "AC3: waiting for the health overview"
wait_for "the health overview endpoint" curl -fsS -o /dev/null http://127.0.0.1:8080/api/v1/health/overview
wait_for "the data-plane components" wait_for_overview '.components | map(.name) | (
	index("api") != null and index("gateway") != null and index("writer") != null and
	index("nats") != null and index("postgres") != null and index("clickhouse") != null and
	index("victoriametrics") != null and index("alerting") != null)'
wait_for "the internal alerting heartbeat" wait_for_overview '.alerting.enabled == true and .alerting.last_evaluation_at != null'

log "AC3: checking that the health page route is served"
curl -fsS http://127.0.0.1:8080/health | grep -q 'Observaki' ||
	die "the /health route did not return the app shell"
[ -f web/build/health.html ] || die "web/build/health.html is missing"

log "AC3: sending the sample telemetry"
"$ROOT/deploy/compose/sample.sh" >/dev/null
wait_for "ingestion lag in the overview" wait_for_overview '.ingestion.lag_p95_seconds != null and .ingestion.written_events != null'
wait_for "storage usage from ClickHouse and VictoriaMetrics" wait_for_overview \
	'.storage.clickhouse_bytes != null and .storage.victoriametrics_series != null'
wait_for "a healthy ingestion path" wait_for_overview '.ingestion.status == "healthy"'

# Exercise the query class counter (the real Query Service is Phase 1).
curl -s -o /dev/null -w '' http://127.0.0.1:8080/api/v1/query || true
wait_for "the query error rate" wait_for_overview '.query.requests >= 1 and .query.errors >= 1 and .query.error_rate > 0'
AC3_OVERVIEW=$(overview)
log "AC3: overview collected"

# ---------------------------------------------------------------------------
# AC1: metadata backup/restore
# ---------------------------------------------------------------------------
# Remove leftovers from an interrupted drill so the backup represents the
# clean seeded metadata.
psql_file <<SQL
DELETE FROM alert_rules WHERE id = '${DRILL_RULE_ID}';
DELETE FROM silences WHERE id = '${DRILL_SILENCE_ID}';
UPDATE alert_rules SET enabled = true WHERE id = '${SEED_RULE_ID}';
SQL

log "AC1: collecting metadata counts"
COUNTS_BEFORE=$(metadata_counts)
BACKUP_FILE="$REPORT_DIR/metadata-backup.json"
BACKUP_START=$(date +%s)
docker compose exec -T observaki-api /observaki backup --output - >"$BACKUP_FILE" ||
	die "observaki backup failed"
BACKUP_SECONDS=$(($(date +%s) - BACKUP_START))

jq -e '.format_version == 1 and .counts.users >= 1 and .counts.projects >= 1 and
	.counts.dashboards >= 1 and .counts.alert_rules >= 1' "$BACKUP_FILE" >/dev/null ||
	die "backup document is incomplete"
if grep -q 'observaki-local-dev-token' "$BACKUP_FILE"; then
	die "backup contains the plaintext development token"
fi

log "AC1: mutating and restoring the metadata"
psql_file <<'SQL'
DELETE FROM dashboards;
UPDATE projects SET name = 'Corrupted by the drill';
DELETE FROM alert_rule_channels;
SQL
[ "$(metadata_counts)" != "$COUNTS_BEFORE" ] || die "the mutation did not change the metadata"

RESTORE_START=$(date +%s)
docker compose exec -T observaki-api /observaki restore --input - --force <"$BACKUP_FILE" ||
	die "observaki restore failed"
RESTORE_SECONDS=$(($(date +%s) - RESTORE_START))

COUNTS_AFTER=$(metadata_counts)
[ "$COUNTS_AFTER" = "$COUNTS_BEFORE" ] || die "metadata counts changed: ${COUNTS_BEFORE} -> ${COUNTS_AFTER}"
RESTORED_PROJECT=$(psql "SELECT name FROM projects WHERE id = '00000000-0000-4000-8000-000000000002'")
RESTORED_DASHBOARD=$(psql "SELECT title FROM dashboards WHERE id = '00000000-0000-4000-8000-000000000004'")
[ "$RESTORED_PROJECT" = "Development" ] || die "project was not restored: ${RESTORED_PROJECT}"
[ "$RESTORED_DASHBOARD" = "Development overview" ] || die "dashboard was not restored: ${RESTORED_DASHBOARD}"
log "AC1: restored in ${RESTORE_SECONDS}s (backup ${BACKUP_SECONDS}s)"

# ---------------------------------------------------------------------------
# AC4: threshold alert, dedupe, silence and webhook delivery
# ---------------------------------------------------------------------------
log "AC4: starting the local webhook sink on 0.0.0.0:${SINK_PORT}"
# The API container reaches the sink through host.docker.internal (the bridge
# gateway), so the drill receiver binds all interfaces on a high port.
./bin/webhooksink --addr "0.0.0.0:${SINK_PORT}" --out "$SINK_OUT" >"$SINK_LOG" 2>&1 &
SINK_PID=$!
sleep 0.2
if ! kill -0 "$SINK_PID" 2>/dev/null; then
	cat "$SINK_LOG" >&2 || true
	die "could not start bin/webhooksink (run make build first)"
fi
wait_for "the webhook sink" curl -fsS -o /dev/null "http://127.0.0.1:${SINK_PORT}/healthz"

log "AC4: creating a deterministic drill rule and channel route"
psql_file <<SQL
INSERT INTO alert_rules (id, project_id, name, signal, query, condition, for_seconds, severity, enabled)
VALUES ('${DRILL_RULE_ID}', '00000000-0000-4000-8000-000000000002', 'Story 5.4 drill', 'metrics',
        'ingest_lag_p95_seconds', '{"op":">=","threshold":0}'::jsonb, 0, 'warning', true)
ON CONFLICT (id) DO UPDATE SET enabled = true, condition = '{"op":">=","threshold":0}'::jsonb, for_seconds = 0;
INSERT INTO alert_rule_channels (rule_id, channel_id)
VALUES ('${DRILL_RULE_ID}', '00000000-0000-4000-8000-000000000005')
ON CONFLICT DO NOTHING;
UPDATE alert_rules SET enabled = false WHERE id = '${SEED_RULE_ID}';
DELETE FROM alert_states WHERE rule_id = '${DRILL_RULE_ID}';
DELETE FROM alert_outbox WHERE rule_id = '${DRILL_RULE_ID}';
SQL

DRILL_STARTED=$(date +%s)
wait_for "the drill alert to fire" bash -c \
	"grep -q '\"rule_name\":\"Story 5.4 drill\"' '$SINK_OUT' 2>/dev/null"
NOTIFICATIONS_AFTER_FIRING=$(grep -c '"rule_name":"Story 5.4 drill"' "$SINK_OUT" || true)
[ "$NOTIFICATIONS_AFTER_FIRING" -eq 1 ] || die "expected exactly 1 firing notification, got ${NOTIFICATIONS_AFTER_FIRING}"
STATE=$(psql "SELECT state FROM alert_states WHERE rule_id = '${DRILL_RULE_ID}'")
[ "$STATE" = "firing" ] || die "alert state = ${STATE}, want firing"

log "AC4: verifying dedupe while the alert stays firing"
sleep 8
DEDUPED=$(grep -c '"rule_name":"Story 5.4 drill"' "$SINK_OUT" || true)
[ "$DEDUPED" -eq 1 ] || die "duplicate notification delivered: ${DEDUPED}"

log "AC4: silencing and resolving the drill alert"
psql_file <<SQL
INSERT INTO silences (id, project_id, matchers, starts_at, ends_at, reason, created_by)
VALUES ('${DRILL_SILENCE_ID}', '00000000-0000-4000-8000-000000000002',
        '{"rule_name":"Story 5.4 drill"}'::jsonb, now() - interval '1 minute', now() + interval '1 hour',
        'Story 5.4 drill', '00000000-0000-4000-8000-000000000003')
ON CONFLICT (id) DO UPDATE SET matchers = EXCLUDED.matchers, starts_at = EXCLUDED.starts_at, ends_at = EXCLUDED.ends_at;
UPDATE alert_rules SET condition = '{"op":">","threshold":1000000}'::jsonb WHERE id = '${DRILL_RULE_ID}';
SQL
wait_for "the drill alert to resolve" is_state resolved
NOTIFICATIONS_AFTER_RESOLVE=$(grep -c '"rule_name":"Story 5.4 drill"' "$SINK_OUT" || true)
[ "$NOTIFICATIONS_AFTER_RESOLVE" -eq 1 ] || die "silenced resolution was delivered: ${NOTIFICATIONS_AFTER_RESOLVE}"

log "AC4: re-firing under the silence"
psql "UPDATE alert_rules SET condition = '{\"op\":\">=\",\"threshold\":0}'::jsonb WHERE id = '${DRILL_RULE_ID}'" >/dev/null
wait_for "the suppressed firing state" is_state firing
sleep 6
NOTIFICATIONS_AFTER_REFIRE=$(grep -c '"rule_name":"Story 5.4 drill"' "$SINK_OUT" || true)
[ "$NOTIFICATIONS_AFTER_REFIRE" -eq 1 ] || die "silenced firing was delivered: ${NOTIFICATIONS_AFTER_REFIRE}"
SUPPRESSED_ROWS=$(psql "SELECT count(*) FROM alert_outbox WHERE rule_id = '${DRILL_RULE_ID}' AND suppressed")
[ "$SUPPRESSED_ROWS" -ge 1 ] || die "no suppressed outbox rows were recorded"
DRILL_SECONDS=$(($(date +%s) - DRILL_STARTED))
log "AC4: delivered, deduped and silenced"

log "AC4: cleaning up the drill rule"
psql_file <<SQL
DELETE FROM alert_rules WHERE id = '${DRILL_RULE_ID}';
DELETE FROM silences WHERE id = '${DRILL_SILENCE_ID}';
UPDATE alert_rules SET enabled = true WHERE id = '${SEED_RULE_ID}';
SQL
cleanup
SINK_PID=""

# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------
AC4_PAYLOAD=$(head -n 1 "$SINK_OUT" | jq -c '.body' 2>/dev/null || echo '{}')
COMPONENTS=$(printf '%s' "$AC3_OVERVIEW" | jq -r '[.components[] | "\(.name)=\(.status)"] | join(", ")')
COMPOSE_VERSION=$(docker compose version --short 2>/dev/null || docker compose version)
DOCKER_VERSION=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)

{
	echo "# Story 5.4 — backup/restore, auto-observability and alerting drill"
	echo
	echo "- Date (UTC): \`$STAMP\`"
	echo "- Host: \`$(uname -srm)\`"
	echo "- Docker: \`$DOCKER_VERSION\`; Compose: \`$COMPOSE_VERSION\`"
	echo "- Driver: \`deploy/compose/operator-smoke.sh\` (build=$BUILD)"
	echo
	echo "## AC3 — auto-observability"
	echo
	echo "- Health overview: \`GET /api/v1/health/overview\` returned status **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.status')**."
	echo "- Components: ${COMPONENTS}."
	echo "- Ingestion: lag p95 **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.ingestion.lag_p95_seconds')s**, pending **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.ingestion.pending_messages')**, confirmed **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.ingestion.written_events')** events."
	echo "- Query error rate: **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.query.error_rate * 100 | tostring + "%"')** ($(printf '%s' "$AC3_OVERVIEW" | jq -r '.query.errors') server errors over $(printf '%s' "$AC3_OVERVIEW" | jq -r '.query.requests') requests)."
	echo "- Storage: ClickHouse **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.storage.clickhouse_bytes') bytes** / **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.storage.clickhouse_parts') parts**; VictoriaMetrics **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.storage.victoriametrics_bytes') bytes** / **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.storage.victoriametrics_series') series**."
	echo "- Alerting heartbeat: **$(printf '%s' "$AC3_OVERVIEW" | jq -r '.alerting.last_evaluation_at')**."
	echo "- The health page is served at \`/health\` (client-rendered SPA; \`web/build/health.html\`); the vitest suite covers the four AC3 sections and the error state. It polls the endpoint every 10 s and keeps the last known value when a source is down."
	echo
	echo "## AC1 — backup/restore"
	echo
	echo "- Backup: **${BACKUP_SECONDS}s** to \`metadata-backup.json\` (format 1); restore: **${RESTORE_SECONDS}s** with \`--force\`."
	echo "- Metadata counts: \`users|projects|dashboards|alert_rules|channels|routes|silences|slos|tokens\` = \`${COUNTS_BEFORE}\` before and \`${COUNTS_AFTER}\` after."
	echo "- Project name and dashboard title were restored; the backup contains only the token hash, never the plaintext."
	echo "- Limit: 30 min (AC1); the observed restore is well below it."
	echo
	echo "## AC4 — threshold alert, dedupe and silence"
	echo
	echo "- Threshold rule \`Story 5.4 drill\` (\`ingest_lag_p95_seconds >= 0\`, \`for=0\`) fired and delivered **${NOTIFICATIONS_AFTER_FIRING}** webhook notification; the full dedupe/silence drill took **${DRILL_SECONDS}s**."
	echo "- While the state stayed firing, the notification count stayed at **${DEDUPED}** (outbox idempotency key)."
	echo "- A silence suppressed the resolution and the re-fire; the sink count stayed at **${NOTIFICATIONS_AFTER_REFIRE}** and the outbox recorded **${SUPPRESSED_ROWS}** suppressed rows."
	echo "- Payload: \`${AC4_PAYLOAD}\`"
	echo
	echo "## Notes"
	echo
	echo "- This is a single-node evaluation drill, not G1 evidence."
	echo "- AC2 (upgrade/rollback) is measured separately by \`deploy/compose/upgrade-drill.sh\`; the Helm runbook is in \`docs/runbooks/upgrade-rollback.md\`."
	echo "- The Query Service is Phase 1; the query error rate currently counts the API query class (the \`/api/v1/query\` placeholder returns 501)."
} >"$REPORT_DIR/operator-report.md"

log "report written to $REPORT_DIR/operator-report.md"
log "PASS: AC1 backup/restore, AC3 health overview and AC4 alert/dedupe/silence verified"
