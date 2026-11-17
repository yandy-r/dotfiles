#!/usr/bin/env bash
# ================================================================
# Cross-Platform Environment Variables
# ================================================================
#
# Purpose:
#   Defines environment variables for consistent shell behavior across
#   platforms. Organizes exports by category for maintainability.
#
# Dependencies:
#   - common/platform.sh (for DOTFILES_OS, has_command, is_macos, etc.)
#
# Categories:
#   - Core: XDG directories, locale settings
#   - Editor: EDITOR, VISUAL, GIT_EDITOR
#   - Pager: PAGER, MANPAGER, LESS options
#   - Terminal: Colors, themes, TERM handling
#   - History: Shell history configuration
#   - Development: Language-specific (Go, Rust, Python, Node.js)
#   - Cloud: AWS, GCP, Azure, Kubernetes settings
#   - Tools: FZF, ripgrep, bat, eza configurations
#   - Security: GPG, telemetry opt-outs
#   - Platform-specific: macOS, Linux, WSL overrides
#
# ================================================================

# Source platform detection if not already loaded
if [[ -z "$DOTFILES_OS" ]]; then
    # Intentional dual-shell path resolution: BASH_SOURCE in bash, ${(%):-%x} in zsh.
    # shellcheck disable=SC2296
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)"
    if [[ -f "$SCRIPT_DIR/platform.sh" ]]; then
        source "$SCRIPT_DIR/platform.sh"
    fi
fi

# Ubuntu's USERGROUPS_ENAB login default is umask 002, which leaves files group-writable;
# permission-checking tooling (e.g. itcn/website's Neon CLI trust check) refuses those.
umask 022

# ================================================================
# CORE ENVIRONMENT VARIABLES
# ================================================================
# XDG Base Directory Specification compliance ensures config files
# are stored in standardized locations across applications.
# See: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html

# XDG Base Directory Specification
export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
export XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
export XDG_STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"
export XDG_DATA_DIRS="$HOME/.local/share/flatpak/exports/share:/var/lib/flatpak/exports/share:${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"

# Create XDG directories
mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME" "$XDG_STATE_HOME"

# ================================================================
# FILE-BACKED EXPORTS
# ================================================================
# Machine-specific values live in individual systemd environment.d files
# under $XDG_CONFIG_HOME/environment.d (<name>.conf, one KEY=value line) so
# the repo stays portable across hosts. read_env_file takes the path without
# the extension, prefers <path>.conf, falls back to a bare <path>, and prints
# only the value: blank/comment lines are skipped, a leading `export ` and
# `KEY=` are stripped, as are matching surrounding quotes. A file holding a
# bare value (no KEY=) is still accepted.

