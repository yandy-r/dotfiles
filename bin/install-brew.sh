#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "${0}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[BREW-INSTALL]"

# Auto-detect Homebrew path
if [[ "$(uname)" == "Darwin" ]]; then
  HOMEBREW_PREFIX="/opt/homebrew"
else
  HOMEBREW_PREFIX="/home/linuxbrew/.linuxbrew"
fi

PATH="$HOMEBREW_PREFIX/bin:$PATH"

# Configuration
DRY_RUN="${DRY_RUN:-false}"
SKIP_FORMULAS="${SKIP_FORMULAS:-false}"
SKIP_CASKS="${SKIP_CASKS:-false}"
SKIP_MAS="${SKIP_MAS:-false}"

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install Homebrew and common development tools.

OPTIONS:
    --dry-run           Show what would be installed without actually installing
    --skip-formulas     Skip formula installation
    --skip-casks        Skip cask installation (macOS only)
    --skip-mas          Skip Mac App Store installations (macOS only)
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DRY_RUN            Enable dry-run mode (true/false)
    SKIP_FORMULAS      Skip formula installation (true/false)
    SKIP_CASKS         Skip cask installation (true/false)
    SKIP_MAS           Skip Mac App Store installations (true/false)

EXAMPLES:
    $SCRIPT_NAME --dry-run
    $SCRIPT_NAME --skip-casks
    DRY_RUN=true $SCRIPT_NAME
EOF
  exit "${1:-0}"
}

# Logging functions
log() {
  echo "${LOG_PREFIX} [$(date '+%Y-%m-%d %H:%M:%S')] $*" >&2
}

error_exit() {
  log "ERROR: $1" >&2
  exit "${2:-1}"
}

check_command() {
  if ! command -v "$1" &>/dev/null; then
    return 1
  fi
  return 0
}

# Check if running in interactive mode
is_interactive() {
  [[ -t 0 ]] && [[ -t 1 ]]
}

# Prompt user for confirmation
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
    return 1
  fi

  echo -n "${LOG_PREFIX} ${prompt_message} [y/N]: " >&2
  read -r response

  # Default to "no" on empty input
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

install_with_retry() {
  local package="$1"
  local package_type="${2:-formula}" # formula, cask, or mas
  local max_attempts=3
  local attempt=1

  # Skip if already installed
  case "$package_type" in
  "cask")
    if brew list --cask "$package" &>/dev/null; then
      log "Cask $package already installed, skipping"
      return 0
    fi
    ;;
  "mas")
    if mas list | grep -q "$package"; then
      log "Mac App Store app $package already installed, skipping"
      return 0
    fi
    ;;
  *)
    if brew list "$package" &>/dev/null; then
      log "Formula $package already installed, skipping"
      return 0
    fi
    ;;
  esac

  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY-RUN: Would install $package_type $package"
    return 0
  fi

  while [[ $attempt -le $max_attempts ]]; do
    log "Installing $package_type $package (attempt $attempt/$max_attempts)"

    local install_cmd
    case "$package_type" in
    "cask") install_cmd="brew install --cask \"$package\"" ;;
    "mas") install_cmd="mas install \"$package\"" ;;
    *) install_cmd="brew install \"$package\"" ;;
    esac

    if eval "$install_cmd"; then
      log "Successfully installed $package_type $package"
      return 0
    else
      log "Failed to install $package_type $package on attempt $attempt"
      ((attempt++))
      [[ $attempt -le $max_attempts ]] && sleep 2
    fi
  done

  log "Failed to install $package_type $package after $max_attempts attempts"
  return 1
}

