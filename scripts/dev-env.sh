#!/usr/bin/env bash

# dev-env.sh - Main orchestration script for the MPC development environment
#
# This is the primary entry point for the complete dev-env workflow. It orchestrates
# all phases of the MPC development environment setup and TaskRun testing:
#
# Phase 1: Prerequisites validation (checks tools, paths, builds daemon)
# Phase 2: Daemon startup (starts API server on port 8765)
# Phase 3: Credential collection (prompts for AWS credentials if needed)
# Phase 4: Cluster setup (creates Kind cluster via daemon API)
# Phase 5: MPC stack deployment (deploys Tekton + MPC Operator + OTP)
# Phase 6: MPC build & deployment (builds custom image, patches deployment)
# Phase 7: Secrets deployment (AWS secrets for cloud builds, if needed)
# Phase 8: TaskRun workflow (user selects and applies TaskRun)
# Phase 9: Build verification (monitors TaskRun, streams logs)
# Phase 10: Summary output (shows status, useful commands)
#
# Design Philosophy:
#   - User interaction layer: This bash script handles ALL user prompts and menus
#   - Kubernetes operations: Delegated to Go daemon via REST API
#   - Error handling: Multi-level cleanup options at every failure point
#   - Flexibility: Users can stop at any phase and keep environment running
#
# Architecture:
#   Bash (dev-env.sh) → HTTP API → Go daemon → K8s/Tekton client-go
#
# Usage:
#   make dev-env  # Standard workflow with all prompts
#   make test-e2e # E2E testing with clean slate (fully interactive)

set -euo pipefail

# Always determine script location first - this is reliable regardless of env vars
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source helper scripts from script's actual location (not from env var which may be stale)
# shellcheck source=scripts/utils.sh
source "${SCRIPT_DIR}/utils.sh"

# shellcheck source=scripts/api-client.sh
source "${SCRIPT_DIR}/api-client.sh"

# shellcheck source=scripts/cleanup.sh
source "${SCRIPT_DIR}/cleanup.sh"

# Define base directory structure (set from script location if not already set)
: "${MPC_DEV_ENV_PATH:=$(dirname "$SCRIPT_DIR")}"
export MPC_DEV_ENV_PATH

readonly SCRIPTS_DIR="${SCRIPT_DIR}"
readonly TASKRUNS_DIR="${MPC_DEV_ENV_PATH}/taskruns"
readonly LOGS_DIR="${MPC_DEV_ENV_PATH}/logs"

# Set up session logging - capture all output to a timestamped log file
# This helps with debugging issues on different machines
mkdir -p "$LOGS_DIR"
SESSION_LOG="${LOGS_DIR}/dev-env_$(date '+%Y%m%d_%H%M%S').log"
exec > >(tee -a "$SESSION_LOG") 2>&1
echo "Session log: $SESSION_LOG"

# Additional color codes for dev-env specific logging (COLOR_RESET comes from utils.sh)
readonly COLOR_SUCCESS='\033[0;32m'
readonly COLOR_ERROR='\033[0;31m'
readonly COLOR_WARNING='\033[0;33m'
readonly COLOR_INFO='\033[0;34m'

# Logging function with timestamps and colors
log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')

    case "$level" in
        SUCCESS)
            echo -e "${COLOR_SUCCESS}[${timestamp}] ✓ ${message}${COLOR_RESET}"
            ;;
        ERROR)
            echo -e "${COLOR_ERROR}[${timestamp}] ✗ ${message}${COLOR_RESET}" >&2
            ;;
        WARNING)
            echo -e "${COLOR_WARNING}[${timestamp}] ⚠ ${message}${COLOR_RESET}"
            ;;
        INFO)
            echo -e "${COLOR_INFO}[${timestamp}] ℹ ${message}${COLOR_RESET}"
            ;;
        *)
            echo "[${timestamp}] ${message}"
            ;;
    esac
}

# ============================================================================
# Helper functions
# ============================================================================

