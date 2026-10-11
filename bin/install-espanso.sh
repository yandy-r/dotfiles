#!/usr/bin/env bash

# Install, update, or remove espanso, the cross-platform text expander.
# Upstream: https://espanso.org/docs/install/
#
#   macOS    Homebrew cask (espanso)
#   Linux    picks the upstream-recommended method for the distro and display
#            server (X11 or Wayland, auto-detected):
#              debian (amd64)   official .deb from GitHub Releases
#              fedora           Terra repository (espanso-x11 / espanso-wayland)
#              arch             AUR (espanso-x11 / espanso-wayland) via paru/yay
#              other, X11       Espanso-X11.AppImage (x86_64)
#              anything else    build from source with cargo
#            Wayland builds get cap_dac_override so espanso can read input
#            devices, and the systemd user service is registered and started.
#   WSL      the Windows build via winget.exe (espanso cannot hook keyboards
#            from inside WSL)

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[ESPANSO-INSTALL]"
readonly ESPANSO_REPO_URL="https://github.com/espanso/espanso"
readonly ESPANSO_SOURCE_PATH="/usr/local/bin/espanso"
readonly ESPANSO_WINGET_ID="Espanso.Espanso"
# shellcheck disable=SC2016 # $releasever is expanded by dnf, not the shell
readonly TERRA_REPO_URL='https://repos.fyralabs.com/terra$releasever'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_DIR="${DOTFILES_DIR:-$(dirname "$SCRIPT_DIR")}"

ACTION="${ACTION:-install}"
ESPANSO_VERSION="${ESPANSO_VERSION:-latest}"
ESPANSO_BACKEND="${ESPANSO_BACKEND:-}"
ESPANSO_METHOD="${ESPANSO_METHOD:-}"
ESPANSO_APPIMAGE_PATH="${ESPANSO_APPIMAGE_PATH:-$HOME/opt/Espanso.AppImage}"
SKIP_SERVICE="${SKIP_SERVICE:-false}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"
TEMP_DIR=""
UPDATED="true"

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or remove espanso (text expander).

OPTIONS:
    --install           Install espanso (default)
    --update            Update espanso to the latest (or --version) release
    --remove            Stop, unregister, and remove espanso (alias: --uninstall)
    --x11               Force the X11 build (Linux; default: auto-detect)
    --wayland           Force the Wayland build (Linux; default: auto-detect)
    --method METHOD     Force an install method: deb, terra, aur, appimage, source
    --version VERSION   Release to install, e.g. 2.4.1 (deb/appimage/source only)
    --no-service        Do not register/start the espanso service
    --force             Reinstall even when espanso is already installed
    --dry-run           Print actions without changing anything
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION                 install, update, or remove
    ESPANSO_BACKEND        x11 or wayland (overrides detection)
    ESPANSO_METHOD         Install method (see --method)
    ESPANSO_VERSION        Version to install (default: latest)
    ESPANSO_APPIMAGE_PATH  AppImage location (default: ~/opt/Espanso.AppImage)
    SKIP_SERVICE           Skip service registration (true/false)
    FORCE_INSTALL          Reinstall even when installed (true/false)
    NON_INTERACTIVE        Auto-approve prompts (true/false)
    DRY_RUN                Print actions only (true/false)

Configuration (~/.config/espanso, or ~/Library/Application Support/espanso on
macOS) is never touched by --remove.
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

is_non_interactive() {
  [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]
}

prompt_confirmation() {
  local prompt_message="$1"
  local response

  if is_non_interactive || [[ "$DRY_RUN" == "true" ]]; then
    return 0
  fi
  if ! is_interactive; then
    log "ERROR: Non-interactive shell detected - cannot prompt for confirmation"
    log "Set NON_INTERACTIVE=true to proceed automatically"
    return 1
  fi

  printf '%s %s [y/N]: ' "$LOG_PREFIX" "$prompt_message" >&2
  read -r response
  case "${response:-n}" in
  [yY] | [yY][eE][sS]) return 0 ;;
  *) return 1 ;;
  esac
}

confirm_or_exit() {
  prompt_confirmation "$1" || {
    log "Cancelled by user"
    exit 0
  }
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

make_temp_dir() {
  [[ -n "$TEMP_DIR" ]] && return 0
  TEMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$TEMP_DIR"' EXIT
}

is_linux() {
  case "$DOTFILES_OS" in
  debian | redhat | arch | suse | linux) return 0 ;;
  *) return 1 ;;
  esac
}

