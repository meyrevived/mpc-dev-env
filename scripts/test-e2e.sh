#!/usr/bin/env bash

# test-e2e.sh - Non-interactive end-to-end test pipeline for MPC dev-env
#
# Runs a single TaskRun through the full MPC dev-env lifecycle:
#   1. Teardown any existing environment (clean slate)
#   2. Start daemon
#   3. Create Kind cluster
#   4. Deploy MPC stack (Tekton + MPC Operator + OTP)
#   5. Build & deploy MPC from source
#   6. Deploy cloud credentials if needed (smart detection)
#   7. Apply TaskRun
#   8. Monitor until completion
#   9. Collect logs
#   10. Teardown (always, via EXIT trap)
#
# This script is NON-INTERACTIVE. It never prompts the user.
# If something fails, it tears down and exits with a non-zero code.
#
# Prerequisites:
#   - For cloud platform TaskRuns: AWS_PROFILE must be set (in .env.local or env)
#     and the SSO session must be active (run `aws sso login` beforehand)
#   - The daemon binary must already be built (Makefile `build` target handles this)
#
# Usage:
#   TASKRUN_FILE=taskruns/e2e_arm64_test.yaml bash scripts/test-e2e.sh
#   # Or via Makefile:
#   make test-e2e TASKRUN=taskruns/e2e_arm64_test.yaml

set -euo pipefail

# ============================================================================
# Setup: script location, source helpers, validate inputs
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Source shared non-interactive helpers
# shellcheck source=scripts/utils.sh
source "${SCRIPT_DIR}/utils.sh"
# shellcheck source=scripts/api-client.sh
source "${SCRIPT_DIR}/api-client.sh"

# Set MPC_DEV_ENV_PATH based on script location if not already set
if [ -z "${MPC_DEV_ENV_PATH:-}" ]; then
    export MPC_DEV_ENV_PATH="$(dirname "$SCRIPT_DIR")"
fi

# Resolve TASKRUN_FILE — default to localhost_test.yaml
TASKRUN_FILE="${TASKRUN_FILE:-${MPC_DEV_ENV_PATH}/taskruns/localhost_test.yaml}"