# check_taskrun_uses_local_platforms - Scan TaskRun file for local platform indicators
#
# Scans the TaskRun YAML file for keywords that indicate local platforms:
#   - "localhost"
#   - "local"
#   - "x86_64"
#
# Arguments:
#   $1 - Path to TaskRun YAML file
#
# Returns:
#   0 (true) if local platform indicators found
#   1 (false) if no local platform indicators found (AWS platforms likely needed)
check_taskrun_uses_local_platforms() {
    local taskrun_file="$1"

    if [ ! -f "$taskrun_file" ]; then
        log ERROR "TaskRun file not found: $taskrun_file"
        return 1
    fi

    # Search for local platform indicators (case-insensitive)
    if grep -qi "localhost\|local\|x86_64" "$taskrun_file"; then
        return 0  # Local platforms found
    else
        return 1  # No local platforms found
    fi
}

# ============================================================================
# Phase placeholder functions (to be implemented by later prompts)
# ============================================================================

phase1_prerequisites() {
    log INFO "Phase 1: Checking prerequisites..."

    # Validate and set environment paths (prompts user if paths are invalid)
    if ! validate_and_set_env_paths; then
        log ERROR "Failed to validate environment paths"
        exit 1
    fi

    log SUCCESS "MPC_DEV_ENV_PATH verified: $MPC_DEV_ENV_PATH"
    log SUCCESS "MPC_REPO_PATH verified: $MPC_REPO_PATH"

    # Check if daemon binary exists, if not build it
    local daemon_binary="${MPC_DEV_ENV_PATH}/bin/mpc-daemon"
    if ! file_exists "$daemon_binary"; then
        log WARNING "Daemon binary not found at $daemon_binary"
        log INFO "Building daemon binary..."

        if ! command_exists "make"; then
            log ERROR "make command not found. Please install make."
            exit 1
        fi

        cd "$MPC_DEV_ENV_PATH" || exit 1
        if ! make build; then
            log ERROR "Failed to build daemon binary"
            exit 1
        fi

        if ! file_exists "$daemon_binary"; then
            log ERROR "Daemon binary still not found after build"
            exit 1
        fi
        log SUCCESS "Daemon binary built successfully"
    else
        log SUCCESS "Daemon binary found: $daemon_binary"
    fi

    log SUCCESS "Phase 1 complete: Prerequisites validated"
}

phase2_daemon_startup() {
    log INFO "Phase 2: Starting daemon..."

    local daemon_binary="${MPC_DEV_ENV_PATH}/bin/mpc-daemon"
    local daemon_pid_file="${MPC_DEV_ENV_PATH}/daemon.pid"
    local daemon_log_file="${MPC_DEV_ENV_PATH}/logs/daemon.log"

    # Ensure logs directory exists
    mkdir -p "${MPC_DEV_ENV_PATH}/logs"

    # Check if daemon is already running on port 8765
    if lsof -ti :8765 >/dev/null 2>&1; then
        log WARNING "Daemon already running on port 8765, killing it..."
        lsof -ti :8765 | xargs kill -9 2>/dev/null || true
        sleep 2
    fi

    # Remove old PID file if exists
    rm -f "$daemon_pid_file"

    # Start daemon in background
    log INFO "Starting daemon in background..."
    nohup "$daemon_binary" > "$daemon_log_file" 2>&1 &
    local daemon_pid=$!

    # Save PID to file
    echo "$daemon_pid" > "$daemon_pid_file"
    log INFO "Daemon started with PID: $daemon_pid"
    log INFO "Daemon logs: $daemon_log_file"

    # Wait for daemon to be ready
    if ! daemon_wait_ready 120; then
        log ERROR "Daemon failed to start within 120 seconds"
        log ERROR "Check daemon logs at: $daemon_log_file"
        exit 1
    fi

    # Once daemon is ready, call GET /api/prerequisites to validate all tools
    log INFO "Validating prerequisites via daemon..."
    local prereq_response
    prereq_response=$(api_call GET "/api/prerequisites")

    # Parse the JSON response
    local all_met
    all_met=$(echo "$prereq_response" | jq -r '.all_met // false')

    if [ "$all_met" != "true" ]; then
        log ERROR "Prerequisites check failed"
        log ERROR "Missing or outdated tools:"

        # Extract and display errors
        local errors
        errors=$(echo "$prereq_response" | jq -r '.errors[]? // empty')
        if [ -n "$errors" ]; then
            echo "$errors" | while IFS= read -r error; do
                log ERROR "  - $error"
            done
        fi

        # Display individual prerequisite status
        echo "$prereq_response" | jq -r '.prerequisites | to_entries[] | select(.value.status != "ok") | "  - \(.key): \(.value.status) (version: \(.value.version), required: \(.value.required))"' | while IFS= read -r line; do
            log ERROR "$line"
        done

        exit 1
    fi

    log SUCCESS "All prerequisites validated successfully"
    log SUCCESS "Phase 2 complete: Daemon is running"
}

