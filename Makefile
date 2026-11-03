SHELL := /bin/bash
.DEFAULT_GOAL := help

GO ?= go
PNPM ?= pnpm
CARGO ?= cargo
GIT ?= git

BIN_DIR := bin
BIN := $(BIN_DIR)/observaki
LOADGEN := $(BIN_DIR)/otlploadgen
BENCHCTL := $(BIN_DIR)/benchctl
BENCH_SINK := $(BIN_DIR)/bench-sink
CHLOADGEN := $(BIN_DIR)/chloadgen
VM_LOADGEN := $(BIN_DIR)/vmloadgen
VM_QUERYBENCH := $(BIN_DIR)/vmquerybench
VM_ADMISSION := $(BIN_DIR)/vmadmission
WEB_DIR := web
AGENT_MANIFEST := agent/Cargo.toml
ROWS ?= 1000000000
CHUNK_ROWS ?= 50000000
RUNS ?= 20
VM_SERIES ?= 1000000
VM_RATE ?= 1000000
VM_DURATION ?= 1h
VM_RUNS ?= 5

VERSION ?= $(shell $(GIT) describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell $(GIT) rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/observaki/observaki/internal/version.Version=$(VERSION) \
	-X github.com/observaki/observaki/internal/version.Commit=$(COMMIT) \
	-X github.com/observaki/observaki/internal/version.Date=$(BUILD_DATE)
BENCH_LDFLAGS := -X github.com/observaki/observaki/bench/internal/benchversion.Version=$(VERSION) \
	-X github.com/observaki/observaki/bench/internal/benchversion.Commit=$(COMMIT) \
	-X github.com/observaki/observaki/bench/internal/benchversion.Date=$(BUILD_DATE)

# golangci-lint may be installed outside PATH (e.g. $(go env GOPATH)/bin).
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || printf '%s/bin/golangci-lint' "$$(go env GOPATH)")

.PHONY: help bootstrap check-toolchain tools fmt lint test build dev smoke clean ci adr-check bench-env-check bench-clickhouse bench-clickhouse-check bench-clickhouse-traces bench-clickhouse-traces-check bench-clickhouse-ingest bench-clickhouse-ingest-check bench-clickhouse-query bench-clickhouse-query-check bench-victoriametrics bench-victoriametrics-check bench-gateway-quota bench-gateway-quota-check

help: ## show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