function brews() {
  log "Updating Homebrew..."
  brew update || error_exit "Failed to update Homebrew"
  brew install zsh
  brew install zsh-autosuggestions
  brew install zsh-completions
  brew install zsh-syntax-highlighting
  brew install pyenv

  #BREW CASKS
  echo "# --------------------------------------------------"
  echo "tapping brews"
  echo "# --------------------------------------------------"
  brew tap aws/tap
  brew tap ksonnet/tap
  brew tap wata727/tflint
  brew tap azure/draft
  brew tap stackery/tap
  brew tap hashicorp/tap
  brew tap BarutSRB/tap
  echo ""

  # RUN BREW
  echo "# --------------------------------------------------"
  echo "installing hombrew ports"
  echo "# --------------------------------------------------"
  brew -v update
  brew install ack
  brew install amazon-ecs-cli
  brew install ansible
  brew install ansible-language-server
  brew install asciinema
  brew install awscli
  brew install azure-cli
  brew install bash
  brew install bitwarden-cli
  brew install bottom
  brew install carapace
  brew install cfn-lint
  brew install clang-format
  brew install cmake
  brew install coreutils
  brew install curl
  brew install direnv
  brew install dotenvx/brew/dotenvx
  brew install exiftool
  brew install eza
  brew install fd
  brew install ffmpeg
  brew install flow
  brew install fping
  brew install freetype
  brew install fzf
  brew install bat
  brew install git-cliff
  brew install git-delta
  brew install hadolint
  brew install gawk
  brew install gcc
  brew install gh
  brew install btop
  brew install git
  brew install git-flow
  brew install git-lfs
  brew install gnu-sed
  brew install golang
  brew install golangci-lint
  brew install httpie
  # Note: draftbrew/tap/hping is deprecated, skipping
  brew install jq
  brew install lazygit
  brew install lazydocker
  brew install libjpeg
  brew install libgit2
  brew install libpng
  brew install lua
  brew install luarocks
  # mas is macOS-only (Mac App Store CLI)
  if [[ "$(uname)" == "Darwin" ]]; then
    brew install mas
  fi
  brew install mtr
  brew install netcat
  brew install nmap
  brew install omniwm
  brew install pandoc
  brew install pkg-config
  brew install pnpm
  brew install prettier
  brew install protobuf
  brew install python
  brew install pipx
  brew install ruby-lsp
  # reattach-to-user-namespace is macOS-only (tmux clipboard integration)
  if [[ "$(uname)" == "Darwin" ]]; then
    brew install reattach-to-user-namespace
  fi
  brew install rg
  brew install rtmpdump
  brew install ruby
  brew install screen
  brew install sipcalc
  brew install skaffold
  brew install speedtest-cli
  brew install tcpdump
  brew install hashicorp/tap/terraform
  brew install tflint
  brew install thefuck
  brew install file
  brew install gemini-cli
  brew install skaffold
  brew install tmux
  brew install tmux-mem-cpu-load
  brew install tmux-xpanes
  brew install tmuxinator-completion
  brew install tmux-sessionizer
  brew install tree
  brew install tree-sitter
  brew install tree-sitter-cli
  brew install watch
  brew install wget
  brew install xh
  brew install yq
  brew install zmq
  brew install zoxide
  echo "# --------------------------------------------------"
  echo ""

  echo "# --------------------------------------------------"
  echo "running brew cleanup"
  echo "# --------------------------------------------------"
  log "Running brew cleanup..."
  brew cleanup || log "Warning: brew cleanup failed"
}

casks() {
  # Only run on macOS - casks are macOS-specific GUI applications
  if [[ "$(uname)" != "Darwin" ]]; then
    log "Skipping cask installations (not running on macOS)"
    return 0
  fi

  log "Installing macOS cask applications..."
  brew install --cask "1password-cli@beta"
  brew install --cask "1password@beta"
  brew install --cask "aerospace"
  brew install --cask "alacritty"
  brew install --cask "antigravity"
  brew install --cask beekeeper-studio
  brew install --cask "chatgpt"
  brew install --cask "cursor"
  brew install --cask "cursor-cli"
  brew install --cask "discord"
  brew install --cask "dotnet-sdk"
  brew install --cask "drawio"
  brew install --cask "docker-desktop"
  brew install --cask "firefox"
  brew install --cask "font-jetbrains-mono-nerd-font"
  brew install --cask "ghostty"
  brew install --cask "gitkraken"
  brew install --cask "gitkraken-cli"
  brew install --cask "google-chrome"
  brew install --cask "iina"
  brew install --cask "iterm2@beta"
  brew install --cask "itermai"
  brew install --cask "mplayer-osx-extended"
  brew install --cask "obsidian"
  brew install --cask "postman"
  brew install --cask "powershell"
  brew install --cask "qbittorrent"
  brew install --cask "sublime-text@dev"
  brew install --cask "the-unarchiver"
  brew install --cask "visual-studio-code@insiders"
  brew install --cask "vlc"
  brew install --cask "wezterm"
  brew install --cask "zed@preview"
  brew install --cask "zight"
}