phase3_cluster_setup() {
    log INFO "========================================="
    log INFO "Phase 3: Cluster Setup"
    log INFO "========================================="

    local retry=true
    while $retry; do
        retry=false

        # Call cluster start API
        log INFO "Creating Kind cluster 'konflux'..."
        local response
        response=$(api_call POST "/api/cluster/start")

        # Check if accepted
        if echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
            log SUCCESS "Cluster creation initiated"
        else
            local error
            error=$(echo "$response" | jq -r '.error // "Unknown error"')

            # Offer cleanup with retry option
            cleanup_level_1 "$error"
            # If we reach here, user chose retry (option 3)
            retry=true
            continue
        fi

        # Poll cluster status until running (max 5 minutes)
        log INFO "Waiting for cluster to be ready..."
        log INFO "Cluster creation typically takes 2-3 minutes..."

        # Give cluster creation time to start (initial grace period)
        log INFO "Allowing cluster creation to initialize..."
        sleep 10

        local max_attempts=145  # 145 * 2s = ~4.8 minutes (plus 10s initial = ~5 min total)
        local attempt=0
        local cluster_ready=false
        local last_logged_status=""

        while [ $attempt -lt $max_attempts ]; do
            local cluster_status_response
            cluster_status_response=$(api_call GET "/api/cluster/status")

            local status
            status=$(echo "$cluster_status_response" | jq -r '.status // "Error"')

            if [ "$status" = "Running" ]; then
                cluster_ready=true
                break
            elif [ "$status" = "Error" ]; then
                local error_msg
                error_msg=$(echo "$cluster_status_response" | jq -r '.error // "Unknown error"')
                log ERROR "Cluster status check returned an error"
                log ERROR "This usually indicates a problem with Docker/Podman or the kind binary"
                cleanup_level_1 "Cluster status check failed: $error_msg. Check daemon logs at ${MPC_DEV_ENV_PATH}/logs/daemon.log"
                # If we reach here, user chose retry (option 3)
                retry=true
                continue 2  # Continue outer while loop
            fi

            # Only log status if it changed or every 30 seconds (15 attempts)
            if [ "$status" != "$last_logged_status" ] || [ $((attempt % 15)) -eq 0 ]; then
                log INFO "Cluster status: $status, waiting... (attempt $((attempt + 1))/$max_attempts)"
                last_logged_status="$status"
            fi

            sleep 2
            attempt=$((attempt + 1))
        done

        if [ "$cluster_ready" = false ]; then
            log ERROR "Cluster did not become ready within the timeout period"
            log ERROR "Last status check returned: $status"
            cleanup_level_1 "Timeout waiting for cluster to become ready (waited ~5 minutes). Check daemon logs for details."
            # If we reach here, user chose retry (option 3)
            retry=true
            continue
        fi

        # Verify cluster status one more time
        log INFO "Verifying cluster status..."
        local cluster_status_response
        cluster_status_response=$(api_call GET "/api/cluster/status")
        local cluster_status
        cluster_status=$(echo "$cluster_status_response" | jq -r '.status // "Error"')

        if [ "$cluster_status" != "Running" ]; then
            log ERROR "Final cluster status verification failed"
            log ERROR "Expected status: 'Running', got: '$cluster_status'"
            cleanup_level_1 "Cluster verification failed - status is '$cluster_status' instead of 'Running'. Check daemon logs at ${MPC_DEV_ENV_PATH}/logs/daemon.log"
            # If we reach here, user chose retry (option 3)
            retry=true
            continue
        fi

        # Additional verification with kubectl
        if ! kubectl cluster-info --context kind-konflux >/dev/null 2>&1; then
            log ERROR "kubectl cannot access the cluster (context: kind-konflux)"
            log ERROR "The cluster may have been created but is not properly configured"
            cleanup_level_1 "kubectl cannot access cluster. The cluster may exist but kubectl cannot connect. Check daemon logs at ${MPC_DEV_ENV_PATH}/logs/daemon.log"
            # If we reach here, user chose retry (option 3)
            retry=true
            continue
        fi

        log SUCCESS "Cluster is running and accessible"
    done
}

