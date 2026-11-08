#!/usr/bin/env bash
# ================================================================
# Cross-Platform Aliases
# ================================================================
#
# Purpose:
#   Provides shell aliases for common commands, tool shortcuts, and
#   platform-specific operations. Uses modern CLI tools when available
#   with graceful fallbacks to standard commands.
#
# Dependencies:
#   - common/platform.sh (for has_command, is_macos, is_linux, etc.)
#
# Organization:
#   - Navigation: Directory shortcuts (cd, .., ~)
#   - File Operations: ls, cp, mv, rm with modern alternatives
#   - Text Processing: grep, cat with ripgrep, bat fallbacks
#   - Git: Common git workflow shortcuts
#   - Docker/Kubernetes: Container and orchestration shortcuts
#   - Cloud: Terraform, AWS, 1Password aliases
#   - Platform-specific: macOS, Linux, WSL specific commands
#
# Modern Tool Aliases:
#   - eza  -> ls (with icons, git integration)
#   - bat/batcat -> cat (with syntax highlighting; Debian/Ubuntu name: batcat)
#   - rg   -> grep (faster, smarter search)
#   - fd/fdfind used when present (Debian/Ubuntu name: fdfind)
#
# ================================================================

# Source platform detection if not already loaded
if [[ -z "$DOTFILES_OS" ]]; then
    if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
        SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    elif [[ -n "${ZSH_VERSION:-}" ]]; then
        # Zsh-sourced path (bash cannot parse ${(%):-%x})
        # shellcheck disable=SC2296
        _dotfiles_script_path="${(%):-%x}"
        SCRIPT_DIR="$(cd "$(dirname "$_dotfiles_script_path")" && pwd)"
        unset -v _dotfiles_script_path
    else
        SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
    fi
    if [[ -f "$SCRIPT_DIR/platform.sh" ]]; then
        source "$SCRIPT_DIR/platform.sh"
    fi
fi

# Re-resolve bat/batcat + fd/fdfind now that paths.sh has run (the rc order
# loads exports.sh before paths.sh, so the exports-time lookup missed tools
# only reachable via ~/.cargo/bin, Homebrew, etc.). This refreshes PAGER,
# MANPAGER and the FZF commands for the aliases below.
if command -v dotfiles_resolve_tool_cmds >/dev/null 2>&1; then
	dotfiles_resolve_tool_cmds
fi

# ================================================================
# NAVIGATION ALIASES
# ================================================================
# Quick directory navigation shortcuts. Use 'z' command (zoxide) for
# smart jumping based on frecency.

# Parent directory shortcuts
alias ..="cd .."
alias cd..="cd .."
alias ...="cd ../.."
alias ....="cd ../../.."
alias .....="cd ../../../.."
alias ~="cd ~"
alias -- -="cd -"

# Directory shortcuts
alias h="cd ~"
alias d="cd ~/Desktop"
alias dl="cd ~/Downloads"
alias doc="cd ~/Documents"
alias dev="cd ~/dev"
alias proj="cd ~/Projects"

# ================================================================
# FILE OPERATIONS
# ================================================================
# File listing and manipulation. Uses eza when available for
# better output (icons, git status, colors).

# Enhanced ls aliases - eza provides icons, git integration, better colors
if has_command eza; then
	alias ls="eza -lg --group-directories-first --color always --icons always"
	alias la="eza -lg --all --group-directories-first --color always --icons always"
	alias ll="eza -alg --group-directories-first --color always --icons always"
	alias ld="eza -lDg --color always --icons always"
	alias lf="eza -lfg --color always --icons always"
	alias lh="eza -dlg .* --color always --icons always --group-directories-first"
	alias lt="eza -alg --color always --icons always --group-directories-first --sort=modified"
	alias tree="eza --tree --color always --icons always"
