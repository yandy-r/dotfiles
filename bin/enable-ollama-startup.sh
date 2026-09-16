#!/usr/bin/env bash
#
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#

set -euo pipefail

# ---------------------------------------------------------------------------
# Unified cross-platform Ollama headless startup configurator
# Supports macOS (LaunchAgent) and Linux (systemd)
# ---------------------------------------------------------------------------

readonly LOG_PREFIX="[OLLAMA-STARTUP]"
readonly LAUNCH_AGENT_LABEL="com.yandy.ollama.startup"
readonly SERVICE_NAME="ollama"
readonly SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# ── Defaults ──────────────────────────────────────────────────────────────
INSTALL_IF_MISSING="false"
LISTEN_ADDRESS=""
MODELS_DIR=""
KEEP_ALIVE=""
NUM_PARALLEL=""
MAX_LOADED_MODELS=""
FLASH_ATTENTION="false"
DEBUG_MODE="false"
ORIGINS=""
MAX_QUEUE=""

# ── macOS identity state ──────────────────────────────────────────────────
TARGET_USER=""
TARGET_UID=""
TARGET_GROUP=""
TARGET_HOME=""
LAUNCH_AGENT_DIR=""
LAUNCH_AGENT_PLIST=""
LOG_DIR=""
STDOUT_LOG=""
STDERR_LOG=""

# ── Shared utilities ─────────────────────────────────────────────────────

log() {
  echo "${LOG_PREFIX} $*" >&2
}

error_exit() {
  log "ERROR: $1"
  exit "${2:-1}"
}

has_command() {
  command -v "$1" >/dev/null 2>&1
}

# ── Help ──────────────────────────────────────────────────────────────────

show_usage() {
  local platform
  platform="$(uname -s)"

  cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Enable headless Ollama startup via LaunchAgent (macOS) or systemd (Linux).

OPTIONS:
  --install-if-missing             Install Ollama via official installer if not found
  --allow-network                  Bind to 0.0.0.0:11434 (accessible from other hosts)
  --listen-address HOST:PORT       Custom bind address (sets OLLAMA_HOST)
  --models-dir PATH                Custom model storage directory (OLLAMA_MODELS)
  --keep-alive DURATION            Model memory retention, e.g. 5m, 1h, 0 (OLLAMA_KEEP_ALIVE)
  --num-parallel N                 Concurrent requests per model (OLLAMA_NUM_PARALLEL)
  --max-loaded-models N            Max models loaded simultaneously (OLLAMA_MAX_LOADED_MODELS)
  --flash-attention                Enable flash attention GPU optimization (OLLAMA_FLASH_ATTENTION)
  --debug                          Enable debug logging (OLLAMA_DEBUG)
  --origins ORIGINS                CORS allowed origins, comma-separated (OLLAMA_ORIGINS)
  --max-queue N                    Max queued requests before rejecting (OLLAMA_MAX_QUEUE)
  -h, --help                       Show this help message
EOF

  if [[ "$platform" == "Darwin" ]]; then
    cat <<EOF

macOS details:
  Creates: ~/Library/LaunchAgents/${LAUNCH_AGENT_LABEL}.plist
  Loads the LaunchAgent into launchd and runs 'ollama serve' immediately.

  Run as normal user (recommended) or via sudo (targets original sudo user).

  Disable later with:
    launchctl bootout gui/\$(id -u)/${LAUNCH_AGENT_LABEL}
    rm -f ~/Library/LaunchAgents/${LAUNCH_AGENT_LABEL}.plist
EOF
  else
    cat <<EOF

Linux details:
  Creates: ${SERVICE_FILE}
  Enables and starts the ollama systemd service.

  Disable later with:
    sudo systemctl disable --now ollama
EOF
  fi
}

