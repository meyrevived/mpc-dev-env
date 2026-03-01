#!/usr/bin/env bash

# dev-env.sh - Main orchestration script for the MPC development environment
#
# This is the primary entry point for the complete dev-env workflow. It orchestrates
# all phases of the MPC development environment setup and TaskRun testing:
#
# Phase 1: Prerequisites validation (checks tools, paths, builds daemon)
# Phase 2: Daemon startup (starts API server on port 8765)
# Phase 3: Cluster setup (creates Kind cluster via daemon API)
# Phase 4: MPC stack deployment (deploys Tekton + MPC Operator + OTP)
# Phase 5: MPC build & deployment (builds custom image, patches deployment)
# Phase 6: TaskRun workflow (selects TaskRun, detects platform requirements, smart credential handling, applies)
# Phase 7: TaskRun monitoring (monitors TaskRun, streams logs)
# Phase 8: Summary output (shows status, useful commands)
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

# Set up session directory — all logs for this session go here
# The "latest" directory holds the current TaskRun's logs. When a new TaskRun starts,
# "latest" is renamed to a timestamped directory and a fresh "latest" is created.
SESSION_TIMESTAMP=$(date '+%Y%m%d_%H%M%S')
SESSION_TYPE="${SESSION_TYPE:-dev-env}"
SESSION_DIR="${LOGS_DIR}/latest"

# If latest/ already has content from a previous session, rotate it
if [ -d "$SESSION_DIR" ] && [ "$(ls -A "$SESSION_DIR" 2>/dev/null)" ]; then
    ROTATED_DIR="${LOGS_DIR}/${SESSION_TYPE}_${SESSION_TIMESTAMP}"
    mv "$SESSION_DIR" "$ROTATED_DIR"
fi
mkdir -p "$SESSION_DIR"

# Export for daemon and child processes
export SESSION_LOG_DIR="$SESSION_DIR"

# Capture script output to session log (inside latest/ so it participates in rotation)
SESSION_LOG="${SESSION_DIR}/${SESSION_TYPE}_session_${SESSION_TIMESTAMP}.log"
exec > >(tee -a "$SESSION_LOG") 2>&1
echo "Session log: $SESSION_LOG"
echo "Session directory: $SESSION_DIR"

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

# get_taskrun_platform - Extract PLATFORM parameter from TaskRun YAML
#
# Uses yq to parse the TaskRun YAML and extract the PLATFORM parameter value.
# Returns empty string if PLATFORM parameter not found.
#
# Arguments:
#   $1 - Path to TaskRun YAML file
#
# Outputs:
#   Platform value (e.g., "linux/arm64", "local", "localhost") to stdout
#
# Returns:
#   0 on success (even if PLATFORM not found)
#   1 on file not found error
get_taskrun_platform() {
    local taskrun_file="$1"

    if [ ! -f "$taskrun_file" ]; then
        log_error "TaskRun file not found: $taskrun_file"
        return 1
    fi

    # Check if yq is available
    if ! command -v yq &> /dev/null; then
        log_warning "yq not found, cannot parse PLATFORM parameter" >&2
        return 0
    fi

    # Extract PLATFORM parameter using yq
    local platform
    platform=$(yq eval '.spec.params[] | select(.name == "PLATFORM") | .value' "$taskrun_file" 2>/dev/null || true)

    # Output the platform value (may be empty)
    echo "$platform"
    return 0
}

# check_local_platform_compatible - Verify host can run local platform TaskRun
#
# Checks if the host OS and architecture match requirements for local execution.
# Local platforms require Linux x86_64.
#
# Arguments:
#   $1 - Platform value ("local", "localhost", or "linux/x86_64")
#
# Returns:
#   0 if compatible (Linux x86_64)
#   1 if incompatible (shows error message and returns)
check_local_platform_compatible() {
    local platform="$1"

    # Detect host OS and architecture
    local host_os
    local host_arch
    host_os=$(uname -s)
    host_arch=$(uname -m)

    # Check if compatible
    if [ "$host_os" != "Linux" ] || [ "$host_arch" != "x86_64" ]; then
        echo ""
        log_error "TaskRun requires local execution on Linux x86_64"
        log_error "Your system: $host_os $host_arch"
        echo ""
        log_error "This TaskRun cannot run on your machine."
        log_error "Please select a different TaskRun."
        echo ""
        return 1
    fi

    return 0
}

