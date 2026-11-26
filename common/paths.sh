#!/usr/bin/env bash
# ================================================================
# Cross-Platform PATH Management
# ================================================================
#
# Purpose:
#   Manages PATH and related environment variables (MANPATH, FPATH,
#   LD_LIBRARY_PATH) for consistent tool access across platforms.
#
# Dependencies:
#   - common/platform.sh (for DOTFILES_OS, DOTFILES_HOMEBREW_PATH, is_macos, etc.)
#
# PATH Construction Order:
#   macOS:  User paths -> Homebrew -> GNU utils -> Language runtimes -> System
#   Linux:  User paths -> System -> Language runtimes -> Homebrew (supplementary)
#
# Key Functions:
#   - setup_all_paths()      - Main entry point, sets up everything
#   - prepend_path()         - Add to front of PATH
#   - append_path()          - Add to end of PATH
#   - finalize_path()        - Final cleanup pass, called late by both rc files
#   - print_path()           - Debug: show PATH entries
#
# Guard Variables:
#   DOTFILES_NO_AUTO_PATH_SETUP - Disable automatic setup on source
#   DOTFILES_PATHS_LOADED       - Prevents re-running (set after first load)
#   DOTFILES_CLEANUP_PATHS      - Set to "true" to remove non-existent paths
#
# ================================================================

# Source platform detection if not already loaded
if [[ -z "$DOTFILES_OS" ]]; then
    # Get the directory of this script.
    # Intentional dual-shell path resolution: BASH_SOURCE in bash, ${(%):-%x} in zsh.
    # shellcheck disable=SC2296
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${(%):-%x}}")" && pwd)"
    if [[ -f "$SCRIPT_DIR/platform.sh" ]]; then
        source "$SCRIPT_DIR/platform.sh"
    fi
fi

# ================================================================
# PATH SETUP FUNCTIONS
# ================================================================
# Core functions for safe PATH manipulation. All functions check for
# directory existence and prevent duplicate entries.

# Append a directory to PATH if it exists and is not already in PATH
# Usage: append_path "/usr/local/bin"
append_path() {
    local dir="$1"
    if [[ -n "$dir" && -d "$dir" && ":$PATH:" != *":$dir:"* ]]; then
        export PATH="$PATH:$dir"
    fi
}

# Prepend a directory to PATH (higher priority than append)
# Usage: prepend_path "$HOME/.local/bin"
prepend_path() {
    local dir="$1"
    if [[ -n "$dir" && -d "$dir" && ":$PATH:" != *":$dir:"* ]]; then
        export PATH="$dir:$PATH"
    fi
}

# Resolve bash-completion without sourcing it inside a function: older
# releases use top-level declare -A, which must remain global in bashrc.
_dotf_bash_completion_file() {
    local completion_script
    for completion_script in \
        "${DOTFILES_HOMEBREW_PATH:-}/etc/profile.d/bash_completion.sh" \
        /usr/share/bash-completion/bash_completion \
        /etc/bash_completion; do
        if [[ -f "$completion_script" ]]; then
            printf '%s\n' "$completion_script"
            return 0
        fi
    done
    return 1
}

# Remove a specific directory from PATH (safe, won't corrupt PATH)
# Usage: remove_from_path "/path/to/remove"
remove_from_path() {
    local path_to_remove="$1"

    # Safety check
    if [[ -z "$path_to_remove" || -z "$PATH" ]]; then
        return 0
    fi

    # Use sed to remove the path (more robust than manual parsing)
    local new_path
    new_path=$(echo "$PATH" | sed "s|:$path_to_remove||g" | sed "s|^$path_to_remove:||g")

    # Only update PATH if we have a valid result
    if [[ -n "$new_path" ]]; then
        export PATH="$new_path"
    fi
}

# Force a path to the very front of PATH (removes existing, then prepends)
# Use sparingly - for paths that MUST take priority
# Usage: force_path_front "$HOMEBREW_PATH/bin"
force_path_front() {
    local new_path="$1"

    # Safety checks
    if [[ -z "$new_path" || ! -d "$new_path" ]]; then
        return 0
    fi

    # Ensure we have a valid PATH before manipulation
    if [[ -z "$PATH" ]]; then
        export PATH="/usr/bin:/bin:/usr/sbin:/sbin"
    fi

    # Remove if already exists, then prepend
    remove_from_path "$new_path"
    export PATH="$new_path:$PATH"
}

