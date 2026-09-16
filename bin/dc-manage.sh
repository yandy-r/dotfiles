#!/usr/bin/env bash
#
# manage.sh - Unified Docker application management script
#
# This script consolidates start-apps.sh and stop-apps.sh into a single
# management interface with commands for build, deploy, rebuild, and delete.
#
# Usage:
#   ./manage.sh <command> [OPTIONS]
#
# Commands:
#   build       Pull images and build containers (no start)
#   deploy      Start services with health checks
#   rebuild     Stop, rebuild, and restart services
#   delete      Stop and remove services with cleanup
#
# Options:
#   --service=NAME          Target a specific service
#   --services=N1,N2,N3     Target multiple services
#   --service=all           Target all services (default)
#   -h, --help              Show help message
#   -v, --verbose           Enable verbose/debug output
#   -d, --dry-run           Show what would be done without executing
#
# Author: RFamily Apps Project
# License: MIT

set -euo pipefail

#==============================================================================
# SCRIPT INITIALIZATION
#==============================================================================

# Determine script directory (must be set before sourcing utils.sh)
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT

# Source utility library
# shellcheck source=scripts/lib/utils.sh
if [[ ! -f "${PROJECT_ROOT}/scripts/utils.sh" ]]; then
  echo "ERROR: Cannot find utils.sh library at ${PROJECT_ROOT}/scripts/lib/utils.sh" >&2
  exit 1
fi
source "${PROJECT_ROOT}/scripts/utils.sh"

# Source configuration file
# shellcheck source=scripts/config.sh
readonly CONFIG_FILE="${PROJECT_ROOT}/scripts/config.sh"
if [[ ! -f "${CONFIG_FILE}" ]]; then
  log_error "Cannot find config.sh at ${CONFIG_FILE}"
  exit "${EXIT_FILE_NOT_FOUND}"
fi
source "${CONFIG_FILE}"

#==============================================================================
# SCRIPT CONFIGURATION
#==============================================================================

# Compose file configuration (will be set dynamically)
CUSTOM_COMPOSE_FILE=""   # Custom compose file path from --compose
COMPOSE_FILE_PATH=""     # Final resolved compose file path

# Environment configuration (will be set dynamically)
ENVIRONMENT=""           # Environment name (local, dev, staging, prod)
CUSTOM_ENV_FILE=""       # Custom env file path from --env-file
ENV_FILE_PATH=""         # Final resolved env file path

# Command to execute
COMMAND=""

# Service selection
SELECTED_SERVICES=()
SERVICE_MODE="all" # all, single, multiple

# Script options (can be overridden by command line arguments)
SKIP_PULL=false
SKIP_HEALTH_CHECK=false
SKIP_CONFIRMATION=false
SKIP_BACKUP=false
FORCE_STOP=false
FORCE_BACKUP=false
DRY_RUN_MODE=false
CUSTOM_TIMEOUT=""
AUTO_ROLLBACK=false # Disabled by default for deploy command

# State tracking
SERVICES_STARTED=()
ROLLBACK_REQUIRED=false

#==============================================================================
# HELP FUNCTIONS
#==============================================================================

#------------------------------------------------------------------------------
# Display main usage information
#------------------------------------------------------------------------------
show_usage() {
  cat <<EOF
Usage: ${SCRIPT_NAME} [ENVIRONMENT] <command> [OPTIONS]

Unified Docker application management script.

ENVIRONMENT (optional):
    local               Use .env.local (default if not specified)
    dev                 Use .env.dev
    staging             Use .env.staging
    prod                Use .env.prod

COMMANDS:
    pull        Pull latest Docker images (no build or start)
    build       Pull images and build containers (no start)
    deploy      Start services with health checks
    rebuild     Stop, rebuild, and restart services
    delete      Stop and remove services with complete cleanup
    prune       Stop, remove containers, and delete their images

OPTIONS:
    --service=NAME          Target a specific service
    --services=N1,N2,N3     Target multiple services (comma-separated)
    --service=all           Target all services (default)
    
    --compose=PATH          Specify custom Docker Compose file path
    --env-file=PATH         Specify custom environment file path
    
    -h, --help              Show this help message or command-specific help
    -v, --verbose           Enable verbose/debug output
    -d, --dry-run           Show what would be done without executing

COMMAND-SPECIFIC OPTIONS:
    Use './manage.sh <command> --help' for detailed options per command

EXAMPLES:
    # Show help
    ${SCRIPT_NAME}
    ${SCRIPT_NAME} --help
    ${SCRIPT_NAME} deploy --help

    # Use default environment (.env.local)
    ${SCRIPT_NAME} pull
    ${SCRIPT_NAME} deploy

    # Use specific environment
    ${SCRIPT_NAME} dev deploy
    ${SCRIPT_NAME} prod deploy --service=portainer
    ${SCRIPT_NAME} staging rebuild

    # Use custom compose file
    ${SCRIPT_NAME} deploy --compose=docker-compose.prod.yml
    ${SCRIPT_NAME} build --compose=/path/to/compose.yml
    
    # Use custom env file
    ${SCRIPT_NAME} deploy --env-file=.env.custom
    ${SCRIPT_NAME} build --env-file=/path/to/.env

    # Deploy a specific service
    ${SCRIPT_NAME} deploy --service=portainer

    # Rebuild multiple services
    ${SCRIPT_NAME} rebuild --services=portainer,npm

    # Delete all services with backup
    ${SCRIPT_NAME} delete --backup

    # Prune specific service (remove container and image)
    ${SCRIPT_NAME} prune --service=portainer

    # Dry run to preview actions
    ${SCRIPT_NAME} deploy --dry-run

CONFIGURATION:
    Configuration: ${PROJECT_ROOT}/scripts/config.sh
    
COMPOSE FILES:
    Compose files are loaded from the current working directory.
    Default: docker-compose.yml (in current directory)
    
    Priority (highest to lowest):
    1. --compose option (custom path)
    2. Default (docker-compose.yml in current directory)
    
ENVIRONMENT FILES:
    Environment files are loaded from the current working directory.
    Default: .env.local (if no environment specified)
    
    Priority (highest to lowest):
    1. --env-file option (custom path)
    2. Environment argument (local/dev/staging/prod)
    3. Default (.env.local)

EXIT CODES:
    0   Success
    1   General error
    2   Missing dependency
    3   Invalid argument
    4   File not found
    5   Permission denied
    130 Interrupted by user

For more information, see the documentation at ${PROJECT_ROOT}/README.md

EOF
}

#------------------------------------------------------------------------------
# Display pull command help
#------------------------------------------------------------------------------
show_pull_help() {
  cat <<EOF
Usage: ${SCRIPT_NAME} pull [OPTIONS]

Pull latest Docker images without building or starting containers.

This command performs:
  1. Validates prerequisites and configuration
  2. Pulls latest Docker images from registries

OUTPUT BEHAVIOR:
  - All pull output lines prefixed with service name: [servicename]
  - Shows progress bars and layer extraction in real-time
  - Use --verbose for additional debug information

OPTIONS:
    --service=NAME          Pull image for a specific service
    --services=N1,N2        Pull images for multiple services
    --service=all           Pull images for all services (default)

    -v, --verbose           Enable verbose/debug output with progress bars
    -d, --dry-run           Show what would be done without executing

EXAMPLES:
    # Pull all images
    ${SCRIPT_NAME} pull

    # Pull image for specific service
    ${SCRIPT_NAME} pull --service=portainer

    # Pull images for multiple services with verbose output
    ${SCRIPT_NAME} pull --services=portainer,npm --verbose

    # Preview pull command without executing
    ${SCRIPT_NAME} pull --dry-run

EOF
}

#------------------------------------------------------------------------------
# Display build command help
#------------------------------------------------------------------------------
show_build_help() {
  cat <<EOF
Usage: ${SCRIPT_NAME} build [OPTIONS]

Pull images and build containers without starting them.

This command performs:
  1. Pulls latest Docker images (unless --skip-pull)
  2. Builds containers using docker compose build

OUTPUT BEHAVIOR:
  - All pull output lines prefixed with service name: [servicename]
  - Shows progress bars and layer extraction in real-time
  - Use --verbose for additional debug information

OPTIONS:
    --service=NAME          Build a specific service
    --services=N1,N2        Build multiple services
    --service=all           Build all services (default)

    -v, --verbose           Enable verbose/debug output with progress bars
    -d, --dry-run           Show what would be done without executing
    --skip-pull             Skip pulling latest images before build

EXAMPLES:
    # Build all services
    ${SCRIPT_NAME} build

    # Build specific service without pulling
    ${SCRIPT_NAME} build --service=portainer --skip-pull

    # Build multiple services with verbose output
    ${SCRIPT_NAME} build --services=portainer,npm --verbose

EOF
}

