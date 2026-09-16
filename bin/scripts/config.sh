#!/usr/bin/env bash

################################################################################
# Docker Application Management Configuration
################################################################################
#
# This configuration file controls the behavior of start-apps.sh and stop-apps.sh
# scripts. Modify these settings to customize how your Docker services are managed.
#
# Usage:
#   Source this file in your scripts: source "$(dirname "$0")/scripts/config.sh"
#
################################################################################

################################################################################
# LOGGING CONFIGURATION
################################################################################

# LOG_TO_FILE
# Description: Enable or disable logging to file
# Values: true | false
# Default: false
# When enabled, logs will be written to ${LOG_DIR}/docker-apps-$(date +%Y%m%d).log
LOG_TO_FILE="${LOG_TO_FILE:-false}"

# LOG_LEVEL
# Description: Minimum log level to display/record
# Values: DEBUG | INFO | WARN | ERROR
# Default: INFO
# - DEBUG: Show all messages including detailed debugging information
# - INFO: Show informational messages, warnings, and errors
# - WARN: Show only warnings and errors
# - ERROR: Show only error messages
LOG_LEVEL="${LOG_LEVEL:-INFO}"

# LOG_DIR
# Description: Directory where log files will be stored (if LOG_TO_FILE=true)
# Default: ./logs
# Note: Directory will be created automatically if it doesn't exist
LOG_DIR="${LOG_DIR:-./logs}"

# LOG_RETENTION_DAYS
# Description: Number of days to keep log files before automatic cleanup
# Default: 30
# Set to 0 to disable automatic log cleanup
LOG_RETENTION_DAYS="${LOG_RETENTION_DAYS:-30}"

################################################################################
# DOCKER CLEANUP CONFIGURATION
################################################################################

# PRUNE_IMAGES
# Description: Remove unused Docker images after operations
# Values: true | false
# Default: true
# This helps free up disk space by removing dangling images
# Warning: Does not remove images used by stopped containers
PRUNE_IMAGES="${PRUNE_IMAGES:-true}"

# PRUNE_NETWORKS
# Description: Remove unused Docker networks after stopping services
# Values: true | false
# Default: true
# Only removes networks not referenced by any containers
PRUNE_NETWORKS="${PRUNE_NETWORKS:-true}"

# PRUNE_VOLUMES
# Description: Remove unused Docker volumes after stopping services
# Values: true | false
# Default: false
# WARNING: This can result in data loss! Only enable if you're sure
# volumes are properly backed up or are truly disposable
PRUNE_VOLUMES="${PRUNE_VOLUMES:-false}"

# PRUNE_ALL_IMAGES
# Description: Remove ALL unused images, not just dangling ones
# Values: true | false
# Default: false
# WARNING: This will remove all images not used by running containers,
# which may significantly increase startup time on next run
PRUNE_ALL_IMAGES="${PRUNE_ALL_IMAGES:-false}"

################################################################################
# HEALTH CHECK CONFIGURATION
################################################################################

# HEALTH_CHECK_TIMEOUT
# Description: Maximum time (in seconds) to wait for service health checks
# Default: 300 (5 minutes)
# Set to 0 to disable health check waiting
# Services will be considered healthy after this timeout expires
HEALTH_CHECK_TIMEOUT="${HEALTH_CHECK_TIMEOUT:-300}"

# HEALTH_CHECK_INTERVAL
# Description: Time (in seconds) between health check polls
# Default: 5
# How often to check if services have become healthy
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-5}"

# HEALTH_CHECK_SERVICES
# Description: Space-separated list of critical services to health check
# Default: empty (check all services)
# Example: "portainer npm authentik-server"
# Leave empty to check all services defined in docker-compose.yml
HEALTH_CHECK_SERVICES="${HEALTH_CHECK_SERVICES:-}"

################################################################################
# BACKUP CONFIGURATION
################################################################################

# BACKUP_ON_STOP
# Description: Create backups of Docker volumes before stopping services
# Values: true | false
# Default: false
# Backups are stored in ${BACKUP_DIR}/volumes-backup-$(date +%Y%m%d-%H%M%S)
BACKUP_ON_STOP="${BACKUP_ON_STOP:-false}"

