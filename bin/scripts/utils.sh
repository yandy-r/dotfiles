#!/usr/bin/env bash
#
# utils.sh - Professional bash utility library for Docker management scripts
#
# This library provides reusable functions for logging, error handling,
# prerequisite validation, and common operations used across Docker
# management scripts.
#
# Usage:
#   source "$(dirname "${BASH_SOURCE[0]}")/lib/utils.sh"
#
# Author: RFamily Apps Project
# License: MIT

# Prevent multiple sourcing
if [[ -n "${UTILS_SH_LOADED:-}" ]]; then
    return 0
fi
readonly UTILS_SH_LOADED=1

#==============================================================================
# CONSTANTS
#==============================================================================

# Exit codes
readonly EXIT_SUCCESS=0
readonly EXIT_GENERAL_ERROR=1
readonly EXIT_MISSING_DEPENDENCY=2
readonly EXIT_INVALID_ARGUMENT=3
readonly EXIT_FILE_NOT_FOUND=4
readonly EXIT_PERMISSION_DENIED=5
readonly EXIT_INTERRUPTED=130

# Color codes for terminal output
readonly COLOR_RESET='\033[0m'
readonly COLOR_RED='\033[0;31m'
readonly COLOR_GREEN='\033[0;32m'
readonly COLOR_YELLOW='\033[0;33m'
readonly COLOR_BLUE='\033[0;34m'
readonly COLOR_MAGENTA='\033[0;35m'
readonly COLOR_CYAN='\033[0;36m'
readonly COLOR_WHITE='\033[0;37m'
readonly COLOR_BOLD='\033[1m'
readonly COLOR_DIM='\033[2m'

# Log level constants
readonly LOG_LEVEL_DEBUG=0
readonly LOG_LEVEL_INFO=1
readonly LOG_LEVEL_WARN=2
readonly LOG_LEVEL_ERROR=3
readonly LOG_LEVEL_SUCCESS=4

# Default log level (can be overridden by setting LOG_LEVEL environment variable)
: "${LOG_LEVEL:=${LOG_LEVEL_INFO}}"

# Script metadata
SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
SCRIPT_START_TIME="$(date +%s)"
readonly SCRIPT_START_TIME

#==============================================================================
# GLOBAL VARIABLES
#==============================================================================

# Log file path (will be set by init_logging)
LOG_FILE=""

# Flag to track if cleanup has been performed
CLEANUP_PERFORMED=0

#==============================================================================
# LOGGING FUNCTIONS
#==============================================================================

#------------------------------------------------------------------------------
# Initialize logging system
#
# Sets up log file path and creates log directory if needed.
# Should be called early in script execution.
#
# Arguments:
#   $1 - Optional: Custom log file path
#
# Returns:
#   0 on success, non-zero on failure
#------------------------------------------------------------------------------
# shellcheck disable=SC2120 # optional custom_log_file argument
init_logging() {
    local custom_log_file="${1:-}"
    
    # Determine log file path
    if [[ -n "${custom_log_file}" ]]; then
        LOG_FILE="${custom_log_file}"
    elif [[ -n "${DOCKER_DIR:-}" ]]; then
        LOG_FILE="${DOCKER_DIR}/logs/script.log"
    else
        # Fallback to temp directory if DOCKER_DIR not set
        LOG_FILE="/tmp/docker-scripts-$(date +%Y%m%d).log"
    fi
    
    # Create log directory if it doesn't exist
    local log_dir
    log_dir="$(dirname "${LOG_FILE}")"
    
    if [[ ! -d "${log_dir}" ]]; then
        if ! mkdir -p "${log_dir}" 2>/dev/null; then
            echo "WARNING: Failed to create log directory: ${log_dir}" >&2
            echo "Logging to console only." >&2
            LOG_FILE=""
            return 1
        fi
    fi
    
    # Test write permissions
    if [[ -n "${LOG_FILE}" ]] && ! touch "${LOG_FILE}" 2>/dev/null; then
        echo "WARNING: Cannot write to log file: ${LOG_FILE}" >&2
        echo "Logging to console only." >&2
        LOG_FILE=""
        return 1
    fi
    
    return 0
}