#------------------------------------------------------------------------------
# Display deploy command help
#------------------------------------------------------------------------------
show_deploy_help() {
  cat <<EOF
Usage: ${SCRIPT_NAME} deploy [OPTIONS]

Deploy services with full startup, health checks, and resource management.

This command performs:
  1. Validates prerequisites and configuration
  2. Pulls latest Docker images (unless --skip-pull)
  3. Starts Docker Compose services (with full output visibility)
  4. Performs health checks (unless --skip-health-check)
  5. Prunes unused Docker resources (if configured)

KEY BEHAVIOR:
  - On startup failure, existing containers are NOT stopped (safe for partial deployments)
  - Image pulling prefixes all output with service name [servicename]
  - Shows real-time progress bars and layer extraction

OUTPUT BEHAVIOR:
  - All pull output lines prefixed with service name: [servicename]
  - Shows progress bars and layer extraction in real-time
  - Use --verbose for additional debug information

OPTIONS:
    --service=NAME          Deploy a specific service
    --services=N1,N2        Deploy multiple services
    --service=all           Deploy all services (default)

    -v, --verbose           Enable verbose/debug output with progress bars
    -d, --dry-run           Show what would be done without executing
    --skip-pull             Skip pulling latest images
    --skip-health-check     Skip health checks after startup

EXAMPLES:
    # Deploy all services
    ${SCRIPT_NAME} deploy

    # Deploy specific service
    ${SCRIPT_NAME} deploy --service=portainer

    # Deploy without pulling images
    ${SCRIPT_NAME} deploy --skip-pull

    # Quick deploy without health checks
    ${SCRIPT_NAME} deploy --skip-health-check

    # Deploy with verbose output to debug issues
    ${SCRIPT_NAME} deploy --verbose

ENVIRONMENT:
    PULL_BEFORE_START       Override pull behavior (true/false)
    HEALTH_CHECK_TIMEOUT    Health check timeout in seconds

EOF
}

#------------------------------------------------------------------------------
# Display rebuild command help
#------------------------------------------------------------------------------
show_rebuild_help() {
  cat <<EOF
Usage: ${SCRIPT_NAME} rebuild [OPTIONS]

Stop services, rebuild them, and restart with full deployment cycle.

This command performs:
  1. Stops running services gracefully
  2. Pulls latest images and rebuilds containers
  3. Starts services with health checks

OUTPUT BEHAVIOR:
  - All pull output lines prefixed with service name: [servicename]
  - Shows progress bars and layer extraction in real-time
  - Use --verbose for additional debug information

OPTIONS:
    --service=NAME          Rebuild a specific service
    --services=N1,N2        Rebuild multiple services
    --service=all           Rebuild all services (default)

    -v, --verbose           Enable verbose/debug output with progress bars
    -d, --dry-run           Show what would be done without executing
    -y, --yes               Skip confirmation prompts
    -t, --timeout SEC       Override stop timeout (default: ${STOP_TIMEOUT}s)
    --skip-pull             Skip pulling latest images
    --skip-health-check     Skip health checks after restart

EXAMPLES:
    # Rebuild all services
    ${SCRIPT_NAME} rebuild

    # Rebuild specific service
    ${SCRIPT_NAME} rebuild --service=portainer

    # Rebuild multiple services without confirmation
    ${SCRIPT_NAME} rebuild --services=portainer,npm --yes

    # Quick rebuild with custom timeout
    ${SCRIPT_NAME} rebuild --timeout 30

EOF
}

#------------------------------------------------------------------------------
# Display delete command help
#------------------------------------------------------------------------------
show_delete_help() {
  cat <<EOF
Usage: ${SCRIPT_NAME} delete [OPTIONS]

Stop and completely remove services with optional backup and cleanup.

This command performs:
  1. Optionally backs up Docker volumes
  2. Stops containers (gracefully or forced)
  3. Removes containers and networks
  4. Optionally removes volumes
  5. Prunes unused resources (images, networks, volumes)

OPTIONS:
    --service=NAME          Delete a specific service
    --services=N1,N2        Delete multiple services
    --service=all           Delete all services (default)

    -v, --verbose           Enable verbose/debug output
    -d, --dry-run           Show what would be done without executing
    -y, --yes               Skip confirmation prompts
    -f, --force             Force stop (SIGKILL, no graceful shutdown)
    -b, --backup            Force backup even if not configured
    -s, --skip-backup       Skip backup even if configured
    -t, --timeout SEC       Override stop timeout (default: ${STOP_TIMEOUT}s)

EXAMPLES:
    # Delete all services with confirmation
    ${SCRIPT_NAME} delete

    # Delete specific service
    ${SCRIPT_NAME} delete --service=portainer

    # Delete all with backup and no confirmation
    ${SCRIPT_NAME} delete --backup --yes

    # Force delete without graceful shutdown
    ${SCRIPT_NAME} delete --force

    # Delete with custom timeout
    ${SCRIPT_NAME} delete --timeout 30 --yes

ENVIRONMENT:
    BACKUP_ON_STOP          Enable automatic backup (true/false)
    PRUNE_IMAGES            Prune unused images (true/false)
    PRUNE_NETWORKS          Prune unused networks (true/false)
    PRUNE_VOLUMES           Prune unused volumes (true/false)
    STOP_TIMEOUT            Graceful shutdown timeout in seconds

WARNING:
    The delete command with default settings will remove volumes and prune
    resources, which may result in data loss. Use --backup to preserve data.

EOF
}

#------------------------------------------------------------------------------
# Display prune command help
#------------------------------------------------------------------------------
show_prune_help() {
  cat <<EOF
Usage: ${SCRIPT_NAME} prune [OPTIONS]

Stop containers, remove them, and delete their associated images.

This command performs:
  1. Stops containers (gracefully or forced)
  2. Removes containers
  3. Deletes associated Docker images (only if not used by other services)

IMPORTANT: Images shared by multiple services will NOT be removed unless all
services using that image are being pruned. This prevents breaking other
running services. Unlike 'delete', this does NOT remove volumes or backups.

OPTIONS:
    --service=NAME          Prune a specific service
    --services=N1,N2        Prune multiple services
    --service=all           Prune all services (default)

    -v, --verbose           Enable verbose/debug output
    -d, --dry-run           Show what would be done without executing
    -y, --yes               Skip confirmation prompts
    -f, --force             Force stop (SIGKILL, no graceful shutdown)
    -t, --timeout SEC       Override stop timeout (default: ${STOP_TIMEOUT}s)

EXAMPLES:
    # Prune all services with confirmation
    ${SCRIPT_NAME} prune

    # Prune specific service
    ${SCRIPT_NAME} prune --service=portainer

    # Prune multiple services without confirmation
    ${SCRIPT_NAME} prune --services=portainer,npm --yes

    # Force prune without graceful shutdown
    ${SCRIPT_NAME} prune --force

    # Prune with custom timeout
    ${SCRIPT_NAME} prune --timeout 30 --yes

BEHAVIOR:
    - Images are only removed if they are not being used by other services
    - If an image is shared (e.g., authentik-server and authentik-worker), it
      will be skipped unless ALL services sharing it are being pruned
    - Running containers using an image will prevent its removal
    - Volumes are NOT affected by this command

WARNING:
    This command will permanently delete Docker images. You will need to
    pull/build images again to redeploy services.

EOF
}

#==============================================================================
# CUSTOM FUNCTIONS
#==============================================================================

#------------------------------------------------------------------------------
# Custom cleanup function
#------------------------------------------------------------------------------
cleanup_on_exit() {
  if [[ "${CLEANUP_PERFORMED}" -eq 1 ]]; then
    return 0
  fi

  CLEANUP_PERFORMED=1

  log_debug "Performing cleanup..."

  # Perform rollback if required
  if [[ "${ROLLBACK_REQUIRED}" == true ]]; then
    log_warn "Operation failed - performing rollback..."
    rollback_services
  fi

  local end_time
  end_time="$(date +%s)"
  local duration=$((end_time - SCRIPT_START_TIME))

  log_debug "Total execution time: ${duration} seconds"
}

#------------------------------------------------------------------------------
# Parse service selection arguments
#------------------------------------------------------------------------------
parse_service_selection() {
  local service_arg="$1"

  if [[ "${service_arg}" == "all" ]]; then
    SERVICE_MODE="all"
    SELECTED_SERVICES=()
    log_debug "Service mode: all services"
  elif [[ "${service_arg}" == *","* ]]; then
    SERVICE_MODE="multiple"
    IFS=',' read -ra SELECTED_SERVICES <<<"${service_arg}"
    log_debug "Service mode: multiple (${SELECTED_SERVICES[*]})"
  else
    SERVICE_MODE="single"
    SELECTED_SERVICES=("${service_arg}")
    log_debug "Service mode: single (${service_arg})"
  fi
}