is_amd64() {
  case "${DOTFILES_ARCH:-$(uname -m)}" in
  x86_64 | amd64) return 0 ;;
  *) return 1 ;;
  esac
}

is_fedora() {
  [[ -r /etc/os-release ]] && grep -qE '^ID="?fedora"?$' /etc/os-release
}

# Some releases print nothing for --version without a display, so fall back to
# the owning package's version.
installed_version() {
  command -v espanso >/dev/null 2>&1 || return 0
  local version method pkg
  version="$(espanso --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || true)"
  if [[ -z "$version" ]]; then
    read -r method pkg <<<"$(detect_installed_method)"
    case "$method" in
    deb) version="$(dpkg-query -W -f='${Version}' "$pkg" 2>/dev/null || true)" ;;
    aur) version="$(pacman -Q "$pkg" 2>/dev/null | awk '{print $2}' || true)" ;;
    terra) version="$(rpm -q --qf '%{VERSION}' "$pkg" 2>/dev/null || true)" ;;
    esac
    version="${version#*:}"
    version="${version%%-*}"
  fi
  echo "$version"
}

# Resolve ESPANSO_VERSION to a concrete x.y.z without the GitHub API (no rate
# limit): the releases/latest page redirects to .../tag/vX.Y.Z.
resolve_version() {
  local version="$ESPANSO_VERSION"
  if [[ "$version" == "latest" ]]; then
    version="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$ESPANSO_REPO_URL/releases/latest" 2>/dev/null)" || true
    version="${version##*/}"
  fi
  version="${version#v}"
  [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || error_exit "Could not resolve a valid espanso version (got: '$version')"
  echo "$version"
}

release_asset_url() {
  echo "$ESPANSO_REPO_URL/releases/download/v$1/$2"
}

# ---------------------------------------------------------------- detection

# Echo x11 or wayland for the current Linux graphical session.
detect_backend() {
  if [[ -n "$ESPANSO_BACKEND" ]]; then
    case "$ESPANSO_BACKEND" in
    x11 | wayland) echo "$ESPANSO_BACKEND" && return 0 ;;
    *) error_exit "Invalid backend: $ESPANSO_BACKEND (expected x11 or wayland)" ;;
    esac
  fi

  case "${XDG_SESSION_TYPE:-}" in
  wayland | x11) echo "$XDG_SESSION_TYPE" && return 0 ;;
  esac
  [[ -n "${WAYLAND_DISPLAY:-}" ]] && echo "wayland" && return 0
  [[ -n "${DISPLAY:-}" ]] && echo "x11" && return 0

  # Over SSH or from a TTY the session env is missing; ask logind for the
  # user's graphical session instead.
  if command -v loginctl >/dev/null 2>&1; then
    local session type
    for session in $(loginctl list-sessions --no-legend 2>/dev/null | awk -v u="${USER:-$(id -un)}" '$3 == u {print $1}'); do
      type="$(loginctl show-session "$session" -p Type --value 2>/dev/null || true)"
      case "$type" in
      wayland | x11) echo "$type" && return 0 ;;
      esac
    done
  fi

  error_exit "Could not detect an X11 or Wayland session - pass --x11 or --wayland"
}

aur_helper() {
  local helper
  for helper in paru yay; do
    command -v "$helper" >/dev/null 2>&1 && echo "$helper" && return 0
  done
  return 1
}

# Pick the upstream-recommended install method for this distro and backend.
choose_method() {
  local backend="$1"
  if [[ -n "$ESPANSO_METHOD" ]]; then
    case "$ESPANSO_METHOD" in
    deb | terra | aur | appimage | source) echo "$ESPANSO_METHOD" && return 0 ;;
    *) error_exit "Invalid method: $ESPANSO_METHOD (expected deb, terra, aur, appimage, or source)" ;;
    esac
  fi

  case "$DOTFILES_OS" in
  debian) is_amd64 && echo "deb" && return 0 ;;
  redhat) is_fedora && echo "terra" && return 0 ;;
  arch) aur_helper >/dev/null && echo "aur" && return 0 ;;
  esac
  if [[ "$backend" == "x11" ]] && is_amd64; then
    echo "appimage"
  else
    echo "source"
  fi
}

