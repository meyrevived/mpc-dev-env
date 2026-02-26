#!/usr/bin/env bash

# cleanup.sh - Multi-level cleanup functions for the MPC dev-env workflow
#
# This script implements 6 distinct cleanup levels that correspond to different
# failure points or completion states in the dev-env workflow:
#
#   Level 1: Cluster failed to start
#   Level 2: MPC build/deployment failed or user declined to continue
#   Level 3: Secrets deployment failed or user declined
#   Level 4: Konflux deployment failed or user declined
#   Level 5: After TaskRun completion (success or failure)
#   Level 6: User interruption (Ctrl+C)
#
# Each cleanup level offers appropriate options for that stage:
#   - What to delete (cluster, nothing)
#   - Whether to stop the daemon
#   - Whether to exit the tool
#   - Whether to retry the failed operation
#
# Design principle: Give users maximum flexibility to debug issues by choosing
# exactly what to keep running and what to clean up.
#
# Usage:
#   source scripts/cleanup.sh
#   cleanup_level_1  # Called when cluster fails to start
#   cleanup_level_5  # Called after TaskRun completes

# Stop the daemon
cleanup_stop_daemon() {
    log_info "Stopping daemon..."
    if [ -f daemon.pid ]; then
        local pid
        pid=$(cat daemon.pid)
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid"
            rm -f daemon.pid
            log_success "Daemon stopped"
        fi
    elif lsof -ti :8765 >/dev/null 2>&1; then
        lsof -ti :8765 | xargs kill -9 2>/dev/null
        log_success "Daemon stopped (via port)"
    else
        log_info "Daemon not running"
    fi
}

# Delete Kind cluster
cleanup_delete_cluster() {
    log_info "Deleting Kind cluster..."
    if KIND_EXPERIMENTAL_PROVIDER=podman kind get clusters 2>/dev/null | grep -q "^konflux$"; then
        KIND_EXPERIMENTAL_PROVIDER=podman kind delete cluster --name konflux
        log_success "Cluster deleted"
    else
        log_info "No cluster to delete"
    fi
}

# Prompt user if they want to exit
prompt_exit() {
    if prompt_yes_no "Do you want to exit the tool?"; then
        log_info "Exiting..."
        exit 0
    fi
    log_info "Continuing without exiting..."
}

# Level 1: Cluster failed to start
cleanup_level_1() {
    local error_message="$1"

    log_error "========================================="
    log_error "  Cluster Creation Failed"
    log_error "========================================="
    log_error ""
    log_error "Error: $error_message"
    log_error ""
    log_error "Logs can be found at:"
    log_error "  - Daemon logs: ${MPC_DEV_ENV_PATH}/logs/daemon.log"
    log_error ""
    log_error "Common causes:"
    log_error "  - Docker/Podman not running"
    log_error "  - Insufficient resources (disk space, memory)"
    log_error "  - kind binary not found or not executable"
    log_error "========================================="
    echo ""
    echo "Cleanup options:"
    echo "[1] Stop daemon and exit"
    echo "[2] Keep daemon running for debugging"
    echo "[3] Retry cluster creation"
    echo ""

    local choice
    choice=$(read_choice "Your choice: " "1" "1 2 3")

    case "$choice" in
        1)
            cleanup_stop_daemon
            exit 1
            ;;
        2)
            log_info "Keeping daemon running"
            exit 1
            ;;
        3)
            log_info "Retrying cluster creation..."
            return 0  # Signal retry
            ;;
        *)
            # Should never reach here due to validation in read_choice
            log_error "Unexpected choice: $choice, using default"
            cleanup_stop_daemon
            exit 1
            ;;
    esac
}

# Level 2: MPC Stack Deployment Failed or User Declined
cleanup_level_2() {
    local error_message="${1:-User chose not to continue}"
    local show_error="${2:-true}"

    if [ "$show_error" = "true" ]; then
        log_error "========================================="
        log_error "  MPC Stack Deployment Failed"
        log_error "========================================="
        log_error ""
        log_error "Error: $error_message"
        log_error ""
        log_error "Logs can be found at:"
        log_error "  - Daemon logs: ${MPC_DEV_ENV_PATH}/logs/daemon.log"
        log_error "========================================="
    else
        log_warning "$error_message"
    fi

    echo ""
    echo "Cleanup options:"
    echo "[1] Delete cluster, stop daemon"
    echo "[2] Keep everything, stop daemon"
    echo "[3] Keep everything, keep daemon (you can manually fix the stack)"
    echo "[4] Retry MPC stack deployment"
    echo ""

    local choice
    choice=$(read_choice "Your choice: " "1" "1 2 3 4")

    case "$choice" in
        1)
            cleanup_delete_cluster
            cleanup_stop_daemon
            prompt_exit
            return 1
            ;;
        2)
            cleanup_stop_daemon
            prompt_exit
            return 1
            ;;
        3)
            prompt_exit
            return 1
            ;;
        4)
            log_info "Retrying MPC stack deployment..."
            return 0  # Signal retry
            ;;
        *)
            # Should never reach here due to validation in read_choice
            log_error "Unexpected choice: $choice, using default"
            cleanup_delete_cluster
            cleanup_stop_daemon
            prompt_exit
            return 1
            ;;
    esac
}

