# MIT License

# Copyright (c) 2026 René-Jean Corneille

# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:

# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.

# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

BUILD          ?= $(shell git rev-parse --short HEAD)
BUILD_CODENAME ?= dgraph
BUILD_DATE     ?= $(shell git log -1 --format=%cI)
BUILD_BRANCH   ?= $(shell git rev-parse --abbrev-ref HEAD)
BUILD_VERSION  ?= $(shell git describe --always --tags)

GOPATH         ?= $(shell go env GOPATH)

# Tool commands
GO_CMD         := go
GO_BUILD       := $(GO_CMD) build
GO_TEST        := $(GO_CMD) test
GO_CLEAN       := $(GO_CMD) clean
GO_FMT         := $(GO_CMD) fmt
GO_VET         := $(GO_CMD) vet

# Directories
BIN_DIR        := bin
CMD_DIR        := ./cmd/server
PY_DIR         := sdk/python

# The agent-framework integrations import their SDK at module scope, so their
# tests skip entirely unless these optional groups are installed.
PY_GROUPS      := --group openai --group anthropic

# Binary name
BINARY_NAME    := fraise

# Package manager commands
UV_CMD         := uv

# Color output
CYAN           := \033[0;36m
GREEN          := \033[0;32m
YELLOW         := \033[0;33m
RESET          := \033[0m

# Build flags. Injects the same pkg/version symbols GoReleaser sets on a
# release (see .goreleaser.yaml), so every build path reports its real version.
VERSION_PKG    := github.com/FraiseHQ/fraise/pkg/version
LDFLAGS        := -X '$(VERSION_PKG).Version=$(BUILD_VERSION)' \
                  -X '$(VERSION_PKG).Commit=$(BUILD)' \
                  -X '$(VERSION_PKG).Date=$(BUILD_DATE)'

.PHONY: help build test test-e2e clean install dev fmt lint check all publish publish-py
.DEFAULT_GOAL := help

##@ General

help: ## Display this help message
	@echo "$(CYAN)Fraise Build System$(RESET)"
	@echo "$(GREEN)Version: $(BUILD_VERSION) | Branch: $(BUILD_BRANCH)$(RESET)"
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(CYAN)%-25s$(RESET) %s\n", $$1, $$2 } /^##@/ { printf "\n$(YELLOW)%s$(RESET)\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

build: build-go ## Build all components (currently Go only)

build-all: build-go build-py ## Build Go server and all SDKs

build-go: ## Build Go server binary
	@echo "$(CYAN)Building Go server ($(BUILD_VERSION))...$(RESET)"
	@mkdir -p $(BIN_DIR)
	$(GO_BUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "$(GREEN)✓ Binary created: $(BIN_DIR)/$(BINARY_NAME)$(RESET)"

build-py: ## Build Python SDK
	@echo "$(CYAN)Building Python SDK...$(RESET)"
	@cd $(PY_DIR) && $(UV_CMD) build
	@echo "$(GREEN)✓ Python SDK built$(RESET)"

##@ Testing

test: test-go ## Run all Go tests

coverage: coverage-go coverage-py

test-all: test-go test-py ## Run tests for Go and all SDKs

test-go: ## Run Go tests with verbose output
	@echo "$(CYAN)Running Go tests...$(RESET)"
	$(GO_TEST) -v ./...

coverage-go: ## Run Go tests with coverage report
	@echo "$(CYAN)Running Go tests with coverage...$(RESET)"
	$(GO_TEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	@echo "$(GREEN)✓ Coverage report: coverage.txt$(RESET)"

coverage-py: ## Run Python SDK unit tests with coverage report
	@echo "$(CYAN)Running Python SDK tests with coverage...$(RESET)"
	@$(UV_CMD) run --package fraise-sdk $(PY_GROUPS) pytest $(PY_DIR)/src/tests \
		--cov=fraise_sdk --cov-report=xml:coverage-py.xml --cov-report=term
	@echo "$(GREEN)✓ Coverage report: coverage-py.xml$(RESET)"

test-go-bench: ## Run Go benchmarks
	@echo "$(CYAN)Running Go benchmarks...$(RESET)"
	$(GO_TEST) -bench=. -benchmem ./...

# Host port fraise binds to for the test suites (override if 9876 is taken).
# Both suites run pytest locally and reach the container through this port.
FRAISE_E2E_PORT ?= 9876

COMPOSE        := docker compose -f docker-compose.yaml

# Dump the server's logs when a suite fails. pytest says which assertion broke;
# only the container says why — a panic, a rejected write, a config it did not
# like on boot. Printed before the trap tears the container down, since after
# that the logs are gone.
FRAISE_LOGS    = echo "$(YELLOW)--- fraise server logs ---$(RESET)"; \
                 $(COMPOSE) logs --no-color --tail=200 fraise

test-e2e: ## Run end-to-end tests against fraise in Docker
	@echo "$(CYAN)Starting fraise (docker) for end-to-end tests...$(RESET)"
	@FRAISE_PORT=$(FRAISE_E2E_PORT) $(COMPOSE) up --build --detach fraise
	@trap '$(COMPOSE) down --remove-orphans' EXIT INT TERM; \
	  $(UV_CMD) run --package tests pytest tests/e2e -v \
	    || ( $(FRAISE_LOGS); exit 1 )

test-integration-py: ## Run Python SDK integration tests against fraise in Docker
	@echo "$(CYAN)Starting fraise (docker) for Python SDK integration tests...$(RESET)"
	@FRAISE_PORT=$(FRAISE_E2E_PORT) $(COMPOSE) up --build --detach fraise
	@trap '$(COMPOSE) down --remove-orphans' EXIT INT TERM; \
	  $(UV_CMD) run --package fraise-sdk pytest tests/integration/python -v \
	    || ( $(FRAISE_LOGS); exit 1 )

test-py: ## Run Python tests with pytest
	@echo "$(CYAN)Running Python tests...$(RESET)"
	@$(UV_CMD) run --package fraise-sdk $(PY_GROUPS) pytest $(PY_DIR)/src/tests || echo "$(YELLOW)⚠ No Python tests configured$(RESET)"

test-watch: ## Run Go tests in watch mode (requires reflex)
	@echo "$(CYAN)Running Go tests in watch mode...$(RESET)"
	@which reflex > /dev/null || (echo "$(YELLOW)Installing reflex...$(RESET)" && $(GO_CMD) install github.com/cespare/reflex@latest)
	reflex -r '\.go$$' -s -- $(GO_TEST) -v ./...

##@ Development

dev: ## Run development server
	@echo "$(CYAN)Starting development server...$(RESET)"
	$(GO_CMD) run $(CMD_DIR)/main.go

install: ## Install all dependencies
	@echo "$(CYAN)Installing Go dependencies...$(RESET)"
	$(GO_CMD) mod download
	@echo "$(CYAN)Installing Python dependencies...$(RESET)"
	@cd $(PY_DIR) && $(UV_CMD) sync
	@echo "$(GREEN)✓ All dependencies installed$(RESET)"

install-tools: ## Install development tools
	@echo "$(CYAN)Installing development tools...$(RESET)"
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && $(GO_CMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@which reflex > /dev/null || (echo "Installing reflex..." && $(GO_CMD) install github.com/cespare/reflex@latest)
	@echo "$(GREEN)✓ Development tools installed$(RESET)"

##@ Code Quality

fmt: fmt-go fmt-py ## Format all code

fmt-go: ## Format Go code
	@echo "$(CYAN)Formatting Go code...$(RESET)"
	$(GO_FMT) ./...
	@echo "$(GREEN)✓ Go code formatted$(RESET)"

fmt-py: ## Format Python code with ruff
	@echo "$(CYAN)Formatting Python code...$(RESET)"
	@cd $(PY_DIR) && $(UV_CMD) run ruff format 2>/dev/null || echo "$(YELLOW)⚠ Ruff not configured$(RESET)"

# All linting flows through pre-commit (config: .pre-commit-config.yaml) so local
# and CI run identical hooks. Each target runs one slice; CI calls these directly.
PRECOMMIT     := uvx pre-commit run --all-files --show-diff-on-failure

lint: ## Lint all code via pre-commit (every hook)
	@echo "$(CYAN)Running pre-commit hooks on all files...$(RESET)"
	@uvx pre-commit run --all-files

lint-go: ## Lint Go via golangci-lint (pre-commit)
	@echo "$(CYAN)Linting Go code...$(RESET)"
	@$(PRECOMMIT) golangci-lint-full

lint-py: ## Lint Python via ruff + ty (pre-commit)
	@echo "$(CYAN)Linting Python code...$(RESET)"
	@$(PRECOMMIT) ruff
	@$(PRECOMMIT) ruff-format
	@$(PRECOMMIT) ty

lint-docker: ## Lint Dockerfiles via hadolint (pre-commit)
	@echo "$(CYAN)Linting Dockerfiles...$(RESET)"
	@$(PRECOMMIT) hadolint-docker

check: fmt lint test ## Format, lint, and test Go code
	@echo "$(GREEN)✓ All checks passed$(RESET)"

check-all: fmt lint test-all ## Format, lint, and test everything
	@echo "$(GREEN)✓ All checks passed$(RESET)"

##@ Cleanup

clean: clean-go clean-py ## Clean all build artifacts

clean-go: ## Clean Go build artifacts
	@echo "$(CYAN)Cleaning Go artifacts...$(RESET)"
	$(GO_CLEAN)
	rm -rf $(BIN_DIR)/
	rm -f coverage.out coverage.html
	@echo "$(GREEN)✓ Go artifacts cleaned$(RESET)"

clean-py: ## Clean Python build artifacts
	@echo "$(CYAN)Cleaning Python artifacts...$(RESET)"
	@cd $(PY_DIR) && rm -rf dist/ .pytest_cache/ __pycache__/
	@find . -type d -name "__pycache__" -exec rm -rf {} + 2>/dev/null || true
	@find . -type f -name "*.pyc" -delete 2>/dev/null || true
	@echo "$(GREEN)✓ Python artifacts cleaned$(RESET)"

clean-all: clean ## Alias for clean

##@ Publishing

publish: publish-py ## Publish Python SDK to PyPI

publish-py: build-py ## Publish Python SDK to PyPI (set UV_PUBLISH_TOKEN or PyPI credentials)
	@echo "$(CYAN)Publishing Python SDK to PyPI...$(RESET)"
	@cd $(PY_DIR) && $(UV_CMD) publish
	@echo "$(GREEN)✓ Python SDK published to PyPI$(RESET)"

##@ Workflows

all: clean install build-all test-all ## Full rebuild: clean, install, build, and test everything
	@echo "$(GREEN)✓ Full build completed$(RESET)"

quick: build-go test-go-short ## Quick development cycle: build and test Go (short mode)
	@echo "$(GREEN)✓ Quick build completed$(RESET)"

ci: install lint test ## CI pipeline: install, lint, and test
	@echo "$(GREEN)✓ CI pipeline completed$(RESET)"

release: clean build-go test-go lint-go ## Prepare release build
	@echo "$(GREEN)✓ Release build ready: $(BIN_DIR)/$(BINARY_NAME)$(RESET)"
	@echo "$(GREEN)Version: $(BUILD_VERSION)$(RESET)"
	@echo "$(GREEN)Build: $(BUILD)$(RESET)"
	@echo "$(GREEN)Date: $(BUILD_DATE)$(RESET)"