# Echo "<method> [package]" for the current espanso install, or nothing.
# The binary on PATH decides; a stray AppImage file only counts when no other
# espanso is found, so it cannot hide a package-manager install.
detect_installed_method() {
  local bin pkg
  bin="$(command -v espanso 2>/dev/null || true)"

  if [[ -z "$bin" ]]; then
    [[ -x "$ESPANSO_APPIMAGE_PATH" ]] && echo "appimage"
    return 0
  fi
  bin="$(readlink -f "$bin")"

  if [[ "$bin" -ef "$ESPANSO_APPIMAGE_PATH" ]]; then
    echo "appimage" && return 0
  fi
  if command -v dpkg-query >/dev/null 2>&1 &&
    pkg="$(dpkg-query -S "$bin" 2>/dev/null | cut -d: -f1)" && [[ -n "$pkg" ]]; then
    echo "deb $pkg" && return 0
  fi
  if command -v pacman >/dev/null 2>&1 && pkg="$(pacman -Qqo "$bin" 2>/dev/null)"; then
    echo "aur $pkg" && return 0
  fi
  if command -v rpm >/dev/null 2>&1 && pkg="$(rpm -qf --qf '%{NAME}' "$bin" 2>/dev/null)"; then
    echo "terra $pkg" && return 0
  fi
  if [[ "$bin" -ef "$ESPANSO_SOURCE_PATH" ]]; then
    echo "source" && return 0
  fi
  echo "unknown $bin"
}

# The display-server build of an installed espanso, so an update never swaps
# the X11 and Wayland builds based on the shell it happens to run in. An
# explicit --x11/--wayland wins.
installed_backend() {
  local method="$1" pkg="$2"
  if [[ -n "$ESPANSO_BACKEND" ]]; then
    detect_backend
    return 0
  fi
  case "$method" in
  appimage) echo "x11" ;;
  deb | terra | aur)
    case "$pkg" in
    *wayland*) echo "wayland" ;;
    *) echo "x11" ;;
    esac
    ;;
  *) detect_backend ;;
  esac
}

# ---------------------------------------------------------------- Linux methods

# Upstream publishes no checksums or signatures for the .deb and AppImage
# assets, so HTTPS to GitHub Releases is the only integrity guarantee.
install_deb() {
  local backend="$1" version file
  require_commands curl apt-get
  version="$(resolve_version)"
  file="espanso-debian-${backend}-amd64.deb"
  make_temp_dir
  log "Installing espanso v$version ($backend) from $file"
  run curl -fsSL "$(release_asset_url "$version" "$file")" -o "$TEMP_DIR/$file"
  # --allow-downgrades so an explicit --version can pin an older release.
  run_root apt-get install -y --allow-downgrades "$TEMP_DIR/$file"
}

install_terra() {
  local backend="$1"
  require_commands dnf rpm
  if ! rpm -q terra-release >/dev/null 2>&1; then
    confirm_or_exit "espanso on Fedora comes from the third-party Terra repository. Add it?"
    run_root dnf install -y --nogpgcheck --repofrompath "terra,$TERRA_REPO_URL" terra-release
  fi
  run_root dnf install -y "espanso-$backend"
}

aur_install() {
  local helper="$1" pkg="$2"
  local -a args=(-S --needed "$pkg")
  is_non_interactive && args+=(--noconfirm)
  run "$helper" "${args[@]}"
}

install_aur() {
  local backend="$1" helper
  helper="$(aur_helper)" || error_exit "An AUR helper (paru or yay) is required for --method aur"
  # The AUR package's install hook applies the Wayland capability itself.
  aur_install "$helper" "espanso-$backend"
}

# AppImages need the FUSE 2 runtime, which newer distros no longer ship.
ensure_fuse2() {
  local ldc
  ldc="$(PATH="$PATH:/sbin:/usr/sbin" command -v ldconfig || true)"
  if [[ -n "$ldc" ]] && "$ldc" -p 2>/dev/null | grep -q 'libfuse\.so\.2'; then
    return 0
  fi
  local pkg
  case "$DOTFILES_OS" in
  debian)
    # Ubuntu 24.04+ renamed the package for the 64-bit time_t transition.
    pkg="libfuse2"
    apt-cache show libfuse2t64 >/dev/null 2>&1 && pkg="libfuse2t64"
    run_root apt-get install -y "$pkg"
    ;;
  redhat) run_root dnf install -y fuse-libs ;;
  arch) run_root pacman -S --needed --noconfirm fuse2 ;;
  suse) run_root zypper --non-interactive install libfuse2 ;;
  *) error_exit "The AppImage needs FUSE 2 (libfuse.so.2) - install it, then re-run" ;;
  esac
}

