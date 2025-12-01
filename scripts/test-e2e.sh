#!/usr/bin/env bash

# test-e2e.sh - End-to-end testing for MPC development environment
#
# This script provides a streamlined way to test the complete MPC dev-env workflow:
#   - Spins up a fresh Kind cluster
#   - Deploys MPC stack and controller
#   - Lets you interactively select and run TaskRuns
#   - Lets you decide what to do with the environment afterwards
#
# This is the most basic integration testing tool - not unit tests, but full system tests.
# All choices are interactive - no automation.
#
# Usage:
#   make test-e2e  # Interactive e2e testing

set -euo pipefail

# Auto-detect environment paths if not set
if [ -z "${MPC_DEV_ENV_PATH:-}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    export MPC_DEV_ENV_PATH="$(dirname "$SCRIPT_DIR")"
    echo "Auto-detected MPC_DEV_ENV_PATH: $MPC_DEV_ENV_PATH"
fi

if [ -z "${MPC_REPO_PATH:-}" ]; then
    parent_dir="$(dirname "$MPC_DEV_ENV_PATH")"
    candidate_path="${parent_dir}/multi-platform-controller"

    if [ -d "$candidate_path" ]; then
        export MPC_REPO_PATH="$candidate_path"
        echo "Auto-detected MPC_REPO_PATH: $MPC_REPO_PATH"
    else
        echo "Error: MPC_REPO_PATH not set and auto-detection failed"
        echo "Looked for multi-platform-controller at: $candidate_path"
        exit 1
    fi
fi

# Color codes for output
readonly COLOR_SUCCESS='\033[0;32m'
readonly COLOR_ERROR='\033[0;31m'
readonly COLOR_INFO='\033[0;34m'
readonly COLOR_RESET='\033[0m'

# Logging functions
log_info() {
    echo -e "${COLOR_INFO}[INFO]${COLOR_RESET} $*"
}

log_success() {
    echo -e "${COLOR_SUCCESS}[PASS]${COLOR_RESET} $*"
}

log_error() {
    echo -e "${COLOR_ERROR}[FAIL]${COLOR_RESET} $*"
}

# Main test function
main() {
    log_info "========================================="
    log_info "  MPC Dev Environment E2E Test"
    log_info "========================================="
    log_info ""

    # Validate environment variables
    log_info "Validating environment variables..."
    if [ -z "${MPC_REPO_PATH:-}" ]; then
        log_error "MPC_REPO_PATH is not set"
        exit 1
    fi
    if [ -z "${MPC_DEV_ENV_PATH:-}" ]; then
        log_error "MPC_DEV_ENV_PATH is not set"
        exit 1
    fi

    log_success "Environment variables validated"
    log_info "  MPC_DEV_ENV_PATH: $MPC_DEV_ENV_PATH"
    log_info "  MPC_REPO_PATH: $MPC_REPO_PATH"
    log_info ""

    # Clean slate - tear down any existing environment
    log_info "Starting with clean slate..."
    cd "$MPC_DEV_ENV_PATH" || exit 1
    make teardown 2>/dev/null || true
    sleep 2
    log_success "Clean slate ready"
    log_info ""

    # Run dev-env fully interactively
    log_info "========================================="
    log_info "  Starting dev-env workflow"
    log_info "========================================="
    log_info ""
    log_info "You will be prompted to:"
    log_info "  - Select a TaskRun to test"
    log_info "  - Choose whether AWS credentials are needed"
    log_info "  - Decide what to do after TaskRun completes"
    log_info ""

    cd "$MPC_DEV_ENV_PATH" || exit 1

    # Run dev-env fully interactively - no piped input, no automation
    make dev-env

    local exit_code=$?

    if [ $exit_code -ne 0 ]; then
        log_error "make dev-env failed with exit code: $exit_code"
        exit $exit_code
    fi

    log_info ""
    log_info "========================================="
    log_info "  E2E Test completed!"
    log_info "========================================="
    log_success "Environment setup and TaskRun execution completed"
}

# Run the test
main "$@"
