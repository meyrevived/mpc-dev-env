#!/usr/bin/env bash

# utils.sh - Utility functions for the MPC dev-env workflow
#
# This script provides reusable functions used throughout the dev-env workflow:
#   - Logging functions with color-coded output (info, success, error, warning)
#   - User input prompts (text input, yes/no, multiple choice)
#   - Input validation and default value handling
#
# All functions output to stderr for logging (except read_choice which uses stdout
# for return values) to avoid polluting command substitution results.
#
# Usage:
#   source scripts/utils.sh
#   log_info "Starting operation..."
#   if prompt_yes_no "Continue?"; then
#       choice=$(read_choice "Select option [1-3]: " "1" "1 2 3")
#   fi

# Color codes for output
readonly COLOR_RESET='\033[0m'
readonly COLOR_RED='\033[0;31m'
readonly COLOR_GREEN='\033[0;32m'
readonly COLOR_YELLOW='\033[0;33m'
readonly COLOR_BLUE='\033[0;34m'

# Logging functions - Color-coded output for visibility
# INFO/SUCCESS/WARNING go to stdout for automation compatibility (stderr often indicates failure)
# ERROR goes to stderr as expected by automation tools
#
# Note: When stdout/stderr are mixed and piped, ordering may vary due to buffering differences.
# In interactive terminals (line-buffered stdout), ordering is typically correct.

# log_info: Blue [INFO] prefix for informational messages (stdout)
log_info() { echo -e "${COLOR_BLUE}[INFO]${COLOR_RESET} $*"; }

# log_success: Green [✓] prefix for success messages (stdout)
log_success() { echo -e "${COLOR_GREEN}[✓]${COLOR_RESET} $*"; }

# log_error: Red [ERROR] prefix for error messages (stderr)
log_error() { echo -e "${COLOR_RED}[ERROR]${COLOR_RESET} $*" >&2; }

# log_warning: Yellow [WARNING] prefix for warning messages (stdout)
log_warning() { echo -e "${COLOR_YELLOW}[WARNING]${COLOR_RESET} $*"; }

# prompt_user - Prompt user for text input with optional default value
#
# Arguments:
#   $1 - Prompt text to display
#   $2 - Variable name to store the result
#   $3 - Optional default value (if provided, shown in brackets)
#
# Example:
#   prompt_user "Enter path" MY_PATH "/default/path"
#   # Displays: "Enter path [/default/path]: "
#   # Result stored in $MY_PATH
prompt_user() {
    local prompt_text="$1"
    local var_name="$2"
    local default_value="${3:-}"

    if [ -n "$default_value" ]; then
        read -r -p "$prompt_text [$default_value]: " input
        eval "$var_name=\"${input:-$default_value}\""
    else
        read -r -p "$prompt_text: " input
        eval "$var_name=\"$input\""
    fi
}

# prompt_yes_no - Prompt user for yes/no confirmation
#
# Prompts the user repeatedly until they provide a valid y/n response.
#
# Arguments:
#   $1 - Prompt text to display
#
# Returns:
#   0 (success) if user answers yes
#   1 (failure) if user answers no
#
# Example:
#   if prompt_yes_no "Continue?"; then
#       echo "Continuing..."
#   fi
prompt_yes_no() {
    local prompt_text="$1"
    local response

    while true; do
        read -r -p "$prompt_text [y/n]: " response
        case "$response" in
            [Yy]*) return 0 ;;
            [Nn]*) return 1 ;;
            *) echo "Please answer y or n." ;;
        esac
    done
}