install_appimage() {
  local version url
  require_commands curl
  ensure_fuse2
  version="$(resolve_version)"
  url="$(release_asset_url "$version" "Espanso-X11.AppImage")"
  log "Installing espanso v$version AppImage to $ESPANSO_APPIMAGE_PATH (requires FUSE 2)"
  make_temp_dir
  # Download to a temp file first: a truncated download must never land on the
  # path that marks espanso as installed.
  run curl -fsSL "$url" -o "$TEMP_DIR/Espanso.AppImage"
  run mkdir -p "$(dirname "$ESPANSO_APPIMAGE_PATH")"
  run install -m 755 "$TEMP_DIR/Espanso.AppImage" "$ESPANSO_APPIMAGE_PATH"
  # Links /usr/local/bin/espanso to the AppImage.
  run_root "$ESPANSO_APPIMAGE_PATH" env-path register
}

install_build_deps() {
  local backend="$1"
  local -a pkgs
  case "$DOTFILES_OS" in
  debian)
    local wx
    wx="$(apt-cache pkgnames libwxgtk3 2>/dev/null | grep -E '^libwxgtk3\.[0-9]+-dev$' | sort -V | tail -1 || true)"
    pkgs=(build-essential git libxkbcommon-dev libdbus-1-dev libssl-dev "${wx:-libwxgtk3.2-dev}")
    if [[ "$backend" == "wayland" ]]; then pkgs+=(wl-clipboard); else pkgs+=(libx11-dev libxtst-dev); fi
    run_root apt-get install -y "${pkgs[@]}"
    ;;
  redhat)
    pkgs=(@development-tools gcc-c++ git libxkbcommon-devel dbus-devel wxGTK-devel openssl-devel)
    if [[ "$backend" == "wayland" ]]; then pkgs+=(wl-clipboard); else pkgs+=(libX11-devel libXtst-devel); fi
    run_root dnf install -y "${pkgs[@]}"
    ;;
  arch)
    pkgs=(base-devel git dbus wxwidgets-gtk3 libxkbcommon)
    if [[ "$backend" == "wayland" ]]; then pkgs+=(wl-clipboard); else pkgs+=(libx11 libxtst); fi
    run_root pacman -S --needed --noconfirm "${pkgs[@]}"
    ;;
  *)
    log "Build dependencies are not automated for $DOTFILES_OS - install git, a C/C++"
    log "toolchain, and the dbus, libxkbcommon, openssl, and wxWidgets (GTK3) dev packages"
    confirm_or_exit "Continue with the source build?"
    ;;
  esac
}

install_source() {
  local backend="$1" version features src
  require_commands git cargo
  version="$(resolve_version)"
  features="modulo,vendored-tls"
  [[ "$backend" == "wayland" ]] && features+=",wayland"

  log "Building espanso v$version ($backend) from source - this takes a while"
  install_build_deps "$backend"
  make_temp_dir
  src="$TEMP_DIR/espanso"
  run git clone --depth 1 --branch "v$version" "$ESPANSO_REPO_URL" "$src"
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would run cargo build -p espanso --release --no-default-features --features $features"
  else
    (cd "$src" && cargo build -p espanso --release --no-default-features --features "$features")
  fi
  run_root install -Dm 755 "$src/target/release/espanso" "$ESPANSO_SOURCE_PATH"
}

# The Wayland build reads /dev/input directly and needs this capability. Package
# upgrades and reinstalls drop it, so it is reapplied after every install.
apply_wayland_capability() {
  local backend="$1" method="$2" bin
  [[ "$backend" == "wayland" ]] || return 0
  [[ "$method" == "aur" ]] && return 0
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: would run setcap cap_dac_override+p on the espanso binary"
    return 0
  fi
  require_commands setcap
  bin="$(readlink -f "$(command -v espanso)")"
  run_root setcap "cap_dac_override+p" "$bin"
}

# ---------------------------------------------------------------- service

espanso_running() {
  espanso status 2>/dev/null | grep -q '^espanso is running'
}

# `espanso service register` starts the unit, but the daemon needs a few seconds
# before `espanso status` reports it. Calling `espanso start` inside that window
# fails (exit 3/5), so poll first.
wait_for_espanso() {
  local i
  for ((i = 0; i < 15; i++)); do
    espanso_running && return 0
    sleep 1
  done
  return 1
}

