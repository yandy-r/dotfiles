#!/usr/bin/env bash
#
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#
# cleanup-zed.sh
#
# Prepares ~/.config/zed/ for Ansible copy-based management by clearing
# old symlinks at the managed destinations. Leaves real files untouched
# since they may contain local secrets (e.g. MCP server tokens).
#
# For each managed path, the script handles three cases:
#   1. Real file      -> leave alone (may contain local secrets)
#   2. Broken symlink -> remove (stale from old symlink approach)
#   3. Valid symlink   -> back up target, remove symlink (migration to copy)
#
# Usage:
#   ./bin/cleanup-zed.sh [--dry-run]
#   DRY_RUN=true ./bin/cleanup-zed.sh
#
# Options:
#   -n, --dry-run   Show what would be done without making changes
#   -h, --help      Show this help message

set -euo pipefail

# --- Configuration -----------------------------------------------------------

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly ZED_CONFIG_DIR="${HOME}/.config/zed"
TIMESTAMP="$(date '+%Y%m%d_%H%M%S')"
readonly TIMESTAMP

# Files managed as symlinks by the Ansible playbook (special_symlinks).
# If new entries are added to special_symlinks for zed, add them here too.
readonly MANAGED_FILES=(
  "${ZED_CONFIG_DIR}/settings.json"
)

# Dry-run: honour env var or CLI flag
DRY_RUN="${DRY_RUN:-false}"

# --- Helpers -----------------------------------------------------------------

usage() {
  cat << 'EOF'
Usage: cleanup-zed.sh [OPTIONS]

Prepares ~/.config/zed/ for Ansible symlink management by cleaning up
real files at managed symlink destinations.

Options:
  -n, --dry-run   Show what would be done without making changes
  -h, --help      Show this help message
EOF
  exit "${1:-0}"
}

log() {
  echo "[${SCRIPT_NAME}] $*"
}

dry_log() {
  echo "[${SCRIPT_NAME}] DRY-RUN: $*"
}

# --- Core logic: handle one path ---------------------------------------------

handle_path() {
  local target="$1"

  # Case 3: valid symlink — remove it (migrating from symlink to copy-based)
  if [[ -L "${target}" && -e "${target}" ]]; then
    local link_dest
    link_dest="$(readlink "${target}")"
    if [[ "${DRY_RUN}" == "true" ]]; then
      dry_log "Would remove symlink: ${target} -> ${link_dest}"
    else
      log "Removing symlink (migrating to copy): ${target} -> ${link_dest}"
      rm "${target}"
      log "  Removed. Ansible will deploy a fresh copy."
    fi
    return 0
  fi

  # Case 2: broken symlink (stale from old approach)
  if [[ -L "${target}" && ! -e "${target}" ]]; then
    if [[ "${DRY_RUN}" == "true" ]]; then
      dry_log "Would remove broken symlink: ${target}"
    else
      log "Removing broken symlink: ${target}"
      rm "${target}"
      log "  Removed."
    fi
    return 0
  fi

  # Case 1: real file — leave it alone (may contain local secrets)
  if [[ -f "${target}" ]]; then
    log "OK (real file, preserving): ${target}"
    return 0
  fi

  # Path does not exist at all — nothing to do
  log "Not present (nothing to do): ${target}"
}

# --- Main --------------------------------------------------------------------

main() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -n | --dry-run)
        DRY_RUN="true"
        shift
        ;;
      -h | --help)
        usage 0
        ;;
      *)
        echo "Unknown option: $1" >&2
        usage 1
        ;;
    esac
  done

  if [[ "${DRY_RUN}" == "true" ]]; then
    log "Running in DRY-RUN mode — no changes will be made."
  fi

  log "Checking Zed managed config files in: ${ZED_CONFIG_DIR}"

  for path in "${MANAGED_FILES[@]}"; do
    handle_path "${path}"
  done

  log "Done."
  if [[ "${DRY_RUN}" != "true" ]]; then
    log "You can now run: ansible-playbook ~/.config/dotfiles/playbooks/symlinks.yml"
  fi
}

main "$@"