read_env_file() {
    local env_file="$1"
    local line value=""

    if [[ -r "${env_file}.conf" ]]; then
        env_file="${env_file}.conf"
    elif [[ ! -r "$env_file" ]]; then
        return 1
    fi

    while IFS= read -r line || [[ -n "$line" ]]; do
        line="${line%$'\r'}"
        # Trim leading whitespace; skip blank lines and comments.
        line="${line#"${line%%[![:space:]]*}"}"
        [[ -z "$line" || "$line" == \#* ]] && continue
        line="${line#export }"
        if [[ "$line" =~ ^[A-Za-z0-9_.-]+= ]]; then
            line="${line#*=}"
        fi
        value="$line"
        break
    done < "$env_file"

    # Strip one pair of matching surrounding quotes.
    if [[ ${#value} -ge 2 ]]; then
        if [[ "$value" == \"*\" || "$value" == \'*\' ]]; then
            value="${value:1:${#value}-2}"
        fi
    fi

    [[ -n "$value" ]] || return 1
    printf '%s' "$value"
}

# if github_api_token="$(read_env_file "$XDG_CONFIG_HOME/environment.d/github_api_token")"; then
#     export GITHUB_API_TOKEN="$github_api_token"
#     export GITHUB_TOKEN="$github_api_token"
#     export GITHUB_PERSONAL_ACCESS_TOKEN="$github_api_token"
# else
#     unset GITHUB_API_TOKEN
#     unset GITHUB_TOKEN
#     unset GITHUB_PERSONAL_ACCESS_TOKEN
# fi

if dropbox_dir="$(read_env_file "$XDG_CONFIG_HOME/environment.d/dropbox_dir")"; then
    export DROPBOX_DIR="$dropbox_dir"
else
    unset DROPBOX_DIR
fi

if tokenhop_openai_base_url="$(read_env_file "$XDG_CONFIG_HOME/environment.d/tokenhop_openai_base_url")"; then
    export TOKENHOP_OPENAI_BASE_URL="$tokenhop_openai_base_url"
else
    unset TOKENHOP_OPENAI_BASE_URL
fi

if tokenhop_openai_model="$(read_env_file "$XDG_CONFIG_HOME/environment.d/tokenhop_openai_model")"; then
    export TOKENHOP_OPENAI_MODEL="$tokenhop_openai_model"
else
    unset TOKENHOP_OPENAI_MODEL
fi

# The 1Password service-account token stays file-backed: every secret below
# is read with it, so it must be exported first.
if op_service_account_token="$(read_env_file "$XDG_CONFIG_HOME/environment.d/op_service_account_token")"; then
    export OP_SERVICE_ACCOUNT_TOKEN="$op_service_account_token"
else
    unset OP_SERVICE_ACCOUNT_TOKEN
fi
unset op_service_account_token

# ================================================================
# 1PASSWORD-BACKED SECRETS
# ================================================================
# Read with the service-account token above, so no desktop prompt opens on
# shell start. `op whoami` first proves the token authenticates; then a single
# `op inject` resolves every reference in one call (~0.35s total, vs ~0.35s
# per secret with `op read`), capped at 5s so a network outage can't hang the
# shell. Each entry is NAME=op://... ; a secret that can't be read, or whose
# field is empty, stays unset. Shell Env Secrets (EnvSecrets vault, section
# `env`) holds one field per variable, labelled with the variable name.
_op_secret_refs=(
    "TEA_API_TOKEN=op://EnvSecrets/Shell Env Secrets/env/TEA_API_TOKEN"
    "GREPTILE_API_KEY=op://EnvSecrets/Shell Env Secrets/env/GREPTILE_API_KEY"
    "POSTMAN_API_KEY=op://EnvSecrets/Shell Env Secrets/env/POSTMAN_API_KEY"
    "CONTEXT7_API_KEY=op://EnvSecrets/Shell Env Secrets/env/CONTEXT7_API_KEY"
    "XERO_CLIENT_ID=op://EnvSecrets/Shell Env Secrets/env/XERO_CLIENT_ID"
    "XERO_CLIENT_SECRET=op://EnvSecrets/Shell Env Secrets/env/XERO_CLIENT_SECRET"
    "NINE_ROUTER_API_KEY=op://EnvSecrets/Shell Env Secrets/env/NINE_ROUTER_API_KEY"
    "TOKENHOP_OPENCODE_API_KEY=op://EnvSecrets/Shell Env Secrets/env/TOKENHOP_OPENCODE_API_KEY"
    "TOKENHOP_OPENAI_API_KEY=op://EnvSecrets/Shell Env Secrets/env/TOKENHOP_OPENAI_API_KEY"
    "CLAUDE_CODE_TOKENHOP_API_KEY=op://EnvSecrets/Shell Env Secrets/env/CLAUDE_CODE_TOKENHOP_API_KEY"
    "CLAUDE_CODE_OAUTH_TOKEN=op://EnvSecrets/Shell Env Secrets/env/CLAUDE_CODE_OAUTH_TOKEN"
    "GEMINI_API_KEY=op://EnvSecrets/Shell Env Secrets/env/GEMINI_API_KEY"
    "OPENCODE_PASSWORD=op://EnvSecrets/Shell Env Secrets/env/OPENCODE_PASSWORD"
    "CARTIO_LAB_MCP_TOKEN=op://EnvSecrets/Cartio Lab Secrets/ctrl/CARTIO_LAB_MCP_TOKEN"
)

_op_template=""
for _op_ref in "${_op_secret_refs[@]}"; do
    unset "${_op_ref%%=*}"
    _op_template+="${_op_ref%%=*}={{ ${_op_ref#*=} }}"$'\n'
done
unset TEA_TOKEN

if [[ -n "${OP_SERVICE_ACCOUNT_TOKEN:-}" ]] && command -v op >/dev/null 2>&1 &&
    timeout 5 op whoami >/dev/null 2>&1 &&
    _op_env="$(printf '%s' "$_op_template" | timeout 5 op inject 2>/dev/null)"; then
    while IFS= read -r _op_line; do
        [[ "$_op_line" == [A-Z]*=?* ]] || continue
        export "${_op_line%%=*}=${_op_line#*=}"
    done <<<"$_op_env"
fi

# Aliases of 1Password-backed secrets.
[[ -n "${TEA_API_TOKEN:-}" ]] && export TEA_TOKEN="$TEA_API_TOKEN"

unset _op_secret_refs _op_ref _op_template _op_env _op_line

# ================================================================
# EDITOR CONFIGURATION
# ================================================================
# Sets the default text editor for shell commands, git commits, etc.
# Priority: nvim > vim > nano > vi

# Set preferred editor with fallbacks
if has_command nvim; then
    export EDITOR="nvim"
    export VISUAL="nvim"
elif has_command vim; then
    export EDITOR="vim"
    export VISUAL="vim"
elif has_command nano; then
    export EDITOR="nano"
    export VISUAL="nano"
else
    export EDITOR="vi"
    export VISUAL="vi"
fi

# Git editor (prefer GUI editors for commits)
export GIT_EDITOR="$EDITOR"

# ================================================================
# PAGER CONFIGURATION
# ================================================================
# Configures the pager for viewing command output and man pages.
# Priority: bat/batcat (with syntax highlighting) > less > more.
# Resolution (incl. the FZF commands below) lives in
# dotfiles_resolve_tool_cmds (platform.sh); aliases.sh re-runs it after
# paths.sh, because the rc order loads exports.sh before paths.sh.

if command -v dotfiles_resolve_tool_cmds >/dev/null 2>&1; then
    dotfiles_resolve_tool_cmds
fi

# ================================================================
# TERMINAL CONFIGURATION
# ================================================================
# Terminal emulator settings, color support, and theme configuration.
# Handles modern terminals (Ghostty, WezTerm) with graceful fallbacks.

# WezTerm theme configuration
export WEZTERM_THEME="Catppuccin Mocha"
export WEZTERM_AUTO_THEME=false

# Terminal type - handle Ghostty and other modern terminals
if [ "$TERM" = "xterm-ghostty" ]; then
    # Fallback for Ghostty if terminfo is not available
    if command -v infocmp >/dev/null 2>&1; then
        if ! infocmp xterm-ghostty >/dev/null 2>&1; then
            export TERM="xterm-256color"
        fi
    else
        # infocmp not available, use fallback
        export TERM="xterm-256color"
    fi
elif [ -z "$TERM" ]; then
    export TERM="xterm-256color"
fi

# Enable colors in terminal
export CLICOLOR=1

# Color settings for ls
if is_macos; then
    export LSCOLORS="ExFxCxDxBxegedabagacad"
else
    export LS_COLORS="di=1;34:ln=1;35:so=1;32:pi=1;33:ex=1;31:bd=1;36:cd=1;33:su=1;35:sg=1;33:tw=1;34:ow=1;34"
fi

# Grep colors
export GREP_COLOR="1;33"      # Yellow
export GREP_COLORS="mt=1;33"  # Match text yellow

# ================================================================
# HISTORY CONFIGURATION
# ================================================================
# Shell command history settings. Separate configs for bash/zsh
# due to different option names and behaviors.

# History file locations (shell-specific)
if [[ -n "${ZSH_VERSION:-}" ]]; then
    export HISTFILE="$HOME/.zsh_history"
    export HISTSIZE=10000000
    export SAVEHIST=$HISTSIZE
else
    export HISTFILE="$HOME/.bash_history"
    export HISTSIZE=100000
    export HISTFILESIZE=$HISTSIZE
    export HISTCONTROL="ignoreboth:erasedups"
fi

# History timestamp format
export HISTTIMEFORMAT="%F %T "

# Commands to ignore in history
export HISTIGNORE="ls:ls *:cd:cd -:cd ~:pwd:exit:date:history:clear:cls:* --help:bg:fg"

# ================================================================
# LANGUAGE AND LOCALE
# ================================================================
# UTF-8 encoding for proper international character support.

# Pick the best available UTF-8 locale (minimal Ubuntu images often lack en_US.UTF-8)
dotfiles_to_lower() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

# Fold a locale name to a comparable form: lowercase, and collapse the codeset
# separator so glibc's `locale -a` spelling (de_DE.utf8) matches the canonical
# request spelling (de_DE.UTF-8). Without this every non-en_US locale fails to
# match and gets silently rewritten below.
dotfiles_normalize_locale() {
  local normalized
  normalized="$(dotfiles_to_lower "$1")"
  printf '%s' "${normalized//utf-8/utf8}"
}

locale_exists() {
  local target="${1:-}"
  local entry target_lower entry_lower

  [[ -n "$target" ]] || return 1
  target_lower="$(dotfiles_normalize_locale "$target")"

  while IFS= read -r entry; do
    entry="${entry//[[:space:]]/}"
    [[ -z "$entry" ]] && continue
    entry_lower="$(dotfiles_normalize_locale "$entry")"
    if [[ "$entry_lower" == "$target_lower" ]]; then
      return 0
    fi
  done <<EOF
$(locale -a 2>/dev/null)
EOF

  return 1
}

resolve_dotfiles_locale() {
  local entry entry_lower

  if ! command -v locale >/dev/null 2>&1; then
    echo "C.UTF-8"
    return
  fi

  while IFS= read -r entry; do
    entry="${entry//[[:space:]]/}"
    [[ -z "$entry" ]] && continue
    entry_lower="$(dotfiles_to_lower "$entry")"
    case "$entry_lower" in
    en_us.utf-8 | en_us.utf8)
      echo "en_US.UTF-8"
      return
      ;;
    esac
  done <<EOF
$(locale -a 2>/dev/null)
EOF

  while IFS= read -r entry; do
    entry="${entry//[[:space:]]/}"
    [[ -z "$entry" ]] && continue
    entry_lower="$(dotfiles_to_lower "$entry")"
    case "$entry_lower" in
    c.utf-8 | c.utf8)
      echo "C.UTF-8"
      return
      ;;
    esac
  done <<EOF
$(locale -a 2>/dev/null)
EOF

  echo "C"
}

pick_dotfiles_locale() {
  local candidate="$1"

  if [[ -n "$candidate" ]] && locale_exists "$candidate"; then
    echo "$candidate"
  else
    resolve_dotfiles_locale
  fi
}

DOTFILES_LANG="$(pick_dotfiles_locale "${LANG:-}")"
DOTFILES_LC_CTYPE="$(pick_dotfiles_locale "${LC_CTYPE:-}")"
export LANG="$DOTFILES_LANG"
export LC_CTYPE="$DOTFILES_LC_CTYPE"

# LC_ALL is only normalised when the user already set it: inventing one would
# override every other LC_* category the user deliberately chose.
if [[ -n "${LC_ALL:-}" ]]; then
  DOTFILES_LC_ALL="$(pick_dotfiles_locale "$LC_ALL")"
  export LC_ALL="$DOTFILES_LC_ALL"
fi

unset DOTFILES_LANG DOTFILES_LC_ALL DOTFILES_LC_CTYPE

# ================================================================
# DEVELOPMENT ENVIRONMENT
# ================================================================
# Language-specific environment variables for Go, Rust, Python, Node.js, etc.
# These set up GOPATH, CARGO_HOME, virtual environments, and package managers.

# Go environment
export GOPATH="${GOPATH:-$HOME/go}"
export GOBIN="$GOPATH/bin"
export GO111MODULE="on"
export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.org}"

# Rust environment
export CARGO_HOME="${CARGO_HOME:-$HOME/.cargo}"
export RUSTUP_HOME="${RUSTUP_HOME:-$HOME/.rustup}"

# Python environment
export PYTHONSTARTUP="$HOME/.pythonrc"
export PYTHONUSERBASE="$HOME/.local"
export PIP_USER="false"
export PIPX_HOME="$HOME/.local/share/pipx"
export PIPX_BIN_DIR="$HOME/.local/bin"

# Virtual environment settings
export WORKON_HOME="${WORKON_HOME:-$HOME/.virtualenvs}"
export VIRTUALENVWRAPPER_PYTHON="${VIRTUALENVWRAPPER_PYTHON:-$(command -v python3)}"
export VIRTUAL_ENV_DISABLE_PROMPT=1

# Node.js environment
# Note: NPM_CONFIG_PREFIX is set in paths.sh (defaults to ~/.npm-global)
export NODE_REPL_HISTORY="$XDG_DATA_HOME/node_repl_history"
export NPM_CONFIG_USERCONFIG="$XDG_CONFIG_HOME/npm/npmrc"

# Deno environment
export DENO_INSTALL="$HOME/.deno"

# Bun environment
export BUN_INSTALL="$HOME/.bun"

# ================================================================
# CLOUD PROVIDER CONFIGURATION
# ================================================================
# Settings for AWS, Google Cloud, and Azure CLI tools.
# Disables pagers and sets sensible defaults for interactive use.

# AWS CLI
export AWS_DEFAULT_OUTPUT="${AWS_DEFAULT_OUTPUT:-table}"
export AWS_CLI_AUTO_PROMPT="${AWS_CLI_AUTO_PROMPT:-on-partial}"
export AWS_PAGER=""  # Disable pager for AWS CLI
export AWS_SDK_LOAD_CONFIG=1

# Google Cloud
export CLOUDSDK_PYTHON="${CLOUDSDK_PYTHON:-$(command -v python3)}"

# Azure CLI
export AZURE_CONFIG_DIR="$XDG_CONFIG_HOME/azure"

# ================================================================
# KUBERNETES CONFIGURATION
# ================================================================
# Kubernetes tooling environment: kubectl, krew plugin manager, helm.
# KUBECONFIG can be extended with additional configs separated by colons.

# kubectl
export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
export KUBE_EDITOR="$EDITOR"

# Krew (kubectl plugin manager)
export KREW_ROOT="${KREW_ROOT:-$HOME/.krew}"

# Helm
export HELM_CACHE_HOME="$XDG_CACHE_HOME/helm"
export HELM_CONFIG_HOME="$XDG_CONFIG_HOME/helm"
export HELM_DATA_HOME="$XDG_DATA_HOME/helm"

# ================================================================
# DOCKER CONFIGURATION
# ================================================================
# Docker BuildKit and Compose settings for improved build performance.

# Docker buildx - enables BuildKit features
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Docker scan (disable telemetry)
export DOCKER_SCAN_SUGGEST=false

# ================================================================
# BUILD TOOLS CONFIGURATION
# ================================================================
# Compiler and build tool settings. Automatically detects CPU count
# for parallel builds with make and cmake.

# Make - parallel jobs based on CPU count
export MAKEFLAGS
MAKEFLAGS="-j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)"

# CMake
export CMAKE_GENERATOR="${CMAKE_GENERATOR:-Ninja}"

# Ninja (if available)
if has_command ninja; then
    export CMAKE_MAKE_PROGRAM
    CMAKE_MAKE_PROGRAM="$(command -v ninja)"
fi

# ================================================================
# SECURITY AND PRIVACY
# ================================================================
# GPG signing setup and telemetry opt-outs for various tools.
# Privacy-respecting defaults: disable analytics where possible.

# GPG TTY (required for signing commits in terminal)
export GPG_TTY
GPG_TTY=$(tty)

# SSH
export SSH_AUTH_SOCK="${SSH_AUTH_SOCK:-$(gpgconf --list-dirs agent-ssh-socket 2>/dev/null)}"

# Disable telemetry for various tools
export DOTNET_CLI_TELEMETRY_OPTOUT=1
export GATSBY_TELEMETRY_DISABLED=1
export NEXT_TELEMETRY_DISABLED=1
export DO_NOT_TRACK=1
export HOMEBREW_NO_ANALYTICS=1
export HOMEBREW_NO_AUTO_UPDATE=1
export HOMEBREW_NO_INSECURE_REDIRECT=1
export HOMEBREW_CASK_OPTS="--require-sha"

# ================================================================
# TOOL-SPECIFIC CONFIGURATION
# ================================================================
# Modern CLI tool configurations: FZF, ripgrep, bat, eza, delta.
# These tools are "better" replacements for standard Unix tools.

# FZF_DEFAULT_OPTS/FZF_DEFAULT_COMMAND/FZF_CTRL_T_COMMAND/FZF_ALT_C_COMMAND
# are set by dotfiles_resolve_tool_cmds (platform.sh), called in the pager
# section above and again from aliases.sh after paths.sh.

# Ripgrep configuration
export RIPGREP_CONFIG_PATH="$XDG_CONFIG_HOME/ripgrep/config"

# Bat (better cat) configuration
export BAT_THEME="${BAT_THEME:-Sublime Snazzy}"
export BAT_PAGER="less -RF"

# eza (better ls) configuration
export EZA_COLORS="da=1;34:gm=1;34"

# Delta (better diff) configuration
export DELTA_PAGER="less -RF"

# ================================================================
# WEZTERM WALLPAPER CONFIGURATION
# ================================================================
# WezTerm terminal wallpaper settings. These variables are read by
# the wezterm.lua configuration for dynamic background images.

# Enable WezTerm wallpapers
export WEZTERM_WALLPAPER="${WEZTERM_WALLPAPER:-true}"
export WEZTERM_WALLPAPER_CATEGORY="${WEZTERM_WALLPAPER_CATEGORY:-Fiction}"
export WEZTERM_WALLPAPER_OPACITY="${WEZTERM_WALLPAPER_OPACITY:-0.8}"
export WEZTERM_WALLPAPER_BRIGHTNESS="${WEZTERM_WALLPAPER_BRIGHTNESS:-0.1}"
export WEZTERM_WALLPAPER_ROTATE="${WEZTERM_WALLPAPER_ROTATE:-false}"
export WEZTERM_WALLPAPER_TIME_BASED="${WEZTERM_WALLPAPER_TIME_BASED:-true}"
export WEZTERM_WALLPAPER_TIME_BRIGHTNESS="${WEZTERM_WALLPAPER_TIME_BRIGHTNESS:-true}"
export WEZTERM_WALLPAPER_H_ALIGN="${WEZTERM_WALLPAPER_H_ALIGN:-Center}"
export WEZTERM_WALLPAPER_V_ALIGN="${WEZTERM_WALLPAPER_V_ALIGN:-Middle}"
export WEZTERM_WALLPAPER_REPEAT="${WEZTERM_WALLPAPER_REPEAT:-NoRepeat}"

# ================================================================
# ANSIBLE CONFIGURATION
# ================================================================
# Ansible configuration paths following XDG specification.

export ANSIBLE_CONFIG="$XDG_CONFIG_HOME/ansible/ansible.cfg"
export ANSIBLE_LOG_PATH="$HOME/.ansible/ansible.log"
# export ANSIBLE_VAULT_PASSWORD_FILE="$HOME/.ansible/vault_pass"  # Commented out - not using vault in dotfiles
export ANSIBLE_DEBUG="${ANSIBLE_DEBUG:-False}"

# ================================================================
# TERRAFORM CONFIGURATION
# ================================================================
# Terraform CLI configuration and plugin cache for faster init.

# Export TF_CLI_CONFIG_FILE only when the file is actually there: terraform
# prints "Warning: Unable to open CLI configuration file" on EVERY command when
# the variable points at a path that does not exist.
[[ -f "$XDG_CONFIG_HOME/terraform/terraformrc" ]] &&
    export TF_CLI_CONFIG_FILE="$XDG_CONFIG_HOME/terraform/terraformrc"

# The plugin cache dir is worse than a warning: terraform hard-FAILS with
# "The specified plugin cache dir … cannot be opened" when it is missing, and it
# never creates the directory itself. Create it, and only export the variable if
# that succeeded.
if [[ -d "$XDG_CACHE_HOME/terraform" ]] || mkdir -p "$XDG_CACHE_HOME/terraform" 2>/dev/null; then
    export TF_PLUGIN_CACHE_DIR="$XDG_CACHE_HOME/terraform"
fi

# ================================================================
# DIRENV CONFIGURATION
# ================================================================

# Configure direnv for cleaner output
# Keeping default direnv behavior for now

# ================================================================
# PLATFORM-SPECIFIC EXPORTS
# ================================================================
# OS-specific environment variables. These override or extend
# the common settings above based on the detected platform.

case "$DOTFILES_OS" in
    macos)
        # macOS specific exports
        export HOMEBREW_PREFIX="$DOTFILES_HOMEBREW_PATH"
        export HOMEBREW_CELLAR="$HOMEBREW_PREFIX/Cellar"
        export HOMEBREW_REPOSITORY="$HOMEBREW_PREFIX"

        # macOS Java
        if [[ -d "/Library/Java/JavaVirtualMachines" ]]; then
            latest_java=$(find /Library/Java/JavaVirtualMachines -maxdepth 1 -mindepth 1 -type d -printf '%f\n' 2>/dev/null | grep -E '^[0-9]+' | sort -V | tail -1)
            if [[ -n "$latest_java" ]]; then
                export JAVA_HOME="/Library/Java/JavaVirtualMachines/$latest_java/Contents/Home"
            fi
        fi

        # macOS specific paths
        if [[ -n "$DOTFILES_HOMEBREW_PATH" ]]; then
            export LDFLAGS="${LDFLAGS:-} -L$DOTFILES_HOMEBREW_PATH/lib"
            export CPPFLAGS="${CPPFLAGS:-} -I$DOTFILES_HOMEBREW_PATH/include"
            export PKG_CONFIG_PATH="${PKG_CONFIG_PATH:-}:$DOTFILES_HOMEBREW_PATH/lib/pkgconfig"
        fi

        # Disable Gatekeeper warnings
        export HOMEBREW_CASK_OPTS="${HOMEBREW_CASK_OPTS:-} --no-quarantine"
        ;;

    debian)
        # Debian/Ubuntu specific exports
        export DEBIAN_FRONTEND=noninteractive

        # Java on Ubuntu
        if [[ -d "/usr/lib/jvm/default-java" ]]; then
            export JAVA_HOME="/usr/lib/jvm/default-java"
        fi
        ;;

    redhat|fedora)
        # RedHat/Fedora specific exports
        if [[ -d "/usr/lib/jvm/java" ]]; then
            export JAVA_HOME="/usr/lib/jvm/java"
        fi
        ;;

    arch)
        # Arch Linux specific exports
        if [[ -d "/usr/lib/jvm/default" ]]; then
            export JAVA_HOME="/usr/lib/jvm/default"
        fi
        ;;

    wsl)
        # WSL specific exports
        export WSL_DISTRO_NAME="${WSL_DISTRO_NAME:-$(cat /proc/version | grep -oP 'Microsoft.*' | head -1)}"

        # Windows interop
        export WSLENV="USERPROFILE/up:USERNAME/u"

        # Fix for WSL clock drift
        export TZ
        TZ="$(cat /etc/timezone 2>/dev/null || echo 'UTC')"
        ;;
