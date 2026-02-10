SHELL := /bin/bash

GO ?= go
NODE ?= node
JS_PM ?= yarn
COREPACK_HOME_DIR := $(PWD)/.cache/corepack

ADAPTER_BIN := bin/git-lfs-proton-adapter
BRIDGE_DIR := proton-lfs-bridge
GIT_LFS_DIR := submodules/git-lfs
DRIVE_CLI_DIR := submodules/proton-drive-cli
GO_CACHE_DIR := .cache/go-build

.PHONY: help setup setup-env install-deps \
	build build-adapter build-lfs build-drive-cli build-all \
	test test-adapter test-sdk test-lfs test-integration test-integration-timeout test-integration-stress test-integration-sdk test-integration-sdk-real test-e2e-mock test-e2e-real test-all \
	pass-env check-sdk-prereqs check-sdk-real-prereqs \
	fmt lint lint-go lint-sdk \
	clean status install-hooks

.DEFAULT_GOAL := help

help: ## Show available commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "%-20s %s\n", $$1, $$2}'

setup: setup-env install-deps ## Prepare local environment

setup-env: ## Create .env from .env.example if needed
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "Created .env from .env.example"; \
	else \
		echo ".env already exists"; \
	fi

install-deps: ## Install Go dependencies and JS dependencies (default: yarn via JS_PM)
	$(GO) mod download
	@if [ ! -f package.json ]; then \
		echo "package.json not found; skipped JS dependency install"; \
	elif [ "$(JS_PM)" = "yarn" ] && ! command -v yarn >/dev/null 2>&1; then \
		echo "yarn not found on PATH. Run: corepack enable"; \
		echo "Fallback: make setup JS_PM=npm"; \
		exit 1; \
	elif command -v $(JS_PM) >/dev/null 2>&1; then \
		if [ "$(JS_PM)" = "yarn" ]; then \
			YARN_VERSION="$$(COREPACK_HOME=$(COREPACK_HOME_DIR) yarn --version)"; \
			YARN_MAJOR="$${YARN_VERSION%%.*}"; \
			if [ "$$YARN_MAJOR" -lt 4 ]; then \
				echo "yarn $$YARN_VERSION detected; Yarn 4+ required for this repository."; \
				echo "Run: corepack enable && corepack prepare yarn@4.1.1 --activate"; \
				echo "Fallback: make setup JS_PM=npm"; \
				exit 1; \
			fi; \
			COREPACK_HOME=$(COREPACK_HOME_DIR) $(JS_PM) install; \
		else \
			$(JS_PM) install; \
		fi; \
	else \
		echo "$(JS_PM) not found; skipped JS dependency install"; \
		echo "Install npm/yarn or run with JS_PM=<available-manager>"; \
	fi

build: build-adapter ## Build first-party binaries

build-all: build-adapter build-lfs build-drive-cli ## Build adapter, Git LFS submodule, and proton-drive-cli

build-adapter: ## Build the custom transfer adapter
	@mkdir -p bin
	$(GO) build -trimpath -o $(ADAPTER_BIN) ./cmd/adapter

build-lfs: ## Build Git LFS submodule
	@if [ ! -d $(GIT_LFS_DIR) ]; then \
		echo "$(GIT_LFS_DIR) not found"; \
		exit 1; \
	fi
	@$(MAKE) -C $(GIT_LFS_DIR)

build-drive-cli: ## Build proton-drive-cli TypeScript bridge
	@if [ ! -d $(DRIVE_CLI_DIR) ]; then \
		echo "$(DRIVE_CLI_DIR) not found. Run: git submodule update --init --recursive"; \
		exit 1; \
	fi
	@if [ "$(JS_PM)" = "yarn" ]; then \
		COREPACK_HOME=$(COREPACK_HOME_DIR) $(JS_PM) workspace proton-drive-cli build; \
	else \
		$(JS_PM) --workspace $(DRIVE_CLI_DIR) run build; \
	fi

test: test-adapter ## Run core tests

test-all: test-adapter test-sdk test-lfs test-integration test-e2e-mock ## Run all test suites

test-adapter: ## Run adapter tests
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -race -cover ./cmd/adapter/...

