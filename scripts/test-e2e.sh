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

# Always determine script location first - this is reliable regardless of env vars
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source shared utilities from script's actual location (not from env var which may be stale)
# shellcheck source=scripts/utils.sh
source "${SCRIPT_DIR}/utils.sh"

# Set MPC_DEV_ENV_PATH based on script location if not already set
if [ -z "${MPC_DEV_ENV_PATH:-}" ]; then
    export MPC_DEV_ENV_PATH="$(dirname "$SCRIPT_DIR")"
fi

# Set up session logging - capture all output to a timestamped log file
# This helps with debugging issues on different machines
LOGS_DIR="${MPC_DEV_ENV_PATH}/logs"
mkdir -p "$LOGS_DIR"
SESSION_LOG="${LOGS_DIR}/test-e2e_$(date '+%Y%m%d_%H%M%S').log"
exec > >(tee -a "$SESSION_LOG") 2>&1
echo "Session log: $SESSION_LOG"

# Color codes for output (used by local logging functions with different prefixes)
readonly COLOR_SUCCESS_LOCAL='\033[0;32m'
readonly COLOR_ERROR_LOCAL='\033[0;31m'
readonly COLOR_INFO_LOCAL='\033[0;34m'
readonly COLOR_RESET_LOCAL='\033[0m'

# Local logging functions with [PASS]/[FAIL] prefixes for test output
log_pass() {
    echo -e "${COLOR_SUCCESS_LOCAL}[PASS]${COLOR_RESET_LOCAL} $*"
}

log_fail() {
    echo -e "${COLOR_ERROR_LOCAL}[FAIL]${COLOR_RESET_LOCAL} $*"
}

# Main test function
main() {
    log_info "========================================="
    log_info "  MPC Dev Environment E2E Test"
    log_info "========================================="
    log_info ""

    # Validate environment paths (prompts user if paths are invalid)
    log_info "Validating environment variables..."
    if ! validate_and_set_env_paths; then
        log_fail "Failed to validate environment paths"
        exit 1
    fi

    log_pass "Environment variables validated"
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