# ── Argument parsing ─────────────────────────────────────────────────────

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
    --install-if-missing)
      INSTALL_IF_MISSING="true"
      shift
      ;;
    --allow-network)
      LISTEN_ADDRESS="0.0.0.0:11434"
      shift
      ;;
    --listen-address)
      [[ $# -lt 2 ]] && error_exit "--listen-address requires a value (example: 0.0.0.0:11434)"
      LISTEN_ADDRESS="$2"
      shift 2
      ;;
    --models-dir)
      [[ $# -lt 2 ]] && error_exit "--models-dir requires a path"
      MODELS_DIR="$2"
      shift 2
      ;;
    --keep-alive)
      [[ $# -lt 2 ]] && error_exit "--keep-alive requires a duration (e.g. 5m, 1h, 0)"
      KEEP_ALIVE="$2"
      shift 2
      ;;
    --num-parallel)
      [[ $# -lt 2 ]] && error_exit "--num-parallel requires a number"
      NUM_PARALLEL="$2"
      shift 2
      ;;
    --max-loaded-models)
      [[ $# -lt 2 ]] && error_exit "--max-loaded-models requires a number"
      MAX_LOADED_MODELS="$2"
      shift 2
      ;;
    --flash-attention)
      FLASH_ATTENTION="true"
      shift
      ;;
    --debug)
      DEBUG_MODE="true"
      shift
      ;;
    --origins)
      [[ $# -lt 2 ]] && error_exit "--origins requires a value"
      ORIGINS="$2"
      shift 2
      ;;
    --max-queue)
      [[ $# -lt 2 ]] && error_exit "--max-queue requires a number"
      MAX_QUEUE="$2"
      shift 2
      ;;
    -h | --help)
      show_usage
      exit 0
      ;;
    *)
      error_exit "Unknown option: $1"
      ;;
    esac
  done
}

# ── Environment variable builder ─────────────────────────────────────────
# Outputs KEY=VALUE lines consumed by both macOS and Linux writers.

build_env_vars() {
  local platform="$1"

  if [[ "$platform" == "Darwin" ]]; then
    echo "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
  else
    echo "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
  fi

  [[ -n "$LISTEN_ADDRESS" ]]   && echo "OLLAMA_HOST=${LISTEN_ADDRESS}"
  [[ -n "$MODELS_DIR" ]]       && echo "OLLAMA_MODELS=${MODELS_DIR}"
  [[ -n "$KEEP_ALIVE" ]]       && echo "OLLAMA_KEEP_ALIVE=${KEEP_ALIVE}"
  [[ -n "$NUM_PARALLEL" ]]     && echo "OLLAMA_NUM_PARALLEL=${NUM_PARALLEL}"
  [[ -n "$MAX_LOADED_MODELS" ]] && echo "OLLAMA_MAX_LOADED_MODELS=${MAX_LOADED_MODELS}"
  [[ -n "$ORIGINS" ]]          && echo "OLLAMA_ORIGINS=${ORIGINS}"
  [[ -n "$MAX_QUEUE" ]]        && echo "OLLAMA_MAX_QUEUE=${MAX_QUEUE}"
  [[ "$FLASH_ATTENTION" == "true" ]] && echo "OLLAMA_FLASH_ATTENTION=1"
  [[ "$DEBUG_MODE" == "true" ]]      && echo "OLLAMA_DEBUG=1"

  return 0
}

# ── Binary resolution ────────────────────────────────────────────────────

resolve_ollama_binary_macos() {
  local bin_path
  bin_path="$(command -v ollama 2>/dev/null || true)"
  if [[ -n "$bin_path" && -x "$bin_path" ]]; then
    echo "$bin_path"
    return 0
  fi

  local app_bin="/Applications/Ollama.app/Contents/Resources/ollama"
  if [[ -x "$app_bin" ]]; then
    echo "$app_bin"
    return 0
  fi

  return 1
}

resolve_ollama_binary_linux() {
  local bin_path
  bin_path="$(command -v ollama 2>/dev/null || true)"
  if [[ -n "$bin_path" && -x "$bin_path" ]]; then
    echo "$bin_path"
    return 0
  fi

  for candidate in /usr/local/bin/ollama /usr/bin/ollama /bin/ollama; do
    if [[ -x "$candidate" ]]; then
      echo "$candidate"
      return 0
    fi
  done

  return 1
}

resolve_ollama_binary() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    resolve_ollama_binary_macos
  else
    resolve_ollama_binary_linux
  fi
}

ensure_ollama_installed() {
  local ollama_bin
  ollama_bin="$(resolve_ollama_binary)" || ollama_bin=""

  if [[ -n "$ollama_bin" ]]; then
    log "Found Ollama binary: $ollama_bin"
    echo "$ollama_bin"
    return 0
  fi

  if [[ "$INSTALL_IF_MISSING" != "true" ]]; then
    error_exit "Ollama is not installed. Re-run with --install-if-missing"
  fi

  if ! has_command curl; then
    error_exit "curl is required to install Ollama"
  fi

  log "Ollama not found, running official installer"
  curl -fsSL https://ollama.com/install.sh | sh || true

  ollama_bin="$(resolve_ollama_binary)" || error_exit "Ollama still not found after installation"
  log "Installed Ollama binary: $ollama_bin"
  echo "$ollama_bin"
}

# ── macOS functions ──────────────────────────────────────────────────────

resolve_target_identity() {
  if [[ "${EUID}" -eq 0 ]]; then
    if [[ -z "${SUDO_USER:-}" ]]; then
      error_exit "When running as root, invoke via sudo from your user account"
    fi
    TARGET_USER="${SUDO_USER}"
  else
    TARGET_USER="${USER:-$(id -un)}"
  fi

  TARGET_UID="$(id -u "${TARGET_USER}")" || error_exit "Unable to resolve UID for ${TARGET_USER}"
  TARGET_GROUP="$(id -gn "${TARGET_USER}")" || error_exit "Unable to resolve group for ${TARGET_USER}"

  TARGET_HOME="$(dscl . -read "/Users/${TARGET_USER}" NFSHomeDirectory 2>/dev/null | awk '{print $2}' || true)"
  if [[ -z "${TARGET_HOME}" || ! -d "${TARGET_HOME}" ]]; then
    TARGET_HOME="$(eval echo "~${TARGET_USER}")"
  fi
  if [[ -z "${TARGET_HOME}" || ! -d "${TARGET_HOME}" ]]; then
    error_exit "Unable to resolve home directory for ${TARGET_USER}"
  fi

  LAUNCH_AGENT_DIR="${TARGET_HOME}/Library/LaunchAgents"
  LAUNCH_AGENT_PLIST="${LAUNCH_AGENT_DIR}/${LAUNCH_AGENT_LABEL}.plist"
  LOG_DIR="${TARGET_HOME}/Library/Logs"
  STDOUT_LOG="${LOG_DIR}/${LAUNCH_AGENT_LABEL}.out.log"
  STDERR_LOG="${LOG_DIR}/${LAUNCH_AGENT_LABEL}.err.log"
}

write_launchagent_plist() {
  local ollama_bin="$1"

  mkdir -p "${LAUNCH_AGENT_DIR}" "${LOG_DIR}"

  # Build environment variable XML entries from build_env_vars output
  local env_xml=""
  while IFS='=' read -r key value; do
    env_xml+="    <key>${key}</key>"$'\n'
    env_xml+="    <string>${value}</string>"$'\n'
  done < <(build_env_vars "Darwin")

  # Always include HOME
  env_xml+="    <key>HOME</key>"$'\n'
  env_xml+="    <string>${TARGET_HOME}</string>"$'\n'

  cat > "${LAUNCH_AGENT_PLIST}" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>${LAUNCH_AGENT_LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>${ollama_bin}</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ProcessType</key>
  <string>Background</string>
  <key>EnvironmentVariables</key>
  <dict>
${env_xml}  </dict>
  <key>StandardOutPath</key>
  <string>${STDOUT_LOG}</string>
  <key>StandardErrorPath</key>
  <string>${STDERR_LOG}</string>
</dict>
</plist>
EOF

  if [[ "${EUID}" -eq 0 ]]; then
    chown "${TARGET_USER}:${TARGET_GROUP}" "${LAUNCH_AGENT_PLIST}"
  fi
  chmod 644 "${LAUNCH_AGENT_PLIST}"

  if has_command plutil; then
    plutil -lint "${LAUNCH_AGENT_PLIST}" >/dev/null
  fi
}

load_launchagent() {
  local launch_domain="gui/${TARGET_UID}"

  if launchctl print "${launch_domain}/${LAUNCH_AGENT_LABEL}" >/dev/null 2>&1; then
    log "Existing LaunchAgent detected, reloading"
    launchctl bootout "${launch_domain}/${LAUNCH_AGENT_LABEL}" >/dev/null 2>&1 || \
      launchctl bootout "${launch_domain}" "${LAUNCH_AGENT_PLIST}" >/dev/null 2>&1 || true
  fi

  local launchctl_err
  launchctl_err="$(mktemp)"
  if ! launchctl bootstrap "${launch_domain}" "${LAUNCH_AGENT_PLIST}" 2>"${launchctl_err}"; then
    local err_text
    err_text="$(cat "${launchctl_err}" 2>/dev/null || true)"
    rm -f "${launchctl_err}"
    if [[ "${EUID}" -ne 0 ]]; then
      error_exit "launchctl bootstrap failed. Re-run with sudo. Details: ${err_text}"
    fi
    error_exit "launchctl bootstrap failed. Details: ${err_text}"
  fi
  rm -f "${launchctl_err}"

  launchctl enable "${launch_domain}/${LAUNCH_AGENT_LABEL}" >/dev/null 2>&1 || true
  launchctl kickstart -k "${launch_domain}/${LAUNCH_AGENT_LABEL}" >/dev/null 2>&1 || true
}

setup_macos() {
  resolve_target_identity

  local ollama_bin
  ollama_bin="$(ensure_ollama_installed)"

  write_launchagent_plist "$ollama_bin"
  load_launchagent

  local listen="${LISTEN_ADDRESS:-127.0.0.1:11434}"
  log "Enabled headless Ollama startup at login"
  log "Target user: ${TARGET_USER} (${TARGET_UID})"
  log "Using binary: ${ollama_bin}"
  log "Listening on: ${listen}"
  if [[ "${listen}" == 0.0.0.0:* ]]; then
    log "Network access enabled. Ensure macOS firewall allows incoming connections for Ollama."
  fi
  log "LaunchAgent: ${LAUNCH_AGENT_PLIST}"
}

# ── Linux functions ──────────────────────────────────────────────────────

detect_linux_family() {
  if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    source /etc/os-release
    case "${ID:-unknown}" in
    ubuntu | debian) echo "debian" ;;
    fedora | rhel | centos | rocky | almalinux) echo "redhat" ;;
    arch | cachyos | manjaro | endeavouros) echo "arch" ;;
    opensuse* | sles) echo "suse" ;;
    *)
      case "${ID_LIKE:-}" in
      *debian* | *ubuntu*) echo "debian" ;;
      *fedora* | *rhel*) echo "redhat" ;;
      *arch*) echo "arch" ;;
      *suse*) echo "suse" ;;
      *) echo "linux" ;;
      esac
      ;;
    esac
    return 0
  fi

  if [[ -f /proc/version ]] && grep -qi microsoft /proc/version; then
    echo "wsl"
    return 0
  fi

  echo "linux"
}