test-sdk: ## Run SDK service tests
	@if [ "$(JS_PM)" = "yarn" ] && ! command -v yarn >/dev/null 2>&1; then \
		echo "yarn not found on PATH. Run: corepack enable"; \
		exit 1; \
	elif ! command -v $(JS_PM) >/dev/null 2>&1; then \
		echo "$(JS_PM) not found"; \
		exit 1; \
	fi
	@if [ "$(JS_PM)" = "yarn" ]; then \
		YARN_VERSION="$$(COREPACK_HOME=$(COREPACK_HOME_DIR) yarn --version)"; \
		YARN_MAJOR="$${YARN_VERSION%%.*}"; \
		if [ "$$YARN_MAJOR" -lt 4 ]; then \
			echo "yarn $$YARN_VERSION detected; Yarn 4+ required. Run: corepack enable && corepack prepare yarn@4.1.1 --activate"; \
			exit 1; \
		fi; \
		COREPACK_HOME=$(COREPACK_HOME_DIR) $(JS_PM) workspace proton-lfs-bridge test --runInBand; \
	else \
		$(JS_PM) --workspace $(BRIDGE_DIR) test -- --runInBand; \
	fi

test-lfs: ## Run Git LFS submodule tests
	@if [ ! -d $(GIT_LFS_DIR) ]; then \
		echo "$(GIT_LFS_DIR) not found"; \
		exit 1; \
	fi
	@$(MAKE) -C $(GIT_LFS_DIR) test

test-integration: ## Run integration tests (requires git + git-lfs binaries)
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/...

test-integration-timeout: ## Run timeout semantics integration tests for stalled adapter behavior
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run '^TestGitLFSCustomTransferTimeout' -v

test-integration-stress: ## Run high-volume concurrency stress/soak integration tests
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run '^TestGitLFSCustomTransferConcurrentStressSoak$$' -v

test-integration-proton-drive-cli: ## Run proton-drive-cli bridge integration tests
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run ProtonDriveCli -v

test-integration-credentials: ## Run credential flow security tests
	@mkdir -p $(GO_CACHE_DIR)
	GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run Credential -v

test-integration-sdk: check-sdk-prereqs ## Run sdk backend integration tests (local service by default, external when PROTON_LFS_BRIDGE_URL is set; use SDK_BACKEND_MODE=real for in-repo real mode)
	@mkdir -p $(GO_CACHE_DIR)
	@if [ -n "$${PROTON_LFS_BRIDGE_URL:-}" ]; then \
		echo "Using external bridge URL: $$PROTON_LFS_BRIDGE_URL"; \
		eval "$$(./scripts/export-pass-env.sh)" && \
			GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run SDK -v; \
	else \
		NODE_BIN_RESOLVED="$$(command -v $(NODE) 2>/dev/null || true)"; \
		if [ -z "$$NODE_BIN_RESOLVED" ] && command -v zsh >/dev/null 2>&1; then \
			NODE_BIN_RESOLVED="$$(zsh -lc 'command -v node' 2>/dev/null || true)"; \
		fi; \
		if [ -z "$$NODE_BIN_RESOLVED" ]; then \
			echo "node not found for sdk integration test"; \
			exit 1; \
		fi; \
		eval "$$(./scripts/export-pass-env.sh)" && \
			NODE_BIN="$$NODE_BIN_RESOLVED" GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run SDK -v; \
	fi

test-integration-sdk-real: check-sdk-real-prereqs ## Run SDK integration tests against external PROTON_LFS_BRIDGE_URL
	@mkdir -p $(GO_CACHE_DIR)
	@echo "Using external bridge URL: $${PROTON_LFS_BRIDGE_URL}"
	@eval "$$(./scripts/export-pass-env.sh)" && \
		GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run SDK -v

test-e2e-mock: build-adapter ## Mocked E2E pipeline (no real credentials)
	@mkdir -p $(GO_CACHE_DIR)
	@chmod +x scripts/mock-pass-cli.sh
	PROTON_PASS_CLI_BIN=$(PWD)/scripts/mock-pass-cli.sh \
		PROTON_DRIVE_CLI_BIN=$(PWD)/$(BRIDGE_DIR)/tests/testdata/mock-proton-drive-cli.js \
		PASS_MOCK_USERNAME=integration-user@proton.test \
		PASS_MOCK_PASSWORD=integration-password \
		SDK_BACKEND_MODE=proton-drive-cli \
		GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run E2EMocked -v