#------------------------------------------------------------------------------
# Parse command line arguments
#------------------------------------------------------------------------------
parse_arguments() {
  # First argument could be environment name (local/dev/staging/prod) or command
  if [[ $# -gt 0 ]] && [[ ! "$1" =~ ^- ]]; then
    # Check if it's a valid environment name
    if [[ "$1" =~ ^(local|dev|staging|prod)$ ]]; then
      ENVIRONMENT="$1"
      log_debug "Environment set to: ${ENVIRONMENT}"
      shift
    fi
    
    # Next argument should be the command (if provided)
    if [[ $# -gt 0 ]] && [[ ! "$1" =~ ^- ]]; then
      COMMAND="$1"
      shift
    fi
  fi

  while [[ $# -gt 0 ]]; do
    case $1 in
    -h | --help)
      case "${COMMAND}" in
      pull)
        show_pull_help
        ;;
      build)
        show_build_help
        ;;
      deploy)
        show_deploy_help
        ;;
      rebuild)
        show_rebuild_help
        ;;
      delete)
        show_delete_help
        ;;
      prune)
        show_prune_help
        ;;
      *)
        show_usage
        ;;
      esac
      exit "${EXIT_SUCCESS}"
      ;;
    -v | --verbose)
      export LOG_LEVEL="${LOG_LEVEL_DEBUG}"
      export VERBOSE=true
      log_debug "Verbose mode enabled"
      shift
      ;;
    -d | --dry-run)
      DRY_RUN_MODE=true
      export DRY_RUN=true
      log_info "Dry run mode enabled - no changes will be made"
      shift
      ;;
    -y | --yes)
      SKIP_CONFIRMATION=true
      log_debug "Confirmation prompts disabled"
      shift
      ;;
    -f | --force)
      FORCE_STOP=true
      log_debug "Force stop mode enabled"
      shift
      ;;
    -b | --backup)
      FORCE_BACKUP=true
      log_debug "Force backup enabled"
      shift
      ;;
    -s | --skip-backup)
      SKIP_BACKUP=true
      log_debug "Backup disabled"
      shift
      ;;
    -t | --timeout)
      if [[ -z "${2:-}" ]] || [[ "${2}" =~ ^- ]]; then
        die "Option --timeout requires a numeric argument" "${EXIT_INVALID_ARGUMENT}"
      fi
      if ! [[ "${2}" =~ ^[0-9]+$ ]]; then
        die "Timeout must be a positive integer: ${2}" "${EXIT_INVALID_ARGUMENT}"
      fi
      CUSTOM_TIMEOUT="${2}"
      log_debug "Custom timeout set: ${CUSTOM_TIMEOUT}s"
      shift 2
      ;;
    --skip-pull)
      SKIP_PULL=true
      log_debug "Image pull will be skipped"
      shift
      ;;
    --skip-health-check)
      SKIP_HEALTH_CHECK=true
      log_debug "Health checks will be skipped"
      shift
      ;;
    --env-file=*)
      CUSTOM_ENV_FILE="${1#*=}"
      log_debug "Custom env file set: ${CUSTOM_ENV_FILE}"
      shift
      ;;
    --env-file)
      if [[ -z "${2:-}" ]] || [[ "${2}" =~ ^- ]]; then
        die "Option --env-file requires a file path argument" "${EXIT_INVALID_ARGUMENT}"
      fi
      CUSTOM_ENV_FILE="${2}"
      log_debug "Custom env file set: ${CUSTOM_ENV_FILE}"
      shift 2
      ;;
    --compose=*)
      CUSTOM_COMPOSE_FILE="${1#*=}"
      log_debug "Custom compose file set: ${CUSTOM_COMPOSE_FILE}"
      shift
      ;;
    --compose)
      if [[ -z "${2:-}" ]] || [[ "${2}" =~ ^- ]]; then
        die "Option --compose requires a file path argument" "${EXIT_INVALID_ARGUMENT}"
      fi
      CUSTOM_COMPOSE_FILE="${2}"
      log_debug "Custom compose file set: ${CUSTOM_COMPOSE_FILE}"
      shift 2
      ;;
    --service=*)
      parse_service_selection "${1#*=}"
      shift
      ;;
    --services=*)
      parse_service_selection "${1#*=}"
      shift
      ;;
    *)
      log_error "Unknown option: $1"
      show_usage
      exit "${EXIT_INVALID_ARGUMENT}"
      ;;
    esac
  done

  # Validate conflicting options
  if [[ "${FORCE_BACKUP}" == true ]] && [[ "${SKIP_BACKUP}" == true ]]; then
    die "Cannot use --backup and --skip-backup together" "${EXIT_INVALID_ARGUMENT}"
  fi
  
  if [[ -n "${ENVIRONMENT}" ]] && [[ -n "${CUSTOM_ENV_FILE}" ]]; then
    die "Cannot use both environment argument and --env-file option together" "${EXIT_INVALID_ARGUMENT}"
  fi
}

#------------------------------------------------------------------------------
# Resolve compose file path based on arguments
#------------------------------------------------------------------------------
resolve_compose_file() {
  # Priority: --compose > default (docker-compose.yml in current directory)
  
  if [[ -n "${CUSTOM_COMPOSE_FILE}" ]]; then
    # Custom compose file specified with --compose
    if [[ "${CUSTOM_COMPOSE_FILE}" =~ ^/ ]]; then
      # Absolute path
      COMPOSE_FILE_PATH="${CUSTOM_COMPOSE_FILE}"
    else
      # Relative path - resolve from current directory
      COMPOSE_FILE_PATH="$(pwd)/${CUSTOM_COMPOSE_FILE}"
    fi
    log_debug "Using custom compose file: ${COMPOSE_FILE_PATH}"
  else
    # Default to docker-compose.yml in current directory
    COMPOSE_FILE_PATH="$(pwd)/docker-compose.yml"
    log_debug "Using default compose file: ${COMPOSE_FILE_PATH}"
  fi
  
  # Make path absolute
  if [[ -f "${COMPOSE_FILE_PATH}" ]]; then
    COMPOSE_FILE_PATH="$(cd "$(dirname "${COMPOSE_FILE_PATH}")" && pwd)/$(basename "${COMPOSE_FILE_PATH}")"
  fi
}

#------------------------------------------------------------------------------
# Resolve environment file path based on arguments
#------------------------------------------------------------------------------
resolve_env_file() {
  # Priority: --env-file > environment argument > default (.env.local)
  
  if [[ -n "${CUSTOM_ENV_FILE}" ]]; then
    # Custom env file specified with --env-file
    if [[ "${CUSTOM_ENV_FILE}" =~ ^/ ]]; then
      # Absolute path
      ENV_FILE_PATH="${CUSTOM_ENV_FILE}"
    else
      # Relative path - resolve from current directory
      ENV_FILE_PATH="$(pwd)/${CUSTOM_ENV_FILE}"
    fi
    log_debug "Using custom env file: ${ENV_FILE_PATH}"
  elif [[ -n "${ENVIRONMENT}" ]]; then
    # Environment specified (local, dev, staging, prod)
    ENV_FILE_PATH="$(pwd)/.env.${ENVIRONMENT}"
    log_debug "Using environment '${ENVIRONMENT}' file: ${ENV_FILE_PATH}"
  else
    # Default to .env.local in current directory
    ENV_FILE_PATH="$(pwd)/.env.local"
    log_debug "Using default env file: ${ENV_FILE_PATH}"
  fi
  
  # Make path absolute
  if [[ -f "${ENV_FILE_PATH}" ]]; then
    ENV_FILE_PATH="$(cd "$(dirname "${ENV_FILE_PATH}")" && pwd)/$(basename "${ENV_FILE_PATH}")"
  fi
}

#------------------------------------------------------------------------------
# Validate all prerequisites
#------------------------------------------------------------------------------
validate_prerequisites() {
  print_section "Validating Prerequisites"

  # Check for required commands
  log_info "Checking for required commands..."
  require_commands docker grep awk sed

  # Check Docker and Docker Compose
  log_info "Checking Docker installation..."
  check_docker

  # Check for environment tool based on configuration
  case "${ENV_TOOL}" in
  dotenvx)
    log_info "Checking for dotenvx..."
    check_dotenvx "required"
    ;;
  op)
    log_info "Checking for 1Password CLI..."
    check_1password "required"
    ;;
  none)
    log_info "No environment tool configured - using standard docker compose"
    ;;
  *)
    log_error "Invalid ENV_TOOL setting: ${ENV_TOOL}"
    log_error "Valid options: dotenvx, op, none"
    exit "${EXIT_INVALID_ARGUMENT}"
    ;;
  esac

  log_success "All prerequisites validated"
}

