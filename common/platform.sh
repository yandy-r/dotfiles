#!/usr/bin/env bash
# ================================================================
# Cross-Platform Detection and Configuration
# ================================================================
#
# Purpose:
#   Provides comprehensive platform detection for cross-platform dotfiles
#   compatibility. Detects OS, architecture, container environment, and
#   package managers to enable platform-aware configuration loading.
#
# Source of truth:
#   The Go CLI (`dotf platform`) is canonical. setup_platform_env first loads
#   the cache written by `dotf platform --write`:
#     ${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles/platform.env
#   (XDG_CACHE_HOME is honored only when absolute). The file is parsed and
#   strictly validated by _dotfiles_load_platform_cache -- never sourced or
#   eval'd -- and is ignored unless it is a regular, non-symlink file owned by
#   the user, schema 1, written on this host, and contains exactly the
#   whitelisted keys with shell-safe values. It is also rejected when it
#   contradicts the running process: a different arch (Rosetta's
#   `arch -x86_64 zsh`; read fork-free from bash $HOSTTYPE / zsh $CPUTYPE) or
#   a container marker (/.dockerenv, /run/.containerenv) while the cache says
#   `none` (a container sharing $HOME and hostname). Only when the file does
#   NOT exist and `dotf` is on PATH does `dotf platform --write` run (once)
#   before a retry; an existing-but-rejected file never forks dotf. Otherwise
#   the shell detect_* functions below run as a fallback and export the same
#   platform variables.
#
# Dependencies:
#   - None (standalone, foundational script)
#
# Exported Variables:
#   DOTFILES_OS          - Detected OS family (macos, debian, redhat, arch, suse, linux, wsl, windows)
#   DOTFILES_ARCH        - CPU architecture (x86_64, arm64, arm)
#   DOTFILES_CONTAINER   - Container type (docker, kubernetes, wsl, none)
#   DOTFILES_PKG_MGR     - Package manager (homebrew, apt, dnf, yum, pacman, zypper)
#   DOTFILES_DISTRO      - Raw /etc/os-release ID (ubuntu, fedora, cachyos, ...; "unknown" if none)
#   DOTFILES_CODENAME    - Release codename (VERSION_CODENAME, else UBUNTU_CODENAME; may be empty)
#   DOTFILES_HOMEBREW_PATH - Homebrew installation prefix
#   DOTFILES_PLATFORM_SCHEMA, DOTFILES_PLATFORM_HOST
#                        - Cache schema and writer host; set ONLY when the
#                          values came from the dotf cache (unset on fallback)
#   DOTFILES_IS_SSH      - "true" if running in SSH session
#   DOTFILES_USE_OSC52   - "true" if OSC 52 clipboard should be used
#   DOTFILES_COPY_CMD    - Platform-appropriate clipboard copy command
#   DOTFILES_PASTE_CMD   - Platform-appropriate clipboard paste command
#   DOTFILES_OPEN_CMD    - Platform-appropriate file/URL opener command
#   XDG_CONFIG_HOME      - XDG config directory (default: ~/.config)
#   XDG_DATA_HOME        - XDG data directory (default: ~/.local/share)
#   XDG_CACHE_HOME       - XDG cache directory (default: ~/.cache)
#   XDG_STATE_HOME       - XDG state directory (default: ~/.local/state)
#
# Usage:
#   source common/platform.sh
#   # Variables are automatically exported
#   echo "Running on: $DOTFILES_OS ($DOTFILES_ARCH)"
#
# Disabling Auto-Setup:
#   export DOTFILES_NO_AUTO_SETUP=1
#   source common/platform.sh
#   # Then call setup_platform_env manually when ready
#
# ================================================================

# ================================================================
# CORE DETECTION FUNCTIONS
# ================================================================
# These functions detect the runtime environment characteristics.
# Each returns a standardized string for consistent cross-platform logic.