ensure_systemd_available() {
  if ! has_command systemctl; then
    error_exit "systemctl not found. systemd is required for this script"
  fi

  local system_state
  system_state="$(systemctl is-system-running 2>/dev/null || true)"
  case "$system_state" in
  running | degraded)
    return 0
    ;;
  *)
    error_exit "systemd is not running (state: ${system_state:-unknown})"
    ;;
  esac
}

create_ollama_user() {
  if ! id ollama >/dev/null 2>&1; then
    sudo useradd -r -s /bin/false -U -m -d /usr/share/ollama ollama
  fi

  if getent group render >/dev/null 2>&1; then
    sudo usermod -a -G render ollama
  fi
  if getent group video >/dev/null 2>&1; then
    sudo usermod -a -G video ollama
  fi

  local login_user="${SUDO_USER:-${USER:-}}"
  if [[ -n "$login_user" ]] && id "$login_user" >/dev/null 2>&1; then
    sudo usermod -a -G ollama "$login_user" || true
  fi
}

ensure_models_dir_accessible() {
  [[ -z "$MODELS_DIR" ]] && return 0

  if [[ ! -d "$MODELS_DIR" ]]; then
    log "Creating models directory: ${MODELS_DIR}"
    sudo mkdir -p "$MODELS_DIR"
  fi

  log "Setting models directory ownership to ollama:ollama"
  sudo chown -R ollama:ollama "$MODELS_DIR"
}

