#!/usr/bin/env bash

# Install, update, or uninstall the 1Password CLI (`op`).
# Upstream: https://www.1password.dev/cli
#
#   macOS    Homebrew cask (1password-cli, or the 1password-cli@beta cask
#            bin/install-brew.sh installs, when that is the one present)
#   Linux    official release zip from the 1Password CDN, GPG-verified against
#            the pinned 1Password signing key, installed to /usr/local/bin/op
#            with the setgid `onepassword-cli` group the desktop-app integration
#            requires (debian, redhat, arch, suse, generic linux, WSL)
#   Windows  winget (AgileBits.1Password.CLI)

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[1PASSWORD-CLI-INSTALL]"
readonly OP_INSTALL_PATH="/usr/local/bin/op"
readonly OP_GROUP="onepassword-cli"
readonly OP_KEY_URL="https://downloads.1password.com/linux/keys/1password.asc"
readonly OP_KEY_FINGERPRINT="3FEF9748469ADBE15DA7CA80AC2D62742012EA22"
readonly OP_VERSION_URL="https://app-updates.agilebits.com/check/1/0/CLI2/en/2.0.0/N"
readonly OP_DIST_URL="https://cache.agilebits.com/dist/1P/op2/pkg"
readonly OP_WINGET_ID="AgileBits.1Password.CLI"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_DIR="${DOTFILES_DIR:-$(dirname "$SCRIPT_DIR")}"

ACTION="${ACTION:-install}"
OP_VERSION="${OP_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall the 1Password CLI (op).

OPTIONS:
    --install           Install the 1Password CLI (default)
    --update            Update to the latest (or --version) release
    --uninstall         Uninstall the 1Password CLI
    --version VERSION   Install a specific version, e.g. 2.39.0 (default: latest)
    --force             Reinstall even when op is already installed
    --dry-run           Print actions without changing anything
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION              install, update, or uninstall
    OP_VERSION          Version to install (default: latest)
    FORCE_INSTALL       Reinstall even when installed (true/false)
    NON_INTERACTIVE     Auto-approve prompts (true/false)
    DRY_RUN             Print actions only (true/false)

INSTALL METHODS:
    macOS     brew install --cask 1password-cli
    Linux     GPG-verified release zip -> $OP_INSTALL_PATH (sudo)
    Windows   winget install $OP_WINGET_ID

After installing, turn on Settings > Developer > "Integrate with 1Password CLI"
in the 1Password app, or export OP_SERVICE_ACCOUNT_TOKEN for headless use.
EOF
}

log() {
  printf '%s %s\n' "$LOG_PREFIX" "$*" >&2
}

error_exit() {
  log "ERROR: $1"
  exit "${2:-1}"
}

run() {
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '%s DRY-RUN:' "$LOG_PREFIX" >&2
    printf ' %q' "$@" >&2
    printf '\n' >&2
    return 0
  fi
  "$@"
}

# Run with root privileges: directly as root, otherwise via sudo.
run_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    run "$@"
  else
    command -v sudo >/dev/null 2>&1 || error_exit "sudo is required to run: $*"
    run sudo "$@"
  fi
}

is_interactive() {
  [[ -t 0 ]] && [[ -t 1 ]]
}

prompt_confirmation() {
  local prompt_message="$1"
  local response

  # Auto-approve in non-interactive mode
  if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${FORCE_INSTALL:-false}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]; then
    log "Non-interactive mode: auto-approving prompt"
    return 0
  fi

  if ! is_interactive; then
    log "ERROR: Non-interactive shell detected - cannot prompt for confirmation"
    log "Set NON_INTERACTIVE=true or FORCE_INSTALL=true to proceed automatically"
    return 1
  fi

  printf '%s %s [y/N]: ' "$LOG_PREFIX" "$prompt_message" >&2
  read -r response
  case "${response:-n}" in
  [yY] | [yY][eE][sS]) return 0 ;;
  *) return 1 ;;
  esac
}

load_platform_detection() {
  if [[ -n "${DOTFILES_OS:-}" ]] && [[ -n "${DOTFILES_ARCH:-}" ]]; then
    return 0
  fi
  local platform_script="$DOTFILES_DIR/common/platform.sh"
  [[ -f "$platform_script" ]] || error_exit "Platform detection script not found at: $platform_script"
  # shellcheck source=../common/platform.sh
  source "$platform_script"
  [[ -n "${DOTFILES_OS:-}" ]] || error_exit "Platform detection failed - DOTFILES_OS not set"
}

require_commands() {
  local cmd
  for cmd in "$@"; do
    command -v "$cmd" >/dev/null 2>&1 || error_exit "$cmd is required but not installed"
  done
}

installed_version() {
  op --version 2>/dev/null || true
}

