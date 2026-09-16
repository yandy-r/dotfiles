#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[ARCH-INSTALL]"

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m'

# Configuration
DRY_RUN="${DRY_RUN:-false}"
SKIP_OFFICIAL="${SKIP_OFFICIAL:-false}"
SKIP_AUR="${SKIP_AUR:-false}"
NON_INTERACTIVE="${NON_INTERACTIVE:-false}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
INSTALL_GAMING="${INSTALL_GAMING:-false}"

# Force CMake to use Unix Makefiles by default. Many AUR PKGBUILDs call
# bare `cmake` + `make`; when ninja is installed CMake silently switches
# to the Ninja generator, breaking those builds.
export CMAKE_GENERATOR="Unix Makefiles"

# Track installation statistics
INSTALLED_COUNT=0
SKIPPED_COUNT=0
FAILED_COUNT=0
declare -a FAILED_PACKAGES=()
OPTIONAL_FAILED_COUNT=0
declare -a OPTIONAL_FAILED_PACKAGES=()

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install packages on Arch Linux using pacman and yay.

OPTIONS:
    --dry-run           Show what would be installed without actually installing
    --skip-official     Skip official repository packages
    --skip-aur          Skip AUR packages
    --gaming            Install optional gaming packages for CachyOS and Manjaro
    --non-interactive   Run without user prompts
    --force             Force reinstallation of already-installed packages
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DRY_RUN            Enable dry-run mode (true/false)
    SKIP_OFFICIAL      Skip official repository packages (true/false)
    SKIP_AUR           Skip AUR packages (true/false)
    INSTALL_GAMING     Install gaming packages (true/false)
    NON_INTERACTIVE    Run non-interactively (true/false)
    FORCE_INSTALL      Force reinstallation (true/false)

EXAMPLES:
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --skip-aur
    $SCRIPT_NAME --gaming
    NON_INTERACTIVE=true $SCRIPT_NAME

NOTES:
    - This script is designed for Arch Linux systems
    - Requires pacman for official packages
    - Optionally uses yay for AUR packages
    - Equivalent packages to Homebrew installation are installed

EOF
  exit "${1:-0}"
}

# ================================================================
# LOGGING FUNCTIONS
# ================================================================