else
	# Fallback to standard ls with platform-specific options
	if is_macos; then
		alias ls="ls -G"
		alias la="ls -laG"
		alias ll="ls -alFG"
	else
		alias ls="ls --color=auto"
		alias la="ls -la --color=auto"
		alias ll="ls -alF --color=auto"
	fi
	alias ld="ls -ld"
	alias lh="ls -lah"
	alias lt="ls -alt"
fi

# File operations with verbose output
alias mv="mv -v"
alias cp="cp -v"

# Safe remove (prompt for multiple files)
alias rm="rm -v"
alias rmd="rm -rf"
alias rmdir="rm -rf"

# File size
alias fs="stat -f '%z bytes'" 2>/dev/null || alias fs="stat --printf='%s bytes'"

# Disk usage
alias du1="du -h -d 1"
alias dus="du -sh"

# ================================================================
# TEXT PROCESSING
# ================================================================
# Search and file viewing. Ripgrep (rg) is faster than grep.
# Bat provides syntax highlighting for cat.

# Ripgrep (rg) is configured via ~/.config/ripgrep/config
if has_command rg; then
	alias rg="rg --hidden --smart-case"
	alias rgr="rg --hidden --smart-case"  # explicit ripgrep alias
else
	alias grep="grep --color=auto"
fi

# Always use colored grep variants
alias fgrep="fgrep --color=auto"
alias egrep="egrep --color=auto"
alias grep="grep --color=auto"


# Enhanced cat (bat, or batcat on Debian/Ubuntu)
if [[ -n "${DOTFILES_BAT_CMD:-}" ]]; then
	# shellcheck disable=SC2139 # resolved command name is baked in at definition
	alias bat="$DOTFILES_BAT_CMD --paging=never"
	# shellcheck disable=SC2139 # resolved command name is baked in at definition
	alias less="$DOTFILES_BAT_CMD --paging=always"
else
	# Keep original cat
	alias cat="cat"
fi

# fd is fdfind on Debian/Ubuntu; alias only when the resolved name differs
if [[ "${DOTFILES_FD_CMD:-}" == "fdfind" ]]; then
	alias fd="fdfind"
fi

# Text utilities
alias rot13='tr a-zA-Z n-za-mN-ZA-M'
alias trimcopy='tr -d '\''\n'\'' | $DOTFILES_COPY_CMD'

# ================================================================
# GIT ALIASES
# ================================================================
# Common git operations. For complex git workflows, consider using
# lazygit (alias: lg) for a terminal UI.

# Basic git shortcuts - single letter for most common operations
alias g="git"
alias gs="git status --short --branch"
alias gsu="git status --short --branch --untracked-files"
alias ga="git add"
alias gc="git commit"
alias gp="git push"
alias gpl="git pull --ff-only"
alias gd="git diff"
alias gl="git log --oneline --all --graph --decorate"
alias glo="git log --oneline"
alias gb="git branch"
alias gco="git switch"
alias gcb="git switch -c"
alias gsw="git switch"
alias gswc="git switch -c"
alias gm="git merge"

# Advanced git aliases
alias gst="git stash"
alias gsp="git stash pop"
alias gr="git remote -v"
alias gf="git fetch --prune --tags"
alias gt="git tag"
alias grs="git restore --staged"
alias grh="git reset --hard"
alias gcp="git cherry-pick"

# Git root directory - jump to repo root from any subdirectory
alias groot='cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)"'

# Force push with lease (explicit and safer than plain -f)
alias gpf="git push --force-with-lease"

# ================================================================
# BRANCHLET ALIASES
# ================================================================
# Branchlet manages git worktrees for parallel branch development.
# See: npm install -g branchlet

# Branchlet worktree management
alias bl="branchlet"
alias blc="branchlet create"
alias bll="branchlet list"
alias bld="branchlet delete"
alias bls="branchlet settings"

# Quick worktree operations
alias wt="branchlet"
alias wtc="branchlet create"
alias wtl="branchlet list"
alias wtd="branchlet delete"