phase4_minimal_stack_deployment() {
    log INFO "========================================="
    log INFO "Phase 4: MPC Stack Deployment"
    log INFO "========================================="
    log INFO "Deploying minimal MPC stack:"
    log INFO "  - Tekton Pipelines (TaskRun engine)"
    log INFO "  - MPC Operator (controller)"
    log INFO "  - OTP Server (one-time passwords)"
    echo ""

    local retry=true
    while $retry; do
        retry=false

        # Call minimal stack deployment API
        log INFO "Deploying minimal MPC stack..."
        log INFO "This should take 2-3 minutes..."
        local response
        response=$(api_call POST "/api/deploy/minimal-stack")

        # Check if accepted
        if echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
            log SUCCESS "Minimal stack deployment initiated"
        else
            local error
            error=$(echo "$response" | jq -r '.error // "Unknown error"')
            cleanup_level_2 "$error"
            retry=$?
            continue
        fi

        # Poll operation status until deployment completes
        log INFO "Waiting for minimal stack deployment to complete..."

        # Wait for operation to complete (max 5 minutes)
        # poll_operation_status polls every 2 seconds, so 150 iterations = 5 minutes
        if ! poll_operation_status 150; then
            log ERROR "Minimal stack deployment failed or timed out"

            # Get error from daemon
            local daemon_response
            daemon_response=$(daemon_get_status)
            local last_error
            last_error=$(echo "$daemon_response" | jq -r '.last_operation_error // "Unknown error"')

            cleanup_level_2 "$last_error"
            retry=$?
            continue
        fi

        log SUCCESS "MPC stack deployed successfully"
    done
}

