# Makefile for MPC Dev Environment Tool
#
# This Makefile provides targets for building, testing, and running the MPC development
# environment tool. The tool consists of a Go daemon (HTTP API server) and bash scripts
# that orchestrate the complete workflow.
#
# Quick Start:
#   make dev-env    # Set up complete environment and test TaskRuns
#   make test-e2e   # Quick E2E testing (clean slate + interactive setup)
#   make teardown   # Clean up everything
#
# Architecture:
#   - Go daemon (bin/mpc-daemon): HTTP API server for K8s/Tekton operations
#   - Bash scripts (scripts/*.sh): User interaction and workflow orchestration
#   - API communication: Bash → HTTP → Go → client-go (K8s/Tekton)
#
# Requirements:
#   - Go 1.24+
#   - kubectl, kind, podman (or docker), helm, git, jq
#   - Multi-platform-controller repository as sibling directory
#
# Environment Variables (auto-detected if not set):
#   MPC_DEV_ENV_PATH - Path to this repository
#   MPC_REPO_PATH    - Path to multi-platform-controller repository

.PHONY: help build test clean verify install run plugin

# help - Display all available make targets with descriptions
help:
	@echo "MPC Dev Studio - Available Commands"
	@echo "===================================="
	@echo ""
	@echo "Build & Run:"
	@echo "  make build          - Build the Go daemon binary"
	@echo "  make run            - Run the Go daemon (auto-detects paths from directory structure)"
	@echo "  make install        - Install the daemon to /usr/local/bin"
	@echo ""
	@echo "Testing & Verification:"
	@echo "  make test           - Run all Go tests"
	@echo "  make test-api       - Run API tests only"
	@echo "  make test-e2e       - Run end-to-end test (interactive)"
	@echo "  make lint           - Run golangci-lint (if installed)"
	@echo ""
#	@echo "Plugin Development:"
#	@echo "  make plugin         - Build the GoLand plugin"
#	@echo "  make plugin-run     - Run the plugin in sandbox IDE"
#	@echo "  make plugin -clean   - Clean plugin build artifacts"
#	@echo ""
	@echo "Cleanup & Maintenance:"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make clean-all      - Remove all generated files"
	@echo ""
	@echo "Development:"
	@echo "  make fmt            - Format Go code"
	@echo "  make vet            - Run go vet"
	@echo "  make deps           - Download Go dependencies"
	@echo "  make setup          - Setup development environment"
	@echo "  make env            - Show environment variables"
	@echo ""
	@echo "Development Environment:"
	@echo "  make dev-env        - Start complete MPC development environment"
	@echo "  make teardown       - Tear down development environment (cluster + daemon)"
	@echo ""

# Build the daemon
build:
	@echo "Building Go daemon..."
	@mkdir -p bin
	@go build -o bin/mpc-daemon cmd/mpc-daemon/main.go
	@echo "✓ Build complete: bin/mpc-daemon"
	@ls -lh bin/mpc-daemon

# Run the daemon (auto-detects paths if not set)
run: build
	@# Auto-detect paths if not set
	@if [ -z "$$MPC_DEV_ENV_PATH" ]; then \
		export MPC_DEV_ENV_PATH="$(PWD)"; \
		echo "Auto-detected MPC_DEV_ENV_PATH: $$MPC_DEV_ENV_PATH"; \
	fi; \
	if [ -z "$$MPC_REPO_PATH" ]; then \
		PARENT_DIR="$$(dirname $(PWD))"; \
		if [ -d "$$PARENT_DIR/multi-platform-controller" ]; then \
			export MPC_REPO_PATH="$$PARENT_DIR/multi-platform-controller"; \
			echo "Auto-detected MPC_REPO_PATH: $$MPC_REPO_PATH"; \
		else \
			echo "Error: Cannot auto-detect MPC_REPO_PATH. multi-platform-controller not found as sibling directory."; \
			echo "Please set manually: export MPC_REPO_PATH=/path/to/multi-platform-controller"; \
			exit 1; \
		fi; \
	fi; \
	echo "Starting daemon..."; \
	MPC_DEV_ENV_PATH="$$MPC_DEV_ENV_PATH" MPC_REPO_PATH="$$MPC_REPO_PATH" ./bin/mpc-daemon

