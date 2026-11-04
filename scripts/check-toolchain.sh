#!/usr/bin/env bash
# Validate the toolchain required by Observaki Phase 0.
#
# Exit codes: 0 = ready, 1 = a required tool is missing or too old.
set -uo pipefail

failures=0
warnings=0

ok() { printf '  ok    %s\n' "$1"; }
warn() {
	printf '  warn  %s\n' "$1"
	warnings=$((warnings + 1))
}
fail() {
	printf '  FAIL  %s\n' "$1"
	failures=$((failures + 1))
}

have() { command -v "$1" >/dev/null 2>&1; }

# version_ge <actual> <required> — true when actual >= required (GNU sort -V).
version_ge() {
	[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
}

strip_v() { printf '%s' "${1#go}" | sed 's/^v//'; }

echo "Checking the Observaki toolchain..."

# Go: validate the effective toolchain, which may be selected by go.mod.
if have go; then
	go_version="$(strip_v "$(go env GOVERSION 2>/dev/null || go version | awk '{print $3}')")"
	if version_ge "$go_version" "1.24"; then
		ok "Go ${go_version} (>= 1.24)"
	else
		fail "Go ${go_version} is too old (>= 1.24 required). Install Go 1.24+ or set GOTOOLCHAIN=auto."
	fi
else
	fail "Go not found. Install Go 1.24+."
fi

# Node: 26 LTS.
if have node; then
	node_version="$(strip_v "$(node --version)")"
	if version_ge "$node_version" "26.0.0"; then
		ok "Node ${node_version} (>= 26)"
	else
		fail "Node ${node_version} is too old (>= 26 required). Run: nvm install 26 && nvm use 26"
	fi
else
	fail "Node not found. Install Node 26 LTS."
fi

# pnpm.
if have pnpm; then
	pnpm_version="$(pnpm --version 2>/dev/null)"
	if version_ge "$pnpm_version" "10.0.0"; then
		ok "pnpm ${pnpm_version} (>= 10)"
	else
		fail "pnpm ${pnpm_version} is too old (>= 10 required). Run: corepack enable pnpm"
	fi
else
	fail "pnpm not found. Run: corepack enable pnpm"
fi

# Rust: stable toolchain plus rustfmt and clippy.
if have cargo; then
	cargo_version="$(cargo --version | awk '{print $2}')"
	if version_ge "$cargo_version" "1.85.0"; then
		ok "Rust ${cargo_version} (>= 1.85)"
	else
		fail "Rust ${cargo_version} is too old (>= 1.85 required). Run: rustup update stable"
	fi
	have rustfmt || fail "rustfmt not found. Run: rustup component add rustfmt"
	have clippy-driver || have cargo-clippy || fail "clippy not found. Run: rustup component add clippy"
else
	fail "cargo not found. Install Rust 1.85+ (stable) with rustfmt and clippy."
fi

# golangci-lint is required by `make lint`.
if have golangci-lint || [ -x "$(go env GOPATH 2>/dev/null)/bin/golangci-lint" ]; then
	ok "golangci-lint available"
else
	fail "golangci-lint not found. Run: make tools (or: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0)"
fi

# Docker Compose v2 is only needed from Story 5.1 onwards.
if have docker && docker compose version >/dev/null 2>&1; then
	compose_version="$(docker compose version | grep -oE 'v?[0-9]+\.[0-9]+\.[0-9]+' | head -n1 | tr -d v)"
	if version_ge "$compose_version" "2.0.0"; then
		ok "Docker Compose ${compose_version} (>= 2)"
	else
		warn "Docker Compose ${compose_version} is older than v2 (not required until Story 5.1)"
	fi
else
	warn "Docker Compose v2 not found (not required until Story 5.1)"
fi

# Helm is needed from Story 5.2 onwards (chart lint and the kind smoke).
if have helm; then
	helm_version="$(helm version --template '{{ .Version }}' 2>/dev/null | sed 's/^v//')"
	if version_ge "$helm_version" "4.3.0"; then
		ok "Helm ${helm_version} (>= 4.3)"
	else
		warn "Helm ${helm_version} is older than 4.3 (required by the alpha chart)"
	fi
else
	warn "Helm not found (required for make chart-check / chart-smoke)"
fi

# kubectl and kind are only needed for the Helm end-to-end smoke.
have kubectl || warn "kubectl not found (required for make chart-smoke)"
have kind || warn "kind not found (required for make chart-smoke)"

# Basic utilities used by the scripts.
have make || fail "make not found."
have curl || fail "curl not found."

echo
if [ "$failures" -gt 0 ]; then
	echo "Toolchain not ready: ${failures} required check(s) failed, ${warnings} warning(s)."
	exit 1
fi

echo "Toolchain ready (${warnings} warning(s))."
