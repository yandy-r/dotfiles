#!/usr/bin/env bash
# gpu-switch.sh — Switch between NVIDIA and AMD GPU drivers on CachyOS
# Automates: https://wiki.cachyos.org/features/chwd/gpu_migration/
set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
readonly HYPR_DIR="${SCRIPT_DIR}/../hypr"
readonly STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/gpu-switch"

# ---------------------------------------------------------------------------
# Colors & logging
# ---------------------------------------------------------------------------
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m'

log()         { echo -e "${CYAN}[gpu-switch]${NC} $*" >&2; }
log_success() { echo -e "${GREEN}[gpu-switch] $*${NC}" >&2; }
log_warning() { echo -e "${YELLOW}[gpu-switch] $*${NC}" >&2; }
log_error()   { echo -e "${RED}[gpu-switch] $*${NC}" >&2; }
error_exit()  { log_error "$1"; exit "${2:-1}"; }

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
show_usage() {
  cat <<EOF
Usage: ${SCRIPT_NAME} <command> [OPTIONS]

Switch between NVIDIA and AMD GPU drivers on CachyOS.
Based on: https://wiki.cachyos.org/features/chwd/gpu_migration/

COMMANDS:
    nvidia          Switch to NVIDIA GPU drivers
    amd             Switch to AMD GPU drivers
    status          Show current GPU profile and driver state

OPTIONS:
    --no-reboot     Prepare switch but skip the reboot prompt
    -h, --help      Show this help

EXAMPLES:
    ${SCRIPT_NAME} status
    ${SCRIPT_NAME} amd
    ${SCRIPT_NAME} nvidia --no-reboot
EOF
  exit "${1:-0}"
}

# ---------------------------------------------------------------------------
# State helpers
# ---------------------------------------------------------------------------
ensure_state_dir() {
  mkdir -p "${STATE_DIR}"
}

read_state() {
  local key="$1" default="${2:-}"
  local file="${STATE_DIR}/${key}"
  if [[ -f "$file" ]]; then
    cat "$file"
  else
    printf '%s' "$default"
  fi
}

write_state() {
  local key="$1" value="$2"
  ensure_state_dir
  printf '%s\n' "$value" > "${STATE_DIR}/${key}"
}

# ---------------------------------------------------------------------------
# GPU detection
# ---------------------------------------------------------------------------
detect_gpu() {
  local vendor="$1"
  lspci 2>/dev/null | grep -iE "(vga|3d|display)" | grep -iq "$vendor"
}

get_installed_profiles() {
  sudo chwd --list-installed 2>/dev/null
}

get_nvidia_profile_name() {
  get_installed_profiles | grep -oE 'nvidia[a-z0-9_-]+' | head -1 || true
}

get_amd_profile_name() {
  get_installed_profiles | grep -oE '\bamd\b' | head -1 || true
}

# ---------------------------------------------------------------------------
# Marker-block config management
# ---------------------------------------------------------------------------
update_marker_block() {
  local file="$1" tag="$2" new_content="$3"
  local comment_prefix="${4:-#}"
  local start_marker="${comment_prefix} >>> ${tag}"
  local end_marker="${comment_prefix} <<< ${tag}"

  if ! grep -q "$start_marker" "$file"; then
    error_exit "Marker '${start_marker}' not found in ${file}."
  fi

  awk -v start="$start_marker" -v end_m="$end_marker" -v content="$new_content" '
    index($0, start) { print; printf "%s\n", content; skip=1; next }
    index($0, end_m)  { skip=0 }
    !skip              { print }
  ' "$file" > "${file}.tmp" && mv "${file}.tmp" "$file"
}

# ---------------------------------------------------------------------------
# Hyprland config templates
# ---------------------------------------------------------------------------
readonly NVIDIA_ENVS_LUA='hl.env("NVD_BACKEND", "direct")
hl.env("LIBVA_DRIVER_NAME", "nvidia")
hl.env("VDPAU_DRIVER", "nvidia")
hl.env("__GLX_VENDOR_LIBRARY_NAME", "nvidia")'

readonly AMD_ENVS_LUA='-- AMD GPU — mesa drivers handle everything automatically'

readonly NVIDIA_DBUS_LUA='  sync_desktop_session(hl)'

readonly AMD_DBUS_LUA='  sync_desktop_session(hl)'

apply_hypr_config() {
  local target="$1"  # "nvidia" or "amd"
  local envs_lua="${HYPR_DIR}/envs.lua"
  local autostart_lua="${HYPR_DIR}/autostart.lua"

  if [[ "$target" == "amd" ]]; then
    update_marker_block "$envs_lua" "GPU CONFIG" "$AMD_ENVS_LUA" "--"
    update_marker_block "$autostart_lua" "GPU DBUS" "$AMD_DBUS_LUA" "--"
  else
    update_marker_block "$envs_lua" "GPU CONFIG" "$NVIDIA_ENVS_LUA" "--"
    update_marker_block "$autostart_lua" "GPU DBUS" "$NVIDIA_DBUS_LUA" "--"
  fi
  log_success "Hyprland Lua configs updated for ${target}"
}

# ---------------------------------------------------------------------------
# Bootloader
# ---------------------------------------------------------------------------
update_bootloader() {
  log "Updating Limine bootloader config..."
  sudo limine-mkinitcpio
  log_success "Bootloader updated"
}