log() {
  echo -e "${BLUE}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

log_success() {
  echo -e "${GREEN}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] ✓ $*" >&2
}

log_warning() {
  echo -e "${YELLOW}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] ⚠ $*" >&2
}

log_error() {
  echo -e "${RED}${LOG_PREFIX}${NC} [$(date '+%Y-%m-%d %H:%M:%S')] ✗ $*" >&2
}

error_exit() {
  log_error "$1" >&2
  exit "${2:-1}"
}

print_section() {
  echo
  echo -e "${CYAN}================================================================${NC}"
  echo -e "${CYAN}  $1${NC}"
  echo -e "${CYAN}================================================================${NC}"
}

# ================================================================
# UTILITY FUNCTIONS
# ================================================================

has_command() {
  command -v "$1" &>/dev/null
}

is_interactive() {
  [[ -t 0 ]] && [[ -t 1 ]]
}

prompt_confirmation() {
  local prompt_message="$1"
  local response

  # Auto-approve in non-interactive mode
  if [[ "${NON_INTERACTIVE}" == "true" ]] || [[ "${FORCE_INSTALL}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]; then
    log "Non-interactive mode: auto-approving prompt"
    return 0
  fi

  if ! is_interactive; then
    log_error "Non-interactive shell detected - cannot prompt for confirmation"
    return 1
  fi

  echo -n "${LOG_PREFIX} ${prompt_message} [y/N]: " >&2
  read -r response

  response="${response:-n}"

  case "$response" in
  [yY] | [yY][eE][sS])
    return 0
    ;;
  *)
    return 1
    ;;
  esac
}

is_installed() {
  local package="$1"
  pacman -Qi "$package" &>/dev/null
}

get_raw_distro_id() {
  if [[ -f /etc/os-release ]]; then
    local distro_id
    distro_id="$(. /etc/os-release && printf '%s' "${ID:-unknown}")"
    printf '%s\n' "${distro_id,,}"
  else
    printf 'unknown\n'
  fi
}

# ================================================================
# PACKAGE INSTALLATION FUNCTIONS
# ================================================================

install_package_internal() {
  local package="$1"
  local use_aur="${2:-false}"
  local track_failure="${3:-true}"

  # Check if already installed
  if is_installed "$package"; then
    if [[ "$FORCE_INSTALL" != "true" ]]; then
      log "Package '$package' already installed, skipping"
      SKIPPED_COUNT=$((SKIPPED_COUNT + 1))
      return 0
    else
      log "Package '$package' already installed, reinstalling due to --force"
    fi
  fi

  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would install $package"
    return 0
  fi

  log "Installing $package..."

  local install_cmd
  if [[ "$use_aur" == "true" ]]; then
    if ! has_command yay; then
      if [[ "$track_failure" == "true" ]]; then
        log_warning "yay not available, cannot install AUR package: $package"
        FAILED_COUNT=$((FAILED_COUNT + 1))
        FAILED_PACKAGES+=("$package (AUR - yay not available)")
      else
        log_warning "Optional AUR package unavailable (yay missing): $package"
        OPTIONAL_FAILED_COUNT=$((OPTIONAL_FAILED_COUNT + 1))
        OPTIONAL_FAILED_PACKAGES+=("$package (AUR - yay not available)")
      fi
      return 1
    fi
    install_cmd="yay -S --noconfirm --needed"
  else
    install_cmd="sudo pacman -S --noconfirm --needed"
  fi

  if $install_cmd "$package"; then
    log_success "Successfully installed $package"
    INSTALLED_COUNT=$((INSTALLED_COUNT + 1))
    return 0
  else
    if [[ "$track_failure" == "true" ]]; then
      log_error "Failed to install $package"
      FAILED_COUNT=$((FAILED_COUNT + 1))
      FAILED_PACKAGES+=("$package")
    else
      log_warning "Optional package failed to install: $package"
      OPTIONAL_FAILED_COUNT=$((OPTIONAL_FAILED_COUNT + 1))
      OPTIONAL_FAILED_PACKAGES+=("$package")
    fi
    return 1
  fi
}

install_package() {
  install_package_internal "$1" "${2:-false}" "true"
}

install_package_optional() {
  install_package_internal "$1" "${2:-false}" "false"
}

install_packages_batch() {
  local -a packages=("$@")
  local use_aur="${packages[-1]}"
  unset 'packages[-1]'

  for package in "${packages[@]}"; do
    install_package "$package" "$use_aur" || true
  done
}

# ================================================================
# YAY INSTALLATION
# ================================================================

install_yay() {
  if has_command yay; then
    log_success "yay is already installed"
    return 0
  fi

  if [[ "$SKIP_AUR" == "true" ]]; then
    log "Skipping yay installation (AUR disabled)"
    return 0
  fi

  print_section "Installing yay (AUR helper)"

  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would install yay from AUR"
    return 0
  fi

  # Install base-devel if not present
  if ! pacman -Qg base-devel &>/dev/null; then
    log "Installing base-devel..."
    sudo pacman -S --noconfirm --needed base-devel git
  fi

  local yay_dir="/tmp/yay-install-$$"
  log "Cloning yay repository..."

  if ! git clone https://aur.archlinux.org/yay.git "$yay_dir"; then
    error_exit "Failed to clone yay repository"
  fi

  log "Building and installing yay..."
  (
    cd "$yay_dir"
    makepkg -si --noconfirm
  ) || error_exit "Failed to build yay"

  rm -rf "$yay_dir"
  log_success "yay installed successfully"
}

# ================================================================
# SYSTEM UPDATE
# ================================================================

update_system() {
  print_section "Updating package database"

  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would update package database"
    return 0
  fi

  log "Updating pacman database..."
  if ! sudo pacman -Sy; then
    log_warning "Failed to update package database"
    return 1
  fi

  log_success "Package database updated"
}

# ================================================================
# PACKAGE INSTALLATION SECTIONS
# ================================================================

install_core_packages() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping core packages (official repos disabled)"
    return 0
  fi

  print_section "Installing Core Packages"

  # Shells and shell tools
  install_package "zsh" "false" || true
  install_package "zsh-autosuggestions" "false" || true
  install_package "zsh-completions" "false" || true
  install_package "zsh-syntax-highlighting" "false" || true
  install_package "bash" "false" || true

  # Core utilities (coreutils is part of base, but ensure it's there)
  install_package "coreutils" "false" || true
  install_package "util-linux" "false" || true

  # System essentials
  install_package "openssh" "false" || true
  install_package "unzip" "false" || true
  install_package "man-db" "false" || true
  install_package "man-pages" "false" || true
}