# Fix PATH order after macOS path_helper reordering
# Problem: macOS /etc/zprofile runs path_helper which moves system paths to front
# Solution: Re-organize PATH with proper priority for each platform
#
# Final order on macOS: Priority paths -> Other -> Homebrew -> System
# Final order on Linux: Priority paths -> System -> Other -> Homebrew
#
# Callers (finalize_path, zsh/zprofile) run this on macOS only: on Linux it
# would push prepended entries such as mise shims behind /usr/bin.
fix_homebrew_path_order() {
    local brew_path="$DOTFILES_HOMEBREW_PATH"

    if [[ -n "$brew_path" && -d "$brew_path" ]]; then
        # First, deduplicate PATH to avoid duplicate entries
        PATH=$(echo "$PATH" | tr ':' '\n' | awk '!seen[$0]++' | tr '\n' ':' | sed 's/:$//')

        # Collect custom paths based on platform
        # Order matters: earlier in array = earlier in PATH
        # Note: We list paths even if they don't exist, to catch them if they're in current PATH
        local priority_paths=()

        # Dotfiles venv (highest priority on all platforms)
        priority_paths+=("$DOTFILES_ROOT/.venv/bin")

        # Carapace completions
        priority_paths+=("$HOME/.config/carapace/bin")

        # Platform-specific priority ordering
        if is_macos; then
            # macOS: Homebrew paths come before system paths
            priority_paths+=("$brew_path/bin")
            priority_paths+=("$brew_path/sbin")
            priority_paths+=("$HOME/.bin")
            priority_paths+=("$HOME/.local/bin")
            priority_paths+=("$HOME/.local/go/bin")
            priority_paths+=("/usr/local/sbin")
            priority_paths+=("${HOME}/.opencode/bin")
            priority_paths+=("${KREW_ROOT:-$HOME/.krew}/bin")
            priority_paths+=("$brew_path/opt/gnu-sed/libexec/gnubin")
            priority_paths+=("$brew_path/opt/gawk/libexec/gnubin")
        else
            # Linux: User paths first, but NOT Homebrew (it goes with other paths)
            priority_paths+=("$HOME/.bin")
            priority_paths+=("$HOME/.local/bin")
            priority_paths+=("$HOME/.local/go/bin")
            priority_paths+=("${HOME}/.opencode/bin")
            priority_paths+=("${KREW_ROOT:-$HOME/.krew}/bin")
        fi

        # Build a set of priority paths for easy lookup
        local priority_list
        priority_list=":$(IFS=:; echo "${priority_paths[*]}"):"

        # Separate system, homebrew, and other paths
        local system_paths=""
        local homebrew_paths=""
        local other_paths=""

        # Split PATH using each shell's array syntax (including bash 3.2).
        local -a path_array
        if [[ -n "${ZSH_VERSION:-}" ]]; then
            # shellcheck disable=SC2296
            path_array=("${(@s/:/)PATH}")
        else
            IFS=: read -r -a path_array <<<"$PATH"
        fi

        for dir in "${path_array[@]}"; do
            # Skip empty entries
            [[ -z "$dir" ]] && continue

            # Skip if it's already in our priority list
            [[ "$priority_list" == *":$dir:"* ]] && continue

            # Check if it's a Homebrew path
            if [[ "$dir" == "$brew_path"* ]]; then
                [[ -z "$homebrew_paths" ]] && homebrew_paths="$dir" || homebrew_paths="$homebrew_paths:$dir"
            # Check if it's a system path
            elif [[ "$dir" == /usr/bin || "$dir" == /bin || "$dir" == /usr/sbin || "$dir" == /sbin || "$dir" == /usr/local/bin || "$dir" == /System/* || "$dir" == /Library/* || "$dir" == /var/run/* || "$dir" == /Applications/* || "$dir" == /usr/local/share/* ]]; then
                [[ -z "$system_paths" ]] && system_paths="$dir" || system_paths="$system_paths:$dir"
            else
                # Everything else (cargo, go, etc.) goes in the middle
                [[ -z "$other_paths" ]] && other_paths="$dir" || other_paths="$other_paths:$dir"
            fi
        done

        # Rebuild PATH based on platform
        local new_path=""
        if is_macos; then
            # macOS: priority paths → other paths → homebrew paths → system paths
            for p in "${priority_paths[@]}"; do
                [[ -d "$p" ]] || continue
                [[ -z "$new_path" ]] && new_path="$p" || new_path="$new_path:$p"
            done
            [[ -n "$other_paths" ]] && new_path="$new_path:$other_paths"
            [[ -n "$homebrew_paths" ]] && new_path="$new_path:$homebrew_paths"
            [[ -n "$system_paths" ]] && new_path="$new_path:$system_paths"
        else
            # Linux: priority paths → system paths → other paths → homebrew paths
            for p in "${priority_paths[@]}"; do
                [[ -d "$p" ]] || continue
                [[ -z "$new_path" ]] && new_path="$p" || new_path="$new_path:$p"
            done
            [[ -n "$system_paths" ]] && new_path="$new_path:$system_paths"
            [[ -n "$other_paths" ]] && new_path="$new_path:$other_paths"
            [[ -n "$homebrew_paths" ]] && new_path="$new_path:$homebrew_paths"
        fi

        export PATH="$new_path"
    fi
}

# Set up Homebrew paths with platform-aware priority
# macOS: Homebrew FIRST (provides modern tools over BSD defaults)
# Linux: Homebrew AFTER system (supplements native packages)
setup_homebrew_paths() {
    local brew_path="$DOTFILES_HOMEBREW_PATH"

    if [[ -n "$brew_path" ]]; then
        # macOS: Homebrew FIRST (force to front for modern tools)
        # Linux: Homebrew AFTER system paths (append for supplementary tools)
        if is_macos; then
            # Force core Homebrew paths to front (in reverse order so bin comes first)
            force_path_front "$brew_path/sbin"
            force_path_front "$brew_path/bin"

            # Add GNU utilities paths (prepend so they override macOS defaults)
            # Order matters: later calls end up earlier in PATH
            force_path_front "$brew_path/opt/gnu-units/libexec/gnubin"
            force_path_front "$brew_path/opt/gnu-time/libexec/gnubin"
            force_path_front "$brew_path/opt/gnu-indent/libexec/gnubin"
            force_path_front "$brew_path/opt/gnu-getopt/libexec/gnubin"
            force_path_front "$brew_path/opt/ed/libexec/gnubin"
            force_path_front "$brew_path/opt/wdiff/bin"
            force_path_front "$brew_path/opt/watch/bin"
            force_path_front "$brew_path/opt/gawk/libexec/gnubin"
            force_path_front "$brew_path/opt/diffutils/bin"
            force_path_front "$brew_path/opt/findutils/libexec/gnubin"
            force_path_front "$brew_path/opt/grep/libexec/gnubin"
            force_path_front "$brew_path/opt/gnu-which/libexec/gnubin"
            force_path_front "$brew_path/opt/gnu-tar/libexec/gnubin"
            force_path_front "$brew_path/opt/gnu-sed/libexec/gnubin"
            force_path_front "$brew_path/opt/coreutils/libexec/gnubin"
        else
            # Linux: Append Homebrew paths (native system tools take priority)
            append_path "$brew_path/bin"
            append_path "$brew_path/sbin"

            # GNU utilities are less important on Linux (native versions usually better)
            # But add them for completeness
            append_path "$brew_path/opt/coreutils/libexec/gnubin"
            append_path "$brew_path/opt/gnu-sed/libexec/gnubin"
            append_path "$brew_path/opt/gnu-tar/libexec/gnubin"
            append_path "$brew_path/opt/gnu-which/libexec/gnubin"
            append_path "$brew_path/opt/grep/libexec/gnubin"
            append_path "$brew_path/opt/findutils/libexec/gnubin"
            append_path "$brew_path/opt/diffutils/bin"
            append_path "$brew_path/opt/gawk/libexec/gnubin"
            append_path "$brew_path/opt/watch/bin"
            append_path "$brew_path/opt/wdiff/bin"
            append_path "$brew_path/opt/ed/libexec/gnubin"
            append_path "$brew_path/opt/gnu-getopt/libexec/gnubin"
            append_path "$brew_path/opt/gnu-indent/libexec/gnubin"
            append_path "$brew_path/opt/gnu-time/libexec/gnubin"
            append_path "$brew_path/opt/gnu-units/libexec/gnubin"
        fi

        # Language-specific paths (append on all platforms)
        append_path "$brew_path/opt/ruby/bin"
        append_path "$brew_path/opt/python@3.11/bin"
        append_path "$brew_path/opt/python@3.12/bin"
        append_path "$brew_path/opt/dotnet@8/bin"
        append_path "$brew_path/opt/sqlite/bin"
        append_path "$brew_path/opt/tcl-tk/bin"
        append_path "$brew_path/opt/libiconv/bin"

        # Set library paths
        if [[ -d "$brew_path/opt/libiconv/lib" ]]; then
            export LDFLAGS="${LDFLAGS:-} -L$brew_path/opt/libiconv/lib"
            export CPPFLAGS="${CPPFLAGS:-} -I$brew_path/opt/libiconv/include"
            export LIBRARY_PATH="${LIBRARY_PATH:-}:$brew_path/lib:$brew_path/opt/libiconv/lib"
        fi
    fi
}

# Set up standard system paths (/usr/local/bin, /usr/sbin, etc.)
# Also adds platform-specific paths (snap on Ubuntu, TeX on macOS)
setup_system_paths() {
    # Standard system paths
    prepend_path "/usr/local/bin"
    prepend_path "/usr/local/sbin"
    append_path "/usr/sbin"
    append_path "/sbin"

    # Platform-specific system paths
    case "$DOTFILES_OS" in
        macos)
            append_path "/System/Library/PrivateFrameworks/Apple80211.framework/Versions/Current/Resources"
            append_path "/Library/TeX/texbin"
            append_path "/Applications/Visual Studio Code.app/Contents/Resources/app/bin"
            ;;
        debian)
            append_path "/snap/bin"
            ;;
    esac
}

# Set up user-local paths (~/.local/bin, ~/.bin, language runtimes)
# These include: Cargo (Rust), Go, Deno, Bun, various CLI tools
setup_user_paths() {
    # Standard user paths
    prepend_path "$HOME/.local/bin"
    prepend_path "$HOME/.bin"
    prepend_path "$HOME/bin"

    # Dotfiles virtual environment (global access to development tools)
    if [[ -d "$DOTFILES_ROOT/.venv/bin" ]]; then
        prepend_path "$DOTFILES_ROOT/.venv/bin"
    fi

    # Official CLI tools (installed by package managers to their own directories)
    append_path "$HOME/.opencode/bin"           # OpenCode CLI
    append_path "$HOME/.claude/bin"             # Claude CLI
    append_path "$HOME/.cursor-agent/bin"       # Cursor Agent CLI
    append_path "$HOME/.coderabbit/bin"         # CodeRabbit CLI
    append_path "$HOME/.grok/bin"               # Grok CLI
    append_path "$HOME/.antigravity/antigravity/bin" # Antigravity CLI
    append_path "$HOME/.atuin/bin"              # Atuin shell history
    append_path "$HOME/.turso"                  # Turso database CLI

    # Development language paths
    append_path "$HOME/.cargo/bin"              # Rust
    append_path "$HOME/.deno/bin"               # Deno
    append_path "$HOME/.bun/bin"                # Bun
    setup_pnpm_paths                            # PNPM (v11+ bins in $PNPM_HOME/bin)

    # Go paths
    # Official Go toolchain (bootstrap installs it to ~/.local/go). Prepended so
    # it shadows an older distro Go in /usr/bin that can't satisfy go.mod.
    prepend_path "$HOME/.local/go/bin"
    if [[ -n "${GOPATH:-}" ]]; then
        append_path "$GOPATH/bin"
    elif [[ -d "$HOME/go" ]]; then
        export GOPATH="$HOME/go"
        append_path "$GOPATH/bin"
    fi

    # Python paths
    append_path "$HOME/.poetry/bin"
    append_path "$HOME/.local/lib/python*/site-packages"

    # mise version manager shims (replaces NVM, pyenv, etc.)
    # mise shims provide access to managed runtimes
    if [[ -d "$HOME/.local/share/mise/shims" ]]; then
        prepend_path "$HOME/.local/share/mise/shims"
    fi
}

# Set up development tool paths (Docker, Kubernetes, IDEs, JetBrains)
# These are typically installed to application-specific locations
setup_dev_tools_paths() {
    # Cloud CLI tools
    append_path "$HOME/.local/share/cloud-sql-proxy"

    # Kubernetes tools
    append_path "${KREW_ROOT:-$HOME/.krew}/bin"

    # Docker tools
    if is_macos; then
        append_path "/Applications/Docker.app/Contents/Resources/bin"
    fi

    # VS Code (if not already added)
    if is_linux && [[ ! ":$PATH:" == *"code"* ]]; then
        append_path "/usr/share/code/bin"
    fi

    # Cursor Server
    append_path "$HOME/.cursor-server/bin"

    # JetBrains Toolbox
    if is_macos; then
        append_path "$HOME/Library/Application Support/JetBrains/Toolbox/scripts"
    elif is_linux; then
        append_path "$HOME/.local/share/JetBrains/Toolbox/scripts"
    fi
}

# ================================================================
# MANPATH SETUP
# ================================================================
# Configure man page search paths for Homebrew and GNU tools.
# Ensures 'man ls' shows GNU coreutils docs when installed.

setup_manpath() {
    local brew_path="$DOTFILES_HOMEBREW_PATH"

    # Initialize MANPATH if empty
    if [[ -z "${MANPATH:-}" ]]; then
        MANPATH="$(manpath 2>/dev/null || echo '/usr/share/man:/usr/local/man')"
    fi

    # Add Homebrew man pages
    if [[ -n "$brew_path" ]]; then
        MANPATH="$brew_path/share/man:$MANPATH"

        # Add GNU utilities man pages
        MANPATH="$brew_path/opt/coreutils/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/findutils/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gnu-tar/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gnu-sed/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gawk/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gnu-indent/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gnu-getopt/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/grep/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gnu-time/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/make/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/diffutils/share/man:$MANPATH"
        MANPATH="$brew_path/opt/ed/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gnu-which/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/gnu-units/libexec/gnuman:$MANPATH"
        MANPATH="$brew_path/opt/wdiff/share/man:$MANPATH"
    fi

    # Add user man pages
    if [[ -d "$HOME/.local/share/man" ]]; then
        MANPATH="$HOME/.local/share/man:$MANPATH"
    fi

    export MANPATH
}

# ================================================================
# LIBRARY PATHS
# ================================================================
# Dynamic library paths for linking. CAUTION: Setting LD_LIBRARY_PATH
# globally on Linux can break system binaries. See comments below.

setup_library_paths() {
    local brew_path="$DOTFILES_HOMEBREW_PATH"

    # Dynamic library paths
    case "$DOTFILES_OS" in
        macos)
            # macOS uses DYLD_LIBRARY_PATH
            if [[ -n "$brew_path" ]]; then
                export DYLD_LIBRARY_PATH="${DYLD_LIBRARY_PATH:-}:$brew_path/lib"
            fi
            ;;
        *)
            # Linux: DO NOT set LD_LIBRARY_PATH by default!
            # Setting LD_LIBRARY_PATH causes system binaries (like SSH, git, gh) that were
            # compiled against system OpenSSL to find Homebrew's newer OpenSSL instead,
            # resulting in "OpenSSL version mismatch" errors.
            #
            # Instead, let binaries use their system-linked libraries naturally.
            # For Homebrew-installed binaries, they will use their own RPATH to find
            # Homebrew libraries correctly.
            #
            # If specific tools need Homebrew libraries, set LD_LIBRARY_PATH temporarily
            # for just those tools, or reinstall them with Homebrew to get proper linking.
            :  # No-op: intentionally not setting LD_LIBRARY_PATH on Linux
            ;;
    esac

    # Package config paths (safe to set - used at compile time, not runtime)
    if [[ -n "$brew_path" ]]; then
        export PKG_CONFIG_PATH="${PKG_CONFIG_PATH:-}:$brew_path/lib/pkgconfig"
    fi
    export PKG_CONFIG_PATH="${PKG_CONFIG_PATH:-}:/usr/local/lib/pkgconfig:$HOME/.local/lib/pkgconfig"
}