# ---------------------------------------------------------------------------
# Reboot prompt
# ---------------------------------------------------------------------------
prompt_reboot() {
  if [[ "${NO_REBOOT:-false}" == "true" ]]; then
    log_warning "Reboot skipped (--no-reboot). Reboot manually to apply changes."
    return 0
  fi

  if [[ -t 0 ]] && [[ -t 1 ]]; then
    echo -en "${YELLOW}[gpu-switch] Reboot now to apply changes? [y/N]: ${NC}" >&2
    read -r response
    if [[ "$response" =~ ^[yY]$ ]]; then
      log "Rebooting..."
      sudo reboot
    else
      log_warning "Reboot deferred. Reboot manually to apply changes."
    fi
  else
    log_warning "Non-interactive session. Reboot manually to apply changes."
  fi
}

# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------
cmd_status() {
  echo -e "${BLUE}=== GPU Switch Status ===${NC}" >&2

  log "Detected GPUs:"
  lspci 2>/dev/null | grep -iE "(vga|3d|display)" | sed 's/^/  /' >&2 || true
  echo >&2

  log "Installed chwd profiles:"
  get_installed_profiles 2>/dev/null | sed 's/^/  /' >&2 || log_warning "  Could not query chwd"
  echo >&2

  local current
  current="$(read_state current-gpu "unknown")"
  log "Saved state: ${current}"

  local saved_nvidia
  saved_nvidia="$(read_state nvidia-profile "not saved")"
  log "Saved NVIDIA profile: ${saved_nvidia}"
  echo >&2

  if command -v vulkaninfo &>/dev/null; then
    log "Vulkan device:"
    vulkaninfo --summary 2>/dev/null | grep -i "deviceName" | sed 's/^/  /' >&2 || true
  fi

  if command -v nvidia-smi &>/dev/null; then
    log "NVIDIA SMI:"
    nvidia-smi --query-gpu=name,driver_version --format=csv,noheader 2>/dev/null | sed 's/^/  /' >&2 || true
  fi
}

cmd_switch_amd() {
  log "Switching to AMD GPU drivers..."

  # Safety: AMD GPU must be visible (eGPU connected)
  if ! detect_gpu "amd\|radeon"; then
    error_exit "No AMD GPU detected. Is the eGPU dock connected?"
  fi

  # Already-on check
  local current
  current="$(read_state current-gpu "")"
  if [[ "$current" == "amd" ]]; then
    log_warning "Already configured for AMD. Use 'status' to verify."
    exit 0
  fi

  # Detect and save current NVIDIA profile
  local nvidia_profile
  nvidia_profile="$(get_nvidia_profile_name)"
  if [[ -z "$nvidia_profile" ]]; then
    log_warning "No NVIDIA profile found in chwd. Skipping profile removal."
  else
    log "Current NVIDIA profile: ${nvidia_profile}"
    write_state nvidia-profile "$nvidia_profile"

    log "Removing NVIDIA profile: ${nvidia_profile}..."
    sudo chwd -r "$nvidia_profile"
    log_success "NVIDIA profile removed"
  fi

  # Install AMD profile
  log "Installing AMD profile..."
  sudo chwd -i amd
  log_success "AMD profile installed"

  # Update Hyprland configs
  apply_hypr_config amd

  # Update bootloader
  update_bootloader

  # Save state
  write_state current-gpu amd
  log_success "GPU switch to AMD complete"

  prompt_reboot
}

cmd_switch_nvidia() {
  log "Switching to NVIDIA GPU drivers..."

  # Safety: NVIDIA GPU must be visible
  if ! detect_gpu "nvidia"; then
    error_exit "No NVIDIA GPU detected."
  fi

  # Already-on check
  local current
  current="$(read_state current-gpu "")"
  if [[ "$current" == "nvidia" ]]; then
    log_warning "Already configured for NVIDIA. Use 'status' to verify."
    exit 0
  fi

  # Remove AMD profile if installed
  local amd_profile
  amd_profile="$(get_amd_profile_name)"
  if [[ -z "$amd_profile" ]]; then
    log_warning "No AMD profile found in chwd. Skipping profile removal."
  else
    log "Removing AMD profile..."
    sudo chwd -r amd
    log_success "AMD profile removed"
  fi

  # Restore NVIDIA profile
  local nvidia_profile
  nvidia_profile="$(read_state nvidia-profile "nvidia-open-dkms")"
  log "Installing NVIDIA profile: ${nvidia_profile}..."
  sudo chwd -i "$nvidia_profile"
  log_success "NVIDIA profile installed"

  # Update Hyprland configs
  apply_hypr_config nvidia

  # Update bootloader
  update_bootloader

  # Save state
  write_state current-gpu nvidia
  log_success "GPU switch to NVIDIA complete"

  prompt_reboot
}

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
main() {
  local command=""
  NO_REBOOT="false"

  while [[ $# -gt 0 ]]; do
    case "$1" in
      nvidia|amd|status)
        command="$1"
        shift
        ;;
      --no-reboot)
        NO_REBOOT="true"
        shift
        ;;
      -h|--help)
        show_usage
        ;;
      *)
        error_exit "Unknown option: $1. Use --help for usage."
        ;;
    esac
  done

  if [[ -z "$command" ]]; then
    show_usage 1
  fi

  ensure_state_dir

  case "$command" in
    status)  cmd_status ;;
    amd)     cmd_switch_amd ;;
    nvidia)  cmd_switch_nvidia ;;
  esac
}

main "$@"