#------------------------------------------------------------------------------
# Get timestamp for log entries
#
# Returns:
#   ISO 8601 formatted timestamp
#------------------------------------------------------------------------------
get_timestamp() {
    date '+%Y-%m-%d %H:%M:%S'
}

#------------------------------------------------------------------------------
# Write log entry to file and console
#
# Arguments:
#   $1 - Log level (INFO, WARN, ERROR, SUCCESS, DEBUG)
#   $2 - Message
#   $3 - Color code for console output
#   $4 - Minimum log level required to display (optional)
#
# Returns:
#   0 always
#------------------------------------------------------------------------------
log_message() {
    local level="${1}"
    local message="${2}"
    local color="${3}"
    local min_level="${4:-${LOG_LEVEL_INFO}}"
    
    local timestamp
    timestamp="$(get_timestamp)"
    
    local log_entry="[${timestamp}] [${level}] ${message}"
    
    # Write to log file if available
    if [[ -n "${LOG_FILE}" ]]; then
        echo "${log_entry}" >> "${LOG_FILE}" 2>/dev/null || true
    fi
    
    # Write to console with color if log level is sufficient
    local current_level="${LOG_LEVEL:-${LOG_LEVEL_INFO}}"
    if [[ "${current_level}" -le "${min_level}" ]]; then
        if [[ -t 1 ]]; then
            # Terminal supports colors
            echo -e "${color}${level}:${COLOR_RESET} ${message}"
        else
            # No color support
            echo "${level}: ${message}"
        fi
    fi
    
    return 0
}

#------------------------------------------------------------------------------
# Log informational message
#
# Arguments:
#   $1 - Message
#------------------------------------------------------------------------------
log_info() {
    log_message "INFO" "${1}" "${COLOR_BLUE}" "${LOG_LEVEL_INFO}"
}

#------------------------------------------------------------------------------
# Log warning message
#
# Arguments:
#   $1 - Message
#------------------------------------------------------------------------------
log_warn() {
    log_message "WARN" "${1}" "${COLOR_YELLOW}" "${LOG_LEVEL_WARN}"
}

#------------------------------------------------------------------------------
# Log error message
#
# Arguments:
#   $1 - Message
#------------------------------------------------------------------------------
log_error() {
    log_message "ERROR" "${1}" "${COLOR_RED}" "${LOG_LEVEL_ERROR}" >&2
}

#------------------------------------------------------------------------------
# Log success message
#
# Arguments:
#   $1 - Message
#------------------------------------------------------------------------------
log_success() {
    log_message "SUCCESS" "${1}" "${COLOR_GREEN}" "${LOG_LEVEL_SUCCESS}"
}

#------------------------------------------------------------------------------
# Log debug message
#
# Arguments:
#   $1 - Message
#------------------------------------------------------------------------------
log_debug() {
    log_message "DEBUG" "${1}" "${COLOR_DIM}" "${LOG_LEVEL_DEBUG}"
}

#------------------------------------------------------------------------------
# Display formatted section header
#
# Arguments:
#   $1 - Section title
#   $2 - Optional: Character to use for border (default: =)
#------------------------------------------------------------------------------
print_section() {
    local title="${1}"
    local border_char="${2:-=}"
    local border_length=80
    
    local border
    border="$(printf '%*s' "${border_length}" '' | tr ' ' "${border_char}")"
    
    if [[ -t 1 ]]; then
        echo -e "\n${COLOR_BOLD}${COLOR_CYAN}${border}${COLOR_RESET}"
        echo -e "${COLOR_BOLD}${COLOR_CYAN}${title}${COLOR_RESET}"
        echo -e "${COLOR_BOLD}${COLOR_CYAN}${border}${COLOR_RESET}\n"
    else
        echo ""
        echo "${border}"
        echo "${title}"
        echo "${border}"
        echo ""
    fi
    
    if [[ -n "${LOG_FILE}" ]]; then
        {
            echo ""
            echo "${border}"
            echo "${title}"
            echo "${border}"
            echo ""
        } >> "${LOG_FILE}" 2>/dev/null || true
    fi
}

#==============================================================================
# ERROR HANDLING FUNCTIONS
#==============================================================================