# prompt_for_aws_profile - Prompt user for AWS SSO profile name
#
# Prompts user to enter their AWS SSO profile name and saves it
# to .env.local for future sessions.
#
# Side effects:
#   - Sets AWS_PROFILE env var
#   - Calls save_config to persist profile name
#
# Returns:
#   0 if profile collected and validated successfully
#   1 if user cancelled
prompt_for_aws_profile() {
    echo ""
    log_info "AWS SSO profile needed for this TaskRun."
    echo ""

    local aws_profile
    prompt_user "Enter your AWS SSO profile name" aws_profile

    if [ -z "$aws_profile" ]; then
        log_error "Profile name cannot be empty"
        return 1
    fi

    export AWS_PROFILE="$aws_profile"

    # Validate the SSO session
    if ! validate_sso_session "$aws_profile"; then
        # Session invalid - enter recovery loop
        if ! sso_recovery_loop "$aws_profile"; then
            return 1
        fi
    fi

    # Save profile to .env.local
    save_config
    log_success "AWS profile '$aws_profile' saved to .env.local"

    return 0
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
    local daemon_log_file="${SESSION_DIR}/daemon_${SESSION_TIMESTAMP}.log"

    # Ensure session directory exists
    mkdir -p "${SESSION_DIR}"

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
    nohup "$daemon_binary" >> "$daemon_log_file" 2>&1 &
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
                cleanup_level_1 "Cluster status check failed: $error_msg. Check daemon logs at ${SESSION_DIR}/daemon_${SESSION_TIMESTAMP}.log"
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
            cleanup_level_1 "Cluster verification failed - status is '$cluster_status' instead of 'Running'. Check daemon logs at ${SESSION_DIR}/daemon_${SESSION_TIMESTAMP}.log"
            # If we reach here, user chose retry (option 3)
            retry=true
            continue
        fi

        # Additional verification with kubectl
        if ! kubectl cluster-info --context kind-konflux >/dev/null 2>&1; then
            log ERROR "kubectl cannot access the cluster (context: kind-konflux)"
            log ERROR "The cluster may have been created but is not properly configured"
            cleanup_level_1 "kubectl cannot access cluster. The cluster may exist but kubectl cannot connect. Check daemon logs at ${SESSION_DIR}/daemon_${SESSION_TIMESTAMP}.log"
            # If we reach here, user chose retry (option 3)
            retry=true
            continue
        fi

        log SUCCESS "Cluster is running and accessible"
    done
}