#------------------------------------------------------------------------------
# Validate configuration files
#------------------------------------------------------------------------------
validate_configuration() {
  print_section "Validating Configuration Files"

  # Check if environment file exists
  log_info "Validating environment file: ${ENV_FILE_PATH}"
  validate_env_file "${ENV_FILE_PATH}"

  # Check if docker-compose.yml exists
  log_info "Validating Docker Compose file: ${COMPOSE_FILE_PATH}"
  validate_compose_file "${COMPOSE_FILE_PATH}"

  log_debug "Skipping environment validation (variables are encrypted)"

  log_success "Configuration files validated"
}

#------------------------------------------------------------------------------
# Build docker compose command with appropriate environment tool
#------------------------------------------------------------------------------
build_compose_command() {
  local compose_cmd=""

  case "${ENV_TOOL}" in
  dotenvx)
    compose_cmd="dotenvx run -f ${ENV_FILE_PATH} -- docker compose"
    ;;
  op)
    compose_cmd="op run --env-file=${ENV_FILE_PATH} -- docker compose"
    ;;
  none)
    compose_cmd="docker compose"
    ;;
  esac

  # Add compose file
  compose_cmd="${compose_cmd} -f ${COMPOSE_FILE_PATH}"

  # Add project name if specified
  if [[ -n "${COMPOSE_PROJECT_NAME}" ]]; then
    compose_cmd="${compose_cmd} -p ${COMPOSE_PROJECT_NAME}"
  fi

  echo "${compose_cmd}"
}

#------------------------------------------------------------------------------
# Get service arguments for docker compose commands
#------------------------------------------------------------------------------
get_service_args() {
  if [[ "${SERVICE_MODE}" == "all" ]]; then
    echo ""
  else
    echo "${SELECTED_SERVICES[*]}"
  fi
}

#------------------------------------------------------------------------------
# Pull latest Docker images
#------------------------------------------------------------------------------
pull_images() {
  print_section "Pulling Latest Docker Images"

  # Check if pull should be skipped
  if [[ "${SKIP_PULL}" == true ]] || [[ "${PULL_BEFORE_START}" != "true" ]]; then
    log_info "Skipping image pull (disabled by configuration or flag)"
    return 0
  fi

  local compose_cmd
  compose_cmd="$(build_compose_command)"

  local service_args
  service_args="$(get_service_args)"

  # Build pull command
  # Without --quiet, docker compose pull shows service names and progress bars
  # This provides visibility without verbose layer-by-layer output
  local pull_cmd="${compose_cmd} pull ${service_args}"

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would execute: ${pull_cmd}"
    return 0
  fi

  log_info "Pulling images (timeout: ${PULL_TIMEOUT}s)..."
  log_debug "Command: ${pull_cmd}"

  # Execute pull with timeout
  local current_service=""
  if timeout "${PULL_TIMEOUT}" bash -c "${pull_cmd}" 2>&1 | while IFS= read -r line; do
    # Skip noise from environment tools
    if [[ "${line}" =~ ^\[dotenvx ]] || [[ "${line}" =~ ^\[op\] ]] || \
       [[ "${line}" =~ "injecting env" ]] || [[ "${line}" =~ ^time= ]] || \
       [[ "${line}" =~ level=warning ]] || [[ "${line}" =~ "variable is not set" ]] || \
       [[ "${line}" =~ "Defaulting to" ]]; then
      continue
    fi

    # Track which service is being pulled (handle optional leading space)
    if [[ "${line}" =~ ^[[:space:]]*([a-zA-Z0-9_-]+)[[:space:]]+Pulling ]]; then
      current_service="${BASH_REMATCH[1]}"
      continue
    fi

    # Show completion message with service name (handle optional leading space)
    if [[ "${line}" =~ ^[[:space:]]*([a-zA-Z0-9_-]+)[[:space:]]+Pulled ]]; then
      current_service="${BASH_REMATCH[1]}"
      log_success "[${current_service}] Pulled"
      continue
    fi

    # Prefix all lines with current service name if known
    # Strip leading/trailing whitespace from line
    line="${line#"${line%%[![:space:]]*}"}"  # Remove leading whitespace
    line="${line%"${line##*[![:space:]]}"}"  # Remove trailing whitespace
    
    if [[ -n "${current_service}" ]] && [[ -n "${line}" ]]; then
      echo "[${current_service}] ${line}"
    elif [[ -n "${line}" ]]; then
      echo "  ${line}"
    fi
  done; then
    log_success "Images pulled successfully"
    return 0
  else
    local exit_code=$?
    if [[ ${exit_code} -eq 124 ]]; then
      log_error "Image pull timed out after ${PULL_TIMEOUT} seconds"
    else
      log_error "Failed to pull images (exit code: ${exit_code})"
    fi
    return 1
  fi
}

#------------------------------------------------------------------------------
# Build Docker containers
#------------------------------------------------------------------------------
build_containers() {
  print_section "Building Docker Containers"

  local compose_cmd
  compose_cmd="$(build_compose_command)"

  local service_args
  service_args="$(get_service_args)"

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would execute: ${compose_cmd} build ${service_args}"
    return 0
  fi

  log_info "Building containers..."
  log_debug "Command: ${compose_cmd} build ${service_args}"

  # Execute build
  if bash -c "${compose_cmd} build ${service_args}" 2>&1 | while IFS= read -r line; do
    log_debug "  ${line}"
  done; then
    log_success "Containers built successfully"
    return 0
  else
    log_error "Failed to build containers"
    return 1
  fi
}

#------------------------------------------------------------------------------
# Start Docker Compose services
#------------------------------------------------------------------------------
start_services() {
  print_section "Starting Docker Compose Services"

  local compose_cmd
  compose_cmd="$(build_compose_command)"

  local service_args
  service_args="$(get_service_args)"

  # Build docker compose up command
  local up_cmd="${compose_cmd} up -d"

  # Add remove orphans flag if configured
  if [[ "${REMOVE_ORPHANS}" == "true" ]]; then
    up_cmd="${up_cmd} --remove-orphans"
  fi

  # Add recreate option based on configuration
  case "${RECREATE_CONTAINERS}" in
  always)
    up_cmd="${up_cmd} --force-recreate"
    ;;
  never)
    up_cmd="${up_cmd} --no-recreate"
    ;;
  changed)
    # Default behavior - no flag needed
    ;;
  esac

  # Add service arguments
  up_cmd="${up_cmd} ${service_args}"

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would execute: ${up_cmd}"
    return 0
  fi

  log_info "Starting services (timeout: ${START_TIMEOUT}s)..."
  log_debug "Command: ${up_cmd}"

  # Create temporary file to capture output
  local output_file
  output_file="$(mktemp)"
  # shellcheck disable=SC2064  # expand output_file now, deliberately
  trap "rm -f '${output_file}'" RETURN

  # Execute docker compose up with timeout
  if timeout "${START_TIMEOUT}" bash -c "${up_cmd}" >"${output_file}" 2>&1; then
    log_success "Services started successfully"

    # Show all output at INFO level
    while IFS= read -r line; do
      [[ -n "${line}" ]] && log_info "  ${line}"
    done <"${output_file}"

    # Get list of started services
    if [[ "${SERVICE_MODE}" == "all" ]]; then
      # Filter out dotenvx/op messages and docker compose warnings, keep only valid service names
      mapfile -t SERVICES_STARTED < <(bash -c "${compose_cmd} ps --services" 2>&1 |
        grep -v "\[dotenvx" |
        grep -v "\[op\]" |
        grep -v "injecting env" |
        grep -v '^time=' |
        grep -v 'level=warning' |
        grep -v 'variable is not set' |
        grep -v 'Defaulting to' |
        grep -E '^[a-zA-Z0-9_-]+$')
    else
      SERVICES_STARTED=("${SELECTED_SERVICES[@]}")
    fi
    log_debug "Started services: ${SERVICES_STARTED[*]}"

    return 0
  else
    local exit_code=$?
    log_error "Failed to start services (exit code: ${exit_code})"
    log_error "Docker Compose output:"
    while IFS= read -r line; do
      [[ -n "${line}" ]] && log_error "  ${line}"
    done <"${output_file}"

    if [[ ${exit_code} -eq 124 ]]; then
      log_error "Service startup timed out after ${START_TIMEOUT} seconds"
    fi

    # Only set rollback for rebuild/delete commands, not for deploy
    if [[ "${AUTO_ROLLBACK}" == true ]]; then
      ROLLBACK_REQUIRED=true
    else
      log_warn "Startup failed - automatic rollback is disabled for deploy command"
      log_warn "Existing containers remain running. Fix configuration and retry."
    fi
    return 1
  fi
}