# ================================================================
# DOCKER ALIASES
# ================================================================
# Docker and Docker Compose shortcuts. Use 'dsh <container>' to
# get a shell inside a running container.

# Docker shortcuts
alias d="docker"
alias dc="docker compose"
alias dps="docker ps"
alias dpsa="docker ps -a"
alias di="docker images"
alias dv="docker volume ls"
alias dn="docker network ls"

# Docker Compose shortcuts
alias dcu="docker compose up -d"
alias dcd="docker compose down"
alias dcr="docker compose restart"
alias dcl="docker compose logs -f"
alias dcp="docker compose pull"
alias dcb="docker compose build"

# ================================================================
# KUBERNETES ALIASES
# ================================================================
# Kubernetes CLI shortcuts. Use kubectx/kubens (kctx/kns) to switch
# contexts and namespaces. Consider k9s for a terminal UI.

# kubectl shortcuts (only when kubectl is on PATH)
if has_command kubectl; then
	alias k="kubectl"
	alias kg="kubectl get"
	alias kd="kubectl describe"
	alias kdel="kubectl delete"
	alias ka="kubectl apply"
	alias kl="kubectl logs"
	alias kex="kubectl exec"

	# Common resource shortcuts
	alias pods="kubectl get pods"
	alias svc="kubectl get services"
	alias ing="kubectl get ingress"
	alias ns="kubectl get namespaces"
	alias pvc="kubectl get pvc"
	alias pv="kubectl get pv"
	alias deps="kubectl get deployments"
fi

# Kubernetes context
if has_command kubectx; then
	alias kctx="kubectx"
fi
if has_command kubens; then
	alias kns="kubens"
fi

# ================================================================
# CLOUD PROVIDER ALIASES
# ================================================================
# Infrastructure as Code and cloud CLI shortcuts.

# Terraform shortcuts - tfi/tfp/tfa for init/plan/apply workflow
alias tf="terraform"
alias tfi="terraform init"
alias tfp="terraform plan"
alias tfa="terraform apply"
alias tfd="terraform destroy"
alias tfs="terraform show"
alias tfv="terraform validate"
alias tff="terraform fmt"

# AWS CLI
alias aws-whoami="aws sts get-caller-identity"
alias aws-regions="aws ec2 describe-regions --output table"

# 1Password CLI integration - run commands with secrets injected
# Example: opr -- aws s3 ls (injects AWS credentials from 1Password)
if has_command op; then
    alias opr="op run --no-masking --"
    alias opl="op run --no-masking"
    alias ope="op run --no-masking --env-file"
fi

# ================================================================
# MISE VERSION MANAGER ALIASES
# ================================================================
# mise is a polyglot runtime manager (replaces nvm, pyenv, rbenv).
# Install with: brew install mise

if has_command mise; then
    alias m="mise"
    alias mi="mise install"
    alias mu="mise use"
    alias mug="mise use -g"       # Global version
    alias mul="mise use"          # Local version (default)
    alias ml="mise list"
    alias mla="mise list --all"
    alias mr="mise run"
    alias mx="mise exec"
    alias mup="mise upgrade"
    alias mself="mise self-update"
    alias mw="mise which"         # Show path to tool
    alias mp="mise plugins"       # List plugins
    alias mc="mise current"       # Show current versions
fi

# ================================================================
# LAZYGIT ALIASES
# ================================================================
# Lazygit provides a terminal UI for git. Much faster than CLI for
# staging hunks, interactive rebase, etc. Press '?' for help.

if has_command lazygit; then
    alias lg="lazygit"
    alias lgs="lazygit status"
    alias lgb="lazygit branch"
    alias lgl="lazygit log"
    alias lgc="lazygit commit"
    alias lgp="lazygit pull"
    alias lgf="lazygit fetch"
fi

# ================================================================
# LAZYDOCKER ALIASES
# ================================================================
# Terminal UI for Docker. Alias is lzd (not ld — reserved for the linker).