phase3_5_aws_secrets_deployment() {
    log INFO "========================================="
    log INFO "Phase 3.5: AWS Secrets Deployment"
    log INFO "========================================="
    log INFO "Deploying AWS credentials early to allow controller cache sync time"
    echo ""

    # Check if secrets already exist
    if kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1 && \
       kubectl get secret aws-ssh-key -n multi-platform-controller >/dev/null 2>&1; then
        # Check for stale pre-SSO secrets (missing session-token field)
        if kubectl get secret aws-account -n multi-platform-controller -o jsonpath='{.data.session-token}' 2>/dev/null | grep -q .; then
            # Secrets exist with session-token — but SSO temp creds expire, so validate the session
            local profile="${AWS_PROFILE:-default}"
            if validate_sso_session "$profile" 2>/dev/null; then
                log_success "AWS secrets deployed and SSO session still valid, skipping..."
                return 0
            else
                log_warning "AWS secrets exist but SSO session has expired"
                log_info "Deleting secrets to recreate with fresh SSO credentials..."
                kubectl delete secret aws-account aws-ssh-key -n multi-platform-controller 2>/dev/null || true
            fi
        else
            log_warning "Stale AWS secrets detected (missing session-token field, likely from pre-SSO run)"
            log_info "Deleting stale secrets to recreate with SSO credentials..."
            kubectl delete secret aws-account aws-ssh-key -n multi-platform-controller 2>/dev/null || true
        fi
    fi

    # Check if AWS_PROFILE is set (from .env.local)
    if [ -z "${AWS_PROFILE:-}" ]; then
        # No profile saved, prompt for one
        if ! prompt_for_aws_profile; then
            echo ""
            log_info "Skipping AWS secrets deployment - will prompt again in Phase 6 if needed"
            return 0
        fi
    else
        # Profile exists, validate silently
        if ! validate_sso_session "$AWS_PROFILE"; then
            # Session expired, enter recovery loop
            if ! sso_recovery_loop "$AWS_PROFILE"; then
                log_info "Skipping AWS secrets deployment - will prompt again in Phase 6 if needed"
                return 0
            fi
        fi
    fi

    # Extract temporary credentials from SSO session
    if ! extract_sso_credentials "$AWS_PROFILE"; then
        log_error "Failed to extract credentials from SSO session"
        log_info "Skipping - will retry in Phase 6 if needed"
        return 0
    fi

    # Collect SSH key
    echo ""
    local ssh_key_path
    local default_ssh_key="${HOME}/.ssh/id_rsa"

    # Check if SSH key is already configured and valid
    if [ -n "${SSH_KEY_PATH:-}" ] && file_exists "$SSH_KEY_PATH"; then
        log_info "Using saved SSH key: $SSH_KEY_PATH"
        ssh_key_path="$SSH_KEY_PATH"
    else
        # Prompt for SSH key path
        prompt_user "Enter SSH key path" ssh_key_path "$default_ssh_key"

        # Validate SSH key exists
        if ! file_exists "$ssh_key_path"; then
            log_error "SSH key file does not exist: $ssh_key_path"
            log_info "Skipping AWS secrets - will prompt again in Phase 6 if needed"
            return 0
        fi
        log_success "SSH key verified: $ssh_key_path"
    fi

    export SSH_KEY_PATH="$ssh_key_path"
    save_config

    # Deploy secrets via daemon API
    log_info "Deploying AWS secrets to cluster..."

    local credentials_json
    credentials_json=$(jq -n \
        --arg access_key "$AWS_ACCESS_KEY_ID" \
        --arg secret_key "$AWS_SECRET_ACCESS_KEY" \
        --arg session_token "${AWS_SESSION_TOKEN:-}" \
        --arg ssh_key "$ssh_key_path" \
        '{
            aws_access_key_id: $access_key,
            aws_secret_access_key: $secret_key,
            aws_session_token: $session_token,
            ssh_key_path: $ssh_key
        }')

    local response
    response=$(api_call POST "/api/deploy/secrets" "$credentials_json")

    if ! echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
        log_error "Failed to deploy AWS secrets"
        local error_msg
        error_msg=$(echo "$response" | jq -r '.error // "Unknown error"')
        log_error "Error: $error_msg"
        log_info "Skipping - will retry in Phase 6 if needed"
        return 0
    fi

    # Wait for secrets to exist
    log_info "Waiting for secrets to be created in Kubernetes..."

    local max_wait=30
    local elapsed=0
    while [ $elapsed -lt $max_wait ]; do
        if kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1 && \
           kubectl get secret aws-ssh-key -n multi-platform-controller >/dev/null 2>&1; then
            log_success "AWS secrets deployed successfully"
            log_info "Controller cache will sync secrets in background during MPC deployment"

            # Clear temporary credentials from environment
            unset AWS_ACCESS_KEY_ID
            unset AWS_SECRET_ACCESS_KEY
            unset AWS_SESSION_TOKEN
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done

    log_warning "Secrets deployment taking longer than expected, continuing anyway..."
    log_info "Will verify again in Phase 6 if needed"

    # Clear temporary credentials from environment
    unset AWS_ACCESS_KEY_ID
    unset AWS_SECRET_ACCESS_KEY
    unset AWS_SESSION_TOKEN
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

    # Start streaming controller logs to file (background process)
    log INFO "Starting controller log streaming..."
    start_controller_log_stream

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

    # TaskRun workflow loop - keeps running until user explicitly exits
    while true; do
        # Prompt user for TaskRun selection
        echo ""
        echo "TaskRun Menu:"
        echo "[1] Run TaskRun from taskruns/ directory"
        echo "[2] Provide path to TaskRun file"
        echo "[3] Switch AWS account"
        echo "[4] Skip to summary (cluster stays running)"
        echo "[5] Exit and cleanup (tears down cluster + daemon)"
        echo ""

        local choice
        choice=$(read_choice "Your choice: " "4" "1 2 3 4 5")

        local taskrun_file=""

        case "$choice" in
            1)
                # List files in taskruns/ directory
                log INFO "Available TaskRuns in taskruns/:"
                if ! list_taskrun_files "$MPC_DEV_ENV_PATH/taskruns"; then
                    log ERROR "No TaskRun files found in taskruns/ directory"
                    log INFO "Please add TaskRun YAML files to taskruns/ and try again"
                    echo ""
                    sleep 2
                    continue  # Return to menu
                fi

                echo ""
                local selection
                selection=$(read_choice "Select TaskRun number: " "1")

                if ! [[ "$selection" =~ ^[0-9]+$ ]] || [ "$selection" -lt 1 ] || [ "$selection" -gt "${#TASKRUN_FILES[@]}" ]; then
                    log ERROR "Invalid selection"
                    echo ""
                    sleep 2
                    continue  # Return to menu
                fi

                taskrun_file="${TASKRUN_FILES[$((selection-1))]}"
                ;;

            2)
                # Prompt for file path
                read -r -p "Enter path to TaskRun YAML file: " taskrun_file
                taskrun_file="${taskrun_file/#\~/$HOME}"  # Expand tilde

                if [ ! -f "$taskrun_file" ]; then
                    log ERROR "File not found: $taskrun_file"
                    echo ""
                    sleep 2
                    continue  # Return to menu
                fi
                ;;

            3)
                # Switch AWS account
                echo ""
                log_info "Current AWS profile: ${AWS_PROFILE:-not set}"
                echo ""

                local new_profile
                prompt_user "Enter new AWS SSO profile name" new_profile

                if [ -z "$new_profile" ]; then
                    log_error "Profile name cannot be empty"
                    sleep 2
                    continue
                fi

                export AWS_PROFILE="$new_profile"

                if ! validate_sso_session "$new_profile"; then
                    if ! sso_recovery_loop "$new_profile"; then
                        continue
                    fi
                fi

                # Extract new credentials and redeploy secrets
                if ! extract_sso_credentials "$new_profile"; then
                    log_error "Failed to extract credentials"
                    sleep 2
                    continue
                fi

                save_config
                log_success "Switched to AWS profile: $new_profile"

                # Redeploy secrets with new credentials
                local ssh_key_path="${SSH_KEY_PATH:-${HOME}/.ssh/id_rsa}"
                if [ -z "${SSH_KEY_PATH:-}" ] || ! file_exists "$SSH_KEY_PATH"; then
                    prompt_user "Enter SSH key path" ssh_key_path "$ssh_key_path"
                    export SSH_KEY_PATH="$ssh_key_path"
                    save_config
                fi

                log_info "Redeploying AWS secrets with new profile..."

                local credentials_json
                credentials_json=$(jq -n \
                    --arg access_key "$AWS_ACCESS_KEY_ID" \
                    --arg secret_key "$AWS_SECRET_ACCESS_KEY" \
                    --arg session_token "${AWS_SESSION_TOKEN:-}" \
                    --arg ssh_key "$ssh_key_path" \
                    '{
                        aws_access_key_id: $access_key,
                        aws_secret_access_key: $secret_key,
                        aws_session_token: $session_token,
                        ssh_key_path: $ssh_key
                    }')

                local response
                response=$(api_call POST "/api/deploy/secrets" "$credentials_json")

                if echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
                    sleep 5  # Give secrets time to deploy
                    log_success "AWS secrets redeployed with new profile"
                else
                    log_error "Failed to redeploy secrets"
                fi

                # Clear temp credentials
                unset AWS_ACCESS_KEY_ID
                unset AWS_SECRET_ACCESS_KEY
                unset AWS_SESSION_TOKEN

                echo ""
                sleep 2
                continue
                ;;

            4)
                log INFO "Skipping to summary..."
                return 0  # Exit loop, continue to phase 8
                ;;

            5)
                log INFO "Exiting and cleaning up..."
                cleanup_level_6
                exit 0
                ;;

            *)
                log ERROR "Invalid choice"
                echo ""
                sleep 2
                continue  # Return to menu
                ;;
        esac

        # Parse PLATFORM parameter from TaskRun
        echo ""
        log_info "Analyzing TaskRun file: $(basename "$taskrun_file")"

        local platform
        platform=$(get_taskrun_platform "$taskrun_file")

        if [ -z "$platform" ]; then
            log_warning "TaskRun does not specify PLATFORM parameter"
            log_info "Assuming local platform (no cloud credentials needed)"
            echo ""
            if ! prompt_yes_no "Continue with this TaskRun?"; then
                log_info "TaskRun cancelled"
                echo ""
                sleep 2
                continue  # Return to menu
            fi
        else
            log_info "Platform detected: $platform"

            # Determine platform requirements
            case "$platform" in
                local|localhost|linux/x86_64)
                    # Local platform - check compatibility
                    log_info "Local platform detected, checking compatibility..."
                    if ! check_local_platform_compatible "$platform"; then
                        # Incompatible, error already shown
                        echo ""
                        sleep 2
                        continue  # Return to TaskRun selection
                    fi
                    log_success "System compatible with local platform"
                    # No credentials needed
                    ;;

                linux/ppc64le|linux/s390x)
                    # IBM Cloud platform - not yet supported
                    echo ""
                    log_error "IBM Cloud platforms not yet supported"
                    log_error "Platform detected: $platform"
                    echo ""
                    log_error "IBM Cloud credential support is coming soon."
                    log_error "Please select a different TaskRun."
                    echo ""
                    sleep 2
                    continue  # Return to TaskRun selection
                    ;;

                *)
                    # AWS platform - need SSO profile
                    log_info "AWS platform detected: $platform"

                    # Check if AWS secrets already deployed (from Phase 3.5)
                    if kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1 && \
                       kubectl get secret aws-ssh-key -n multi-platform-controller >/dev/null 2>&1; then
                        # Check for stale pre-SSO secrets (missing session-token field)
                        if kubectl get secret aws-account -n multi-platform-controller -o jsonpath='{.data.session-token}' 2>/dev/null | grep -q .; then
                            # Secrets exist with session-token — validate SSO session is still active
                            local sso_profile="${AWS_PROFILE:-default}"
                            if validate_sso_session "$sso_profile" 2>/dev/null; then
                                log_success "AWS secrets already deployed and SSO session valid, skipping credential collection"
                                # Secrets exist and valid, fall through to TaskRun deployment
                            else
                                log_warning "AWS secrets exist but SSO session has expired"
                                log_info "Deleting secrets to recreate with fresh SSO credentials..."
                                kubectl delete secret aws-account aws-ssh-key -n multi-platform-controller 2>/dev/null || true
                                # Fall through to credential collection below
                            fi
                        else
                            log_warning "Stale AWS secrets detected (missing session-token field, likely from pre-SSO run)"
                            log_info "Deleting stale secrets to recreate with SSO credentials..."
                            kubectl delete secret aws-account aws-ssh-key -n multi-platform-controller 2>/dev/null || true
                            # Fall through to credential collection below
                        fi
                    fi

                    # Check if secrets need to be deployed (either never existed, stale, or expired and deleted)
                    if ! kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1 || \
                       ! kubectl get secret aws-ssh-key -n multi-platform-controller >/dev/null 2>&1; then
                        # Secrets not found, need SSO profile
                        log_info "AWS secrets not found, setting up SSO credentials..."

                        # Check if profile is set
                        if [ -z "${AWS_PROFILE:-}" ]; then
                            if ! prompt_for_aws_profile; then
                                echo ""
                                sleep 2
                                continue  # Return to TaskRun menu
                            fi
                        else
                            # Profile exists, validate
                            if ! validate_sso_session "$AWS_PROFILE"; then
                                if ! sso_recovery_loop "$AWS_PROFILE"; then
                                    echo ""
                                    sleep 2
                                    continue  # Return to TaskRun menu
                                fi
                            fi
                        fi

                        # Extract temporary credentials
                        if ! extract_sso_credentials "$AWS_PROFILE"; then
                            log_error "Failed to extract credentials from SSO session"
                            echo ""
                            sleep 2
                            continue  # Return to TaskRun menu
                        fi

                        # Collect SSH key
                        echo ""
                        local ssh_key_path
                        local default_ssh_key="${HOME}/.ssh/id_rsa"

                        if [ -n "${SSH_KEY_PATH:-}" ] && file_exists "$SSH_KEY_PATH"; then
                            log_info "Using saved SSH key: $SSH_KEY_PATH"
                            ssh_key_path="$SSH_KEY_PATH"
                        else
                            prompt_user "Enter SSH key path" ssh_key_path "$default_ssh_key"

                            if ! file_exists "$ssh_key_path"; then
                                log_error "SSH key file does not exist: $ssh_key_path"
                                echo ""
                                sleep 2
                                continue  # Return to menu
                            fi
                            log_success "SSH key verified: $ssh_key_path"
                        fi

                        export SSH_KEY_PATH="$ssh_key_path"
                        save_config

                        # Deploy secrets via daemon API
                        log_info "Deploying AWS secrets to cluster..."

                        local credentials_json
                        credentials_json=$(jq -n \
                            --arg access_key "$AWS_ACCESS_KEY_ID" \
                            --arg secret_key "$AWS_SECRET_ACCESS_KEY" \
                            --arg session_token "${AWS_SESSION_TOKEN:-}" \
                            --arg ssh_key "$ssh_key_path" \
                            '{
                                aws_access_key_id: $access_key,
                                aws_secret_access_key: $secret_key,
                                aws_session_token: $session_token,
                                ssh_key_path: $ssh_key
                            }')

                        local response
                        response=$(api_call POST "/api/deploy/secrets" "$credentials_json")

                        if ! echo "$response" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
                            log_error "Failed to deploy AWS secrets"
                            local error_msg
                            error_msg=$(echo "$response" | jq -r '.error // "Unknown error"')
                            log_error "Error: $error_msg"
                            echo ""
                            log_info "Returning to TaskRun menu..."
                            sleep 2

                            # Clear temp credentials
                            unset AWS_ACCESS_KEY_ID
                            unset AWS_SECRET_ACCESS_KEY
                            unset AWS_SESSION_TOKEN
                            continue  # Return to menu
                        fi

                        # Wait for secrets to exist
                        log_info "Waiting for secrets deployment to complete..."

                        local max_wait=30
                        local elapsed=0
                        local secrets_exist=false

                        while [ $elapsed -lt $max_wait ]; do
                            if kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1 && \
                               kubectl get secret aws-ssh-key -n multi-platform-controller >/dev/null 2>&1; then
                                secrets_exist=true
                                log_success "AWS secrets deployed successfully"
                                break
                            fi
                            sleep 2
                            elapsed=$((elapsed + 2))
                        done

                        if [ "$secrets_exist" = false ]; then
                            log_error "Timeout: Secrets did not appear in Kubernetes"
                            echo ""
                            log_info "Returning to TaskRun menu..."
                            sleep 2

                            # Clear temp credentials
                            unset AWS_ACCESS_KEY_ID
                            unset AWS_SECRET_ACCESS_KEY
                            unset AWS_SESSION_TOKEN
                            continue  # Return to menu
                        fi

                        # Clear temporary credentials from environment
                        unset AWS_ACCESS_KEY_ID
                        unset AWS_SECRET_ACCESS_KEY
                        unset AWS_SESSION_TOKEN
                    fi  # End of secrets deployment block
                    ;;
            esac
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
            echo ""
            log_info "Returning to TaskRun menu..."
            sleep 2
            continue  # Return to menu
        fi

        log SUCCESS "TaskRun workflow started"

        # Move to Phase 7 for monitoring
        # Note: phase7_monitoring may return non-zero (e.g. poll timeout),
        # but we always want to loop back to the TaskRun menu regardless.
        # Use || true to prevent set -e from killing the script.
        phase7_monitoring || true

        # After monitoring completes, return to menu
        continue
    done  # End of TaskRun workflow loop
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

    # Verify actual success by checking logs for errors
    # Tekton may report "Succeeded" even if MPC host allocation failed
    if [ "$taskrun_status" = "Succeeded" ] && [ -f "$log_file" ]; then
        # Check for common MPC allocation errors in the log
        if grep -qi "Error allocating host\|failed to retrieve EC2 instances\|Secret.*not found\|failed to refresh cached credentials" "$log_file"; then
            log_info "Tekton reported 'Succeeded' but log contains allocation errors - overriding status"
            taskrun_status="Failed"
        fi
    fi

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
    # Note: cleanup_level_5 handles exits (options 5-7) and account switching (option 3) directly
    # For options 1, 2, 4 we return codes 1, 2, 3 to let phase6 loop continue
    # Use || to safely capture non-zero return codes under set -e
    local level5_result=0
    cleanup_level_5 "$taskrun_name" "$taskrun_status" "$log_file" || level5_result=$?

    case $level5_result in
        1)
            # Apply another TaskRun - rotate logs and return to phase6 loop
            rotate_latest_logs
            log_info "Returning to TaskRun menu..."
            return 0
            ;;
        2)
            # Rebuild MPC only
            if rebuild_mpc_only; then
                log_success "MPC rebuild complete"
                echo ""
                log_info "Returning to TaskRun menu..."
                return 0
            else
                log_error "MPC rebuild failed"
                echo ""
                log_info "Returning to TaskRun menu..."
                return 0
            fi
            ;;
        3)
            # Rebuild MPC + apply new TaskRun
            if rebuild_mpc_only; then
                log_success "MPC rebuild complete"
                rotate_latest_logs
                echo ""
                log_info "Returning to TaskRun menu..."
                return 0
            else
                log_error "MPC rebuild failed"
                echo ""
                log_info "Returning to TaskRun menu..."
                return 0
            fi
            ;;
    esac
}