#------------------------------------------------------------------------------
# Display error message and exit with specified code
#
# Arguments:
#   $1 - Error message
#   $2 - Exit code (default: EXIT_GENERAL_ERROR)
#------------------------------------------------------------------------------
die() {
    local message="${1}"
    local exit_code="${2:-${EXIT_GENERAL_ERROR}}"
    
    log_error "${message}"
    log_error "Script failed. Exit code: ${exit_code}"
    
    exit "${exit_code}"
}

#------------------------------------------------------------------------------
# Handle script interruption (SIGINT/SIGTERM)
#
# Performs cleanup and exits gracefully
#------------------------------------------------------------------------------
handle_interrupt() {
    echo "" # New line after ^C
    log_warn "Script interrupted by user"
    
    # Perform cleanup if not already done
    if [[ "${CLEANUP_PERFORMED}" -eq 0 ]]; then
        cleanup_on_exit
    fi
    
    exit "${EXIT_INTERRUPTED}"
}

#------------------------------------------------------------------------------
# Cleanup function called on script exit
#
# Override this function in your script to perform custom cleanup
#------------------------------------------------------------------------------
cleanup_on_exit() {
    if [[ "${CLEANUP_PERFORMED}" -eq 1 ]]; then
        return 0
    fi
    
    CLEANUP_PERFORMED=1
    
    log_debug "Performing cleanup..."
    
    # Add custom cleanup logic here or override in calling script
    
    local end_time
    end_time="$(date +%s)"
    local duration=$((end_time - SCRIPT_START_TIME))
    
    log_debug "Script execution time: ${duration} seconds"
}

#------------------------------------------------------------------------------
# Setup signal handlers and exit trap
#
# Should be called early in script execution
#------------------------------------------------------------------------------
setup_error_handling() {
    # Trap signals
    trap handle_interrupt SIGINT SIGTERM
    
    # Trap exit
    trap cleanup_on_exit EXIT
    
    log_debug "Error handling initialized"
}

#==============================================================================
# PREREQUISITE VALIDATION FUNCTIONS
#==============================================================================

#------------------------------------------------------------------------------
# Check if a command exists in PATH
#
# Arguments:
#   $1 - Command name
#
# Returns:
#   0 if command exists, 1 otherwise
#------------------------------------------------------------------------------
command_exists() {
    command -v "${1}" >/dev/null 2>&1
}

#------------------------------------------------------------------------------
# Validate that required commands are available
#
# Arguments:
#   $@ - List of required commands
#
# Returns:
#   0 if all commands exist, exits with EXIT_MISSING_DEPENDENCY otherwise
#------------------------------------------------------------------------------
require_commands() {
    local missing_commands=()
    
    for cmd in "${@}"; do
        if ! command_exists "${cmd}"; then
            missing_commands+=("${cmd}")
        fi
    done
    
    if [[ "${#missing_commands[@]}" -gt 0 ]]; then
        log_error "Missing required commands: ${missing_commands[*]}"
        log_error "Please install the missing dependencies and try again."
        exit "${EXIT_MISSING_DEPENDENCY}"
    fi
    
    log_debug "All required commands are available: ${*}"
    return 0
}

#------------------------------------------------------------------------------
# Check for Docker and Docker Compose
#
# Returns:
#   0 if Docker is available, exits with EXIT_MISSING_DEPENDENCY otherwise
#------------------------------------------------------------------------------
check_docker() {
    log_debug "Checking for Docker..."
    
    if ! command_exists docker; then
        die "Docker is not installed or not in PATH" "${EXIT_MISSING_DEPENDENCY}"
    fi
    
    # Check if Docker daemon is running
    if ! docker info >/dev/null 2>&1; then
        die "Docker daemon is not running" "${EXIT_GENERAL_ERROR}"
    fi
    
    # Check for docker compose (plugin or standalone)
    if docker compose version >/dev/null 2>&1; then
        log_debug "Docker Compose plugin detected"
    elif command_exists docker-compose; then
        log_debug "Docker Compose standalone detected"
    else
        die "Docker Compose is not installed" "${EXIT_MISSING_DEPENDENCY}"
    fi
    
    log_success "Docker and Docker Compose are available"
    return 0
}