phase5_mpc_deployment() {
    log INFO "========================================="
    log INFO "Phase 5: MPC Build & Deployment"
    log INFO "========================================="

    local retry=true
    while $retry; do
        retry=false

        # Call rebuild-and-redeploy API
        log INFO "Building and deploying MPC..."
        log INFO "This may take several minutes..."
        local response
        response=$(api_call POST "/api/mpc/rebuild-and-redeploy")

        # Check if accepted
        if echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
            log SUCCESS "MPC rebuild and redeploy initiated"
        elif echo "$response" | jq -e '.status == "conflict"' >/dev/null 2>&1; then
            log ERROR "Another operation is already in progress"
            cleanup_level_4 "Concurrent operation detected"
            retry=$?
            continue
        else
            local error
            error=$(echo "$response" | jq -r '.error // "Unknown error"')
            cleanup_level_4 "$error"
            retry=$?
            continue
        fi

        # NOTE: The rebuild-and-redeploy endpoint doesn't update operation_status
        # So we need to poll differently - wait for the operation to complete
        # by checking deployment readiness directly
        log INFO "Waiting for MPC build and deployment to complete..."
        log INFO "Build typically takes 5-10 minutes, deployment takes 2-3 minutes..."

        # Wait for build/deploy to complete by checking deployment rollout status
        # Give it time to start and complete (max 20 minutes)
        local max_wait=1200  # 20 minutes in seconds
        local elapsed=0
        local check_interval=10

        # Wait at least 30 seconds for the operation to start
        sleep 30

        local deployment_ready=false
        while [ $elapsed -lt $max_wait ]; do
            # First check if daemon reported an error
            local daemon_response
            daemon_response=$(daemon_get_status)
            local last_error
            last_error=$(echo "$daemon_response" | jq -r '.last_operation_error // ""')
            if [ -n "$last_error" ] && [ "$last_error" != "null" ] && [ "$last_error" != "" ]; then
                log ERROR "Daemon reported deployment error: $last_error"
                cleanup_level_4 "$last_error"
                retry=$?
                break 2  # Break out of both loops
            fi

            # Check if BOTH deployments exist and are ready (MPC controller and OTP server)
            local mpc_ready=false
            local otp_ready=false

            if kubectl rollout status deployment/multi-platform-controller \
                -n multi-platform-controller --timeout=10s >/dev/null 2>&1; then
                mpc_ready=true
            fi

            if kubectl rollout status deployment/multi-platform-otp-server \
                -n multi-platform-controller --timeout=10s >/dev/null 2>&1; then
                otp_ready=true
            fi

            if [ "$mpc_ready" = true ] && [ "$otp_ready" = true ]; then
                deployment_ready=true
                break
            fi

            # Only log every 30 seconds
            if [ $((elapsed % 30)) -eq 0 ]; then
                log INFO "Still waiting for deployments... (MPC: $mpc_ready, OTP: $otp_ready) ($elapsed/$max_wait seconds)"
            fi
            sleep $check_interval
            elapsed=$((elapsed + check_interval))
        done

        if [ "$deployment_ready" = false ]; then
            cleanup_level_4 "Timeout waiting for MPC deployment to complete"
            retry=$?
            continue
        fi

        # Verify both deployments exist
        log INFO "Verifying MPC and OTP deployments..."
        if ! kubectl get deployment multi-platform-controller -n multi-platform-controller >/dev/null 2>&1; then
            cleanup_level_4 "MPC deployment not found in cluster"
            retry=$?
            continue
        fi

        if ! kubectl get deployment multi-platform-otp-server -n multi-platform-controller >/dev/null 2>&1; then
            cleanup_level_4 "OTP server deployment not found in cluster"
            retry=$?
            continue
        fi

        log SUCCESS "MPC and OTP server deployed successfully"
    done

    # Prompt to continue to TaskRun
    echo ""
    if ! prompt_yes_no "MPC deployed successfully. Continue to TaskRun?"; then
        log INFO "User chose not to continue to TaskRun"
        cleanup_level_4 "User chose not to continue" "false"
    fi
}