if has_command lazydocker; then
    alias lzd="lazydocker"
fi

# ================================================================
# YAZI FILE MANAGER ALIASES
# ================================================================
# Yazi is a terminal file manager with image preview.
# Use 'yy' (function in tools.sh) to change directory on exit.

if has_command yazi; then
    alias y="yazi"
    alias ya="yazi"
    # Note: yy function (changes dir on exit) is defined in tools.sh
fi

# ================================================================
# ZOXIDE ALIASES
# ================================================================
# Zoxide is a smarter 'cd' that learns your most used directories.
# Use 'z foo' to jump to most frecent directory matching 'foo'.
# Use 'zi' for interactive selection with fzf.
# These are additional explicit aliases for power users
if has_command zoxide; then
    alias zi="__zoxide_zi"        # Interactive selection with fzf
    alias zq="zoxide query"       # Query the database
    alias za="zoxide add"         # Add directory to database
    alias zr="zoxide remove"      # Remove directory from database
fi

# ================================================================
# NETWORK ALIASES
# ================================================================
# Network diagnostics and HTTP testing shortcuts.

# IP addresses - myip for public, localip for LAN address
alias get-public-ip="dig +short myip.opendns.com @resolver1.opendns.com"
alias myip="get-public-ip"

# Platform-specific local IP (functions: shellcheck rejects $1 in awk/perl inside aliases)
if is_macos; then
    localip() { ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1; }
    ips() { ifconfig | perl -nle'/(\d+\.\d+\.\d+\.\d+)/ && print $1'; }
else
    localip() { hostname -I | awk '{print $1}'; }
    ips() { ip addr | grep -oP '(?<=inet\s)\d+(\.\d+){3}'; }
fi

# Network utilities
alias ports="netstat -tulpn 2>/dev/null || netstat -p tcp -p udp"
alias whois="whois -h whois-servers.net"

# mtr needs root for its raw sockets
has_command mtr && alias mtr="sudo mtr"

# HTTP testing - aliases for HTTP methods (e.g., GET http://example.com)
alias httpdump="sudo tcpdump -i any -n -s 0 -w - | grep -a -o -E 'Host\: .*|GET \/.*'"
# Dynamic HTTP verb aliases: SC2139 suppressed (verb baked in per loop iteration).
for method in GET HEAD POST PUT DELETE TRACE OPTIONS; do
	if has_command curl; then
		# shellcheck disable=SC2139
		alias "$method"="curl -X '$method'"
	elif has_command lwp-request; then
		# shellcheck disable=SC2139
		alias "$method"="lwp-request -m '$method'"
	fi
done

# ================================================================
# SYSTEM ALIASES
# ================================================================
# Process monitoring and system information. htop replaces top when available.

# Process management
alias psg="ps aux | grep -v grep | grep -i"
alias topc="top -o cpu"
alias topm="top -o mem"

# System monitoring
if has_command htop; then
	alias top="htop"
fi

if has_command glances; then
	alias monitor="glances"
elif has_command htop; then
	alias monitor="htop"
else
    alias monitor="top"
fi

# Disk space
alias df="df -h"
alias free="free -h" 2>/dev/null || alias free="vm_stat"

# ================================================================
# PLATFORM-SPECIFIC ALIASES
# ================================================================
# OS-specific commands. These only load on the appropriate platform.

# macOS specific aliases - Finder, Dock, system preferences
if is_macos; then
    # DNS and system
    alias cleardns="sudo killall -HUP mDNSResponder"
    alias flush="dscacheutil -flushcache && sudo killall -HUP mDNSResponder"

    # Applications
    alias slt='open -a "Sublime Text"'
    alias code="open -a 'Visual Studio Code'"
    alias finder="open ."

    # System preferences
    alias hidedesktop="defaults write com.apple.finder CreateDesktop -bool false && killall Finder"
    alias showdesktop="defaults write com.apple.finder CreateDesktop -bool true && killall Finder"

    # Sound
    alias stfu="osascript -e 'set volume output muted true'"
    alias pumpitup="osascript -e 'set volume 10'"

    # Clipboard ($ expand at use time)
    alias copy='$DOTFILES_COPY_CMD'
    alias paste='$DOTFILES_PASTE_CMD'

    # PlistBuddy
    alias plistbuddy="/usr/libexec/PlistBuddy"

    # Trash (safer than rm)
    if has_command trash; then
        alias del="trash"
    fi