#------------------------------------------------------------------------------
# Check for 1Password CLI (op)
#
# Arguments:
#   $1 - Optional: "required" to make it mandatory, "optional" to just warn
#
# Returns:
#   0 if op is available or optional, 1 if required but missing
#------------------------------------------------------------------------------
check_1password() {
    local requirement="${1:-optional}"
    
    log_debug "Checking for 1Password CLI..."
    
    if ! command_exists op; then
        if [[ "${requirement}" == "required" ]]; then
            die "1Password CLI (op) is not installed or not in PATH" "${EXIT_MISSING_DEPENDENCY}"
        else
            log_warn "1Password CLI (op) is not installed. Encrypted variables will not be decrypted."
            return 1
        fi
    fi
    
    # Check if op is authenticated
    if ! op account list >/dev/null 2>&1; then
        if [[ "${requirement}" == "required" ]]; then
            die "1Password CLI is not authenticated. Run 'op signin' first." "${EXIT_GENERAL_ERROR}"
        else
            log_warn "1Password CLI is not authenticated. Run 'op signin' to use encrypted variables."
            return 1
        fi
    fi
    
    log_success "1Password CLI is available and authenticated"
    return 0
}

#------------------------------------------------------------------------------
# Check for dotenvx CLI
#
# Arguments:
#   $1 - Optional: "required" to make it mandatory, "optional" to just warn
#
# Returns:
#   0 if dotenvx is available or optional, 1 if required but missing
#------------------------------------------------------------------------------
check_dotenvx() {
    local requirement="${1:-optional}"
    
    log_debug "Checking for dotenvx..."
    
    if ! command_exists dotenvx; then
        if [[ "${requirement}" == "required" ]]; then
            die "dotenvx is not installed or not in PATH" "${EXIT_MISSING_DEPENDENCY}"
        else
            log_warn "dotenvx is not installed. Encrypted .env files cannot be decrypted."
            return 1
        fi
    fi
    
    log_success "dotenvx is available"
    return 0
}

#==============================================================================
# FILE VALIDATION FUNCTIONS
#==============================================================================

#------------------------------------------------------------------------------
# Check if a file exists and is readable
#
# Arguments:
#   $1 - File path
#
# Returns:
#   0 if file exists and is readable, 1 otherwise
#------------------------------------------------------------------------------
file_exists_readable() {
    local file="${1}"
    
    if [[ ! -f "${file}" ]]; then
        log_debug "File does not exist: ${file}"
        return 1
    fi
    
    if [[ ! -r "${file}" ]]; then
        log_debug "File is not readable: ${file}"
        return 1
    fi
    
    return 0
}

#------------------------------------------------------------------------------
# Validate that environment file exists
#
# Arguments:
#   $1 - Environment file path (default: .env)
#
# Returns:
#   0 if file exists, exits with EXIT_FILE_NOT_FOUND otherwise
#------------------------------------------------------------------------------
validate_env_file() {
    local env_file="${1:-.env}"
    
    log_debug "Validating environment file: ${env_file}"
    
    if [[ ! -f "${env_file}" ]]; then
        die "Environment file not found: ${env_file}" "${EXIT_FILE_NOT_FOUND}"
    fi
    
    if [[ ! -r "${env_file}" ]]; then
        die "Environment file is not readable: ${env_file}" "${EXIT_PERMISSION_DENIED}"
    fi
    
    log_success "Environment file validated: ${env_file}"
    return 0
}

#------------------------------------------------------------------------------
# Validate multiple environment files exist
#
# Arguments:
#   $@ - List of environment file paths
#
# Returns:
#   0 if all files exist, exits with EXIT_FILE_NOT_FOUND otherwise
#------------------------------------------------------------------------------
validate_env_files() {
    local missing_files=()
    
    for env_file in "${@}"; do
        if [[ ! -f "${env_file}" ]]; then
            missing_files+=("${env_file}")
        elif [[ ! -r "${env_file}" ]]; then
            die "Environment file is not readable: ${env_file}" "${EXIT_PERMISSION_DENIED}"
        fi
    done
    
    if [[ "${#missing_files[@]}" -gt 0 ]]; then
        log_error "Missing environment files:"
        for file in "${missing_files[@]}"; do
            log_error "  - ${file}"
        done
        exit "${EXIT_FILE_NOT_FOUND}"
    fi
    
    log_success "All environment files validated"
    return 0
}