# If TASKRUN_FILE is a relative path, resolve it relative to MPC_DEV_ENV_PATH
if [[ "$TASKRUN_FILE" != /* ]]; then
    TASKRUN_FILE="${MPC_DEV_ENV_PATH}/${TASKRUN_FILE}"
fi

# ============================================================================
# Logging setup
# ============================================================================

LOGS_DIR="${MPC_DEV_ENV_PATH}/logs"
SESSION_TIMESTAMP=$(date '+%Y%m%d_%H%M%S')
SESSION_TYPE="test-e2e"
SESSION_DIR="${LOGS_DIR}/latest"

# Rotate previous latest/ if it has content
if [ -d "$SESSION_DIR" ] && [ "$(ls -A "$SESSION_DIR" 2>/dev/null)" ]; then
    ROTATED_DIR="${LOGS_DIR}/${SESSION_TYPE}_${SESSION_TIMESTAMP}"
    mv "$SESSION_DIR" "$ROTATED_DIR"
fi
mkdir -p "$SESSION_DIR"

export SESSION_LOG_DIR="$SESSION_DIR"

SESSION_LOG="${SESSION_DIR}/${SESSION_TYPE}_session_${SESSION_TIMESTAMP}.log"
exec > >(tee -a "$SESSION_LOG") 2>&1
echo "Session log: $SESSION_LOG"
echo "Session directory: $SESSION_DIR"

# ============================================================================
# Track overall result for exit code
# ============================================================================

TEST_RESULT=1  # Default to failure; set to 0 only on TaskRun success

# ============================================================================
# EXIT trap: always teardown, always collect logs
# ============================================================================

cleanup_on_exit() {
    echo ""

    # Collect logs if daemon is still running
    if daemon_is_running; then
        log_info "Collecting Kubernetes logs and artifacts before teardown..."
        api_call POST "/api/collect-logs" 2>/dev/null || log_warning "Log collection failed (non-fatal)"
    fi

    # Stop controller log stream if running
    local pid_file="${SESSION_DIR}/.controller_log_stream.pid"
    if [ -f "$pid_file" ]; then
        local stream_pid
        stream_pid=$(cat "$pid_file")
        if kill -0 "$stream_pid" 2>/dev/null; then
            kill "$stream_pid" 2>/dev/null || true
        fi
        rm -f "$pid_file"
    fi

    log_info "Tearing down environment..."
    cd "$MPC_DEV_ENV_PATH" || true
    make teardown 2>/dev/null || true

    echo ""
    log_info "========================================="
    if [ "$TEST_RESULT" -eq 0 ]; then
        log_success "  E2E TEST PASSED"
    else
        log_error "  E2E TEST FAILED"
    fi
    log_info "  Logs: $SESSION_DIR"
    log_info "========================================="

    exit "$TEST_RESULT"
}

trap cleanup_on_exit EXIT

# ============================================================================
# fail: log error and exit (EXIT trap handles teardown)
# ============================================================================

fail() {
    log_error "$@"
    exit 1
}

# ============================================================================
# Phase 1: Validate environment (non-interactive)
# ============================================================================

log_info "========================================="
log_info "  MPC Dev Environment E2E Test"
log_info "  Non-interactive pipeline"
log_info "========================================="
log_info ""
log_info "TaskRun: $TASKRUN_FILE"
log_info ""

# Validate TaskRun file exists
if [ ! -f "$TASKRUN_FILE" ]; then
    fail "TaskRun file not found: $TASKRUN_FILE"
fi

# Load saved config (AWS_PROFILE, SSH_KEY_PATH, etc.)
load_config

# Validate/auto-detect MPC_REPO_PATH (non-interactive)
if [ -z "${MPC_REPO_PATH:-}" ]; then
    PARENT_DIR="$(dirname "$MPC_DEV_ENV_PATH")"
    if [ -d "${PARENT_DIR}/multi-platform-controller" ]; then
        export MPC_REPO_PATH="${PARENT_DIR}/multi-platform-controller"
    else
        fail "MPC_REPO_PATH not set and auto-detection failed. Set it in .env.local or environment."
    fi
fi

if [ ! -d "$MPC_REPO_PATH" ]; then
    fail "MPC_REPO_PATH does not exist: $MPC_REPO_PATH"
fi

log_success "MPC_DEV_ENV_PATH: $MPC_DEV_ENV_PATH"
log_success "MPC_REPO_PATH: $MPC_REPO_PATH"

# Validate daemon binary exists (Makefile `build` dependency should have built it)
DAEMON_BINARY="${MPC_DEV_ENV_PATH}/bin/mpc-daemon"
if [ ! -x "$DAEMON_BINARY" ]; then
    fail "Daemon binary not found at $DAEMON_BINARY. Run 'make build' first."
fi

# ============================================================================
# Phase 2: Clean slate
# ============================================================================

log_info "Starting with clean slate..."
cd "$MPC_DEV_ENV_PATH" || fail "Cannot cd to $MPC_DEV_ENV_PATH"
make teardown 2>/dev/null || true
sleep 2
log_success "Clean slate ready"

# ============================================================================
# Phase 3: Start daemon
# ============================================================================

log_info "Starting daemon..."

DAEMON_PID_FILE="${MPC_DEV_ENV_PATH}/daemon.pid"
DAEMON_LOG_FILE="${SESSION_DIR}/daemon_${SESSION_TIMESTAMP}.log"

# Kill any lingering daemon
if lsof -ti :8765 >/dev/null 2>&1; then
    lsof -ti :8765 | xargs kill -9 2>/dev/null || true
    sleep 2
fi
rm -f "$DAEMON_PID_FILE"

# Start daemon in background
nohup "$DAEMON_BINARY" >> "$DAEMON_LOG_FILE" 2>&1 &
DAEMON_PID=$!
echo "$DAEMON_PID" > "$DAEMON_PID_FILE"
log_info "Daemon PID: $DAEMON_PID"

# Wait for daemon (120s timeout)
if ! daemon_wait_ready 120; then
    fail "Daemon failed to start within 120 seconds. Check: $DAEMON_LOG_FILE"
fi

# Validate prerequisites via daemon
log_info "Validating prerequisites..."
PREREQ_RESPONSE=$(api_call GET "/api/prerequisites")
ALL_MET=$(echo "$PREREQ_RESPONSE" | jq -r '.all_met // false')
if [ "$ALL_MET" != "true" ]; then
    log_error "Prerequisites check failed:"
    echo "$PREREQ_RESPONSE" | jq -r '.errors[]? // empty' | while IFS= read -r error; do
        log_error "  - $error"
    done
    fail "Prerequisites not met"
fi
log_success "Prerequisites validated"

# ============================================================================
# Phase 4: Create Kind cluster
# ============================================================================

log_info "Creating Kind cluster..."
RESPONSE=$(api_call POST "/api/cluster/start")
if ! echo "$RESPONSE" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
    ERROR=$(echo "$RESPONSE" | jq -r '.error // "Unknown error"')
    fail "Cluster creation failed: $ERROR"
fi

log_info "Waiting for cluster to be ready (up to 5 minutes)..."
sleep 10  # Initial grace period

MAX_ATTEMPTS=145  # ~4.8 minutes at 2s intervals
ATTEMPT=0
CLUSTER_READY=false

while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
    STATUS_RESPONSE=$(api_call GET "/api/cluster/status")
    STATUS=$(echo "$STATUS_RESPONSE" | jq -r '.status // "Error"')

    if [ "$STATUS" = "Running" ]; then
        CLUSTER_READY=true
        break
    elif [ "$STATUS" = "Error" ]; then
        ERROR=$(echo "$STATUS_RESPONSE" | jq -r '.error // "Unknown error"')
        fail "Cluster creation error: $ERROR"
    fi

    if [ $((ATTEMPT % 15)) -eq 0 ]; then
        log_info "Cluster status: $STATUS (attempt $((ATTEMPT + 1))/$MAX_ATTEMPTS)"
    fi

    sleep 2
    ATTEMPT=$((ATTEMPT + 1))
done

if [ "$CLUSTER_READY" = false ]; then
    fail "Cluster did not become ready within 5 minutes"
fi

# Verify kubectl access
if ! kubectl cluster-info --context kind-konflux >/dev/null 2>&1; then
    fail "kubectl cannot access the cluster (context: kind-konflux)"
fi

log_success "Cluster is running and accessible"

# ============================================================================
# Phase 5: Deploy MPC stack
# ============================================================================

log_info "Deploying MPC stack (Tekton + MPC Operator + OTP)..."
RESPONSE=$(api_call POST "/api/deploy/minimal-stack")
if ! echo "$RESPONSE" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
    ERROR=$(echo "$RESPONSE" | jq -r '.error // "Unknown error"')
    fail "MPC stack deployment failed: $ERROR"
fi

log_info "Waiting for MPC stack deployment (up to 5 minutes)..."
if ! poll_operation_status 150; then
    fail "MPC stack deployment failed or timed out"
fi
log_success "MPC stack deployed"

# ============================================================================
# Phase 6: Build & deploy MPC from source
# ============================================================================

log_info "Building and deploying MPC from source..."
RESPONSE=$(api_call POST "/api/mpc/rebuild-and-redeploy")
if ! echo "$RESPONSE" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
    ERROR=$(echo "$RESPONSE" | jq -r '.error // "Unknown error"')
    fail "MPC rebuild-and-redeploy failed: $ERROR"
fi

log_info "Waiting for MPC build and deployment (up to 20 minutes)..."
sleep 30  # Grace period for build to start

MAX_WAIT=1200  # 20 minutes
ELAPSED=0
CHECK_INTERVAL=10
DEPLOYMENT_READY=false

while [ $ELAPSED -lt $MAX_WAIT ]; do
    # Check for daemon error
    DAEMON_RESPONSE=$(daemon_get_status)
    LAST_ERROR=$(echo "$DAEMON_RESPONSE" | jq -r '.last_operation_error // ""')
    if [ -n "$LAST_ERROR" ] && [ "$LAST_ERROR" != "null" ] && [ "$LAST_ERROR" != "" ]; then
        fail "MPC deployment error: $LAST_ERROR"
    fi

    # Check deployments
    MPC_READY=false
    OTP_READY=false

    if kubectl rollout status deployment/multi-platform-controller \
        -n multi-platform-controller --timeout=10s >/dev/null 2>&1; then
        MPC_READY=true
    fi

    if kubectl rollout status deployment/multi-platform-otp-server \
        -n multi-platform-controller --timeout=10s >/dev/null 2>&1; then
        OTP_READY=true
    fi

    if [ "$MPC_READY" = true ] && [ "$OTP_READY" = true ]; then
        DEPLOYMENT_READY=true
        break
    fi

    if [ $((ELAPSED % 30)) -eq 0 ]; then
        log_info "Waiting for deployments... (MPC: $MPC_READY, OTP: $OTP_READY) ($ELAPSED/${MAX_WAIT}s)"
    fi

    sleep $CHECK_INTERVAL
    ELAPSED=$((ELAPSED + CHECK_INTERVAL))
done

if [ "$DEPLOYMENT_READY" = false ]; then
    fail "MPC deployment timed out after 20 minutes"
fi

log_success "MPC and OTP server deployed"

# Wait for daemon to finalize the rebuild-and-redeploy operation.
# The kubectl rollout checks above can succeed before the daemon's goroutine
# sets operation_status back to "idle". If we proceed too early, the next
# operation's status can be overwritten by the late-finishing rebuild goroutine.
log_info "Waiting for daemon to finalize rebuild operation..."
if ! poll_operation_status 60; then
    fail "Daemon did not complete rebuild-and-redeploy operation"
fi

# Start controller log streaming
log_info "Starting controller log streaming..."
(
    MAX_WAIT=60
    ELAPSED=0
    POD_NAME=""
    while [ $ELAPSED -lt $MAX_WAIT ]; do
        POD_NAME=$(kubectl get pods -n multi-platform-controller -l app=multi-platform-controller \
            -o jsonpath='{.items[0].metadata.name}' 2>/dev/null) && break
        sleep 2
        ELAPSED=$((ELAPSED + 2))
    done
    if [ -n "$POD_NAME" ]; then
        kubectl logs -f -n multi-platform-controller deployment/multi-platform-controller \
            --all-containers=true > "${SESSION_DIR}/controller-pod-${POD_NAME}.log" 2>&1
    fi
) &
STREAM_PID=$!
echo $STREAM_PID > "${SESSION_DIR}/.controller_log_stream.pid"

# ============================================================================
# Phase 7: Smart credential detection + deployment
# ============================================================================

log_info "Detecting platform requirements from TaskRun..."

# Parse PLATFORM from TaskRun YAML using yq
PLATFORM=""
if command_exists yq; then
    PLATFORM=$(yq eval '.spec.params[] | select(.name == "PLATFORM") | .value' "$TASKRUN_FILE" 2>/dev/null || true)
fi

if [ -z "$PLATFORM" ]; then
    log_warning "Could not parse PLATFORM from TaskRun (yq missing or PLATFORM not set)"
    log_info "Assuming local platform — skipping credential deployment"
else
    log_info "Platform detected: $PLATFORM"

    case "$PLATFORM" in
        local|localhost|linux/x86_64)
            log_info "Local platform — no cloud credentials needed"
            ;;
        linux/ppc64le|linux/s390x)
            fail "IBM Cloud platforms not yet supported in test-e2e"
            ;;
        *)
            # Cloud platform — need AWS credentials
            log_info "Cloud platform detected — deploying AWS credentials"

            # Require AWS_PROFILE
            if [ -z "${AWS_PROFILE:-}" ]; then
                fail "Cloud platform detected but AWS_PROFILE not set. Set it in .env.local or environment before running test-e2e."
            fi

            # Validate SSO session
            if ! validate_sso_session "$AWS_PROFILE"; then
                fail "SSO session expired for profile '$AWS_PROFILE'. Run 'aws sso login --profile $AWS_PROFILE' before test-e2e."
            fi

            # Extract temporary credentials
            if ! extract_sso_credentials "$AWS_PROFILE"; then
                fail "Failed to extract credentials from SSO session for profile '$AWS_PROFILE'"
            fi

            # Get SSH key path
            SSH_KEY="${SSH_KEY_PATH:-${HOME}/.ssh/id_rsa}"
            if [ ! -f "$SSH_KEY" ]; then
                fail "SSH key not found: $SSH_KEY. Set SSH_KEY_PATH in .env.local or environment."
            fi

            # Deploy secrets via daemon API
            log_info "Deploying AWS secrets to cluster..."
            CREDENTIALS_JSON=$(jq -n \
                --arg access_key "$AWS_ACCESS_KEY_ID" \
                --arg secret_key "$AWS_SECRET_ACCESS_KEY" \
                --arg session_token "${AWS_SESSION_TOKEN:-}" \
                --arg ssh_key "$SSH_KEY" \
                '{
                    aws_access_key_id: $access_key,
                    aws_secret_access_key: $secret_key,
                    aws_session_token: $session_token,
                    ssh_key_path: $ssh_key
                }')

            RESPONSE=$(api_call POST "/api/deploy/secrets" "$CREDENTIALS_JSON")
            if ! echo "$RESPONSE" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
                ERROR=$(echo "$RESPONSE" | jq -r '.error // "Unknown error"')
                fail "Failed to deploy AWS secrets: $ERROR"
            fi

            # Wait for secrets to exist in K8s
            log_info "Waiting for secrets to be created..."
            MAX_SECRET_WAIT=30
            SECRET_ELAPSED=0
            while [ $SECRET_ELAPSED -lt $MAX_SECRET_WAIT ]; do
                if kubectl get secret aws-account -n multi-platform-controller >/dev/null 2>&1 && \
                   kubectl get secret aws-ssh-key -n multi-platform-controller >/dev/null 2>&1; then
                    break
                fi
                sleep 2
                SECRET_ELAPSED=$((SECRET_ELAPSED + 2))
            done

            if [ $SECRET_ELAPSED -ge $MAX_SECRET_WAIT ]; then
                fail "Timeout waiting for AWS secrets to appear in Kubernetes"
            fi

            log_success "AWS secrets deployed"

            # Clear temporary credentials from environment
            unset AWS_ACCESS_KEY_ID
            unset AWS_SECRET_ACCESS_KEY
            unset AWS_SESSION_TOKEN
            unset CREDENTIALS_JSON
            ;;
    esac
fi

# ============================================================================
# Phase 8: Apply TaskRun
# ============================================================================

log_info "Applying TaskRun: $(basename "$TASKRUN_FILE")"
RESPONSE=$(api_call POST "/api/taskrun/run" "$(jq -n --arg p "$TASKRUN_FILE" '{yaml_path: $p}')")
if ! echo "$RESPONSE" | jq -e '.status == "accepted"' >/dev/null 2>&1; then
    ERROR=$(echo "$RESPONSE" | jq -r '.error // "Unknown error"')
    fail "Failed to start TaskRun: $ERROR"
fi
log_success "TaskRun applied"

# ============================================================================
# Phase 9: Monitor TaskRun (up to 30 minutes)
# ============================================================================

log_info "Monitoring TaskRun (up to 30 minutes)..."
if ! poll_operation_status 900; then  # 900 * 2s = 30 minutes
    fail "TaskRun failed or timed out"
fi

# Get final status
STATUS_JSON=$(daemon_get_status)
TASKRUN_INFO=$(echo "$STATUS_JSON" | jq -r '.taskrun_info // {}')
TASKRUN_NAME=$(echo "$TASKRUN_INFO" | jq -r '.name // "unknown"')
TASKRUN_STATUS=$(echo "$TASKRUN_INFO" | jq -r '.status // "unknown"')
LOG_FILE=$(echo "$TASKRUN_INFO" | jq -r '.log_file // ""')

# Verify actual success by checking logs for allocation errors
if [ "$TASKRUN_STATUS" = "Succeeded" ] && [ -f "$LOG_FILE" ]; then
    if grep -qi "Error allocating host\|failed to retrieve EC2 instances\|Secret.*not found\|failed to refresh cached credentials" "$LOG_FILE"; then
        log_warning "Tekton reported 'Succeeded' but log contains allocation errors — overriding status"
        TASKRUN_STATUS="Failed"
    fi
fi

# ============================================================================
# Phase 10: Report results (EXIT trap handles teardown)
# ============================================================================

echo ""
log_info "========================================="
log_info "  TaskRun: $TASKRUN_NAME"
log_info "  Status:  $TASKRUN_STATUS"
if [ -n "$LOG_FILE" ] && [ "$LOG_FILE" != "null" ]; then
    log_info "  Logs:    $LOG_FILE"
fi
log_info "========================================="

if [ "$TASKRUN_STATUS" = "Succeeded" ]; then
    TEST_RESULT=0
else
    TEST_RESULT=1
    # Show last 20 lines of logs on failure
    if [ -f "$LOG_FILE" ]; then
        echo ""
        log_info "Last 20 lines of TaskRun logs:"
        tail -20 "$LOG_FILE"
    fi
fi

# EXIT trap handles teardown and final pass/fail banner