#------------------------------------------------------------------------------
# Check health of running services
#------------------------------------------------------------------------------
check_service_health() {
  print_section "Checking Service Health"

  if [[ "${SKIP_HEALTH_CHECK}" == true ]] || [[ "${HEALTH_CHECK_TIMEOUT}" -eq 0 ]]; then
    log_info "Skipping health checks (disabled by configuration or flag)"
    return 0
  fi

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would check health of services"
    return 0
  fi

  # Simple health check: just wait a bit and verify containers are up
  log_info "Performing simple health check..."
  log_info "Waiting ${HEALTH_CHECK_INTERVAL} seconds for services to initialize..."
  sleep "${HEALTH_CHECK_INTERVAL}"

  # Get count of running containers using docker compose ps
  # This works regardless of whether containers use project name prefix or explicit container_name
  local compose_cmd
  compose_cmd="$(build_compose_command)"

  local running_count
  running_count=$(bash -c "${compose_cmd} ps --status running --format '{{.Name}}'" 2>/dev/null |
    grep -v "^\[" |
    grep -v "^time=" |
    grep -v "level=" |
    grep -v "variable is not set" |
    grep -v "Defaulting to" |
    grep -E "^[a-zA-Z0-9_-]+$" |
    wc -l |
    tr -d ' ')

  if [[ ${running_count} -gt 0 ]]; then
    log_success "Health check passed: ${running_count} container(s) running"
    return 0
  else
    log_warn "No containers appear to be running for this compose project"
    log_warn "This may be normal if containers are still starting"
    return 0
  fi
}

#------------------------------------------------------------------------------
# Display service status
#------------------------------------------------------------------------------
show_service_status() {
  print_section "Service Status"

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would display service status"
    return 0
  fi

  log_info "Current service status:"

  # Get service status
  local compose_cmd
  compose_cmd="$(build_compose_command)"
  bash -c "${compose_cmd} ps --format 'table {{.Name}}\t{{.Status}}\t{{.Ports}}'" 2>&1 |
    grep -v "\[dotenvx" |
    grep -v "\[op\]" |
    grep -v "injecting env" |
    grep -v '^time=' |
    grep -v 'level=warning' |
    grep -v 'variable is not set' |
    grep -v 'Defaulting to' |
    while IFS= read -r line; do
      log_info "  ${line}"
    done

  # Show resource usage summary
  log_info ""
  log_info "Docker resource usage:"
  docker system df --format "table {{.Type}}\t{{.TotalCount}}\t{{.Size}}" 2>&1 | while IFS= read -r line; do
    log_info "  ${line}"
  done
}

#------------------------------------------------------------------------------
# Prune unused Docker resources
#------------------------------------------------------------------------------
prune_resources() {
  print_section "Cleaning Up Docker Resources"

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would prune Docker resources based on configuration"
    return 0
  fi

  local pruned_something=false

  # Prune images if configured
  if [[ "${PRUNE_IMAGES}" == "true" ]]; then
    log_info "Pruning unused Docker images..."

    if [[ "${PRUNE_ALL_IMAGES}" == "true" ]]; then
      log_warn "Pruning ALL unused images (not just dangling)"
      docker image prune -af 2>&1 | while IFS= read -r line; do
        log_debug "  ${line}"
      done
    else
      docker image prune -f 2>&1 | while IFS= read -r line; do
        log_debug "  ${line}"
      done
    fi

    log_success "Image pruning completed"
    pruned_something=true
  fi

  # Prune networks if configured
  if [[ "${PRUNE_NETWORKS}" == "true" ]]; then
    log_info "Pruning unused Docker networks..."
    docker network prune -f 2>&1 | while IFS= read -r line; do
      log_debug "  ${line}"
    done
    log_success "Network pruning completed"
    pruned_something=true
  fi

  # Prune volumes if configured (with warning)
  if [[ "${PRUNE_VOLUMES}" == "true" ]]; then
    log_warn "Pruning unused Docker volumes (this may result in data loss)..."
    docker volume prune -f 2>&1 | while IFS= read -r line; do
      log_debug "  ${line}"
    done
    log_success "Volume pruning completed"
    pruned_something=true
  fi

  if [[ "${pruned_something}" == false ]]; then
    log_info "Resource pruning disabled by configuration"
  fi
}

#------------------------------------------------------------------------------
# Backup Docker volumes
#------------------------------------------------------------------------------
backup_volumes() {
  local backup_dir
  backup_dir="${BACKUP_DIR}/volumes-backup-$(date +%Y%m%d-%H%M%S)"

  print_section "Backing Up Docker Volumes"

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would create backup in: ${backup_dir}"
    return 0
  fi

  log_info "Creating backup directory: ${backup_dir}"
  if ! mkdir -p "${backup_dir}"; then
    log_error "Failed to create backup directory"
    return 1
  fi

  # Get list of volumes used by compose project
  log_info "Identifying volumes to backup..."
  local compose_cmd
  compose_cmd="$(build_compose_command)"
  local volumes
  volumes=$(bash -c "${compose_cmd} config --volumes" 2>&1 |
    grep -v "\[dotenvx" |
    grep -v "\[op\]" |
    grep -v "injecting env" |
    grep -v '^time=' |
    grep -v 'level=warning' |
    grep -v 'variable is not set' |
    grep -v 'Defaulting to' || echo "")

  if [[ -z "${volumes}" ]]; then
    log_warn "No volumes found to backup"
    return 0
  fi

  local volume_count=0
  local failed_count=0

  while IFS= read -r volume; do
    [[ -z "${volume}" ]] && continue

    log_info "Backing up volume: ${volume}"

    local backup_file="${backup_dir}/${volume}.tar"
    if [[ "${BACKUP_COMPRESSION}" == "true" ]]; then
      backup_file="${backup_file}.gz"
    fi

    # Create backup using a temporary container
    local compress_cmd=""
    local ext=""
    if [[ "${BACKUP_COMPRESSION}" == "true" ]]; then
      compress_cmd="| gzip"
      ext=".gz"
    fi

    if docker run --rm \
      -v "${volume}:/volume:ro" \
      -v "${backup_dir}:/backup" \
      alpine \
      sh -c "cd /volume && tar cf - . ${compress_cmd} > /backup/${volume}.tar${ext}" 2>&1 | while read -r line; do
      log_debug "  ${line}"
    done; then
      log_success "Backed up: ${volume}"
      ((volume_count++))
    else
      log_error "Failed to backup: ${volume}"
      ((failed_count++))
    fi
  done <<<"${volumes}"

  if [[ "${failed_count}" -gt 0 ]]; then
    log_warn "Backup completed with ${failed_count} failure(s)"
    log_info "Successfully backed up ${volume_count} volume(s) to: ${backup_dir}"
    return 1
  else
    log_success "Successfully backed up ${volume_count} volume(s) to: ${backup_dir}"
    return 0
  fi
}

#------------------------------------------------------------------------------
# Stop Docker containers
#------------------------------------------------------------------------------
stop_containers() {
  print_section "Stopping Docker Containers"

  # Determine timeout
  local timeout="${CUSTOM_TIMEOUT:-${STOP_TIMEOUT}}"

  # Build docker compose command
  local compose_cmd
  compose_cmd="$(build_compose_command)"

  local service_args
  service_args="$(get_service_args)"

  local stop_cmd="down"
  local stop_args=()

  # Add timeout
  if [[ -n "${timeout}" ]]; then
    stop_args+=("--timeout" "${timeout}")
  fi

  # Add remove orphans flag
  if [[ "${REMOVE_ORPHANS}" == "true" ]]; then
    stop_args+=("--remove-orphans")
  fi

  # For specific services, use stop instead of down
  if [[ "${SERVICE_MODE}" != "all" ]]; then
    log_info "Stopping specific services: ${SELECTED_SERVICES[*]}"
    stop_cmd="stop"
    if [[ -n "${timeout}" ]]; then
      stop_args=("--timeout" "${timeout}")
    fi
  else
    log_info "Stopping all services"
  fi

  # Show command in dry run mode
  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would execute: ${compose_cmd} ${stop_cmd} ${stop_args[*]} ${service_args}"
    return 0
  fi

  # Execute stop command
  log_info "Executing stop command (timeout: ${timeout}s)..."

  if [[ "${FORCE_STOP}" == true ]]; then
    log_warn "Force stop enabled - containers will be killed immediately"
    if bash -c "${compose_cmd} kill ${service_args}" 2>&1 | while read -r line; do
      log_debug "  ${line}"
    done; then
      log_success "Containers killed successfully"
    else
      log_error "Failed to kill containers"
      return 1
    fi

    # Remove containers after killing
    if bash -c "${compose_cmd} down --remove-orphans" 2>&1 | while read -r line; do
      log_debug "  ${line}"
    done; then
      log_success "Containers removed successfully"
    else
      log_error "Failed to remove containers"
      return 1
    fi
  else
    # Graceful stop
    if bash -c "${compose_cmd} ${stop_cmd} ${stop_args[*]} ${service_args}" 2>&1 | while read -r line; do
      log_debug "  ${line}"
    done; then
      log_success "Services stopped successfully"
    else
      log_error "Failed to stop services"
      return 1
    fi
  fi

  return 0
}