register_service() {
  [[ "$SKIP_SERVICE" == "true" ]] && return 0
  case "$DOTFILES_OS" in
  macos)
    log "Launching Espanso - grant it Accessibility access when macOS asks"
    run open -a Espanso || log "Open Espanso from /Applications to finish setup"
    ;;
  wsl) ;;
  *)
    if ! systemctl --user show-environment >/dev/null 2>&1; then
      log "No systemd user session - start espanso manually with: espanso start --unmanaged"
      return 0
    fi
    run espanso service register
    if [[ "$DRY_RUN" == "true" ]]; then
      run espanso start
    elif ! wait_for_espanso; then
      run espanso start
    fi
    ;;
  esac
}

# Bring espanso back up after an update when it was running beforehand.
resume_service() {
  local was_running="$1"
  [[ "$was_running" == "true" ]] || return 0
  if espanso_running; then
    run espanso restart || log "espanso restart failed - run: espanso restart"
  else
    run espanso start || log "espanso start failed - run: espanso start"
  fi
}

stop_and_unregister() {
  command -v espanso >/dev/null 2>&1 || return 0
  [[ "$DOTFILES_OS" == "wsl" ]] && return 0
  run espanso stop >/dev/null 2>&1 || true
  run espanso service unregister || true
}

# ---------------------------------------------------------------- platforms

winget_cmd() {
  local cmd
  for cmd in winget winget.exe; do
    command -v "$cmd" >/dev/null 2>&1 && echo "$cmd" && return 0
  done
  error_exit "winget.exe not found on PATH - install espanso on Windows: https://espanso.org/docs/install/win/"
}

winget_run() {
  local verb="$1" winget
  winget="$(winget_cmd)"
  local -a args=("$verb" --exact --id "$ESPANSO_WINGET_ID")
  [[ "$verb" != "uninstall" ]] && args+=(--accept-source-agreements --accept-package-agreements)
  run "$winget" "${args[@]}"
}

brew_installed() {
  command -v brew >/dev/null 2>&1 && brew list --cask espanso >/dev/null 2>&1
}

linux_install() {
  local backend method
  backend="$(detect_backend)"
  method="$(choose_method "$backend")"
  log "Detected $DOTFILES_OS/${DOTFILES_ARCH:-$(uname -m)}, $backend session -> method: $method"
  case "$method" in
  deb) install_deb "$backend" ;;
  terra) install_terra "$backend" ;;
  aur) install_aur "$backend" ;;
  appimage) install_appimage ;;
  source) install_source "$backend" ;;
  esac
  apply_wayland_capability "$backend" "$method"
}

linux_update() {
  local installed method pkg backend current target
  installed="$(detect_installed_method)"
  read -r method pkg <<<"$installed"
  backend="$(installed_backend "$method" "$pkg")"
  log "Updating espanso installed via $method${pkg:+ ($pkg)} ($backend)"

  case "$method" in
  terra)
    run_root dnf upgrade -y "$pkg"
    ;;
  aur)
    local helper
    helper="$(aur_helper)" || error_exit "An AUR helper (paru or yay) is required to update $pkg"
    aur_install "$helper" "$pkg"
    ;;
  deb | appimage | source)
    current="$(installed_version)"
    target="$(resolve_version)"
    if [[ "$current" == "$target" ]] && [[ "$FORCE_INSTALL" != "true" ]]; then
      log "espanso is up to date: $current"
      UPDATED="false"
      return 0
    fi
    log "Updating espanso $current -> $target"
    case "$method" in
    deb) ESPANSO_VERSION="$target" install_deb "$backend" ;;
    appimage)
      # The running AppImage cannot be overwritten in place.
      run espanso stop >/dev/null 2>&1 || true
      ESPANSO_VERSION="$target" install_appimage
      ;;
    source) ESPANSO_VERSION="$target" install_source "$backend" ;;
    esac
    ;;
  *) error_exit "espanso at ${pkg:-?} was not installed by a known method - update it manually" ;;
  esac
  apply_wayland_capability "$backend" "$method"
}

linux_remove() {
  local installed method pkg
  installed="$(detect_installed_method)"
  read -r method pkg <<<"$installed"
  case "$method" in
  deb) run_root apt-get remove -y "$pkg" ;;
  terra) run_root dnf remove -y "$pkg" ;;
  aur) run_root pacman -Rns --noconfirm "$pkg" ;;
  appimage)
    run_root "$ESPANSO_APPIMAGE_PATH" env-path unregister || true
    run rm -f "$ESPANSO_APPIMAGE_PATH"
    ;;
  source) run_root rm -f "$ESPANSO_SOURCE_PATH" ;;
  *) error_exit "espanso at ${pkg:-?} was not installed by a known method - remove it manually" ;;
  esac
}