phase6_taskrun_workflow() {
    log INFO "========================================="
    log INFO "Phase 6: TaskRun Workflow"
    log INFO "========================================="

    # Ensure directories exist
    mkdir -p "$MPC_DEV_ENV_PATH/taskruns"
    mkdir -p "$MPC_DEV_ENV_PATH/logs"

    # Prompt user for TaskRun selection FIRST
    echo ""
    echo "Do you want to apply a TaskRun?"
    echo "[1] Use TaskRun from taskruns/ directory"
    echo "[2] Provide path to TaskRun file"
    echo "[3] Skip TaskRun"
    echo ""

    local choice
    choice=$(read_choice "Your choice: " "3" "1 2 3")

    local taskrun_file=""

    case "$choice" in
        1)
            # List files in taskruns/ directory
            log INFO "Available TaskRuns in taskruns/:"
            if ! list_taskrun_files "$MPC_DEV_ENV_PATH/taskruns"; then
                log ERROR "No TaskRun files found in taskruns/ directory"
                log INFO "Please add TaskRun YAML files to taskruns/ and try again"
                return 0
            fi

            echo ""
            local selection
            selection=$(read_choice "Select TaskRun number: " "1")

            if ! [[ "$selection" =~ ^[0-9]+$ ]] || [ "$selection" -lt 1 ] || [ "$selection" -gt "${#TASKRUN_FILES[@]}" ]; then
                log ERROR "Invalid selection"
                return 1
            fi

            taskrun_file="${TASKRUN_FILES[$((selection-1))]}"
            ;;

        2)
            # Prompt for file path
            read -r -p "Enter path to TaskRun YAML file: " taskrun_file
            taskrun_file="${taskrun_file/#\~/$HOME}"  # Expand tilde

            if [ ! -f "$taskrun_file" ]; then
                log ERROR "File not found: $taskrun_file"
                return 1
            fi
            ;;

        3)
            log INFO "Skipping TaskRun"
            return 0
            ;;

        *)
            log ERROR "Invalid choice"
            return 1
            ;;
    esac

    # Now check if the TaskRun uses local platforms
    echo ""
    log INFO "Analyzing TaskRun file: $(basename "$taskrun_file")"

    if check_taskrun_uses_local_platforms "$taskrun_file"; then
        log SUCCESS "Detected local platform usage (localhost/local/x86_64)"
        log INFO "Skipping AWS credential collection - not needed for local platforms"
    else
        log INFO "No local platform indicators found in TaskRun"
        log INFO "AWS credentials may be needed for cloud platforms"
        echo ""
        log INFO "AWS credentials are required for these platforms:"
        log INFO "  - linux/arm64, linux/amd64, linux-mlarge/arm64, linux-mlarge/amd64"
        echo ""

        if prompt_yes_no "Does your TaskRun use AWS platforms and need AWS secrets deployed?"; then
            log INFO "Collecting AWS credentials for secrets deployment..."

            # Prompt for AWS Access Key ID
            local aws_access_key_id
            prompt_user "Enter AWS Access Key ID" aws_access_key_id

            # Prompt for AWS Secret Access Key (hide input)
            local aws_secret_access_key
            read -r -s -p "Enter AWS Secret Access Key: " aws_secret_access_key
            echo  # Print newline after hidden input

            # Prompt for SSH key path with default
            local ssh_key_path
            local default_ssh_key="${HOME}/.ssh/id_rsa"
            prompt_user "Enter SSH key path" ssh_key_path "$default_ssh_key"

            # Validate that SSH key file exists
            if ! file_exists "$ssh_key_path"; then
                log ERROR "SSH key file does not exist: $ssh_key_path"
                return 1
            fi
            log SUCCESS "SSH key verified: $ssh_key_path"

            # Export credentials as environment variables
            export AWS_ACCESS_KEY_ID="$aws_access_key_id"
            export AWS_SECRET_ACCESS_KEY="$aws_secret_access_key"
            export SSH_KEY_PATH="$ssh_key_path"

            # Deploy secrets via daemon API (Go code)
            log INFO "Deploying AWS secrets to cluster..."
            local response
            response=$(api_call POST "/api/deploy/secrets")

            if ! echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
                log ERROR "Failed to deploy AWS secrets"
                local error_msg
                error_msg=$(echo "$response" | jq -r '.error // "Unknown error"')
                log ERROR "Error: $error_msg"
                return 1
            fi

            # Wait for secrets to be deployed
            log INFO "Waiting for secrets deployment to complete..."
            local max_wait=60
            local elapsed=0
            local check_interval=2
            sleep 5

            local secrets_ready=false
            while [ $elapsed -lt $max_wait ]; do
                # First check if daemon reported an error
                local daemon_response
                daemon_response=$(daemon_get_status)
                local last_error
                last_error=$(echo "$daemon_response" | jq -r '.last_operation_error // ""')
                if [ -n "$last_error" ] && [ "$last_error" != "null" ] && [ "$last_error" != "" ]; then
                    log ERROR "Daemon reported error: $last_error"
                    return 1
                fi

                if kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1 && \
                   kubectl get secret aws-ssh-key -n multi-platform-controller >/dev/null 2>&1; then
                    secrets_ready=true
                    break
                fi
                log INFO "Waiting for secrets... ($elapsed/$max_wait seconds)"
                sleep $check_interval
                elapsed=$((elapsed + check_interval))
            done

            if [ "$secrets_ready" = false ]; then
                log ERROR "Timeout waiting for secrets deployment"
                return 1
            fi

            log SUCCESS "AWS secrets deployed successfully"
        else
            log INFO "Skipping AWS secrets deployment"
        fi
    fi

    echo ""

    # Trigger TaskRun workflow via API
    log INFO "Starting TaskRun workflow: $(basename "$taskrun_file")"

    local response
    response=$(api_call POST "/api/taskrun/run" "{\"yaml_path\": \"$taskrun_file\"}")

    if ! echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
        log ERROR "Failed to start TaskRun workflow"
        local error_msg
        error_msg=$(echo "$response" | jq -r '.error // "Unknown error"')
        log ERROR "Error: $error_msg"
        return 1
    fi

    log SUCCESS "TaskRun workflow started"

    # Move to Phase 7 for monitoring
    phase7_monitoring
}

