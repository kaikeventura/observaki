#!/usr/bin/env bash
# Sync the files that the Helm chart must embed.
#
# Helm cannot read files outside the chart directory, so the ClickHouse
# migrations and the public development ingest snapshot are copied into
# deploy/helm/observaki/files/. Run this after changing either source; CI runs
# deploy/helm/check.sh and fails when the copies drift.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$ROOT"

install -d deploy/helm/observaki/files/migrations
rm -f deploy/helm/observaki/files/migrations/*.sql
cp internal/adapters/clickhouse/migrations/*.sql deploy/helm/observaki/files/migrations/
cp deploy/compose/config/ingest-auth.json deploy/helm/observaki/files/ingest-auth-dev.json

echo "synced $(find deploy/helm/observaki/files/migrations -name '*.sql' | wc -l | tr -d ' ') migrations and the development ingest snapshot"