write_systemd_service() {
  local ollama_bin="$1"

  # Build Environment= lines from build_env_vars output
  local env_lines=""
  while IFS='=' read -r key value; do
    env_lines+="Environment=\"${key}=${value}\""$'\n'
  done < <(build_env_vars "Linux")

  log "Writing ${SERVICE_FILE}"
  sudo tee "$SERVICE_FILE" >/dev/null <<EOF
[Unit]
Description=Ollama Service
After=network-online.target

[Service]
ExecStart=${ollama_bin} serve
User=ollama
Group=ollama
Restart=always
RestartSec=3
${env_lines}
[Install]
WantedBy=default.target
EOF
}

enable_and_start_service() {
  sudo systemctl daemon-reload
  sudo systemctl enable "$SERVICE_NAME"
  sudo systemctl restart "$SERVICE_NAME"

  if ! sudo systemctl is-active --quiet "$SERVICE_NAME"; then
    error_exit "Service enabled but not active. Check: sudo systemctl status ${SERVICE_NAME}"
  fi

  log "Ollama service is enabled and active"
}

setup_linux() {
  local distro_family
  distro_family="$(detect_linux_family)"
  log "Detected Linux family: ${distro_family}"

  ensure_systemd_available

  local ollama_bin
  ollama_bin="$(ensure_ollama_installed)"

  create_ollama_user
  ensure_models_dir_accessible
  write_systemd_service "$ollama_bin"
  enable_and_start_service

  local listen="${LISTEN_ADDRESS:-127.0.0.1:11434}"
  log "Done. Ollama API should be available at http://${listen}"
  log "Check status with: sudo systemctl status ${SERVICE_NAME}"
}

# ── Main ─────────────────────────────────────────────────────────────────

main() {
  parse_args "$@"

  case "$(uname -s)" in
  Darwin) setup_macos ;;
  Linux)  setup_linux ;;
  *)      error_exit "Unsupported platform: $(uname -s). Only macOS and Linux are supported." ;;
  esac
}

main "$@"