install_development_tools() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping development tools (official repos disabled)"
    return 0
  fi

  print_section "Installing Development Tools"

  # Version control
  install_package "git" "false" || true
  install_package "git-lfs" "false" || true
  install_package "github-cli" "false" || true # gh

  # Build tools
  install_package "base-devel" "false" || true
  install_package "cmake" "false" || true
  install_package "gcc" "false" || true
  install_package "clang" "false" || true
  install_package "ninja" "false" || true
  install_package "pkg-config" "false" || true

  # Programming languages
  install_package "python" "false" || true
  install_package "python-pip" "false" || true
  install_package "python-pipx" "false" || true
  install_package "uv" "false" || true
  install_package "pyenv" "false" || true
  install_package "ruby" "false" || true
  install_package "ruby-lsp" "false" || true
  install_package "go" "false" || true
  install_package "lua" "false" || true
  install_package "luarocks" "false" || true
  # Node/npm: managed by mise via ~/.config/mise (symlinked from dotfiles mise/config.toml), not pacman

  # Editors
  install_package "neovim" "false" || true

  # Runtime/version management
  install_package "mise" "false" || true # polyglot runtime manager (replaces asdf/nvm/pyenv)
}

install_cli_tools() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping CLI tools (official repos disabled)"
    return 0
  fi

  print_section "Installing Modern CLI Tools"

  # Search and file tools (ack omitted: use ripgrep/rg; install ack from AUR if needed)
  install_package "ripgrep" "false" || true # rg
  install_package "fd" "false" || true
  install_package "fzf" "false" || true
  install_package "tree" "false" || true
  install_package "tree-sitter" "false" || true
  install_package "tree-sitter-cli" "false" || true

  # Modern alternatives
  install_package "eza" "false" || true    # better ls
  install_package "bat" "false" || true    # better cat
  install_package "zoxide" "false" || true # better cd
  install_package "bottom" "false" || true # btm - system monitor
  install_package "btop" "false" || true   # btop - system monitor
  install_package "nload" "false" || true  # network traffic monitor

  # Git tools
  install_package "git-cliff" "false" || true # changelog generator
  install_package "git-delta" "false" || true # better git diff
  install_package "lazygit" "false" || true
  install_package "tea" "false" || true       # Gitea/Forgejo CLI

  # Text processing
  install_package "jq" "false" || true
  install_package "yq" "false" || true
  install_package "gawk" "false" || true
  install_package "sd" "false" || true     # modern find & replace
  install_package "choose" "false" || true # modern cut/awk alternative
  install_package "grex" "false" || true   # generate regexes from examples

  # HTTP clients
  install_package "curl" "false" || true
  install_package "wget" "false" || true
  install_package "httpie" "false" || true
  install_package "xh" "false" || true

  # Shell enhancements
  install_package "direnv" "false" || true
  install_package "thefuck" "false" || true

  # Code formatting
  install_package "prettier" "false" || true

  # Prompt
  install_package "starship" "false" || true # cross-shell prompt

  # System monitoring
  install_package "htop" "false" || true      # interactive process viewer
  install_package "procs" "false" || true     # modern ps alternative
  install_package "sysstat" "false" || true   # iostat, mpstat, sar
  install_package "bandwhich" "false" || true # terminal bandwidth monitor

  # Disk usage
  install_package "ncdu" "false" || true # ncurses disk usage
  install_package "dust" "false" || true # modern du alternative

  # Documentation
  install_package "tealdeer" "false" || true # fast tldr client (provides tldr)

  # Benchmarking and analysis
  install_package "hyperfine" "false" || true # command-line benchmarking
  install_package "tokei" "false" || true     # code statistics

  # File management
  install_package "yazi" "false" || true  # terminal file manager
  install_package "broot" "false" || true # fuzzy directory navigator
  install_package "ouch" "false" || true  # painless compression/decompression
  install_package "vivid" "false" || true # LS_COLORS generator

  # Task runner
  install_package "just" "false" || true # modern command runner (like make)

  # Security
  install_package "age" "false" || true # modern file encryption

  # Miscellaneous
  install_package "pandoc" "false" || true
  install_package "screen" "false" || true
  install_package "file" "false" || true
}