# ================================================================
# FPATH SETUP (ZSH COMPLETIONS)
# ================================================================
# Configure Zsh function path for completion scripts.
# Only applies when running in Zsh.
#
# This is the ONE place completion directories go on fpath: it runs (via
# paths.sh) before the single compinit in common/completions.sh, so every entry
# here is seen by that compinit. A directory appended to fpath later in zshrc
# needs a second compinit, which costs ~150ms and rewrites ~/.zcompdump.
# Each `fpath=(dir …)` prepends, so the LAST entry below wins on a name clash.

setup_fpath() {
    local brew_path="$DOTFILES_HOMEBREW_PATH"

    # Only set up FPATH in Zsh
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        # Add Homebrew completions
        if [[ -n "$brew_path" ]]; then
            fpath=("$brew_path/share/zsh/site-functions" "${fpath[@]}")
            fpath=("$brew_path/share/zsh-completions" "${fpath[@]}")
        fi

        # Add user completions (only if they exist)
        [[ -d "$HOME/.zsh/completions" ]] && fpath=("$HOME/.zsh/completions" "${fpath[@]}")
        [[ -d "$XDG_CONFIG_HOME/zsh/completions" ]] && fpath=("$XDG_CONFIG_HOME/zsh/completions" "${fpath[@]}")
        # Standard XDG data dir for user zsh functions; `dotf completion zsh
        # --install` writes _dotf here.
        [[ -d "${XDG_DATA_HOME:-$HOME/.local/share}/zsh/site-functions" ]] &&
            fpath=("${XDG_DATA_HOME:-$HOME/.local/share}/zsh/site-functions" "${fpath[@]}")

        # Add system completions
        fpath=("/usr/local/share/zsh/site-functions" "${fpath[@]}")
        fpath=("/usr/share/zsh/site-functions" "${fpath[@]}")

        # Repo-owned completions (_op, _bws, _fabric, …) outrank system ones.
        [[ -d "$DOTFILES_ROOT/zsh/completions" ]] && fpath=("$DOTFILES_ROOT/zsh/completions" "${fpath[@]}")

        # Installer-managed completion dirs, formerly appended by their own
        # zshrc blocks followed by an extra compinit each.
        [[ -d "$HOME/.zfunc" ]] && fpath=("$HOME/.zfunc" "${fpath[@]}")                                     # rfmedia et al.
        [[ -d "$HOME/.docker/completions" ]] && fpath=("$HOME/.docker/completions" "${fpath[@]}")           # Docker Desktop
        [[ -d "$HOME/.grok/completions/zsh" ]] && fpath=("$HOME/.grok/completions/zsh" "${fpath[@]}")       # grok

        export fpath
    fi
}