phase7_monitoring() {
    log INFO "========================================="
    log INFO "Phase 7: TaskRun Monitoring"
    log INFO "========================================="

    log INFO "Waiting for TaskRun to complete..."
    log INFO "(Daemon is applying TaskRun, monitoring status, and streaming logs)"

    # Use existing poll_operation_status function (same as MPC rebuild)
    if ! poll_operation_status 360; then  # 30 minutes max (360 * 5s)
        log ERROR "TaskRun workflow failed or timed out"
        log ERROR "Check daemon logs for details"
        return 1
    fi

    # Get final status from daemon
    local status_json
    status_json=$(daemon_get_status)

    local taskrun_info
    taskrun_info=$(echo "$status_json" | jq -r '.taskrun_info // {}')

    local taskrun_name
    taskrun_name=$(echo "$taskrun_info" | jq -r '.name // "unknown"')

    local taskrun_status
    taskrun_status=$(echo "$taskrun_info" | jq -r '.status // "unknown"')

    local log_file
    log_file=$(echo "$taskrun_info" | jq -r '.log_file // ""')

    # Display results
    echo ""
    echo "========================================"
    if [ "$taskrun_status" = "Succeeded" ]; then
        log SUCCESS "TaskRun completed successfully!"
        log SUCCESS "TaskRun: $taskrun_name"
        log INFO "Logs: $log_file"
    elif [ "$taskrun_status" = "Failed" ]; then
        log ERROR "TaskRun failed"
        log ERROR "TaskRun: $taskrun_name"
        log ERROR "Logs: $log_file"

        # Show last 20 lines of error logs
        if [ -f "$log_file" ]; then
            echo ""
            log INFO "Last 20 lines of logs:"
            tail -20 "$log_file"
        fi
    else
        log WARNING "TaskRun status: $taskrun_status"
        log INFO "TaskRun: $taskrun_name"
    fi
    echo "========================================"

    # Handle Level 5 cleanup/options
    local level5_result
    cleanup_level_5 "$taskrun_name" "$taskrun_status" "$log_file"
    level5_result=$?

    case $level5_result in
        1)
            # Apply another TaskRun
            phase6_taskrun_workflow
            ;;
        2)
            # Rebuild MPC only
            if rebuild_mpc_only; then
                # After rebuild, show Level 5 menu again
                cleanup_level_5 "$taskrun_name" "N/A - MPC rebuilt" "$log_file"
            fi
            ;;
        3)
            # Rebuild MPC + apply new TaskRun
            if rebuild_mpc_only; then
                phase6_taskrun_workflow
            fi
            ;;
    esac
}