# Detect operating system
# Returns: macos, debian, redhat, arch, suse, linux, wsl, windows, or unknown
detect_os() {
    # Test seam: defaults are the real paths; the bats suite points the
    # DOTFILES_*_FILE overrides at fixture copies. `uname -s` stays live and is
    # stubbed as a shell function by tests instead.
    local os_release="${DOTFILES_OS_RELEASE_FILE:-/etc/os-release}"
    local proc_version="${DOTFILES_PROC_VERSION_FILE:-/proc/version}"
    case "$(uname -s)" in
        Darwin*)
            echo "macos"
            ;;
        Linux*)
            if [[ -f "$os_release" ]]; then
                . "$os_release"
                case "$ID" in
                    ubuntu|debian) echo "debian" ;;
                    fedora|fedora-asahi-remix|asahi-fedora-remix|rhel|centos|rocky|almalinux) echo "redhat" ;;
                    arch|cachyos|manjaro|endeavouros) echo "arch" ;;
                    opensuse*|sles) echo "suse" ;;
                    *)
                        # Fallback: use ID_LIKE for unknown derivatives
                        case "${ID_LIKE:-}" in
                            *debian*|*ubuntu*) echo "debian" ;;
                            *fedora*|*rhel*) echo "redhat" ;;
                            *arch*) echo "arch" ;;
                            *suse*) echo "suse" ;;
                            *) echo "linux" ;;
                        esac
                        ;;
                esac
            elif [[ -f "$proc_version" ]] && grep -qi microsoft "$proc_version"; then
                echo "wsl"
            else
                echo "linux"
            fi
            ;;
        CYGWIN*|MINGW*|MSYS*)
            echo "windows"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

# Print the per-OS module search path, most-generic-first, one relative
# directory name per line. This is the single source of truth for where
# common/{aliases,exports,functions}.sh look for their platform-specific
# siblings; nothing else should build those paths by hand.
#
# The first line is always the OS *family* directory (macos or linux). When the
# detected OS is a distro family within Linux, a second line names the distro
# overlay directory. Overlays layer on top of the family file instead of
# replacing it, so a future common/redhat/aliases.sh would refine
# common/linux/aliases.sh rather than shadow it. Only common/macos/ and
# common/linux/ exist today; the overlay slot is real, just unused.
#
# Args: $1 - OS token to resolve (default: $DOTFILES_OS, else detect_os)
# Returns: nothing for platforms with no module directory (windows, unknown)
os_module_dirs() {
    local os_type="${1:-${DOTFILES_OS:-$(detect_os)}}"

    case "$os_type" in
        macos)
            echo "macos"
            ;;
        debian|redhat|arch|suse|wsl|linux)
            echo "linux"
            if [[ "$os_type" != "linux" ]]; then
                echo "$os_type"
            fi
            ;;
    esac
}

# Get raw distro ID from /etc/os-release (e.g., "ubuntu", "cachyos", "arch")
# Used by scripts that need the specific distro name for repo URL construction
get_distro_name() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        echo "${ID:-unknown}"
    else
        echo "unknown"
    fi
}

# Get the release codename from /etc/os-release: VERSION_CODENAME, falling
# back to UBUNTU_CODENAME (e.g., "noble", "bookworm"). Prints nothing when
# neither is defined or the file is missing.
get_distro_codename() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        echo "${VERSION_CODENAME:-${UBUNTU_CODENAME:-}}"
    fi
}

# Detect CPU architecture
# Returns: x86_64, arm64, arm, or raw uname output for unknown architectures
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "x86_64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7l|armv6l) echo "arm" ;;
        *) echo "$(uname -m)" ;;
    esac
}