# 1Password release-zip arch token for the host CPU.
op_arch() {
  case "${DOTFILES_ARCH:-$(uname -m)}" in
  x86_64 | amd64) echo "amd64" ;;
  arm64 | aarch64) echo "arm64" ;;
  arm | armv7l | armv6l) echo "arm" ;;
  i386 | i686) echo "386" ;;
  *) error_exit "Unsupported architecture: ${DOTFILES_ARCH:-$(uname -m)}" ;;
  esac
}

# Resolve OP_VERSION to a concrete x.y.z. The value lands in a download URL, so
# anything that is not a plain version is rejected.
resolve_version() {
  local version="$OP_VERSION"
  if [[ "$version" == "latest" ]]; then
    version="$(curl -fsSL "$OP_VERSION_URL" 2>/dev/null |
      grep -o '"version"[[:space:]]*:[[:space:]]*"[^"]*"' |
      head -1 |
      sed 's/.*"\([^"]*\)"$/\1/')" || true
    [[ -n "$version" ]] || error_exit "Failed to resolve the latest 1Password CLI version"
  fi
  version="${version#v}"
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || error_exit "Invalid 1Password CLI version: $version"
  echo "$version"
}

# Name of the Homebrew cask that provides op, if any.
brew_cask() {
  command -v brew >/dev/null 2>&1 || return 1
  local cask
  for cask in 1password-cli 1password-cli@beta; do
    if brew list --cask "$cask" >/dev/null 2>&1; then
      echo "$cask"
      return 0
    fi
  done
  return 1
}

winget_cmd() {
  local cmd
  for cmd in winget winget.exe; do
    if command -v "$cmd" >/dev/null 2>&1; then
      echo "$cmd"
      return 0
    fi
  done
  return 1
}

# ---------------------------------------------------------------- macOS

install_macos() {
  require_commands brew
  run brew install --cask 1password-cli
}

update_macos() {
  local cask
  if cask="$(brew_cask)"; then
    run brew upgrade --cask "$cask" || log "brew reports $cask is already up to date"
  else
    install_macos
  fi
}

uninstall_macos() {
  local cask
  if cask="$(brew_cask)"; then
    run brew uninstall --cask "$cask"
  else
    uninstall_binary
  fi
}

# ---------------------------------------------------------------- Linux

install_linux() {
  require_commands curl unzip gpg
  local version arch zip_url temp_dir
  version="$(resolve_version)"
  arch="$(op_arch)"
  zip_url="$OP_DIST_URL/v${version}/op_linux_${arch}_v${version}.zip"

  log "Installing 1Password CLI v${version} (linux/${arch})"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would download $zip_url, verify op.sig against $OP_KEY_FINGERPRINT,"
    log "DRY-RUN: install to $OP_INSTALL_PATH and set group $OP_GROUP (setgid)"
    return 0
  fi

  if ! prompt_confirmation "Install op to $OP_INSTALL_PATH with sudo?"; then
    log "Installation cancelled by user"
    exit 0
  fi

  temp_dir="$(mktemp -d)"
  # shellcheck disable=SC2064 # expand now: temp_dir is local
  trap "rm -rf '$temp_dir'" EXIT

  log "Downloading $zip_url"
  curl -fsSL "$zip_url" -o "$temp_dir/op.zip" || error_exit "Failed to download $zip_url"
  unzip -q "$temp_dir/op.zip" op op.sig -d "$temp_dir/op" || error_exit "Failed to extract op.zip"

  # Verify with a throwaway keyring holding only the 1Password signing key, and
  # require a VALIDSIG from the pinned primary fingerprint.
  local gnupg="$temp_dir/gnupg"
  mkdir -m 700 "$gnupg"
  curl -fsSL "$OP_KEY_URL" | GNUPGHOME="$gnupg" gpg --batch --quiet --import 2>/dev/null ||
    error_exit "Failed to import the 1Password signing key"
  if ! GNUPGHOME="$gnupg" gpg --batch --status-fd 1 --verify "$temp_dir/op/op.sig" "$temp_dir/op/op" 2>/dev/null |
    grep -q "^\[GNUPG:\] VALIDSIG .* ${OP_KEY_FINGERPRINT}\$"; then
    error_exit "Signature verification failed for op v${version} - refusing to install"
  fi
  log "Signature verified (1Password key $OP_KEY_FINGERPRINT)"

  run_root install -d -m 755 "$(dirname "$OP_INSTALL_PATH")"
  run_root install -m 755 "$temp_dir/op/op" "$OP_INSTALL_PATH"
  run_root groupadd -f "$OP_GROUP"
  run_root chgrp "$OP_GROUP" "$OP_INSTALL_PATH"
  run_root chmod g+s "$OP_INSTALL_PATH"
}

# ---------------------------------------------------------------- Windows

install_windows() {
  local winget
  winget="$(winget_cmd)" || error_exit "winget not found - install manually: https://www.1password.dev/cli"
  run "$winget" install --exact --id "$OP_WINGET_ID" --accept-source-agreements --accept-package-agreements
}