bootstrap: check-toolchain ## validate the toolchain and install dependencies
	$(GO) mod download
	$(PNPM) --dir $(WEB_DIR) install
	$(CARGO) fetch --manifest-path $(AGENT_MANIFEST)
	@if [ -f .env.example ] && [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "created .env from .env.example"; \
	fi

check-toolchain: ## validate the required tool versions
	./scripts/check-toolchain.sh

tools: ## install Go development tools (golangci-lint)
	# Pin the toolchain: the go1.26 build of golangci-lint refuses go.mod 1.27.
	GOTOOLCHAIN=go1.27.2 $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

fmt: ## format Go, web and Rust code
	$(GO) fmt ./...
	$(PNPM) --dir $(WEB_DIR) format
	$(CARGO) fmt --manifest-path $(AGENT_MANIFEST)

adr-check: ## validate ADR sections, status, review and index
	./scripts/check-adr.sh

lint: adr-check ## run golangci-lint, prettier, svelte-check, clippy and ADR checks
	$(GOLANGCI_LINT) run ./...
	$(PNPM) --dir $(WEB_DIR) lint
	$(PNPM) --dir $(WEB_DIR) check
	$(CARGO) clippy --manifest-path $(AGENT_MANIFEST) --all-targets -- -D warnings

test: ## run Go, web and Rust tests
	$(GO) test ./...
	$(PNPM) --dir $(WEB_DIR) test
	$(CARGO) test --manifest-path $(AGENT_MANIFEST)

build: ## build the Go binary, the load generator, the web app and the agent
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/observaki
	$(GO) build -trimpath -ldflags "$(BENCH_LDFLAGS)" -o $(LOADGEN) ./bench/cmd/otlploadgen
	$(GO) build -trimpath -ldflags "$(BENCH_LDFLAGS)" -o $(BENCHCTL) ./bench/cmd/benchctl
	$(GO) build -trimpath -ldflags "$(BENCH_LDFLAGS)" -o $(BENCH_SINK) ./bench/cmd/bench-sink
	$(GO) build -trimpath -ldflags "$(BENCH_LDFLAGS)" -o $(CHLOADGEN) ./bench/cmd/chloadgen
	$(GO) build -trimpath -ldflags "$(BENCH_LDFLAGS)" -o $(VM_LOADGEN) ./bench/cmd/vmloadgen
	$(GO) build -trimpath -ldflags "$(BENCH_LDFLAGS)" -o $(VM_QUERYBENCH) ./bench/cmd/vmquerybench
	$(GO) build -trimpath -ldflags "$(BENCH_LDFLAGS)" -o $(VM_ADMISSION) ./bench/cmd/vmadmission
	$(PNPM) --dir $(WEB_DIR) build
	$(CARGO) build --release --manifest-path $(AGENT_MANIFEST)

dev: ## run the Go API (:8080) and the SvelteKit dev server (:5173)
	./scripts/dev.sh

smoke: ## run the full CI suite and smoke-test the built binary
	./scripts/smoke.sh

clean: ## remove build artifacts
	rm -rf $(BIN_DIR) $(WEB_DIR)/build $(WEB_DIR)/.svelte-kit agent/target

bench-env-check: build ## validate the T1 profile, cost model and Terraform module
	./bin/benchctl env check
	./bin/benchctl env cost --hours 4
	@if command -v terraform >/dev/null 2>&1; then \
		terraform -chdir=bench/env/terraform fmt -check -recursive && \
		terraform -chdir=bench/env/terraform init -backend=false -input=false >/dev/null && \
		terraform -chdir=bench/env/terraform validate; \
	else \
		echo "terraform not installed; skipping Terraform validation"; \
	fi

bench-clickhouse: ## run the ClickHouse logs storage spike (ROWS=1B by default)
	./bench/clickhouse/run.sh --rows $(ROWS) --chunk-rows $(CHUNK_ROWS) --runs $(RUNS) --stop-merges

bench-clickhouse-check: ## validate the ClickHouse logs schema on a 10M-row smoke
	./bench/clickhouse/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5

bench-clickhouse-traces: ## run the ClickHouse traces storage spike (ROWS=1B by default)
	./bench/clickhouse/traces/run.sh --rows $(ROWS) --chunk-rows $(CHUNK_ROWS) --runs $(RUNS) --stop-merges

bench-clickhouse-traces-check: ## validate the ClickHouse traces schema on a 10M-span smoke
	./bench/clickhouse/traces/run.sh --rows 10000000 --chunk-rows 10000000 --runs 5 --stop-merges

bench-clickhouse-ingest: ## run the ClickHouse ingestion spike (400k/s local, 60s)
	./bench/clickhouse/ingest/run.sh

bench-clickhouse-ingest-check: ## run the short ClickHouse ingestion smoke (100k/s, 5s)
	./bench/clickhouse/ingest/run.sh --smoke

bench-clickhouse-query: ## run the query/retention/row-policy spike (1B-event mix)
	./bench/clickhouse/query/run.sh --runs $(RUNS) --stop-merges

bench-clickhouse-query-check: ## validate the query/retention/row-policy spike on a smoke dataset
	./bench/clickhouse/query/run.sh --logs 10000000 --traces 1000000 --runs 5 --stop-merges

bench-victoriametrics: ## run the VictoriaMetrics spike (1M series, 1M samples/s, 1h)
	./bench/victoriametrics/run.sh --reset --series $(VM_SERIES) --rate $(VM_RATE) --duration $(VM_DURATION) --runs $(VM_RUNS)

bench-victoriametrics-check: ## run the short VictoriaMetrics smoke (100k series, 100k/s, 10s)
	./bench/victoriametrics/run.sh --smoke --reset

bench-gateway-quota: ## run the gateway quota isolation workload (100k/s quota, 1M/s noisy tenant)
	./bench/gateway/quota/run.sh

bench-gateway-quota-check: ## run the short gateway auth/quota isolation smoke
	./bench/gateway/quota/run.sh --smoke

ci: bootstrap lint test build ## run the full local CI suite