fi

# Linux specific aliases - systemd services, package managers
if is_linux; then
    # DNS
    alias cleardns="sudo systemctl restart systemd-resolved"
    alias flush="sudo systemctl flush-dns"

    # Package management
    case "$DOTFILES_PKG_MGR" in
        apt)
            alias install="sudo apt install"
            alias update="sudo apt update && sudo apt upgrade"
            alias search="apt search"
            alias show="apt show"
            alias remove="sudo apt remove"
            alias purge="sudo apt purge"
            alias autoremove="sudo apt autoremove"
            alias autoclean="sudo apt autoclean"
            ;;
        dnf)
            alias install="sudo dnf install"
            alias update="sudo dnf upgrade"
            alias search="dnf search"
            alias show="dnf info"
            alias remove="sudo dnf remove"
            ;;
        pacman)
            alias install="sudo pacman -S"
            alias update="sudo pacman -Syu"
            alias search="pacman -Ss"
            alias show="pacman -Si"
            alias remove="sudo pacman -R"
            ;;
    esac

    # System services
    alias services="systemctl list-units --type=service"
    alias start="sudo systemctl start"
    alias stop="sudo systemctl stop"
    alias restart="sudo systemctl restart"
    alias status="systemctl status"
    alias enable="sudo systemctl enable"
    alias disable="sudo systemctl disable"

    # Clipboard
    alias copy='$DOTFILES_COPY_CMD'
    alias paste='$DOTFILES_PASTE_CMD'

    # Open command
    alias open='$DOTFILES_OPEN_CMD'
fi

# WSL specific aliases - Windows integration
if is_wsl; then
    # Windows integration
    alias explorer="explorer.exe"
    alias notepad="notepad.exe"
    alias winget="winget.exe"

    # Path conversion
    alias wslpath="wslpath"
    alias winpath="wslpath -w"

    # Clipboard integration
    alias copy="clip.exe"
    alias paste="powershell.exe Get-Clipboard"
fi

# ================================================================
# DEVELOPMENT ALIASES
# ================================================================
# Editor shortcuts and common development tasks.

# Editors - all point to $EDITOR (usually nvim)
alias v='$EDITOR'
alias vi='$EDITOR'
alias vim='$EDITOR'
alias nano='$EDITOR'
alias edit='$EDITOR'

# Common development tasks (serve, urlencode, urldecode are functions in functions.sh)
alias jsonpp="python3 -m json.tool"

# Node.js
alias npi="npm install"
alias nps="npm start"
alias npt="npm test"
alias npb="npm run build"
alias npd="npm run dev"

# Python
alias py="python3"
alias pip="pip3"
alias venv="python3 -m venv"

# ================================================================
# TMUX ALIASES
# ================================================================
# Tmux session management. Note: ta, ts, tk are functions (not aliases)
# in common/functions.sh for proper tab completion support.
# Simple aliases that don't require argument handling
alias tl="tmux list-sessions"
alias tks="tmux kill-server"
alias tmr="tmux rename-session"
alias td="tmux detach"

# ================================================================
# FUN ALIASES
# ================================================================
# Weather, moon phases, and other fun CLI tools.

# Weather - uses wttr.in service (try: weather London)
if has_command curl; then
    alias weather="curl wttr.in"
    alias moon="curl wttr.in/Moon"
fi

# Random
alias hax="growlnotify -a 'Activity Monitor' 'System error' -m 'WTF R U DOIN'" 2>/dev/null || true