#------------------------------------------------------------------------------
# Cleanup Docker resources (comprehensive for delete command)
#------------------------------------------------------------------------------
cleanup_resources() {
  print_section "Cleaning Up Docker Resources"

  local cleanup_performed=false

  # Prune images
  if [[ "${PRUNE_IMAGES}" == "true" ]]; then
    log_info "Pruning unused Docker images..."

    if [[ "${DRY_RUN_MODE}" == true ]]; then
      log_info "[DRY RUN] Would prune unused images"
    else
      local prune_args=("-f")

      if [[ "${PRUNE_ALL_IMAGES}" == "true" ]]; then
        prune_args+=("--all")
        log_warn "Pruning ALL unused images (not just dangling)"
      fi

      if docker image prune "${prune_args[@]}" 2>&1 | while read -r line; do
        log_debug "  ${line}"
      done; then
        log_success "Images pruned successfully"
        cleanup_performed=true
      else
        log_warn "Failed to prune images"
      fi
    fi
  else
    log_debug "Image pruning disabled (PRUNE_IMAGES=false)"
  fi

  # Prune networks
  if [[ "${PRUNE_NETWORKS}" == "true" ]]; then
    log_info "Pruning unused Docker networks..."

    if [[ "${DRY_RUN_MODE}" == true ]]; then
      log_info "[DRY RUN] Would prune unused networks"
    else
      if docker network prune -f 2>&1 | while read -r line; do
        log_debug "  ${line}"
      done; then
        log_success "Networks pruned successfully"
        cleanup_performed=true
      else
        log_warn "Failed to prune networks"
      fi
    fi
  else
    log_debug "Network pruning disabled (PRUNE_NETWORKS=false)"
  fi

  # Prune volumes (with extra confirmation)
  if [[ "${PRUNE_VOLUMES}" == "true" ]] && [[ "${SERVICE_MODE}" == "all" ]]; then
    log_warn "Volume pruning is enabled - this will delete unused volume data!"

    if [[ "${SKIP_CONFIRMATION}" == false ]]; then
      if ! confirm "Are you sure you want to prune volumes?" "n"; then
        log_info "Skipping volume pruning"
      else
        log_info "Pruning unused Docker volumes..."

        if [[ "${DRY_RUN_MODE}" == true ]]; then
          log_info "[DRY RUN] Would prune unused volumes"
        else
          if docker volume prune -f 2>&1 | while read -r line; do
            log_debug "  ${line}"
          done; then
            log_success "Volumes pruned successfully"
            cleanup_performed=true
          else
            log_warn "Failed to prune volumes"
          fi
        fi
      fi
    else
      # Skip confirmation mode - still prune
      if [[ "${DRY_RUN_MODE}" == true ]]; then
        log_info "[DRY RUN] Would prune unused volumes"
      else
        if docker volume prune -f 2>&1 | while read -r line; do
          log_debug "  ${line}"
        done; then
          log_success "Volumes pruned successfully"
          cleanup_performed=true
        else
          log_warn "Failed to prune volumes"
        fi
      fi
    fi
  else
    if [[ "${SERVICE_MODE}" != "all" ]]; then
      log_debug "Volume pruning skipped (not stopping all services)"
    else
      log_debug "Volume pruning disabled (PRUNE_VOLUMES=false)"
    fi
  fi

  if [[ "${cleanup_performed}" == true ]]; then
    log_success "Resource cleanup completed"
  else
    log_info "No cleanup operations performed"
  fi

  # Show final disk usage
  if [[ "${DRY_RUN_MODE}" == false ]]; then
    log_info "Final Docker disk usage:"
    docker system df --format "table {{.Type}}\t{{.TotalCount}}\t{{.Size}}\t{{.Reclaimable}}" 2>/dev/null | tail -n +2 | while read -r line; do
      log_info "  ${line}"
    done
  fi
}

#------------------------------------------------------------------------------
# Rollback services on failure
#------------------------------------------------------------------------------
rollback_services() {
  print_section "Rolling Back Services"

  log_warn "Stopping services due to failure..."

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would stop all services"
    return 0
  fi

  local compose_cmd
  compose_cmd="$(build_compose_command)"

  # Stop services
  if bash -c "${compose_cmd} down --remove-orphans" 2>&1 | while IFS= read -r line; do
    log_debug "  ${line}"
  done; then
    log_success "Services stopped successfully"
  else
    log_error "Failed to stop services during rollback"
  fi
}

#------------------------------------------------------------------------------
# Send notification (if configured)
#------------------------------------------------------------------------------
send_notification() {
  local status="$1"
  local message="$2"

  # Check if notifications are enabled
  if [[ "${ENABLE_NOTIFICATIONS}" != "true" ]]; then
    return 0
  fi

  # Check if we should notify for this status
  if [[ "${status}" == "success" ]] && [[ "${NOTIFY_ON_SUCCESS}" != "true" ]]; then
    return 0
  fi

  if [[ "${status}" == "error" ]] && [[ "${NOTIFY_ON_ERROR}" != "true" ]]; then
    return 0
  fi

  # Check if notification URL is configured
  if [[ -z "${NOTIFICATION_URL}" ]]; then
    log_debug "Notification URL not configured"
    return 0
  fi

  log_debug "Sending ${status} notification..."

  # Send notification via webhook
  if command_exists curl; then
    curl -X POST "${NOTIFICATION_URL}" \
      -H "Content-Type: application/json" \
      -d "{\"status\":\"${status}\",\"message\":\"${message}\",\"script\":\"${SCRIPT_NAME}\",\"command\":\"${COMMAND}\"}" \
      >/dev/null 2>&1 || log_debug "Failed to send notification"
  fi
}