test-e2e-real: build-adapter build-drive-cli ## Real Proton Drive E2E (requires pass-cli login + build-drive-cli)
	@mkdir -p $(GO_CACHE_DIR)
	@eval "$$(./scripts/export-pass-env.sh)" && \
		SDK_BACKEND_MODE=proton-drive-cli \
		GOCACHE=$(PWD)/$(GO_CACHE_DIR) $(GO) test -tags integration ./tests/integration/... -run E2E -v

pass-env: ## Print export commands for Proton Pass-based adapter credentials
	@./scripts/export-pass-env.sh

check-sdk-prereqs: ## Verify prerequisites for sdk integration tests
	@command -v git-lfs >/dev/null 2>&1 || (echo "git-lfs not found on PATH" && exit 1)
	@command -v "$${PROTON_PASS_CLI_BIN:-pass-cli}" >/dev/null 2>&1 || (echo "pass-cli not found on PATH (or PROTON_PASS_CLI_BIN invalid)" && exit 1)
	@if [ "$${SDK_BACKEND_MODE:-local}" = "real" ] || [ "$${SDK_BACKEND_MODE:-local}" = "proton-drive-cli" ]; then \
		if [ -z "$${PROTON_LFS_BRIDGE_URL:-}" ]; then \
			DRIVE_CLI_BIN="$${PROTON_DRIVE_CLI_BIN:-$(DRIVE_CLI_DIR)/dist/index.js}"; \
			if [ ! -f "$$DRIVE_CLI_BIN" ]; then \
				echo "proton-drive-cli not built: $$DRIVE_CLI_BIN not found"; \
				echo "Run: make build-drive-cli"; \
				exit 1; \
			fi; \
		fi; \
	fi
	@if [ -z "$${PROTON_LFS_BRIDGE_URL:-}" ]; then \
		NODE_BIN_RESOLVED="$$(command -v $(NODE) 2>/dev/null || true)"; \
		if [ -z "$$NODE_BIN_RESOLVED" ] && command -v zsh >/dev/null 2>&1; then \
			NODE_BIN_RESOLVED="$$(zsh -lc 'command -v node' 2>/dev/null || true)"; \
		fi; \
		if [ -z "$$NODE_BIN_RESOLVED" ]; then \
			echo "node not found on PATH for non-interactive make shell"; \
			echo "If node is configured in ~/.zshrc (nvm/fnm), run:"; \
			echo "  make test-integration-sdk NODE=/absolute/path/to/node"; \
			exit 1; \
		fi; \
		echo "Resolved node binary: $$NODE_BIN_RESOLVED"; \
	fi
	@if [ -z "$${PROTON_LFS_BRIDGE_URL:-}" ]; then \
		if [ "$(JS_PM)" = "yarn" ] && ! command -v yarn >/dev/null 2>&1; then \
			echo "yarn not found on PATH. Run: corepack enable"; \
			exit 1; \
		elif ! command -v $(JS_PM) >/dev/null 2>&1; then \
			echo "$(JS_PM) not found on PATH"; \
			exit 1; \
		fi; \
	fi
	@if [ -z "$${PROTON_LFS_BRIDGE_URL:-}" ] && [ "$(JS_PM)" = "yarn" ]; then \
		YARN_VERSION="$$(COREPACK_HOME=$(COREPACK_HOME_DIR) yarn --version)"; \
		YARN_MAJOR="$${YARN_VERSION%%.*}"; \
		if [ "$$YARN_MAJOR" -lt 4 ]; then \
			echo "yarn $$YARN_VERSION detected; Yarn 4+ required. Run: corepack enable && corepack prepare yarn@4.1.1 --activate"; \
			exit 1; \
		fi; \
	fi
	@if [ -z "$${PROTON_LFS_BRIDGE_URL:-}" ]; then \
		if [ "$(JS_PM)" = "yarn" ]; then \
			COREPACK_HOME=$(COREPACK_HOME_DIR) $(JS_PM) workspace proton-lfs-bridge node -e "require.resolve('express')" >/dev/null 2>&1 || { \
				echo "JS dependencies for $(BRIDGE_DIR) are missing (cannot resolve express via yarn workspace)."; \
				echo "Run: $(JS_PM) install"; \
				exit 1; \
			}; \
		else \
			$(JS_PM) --workspace $(BRIDGE_DIR) exec -- node -e "require.resolve('express')" >/dev/null 2>&1 || { \
				echo "JS dependencies for $(BRIDGE_DIR) are missing (cannot resolve express via npm workspace)."; \
				echo "Run: $(JS_PM) install"; \
				exit 1; \
			}; \
		fi; \
	else \
		echo "Using external bridge URL: $$PROTON_LFS_BRIDGE_URL"; \
	fi
	@./scripts/export-pass-env.sh >/dev/null
	@echo "SDK integration prerequisites OK"