# ================================================================
# UTILITY ALIASES
# ================================================================
# Quick file editing, path display, and common operations.

# Quick config editing
alias hosts='sudo $EDITOR /etc/hosts'
alias profile='$EDITOR ~/.profile'
alias bashrc='$EDITOR ~/.bashrc'
alias zshrc='$EDITOR ~/.zshrc'

# Path
alias path='echo -e ${PATH//:/\\n}'
alias printpath='echo -e ${PATH//:/\\n}'

# History
alias h="history"
alias hg="history | grep"

# Processes
alias psa="ps aux"
alias psg="ps aux | grep -v grep | grep"

# Rsync with common options
alias rsynca="rsync -ravu"
alias rsync-copy="rsync -avh --progress"
alias rsync-move="rsync -avh --progress --remove-source-files"

# Quick file operations (functions mkd and mkf are defined in functions.sh)

# ================================================================
# CONDITIONAL ALIASES
# ================================================================

# Only create aliases for tools that exist
if has_command code; then
    alias c="code"
fi

if has_command subl; then
    alias s="subl"
fi

if has_command firefox; then
    alias ff="firefox"
fi

if has_command chrome || has_command google-chrome; then
    alias chrome="google-chrome 2>/dev/null || chrome"
fi

# ================================================================
# DIRENV ALIASES
# ================================================================
# direnv manages per-directory environment variables via .envrc files.
# Remember to run 'da' (direnv allow) after creating/editing .envrc.

# Quick direnv operations
alias da="direnv allow"              # Allow .envrc in current directory
alias dr="direnv reload"             # Reload direnv
alias ds="direnv status"             # Show direnv status
alias dd="direnv deny"               # Deny .envrc in current directory
alias de="direnv edit"               # Edit .envrc in current directory

# Initialize new .envrc
alias envrc="init-envrc"             # Create .envrc in current directory

# ================================================================
# MONITOR AUTO-SWITCH ALIASES
# ================================================================
# Linux/Hyprland monitor management for laptop lid open/close events.

# Lid state checking (Linux only)
alias lidstate="cat /proc/acpi/button/lid/*/state"
alias lid="lidstate"

# Monitor auto-switch scripts
if is_linux && has_command hyprctl; then
    alias mcl="sudo /etc/acpi/lid-close.sh"                    # Monitor close lid
    alias mcl-test="MONITOR_TEST_MODE=1 sudo /etc/acpi/lid-close.sh" # Test with bypass
    alias mol="sudo /etc/acpi/lid-open.sh"                     # Monitor open lid
    alias mls="journalctl -t lid-events -n 50"                 # Monitor log script

    # Hyprland monitor utilities
    alias monlist="hyprctl monitors"
    alias wslist="hyprctl workspaces"
    alias mon-all="hyprctl monitors all"

    # Search client list (class/title); uses rg if available (see bin/hypr-clients)
    alias hypr-clients='command "$DOTFILES_ROOT/bin/hypr-clients.sh"'

    # Hyprland reload
    alias hreload="hyprctl reload"              # Reload Hyprland config
    alias hrestart="hyprctl reload"             # Restart (same as reload)

    # Check current monitor configuration
    alias moncfg="cat /etc/acpi/hypr-utils.sh | grep -E 'LAPTOP_|EXTERNAL_'"

    # Listen to ACPI events
    alias acpilisten="sudo acpi_listen"
fi

# ================================================================
# CLIPBOARD TESTING HELPERS
# ================================================================
# Clipboard operations with automatic platform detection.
# Uses OSC 52 for SSH sessions to copy to local clipboard.

# Test clipboard copy functionality
alias test-clipboard='echo "Testing clipboard: Hello from $(hostname)" | $DOTFILES_COPY_CMD && echo "✓ Text copied to clipboard. Try pasting on your local machine."'