update_windows() {
  local winget
  winget="$(winget_cmd)" || error_exit "winget not found - update manually: https://www.1password.dev/cli"
  run "$winget" upgrade --exact --id "$OP_WINGET_ID" --accept-source-agreements --accept-package-agreements ||
    log "winget reports $OP_WINGET_ID is already up to date"
}

uninstall_windows() {
  local winget
  winget="$(winget_cmd)" || error_exit "winget not found - uninstall manually"
  run "$winget" uninstall --exact --id "$OP_WINGET_ID"
}

# ---------------------------------------------------------------- shared

uninstall_binary() {
  if [[ ! -e "$OP_INSTALL_PATH" ]]; then
    local found
    found="$(command -v op 2>/dev/null || true)"
    if [[ -n "$found" ]]; then
      error_exit "op at $found was not installed by this script - remove it with the package manager that owns it"
    fi
    log "1Password CLI is not installed"
    return 0
  fi
  if ! prompt_confirmation "Remove $OP_INSTALL_PATH with sudo?"; then
    log "Uninstall cancelled by user"
    exit 0
  fi
  # The onepassword-cli group is left in place: the 1Password desktop app on
  # Linux creates and uses the same group.
  run_root rm -f "$OP_INSTALL_PATH"
}

# On Linux, never overwrite an op this script does not own (e.g. one from the
# 1Password apt/dnf repo or the AUR), which would leave two copies on PATH.
linux_owned() {
  local found
  found="$(command -v op 2>/dev/null || true)"
  # -ef, not string equality: Fedora links /usr/local/sbin to /usr/local/bin.
  if [[ -n "$found" ]] && ! [[ "$found" -ef "$OP_INSTALL_PATH" ]]; then
    log "op at $found is managed elsewhere - update it with the package manager that owns it"
    return 1
  fi
}

verify_installation() {
  [[ "$DRY_RUN" == "true" ]] && return 0
  hash -r
  command -v op >/dev/null 2>&1 || error_exit "op is installed but not on PATH - restart your shell"
  log "1Password CLI verified: $(installed_version) ($(command -v op))"
}

do_install() {
  local current
  current="$(installed_version)"
  if [[ -n "$current" ]] && [[ "$FORCE_INSTALL" != "true" ]]; then
    log "1Password CLI is already installed: $current (use --update or --force)"
    return 0
  fi
  case "$DOTFILES_OS" in
  macos) install_macos ;;
  windows) install_windows ;;
  debian | redhat | arch | suse | linux | wsl)
    if [[ -n "$current" ]] && ! linux_owned; then
      error_exit "refusing to install a second op"
    fi
    install_linux
    ;;
  *) error_exit "Unsupported operating system: $DOTFILES_OS" ;;
  esac
  verify_installation
}

do_update() {
  local current
  current="$(installed_version)"
  if [[ -z "$current" ]]; then
    log "1Password CLI is not installed - installing"
    do_install
    return 0
  fi
  case "$DOTFILES_OS" in
  macos) update_macos ;;
  windows) update_windows ;;
  debian | redhat | arch | suse | linux | wsl)
    linux_owned || return 0
    local target
    target="$(resolve_version)"
    if [[ "$current" == "$target" ]] && [[ "$FORCE_INSTALL" != "true" ]]; then
      log "1Password CLI is up to date: $current"
      return 0
    fi
    log "Updating 1Password CLI $current -> $target"
    OP_VERSION="$target" install_linux
    ;;
  *) error_exit "Unsupported operating system: $DOTFILES_OS" ;;
  esac
  verify_installation
}

do_uninstall() {
  case "$DOTFILES_OS" in
  macos) uninstall_macos ;;
  windows) uninstall_windows ;;
  debian | redhat | arch | suse | linux | wsl) uninstall_binary ;;
  *) error_exit "Unsupported operating system: $DOTFILES_OS" ;;
  esac
  [[ "$DRY_RUN" == "true" ]] || log "1Password CLI uninstalled"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
    --install) ACTION="install" ;;
    --update) ACTION="update" ;;
    --uninstall) ACTION="uninstall" ;;
    --version)
      [[ $# -ge 2 ]] || error_exit "--version requires a value"
      OP_VERSION="$2"
      shift
      ;;
    --force) FORCE_INSTALL="true" ;;
    --dry-run) DRY_RUN="true" ;;
    -h | --help)
      usage
      exit 0
      ;;
    *) error_exit "Unknown option: $1. Use --help for usage information." ;;
    esac
    shift
  done
}

main() {
  parse_args "$@"
  load_platform_detection

  case "$ACTION" in
  install) do_install ;;
  update) do_update ;;
  uninstall) do_uninstall ;;
  *) error_exit "Invalid action: $ACTION. Use --help for usage information." ;;
  esac
}

main "$@"