install_fonts() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping fonts (official repos disabled)"
    return 0
  fi

  print_section "Installing Nerd Fonts"

  # JetBrainsMono Nerd Font is the primary terminal/editor font across the repo.
  install_package "ttf-jetbrains-mono-nerd" "false" || true

  # Hyprland/Waybar/SwayOSD still reference the Caskaydia mono/code families.
  install_package "ttf-cascadia-mono-nerd" "false" || true
  install_package "ttf-cascadia-code-nerd" "false" || true
}

install_desktop_apps() {
  print_section "Installing Desktop Applications"

  if [[ "$SKIP_OFFICIAL" != "true" ]]; then
    # Generic Linux desktop baseline for the repo-owned CachyOS/Hyprland path.
    install_package "acpid" "false" || true # lid events for monitor-auto-switch
    install_package "discord" "false" || true
    install_package "fuzzel" "false" || true                      # alternative Wayland launcher kept in the desktop baseline
    install_package "rofi" "false" || true                        # dmenu with tab-stops (keybinding cheatsheet popup)
    install_package "rofi-calc" "false" || true                   # calculator mode for rofi
    install_package "ghostty" "false" || true                     # GPU-accelerated terminal emulator
    install_package "grim" "false" || true                        # screenshots
    install_package "satty" "false" || true                       # screenshot annotation/editing tool
    install_package "hyprland" "false" || true                    # compositor for the repo-owned desktop baseline
    install_package "hypridle" "false" || true                    # idle daemon
    install_package "hyprlauncher" "false" || true                # launcher used by hypr/bindings.lua
    install_package "hyprlock" "false" || true                    # lock screen
    install_package "hyprpaper" "false" || true                   # wallpaper utility
    install_package "hyprpicker" "false" || true                  # color picker utility
    install_package "hyprpolkitagent" "false" || true             # polkit authentication agent
    install_package "hyprsunset" "false" || true                  # display temperature utility
    install_package "libnotify" "false" || true                   # notify-send for monitor hooks
    install_package "slurp" "false" || true                       # region selection for screenshots
    install_package "waybar" "false" || true                      # status bar
    install_package "whatsapp-linux-desktop-bin" "true" || true   # WhatsApp desktop client
    install_package "wlogout" "false" || true                     # power / session menu (logout, reboot, shutdown)
    install_package "wf-recorder" "false" || true                 # screen recording
    install_package "wl-clipboard" "false" || true                # clipboard integration for screenshots
    install_package "xdg-desktop-portal" "false" || true          # Wayland portal base
    install_package "xdg-desktop-portal-gtk" "false" || true      # Settings portal (color-scheme, dark/light preference)
    install_package "xdg-desktop-portal-hyprland" "false" || true # Hyprland portal backend
    install_package "xdg-utils" "false" || true                   # xdg-open for generic binds

    # IDEs
    install_package "zed-preview-bin" "true" || true        # Zed Preview IDE
    install_package "cursor-bin" "true" || true             # Cursor IDE
    install_package "antigravity" "true" || true            # Antigravity IDE
    install_package "visual-studio-code-bin" "true" || true # Visual Studio Code Insiders IDE
    install_package "beekeeper-studio-appimage" "true" || true # SQL/database GUI client

    # Media and creative tools
    install_package "phototonic" "false" || true      # GPU-accelerated image viewer
    install_package "vlc" "false" || true             # GPU-accelerated media player
    install_package "darktable" "false" || true       # photo editing software
    install_package "pinta" "true" || true            # photo editing software
    install_package "gimp" "false" || true            # photo editing software
    install_package "obs-studio" "false" || true      # GPU-accelerated screen recording and streaming
    install_package "kdenlive" "false" || true        # video editing software
    install_package "davinci-resolve" "false" || true # video editing software
    install_package "localsend" "false" || true       # file sharing software
    install_package "foxit-pdf-editor" "true" || true # PDF editing software
    install_package "foxitreader" "true" || true      # PDF viewing software

    # Desktop environment utilities
    install_package "brightnessctl" "false" || true   # laptop brightness keys (hypr/bindings.lua)
    install_package "dolphin" "false" || true         # KDE file manager
    install_package "nautilus" "false" || true        # GNOME file manager (hypr/bindings.lua $fileManager)
    install_package "qalculate-gtk" "false" || true   # GTK desktop calculator
    install_package "sushi" "false" || true           # GNOME Quick Look (Space to preview in Nautilus)
    install_package "gnome-keyring" "false" || true   # credential/secret storage for browsers & SSH
    install_package "playerctl" "false" || true       # media player control (hypr/bindings.lua)
    install_package "seahorse" "false" || true        # GUI for managing gnome-keyring secrets
    install_package "libappindicator" "false" || true # SNI tray icon support (Dropbox, Discord, etc.)
    install_package "mako" "false" || true            # Wayland notification daemon with configurable click actions
    install_package "swayosd" "false" || true         # transient volume and brightness OSD for Hyprland binds
    install_package "wireplumber" "false" || true     # PipeWire session manager (provides wpctl for volume)
  else
    log "Skipping official-repo desktop apps"
  fi

  if [[ "$SKIP_AUR" == "true" ]]; then
    log "Skipping AUR desktop apps"
    return 0
  fi

  if ! has_command yay; then
    log_warning "yay not available, installing via pacman"
    install_package "yay" "false" || true
  fi

  install_package "1password" "true" || true
}