# BACKUP_DIR
# Description: Directory where volume backups will be stored
# Default: ./backups
# Note: Directory will be created automatically if it doesn't exist
BACKUP_DIR="${BACKUP_DIR:-./backups}"

# BACKUP_RETENTION_DAYS
# Description: Number of days to keep backup files before automatic cleanup
# Default: 7
# Set to 0 to disable automatic backup cleanup
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"

# BACKUP_COMPRESSION
# Description: Compress backups using gzip
# Values: true | false
# Default: true
# Compressed backups save disk space but take longer to create
BACKUP_COMPRESSION="${BACKUP_COMPRESSION:-true}"

################################################################################
# STARTUP CONFIGURATION
################################################################################

# PULL_BEFORE_START
# Description: Pull latest Docker images before starting services
# Values: true | false
# Default: true
# Ensures you're running the latest versions of all containers
# Set to false to speed up startup if you don't need latest images
PULL_BEFORE_START="${PULL_BEFORE_START:-true}"

# PULL_TIMEOUT
# Description: Maximum time (in seconds) to wait for image pulls
# Default: 600 (10 minutes)
# Set to 0 for no timeout (not recommended)
PULL_TIMEOUT="${PULL_TIMEOUT:-600}"

# START_TIMEOUT
# Description: Maximum time (in seconds) to wait for services to start
# Default: 300 (5 minutes)
# Set to 0 for no timeout
START_TIMEOUT="${START_TIMEOUT:-300}"

# RECREATE_CONTAINERS
# Description: Recreate containers even if configuration hasn't changed
# Values: always | never | changed
# Default: changed
# - always: Always recreate containers on start
# - never: Never recreate containers unless necessary
# - changed: Only recreate if configuration or image has changed
RECREATE_CONTAINERS="${RECREATE_CONTAINERS:-changed}"

################################################################################
# ENVIRONMENT CONFIGURATION
################################################################################

# ENV_FILE
# Description: Path to the environment file containing secrets and configuration
# Default: .env.local
# This file should contain all environment variables needed by docker-compose.yml
ENV_FILE="${ENV_FILE:-.env.local}"

# ENV_TOOL
# Description: Tool to use for loading environment variables
# Values: dotenvx | op | none
# Default: dotenvx
# - dotenvx: Use @dotenvx/dotenvx for environment variable management
# - op: Use 1Password CLI (op run) for secret management
# - none: Use standard docker compose without env wrapper
ENV_TOOL="${ENV_TOOL:-dotenvx}"

# ENV_VALIDATION
# Description: Validate required environment variables before operations
# Values: true | false
# Default: true
# Checks that critical environment variables are set before proceeding
ENV_VALIDATION="${ENV_VALIDATION:-true}"

# REQUIRED_ENV_VARS
# Description: Space-separated list of required environment variables
# Default: "DOCKER_DIR DOCKER_DATA_DIR TZ PUID PGID"
# These variables will be checked if ENV_VALIDATION=true
REQUIRED_ENV_VARS="${REQUIRED_ENV_VARS:-DOCKER_DIR DOCKER_DATA_DIR TZ PUID PGID}"

################################################################################
# NOTIFICATION CONFIGURATION
################################################################################

# ENABLE_NOTIFICATIONS
# Description: Send notifications on script completion or errors
# Values: true | false
# Default: false
# Requires notification service configuration (e.g., Notifiarr, Apprise)
ENABLE_NOTIFICATIONS="${ENABLE_NOTIFICATIONS:-false}"

# NOTIFICATION_URL
# Description: Webhook URL for sending notifications
# Default: empty
# Example: "https://notifiarr.com/api/v1/notification/webhook"
NOTIFICATION_URL="${NOTIFICATION_URL:-}"

# NOTIFY_ON_SUCCESS
# Description: Send notification on successful operations
# Values: true | false
# Default: false
NOTIFY_ON_SUCCESS="${NOTIFY_ON_SUCCESS:-false}"