# Detect if running in a container environment
# Returns: docker, kubernetes, wsl, or none
detect_container() {
    if [[ -f /.dockerenv ]] || grep -q docker /proc/1/cgroup 2>/dev/null; then
        echo "docker"
    elif [[ -n "${KUBERNETES_SERVICE_HOST:-}" ]]; then
        echo "kubernetes"
    elif [[ -n "${WSL_DISTRO_NAME:-}" ]] || [[ -n "${WSL_INTEROP:-}" ]]; then
        echo "wsl"
    else
        echo "none"
    fi
}

# ================================================================
# PACKAGE MANAGER DETECTION
# ================================================================
# Identifies the primary package manager available on the system.
# Used for automated tool installation and dependency management.

# Print the first native (non-Homebrew) package manager on PATH.
# Returns 1 (printing nothing) when none is found.
_dotfiles_native_package_manager() {
    if command -v apt &> /dev/null; then
        echo "apt"
    elif command -v dnf &> /dev/null; then
        echo "dnf"
    elif command -v yum &> /dev/null; then
        echo "yum"
    elif command -v pacman &> /dev/null; then
        echo "pacman"
    elif command -v zypper &> /dev/null; then
        echo "zypper"
    else
        return 1
    fi
}

# Detect the primary package manager
# Returns: homebrew, apt, dnf, yum, pacman, zypper, or unknown
# Priority matches the canonical Go detector (internal/platform): on aarch64
# Linux-family hosts the native manager wins (Homebrew ships no aarch64-Linux
# bottles) and Homebrew is only a last resort; everywhere else Homebrew is
# probed first.
detect_package_manager() {
    local os_type arch
    os_type="${DOTFILES_OS:-$(detect_os)}"
    arch="${DOTFILES_ARCH:-$(detect_arch)}"

    case "$os_type" in
        debian|redhat|arch|suse|linux|wsl)
            if [[ "$arch" == "arm64" ]]; then
                if _dotfiles_native_package_manager; then
                    return 0
                elif command -v brew &> /dev/null; then
                    echo "homebrew"
                else
                    echo "unknown"
                fi
                return 0
            fi
            ;;
    esac

    if command -v brew &> /dev/null; then
        echo "homebrew"
    elif ! _dotfiles_native_package_manager; then
        echo "unknown"
    fi
}

# Get Homebrew installation path based on OS and architecture
# Returns: Homebrew prefix path or empty string if not available
# macOS ARM64:  /opt/homebrew
# macOS x86_64: /usr/local
# Linux family (incl. WSL): /home/linuxbrew/.linuxbrew
get_homebrew_path() {
    local os_type arch
    os_type="${DOTFILES_OS:-$(detect_os)}"
    arch="${DOTFILES_ARCH:-$(detect_arch)}"

    if command -v brew &> /dev/null; then
        brew --prefix
    elif [[ "$os_type" == "macos" ]]; then
        if [[ "$arch" == "arm64" ]]; then
            echo "/opt/homebrew"
        else
            echo "/usr/local"
        fi
    else
        case "$os_type" in
            debian|redhat|arch|suse|linux|wsl) echo "/home/linuxbrew/.linuxbrew" ;;
        esac
    fi
}

# ================================================================
# TOOL DETECTION
# ================================================================
# Utility functions for checking tool/file/directory availability.
# These are used throughout the dotfiles for conditional configuration.

# Print the first argument found on PATH (e.g. `first_command bat batcat`
# resolves Debian/Ubuntu's batcat/fdfind names); returns 1 if none is found.
# PATH lookup only (`type -P` / zsh `whence -p`): aliases and functions are
# ignored, so `alias bat=…` neither hides a real bat nor stands in for one.
# Usage: first_command bat batcat
first_command() {
    local _first_cmd
    for _first_cmd in "$@"; do
        if [[ -n "${ZSH_VERSION:-}" ]]; then
            whence -p "$_first_cmd" >/dev/null 2>&1 || continue
        else
            type -P "$_first_cmd" >/dev/null 2>&1 || continue
        fi
        printf '%s\n' "$_first_cmd"
        return 0
    done
    return 1
}