# Helper function to rebuild MPC
rebuild_mpc_only() {
    log INFO "Rebuilding MPC..."
    local response
    response=$(api_call POST "/api/mpc/rebuild-and-redeploy")

    if ! echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
        log ERROR "Failed to trigger MPC rebuild"
        return 1
    fi

    log INFO "Waiting for MPC rebuild to complete..."
    if ! poll_operation_status 300; then
        log ERROR "MPC rebuild failed or timed out"
        return 1
    fi

    log SUCCESS "MPC rebuilt successfully"
    return 0
}

phase8_summary() {
    log INFO "========================================="
    log INFO "Environment Setup Complete!"
    log INFO "========================================="

    # Get current state from daemon
    local status_json
    status_json=$(daemon_get_status 2>/dev/null || echo '{}')

    local cluster_status
    cluster_status=$(echo "$status_json" | jq -r '.cluster.status // "unknown"')

    local secrets_configured="skipped"
    if kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1; then
        secrets_configured="configured"
    fi

    # Print summary
    echo ""
    echo "========================================="
    echo "          SETUP SUMMARY"
    echo "========================================="
    echo ""
    echo "✓ Cluster: $cluster_status (konflux)"
    echo "✓ MPC Stack: deployed (Tekton + MPC Operator + OTP)"
    echo "✓ MPC: deployed"
    echo "✓ AWS Secrets: $secrets_configured"

    # Check if any TaskRuns exist
    local taskrun_count
    taskrun_count=$(kubectl get taskrun -n multi-platform-controller --no-headers 2>/dev/null | wc -l)
    if [ "$taskrun_count" -gt 0 ]; then
        echo "✓ TaskRuns: $taskrun_count applied"
    fi

    echo ""
    echo "========================================="
    echo "          USEFUL COMMANDS"
    echo "========================================="
    echo ""
    echo "# View cluster resources:"
    echo "kubectl get all -n multi-platform-controller"
    echo ""
    echo "# View TaskRuns:"
    echo "kubectl get taskrun -n multi-platform-controller"
    echo ""
    echo "# View TaskRun details:"
    echo "kubectl describe taskrun <taskrun-name> -n multi-platform-controller"
    echo ""
    echo "# View logs:"
    echo "ls -lh $MPC_DEV_ENV_PATH/logs/"
    echo "cat $MPC_DEV_ENV_PATH/logs/<logfile>"
    echo ""
    echo "# Rebuild MPC only:"
    echo "curl -X POST http://localhost:8765/api/mpc/rebuild-and-redeploy"
    echo ""
    echo "# Apply another TaskRun:"
    echo "kubectl apply -f $MPC_DEV_ENV_PATH/taskruns/your-taskrun.yaml"
    echo "# or"
    echo "kubectl apply -f <path-to-taskrun.yaml>"
    echo ""
    echo "# Clean up everything:"
    echo "make teardown"
    echo ""
    echo "========================================="
    echo "          DAEMON & LOGS"
    echo "========================================="
    echo ""
    echo "Daemon: running on http://localhost:8765"
    echo "Daemon logs: $MPC_DEV_ENV_PATH/logs/daemon.log"
    echo "Kubeconfig: ~/.kube/config"
    echo ""
    echo "========================================="

    # Final message
    echo ""
    log SUCCESS "Your MPC development environment is ready!"
    echo ""
}

# Setup signal handlers for graceful shutdown
setup_signal_handlers() {
    # Trap SIGINT (Ctrl+C) and SIGTERM
    trap 'cleanup_level_6' INT TERM
}

# ============================================================================
# Main execution function
# ============================================================================

main() {
    log INFO "Starting MPC development environment setup..."
    log INFO "Base directory: ${MPC_DEV_ENV_PATH}"

    # Setup signal handlers
    setup_signal_handlers

    # Execute phases in order
    phase1_prerequisites
    phase2_daemon_startup
    phase3_cluster_setup
    phase4_minimal_stack_deployment
    phase5_mpc_deployment
    phase6_taskrun_workflow
    # phase7_monitoring is called from phase6
    phase8_summary

    log SUCCESS "MPC development environment setup complete!"
}

# Run main function
main "$@"