# ---------------------------------------------------------------- actions

verify_installation() {
  [[ "$DRY_RUN" == "true" ]] && return 0
  [[ "$DOTFILES_OS" == "wsl" ]] && return 0
  hash -r
  command -v espanso >/dev/null 2>&1 || error_exit "espanso is installed but not on PATH - restart your shell"
  log "espanso verified: $(installed_version) ($(command -v espanso))"
}

is_installed() {
  case "$DOTFILES_OS" in
  macos) brew_installed || command -v espanso >/dev/null 2>&1 ;;
  wsl) "$(winget_cmd)" list --exact --id "$ESPANSO_WINGET_ID" >/dev/null 2>&1 ;;
  *) command -v espanso >/dev/null 2>&1 || [[ -x "$ESPANSO_APPIMAGE_PATH" ]] ;;
  esac
}

do_install() {
  if is_installed && [[ "$FORCE_INSTALL" != "true" ]]; then
    local current
    current="$(installed_version)"
    log "espanso is already installed${current:+: $current} (use --update or --force)"
    return 0
  fi
  case "$DOTFILES_OS" in
  macos)
    require_commands brew
    run brew install --cask espanso
    ;;
  wsl) winget_run install ;;
  *) linux_install ;;
  esac
  verify_installation
  register_service
}

do_update() {
  if ! is_installed; then
    log "espanso is not installed - installing"
    do_install
    return 0
  fi
  local was_running="false"
  [[ "$DOTFILES_OS" != "wsl" ]] && espanso_running && was_running="true"
  case "$DOTFILES_OS" in
  macos) run brew upgrade --cask espanso || log "brew reports espanso is already up to date" ;;
  wsl) winget_run upgrade || log "winget reports espanso is already up to date" ;;
  *) linux_update ;;
  esac
  verify_installation
  if [[ "$DRY_RUN" != "true" ]] && [[ "$UPDATED" == "true" ]]; then
    resume_service "$was_running"
  fi
}

# Fail before touching the service when espanso cannot be removed by this script.
ensure_removable() {
  local method pkg
  case "$DOTFILES_OS" in
  macos) brew_installed || error_exit "espanso was not installed with Homebrew - delete /Applications/Espanso.app manually" ;;
  wsl) ;;
  *)
    read -r method pkg <<<"$(detect_installed_method)"
    [[ "$method" != "unknown" ]] || error_exit "espanso at ${pkg:-?} was not installed by a known method - remove it manually"
    ;;
  esac
}

do_remove() {
  if ! is_installed; then
    log "espanso is not installed"
    return 0
  fi
  ensure_removable
  confirm_or_exit "Stop espanso and remove it?"
  stop_and_unregister
  case "$DOTFILES_OS" in
  macos) run brew uninstall --cask espanso ;;
  wsl) winget_run uninstall ;;
  *) linux_remove ;;
  esac
  [[ "$DRY_RUN" == "true" ]] || log "espanso removed (configuration left in place)"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
    --install) ACTION="install" ;;
    --update) ACTION="update" ;;
    --remove | --uninstall) ACTION="remove" ;;
    --x11) ESPANSO_BACKEND="x11" ;;
    --wayland) ESPANSO_BACKEND="wayland" ;;
    --method)
      [[ $# -ge 2 ]] || error_exit "--method requires a value"
      ESPANSO_METHOD="$2"
      shift
      ;;
    --version)
      [[ $# -ge 2 ]] || error_exit "--version requires a value"
      ESPANSO_VERSION="$2"
      shift
      ;;
    --no-service) SKIP_SERVICE="true" ;;
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

  case "$DOTFILES_OS" in
  macos | wsl) ;;
  *) is_linux || error_exit "Unsupported operating system: $DOTFILES_OS" ;;
  esac
  # espanso runs as a per-user service and AUR helpers refuse root; sudo is
  # invoked only for the steps that need it.
  if [[ "$DOTFILES_OS" != "macos" ]] && [[ "$(id -u)" -eq 0 ]]; then
    error_exit "Run as your regular user, not root - sudo is used where needed"
  fi

  case "$ACTION" in
  install) do_install ;;
  update) do_update ;;
  remove | uninstall) do_remove ;;
  *) error_exit "Invalid action: $ACTION. Use --help for usage information." ;;
  esac
}

main "$@"