esac

# ================================================================
# PERFORMANCE OPTIMIZATIONS
# ================================================================
# Shell options and caching for faster operation.

# Enable dotfile globbing (show hidden files in globs)
shopt -s dotglob 2>/dev/null || true

# Enable bash completion caching
export BASH_COMPLETION_USER_DIR="$XDG_CONFIG_HOME/bash-completion"

# ================================================================
# LOAD PLATFORM-SPECIFIC EXPORTS
# ================================================================

# Load platform-specific exports if they exist: family file first, distro
# overlay second (see os_module_dirs in common/platform.sh). Neither
# common/linux/exports.sh nor common/macos/exports.sh exists today.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
while IFS= read -r os_module_dir; do
    [[ -z "$os_module_dir" ]] && continue
    if [[ -f "$SCRIPT_DIR/$os_module_dir/exports.sh" ]]; then
        source "$SCRIPT_DIR/$os_module_dir/exports.sh"
    fi
done <<EOF
$(os_module_dirs)
EOF
unset os_module_dir

# ================================================================
# CLEANUP EMPTY VARIABLES
# ================================================================
# Utility function to clean up path-like variables (PATH, MANPATH, etc.)
# Removes empty entries and duplicates.

# Remove empty or duplicate entries from colon-separated variables
# Usage: cleanup_path_var "LD_LIBRARY_PATH"
cleanup_path_var() {
    local var_name="$1"
    eval "local var_value=\"\${$var_name}\""

    if [[ -n "$var_value" ]]; then
        # Remove empty entries and duplicates
        local cleaned
        cleaned=$(echo "$var_value" | tr ':' '\n' | grep -v '^$' | awk '!seen[$0]++' | tr '\n' ':' | sed 's/:$//')
        export "$var_name"="$cleaned"
    fi
}

# Cleanup common path variables
cleanup_path_var "LD_LIBRARY_PATH"
cleanup_path_var "DYLD_LIBRARY_PATH"
cleanup_path_var "PKG_CONFIG_PATH"
cleanup_path_var "MANPATH"