#------------------------------------------------------------------------------
# Remove Docker images for specified services
#------------------------------------------------------------------------------
remove_service_images() {
  print_section "Removing Docker Images"

  local compose_cmd
  compose_cmd="$(build_compose_command)"

  # Get list of services to remove images for
  local services_to_process=()
  if [[ "${SERVICE_MODE}" == "all" ]]; then
    # Get all services from compose file
    log_info "Getting list of all services..."
    mapfile -t services_to_process < <(bash -c "${compose_cmd} config --services" 2>&1 |
      grep -v "\[dotenvx" |
      grep -v "\[op\]" |
      grep -v "injecting env" |
      grep -v '^time=' |
      grep -v 'level=warning' |
      grep -v 'variable is not set' |
      grep -v 'Defaulting to' |
      grep -E '^[a-zA-Z0-9_-]+$')
  else
    services_to_process=("${SELECTED_SERVICES[@]}")
  fi

  if [[ ${#services_to_process[@]} -eq 0 ]]; then
    log_warn "No services found to remove images for"
    return 0
  fi

  log_info "Services to remove images for: ${services_to_process[*]}"

  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would remove images for ${#services_to_process[@]} service(s)"
    return 0
  fi

  # Get list of ALL services in compose file to check for image reuse
  log_info "Checking for shared images across services..."
  local all_services=()
  mapfile -t all_services < <(bash -c "${compose_cmd} config --services" 2>&1 |
    grep -v "\[dotenvx" |
    grep -v "\[op\]" |
    grep -v "injecting env" |
    grep -v '^time=' |
    grep -v 'level=warning' |
    grep -v 'variable is not set' |
    grep -v 'Defaulting to' |
    grep -E '^[a-zA-Z0-9_-]+$')

  # Build a map of which images are used by which services
  declare -A image_to_services
  local config_json
  if command -v jq &>/dev/null; then
    config_json=$(bash -c "${compose_cmd} config --format json" 2>&1 |
      grep -v "\[dotenvx" |
      grep -v "\[op\]" |
      grep -v "injecting env" |
      grep -v '^time=' |
      grep -v 'level=warning' |
      grep -v 'variable is not set' |
      grep -v 'Defaulting to')
    
    for svc in "${all_services[@]}"; do
      local img
      img=$(echo "${config_json}" | jq -r ".services.\"${svc}\".image // empty" 2>/dev/null)
      if [[ -n "${img}" ]]; then
        if [[ -n "${image_to_services[${img}]:-}" ]]; then
          image_to_services["${img}"]="${image_to_services[${img}]},${svc}"
        else
          image_to_services["${img}"]="${svc}"
        fi
      fi
    done
  fi

  local images_removed=0
  local images_skipped=0
  local images_failed=0

  for service in "${services_to_process[@]}"; do
    log_info "Processing service: ${service}"
    
    # Get container name for this service (to find the actual image ID used)
    local container_name
    container_name=$(bash -c "${compose_cmd} ps --all --format '{{.Name}}' ${service}" 2>&1 |
      grep -v "\[dotenvx" |
      grep -v "\[op\]" |
      grep -v "injecting env" |
      grep -v '^time=' |
      grep -v 'level=warning' |
      grep -v 'variable is not set' |
      grep -v 'Defaulting to' |
      head -1)
    
    # Get the image ID that was used by this container
    local image_id=""
    local image_name=""
    
    if [[ -n "${container_name}" ]]; then
      # Container exists/existed - get the exact image it used
      image_id=$(docker inspect --format='{{.Image}}' "${container_name}" 2>/dev/null || echo "")
      image_name=$(docker inspect --format='{{.Config.Image}}' "${container_name}" 2>/dev/null || echo "")
      log_debug "Found container ${container_name} using image: ${image_name} (${image_id})"
    else
      # No container found, try to get image from compose config
      if command -v jq &>/dev/null; then
        image_name=$(echo "${config_json}" | jq -r ".services.\"${service}\".image // empty" 2>/dev/null)
      fi
      
      if [[ -n "${image_name}" ]]; then
        image_id=$(docker images --format='{{.ID}}' "${image_name}" 2>/dev/null | head -1)
        log_debug "Found image from config: ${image_name} (${image_id})"
      fi
    fi
    
    if [[ -z "${image_id}" ]] && [[ -z "${image_name}" ]]; then
      log_warn "Could not determine image for service: ${service} (may use build or already removed)"
      ((images_failed++))
      continue
    fi
    
    # Check if this image is used by other services
    if [[ -n "${image_name}" ]] && [[ -n "${image_to_services[${image_name}]:-}" ]]; then
      local services_using_image="${image_to_services[${image_name}]:-}"
      local other_services=""
      
      # Check if any OTHER services (not in our prune list) use this image
      IFS=',' read -ra svc_array <<< "${services_using_image}"
      for svc in "${svc_array[@]}"; do
        # Skip if this is the service we're trying to prune
        if [[ "${svc}" == "${service}" ]]; then
          continue
        fi
        
        # Check if this other service is also being pruned
        local is_being_pruned=false
        for prune_svc in "${services_to_process[@]}"; do
          if [[ "${svc}" == "${prune_svc}" ]]; then
            is_being_pruned=true
            break
          fi
        done
        
        if [[ "${is_being_pruned}" == false ]]; then
          other_services="${other_services:+${other_services},}${svc}"
        fi
      done
      
      if [[ -n "${other_services}" ]]; then
        log_warn "Skipping image ${image_name} - also used by: ${other_services}"
        ((images_skipped++))
        continue
      fi
    fi
    
    # Check if image is in use by any running containers
    if [[ -n "${image_id}" ]]; then
      local containers_using_image
      containers_using_image=$(docker ps --format '{{.Names}}' --filter "ancestor=${image_id}" 2>/dev/null | tr '\n' ',' | sed 's/,$//')
      
      if [[ -n "${containers_using_image}" ]]; then
        log_warn "Skipping image ${image_name} - in use by running container(s): ${containers_using_image}"
        ((images_skipped++))
        continue
      fi
    fi
    
    # Safe to remove - not used by other services or running containers
    local target="${image_name}"
    if [[ -z "${target}" ]]; then
      target="${image_id}"
    fi
    
    log_info "Removing image: ${target}"
    if docker rmi "${target}" 2>&1 | while IFS= read -r line; do
      log_debug "  ${line}"
    done; then
      log_success "Removed image: ${target}"
      ((images_removed++))
    else
      log_warn "Failed to remove image: ${target}"
      ((images_failed++))
    fi
  done

  log_info "Image removal summary: ${images_removed} removed, ${images_skipped} skipped (shared/in-use), ${images_failed} failed"
  
  if [[ ${images_removed} -eq 0 ]] && [[ ${images_skipped} -gt 0 ]]; then
    log_warn "No images removed - all images are shared with other services"
    return 0
  fi
  
  if [[ ${images_removed} -gt 0 ]]; then
    log_success "Successfully removed ${images_removed} image(s)"
    return 0
  else
    return 1
  fi
}

#==============================================================================
# COMMAND HANDLERS
#==============================================================================

#------------------------------------------------------------------------------
# Handle pull command
#------------------------------------------------------------------------------
cmd_pull() {
  print_section "Pull Command"
  log_info "Pulling Docker images"

  if [[ "${SERVICE_MODE}" != "all" ]]; then
    log_info "Target services: ${SELECTED_SERVICES[*]}"
  fi

  # Validate prerequisites
  validate_prerequisites

  # Validate configuration
  validate_configuration

  # Pull images
  if ! pull_images; then
    send_notification "error" "Failed to pull Docker images"
    die "Image pull failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Success message
  print_section "Pull Completed Successfully"
  log_success "All images pulled successfully"
  send_notification "success" "Docker images pulled successfully"

  local end_time
  end_time="$(date +%s)"
  local duration=$((end_time - SCRIPT_START_TIME))
  log_info "Total execution time: ${duration} seconds"
}

#------------------------------------------------------------------------------
# Handle build command
#------------------------------------------------------------------------------
cmd_build() {
  print_section "Build Command"
  log_info "Building Docker containers"

  if [[ "${SERVICE_MODE}" != "all" ]]; then
    log_info "Target services: ${SELECTED_SERVICES[*]}"
  fi

  # Validate prerequisites
  validate_prerequisites

  # Validate configuration
  validate_configuration

  # Pull images if configured
  if ! pull_images; then
    send_notification "error" "Failed to pull Docker images"
    die "Image pull failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Build containers
  if ! build_containers; then
    send_notification "error" "Failed to build Docker containers"
    die "Container build failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Success message
  print_section "Build Completed Successfully"
  log_success "All containers built successfully"
  send_notification "success" "Docker containers built successfully"

  local end_time
  end_time="$(date +%s)"
  local duration=$((end_time - SCRIPT_START_TIME))
  log_info "Total execution time: ${duration} seconds"
}

#------------------------------------------------------------------------------
# Handle deploy command
#------------------------------------------------------------------------------
cmd_deploy() {
  print_section "Deploy Command"
  log_info "Deploying Docker services"

  if [[ "${SERVICE_MODE}" != "all" ]]; then
    log_info "Target services: ${SELECTED_SERVICES[*]}"
  fi

  # Disable auto-rollback for deploy - we don't want to stop existing containers
  AUTO_ROLLBACK=false

  # Validate prerequisites
  validate_prerequisites

  # Validate configuration
  validate_configuration

  # Pull images if configured
  if ! pull_images; then
    send_notification "error" "Failed to pull Docker images"
    die "Image pull failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Start services
  if ! start_services; then
    send_notification "error" "Failed to start Docker services"
    die "Service startup failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Check service health
  if ! check_service_health; then
    send_notification "error" "Service health checks failed"
    die "Health checks failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Show service status
  show_service_status

  # Prune resources
  prune_resources

  # Success message
  print_section "Deploy Completed Successfully"
  log_success "All services deployed and are healthy"
  log_info "Log file: ${LOG_FILE}"
  send_notification "success" "All Docker services deployed successfully"

  local end_time
  end_time="$(date +%s)"
  local duration=$((end_time - SCRIPT_START_TIME))
  log_info "Total execution time: ${duration} seconds"
}

#------------------------------------------------------------------------------
# Handle rebuild command
#------------------------------------------------------------------------------
cmd_rebuild() {
  print_section "Rebuild Command"
  log_info "Rebuilding Docker services (stop → build → start)"

  if [[ "${SERVICE_MODE}" != "all" ]]; then
    log_info "Target services: ${SELECTED_SERVICES[*]}"
  fi

  # Enable auto-rollback for rebuild
  AUTO_ROLLBACK=true

  # Validate prerequisites
  validate_prerequisites

  # Validate configuration
  validate_configuration

  # Confirm action unless --yes flag is used
  if [[ "${SKIP_CONFIRMATION}" == false ]] && [[ "${DRY_RUN_MODE}" == false ]]; then
    local action_desc="rebuild all services"
    if [[ "${SERVICE_MODE}" != "all" ]]; then
      action_desc="rebuild services: ${SELECTED_SERVICES[*]}"
    fi

    if ! confirm "Do you want to ${action_desc}?" "y"; then
      log_info "Operation cancelled by user"
      exit "${EXIT_SUCCESS}"
    fi
  fi

  # Step 1: Stop services
  if ! stop_containers; then
    send_notification "error" "Failed to stop services during rebuild"
    die "Failed to stop containers" "${EXIT_GENERAL_ERROR}"
  fi

  # Step 2: Pull and build
  if ! pull_images; then
    send_notification "error" "Failed to pull images during rebuild"
    die "Image pull failed" "${EXIT_GENERAL_ERROR}"
  fi

  if ! build_containers; then
    send_notification "error" "Failed to build containers during rebuild"
    die "Container build failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Step 3: Start services
  if ! start_services; then
    send_notification "error" "Failed to start services during rebuild"
    die "Service startup failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Check service health
  if ! check_service_health; then
    send_notification "error" "Service health checks failed after rebuild"
    die "Health checks failed" "${EXIT_GENERAL_ERROR}"
  fi

  # Show service status
  show_service_status

  # Success message
  print_section "Rebuild Completed Successfully"
  log_success "All services rebuilt and restarted successfully"
  log_info "Log file: ${LOG_FILE}"
  send_notification "success" "Docker services rebuilt successfully"

  local end_time
  end_time="$(date +%s)"
  local duration=$((end_time - SCRIPT_START_TIME))
  log_info "Total execution time: ${duration} seconds"
}

#------------------------------------------------------------------------------
# Handle delete command
#------------------------------------------------------------------------------
cmd_delete() {
  print_section "Delete Command"
  log_info "Deleting Docker services with complete cleanup"

  if [[ "${SERVICE_MODE}" != "all" ]]; then
    log_info "Target services: ${SELECTED_SERVICES[*]}"
  fi

  # Validate prerequisites
  validate_prerequisites

  # Validate configuration
  validate_configuration

  # Confirm action unless --yes flag is used
  if [[ "${SKIP_CONFIRMATION}" == false ]] && [[ "${DRY_RUN_MODE}" == false ]]; then
    local action_desc="delete all services and cleanup resources"
    if [[ "${SERVICE_MODE}" != "all" ]]; then
      action_desc="delete services: ${SELECTED_SERVICES[*]}"
    fi

    log_warn "This will stop containers and prune resources (potentially removing volumes)"
    if ! confirm "Do you want to ${action_desc}?" "n"; then
      log_info "Operation cancelled by user"
      exit "${EXIT_SUCCESS}"
    fi
  fi

  # Backup volumes if configured
  local should_backup=false

  if [[ "${FORCE_BACKUP}" == true ]]; then
    should_backup=true
  elif [[ "${SKIP_BACKUP}" == false ]] && [[ "${BACKUP_ON_STOP}" == "true" ]]; then
    should_backup=true
  fi

  if [[ "${should_backup}" == true ]]; then
    if ! backup_volumes; then
      log_warn "Backup failed, but continuing with deletion"
      if [[ "${SKIP_CONFIRMATION}" == false ]]; then
        if ! confirm "Continue with deletion despite backup failure?" "y"; then
          die "Deletion cancelled due to backup failure" "${EXIT_GENERAL_ERROR}"
        fi
      fi
    fi
  else
    log_debug "Skipping backup (not configured or disabled)"
  fi

  # Stop containers
  if ! stop_containers; then
    send_notification "error" "Failed to stop containers during delete"
    die "Failed to stop containers" "${EXIT_GENERAL_ERROR}"
  fi

  # Cleanup resources
  cleanup_resources

  # Success message
  print_section "Delete Completed Successfully"
  log_success "All services deleted and resources cleaned up"

  if [[ -n "${LOG_FILE}" ]]; then
    log_info "Log file: ${LOG_FILE}"
  fi

  if [[ "${should_backup}" == true ]] && [[ "${DRY_RUN_MODE}" == false ]]; then
    log_info "Backup location: ${BACKUP_DIR}"
  fi

  send_notification "success" "Docker services deleted successfully"

  local end_time
  end_time="$(date +%s)"
  local duration=$((end_time - SCRIPT_START_TIME))
  log_info "Total execution time: ${duration} seconds"
}

#------------------------------------------------------------------------------
# Handle prune command
#------------------------------------------------------------------------------
cmd_prune() {
  print_section "Prune Command"
  log_info "Pruning Docker services (stop → remove → delete images)"

  if [[ "${SERVICE_MODE}" != "all" ]]; then
    log_info "Target services: ${SELECTED_SERVICES[*]}"
  fi

  # Validate prerequisites
  validate_prerequisites

  # Validate configuration
  validate_configuration

  # Confirm action unless --yes flag is used
  if [[ "${SKIP_CONFIRMATION}" == false ]] && [[ "${DRY_RUN_MODE}" == false ]]; then
    local action_desc="prune all services (stop, remove containers, delete images)"
    if [[ "${SERVICE_MODE}" != "all" ]]; then
      action_desc="prune services: ${SELECTED_SERVICES[*]} (stop, remove containers, delete images)"
    fi

    log_warn "This will permanently delete Docker images for the selected services"
    if ! confirm "Do you want to ${action_desc}?" "n"; then
      log_info "Operation cancelled by user"
      exit "${EXIT_SUCCESS}"
    fi
  fi

  # Step 1: Stop containers
  if ! stop_containers; then
    send_notification "error" "Failed to stop containers during prune"
    die "Failed to stop containers" "${EXIT_GENERAL_ERROR}"
  fi

  # Step 2: Remove containers (use down to ensure cleanup)
  print_section "Removing Containers"
  
  local compose_cmd
  compose_cmd="$(build_compose_command)"
  
  local service_args
  service_args="$(get_service_args)"
  
  if [[ "${DRY_RUN_MODE}" == true ]]; then
    log_info "[DRY RUN] Would execute: ${compose_cmd} rm -f ${service_args}"
  else
    log_info "Removing containers..."
    if bash -c "${compose_cmd} rm -f ${service_args}" 2>&1 | while IFS= read -r line; do
      log_debug "  ${line}"
    done; then
      log_success "Containers removed successfully"
    else
      log_warn "Failed to remove some containers"
    fi
  fi

  # Step 3: Remove images
  if ! remove_service_images; then
    log_warn "Some images could not be removed (continuing)"
  fi

  # Success message
  print_section "Prune Completed Successfully"
  log_success "Services pruned: containers stopped and removed, images deleted"
  
  if [[ -n "${LOG_FILE}" ]]; then
    log_info "Log file: ${LOG_FILE}"
  fi

  send_notification "success" "Docker services pruned successfully"

  local end_time
  end_time="$(date +%s)"
  local duration=$((end_time - SCRIPT_START_TIME))
  log_info "Total execution time: ${duration} seconds"
}

#==============================================================================
# MAIN FUNCTION
#==============================================================================

main() {
  # Parse command line arguments
  parse_arguments "${@}"

  # If no command provided, show help
  if [[ -z "${COMMAND}" ]]; then
    show_usage
    exit "${EXIT_SUCCESS}"
  fi

  # Resolve compose file path
  resolve_compose_file
  
  # Resolve environment file path
  resolve_env_file

  # Display script header
  log_info "Script: ${SCRIPT_NAME}"
  log_info "Command: ${COMMAND}"
  log_info "Started at: $(get_timestamp)"
  log_info "Environment tool: ${ENV_TOOL}"
  log_info "Compose file: ${COMPOSE_FILE_PATH}"
  log_info "Environment file: ${ENV_FILE_PATH}"
  if [[ -n "${ENVIRONMENT}" ]]; then
    log_info "Environment: ${ENVIRONMENT}"
  fi

  # Route to appropriate command handler
  case "${COMMAND}" in
  pull)
    cmd_pull
    ;;
  build)
    cmd_build
    ;;
  deploy)
    cmd_deploy
    ;;
  rebuild)
    cmd_rebuild
    ;;
  delete)
    cmd_delete
    ;;
  prune)
    cmd_prune
    ;;
  *)
    log_error "Unknown command: ${COMMAND}"
    log_error "Valid commands: pull, build, deploy, rebuild, delete, prune"
    show_usage
    exit "${EXIT_INVALID_ARGUMENT}"
    ;;
  esac
}

#==============================================================================
# SCRIPT ENTRY POINT
#==============================================================================

# Setup error handling and signal traps
setup_error_handling

# Run main function with all arguments
main "${@}"

# Exit successfully
exit "${EXIT_SUCCESS}"