install_gaming_packages() {
  if [[ "$INSTALL_GAMING" != "true" ]]; then
    log "Skipping gaming packages (--gaming not enabled)"
    return 0
  fi

  local raw_distro
  raw_distro="$(get_raw_distro_id)"

  case "$raw_distro" in
  cachyos | manjaro) ;;
  *)
    log "Skipping gaming packages (supported only on CachyOS and Manjaro; detected: $raw_distro)"
    return 0
    ;;
  esac

  print_section "Installing Gaming Packages"

  if [[ "$raw_distro" == "cachyos" ]]; then
    if [[ "$SKIP_OFFICIAL" == "true" ]]; then
      log "Skipping CachyOS gaming meta packages (official repos disabled)"
    else
      install_package "cachyos-gaming-meta" "false" || true
      install_package "cachyos-gaming-applications" "false" || true
    fi
  fi

  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping official-repo gaming packages"
  else
    # Install a complete gaming runtime/tooling set. On CachyOS the meta packages
    # cover much of this already, so duplicate installs are skipped automatically.
    install_package "steam" "false" || true
    install_package "lutris" "false" || true
    install_package "gamescope" "false" || true
    install_package "mangohud" "false" || true
    install_package "lib32-mangohud" "false" || true
    install_package "gamemode" "false" || true
    install_package "lib32-gamemode" "false" || true
    install_package "goverlay" "false" || true
    install_package "protonup-qt" "false" || true
    install_package "umu-launcher" "false" || true
    install_package "protontricks" "false" || true
    install_package "winetricks" "false" || true
    install_package "wine-staging" "false" || true
    install_package "winboat" "false" || true
    install_package "bottles" "false" || true
    install_package "vkbasalt" "false" || true
    install_package "lib32-vkbasalt" "false" || true

    # Diagnostics and controllers
    install_package "vulkan-tools" "false" || true # vulkaninfo/vkcube for troubleshooting
    install_package "nvtop" "false" || true        # vendor-agnostic GPU monitor
    install_package "antimicrox" "false" || true   # map keyboard/mouse to gamepad
  fi

  if [[ "$SKIP_AUR" == "true" ]]; then
    log "Skipping AUR gaming packages"
    return 0
  fi

  if ! has_command yay; then
    log_warning "yay not available, skipping AUR gaming packages"
    return 0
  fi

  install_package "heroic-games-launcher-bin" "true" || true
  install_package "steamtinkerlaunch" "true" || true  # advanced per-game Proton launcher
  install_package "game-devices-udev" "true" || true  # udev rules for third-party controllers
  install_package "gpu-screen-recorder" "true" || true # ShadowPlay-style replay recorder
  install_package "obs-vkcapture" "true" || true       # Vulkan/OpenGL capture for OBS
  install_package "lib32-obs-vkcapture" "true" || true # 32-bit capture for older/wine games
  install_package "ludusavi-bin" "true" || true        # cross-platform game save backup
  install_package_optional "pince-bin" "true" || true
}