# NOTIFY_ON_ERROR
# Description: Send notification on errors
# Values: true | false
# Default: true
NOTIFY_ON_ERROR="${NOTIFY_ON_ERROR:-true}"

################################################################################
# ADVANCED CONFIGURATION
################################################################################

# DRY_RUN
# Description: Show what would be done without actually doing it
# Values: true | false
# Default: false
# Useful for testing configuration changes
DRY_RUN="${DRY_RUN:-false}"

# VERBOSE
# Description: Enable verbose output for debugging
# Values: true | false
# Default: false
# Shows detailed Docker command output
VERBOSE="${VERBOSE:-false}"

# PARALLEL_OPERATIONS
# Description: Enable parallel Docker operations where possible
# Values: true | false
# Default: true
# Can speed up operations but may make logs harder to read
PARALLEL_OPERATIONS="${PARALLEL_OPERATIONS:-true}"

# COMPOSE_FILE
# Description: Path to docker-compose.yml file
# Default: ./docker-compose.yml
# Override if using a different compose file location
COMPOSE_FILE="${COMPOSE_FILE:-./docker-compose.yml}"

# COMPOSE_PROJECT_NAME
# Description: Docker Compose project name
# Default: empty (uses directory name)
# Set to override the default project name
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-}"

# STOP_TIMEOUT
# Description: Time (in seconds) to wait for containers to stop gracefully
# Default: 10
# After this timeout, containers will be forcefully killed
STOP_TIMEOUT="${STOP_TIMEOUT:-10}"

# REMOVE_ORPHANS
# Description: Remove containers for services not defined in compose file
# Values: true | false
# Default: true
# Helps keep your Docker environment clean
REMOVE_ORPHANS="${REMOVE_ORPHANS:-true}"

################################################################################
# COLOR OUTPUT CONFIGURATION
################################################################################

# ENABLE_COLORS
# Description: Enable colored output in terminal
# Values: true | false
# Default: true (auto-disabled if not a TTY)
# Makes output more readable with color-coded messages
ENABLE_COLORS="${ENABLE_COLORS:-true}"

# Color definitions (only used if ENABLE_COLORS=true and not already set by utils.sh)
if [[ -z "${COLOR_RESET:-}" ]]; then
    if [[ "${ENABLE_COLORS}" == "true" ]] && [[ -t 1 ]]; then
        COLOR_RESET='\033[0m'
        COLOR_RED='\033[0;31m'
        COLOR_GREEN='\033[0;32m'
        COLOR_YELLOW='\033[0;33m'
        COLOR_BLUE='\033[0;34m'
        COLOR_MAGENTA='\033[0;35m'
        COLOR_CYAN='\033[0;36m'
        COLOR_BOLD='\033[1m'
    else
        COLOR_RESET=''
        COLOR_RED=''
        COLOR_GREEN=''
        COLOR_YELLOW=''
        COLOR_BLUE=''
        COLOR_MAGENTA=''
        COLOR_CYAN=''
        COLOR_BOLD=''
    fi
fi

################################################################################
# HELPER FUNCTIONS
################################################################################

# Log message with level and timestamp
log_message() {
    local level="$1"
    shift
    local message="$*"
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Determine if we should log this message based on LOG_LEVEL
    local level_priority
    case "${LOG_LEVEL}" in
        DEBUG) level_priority=0 ;;
        INFO)  level_priority=1 ;;
        WARN)  level_priority=2 ;;
        ERROR) level_priority=3 ;;
        *) level_priority=1 ;;
    esac
    
    local msg_priority
    case "${level}" in
        DEBUG) msg_priority=0 ;;
        INFO)  msg_priority=1 ;;
        WARN)  msg_priority=2 ;;
        ERROR) msg_priority=3 ;;
        *) msg_priority=1 ;;
    esac
    
    # Skip if message priority is lower than configured level
    if [[ ${msg_priority} -lt ${level_priority} ]]; then
        return
    fi
    
    # Color code based on level
    local color=""
    case "${level}" in
        DEBUG) color="${COLOR_CYAN}" ;;
        INFO)  color="${COLOR_GREEN}" ;;
        WARN)  color="${COLOR_YELLOW}" ;;
        ERROR) color="${COLOR_RED}" ;;
    esac
    
    local formatted_message="${color}[${timestamp}] [${level}]${COLOR_RESET} ${message}"
    
    # Output to console
    echo -e "${formatted_message}"
    
    # Output to file if enabled
    if [[ "${LOG_TO_FILE}" == "true" ]]; then
        mkdir -p "${LOG_DIR}"
        local log_file
        log_file="${LOG_DIR}/docker-apps-$(date +%Y%m%d).log"
        echo "[${timestamp}] [${level}] ${message}" >> "${log_file}"
    fi
}

