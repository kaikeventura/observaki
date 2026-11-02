#!/usr/bin/env bash
# Stable entry point for the Observaki benchmark scenarios.
set -euo pipefail
cd "$(dirname "$0")/.."

if [ -x bin/benchctl ]; then
	exec ./bin/benchctl "$@"
fi

exec go run ./bench/cmd/benchctl "$@"