# Test OSC 52 clipboard directly (for SSH sessions)
alias test-osc52='printf "Testing OSC 52 clipboard\n" | tmux-osc52-copy && echo "✓ OSC 52 sequence sent. Try pasting on your local machine."'

# Show clipboard info
alias clipboard-info='echo "Clipboard Detection:"; echo "  OS: $DOTFILES_OS"; echo "  SSH: $DOTFILES_IS_SSH"; echo "  Use OSC52: $DOTFILES_USE_OSC52"; echo "  Copy Command: $DOTFILES_COPY_CMD"; echo "  Paste Command: $DOTFILES_PASTE_CMD"'

# Quick clipboard copy from stdin
alias cb='$DOTFILES_COPY_CMD'

# Quick clipboard paste to stdout
alias cbp='$DOTFILES_PASTE_CMD'

# ================================================================
# CLAUDE / CODEX / OpenCode ALIASES
# ================================================================

alias claude='claude --allow-dangerously-skip-permissions'
alias cc-itcn='CLAUDE_CONFIG_DIR="$HOME/.config/claude-itcn" claude --allow-dangerously-skip-permissions'
alias cc-th='CLAUDE_CONFIG_DIR="$HOME/.config/claude-th" claude --allow-dangerously-skip-permissions'
alias cc-rth='CLAUDE_CONFIG_DIR="$HOME/.config/claude-th" command claude'
alias oc='opencode --auto'

# ================================================================
# FABRIC ALIASES
# ================================================================
# Fabric may be installed as fabric-ai by Homebrew/AUR. Keep the user-facing
# command stable without copying or renaming binaries.

if [[ -n "${ZSH_VERSION:-}" ]]; then
	if ! whence -p fabric >/dev/null 2>&1 && whence -p fabric-ai >/dev/null 2>&1; then
		alias fabric="fabric-ai"
	fi
else
	if ! type -P fabric >/dev/null 2>&1 && type -P fabric-ai >/dev/null 2>&1; then
		alias fabric="fabric-ai"
	fi
fi

_fabric_alias_command() {
	if typeset -f _fabric_command >/dev/null 2>&1; then
		_fabric_command
		return
	fi

	if [[ -n "${ZSH_VERSION:-}" ]]; then
		if whence -p fabric >/dev/null 2>&1; then
			printf '%s\n' "fabric"
			return 0
		fi
		if whence -p fabric-ai >/dev/null 2>&1; then
			printf '%s\n' "fabric-ai"
			return 0
		fi
	else
		if type -P fabric >/dev/null 2>&1; then
			printf '%s\n' "fabric"
			return 0
		fi
		if type -P fabric-ai >/dev/null 2>&1; then
			printf '%s\n' "fabric-ai"
			return 0
		fi
	fi

	return 1
}

_fabric_run() {
	local fabric_cmd
	fabric_cmd="$(_fabric_alias_command)" || {
		echo "Error: Fabric CLI not found. Install fabric or fabric-ai first." >&2
		return 1
	}

	command "$fabric_cmd" "$@"
}

fab() { _fabric_run "$@"; }
fabric-setup() { _fabric_run --setup "$@"; }
fabric-patterns() { _fabric_run --listpatterns "$@"; }
fpatterns() { fabric-patterns "$@"; }
fabric-update-patterns() { _fabric_run --updatepatterns "$@"; }
fupatterns() { fabric-update-patterns "$@"; }
fabric-models() { _fabric_run --listmodels "$@"; }
fmodels() { fabric-models "$@"; }
fabric-vendors() { _fabric_run --listvendors "$@"; }
fvendors() { fabric-vendors "$@"; }
fabric-serve() { _fabric_run --serve "$@"; }
fabric-serve-ollama() { _fabric_run --serveOllama "$@"; }

alias qq="fabric-ask"
alias askai="fabric-ask"