# Resolve bat/batcat and fd/fdfind once, and (re)export the settings that
# depend on them: DOTFILES_BAT_CMD/DOTFILES_FD_CMD (plain vars, not exported;
# child shells re-resolve), PAGER/MANPAGER/LESS and the FZF commands.
# exports.sh calls this, and aliases.sh calls it again: the rc order is
# platform -> exports -> paths -> aliases, so the exports-time lookup runs
# before paths.sh has added ~/.cargo/bin, Homebrew and friends, and the
# aliases-time re-run picks up tools only reachable via paths.sh.
dotfiles_resolve_tool_cmds() {
    DOTFILES_BAT_CMD="$(first_command bat batcat)" || DOTFILES_BAT_CMD=""
    DOTFILES_FD_CMD="$(first_command fd fdfind)" || DOTFILES_FD_CMD=""

    if [[ -n "${DOTFILES_BAT_CMD:-}" ]]; then
        export PAGER="$DOTFILES_BAT_CMD --paging=always"
        export MANPAGER="$DOTFILES_BAT_CMD --language=man --paging=always"
    elif has_command less; then
        export PAGER="less"
        export MANPAGER="less -X" # Don't clear screen after quitting
        # Less options: -R raw colours, -F quit if one screen, -X no clear
        export LESS="-R -F -X"
    else
        export PAGER="more"
        export MANPAGER="more"
    fi

    export FZF_DEFAULT_OPTS="--height 40% --layout=reverse --border --inline-info"
    if [[ -n "${DOTFILES_FD_CMD:-}" ]]; then
        export FZF_DEFAULT_COMMAND="$DOTFILES_FD_CMD --type f --hidden --follow --exclude .git 2>/dev/null || find . -type f -not -path \"*/\\.git/*\""
        export FZF_ALT_C_COMMAND="$DOTFILES_FD_CMD --type d --hidden --follow --exclude .git 2>/dev/null || find . -type d -not -path \"*/\\.git/*\""
    else
        export FZF_DEFAULT_COMMAND='find . -type f -not -path "*/\.git/*"'
        export FZF_ALT_C_COMMAND='find . -type d -not -path "*/\.git/*"'
    fi
    export FZF_CTRL_T_COMMAND="$FZF_DEFAULT_COMMAND"
}

# Check if command exists in PATH
# Usage: has_command git && echo "Git installed"
has_command() {
    command -v "$1" &> /dev/null
}

# ================================================================
# PATH HELPERS
# ================================================================
# Functions for safe PATH manipulation. Prevents duplicates and
# validates directory existence before adding to PATH.

# Add directory to PATH if it exists and is not already present
# Args: $1 = path, $2 = position ("start" or "end", default: "end")
# Usage: add_to_path "/usr/local/bin" "start"
add_to_path() {
    local new_path="$1"
    local position="${2:-end}"  # start or end
    
    if [[ -d "$new_path" ]] && [[ ":$PATH:" != *":$new_path:"* ]]; then
        if [[ "$position" == "start" ]]; then
            export PATH="$new_path:$PATH"
        else
            export PATH="$PATH:$new_path"
        fi
    fi
}

# Add to PATH at start if directory exists
prepend_path() {
    add_to_path "$1" "start"
}

# Add to PATH at end if directory exists
append_path() {
    add_to_path "$1" "end"
}

# ================================================================
# PLATFORM-SPECIFIC SETTINGS
# ================================================================
# Functions that return platform-appropriate commands for common
# operations like clipboard access and file opening.

# Get platform-appropriate clipboard copy command
# Returns: pbcopy (macOS), clip.exe (WSL), xclip/xsel (Linux), or cat (fallback)
# WSL is recognized via is_wsl, so a WSL distro whose DOTFILES_OS is its
# family (debian, ...) still gets the Windows clipboard.
get_copy_command() {
    if [[ "${DOTFILES_OS:-$(detect_os)}" == "macos" ]]; then
        echo "pbcopy"
    elif is_wsl; then
        echo "clip.exe"
    elif has_command xclip; then
        echo "xclip -selection clipboard"
    elif has_command xsel; then
        echo "xsel --clipboard --input"
    else
        echo "cat"
    fi
}

