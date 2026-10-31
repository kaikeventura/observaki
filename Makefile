SHELL := /bin/bash
.DEFAULT_GOAL := help

GO ?= go
PNPM ?= pnpm
CARGO ?= cargo
GIT ?= git

BIN_DIR := bin
BIN := $(BIN_DIR)/observaki
WEB_DIR := web
AGENT_MANIFEST := agent/Cargo.toml

VERSION ?= $(shell $(GIT) describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell $(GIT) rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/observaki/observaki/internal/version.Version=$(VERSION) \
	-X github.com/observaki/observaki/internal/version.Commit=$(COMMIT) \
	-X github.com/observaki/observaki/internal/version.Date=$(BUILD_DATE)

# golangci-lint may be installed outside PATH (e.g. $(go env GOPATH)/bin).
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || printf '%s/bin/golangci-lint' "$$(go env GOPATH)")

.PHONY: help bootstrap check-toolchain tools fmt lint test build dev smoke clean ci

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

lint: ## run golangci-lint, prettier, svelte-check and clippy
	$(GOLANGCI_LINT) run ./...
	$(PNPM) --dir $(WEB_DIR) lint
	$(PNPM) --dir $(WEB_DIR) check
	$(CARGO) clippy --manifest-path $(AGENT_MANIFEST) --all-targets -- -D warnings

test: ## run Go, web and Rust tests
	$(GO) test ./...
	$(PNPM) --dir $(WEB_DIR) test
	$(CARGO) test --manifest-path $(AGENT_MANIFEST)

build: ## build the Go binary, the web app and the agent
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/observaki
	$(PNPM) --dir $(WEB_DIR) build
	$(CARGO) build --release --manifest-path $(AGENT_MANIFEST)

dev: ## run the Go API (:8080) and the SvelteKit dev server (:5173)
	./scripts/dev.sh

smoke: ## run the full CI suite and smoke-test the built binary
	./scripts/smoke.sh

clean: ## remove build artifacts
	rm -rf $(BIN_DIR) $(WEB_DIR)/build $(WEB_DIR)/.svelte-kit agent/target

ci: bootstrap lint test build ## run the full local CI suite
