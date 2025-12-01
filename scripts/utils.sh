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

# Logging functions - Color-coded output to stderr for visibility
# log_info: Blue [INFO] prefix for informational messages
log_info() { echo -e "${COLOR_BLUE}[INFO]${COLOR_RESET} $*"; }

# log_success: Green [✓] prefix for success messages
log_success() { echo -e "${COLOR_GREEN}[✓]${COLOR_RESET} $*"; }

# log_error: Red [ERROR] prefix for error messages (explicitly to stderr)
log_error() { echo -e "${COLOR_RED}[ERROR]${COLOR_RESET} $*" >&2; }

# log_warning: Yellow [WARNING] prefix for warning messages
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