# Install to system
install: build
	@echo "Installing to /usr/local/bin/mpc-daemon..."
	@sudo cp bin/mpc-daemon /usr/local/bin/mpc-daemon
	@echo "✓ Installed successfully"
	@echo "You can now run: mpc-daemon"

# Run all tests
test:
	@echo "Running all tests..."
	@go test -v ./...

# Run API tests only
test-api:
	@echo "Running API tests..."
	@go test -v ./internal/daemon/api/...

# Run end-to-end test (fully interactive)
test-e2e:
	@echo "Running end-to-end test..."
	@# Auto-detect paths if not set
	@if [ -z "$$MPC_DEV_ENV_PATH" ]; then \
		export MPC_DEV_ENV_PATH="$(PWD)"; \
		echo "Auto-detected MPC_DEV_ENV_PATH: $$MPC_DEV_ENV_PATH"; \
	fi; \
	if [ -z "$$MPC_REPO_PATH" ]; then \
		PARENT_DIR="$$(dirname $(PWD))"; \
		if [ -d "$$PARENT_DIR/multi-platform-controller" ]; then \
			export MPC_REPO_PATH="$$PARENT_DIR/multi-platform-controller"; \
			echo "Auto-detected MPC_REPO_PATH: $$MPC_REPO_PATH"; \
		else \
			echo "Error: Cannot auto-detect MPC_REPO_PATH. multi-platform-controller not found as sibling directory."; \
			echo "Please set manually: export MPC_REPO_PATH=/path/to/multi-platform-controller"; \
			exit 1; \
		fi; \
	fi; \
	MPC_DEV_ENV_PATH="$$MPC_DEV_ENV_PATH" MPC_REPO_PATH="$$MPC_REPO_PATH" bash scripts/test-e2e.sh

# Format code
fmt:
	@echo "Formatting Go code..."
	@go fmt ./...
	@echo "✓ Code formatted"

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "✓ Vet complete"

# Run linter (if installed)
lint:
	@GOLANGCI_LINT=$$(command -v golangci-lint 2>/dev/null || echo "$$(go env GOPATH)/bin/golangci-lint"); \
	if [ -x "$$GOLANGCI_LINT" ]; then \
		echo "Running golangci-lint..."; \
		$$GOLANGCI_LINT run; \
	else \
		echo "golangci-lint not installed"; \
		echo "Install with: make setup"; \
		echo "Or manually: https://golangci-lint.run/usage/install/"; \
	fi

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies updated"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f *.log
	@echo "✓ Clean complete"
	@echo ""
	@echo "Cleaning Podman resources..."
	@if command -v podman >/dev/null 2>&1; then \
		echo "Pruning Podman containers..."; \
		podman container prune -f 2>/dev/null || true; \
		echo "Pruning Podman volumes..."; \
		podman volume prune -f 2>/dev/null || true; \
		echo "✓ Podman cleanup complete"; \
	else \
		echo "Podman not found, skipping Podman cleanup"; \
	fi

# Clean everything including caches
clean-all: clean plugin-clean
	@echo "Cleaning Go caches..."
	@go clean -cache -testcache -modcache
	@echo "✓ Go caches cleaned"
	@echo ""
	@echo "Cleaning orphaned Podman images..."
	@if command -v podman >/dev/null 2>&1; then \
		echo "Pruning dangling images..."; \
		podman image prune -f 2>/dev/null || true; \
		echo "✓ Podman images cleaned"; \
	else \
		echo "Podman not found, skipping image cleanup"; \
	fi
	@echo ""
	@echo "✓ All clean complete"

# Build the GoLand plugin
plugin:
	@echo "Building GoLand plugin..."
	@cd goland-plugin && ./gradlew buildPlugin -x jarSearchableOptions
	@echo "✓ Plugin built"
	@ls -lh goland-plugin/build/distributions/

# Run the plugin in sandbox IDE
plugin-run:
	@echo "Starting plugin in sandbox IDE..."
	@cd goland-plugin && ./gradlew runIde -x jarSearchableOptions

# Clean plugin artifacts
plugin-clean:
	@echo "Cleaning plugin build artifacts..."
	@cd goland-plugin && ./gradlew clean
	@echo "✓ Plugin clean"

# Pre-commit checks (useful for git hooks)
pre-commit: fmt vet test verify
	@echo "✓ Pre-commit checks passed"

# Quick check before committing
check: fmt vet build test-api
	@echo "✓ Quick checks passed"