# Level 4: MPC Build/Deployment Failed or User Declined
cleanup_level_4() {
    local error_message="${1:-User chose not to continue}"
    local show_error="${2:-true}"

    if [ "$show_error" = "true" ]; then
        log_error "========================================="
        log_error "  MPC Build/Deployment Failed"
        log_error "========================================="
        log_error ""
        log_error "Error: $error_message"
        log_error ""
        log_error "Logs can be found at:"
        log_error "  - Daemon logs: ${MPC_DEV_ENV_PATH}/logs/daemon.log"
        log_error "  - Build output: Check daemon logs for details"
        log_error "========================================="
    else
        log_warning "$error_message"
    fi

    echo ""
    echo "Cleanup options:"
    echo "[1] Delete cluster, stop daemon"
    echo "[2] Keep cluster, stop daemon (allows you to debug cluster)"
    echo "[3] Keep cluster, keep daemon (you can manually retry MPC deployment)"
    echo "[4] Retry MPC build/deployment"
    echo ""

    local choice
    choice=$(read_choice "Your choice: " "1" "1 2 3 4")

    case "$choice" in
        1)
            cleanup_delete_cluster
            cleanup_stop_daemon
            prompt_exit
            return 1
            ;;
        2)
            cleanup_stop_daemon
            prompt_exit
            return 1
            ;;
        3)
            prompt_exit
            return 1
            ;;
        4)
            log_info "Retrying MPC build/deployment..."
            return 0  # Signal retry
            ;;
        *)
            # Should never reach here due to validation in read_choice
            log_error "Unexpected choice: $choice, using default"
            cleanup_delete_cluster
            cleanup_stop_daemon
            prompt_exit
            return 1
            ;;
    esac
}

# Level 5: After TaskRun Completion
cleanup_level_5() {
    local taskrun_name="$1"
    local taskrun_status="$2"
    local log_file="$3"

    echo ""
    log_info "TaskRun '$taskrun_name' completed with status: $taskrun_status"
    log_info "Logs saved to: $log_file"

    while true; do
        echo ""
        echo "What would you like to do next?"
        echo "[1] Apply another TaskRun (keeps everything running)"
        echo "[2] Rebuild MPC only (fix code, redeploy, test again)"
        echo "[3] Switch AWS account"
        echo "[4] Rebuild MPC + apply new TaskRun"
        echo "[5] Full teardown (delete cluster, stop daemon, exit)"
        echo "[6] Partial teardown (delete cluster, keep daemon, exit)"
        echo "[7] Exit only (keep cluster + daemon running for manual work)"
        echo ""

        local choice
        choice=$(read_choice "Your choice: " "5" "1 2 3 4 5 6 7")

        case "$choice" in
            1)
                log_info "Applying another TaskRun..."
                return 1  # Signal: run taskrun selection again
                ;;
            2)
                log_info "Rebuilding MPC..."
                return 2  # Signal: rebuild MPC then show this menu again
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

                # Delete existing secrets so they get recreated with new credentials
                kubectl delete secret aws-account -n multi-platform-controller 2>/dev/null || true
                kubectl delete secret aws-ssh-key -n multi-platform-controller 2>/dev/null || true

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
                continue  # Return to this menu
                ;;
            4)
                log_info "Rebuilding MPC and applying new TaskRun..."
                return 3  # Signal: rebuild MPC then run taskrun selection
                ;;
            5)
                log_info "Full teardown..."
                cleanup_delete_cluster
                cleanup_stop_daemon
                exit 0
                ;;
            6)
                log_info "Partial teardown..."
                cleanup_delete_cluster
                exit 0
                ;;
            7)
                log_info "Exiting, cluster and daemon still running"
                exit 0
                ;;
            *)
                # Should never reach here due to validation in read_choice
                log_error "Unexpected choice: $choice, using default (full teardown)"
                cleanup_delete_cluster
                cleanup_stop_daemon
                exit 0
                ;;
        esac
    done
}

# Level 6: User interruption (Ctrl+C)
cleanup_level_6() {
    echo ""
    log_warning "Interrupted by user (Ctrl+C)"

    # Get current state
    local cluster_status="stopped"
    local mpc_status="not deployed"

    if kind get clusters 2>/dev/null | grep -q "^konflux$"; then
        cluster_status="running"
    fi

    if kubectl get deployment multi-platform-controller -n multi-platform-controller >/dev/null 2>&1; then
        mpc_status="deployed"
    fi

    echo ""
    echo "Current state:"
    echo "- Cluster: $cluster_status"
    echo "- Daemon: running"
    echo "- MPC: $mpc_status"
    echo ""
    echo "Cleanup options:"
    echo "[1] Delete cluster, stop daemon, exit"
    echo "[2] Keep cluster, stop daemon, exit"
    echo "[3] Keep everything running, exit"
    echo ""

    local choice
    choice=$(read_choice "Your choice: " "1" "1 2 3")

    case "$choice" in
        1)
            cleanup_delete_cluster
            cleanup_stop_daemon
            exit 130  # Standard exit code for SIGINT
            ;;
        2)
            cleanup_stop_daemon
            exit 130
            ;;
        3)
            log_info "Keeping everything running"
            exit 130
            ;;
        *)
            # Should never reach here due to validation in read_choice
            log_error "Unexpected choice: $choice, using default"
            cleanup_delete_cluster
            cleanup_stop_daemon
            exit 130
            ;;
    esac
}