# Rotate the latest/ log directory to a timestamped directory.
# Called before starting a new TaskRun so each TaskRun gets its own log directory.
#
# Files with open file handles (daemon log, session log) are copied to the
# rotated directory and then truncated in place. Both use O_APPEND (daemon via
# ">>" redirect, session via "tee -a"), so writes after truncation start at
# position 0 — no sparse file holes.
rotate_latest_logs() {
    local latest_dir="${LOGS_DIR}/latest"

    # Only rotate if latest/ has content
    if [ ! -d "$latest_dir" ] || [ -z "$(ls -A "$latest_dir" 2>/dev/null)" ]; then
        return 0
    fi

    # Collect Kubernetes artifacts into latest/ before rotating.
    # This is synchronous — guarantees collection finishes before files are moved.
    if daemon_is_running; then
        log_info "Collecting Kubernetes logs and artifacts..."
        api_call POST "/api/collect-logs" || log_warning "Log collection failed (non-fatal)"
    fi

    # Delete old TaskRuns from the namespace so future collections
    # (including the exit-time collection) only find the new TaskRun's pods.
    kubectl delete taskruns --all -n multi-platform-controller 2>/dev/null || true

    # Stop controller log stream before rotation (file handles follow inodes)
    local pid_file="${latest_dir}/.controller_log_stream.pid"
    if [ -f "$pid_file" ]; then
        local stream_pid
        stream_pid=$(cat "$pid_file")
        if kill -0 "$stream_pid" 2>/dev/null; then
            kill "$stream_pid" 2>/dev/null || true
        fi
        rm -f "$pid_file"
    fi

    local rotation_timestamp
    rotation_timestamp=$(date '+%Y%m%d_%H%M%S')
    local rotated_dir="${LOGS_DIR}/${SESSION_TYPE}_${rotation_timestamp}"
    mkdir -p "$rotated_dir"

    # Move/copy files from latest/ to rotated dir
    local file basename
    for file in "$latest_dir"/* "$latest_dir"/.*; do
        [ -e "$file" ] || continue
        basename=$(basename "$file")
        [[ "$basename" == "." || "$basename" == ".." ]] && continue

        if [[ "$basename" == daemon_* || "$basename" == *_session_* ]]; then
            # Daemon and session logs have open fds from long-running processes.
            # Copy content to rotated dir, then truncate originals.
            cp "$file" "$rotated_dir/"
            : > "$file"
        else
            mv "$file" "$rotated_dir/"
        fi
    done

    log_info "Rotated previous logs to: $(basename "$rotated_dir")"

    # Restart controller log stream into fresh latest/
    start_controller_log_stream
}

# Stream controller logs to file in background
start_controller_log_stream() {
    local pid_file="${SESSION_DIR}/.controller_log_stream.pid"

    # Kill any existing log stream process
    if [ -f "$pid_file" ]; then
        local old_pid
        old_pid=$(cat "$pid_file")
        if kill -0 "$old_pid" 2>/dev/null; then
            log_info "Stopping previous controller log stream (PID: $old_pid)"
            kill "$old_pid" 2>/dev/null || true
        fi
        rm -f "$pid_file"
    fi

    # Start new log stream in background
    (
        # Wait for controller pod to be ready and capture its name
        local max_wait=60
        local elapsed=0
        local pod_name=""
        while [ $elapsed -lt $max_wait ]; do
            pod_name=$(kubectl get pods -n multi-platform-controller -l app=multi-platform-controller -o jsonpath='{.items[0].metadata.name}' 2>/dev/null) && break
            sleep 2
            elapsed=$((elapsed + 2))
        done

        if [ -z "$pod_name" ]; then
            echo "WARNING: Controller pod not found after ${max_wait}s" >&2
            return
        fi

        local log_file="${SESSION_DIR}/controller-pod-${pod_name}.log"

        # Stream logs, automatically following pod restarts
        kubectl logs -f -n multi-platform-controller deployment/multi-platform-controller \
            --all-containers=true > "$log_file" 2>&1
    ) &

    local stream_pid=$!
    echo $stream_pid > "$pid_file"
    log_success "Controller logs streaming to: ${SESSION_DIR}/controller-pod-*.log (PID: $stream_pid)"
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
    echo "Daemon logs: ${SESSION_DIR}/daemon_${SESSION_TIMESTAMP}.log"
    echo "Kubeconfig: ~/.kube/config"
    echo ""
    echo "========================================="

    # Final message
    echo ""
    log SUCCESS "Your MPC development environment is ready!"
    echo ""
}

# Setup signal handlers for graceful shutdown
cleanup_level_6() {
    echo ""
    log_info "Cleaning up and exiting..."

    # Collect Kubernetes artifacts before shutdown
    if daemon_is_running; then
        log_info "Collecting Kubernetes logs and artifacts..."
        api_call POST "/api/collect-logs" || log_warning "Log collection failed (non-fatal)"
    fi

    # Stop controller log stream
    local pid_file="${SESSION_DIR}/.controller_log_stream.pid"
    if [ -f "$pid_file" ]; then
        local stream_pid
        stream_pid=$(cat "$pid_file")
        if kill -0 "$stream_pid" 2>/dev/null; then
            log_info "Stopping controller log stream (PID: $stream_pid)"
            kill "$stream_pid" 2>/dev/null || true
        fi
        rm -f "$pid_file"
    fi

    log_info "Goodbye!"
    exit 0
}

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
    phase3_5_aws_secrets_deployment  # Deploy AWS secrets early for cache sync time
    phase4_minimal_stack_deployment
    phase5_mpc_deployment
    phase6_taskrun_workflow
    # phase7_monitoring is called from phase6
    phase8_summary

    log SUCCESS "MPC development environment setup complete!"
}

# Run main function
main "$@"
