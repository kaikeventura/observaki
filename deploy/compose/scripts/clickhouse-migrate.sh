#!/bin/sh
# Apply the versioned ClickHouse migrations to the single-node stack.
#
# The migrations are additive and idempotent (CREATE ... IF NOT EXISTS,
# ADD COLUMN IF NOT EXISTS), so this runs on every `docker compose up`:
# a fresh volume gets the schema and an existing volume is left untouched.
# That is what keeps AC3 ("no manual migration required") true across
# `docker compose down` / `up`.
set -eu

host=${CLICKHOUSE_HOST:-clickhouse}
database=${CLICKHOUSE_DATABASE:-observaki}
migrations=${MIGRATIONS_DIR:-/migrations}

until clickhouse-client --host "$host" --query 'SELECT 1' >/dev/null 2>&1; do
	echo "waiting for ClickHouse at ${host}" >&2
	sleep 1
done

clickhouse-client --host "$host" --query "CREATE DATABASE IF NOT EXISTS \`${database}\`"

found=0
for file in "$migrations"/*.sql; do
	[ -e "$file" ] || continue
	found=$((found + 1))
	echo "applying $(basename "$file")" >&2
	clickhouse-client --host "$host" --database "$database" --multiquery <"$file"
done

if [ "$found" -eq 0 ]; then
	echo "no migrations found in ${migrations}" >&2
	exit 1
fi

echo "applied ${found} migrations to ${database}" >&2