# Development setup
setup:
	@echo "Setting up development environment..."
	@echo ""
	@echo "Checking prerequisites..."
	@command -v go >/dev/null 2>&1 || (echo "Error: Go not installed" && exit 1)
	@command -v kubectl >/dev/null 2>&1 || (echo "Error: kubectl not installed" && exit 1)
	@command -v kind >/dev/null 2>&1 || (echo "Warning: kind not installed")
	@command -v docker >/dev/null 2>&1 || command -v podman >/dev/null 2>&1 || (echo "Warning: Neither docker nor podman found")
	@echo ""
	@echo "Installing golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "✓ golangci-lint already installed ($$(golangci-lint --version))"; \
	else \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin; \
		if command -v golangci-lint >/dev/null 2>&1; then \
			echo "✓ golangci-lint installed successfully"; \
		else \
			echo "⚠ Warning: golangci-lint installation may have failed. Check that $$(go env GOPATH)/bin is in your PATH"; \
		fi; \
	fi
	@echo ""
	@echo "Downloading dependencies..."
	@go mod download
	@echo ""
	@echo "Building daemon..."
	@make build
	@echo ""
	@echo "✓ Setup complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Run the daemon (paths auto-detected if multi-platform-controller is a sibling):"
	@echo "     make run"
	@echo ""
	@echo "  2. Or manually set environment variables if your structure differs:"
	@echo "     export MPC_REPO_PATH=/path/to/multi-platform-controller"
	@echo "     export MPC_DEV_ENV_PATH=/path/to/mpc_dev_env"
	@echo ""

# Show environment
env:
	@echo "Current Environment:"
	@echo "==================="
	@echo "MPC_REPO_PATH:     $${MPC_REPO_PATH:-<not set>}"
	@echo "MPC_DEV_ENV_PATH:  $${MPC_DEV_ENV_PATH:-<not set>}"
	@echo "Go version:        $$(go version 2>/dev/null || echo 'not found')"
	@echo "kubectl version:   $$(kubectl version --client -o json 2>/dev/null | grep -o '"gitVersion":"[^"]*"' | cut -d'"' -f4 || echo 'not found')"
	@echo "kind version:      $$(kind --version 2>/dev/null || echo 'not found')"
	@echo "Docker/Podman:     $$(command -v docker >/dev/null 2>&1 && docker --version || command -v podman >/dev/null 2>&1 && podman --version || echo 'not found')"

# Development environment management
dev-env:
	@echo "Starting MPC development environment setup..."
	@# Auto-detect paths if not set
	@if [ -z "$$MPC_DEV_ENV_PATH" ]; then \
		export MPC_DEV_ENV_PATH="$(PWD)"; \
		echo "Auto-detected MPC_DEV_ENV_PATH: $$MPC_DEV_ENV_PATH"; \
	fi; \
	if [ -z "$$MPC_REPO_PATH" ]; then \
		PARENT_DIR="$$(dirname $(PWD))"; \
		if [ -d "$$PARENT_DIR/multi-platform-controller" ]; then \
			export MPC_REPO_PATH="$$PARENT_DIR/multi-platform-controller"; \
			echo "Auto-detected MPC_REPO_PATH: $$MPC_REPO_PATH"; \
		else \
			echo "Error: Cannot auto-detect MPC_REPO_PATH. multi-platform-controller not found as sibling directory."; \
			echo "Please set manually: export MPC_REPO_PATH=/path/to/multi-platform-controller"; \
			exit 1; \
		fi; \
	fi; \
	MPC_DEV_ENV_PATH="$$MPC_DEV_ENV_PATH" MPC_REPO_PATH="$$MPC_REPO_PATH" bash scripts/dev-env.sh

teardown:
	@echo "Tearing down development environment..."
	@if [ -n "$$(KIND_EXPERIMENTAL_PROVIDER=podman kind get clusters 2>/dev/null | grep konflux)" ]; then \
		KIND_EXPERIMENTAL_PROVIDER=podman kind delete cluster --name konflux; \
		echo "✓ Cluster deleted"; \
	else \
		echo "No cluster to delete"; \
	fi
	@if lsof -ti :8765 >/dev/null 2>&1; then \
		lsof -ti :8765 | xargs kill -9 2>/dev/null; \
		echo "✓ Daemon stopped"; \
	else \
		echo "Daemon not running"; \
	fi