# Validate required environment variables
validate_environment() {
    if [[ "${ENV_VALIDATION}" != "true" ]]; then
        return 0
    fi
    
    log_message "DEBUG" "Validating required environment variables"
    
    local missing_vars=()
    for var in ${REQUIRED_ENV_VARS}; do
        if [[ -z "${!var}" ]]; then
            missing_vars+=("${var}")
        fi
    done
    
    if [[ ${#missing_vars[@]} -gt 0 ]]; then
        log_message "ERROR" "Missing required environment variables: ${missing_vars[*]}"
        return 1
    fi
    
    log_message "DEBUG" "All required environment variables are set"
    return 0
}

# Cleanup old log files
cleanup_old_logs() {
    if [[ "${LOG_RETENTION_DAYS}" -eq 0 ]] || [[ ! -d "${LOG_DIR}" ]]; then
        return 0
    fi
    
    log_message "DEBUG" "Cleaning up log files older than ${LOG_RETENTION_DAYS} days"
    find "${LOG_DIR}" -name "docker-apps-*.log" -type f -mtime "+${LOG_RETENTION_DAYS}" -delete 2>/dev/null || true
}

# Cleanup old backups
cleanup_old_backups() {
    if [[ "${BACKUP_RETENTION_DAYS}" -eq 0 ]] || [[ ! -d "${BACKUP_DIR}" ]]; then
        return 0
    fi
    
    log_message "DEBUG" "Cleaning up backups older than ${BACKUP_RETENTION_DAYS} days"
    find "${BACKUP_DIR}" -name "volumes-backup-*" -type d -mtime "+${BACKUP_RETENTION_DAYS}" -exec rm -rf {} + 2>/dev/null || true
}

# Export configuration for use in scripts
export LOG_TO_FILE LOG_LEVEL LOG_DIR LOG_RETENTION_DAYS
export PRUNE_IMAGES PRUNE_NETWORKS PRUNE_VOLUMES PRUNE_ALL_IMAGES
export HEALTH_CHECK_TIMEOUT HEALTH_CHECK_INTERVAL HEALTH_CHECK_SERVICES
export BACKUP_ON_STOP BACKUP_DIR BACKUP_RETENTION_DAYS BACKUP_COMPRESSION
export PULL_BEFORE_START PULL_TIMEOUT START_TIMEOUT RECREATE_CONTAINERS
export ENV_FILE ENV_TOOL ENV_VALIDATION REQUIRED_ENV_VARS
export ENABLE_NOTIFICATIONS NOTIFICATION_URL NOTIFY_ON_SUCCESS NOTIFY_ON_ERROR
export DRY_RUN VERBOSE PARALLEL_OPERATIONS
export COMPOSE_FILE COMPOSE_PROJECT_NAME STOP_TIMEOUT REMOVE_ORPHANS
export ENABLE_COLORS COLOR_RESET COLOR_RED COLOR_GREEN COLOR_YELLOW COLOR_BLUE COLOR_MAGENTA COLOR_CYAN COLOR_BOLD

################################################################################
# CONFIGURATION VALIDATION
################################################################################

# Validate configuration on load
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
    # Script is being sourced
    log_message "DEBUG" "Configuration loaded from ${BASH_SOURCE[0]}"
    
    # Run cleanup tasks
    cleanup_old_logs
    cleanup_old_backups
fi
