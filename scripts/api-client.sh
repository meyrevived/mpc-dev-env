#!/usr/bin/env bash

# api-client.sh - API client functions for communicating with the MPC daemon
#
# This script provides wrapper functions for interacting with the Go daemon's HTTP API:
#   - Daemon health checking and readiness waiting
#   - Generic API call wrapper with JSON support
#   - Operation status polling (tracks async operations until completion)
#
# All API calls target the daemon running on localhost:8765.
# These functions are used throughout the dev-env workflow to delegate operations
# to the Go daemon rather than using kubectl/bash commands directly.
#
# Usage:
#   source scripts/api-client.sh
#   if daemon_is_running; then
#       api_call POST "/api/mpc/rebuild-and-redeploy"
#       poll_operation_status
#   fi

readonly DAEMON_URL="http://localhost:8765"

# daemon_is_running - Check if the daemon is responding to API requests
#
# Returns:
#   0 if daemon is healthy (GET /api/status returns 200)
#   1 if daemon is unreachable or unhealthy
daemon_is_running() {
    curl -s -f "${DAEMON_URL}/api/status" >/dev/null 2>&1
}

# daemon_get_status - Get the current development environment state from daemon
#
# Returns:
#   JSON object containing the complete DevEnvironment state
#   (cluster status, operation_status, last_operation_error, taskrun_info, etc.)
daemon_get_status() {
    curl -s "${DAEMON_URL}/api/status"
}

# daemon_wait_ready - Poll daemon until it becomes ready or timeout
#
# Arguments:
#   $1 - Timeout in seconds (default: 30)
#
# Returns:
#   0 if daemon becomes ready within timeout
#   1 if timeout expires
daemon_wait_ready() {
    local timeout_seconds="${1:-30}"
    local elapsed=0

    log_info "Waiting for daemon to be ready..."
    while [ $elapsed -lt $timeout_seconds ]; do
        if daemon_is_running; then
            log_success "Daemon is ready"
            return 0
        fi
        sleep 1
        elapsed=$((elapsed + 1))
    done

    log_error "Daemon did not become ready within ${timeout_seconds} seconds"
    return 1
}

# api_call - Generic API call wrapper with JSON support
#
# Arguments:
#   $1 - HTTP method (GET, POST, etc.)
#   $2 - API endpoint path (e.g., "/api/mpc/build")
#   $3 - Optional JSON data for request body
#
# Returns:
#   API response body (typically JSON)
#
# Example:
#   api_call POST "/api/taskrun/run" '{"yaml_path": "/path/to/taskrun.yaml"}'
api_call() {
    local method="$1"
    local endpoint="$2"
    local data="${3:-}"

    if [ -n "$data" ]; then
        curl -s -X "$method" "${DAEMON_URL}${endpoint}" \
            -H "Content-Type: application/json" \
            -d "$data"
    else
        curl -s -X "$method" "${DAEMON_URL}${endpoint}"
    fi
}

# poll_operation_status - Poll daemon state until operation_status becomes "idle"
#
# This function tracks async operations (builds, deployments, TaskRuns) by polling
# the daemon's state every 2 seconds. It waits for operation_status to return to "idle"
# and checks for errors in last_operation_error.
#
# Arguments:
#   $1 - Maximum attempts (default: 300, which is 10 minutes at 2s intervals)
#
# Returns:
#   0 if operation completes successfully (idle with no error)
#   1 if operation fails (idle with error) or timeout
#
# Status changes are logged to reduce spam (only on status change or every 120s)
poll_operation_status() {
    local max_attempts="${1:-300}"  # 300 * 2s = 10 minutes max
    local attempt=0
    local last_logged_status=""

    while [ $attempt -lt $max_attempts ]; do
        local status_json
        status_json=$(daemon_get_status)

        local operation_status
        operation_status=$(echo "$status_json" | jq -r '.operation_status // "idle"')

        local last_error
        last_error=$(echo "$status_json" | jq -r '.last_operation_error // ""')

        if [ "$operation_status" = "idle" ] || [ "$operation_status" = "null" ]; then
            if [ -n "$last_error" ] && [ "$last_error" != "null" ]; then
                log_error "Operation failed: $last_error"
                return 1
            fi
            return 0
        fi

        # Only log status if it changed or every 120 seconds (60 attempts)
        if [ "$operation_status" != "$last_logged_status" ] || [ $((attempt % 60)) -eq 0 ]; then
            log_info "Operation status: $operation_status"
            last_logged_status="$operation_status"
        fi

        sleep 2
        attempt=$((attempt + 1))
    done

    log_error "Operation timed out"
    return 1
}