install_network_tools() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping network tools (official repos disabled)"
    return 0
  fi

  print_section "Installing Network Tools"

  install_package "nmap" "false" || true
  install_package "mtr" "false" || true
  install_package "openbsd-netcat" "false" || true # netcat
  install_package "tcpdump" "false" || true
  install_package "fping" "false" || true
  install_package "speedtest-cli" "false" || true
  install_package "traceroute" "false" || true
  install_package "whois" "false" || true
  install_package "aria2" "false" || true # multi-protocol download utility
  install_package "doggo" "false" || true # modern DNS client (Go)
  install_package "gping" "false" || true # ping with a live graph

  # File sharing
  install_package "nfs-utils" "false" || true # NFS client/server
  install_package "samba" "false" || true     # SMB/CIFS file sharing
}

install_cloud_tools() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping cloud tools (official repos disabled)"
    return 0
  fi

  print_section "Installing Cloud & DevOps Tools"

  # AWS
  install_package "aws-cli-v2" "false" || true

  # Azure
  install_package "azure-cli" "false" || true

  # Terraform and HashiCorp tools
  install_package "terraform" "false" || true
  install_package "tflint" "false" || true

  # Ansible
  install_package "ansible" "false" || true
  install_package "ansible-language-server" "false" || true

  # Kubernetes
  install_package "kubectl" "false" || true

  # Container tools
  install_package "docker" "false" || true
  install_package "docker-compose" "false" || true
  install_package "flatpak-builder" "false" || true
  install_package "lazydocker" "false" || true # Docker TUI (pairs with lazygit)
  install_package "skaffold" "false" || true
  install_package "hadolint" "true" || true

  # Other DevOps tools
  install_package "protobuf" "false" || true
}

install_tmux_tools() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping tmux tools (official repos disabled)"
    return 0
  fi

  print_section "Installing Tmux and Related Tools"

  install_package "tmux" "false" || true
}

install_media_tools() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping media tools (official repos disabled)"
    return 0
  fi

  print_section "Installing Media Tools"

  install_package "ffmpeg" "false" || true
  install_package "perl-image-exiftool" "false" || true # exiftool
}

install_libraries() {
  if [[ "$SKIP_OFFICIAL" == "true" ]]; then
    log "Skipping libraries (official repos disabled)"
    return 0
  fi

  print_section "Installing Development Libraries"

  install_package "freetype2" "false" || true
  install_package "libjpeg-turbo" "false" || true
  install_package "libpng" "false" || true
  install_package "libgit2" "false" || true
  install_package "zeromq" "false" || true # zmq
  install_package "rtmpdump" "false" || true
}