check-sdk-real-prereqs: ## Verify prerequisites for sdk integration tests against external SDK service
	@if [ -z "$${PROTON_LFS_BRIDGE_URL:-}" ]; then \
		echo "PROTON_LFS_BRIDGE_URL is required for real SDK integration tests"; \
		echo "Example: make test-integration-sdk-real PROTON_LFS_BRIDGE_URL=http://127.0.0.1:3000"; \
		exit 1; \
	fi
	@$(MAKE) check-sdk-prereqs PROTON_LFS_BRIDGE_URL="$${PROTON_LFS_BRIDGE_URL}" JS_PM="$(JS_PM)" NODE="$(NODE)"

fmt: ## Format Go code
	$(GO) fmt ./cmd/...

lint: lint-go lint-sdk ## Run lint checks

lint-go: ## Run Go vet and golangci-lint when available
	$(GO) vet ./cmd/adapter/...
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./cmd/adapter/...; \
	else \
		echo "golangci-lint not installed; skipped"; \
	fi

lint-sdk: ## Run SDK service lint checks
	@if [ "$(JS_PM)" = "yarn" ] && ! command -v yarn >/dev/null 2>&1; then \
		echo "yarn not found on PATH. Run: corepack enable"; \
		exit 0; \
	elif ! command -v $(JS_PM) >/dev/null 2>&1; then \
		echo "$(JS_PM) not found; skipped SDK lint"; \
		exit 0; \
	fi
	@if [ "$(JS_PM)" = "yarn" ]; then \
		YARN_VERSION="$$(COREPACK_HOME=$(COREPACK_HOME_DIR) yarn --version)"; \
		YARN_MAJOR="$${YARN_VERSION%%.*}"; \
		if [ "$$YARN_MAJOR" -lt 4 ]; then \
			echo "yarn $$YARN_VERSION detected; Yarn 4+ required. Run: corepack enable && corepack prepare yarn@4.1.1 --activate"; \
			exit 1; \
		fi; \
		COREPACK_HOME=$(COREPACK_HOME_DIR) $(JS_PM) workspace proton-lfs-bridge lint; \
	else \
		$(JS_PM) --workspace $(BRIDGE_DIR) run lint; \
	fi

install-hooks: ## Install pre-commit hooks
	@if ! command -v pre-commit >/dev/null 2>&1; then \
		echo "pre-commit is not installed"; \
		exit 1; \
	fi
	pre-commit install

status: ## Print project status
	@echo "Go: $$($(GO) version)"
	@NODE_BIN_RESOLVED="$$(command -v $(NODE) 2>/dev/null || true)"; \
		if [ -n "$$NODE_BIN_RESOLVED" ]; then \
			echo "Node: $$($$NODE_BIN_RESOLVED --version)"; \
		else \
			echo "Node: not found"; \
		fi
	@if command -v $(JS_PM) >/dev/null 2>&1; then \
		if [ "$(JS_PM)" = "yarn" ]; then \
			echo "JS PM ($(JS_PM)): $$(COREPACK_HOME=$(COREPACK_HOME_DIR) $(JS_PM) --version 2>/dev/null || echo unavailable)"; \
		else \
			echo "JS PM ($(JS_PM)): $$($(JS_PM) --version)"; \
		fi; \
	else \
		echo "JS PM ($(JS_PM)): not found"; \
	fi
	@echo "Adapter binary: $$([ -f $(ADAPTER_BIN) ] && echo present || echo missing)"

clean: ## Remove generated files
	rm -rf bin
	rm -rf $(GO_CACHE_DIR)
	$(GO) clean -cache -testcache