# Get platform-appropriate clipboard paste command
# Returns: pbpaste (macOS), powershell Get-Clipboard (WSL), xclip/xsel (Linux)
get_paste_command() {
    if [[ "${DOTFILES_OS:-$(detect_os)}" == "macos" ]]; then
        echo "pbpaste"
    elif is_wsl; then
        echo "powershell.exe Get-Clipboard"
    elif has_command xclip; then
        echo "xclip -selection clipboard -o"
    elif has_command xsel; then
        echo "xsel --clipboard --output"
    else
        echo "echo '(no clipboard available)'"
    fi
}

# Get platform-appropriate command to open files/URLs
# Returns: open (macOS), explorer.exe (WSL), xdg-open (Linux)
get_open_command() {
    if [[ "${DOTFILES_OS:-$(detect_os)}" == "macos" ]]; then
        echo "open"
    elif is_wsl; then
        echo "explorer.exe"
    else
        echo "xdg-open"
    fi
}

# ================================================================
# PLATFORM CACHE (dotf platform --write)
# ================================================================
# The Go CLI is the canonical platform detector. `dotf platform --write`
# records its result in a small KEY=VALUE file that this loader validates and
# exports without forking, sourcing, or eval'ing anything.

# Set the caller's _pc_file to the default cache path:
# ${XDG_CACHE_HOME:-$HOME/.cache}/dotfiles/platform.env, XDG_CACHE_HOME
# honored only when absolute. Assigns instead of printing so callers need no
# command substitution (bash and zsh both scope locals dynamically).
_dotfiles_platform_cache_path() {
    local _pc_cache_home="${XDG_CACHE_HOME:-}"
    case "$_pc_cache_home" in
        /*) ;;
        *) _pc_cache_home="${HOME:-}/.cache" ;;
    esac
    _pc_file="$_pc_cache_home/dotfiles/platform.env"
}

# Load and export the platform cache written by `dotf platform --write`.
# Args: $1 - cache file (default: see _dotfiles_platform_cache_path)
# Returns: 0 after exporting DOTFILES_PLATFORM_SCHEMA, DOTFILES_PLATFORM_HOST,
#          DOTFILES_OS, DOTFILES_ARCH, DOTFILES_CONTAINER, DOTFILES_PKG_MGR,
#          DOTFILES_DISTRO, DOTFILES_CODENAME and DOTFILES_HOMEBREW_PATH;
#          1 (exporting nothing) unless the file is a regular, non-symlink file
#          owned by the user, every non-blank non-comment line is
#          `export KEY=VALUE` with a whitelisted KEY (each exactly once) and a
#          VALUE of only [A-Za-z0-9._/+-], schema is 1, host matches, the
#          cached arch matches the running shell's arch (bash $HOSTTYPE /
#          zsh $CPUTYPE, when recognizable), and the cached container is not
#          `none` inside a container (/.dockerenv or /run/.containerenv).
# Pure builtins only: this runs on every shell startup.
# $1 is optional (tests/tools pass an explicit path); startup calls omit it.
# shellcheck disable=SC2120
_dotfiles_load_platform_cache() {
    local _pc_file="${1:-}" _pc_line _pc_rest _pc_key _pc_val _pc_rt_arch
    local _pc_schema="" _pc_host="" _pc_os="" _pc_arch="" _pc_container=""
    local _pc_pkg="" _pc_distro="" _pc_codename="" _pc_brew="" _pc_seen=""

    [[ -n "$_pc_file" ]] || _dotfiles_platform_cache_path

    [[ -f "$_pc_file" && ! -L "$_pc_file" && -O "$_pc_file" ]] || return 1

    while IFS= read -r _pc_line || [[ -n "$_pc_line" ]]; do
        case "$_pc_line" in
            '' | '#'*) continue ;;
            'export '*=*) ;;
            *) return 1 ;;
        esac
        _pc_rest="${_pc_line#export }"
        _pc_key="${_pc_rest%%=*}"
        _pc_val="${_pc_rest#*=}"
        # Explicit character list: bracket ranges are locale-dependent in
        # older shells.
        case "$_pc_val" in
            *[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._/+-]*) return 1 ;;
        esac
        case " $_pc_seen " in
            *" $_pc_key "*) return 1 ;;
        esac
        case "$_pc_key" in
            DOTFILES_PLATFORM_SCHEMA) _pc_schema="$_pc_val" ;;
            DOTFILES_PLATFORM_HOST) _pc_host="$_pc_val" ;;
            DOTFILES_OS) _pc_os="$_pc_val" ;;
            DOTFILES_ARCH) _pc_arch="$_pc_val" ;;
            DOTFILES_CONTAINER) _pc_container="$_pc_val" ;;
            DOTFILES_PKG_MGR) _pc_pkg="$_pc_val" ;;
            DOTFILES_DISTRO) _pc_distro="$_pc_val" ;;
            DOTFILES_CODENAME) _pc_codename="$_pc_val" ;;
            DOTFILES_HOMEBREW_PATH) _pc_brew="$_pc_val" ;;
            *) return 1 ;;
        esac
        _pc_seen="$_pc_seen $_pc_key"
    done < "$_pc_file"

    for _pc_key in DOTFILES_PLATFORM_SCHEMA DOTFILES_PLATFORM_HOST DOTFILES_OS \
        DOTFILES_ARCH DOTFILES_CONTAINER DOTFILES_PKG_MGR DOTFILES_DISTRO \
        DOTFILES_CODENAME DOTFILES_HOMEBREW_PATH; do
        case " $_pc_seen " in
            *" $_pc_key "*) ;;
            *) return 1 ;;
        esac
    done

    [[ "$_pc_schema" == "1" ]] || return 1
    [[ -n "$_pc_host" && "$_pc_host" == "${HOSTNAME:-${HOST:-}}" ]] || return 1

    # Runtime arch, fork-free: zsh's $CPUTYPE / bash's $HOSTTYPE reflect the
    # running binary, so a Rosetta shell (`arch -x86_64 zsh`) on an arm64 Mac
    # rejects an arm64 cache. Unset or unrecognized values skip the check.
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        _pc_rt_arch="${CPUTYPE:-}"
    else
        _pc_rt_arch="${HOSTTYPE:-}"
    fi
    case "$_pc_rt_arch" in
        x86_64|amd64) _pc_rt_arch="x86_64" ;;
        aarch64|arm64) _pc_rt_arch="arm64" ;;
        armv6l|armv7l) _pc_rt_arch="arm" ;;
        *) _pc_rt_arch="" ;;
    esac
    [[ -z "$_pc_rt_arch" || "$_pc_rt_arch" == "$_pc_arch" ]] || return 1

    # A container sharing $HOME and hostname with its host must not inherit
    # the host's `none`.
    if [[ "$_pc_container" == "none" ]] && [[ -f /.dockerenv || -f /run/.containerenv ]]; then
        return 1
    fi

    export DOTFILES_PLATFORM_SCHEMA="$_pc_schema"
    export DOTFILES_PLATFORM_HOST="$_pc_host"
    export DOTFILES_OS="$_pc_os"
    export DOTFILES_ARCH="$_pc_arch"
    export DOTFILES_CONTAINER="$_pc_container"
    export DOTFILES_PKG_MGR="$_pc_pkg"
    export DOTFILES_DISTRO="$_pc_distro"
    export DOTFILES_CODENAME="$_pc_codename"
    export DOTFILES_HOMEBREW_PATH="$_pc_brew"
    return 0
}

# ================================================================
# ENVIRONMENT SETUP
# ================================================================
# Main initialization function that exports all platform detection
# results as environment variables for use by other scripts.

# Initialize platform environment and export all detection results
# Called automatically on source unless DOTFILES_NO_AUTO_SETUP is set
# Exports: DOTFILES_OS, DOTFILES_ARCH, DOTFILES_CONTAINER, etc.
# Order: dotf cache -> (only if the cache file is absent) `dotf platform
# --write` + one retry -> shell detect_*. An existing file that fails
# validation goes straight to the fallback, so a stale or tampered cache never
# forks dotf on every shell start. A --write that leaves no file (old dotf,
# unwritable dir) drops a `.attempted` marker so later shells skip it; any
# successful write removes the marker. Only the owner of $HOME runs the writer,
# so `sudo` with the user's HOME never leaves a root-owned cache behind.
# shellcheck disable=SC2119
setup_platform_env() {
    local _pc_file
    if _dotfiles_load_platform_cache; then
        # Go blanks a value with unsafe characters; recover a Homebrew prefix
        # that was dropped that way.
        if [[ "$DOTFILES_PKG_MGR" == "homebrew" && -z "$DOTFILES_HOMEBREW_PATH" ]]; then
            DOTFILES_HOMEBREW_PATH="$(get_homebrew_path)"
        fi
    else
        _dotfiles_platform_cache_path
        local _pc_dir="${_pc_file%/*}"
        if [[ ! -e "$_pc_file" && ! -e "$_pc_file.attempted" && ! -L "$_pc_file.attempted" ]] \
            && [[ -O "${HOME:-/}" ]] && command -v dotf &> /dev/null; then
            # `|| true`: never abort a `set -e` script that sources this file.
            dotf platform --write > /dev/null 2>&1 || true
            if [[ ! -e "$_pc_file" ]]; then
                # Only the leaf (dotfiles/) needs 0700, matching dotf's MkdirAll.
                # shellcheck disable=SC2174
                { [[ -d "$_pc_dir" ]] || mkdir -p -m 700 "$_pc_dir"; } 2> /dev/null || true
                if [[ -d "$_pc_dir" && -O "$_pc_dir" && ! -L "$_pc_file.attempted" ]]; then
                    : > "$_pc_file.attempted" 2> /dev/null || true
                fi
            fi
        fi
        if ! _dotfiles_load_platform_cache; then
            # Fallback: shell detection. OS and arch are exported first so the
            # helpers below read them instead of re-detecting.
            unset DOTFILES_PLATFORM_SCHEMA DOTFILES_PLATFORM_HOST
            export DOTFILES_OS
            DOTFILES_OS="$(detect_os)"
            export DOTFILES_ARCH
            DOTFILES_ARCH="$(detect_arch)"
            export DOTFILES_CONTAINER
            DOTFILES_CONTAINER="$(detect_container)"
            export DOTFILES_PKG_MGR
            DOTFILES_PKG_MGR="$(detect_package_manager)"
            export DOTFILES_DISTRO
            DOTFILES_DISTRO="$(get_distro_name)"
            export DOTFILES_CODENAME
            DOTFILES_CODENAME="$(get_distro_codename)"
            export DOTFILES_HOMEBREW_PATH
            DOTFILES_HOMEBREW_PATH="$(get_homebrew_path)"
        fi
    fi

    # Export SSH and clipboard detection
    if is_ssh; then
        export DOTFILES_IS_SSH="true"
    else
        export DOTFILES_IS_SSH="false"
    fi

    if should_use_osc52; then
        export DOTFILES_USE_OSC52="true"
    else
        export DOTFILES_USE_OSC52="false"
    fi

    # Export platform commands
    export DOTFILES_COPY_CMD
    DOTFILES_COPY_CMD="$(get_copy_command)"
    export DOTFILES_PASTE_CMD
    DOTFILES_PASTE_CMD="$(get_paste_command)"
    export DOTFILES_OPEN_CMD
    DOTFILES_OPEN_CMD="$(get_open_command)"

    # Set up common environment variables
    export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
    export XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
    export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
    export XDG_STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}"

    # Create XDG directories if they don't exist
    mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME" "$XDG_STATE_HOME"
}

# ================================================================
# UTILITY FUNCTIONS
# ================================================================
# Convenience functions for platform checking and debugging.

# Print all detected platform information (useful for debugging)
# Usage: print_platform_info
print_platform_info() {
    echo "Platform Information:"
    echo "  OS: $(detect_os)"
    echo "  Distro: $(get_distro_name)"
    echo "  Codename: $(get_distro_codename)"
    echo "  Architecture: $(detect_arch)"
    echo "  Container: $(detect_container)"
    echo "  Package Manager: $(detect_package_manager)"
    echo "  Homebrew Path: $(get_homebrew_path)"
    echo "  Copy Command: $(get_copy_command)"
    echo "  Paste Command: $(get_paste_command)"
    echo "  Open Command: $(get_open_command)"
}

# Platform check functions - return 0 (true) or 1 (false)
# Usage: is_macos && echo "On macOS"
# They read the cached DOTFILES_OS / DOTFILES_CONTAINER / DOTFILES_ARCH exported
# by setup_platform_env, so no subshell runs on the hot path; only when a var is
# unset or empty do they fall back to live detect_* (YAN-1340).
is_macos() {
    case "${DOTFILES_OS:-$(detect_os)}" in
        macos) return 0 ;;
    esac
    return 1
}

is_linux() {
    case "${DOTFILES_OS:-$(detect_os)}" in
        debian|redhat|arch|suse|linux) return 0 ;;
    esac
    return 1
}

is_wsl() {
    case "${DOTFILES_OS:-$(detect_os)}" in
        wsl) return 0 ;;
    esac
    case "${DOTFILES_CONTAINER:-$(detect_container)}" in
        wsl) return 0 ;;
    esac
    return 1
}

is_container() {
    case "${DOTFILES_CONTAINER:-$(detect_container)}" in
        none) return 1 ;;
    esac
    return 0
}

# Architecture check functions
is_arm64() {
    case "${DOTFILES_ARCH:-$(detect_arch)}" in
        arm64) return 0 ;;
    esac
    return 1
}

is_x86_64() {
    case "${DOTFILES_ARCH:-$(detect_arch)}" in
        x86_64) return 0 ;;
    esac
    return 1
}

# Check if running over SSH (detected via SSH_* environment variables)
is_ssh() {
    [[ -n "${SSH_CLIENT:-}" ]] || [[ -n "${SSH_TTY:-}" ]] || [[ -n "${SSH_CONNECTION:-}" ]]
}

# Check if OSC 52 clipboard escape sequence should be used
# OSC 52 allows copying to local clipboard over SSH in supported terminals
# Returns 0 if SSH session + capable terminal detected
should_use_osc52() {
    # Use OSC 52 if we're over SSH and have a capable terminal
    if is_ssh; then
        # Check if terminal supports OSC 52
        case "${TERM_PROGRAM:-}" in
            iTerm.app|WezTerm|Alacritty|vscode) return 0 ;;
        esac
        case "${TERM:-}" in
            *256color|xterm*|screen*|tmux*|alacritty*|wezterm*) return 0 ;;
        esac
    fi
    return 1
}

# ================================================================
# INITIALIZATION
# ================================================================
# Automatically set up platform environment when this script is sourced.
# Disable by setting DOTFILES_NO_AUTO_SETUP=1 before sourcing.

if [[ -z "${DOTFILES_NO_AUTO_SETUP:-}" ]]; then
    setup_platform_env
fi