# read_choice - Read and validate user choice from a list of options
#
# Prompts the user and validates their choice against a list of valid options.
# IMPORTANT: Returns result via stdout, so use command substitution.
#
# Arguments:
#   $1 - Prompt text to display
#   $2 - Default value (used if user presses Enter)
#   $3 - Space-separated list of valid choices (e.g., "1 2 3 4")
#
# Returns:
#   Valid choice written to stdout
#
# Example:
#   choice=$(read_choice "Your choice [1-4]: " "1" "1 2 3 4")
#   echo "You selected: $choice"
#
# Note: This function will loop until a valid choice is provided or default is used
read_choice() {
    local prompt_text="$1"
    local default="${2:-1}"
    local valid_choices="${3:-}"  # Space-separated list of valid options
    local choice

    while true; do
        read -r -p "$prompt_text" choice

        # If choice is empty, use default
        if [ -z "$choice" ]; then
            choice="$default"
        fi

        # Validate choice if valid_choices was provided
        if [ -n "$valid_choices" ]; then
            if echo "$valid_choices" | grep -qw "$choice"; then
                # Valid choice
                echo "$choice"
                return 0
            else
                # Invalid choice - show error and loop to ask again
                log_error "Invalid choice. Please choose from: $valid_choices" >&2
                continue
            fi
        else
            # No validation needed
            echo "$choice"
            return 0
        fi
    done
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check if directory exists
dir_exists() {
    [ -d "$1" ]
}

# Check if file exists
file_exists() {
    [ -f "$1" ]
}

# List available TaskRun files in directory
list_taskrun_files() {
    local dir="$1"
    if [ ! -d "$dir" ] || [ -z "$(ls -A "$dir"/*.yaml 2>/dev/null)" ]; then
        return 1
    fi

    local count=1
    local files=()

    for file in "$dir"/*.yaml; do
        [ -f "$file" ] || continue
        echo "[$count] $(basename "$file")"
        files+=("$file")
        ((count++))
    done

    # Return array via global variable (bash limitation)
    TASKRUN_FILES=("${files[@]}")
    return 0
}

# Configuration file for persisting environment paths
# This file is created in the mpc_dev_env directory and stores validated paths
readonly CONFIG_FILE_NAME=".env.local"

# get_config_file_path - Get the path to the configuration file
#
# Returns the path to .env.local, preferring MPC_DEV_ENV_PATH if set,
# otherwise using the script's parent directory.
get_config_file_path() {
    if [ -n "${MPC_DEV_ENV_PATH:-}" ] && [ -d "$MPC_DEV_ENV_PATH" ]; then
        echo "${MPC_DEV_ENV_PATH}/${CONFIG_FILE_NAME}"
    else
        # Fall back to script's parent directory
        local script_dir
        script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        echo "$(dirname "$script_dir")/${CONFIG_FILE_NAME}"
    fi
}

# load_config - Load environment paths from configuration file
#
# Reads .env.local if it exists and sources the variables.
# Only loads variables that aren't already set in the environment.
#
# Side effects:
#   - Sets MPC_DEV_ENV_PATH and MPC_REPO_PATH if found in config and not already set
load_config() {
    local config_file
    config_file="$(get_config_file_path)"

    if [ -f "$config_file" ]; then
        log_info "Loading configuration from: $config_file"
        log_info "[DEBUG] Config file size: $(wc -c < "$config_file") bytes"
        # Source the file but only set variables that aren't already set
        while IFS='=' read -r key value || [ -n "$key" ]; do
            # Skip empty lines and comments
            [[ -z "$key" || "$key" =~ ^# ]] && continue
            # Remove quotes from value
            value="${value%\"}"
            value="${value#\"}"
            # Only set if not already in environment
            if [ -z "${!key:-}" ]; then
                export "$key"="$value"
                log_info "  Loaded $key from config"
                # DEBUG: Show loaded AWS profile
                if [ "$key" = "AWS_PROFILE" ]; then
                    log_info "  [DEBUG] AWS_PROFILE: $value"
                elif [ "$key" = "SSH_KEY_PATH" ]; then
                    log_info "  [DEBUG] SSH_KEY_PATH file exists: $([ -f "$value" ] && echo "yes" || echo "no")"
                fi
            fi
        done < "$config_file"
    else
        log_info "[DEBUG] Config file does not exist: $config_file"
    fi
}

# save_config - Save environment paths and AWS profile to configuration file
#
# Writes configuration to .env.local for persistence.
# Creates the file if it doesn't exist, overwrites if it does.
#
# Saved variables:
#   - MPC_DEV_ENV_PATH, MPC_REPO_PATH (always saved)
#   - AWS_PROFILE (if set)
#   - SSH_KEY_PATH (if set)
#
# Side effects:
#   - Creates or updates .env.local file
save_config() {
    local config_file
    config_file="$(get_config_file_path)"

    log_info "Saving configuration to: $config_file"

    # Start with base configuration
    cat > "$config_file" << EOF
# MPC Dev Environment Configuration
# Generated automatically - edit with caution
#
# These paths are used by dev-env.sh and test-e2e.sh

MPC_DEV_ENV_PATH="${MPC_DEV_ENV_PATH}"
MPC_REPO_PATH="${MPC_REPO_PATH}"
EOF

    # Append AWS profile if set
    if [ -n "${AWS_PROFILE:-}" ]; then
        cat >> "$config_file" << EOF

# AWS SSO Configuration
AWS_PROFILE="${AWS_PROFILE}"
EOF
    fi

    # Append SSH key path if set
    if [ -n "${SSH_KEY_PATH:-}" ]; then
        cat >> "$config_file" << EOF
SSH_KEY_PATH="${SSH_KEY_PATH}"
EOF
    fi

    log_success "Configuration saved"
}

# prompt_for_valid_directory - Prompt user for a valid directory path
#
# Displays an error about an invalid path and prompts the user to provide
# a valid directory path. Loops until a valid path is provided.
# Supports tilde expansion for home directory paths.
#
# Arguments:
#   $1 - Environment variable name (e.g., "MPC_DEV_ENV_PATH")
#   $2 - Description of what the path should point to
#   $3 - Current invalid path value
#
# Side effects:
#   Exports the corrected path to the environment variable specified in $1
#
# Example:
#   prompt_for_valid_directory "MPC_REPO_PATH" "the multi-platform-controller repository" "/old/path"
prompt_for_valid_directory() {
    local var_name="$1"
    local description="$2"
    local current_value="$3"

    log_warning "$var_name directory does not exist: $current_value"
    log_info "Please provide the correct path to $description."

    while true; do
        local new_path
        read -r -p "Enter $var_name: " new_path
        new_path="${new_path/#\~/$HOME}"  # Expand tilde

        if [ -z "$new_path" ]; then
            log_error "Path cannot be empty"
            continue
        fi

        if dir_exists "$new_path"; then
            export "$var_name"="$new_path"
            log_success "$var_name updated: $new_path"
            return 0
        else
            log_error "Directory does not exist: $new_path"
            log_info "Please try again."
        fi
    done
}

# validate_and_set_env_paths - Validate MPC_DEV_ENV_PATH and MPC_REPO_PATH
#
# Validates that both required environment paths exist. If either path is
# invalid or doesn't exist, prompts the user interactively for the correct path.
# Also handles auto-detection of MPC_REPO_PATH based on MPC_DEV_ENV_PATH.
#
# This function first loads any previously saved configuration from .env.local,
# then validates paths, and saves the configuration if any changes were made.
#
# Prerequisites:
#   - MPC_DEV_ENV_PATH should be set (will fail if not)
#
# Side effects:
#   - May update MPC_DEV_ENV_PATH and MPC_REPO_PATH environment variables
#   - Exports corrected paths
#   - Creates/updates .env.local configuration file
#
# Returns:
#   0 on success (both paths are valid)
#   Exits with 1 if MPC_DEV_ENV_PATH is not set at all
validate_and_set_env_paths() {
    # First, try to load saved configuration
    load_config

    # Track if we need to save config (paths were modified)
    local config_changed=false

    # Check if MPC_DEV_ENV_PATH is set
    if [ -z "${MPC_DEV_ENV_PATH:-}" ]; then
        log_error "MPC_DEV_ENV_PATH environment variable is not set"
        return 1
    fi

    # Validate MPC_DEV_ENV_PATH exists, prompt for correction if not
    if ! dir_exists "$MPC_DEV_ENV_PATH"; then
        prompt_for_valid_directory "MPC_DEV_ENV_PATH" "the mpc_dev_env repository" "$MPC_DEV_ENV_PATH"
        config_changed=true
    fi

    # Auto-detect or validate MPC_REPO_PATH
    if [ -z "${MPC_REPO_PATH:-}" ]; then
        log_info "MPC_REPO_PATH not set, attempting auto-detection..."
        local parent_dir
        parent_dir="$(dirname "$MPC_DEV_ENV_PATH")"
        local candidate_path="${parent_dir}/multi-platform-controller"

        if dir_exists "$candidate_path"; then
            export MPC_REPO_PATH="$candidate_path"
            log_success "Auto-detected MPC_REPO_PATH: $MPC_REPO_PATH"
            config_changed=true
        else
            log_warning "MPC_REPO_PATH not set and auto-detection failed"
            log_warning "Looked for multi-platform-controller at: $candidate_path"
            prompt_for_valid_directory "MPC_REPO_PATH" "the multi-platform-controller repository" "(not set)"
            config_changed=true
        fi
    fi

    # Validate MPC_REPO_PATH exists, prompt for correction if not
    if ! dir_exists "$MPC_REPO_PATH"; then
        prompt_for_valid_directory "MPC_REPO_PATH" "the multi-platform-controller repository" "$MPC_REPO_PATH"
        config_changed=true
    fi

    # Save configuration if any paths were modified
    if [ "$config_changed" = true ]; then
        save_config
    fi

    return 0
}

# check_aws_cli_installed - Check if AWS CLI is available
#
# Returns:
#   0 if aws command is available
#   1 if not available (shows error message)
check_aws_cli_installed() {
    if ! command -v aws &> /dev/null; then
        echo ""
        log_error "AWS CLI is required to validate credentials"
        echo ""
        log_error "AWS CLI not found. Please install it:"
        log_error "  - macOS: brew install awscli"
        log_error "  - Linux: pip3 install --user awscli"
        log_error "  - Official: https://aws.amazon.com/cli/"
        echo ""
        log_error "After installation, re-run this TaskRun."
        echo ""
        return 1
    fi
    return 0
}

# validate_sso_session - Validate AWS SSO session for a given profile
#
# Attempts to validate the SSO session by calling aws sts get-caller-identity
# with the specified profile. No retries - if it fails, enters the recovery loop.
#
# Arguments:
#   $1 - AWS profile name
#
# Returns:
#   0 if session is valid
#   1 if session is invalid/expired
validate_sso_session() {
    local profile="$1"

    if [ -z "$profile" ]; then
        log_error "No AWS profile specified"
        return 1
    fi

    # Check if AWS CLI is installed
    if ! check_aws_cli_installed; then
        return 1
    fi

    log_info "Validating AWS SSO session for profile: $profile"

    local aws_output
    aws_output=$(aws sts get-caller-identity --profile "$profile" 2>&1)
    local exit_code=$?

    if [ $exit_code -eq 0 ]; then
        log_success "SSO session valid for profile: $profile"
        local account_id
        account_id=$(echo "$aws_output" | jq -r '.Account' 2>/dev/null)
        local arn
        arn=$(echo "$aws_output" | jq -r '.Arn' 2>/dev/null)
        log_info "  Account: ${account_id:-unknown}"
        log_info "  Identity: ${arn:-unknown}"
        return 0
    fi

    log_warning "SSO session invalid or expired for profile: $profile"
    if [ -n "$aws_output" ]; then
        log_info "AWS Error: $aws_output"
    fi
    return 1
}

# extract_sso_credentials - Extract temporary credentials from SSO session
#
# Uses `aws configure export-credentials` to resolve the current SSO session
# into temporary access key, secret key, and session token.
#
# Arguments:
#   $1 - AWS profile name
#
# Sets environment variables:
#   AWS_ACCESS_KEY_ID - Temporary access key
#   AWS_SECRET_ACCESS_KEY - Temporary secret key
#   AWS_SESSION_TOKEN - Session token
#
# Returns:
#   0 if credentials extracted successfully
#   1 if extraction failed
extract_sso_credentials() {
    local profile="$1"

    log_info "Extracting temporary credentials from SSO session..."

    local cred_output
    cred_output=$(aws configure export-credentials --profile "$profile" --format env-no-export 2>&1)
    local exit_code=$?

    if [ $exit_code -ne 0 ]; then
        log_error "Failed to extract credentials from SSO session"
        log_error "Error: $cred_output"
        return 1
    fi

    # Parse the output - format is KEY=VALUE lines
    local access_key secret_key session_token
    access_key=$(echo "$cred_output" | grep '^AWS_ACCESS_KEY_ID=' | cut -d'=' -f2-)
    secret_key=$(echo "$cred_output" | grep '^AWS_SECRET_ACCESS_KEY=' | cut -d'=' -f2-)
    session_token=$(echo "$cred_output" | grep '^AWS_SESSION_TOKEN=' | cut -d'=' -f2-)

    if [ -z "$access_key" ] || [ -z "$secret_key" ]; then
        log_error "Failed to parse credentials from SSO session output"
        return 1
    fi

    export AWS_ACCESS_KEY_ID="$access_key"
    export AWS_SECRET_ACCESS_KEY="$secret_key"
    export AWS_SESSION_TOKEN="$session_token"

    log_success "Temporary credentials extracted successfully"
    return 0
}

# sso_recovery_loop - Interactive recovery when SSO session is expired
#
# Shows a clear message about the expired session and offers the user
# two options: retry after re-authenticating, or quit.
# No retry limit - user controls when to give up.
#
# Arguments:
#   $1 - AWS profile name
#
# Returns:
#   0 if user re-authenticated successfully
#   1 if user chose to quit
sso_recovery_loop() {
    local profile="$1"

    while true; do
        echo ""
        log_error "========================================="
        log_error "AWS SSO session for profile '$profile' is"
        log_error "expired or invalid."
        log_error ""
        log_error "To fix, run this in another terminal:"
        log_error "  aws login"
        log_error ""
        log_error "========================================="
        echo ""

        echo "[r] I've re-authenticated, try again"
        echo "[q] Quit"
        echo ""

        local choice
        choice=$(read_choice "Your choice: " "r" "r q")

        case "$choice" in
            r)
                if validate_sso_session "$profile"; then
                    return 0
                fi
                log_warning "Session still invalid, try again..."
                ;;
            q)
                log_info "Exiting..."
                return 1
                ;;
        esac
    done
}