masApps=(
  # "1Password for Safari" = 1569813296
  1569813296
  # "Adblock Pro" = 1018301773
  1018301773
  # "Amphetamine" = 937984704
  937984704
  # "Boom2" = 948176063
  948176063
  # "CleanMyMac" = 1339170533
  1339170533
  # "Controller for HomeKit" = 1198176727
  1198176727
  # "Divvy" = 413857545
  413857545
  # "Dropzone 4" = 1485052491
  1485052491
  # "Fantastical" = 975937182
  975937182
  # "FileZilla Pro" = 1298486723
  1298486723
  # "Gifski" = 1351639930
  1351639930
  # "Grammarly" = 1462114288
  1462114288
  # "Infuse" = 1136220934
  1136220934
  # "Jump Desktop" = 524141863
  524141863
  # "Magnet" = 441258766
  441258766
  # "MediaInfo" = 510620098
  510620098
  # "Microsoft Remote Desktop" = 1295203466
  1295203466
  # "Notability" = 360593530
  360593530
  # "PDF Expert" = 1055273043
  1055273043
  # "Prime Video" = 545519333
  545519333
  # "Simplenote" = 692867256
  692867256
  # "Spark Mail" = 6445813049
  6445813049
  # "Telegram" = 747648890
  747648890
  # "Terminus" = 1176074088
  1176074088
  # "Things 3" = 904280696
  904280696
  # "TiviMax" = 1632953327
  1632953327
  # "Transmit 5" = 1436522307
  1436522307
  # "Video Speed Controller" = 1588368612
  1588368612
  # "WhatsApp" = 310633997
  310633997
  # "Wifi Explorer" = 494803304
  494803304
  # "xCode" = 497799835
  497799835
  # "Ulysses: Writing App"
  1225570693
  # "Byword"
  420212497
)
mas_apps() {
  # Only run on macOS
  if [[ "$(uname)" != "Darwin" ]]; then
    log "Skipping Mac App Store apps (not running on macOS)"
    return 0
  fi

  # Check if mas is installed
  if ! check_command mas; then
    log "mas (Mac App Store CLI) not found, skipping app installations"
    return 0
  fi

  echo "mas_apps"
  for app in "${masApps[@]}"; do
    if mas list | grep -q "$app"; then
      echo "App with ID $app is already installed"
    else
      echo "Installing app with ID $app"
      mas install "$app"
    fi
  done
}

update_homebrew() {
  log "Updating Homebrew..."
  if ! brew update; then
    error_exit "Failed to update Homebrew"
  fi
  log "Homebrew updated successfully"
}

install_homebrew() {
  if check_command brew; then
    log "Homebrew already installed"
    log "Updating Homebrew to latest version..."
    update_homebrew
    return 0
  fi

  log "Installing Homebrew..."
  echo ""
  log "WARNING: SECURITY WARNING"
  log "This will download and execute a remote script from:"
  log "    https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"
  log "The script will be piped directly to your shell."
  log "The script will request sudo access for installation."
  echo ""
  log "NOTE: For production use, consider pinning to a specific commit hash"
  log "      instead of using HEAD branch for better security."
  echo ""

  if ! prompt_confirmation "Do you want to proceed with downloading and executing the Homebrew installation script?"; then
    log "Installation cancelled by user"
    exit 0
  fi

  # NOTE: For production use, consider using a pinned commit hash instead of HEAD:
  # https://raw.githubusercontent.com/Homebrew/install/<COMMIT_HASH>/install.sh
  if ! /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"; then
    error_exit "Failed to install Homebrew"
  fi

  # Update PATH for current session
  eval "$($HOMEBREW_PREFIX/bin/brew shellenv)"
  log "Homebrew installed successfully"
}

validate_environment() {
  log "Validating environment..."

  # Check available disk space (require at least 5GB)
  local available_space
  available_space=$(df "$HOME" | tail -1 | awk '{print $4}')
  if [[ $available_space -lt 5242880 ]]; then # 5GB in KB
    error_exit "Insufficient disk space. At least 5GB required."
  fi

  # Check network connectivity
  if ! curl -s --connect-timeout 5 https://formulae.brew.sh >/dev/null; then
    error_exit "No internet connectivity or Homebrew servers unreachable"
  fi

  log "Environment validation passed"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
  --dry-run)
    DRY_RUN="true"
    shift
    ;;
  --skip-formulas)
    SKIP_FORMULAS="true"
    shift
    ;;
  --skip-casks)
    SKIP_CASKS="true"
    shift
    ;;
  --skip-mas)
    SKIP_MAS="true"
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
  log "Starting Homebrew installation process..."
  [[ "$DRY_RUN" == "true" ]] && log "DRY-RUN MODE: No packages will be installed"

  validate_environment
  install_homebrew

  local os_type
  os_type="$(uname)"

  case "$os_type" in
  "Linux")
    log "Linux detected, installing brew formulas"
    [[ "$SKIP_FORMULAS" != "true" ]] && brews
    ;;
  "Darwin")
    log "macOS detected, installing formulas, casks, and Mac App Store apps"
    [[ "$SKIP_FORMULAS" != "true" ]] && brews
    [[ "$SKIP_CASKS" != "true" ]] && casks
    [[ "$SKIP_MAS" != "true" ]] && mas_apps
    ;;
  *)
    log "Unsupported OS: $os_type, skipping package installation"
    ;;
  esac

  log "Homebrew installation process completed successfully"
}

main "$@"