_fabric_alias_reserved() {
	case "$1" in
	fabric|fab|fabric-setup|fabric-patterns|fpatterns|fabric-update-patterns|fupatterns|fabric-models|fmodels|fabric-vendors|fvendors|fabric-serve|fabric-serve-ollama|fabric-ask|fabric_ask|askai|qq|yt)
		return 0
		;;
	*)
		return 1
		;;
	esac
}

_fabric_load_pattern_aliases() {
	[[ "${FABRIC_ENABLE_PATTERN_ALIASES:-true}" == "false" ]] && return 0

	local patterns_dir="${FABRIC_PATTERNS_DIR:-$HOME/.config/fabric/patterns}"
	[[ -d "$patterns_dir" ]] || return 0

	local alias_prefix="${FABRIC_ALIAS_PREFIX:-}"
	local overwrite="${FABRIC_ALIAS_OVERWRITE:-false}"
	local pattern_path pattern_name alias_name

	# Snapshot existing aliases once. Per-pattern `whence`/`type` checks across
	# 250+ Fabric patterns previously cost ~400ms of every shell startup.
	# zsh uses an associative array; the bash path (3.2-compatible, no
	# `declare -A`) builds one newline-delimited string of "alias -p" names.
	local _fabric_existing_aliases=""
	if [[ -n "${ZSH_VERSION:-}" ]]; then
		typeset -A _fabric_alias_seen
		local _fabric_alias_key
		# Intentional zsh-only key expansion over the aliases assoc array.
		# shellcheck disable=SC2296
		for _fabric_alias_key in ${(k)aliases}; do
			_fabric_alias_seen[$_fabric_alias_key]=1
		done
	elif [[ -n "${BASH_VERSION:-}" ]]; then
		local _fabric_alias_line _fabric_alias_name
		while IFS= read -r _fabric_alias_line; do
			_fabric_alias_name="${_fabric_alias_line%%=*}"
			_fabric_alias_name="${_fabric_alias_name#alias }"
			[[ -n "$_fabric_alias_name" ]] || continue
			_fabric_existing_aliases+=$'\n'"$_fabric_alias_name"$'\n'
		done < <(alias -p 2>/dev/null)
	fi

	for pattern_path in "$patterns_dir"/*; do
		[[ -d "$pattern_path" ]] || continue

		# Avoid basename(1) — 250+ subshells were ~400ms of startup.
		pattern_name="${pattern_path##*/}"
		[[ "$pattern_name" =~ ^[A-Za-z0-9_]+$ ]] || continue

		alias_name="${alias_prefix}${pattern_name}"

		[[ "$alias_name" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue

		if _fabric_alias_reserved "$alias_name"; then
			continue
		fi

		if [[ "$overwrite" != "true" ]]; then
			if [[ -n "${ZSH_VERSION:-}" ]]; then
				[[ -n "${_fabric_alias_seen[$alias_name]:-}" ]] && continue
			elif [[ "$_fabric_existing_aliases" == *$'\n'"$alias_name"$'\n'* ]]; then
				continue
			fi
		fi

		# shellcheck disable=SC2139 # Capture this pattern name when the alias is generated.
		alias -- "$alias_name=fabric --pattern $pattern_name"
	done

	# Clean up the snapshot vars (unset -f is not used: one may be a zsh assoc).
	unset -v _fabric_existing_aliases _fabric_alias_seen 2>/dev/null || true
}

_fabric_load_pattern_aliases
unset -f _fabric_load_pattern_aliases _fabric_alias_reserved

# Load platform-specific aliases: family file first, distro overlay second, so
# an overlay refines the family instead of replacing it (see os_module_dirs in
# common/platform.sh). Missing directories are simply skipped.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
while IFS= read -r os_module_dir; do
    [[ -z "$os_module_dir" ]] && continue
    if [[ -f "$SCRIPT_DIR/$os_module_dir/aliases.sh" ]]; then
        source "$SCRIPT_DIR/$os_module_dir/aliases.sh"
    fi
done <<EOF
$(os_module_dirs)
EOF
unset os_module_dir