install_aur_packages() {
  if [[ "$SKIP_AUR" == "true" ]]; then
    log "Skipping AUR packages"
    return 0
  fi

  if ! has_command yay; then
    log_warning "yay not available, skipping AUR packages"
    return 0
  fi

  print_section "Installing AUR Packages"

  # AWS SAM CLI (AUR)
  install_package "aws-sam-cli" "true" || true

  # Amazon ECS CLI (AUR)
  install_package "aws-ecs-cli" "true" || true

  # CFN Lint (AUR)
  install_package "python-cfn-lint" "false" || true

  # GolangCI-Lint
  install_package "golangci-lint" "true" || true

  # Google Chrome
  install_package "google-chrome" "true" || true

  # Asciinema
  if ! is_installed "asciinema"; then
    install_package "asciinema" "true" || true
  fi

  # Carapace (universal completion generator)
  install_package "carapace-bin" "true" || true

  # Tmux tools (some might be in AUR)
  install_package "tmux-mem-cpu-load" "true" || true
  install_package "tmuxp" "true" || log_warning "tmuxp not available in AUR"

  # Gemini CLI (if available)
  install_package "gemini-cli" "true" || log_warning "gemini-cli not available"

  # Storage
  install_package "mergerfs" "true" || log_warning "mergerfs not available"

  # Network tools (AUR)
  install_package "sipcalc" "true" || true

  # Cloud sync
  install_package "dropbox" "true" || log_warning "dropbox not available"
}

# ================================================================
# MISE RUNTIMES (run last so node@latest wins in shell PATH)
# ================================================================
# System nodejs/npm may remain for pacman dependents (cursor-bin, prettier, etc.).
# We install and activate node via mise at the end so mise shims take precedence.

install_mise_runtimes() {
  if ! command -v mise &>/dev/null; then
    return 0
  fi
  if [[ ! -f "$HOME/.config/mise/config.toml" ]]; then
    log "No ~/.config/mise/config.toml (symlink from dotfiles?) - skipping mise runtimes"
    return 0
  fi
  log "Installing mise runtimes from config and setting global default (node@latest)..."
  if (eval "$(mise activate bash)" 2>/dev/null && mise install && mise use -g node@latest); then
    log_success "mise runtimes installed and node set as global default"
  else
    log_warning "mise install/use failed (non-fatal); run 'mise install' and 'mise use -g node@latest' manually if needed"
  fi
}

# ================================================================
# VALIDATION AND CLEANUP
# ================================================================

validate_environment() {
  log "Validating environment..."

  # Check if running on Arch Linux (or Arch-based derivative)
  if [[ ! -f /etc/arch-release ]]; then
    if [[ -f /etc/os-release ]]; then
      local os_id os_id_like
      os_id=$(grep "^ID=" /etc/os-release | cut -d'=' -f2 | tr -d '"')
      os_id_like=$(grep "^ID_LIKE=" /etc/os-release | cut -d'=' -f2 | tr -d '"')
      case "$os_id" in
      arch | cachyos | manjaro | endeavouros) ;;
      *)
        if [[ "$os_id_like" == *arch* ]]; then
          : # Arch derivative detected via ID_LIKE
        else
          error_exit "This script is designed for Arch Linux systems. Detected: $os_id"
        fi
        ;;
      esac
    else
      error_exit "This script is designed for Arch Linux systems"
    fi
  fi

  # Check if pacman is available
  if ! has_command pacman; then
    error_exit "pacman not found. This script requires pacman package manager."
  fi

  # Check available disk space (require at least 5GB)
  local available_space
  available_space=$(df "$HOME" | tail -1 | awk '{print $4}')
  if [[ $available_space -lt 5242880 ]]; then # 5GB in KB
    error_exit "Insufficient disk space. At least 5GB required."
  fi

  # Check network connectivity
  if ! curl -s --connect-timeout 5 https://archlinux.org >/dev/null; then
    error_exit "No internet connectivity or Arch repositories unreachable"
  fi

  log_success "Environment validation passed"
}