#------------------------------------------------------------------------------
# Check if docker-compose.yml exists
#
# Arguments:
#   $1 - Optional: Path to docker-compose.yml (default: ./docker-compose.yml)
#
# Returns:
#   0 if file exists, exits with EXIT_FILE_NOT_FOUND otherwise
#------------------------------------------------------------------------------
validate_compose_file() {
    local compose_file="${1:-docker-compose.yml}"
    
    log_debug "Validating Docker Compose file: ${compose_file}"
    
    if [[ ! -f "${compose_file}" ]]; then
        die "Docker Compose file not found: ${compose_file}" "${EXIT_FILE_NOT_FOUND}"
    fi
    
    if [[ ! -r "${compose_file}" ]]; then
        die "Docker Compose file is not readable: ${compose_file}" "${EXIT_PERMISSION_DENIED}"
    fi
    
    # Validate compose file syntax
    if ! docker compose -f "${compose_file}" config >/dev/null 2>&1; then
        die "Docker Compose file has invalid syntax: ${compose_file}" "${EXIT_GENERAL_ERROR}"
    fi
    
    log_success "Docker Compose file validated: ${compose_file}"
    return 0
}

#==============================================================================
# UTILITY FUNCTIONS
#==============================================================================

#------------------------------------------------------------------------------
# Prompt user for confirmation
#
# Arguments:
#   $1 - Prompt message
#   $2 - Optional: Default answer (y/n, default: n)
#
# Returns:
#   0 if user confirms, 1 otherwise
#------------------------------------------------------------------------------
confirm() {
    local prompt="${1}"
    local default="${2:-n}"
    local response
    
    if [[ "${default}" == "y" ]]; then
        prompt="${prompt} [Y/n]: "
    else
        prompt="${prompt} [y/N]: "
    fi
    
    read -r -p "${prompt}" response
    
    # Use default if no response
    if [[ -z "${response}" ]]; then
        response="${default}"
    fi
    
    case "${response}" in
        [yY][eE][sS]|[yY])
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

#------------------------------------------------------------------------------
# Check if script is running as root
#
# Returns:
#   0 if running as root, 1 otherwise
#------------------------------------------------------------------------------
is_root() {
    [[ "${EUID}" -eq 0 ]]
}

#------------------------------------------------------------------------------
# Require script to run as root
#
# Exits with EXIT_PERMISSION_DENIED if not root
#------------------------------------------------------------------------------
require_root() {
    if ! is_root; then
        die "This script must be run as root" "${EXIT_PERMISSION_DENIED}"
    fi
}

#------------------------------------------------------------------------------
# Require script to NOT run as root
#
# Exits with EXIT_PERMISSION_DENIED if running as root
#------------------------------------------------------------------------------
require_not_root() {
    if is_root; then
        die "This script should not be run as root" "${EXIT_PERMISSION_DENIED}"
    fi
}

#------------------------------------------------------------------------------
# Get the absolute path of a file or directory
#
# Arguments:
#   $1 - Path to resolve
#
# Returns:
#   Absolute path
#------------------------------------------------------------------------------
get_abs_path() {
    local path="${1}"
    
    if [[ -d "${path}" ]]; then
        (cd "${path}" && pwd)
    elif [[ -f "${path}" ]]; then
        local dir
        local file
        dir="$(dirname "${path}")"
        file="$(basename "${path}")"
        echo "$(cd "${dir}" && pwd)/${file}"
    else
        echo "${path}"
    fi
}

#------------------------------------------------------------------------------
# Display script usage information
#
# This is a template function that should be overridden in calling scripts
#------------------------------------------------------------------------------
show_usage() {
    cat << EOF
Usage: ${SCRIPT_NAME} [OPTIONS]

This is a template usage function. Override this in your script.

OPTIONS:
    -h, --help      Show this help message
    -v, --verbose   Enable verbose output
    -q, --quiet     Suppress non-error output

EXAMPLES:
    ${SCRIPT_NAME}
    ${SCRIPT_NAME} --verbose

EOF
}

#==============================================================================
# INITIALIZATION
#==============================================================================

# Initialize logging on library load
init_logging

# Export functions for use in other scripts
export -f log_info log_warn log_error log_success log_debug
export -f print_section die
export -f command_exists require_commands
export -f check_docker check_1password check_dotenvx
export -f validate_env_file validate_env_files validate_compose_file
export -f confirm is_root require_root require_not_root
export -f get_abs_path

log_debug "Utils library loaded successfully"