# ================================================================
# LANGUAGE-SPECIFIC SETUPS
# ================================================================
# Language runtime environment setup. These configure GOPATH, CARGO,
# Python paths, and Node.js (NVM/mise) environments.

# Go environment - sets GOPATH, GOBIN, module settings
setup_go_env() {
    # Set GOPATH if not set
    if [[ -z "${GOPATH:-}" ]]; then
        if [[ -d "$HOME/go" ]]; then
            export GOPATH="$HOME/go"
        else
            export GOPATH="$HOME/dev/go"
        fi
    fi

    # Set GOBIN
    export GOBIN="$GOPATH/bin"

    # Go modules
    export GO111MODULE="on"

    # Go proxy settings
    export GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}"
    export GOSUMDB="${GOSUMDB:-sum.golang.org}"
}

# Rust environment - sources cargo env if available
setup_rust_env() {
    if [[ -f "$HOME/.cargo/env" ]]; then
        source "$HOME/.cargo/env"
    fi
}

# Python environment - sets up pip user installs, virtualenv settings
setup_python_env() {
    # Python startup file
    export PYTHONSTARTUP="$HOME/.pythonrc"

    # Pip user installs
    export PYTHONUSERBASE="$HOME/.local"

    # Skip launching Python if there cannot be any macOS user scripts to add.
    if is_macos && [[ -d "$HOME/Library/Python" ]]; then
        local python_version
        python_version=$(python3 -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')" 2>/dev/null || echo "3.11")
        append_path "$HOME/Library/Python/$python_version/bin"
    fi

    # Virtual environment settings
    export WORKON_HOME="${WORKON_HOME:-$HOME/.virtualenvs}"
    export VIRTUALENVWRAPPER_PYTHON="${VIRTUALENVWRAPPER_PYTHON:-$(which python3)}"
}

# pnpm 11+ stores globally installed bins in $PNPM_HOME/bin (not $PNPM_HOME).
# Keep $PNPM_HOME itself on PATH so leftover v10 globals still resolve.
# append_path is a no-op when the directory does not exist yet.
setup_pnpm_paths() {
    local pnpm_home="${PNPM_HOME:-$HOME/.local/share/pnpm}"
    if [[ -d "$pnpm_home" ]]; then
        export PNPM_HOME="$pnpm_home"
        append_path "$PNPM_HOME/bin"
        append_path "$PNPM_HOME"
    fi
}

# Node.js environment setup
setup_node_env() {
    # Set npm global prefix to user-writable location (avoids sudo for global installs)
    # Skip when mise manages node — mise handles the global prefix via shims
    if [[ -z "${NPM_CONFIG_PREFIX:-}" ]] && ! command -v mise &>/dev/null; then
        export NPM_CONFIG_PREFIX="$HOME/.npm-global"
    fi
    if [[ -n "${NPM_CONFIG_PREFIX:-}" ]]; then
        append_path "$NPM_CONFIG_PREFIX/bin"
    fi

    # Yarn global directory
    if has_command yarn; then
        yarn_global_dir="$(yarn global dir 2>/dev/null | tr -d '\n' 2>/dev/null)" || true
        if [[ -n "$yarn_global_dir" ]]; then
            append_path "$yarn_global_dir/node_modules/.bin"
        fi
    fi

    setup_pnpm_paths
}

# ================================================================
# CLEANUP FUNCTIONS
# ================================================================
# Functions to clean up PATH after all additions are complete.
# Removes duplicates, non-existent directories, and platform-wrong paths.

# Remove duplicate PATH entries while preserving order
cleanup_path() {
    # Convert PATH to array, remove duplicates, rejoin
    if [[ -n "${PATH:-}" ]]; then
        PATH="$(echo "$PATH" | tr ':' '\n' | grep -v '^$' | awk '!seen[$0]++' | tr '\n' ':' | sed 's/:$//')"
        export PATH
    fi
}

# Remove non-existent directories from PATH (can be slow on network mounts)
# Enable with: export DOTFILES_CLEANUP_PATHS=true
cleanup_nonexistent_paths() {
    local new_path=""
    local IFS=':'

    for dir in $PATH; do
        if [[ -n "$dir" ]] && [[ -d "$dir" ]]; then
            if [[ -z "$new_path" ]]; then
                new_path="$dir"
            else
                new_path="$new_path:$dir"
            fi
        fi
    done

    export PATH="$new_path"
}

# Remove unwanted paths (Nix, wrong-platform Homebrew, old Python versions)
# Prevents PATH pollution from unused package managers
cleanup_unwanted_paths() {
    local unwanted_paths=()

    # Remove Nix paths (not used)
    unwanted_paths+=("$HOME/.nix-profile/bin")
    unwanted_paths+=("/nix/var/nix/profiles/default/bin")

    # Platform-specific cleanup
    if is_linux; then
        # Remove macOS-specific Homebrew paths when on Linux
        unwanted_paths+=("/opt/homebrew/bin")
        unwanted_paths+=("/opt/homebrew/sbin")
        # Remove any macOS user paths that may have leaked in
        unwanted_paths+=("/Users/yandy/.turso")
    elif is_macos; then
        # Remove Linux-specific Homebrew paths when on macOS
        unwanted_paths+=("/home/linuxbrew/.linuxbrew/bin")
        unwanted_paths+=("/home/linuxbrew/.linuxbrew/sbin")
    fi

    # Remove old Python versions
    unwanted_paths+=("$HOME/Library/Python/3.9/bin")
    unwanted_paths+=("$HOME/Library/Python/3.8/bin")
    unwanted_paths+=("$HOME/Library/Python/3.7/bin")

    # Also remove any paths matching wrong-platform patterns using grep filters
    if is_linux; then
        PATH=$(echo "$PATH" | tr ':' '\n' | grep -v '/opt/homebrew' | grep -v '^/Users/' | tr '\n' ':' | sed 's/:$//')
    elif is_macos; then
        PATH=$(echo "$PATH" | tr ':' '\n' | grep -v '^/home/linuxbrew/.linuxbrew' | tr '\n' ':' | sed 's/:$//')
    fi

    # Remove any remaining Nix paths using pattern matching
    PATH=$(echo "$PATH" | tr ':' '\n' | grep -v '\.nix-profile' | grep -v '^/nix/' | tr '\n' ':' | sed 's/:$//')

    for unwanted_path in "${unwanted_paths[@]}"; do
        remove_from_path "$unwanted_path"
    done
}

# Final PATH pass, run once AFTER all shell initialization (both rc files call
# it late in startup; common/local.sh still runs after it, so machine-local
# PATH additions in dotfiles.local/paths.sh keep their position).
finalize_path() {
    local colons orig_count new_count

    # Entry counts via parameter expansion (no echo|wc pipelines); the count
    # for the debug line is recomputed only when DOTFILES_DEBUG is set.
    colons="${PATH//[^:]/}"
    orig_count=$(( ${#colons} + 1 ))

    cleanup_unwanted_paths
    cleanup_path
    is_macos && fix_homebrew_path_order

    if [[ -n "${DOTFILES_DEBUG:-}" ]]; then
        colons="${PATH//[^:]/}"
        new_count=$(( ${#colons} + 1 ))
        echo "[DOTFILES] PATH cleanup removed $((orig_count - new_count)) entries ($orig_count -> $new_count)"
    fi

    export PATH
}

# ================================================================
# MAIN SETUP FUNCTION
# ================================================================
# Orchestrates all PATH setup in the correct order for each platform.
# Called automatically on source unless DOTFILES_NO_AUTO_PATH_SETUP is set.

# Set up all paths - main entry point
# Order: Homebrew (macOS) -> System -> User -> Dev tools -> Languages -> Cleanup
setup_all_paths() {
    # Platform-aware PATH ordering:
    # - macOS: Homebrew first (primary package manager for dev tools)
    # - Linux: System paths first (native packages are well-maintained)
    if is_macos; then
        # macOS: Homebrew provides the modern tools we want to use
        setup_homebrew_paths
        setup_system_paths
    else
        # Linux: Native system packages take priority over Homebrew
        setup_system_paths
        setup_homebrew_paths
    fi

    # User and development paths (same order on all platforms)
    setup_user_paths
    setup_dev_tools_paths

    # Support paths
    setup_manpath
    setup_library_paths
    setup_fpath

    # Language environments
    setup_go_env
    setup_rust_env
    setup_python_env
    setup_node_env

    # Cleanup
    cleanup_path
    cleanup_unwanted_paths

    # Optionally cleanup non-existent paths (can be slow)
    if [[ "${DOTFILES_CLEANUP_PATHS:-}" == "true" ]]; then
        cleanup_nonexistent_paths
    fi
}

# ================================================================
# PATH DEBUGGING
# ================================================================
# Helper functions for debugging PATH issues. Use these to diagnose
# which directories are in PATH and in what order.

# Print current PATH in readable format (one entry per line, numbered)
# Usage: print_path
print_path() {
    echo "Current PATH:"
    echo "$PATH" | tr ':' '\n' | nl -v 0
}

# Print which directories in PATH don't exist (useful for cleanup)
# Usage: print_missing_path_dirs
print_missing_path_dirs() {
    echo "Non-existent directories in PATH:"
    local IFS=':'
    for dir in $PATH; do
        if [[ -n "$dir" ]] && [[ ! -d "$dir" ]]; then
            echo "  $dir"
        fi
    done
}

# ================================================================
# INITIALIZATION
# ================================================================
# Automatic PATH setup on source. Guards prevent re-running and ensure
# a minimal working PATH exists even in corrupted tmux sessions.

if [[ -z "${DOTFILES_NO_AUTO_PATH_SETUP}" && -z "${DOTFILES_PATHS_LOADED}" ]]; then
    # Set guard to prevent re-running within the same shell session
    # NOT exported: child processes (new tmux panes/windows) should run their own setup
    DOTFILES_PATHS_LOADED=1

    # Ensure we have a minimal PATH before starting
    # This is critical for tmux/screen sessions which might have corrupted PATH
    if [[ -z "$PATH" ]] || [[ "$PATH" == ":" ]] || ! command -v ls >/dev/null 2>&1; then
        export PATH="/usr/bin:/bin:/usr/sbin:/sbin:/usr/local/bin"
    fi

    setup_all_paths
fi