print_summary() {
  echo
  print_section "Installation Summary"
  echo
  echo -e "  ${GREEN}Installed:${NC} $INSTALLED_COUNT packages"
  echo -e "  ${YELLOW}Skipped:${NC}   $SKIPPED_COUNT packages (already installed)"
  echo -e "  ${RED}Failed:${NC}    $FAILED_COUNT packages"
  echo -e "  ${YELLOW}Optional:${NC}  $OPTIONAL_FAILED_COUNT packages failed but were ignored"

  if [[ ${#FAILED_PACKAGES[@]} -gt 0 ]]; then
    echo
    echo -e "${RED}Failed packages:${NC}"
    for pkg in "${FAILED_PACKAGES[@]}"; do
      echo "  - $pkg"
    done
  fi

  if [[ ${#OPTIONAL_FAILED_PACKAGES[@]} -gt 0 ]]; then
    echo
    echo -e "${YELLOW}Optional package failures:${NC}"
    for pkg in "${OPTIONAL_FAILED_PACKAGES[@]}"; do
      echo "  - $pkg"
    done
  fi

  echo
}

cleanup_cache() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would clean package cache"
    return 0
  fi

  print_section "Cleaning Package Cache"

  log "Removing orphaned packages..."
  local orphans=()
  mapfile -t orphans < <(pacman -Qtdq 2>/dev/null || true)
  if ((${#orphans[@]})); then
    sudo pacman -Rns "${orphans[@]}" --noconfirm
  else
    log "No orphaned packages to remove"
  fi

  # Remove stray download-* dirs/files (incomplete pacman/makepkg runs) so -Sc doesn't hit "Error reading fd"
  if [[ -d /var/cache/pacman/pkg ]]; then
    local count
    count=$(find /var/cache/pacman/pkg -maxdepth 1 -name 'download-*' 2>/dev/null | wc -l)
    if [[ "${count:-0}" -gt 0 ]]; then
      log "Removing ${count} stray download-* cache entry/entries..."
      sudo rm -rf /var/cache/pacman/pkg/download-* 2>/dev/null || true
    fi
  fi

  log "Cleaning pacman cache..."
  sudo pacman -Sc --noconfirm || log_warning "Failed to clean cache (run manually if you see 'Error reading fd')"

  if has_command yay; then
    log "Cleaning yay cache..."
    yay -Sc --noconfirm || log_warning "Failed to clean yay cache"
  fi

  log_success "Cache cleanup completed"
}

# ================================================================
# MAIN EXECUTION
# ================================================================

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
  --dry-run)
    DRY_RUN="true"
    shift
    ;;
  --skip-official)
    SKIP_OFFICIAL="true"
    shift
    ;;
  --skip-aur)
    SKIP_AUR="true"
    shift
    ;;
  --gaming)
    INSTALL_GAMING="true"
    shift
    ;;
  --non-interactive)
    NON_INTERACTIVE="true"
    shift
    ;;
  --force)
    FORCE_INSTALL="true"
    shift
    ;;
  -h | --help)
    usage 0
    ;;
  *)
    error_exit "Unknown option: $1. Use --help for usage information."
    ;;
  esac
done

main() {
  log "Starting Arch Linux package installation process..."
  [[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No packages will be installed"

  # Validate environment
  validate_environment

  # System update
  update_system

  # Install yay for AUR access
  install_yay

  # Install package groups
  install_core_packages
  install_development_tools
  install_cli_tools
  install_fonts
  install_desktop_apps
  install_gaming_packages
  install_network_tools
  install_cloud_tools
  install_tmux_tools
  install_media_tools
  install_libraries
  install_aur_packages

  # Mise runtimes last so node@latest is installed and activated (takes precedence in shell)
  install_mise_runtimes

  # Cleanup
  cleanup_cache

  # Print summary
  print_summary

  if [[ $FAILED_COUNT -gt 0 ]]; then
    log_warning "Installation completed with $FAILED_COUNT failed package(s)"
    exit 1
  else
    log_success "Arch Linux package installation completed successfully"
  fi
}

main "$@"
