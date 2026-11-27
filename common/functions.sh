#!/usr/bin/env bash
# ================================================================
# Cross-Platform Functions
# ================================================================
#
# Purpose:
#   Provides utility functions for common operations like file
#   manipulation, network diagnostics, development tasks, and
#   system administration. Functions work across bash and zsh.
#
# Dependencies:
#   - common/platform.sh (for has_command, is_macos, is_linux, etc.)
#
# Categories:
#   - Directory/File: mkd, mkf, f, fd_dir, cdf, filesize
#   - Archive: extract, archive
#   - Text: json, urlencode/urldecode, b64encode/b64decode
#   - Network: httpstatus, httpheaders, portscan, myips, dig_all
#   - Git: gitclone, gitexport, gitclean, git_log_github
#   - Process: killall_name, killport, whoisport
#   - Development: serve, genpass, qr
#   - System: sysinfo, needreboot
#   - Docker: docker-clean, op-dc-up, dsh
#   - Clipboard: clip, unclip, copypath
#   - Tmux: ta, ts, tk, fix_tmux_path
#
# Usage:
#   All functions are available after sourcing this file.
#   Many functions provide help when called without arguments.
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

# ================================================================
# DIRECTORY AND FILE OPERATIONS
# ================================================================
# Functions for creating, navigating, and manipulating files/directories.

# Launch OpenCode safely from tmux under Ghostty.
#
# Ghostty can currently crash when tmux passes certain application escape
# sequences through to the terminal. Keep passthrough available globally, but
# disable it for the pane while OpenCode is running.
opencode() {
    if [[ -n "${TMUX:-}" ]] && has_command tmux; then
        local passthrough
        passthrough="$(tmux show-options -p -v allow-passthrough 2>/dev/null || true)"

        if [[ -n "$passthrough" ]]; then
            tmux set-option -p allow-passthrough off 2>/dev/null || true
            OPENCODE_DISABLE_MOUSE="${OPENCODE_DISABLE_MOUSE:-1}" \
                OPENCODE_DISABLE_TERMINAL_TITLE="${OPENCODE_DISABLE_TERMINAL_TITLE:-1}" \
                command opencode "$@"
            local opencode_status=$?
            tmux set-option -p allow-passthrough "$passthrough" 2>/dev/null || true
            return "$opencode_status"
        fi
    fi

    command opencode "$@"
}

opencode-raw() {
    command opencode "$@"
}

# Run graphify against the tokenhop OpenAI-compatible endpoint.
#
# graphify auto-detects gemini before openai, so the exported GEMINI_API_KEY
# would otherwise win; pin the backend explicitly. GRAPHIFY_BACKEND or
# --backend still override per call. Falls through untouched when the
# tokenhop key is absent (see common/exports.sh).
graphify() {
    if [[ -z "${TOKENHOP_OPENAI_API_KEY:-}" ]]; then
        command graphify "$@"
        return
    fi

    local -a env_args=(
        "GRAPHIFY_BACKEND=${GRAPHIFY_BACKEND:-openai}"
        "OPENAI_API_KEY=$TOKENHOP_OPENAI_API_KEY"
    )
    if [[ -n "${TOKENHOP_OPENAI_BASE_URL:-}" ]]; then
        env_args+=("OPENAI_BASE_URL=$TOKENHOP_OPENAI_BASE_URL")
    fi
    if [[ -n "${TOKENHOP_OPENAI_MODEL:-}" ]]; then
        env_args+=(
            "OPENAI_MODEL=$TOKENHOP_OPENAI_MODEL"
            "GRAPHIFY_OPENAI_MODEL=$TOKENHOP_OPENAI_MODEL"
        )
    fi

    env "${env_args[@]}" graphify "$@"
}

graphify-raw() {
    command graphify "$@"
}

# Launch Codex after checking its app-server daemon package.
#
# Codex 0.167+ cannot start without the daemon package, and the CLI's own
# self-update installs only the CLI, so the daemon can go missing or fall
# behind. Compare the two selected releases (two readlinks, no subprocess when
# they match) and let bin/install-codex.sh --install repair a missing or older
# daemon. A daemon newer than the CLI is fine. The repair prompts, so it only
# runs from an interactive terminal; scripted calls just get a hint.
codex() {
    local packages="${CODEX_HOME:-$HOME/.codex}/packages" cli daemon
    local installer="${DOTFILES_ROOT:-$HOME/.config/dotfiles}/bin/install-codex.sh"
    cli="$(readlink "$packages/standalone/current" 2>/dev/null)"
    daemon="$(readlink "$packages/app-server-daemon/current" 2>/dev/null)"
    cli="${cli##*/}"
    daemon="${daemon##*/}"
    # "~" makes sort -V rank prereleases (0.168.0-alpha.1) before their release.
    local cli_key="${cli/-alpha/~alpha}" daemon_key="${daemon/-alpha/~alpha}"
    cli_key="${cli_key/-beta/~beta}"
    daemon_key="${daemon_key/-beta/~beta}"
    if [[ -n "$cli" && "$cli" != "$daemon" && -x "$installer" ]] &&
        { [[ -z "$daemon" ]] || [[ "$(printf '%s\n%s\n' "$cli_key" "$daemon_key" | sort -V | head -n 1)" == "$daemon_key" ]]; }; then
        printf '[dotfiles] Codex app-server daemon is %s; CLI is %s\n' "${daemon:-missing}" "$cli" >&2
        if [[ -t 0 && -t 1 ]]; then
            "$installer" --install ||
                printf '[dotfiles] Daemon repair failed; run %s --install, or codex --no-daemon\n' "$installer" >&2
        else
            printf '[dotfiles] Run %s --install to update it\n' "$installer" >&2
        fi
    fi
    command codex "$@"
}

codex-raw() {
    command codex "$@"
}

# Resolve the Fabric binary without considering aliases or shell functions.
_fabric_command() {
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

fabric-ask() {
	local fabric_cmd
	fabric_cmd="$(_fabric_command)" || {
		echo "Error: Fabric CLI not found. Install fabric or fabric-ai first." >&2
		return 1
	}

	if [[ $# -gt 0 ]]; then
		printf '%s\n' "$*" | command "$fabric_cmd" --stream
	else
		command "$fabric_cmd" --stream
	fi
}

fabric_ask() {
	fabric-ask "$@"
}

yt() {
	local with_timestamps=false

	while [[ $# -gt 0 ]]; do
		case "$1" in
			-t|--timestamps)
				with_timestamps=true
				shift
				;;
			--)
				shift
				break
				;;
			-*)
				echo "Usage: yt [-t|--timestamps] <youtube-url>" >&2
				return 2
				;;
			*)
				break
				;;
		esac
	done

	if [[ $# -ne 1 ]]; then
		echo "Usage: yt [-t|--timestamps] <youtube-url>" >&2
		return 2
	fi

	local video_link="$1"
	local fabric_cmd
	fabric_cmd="$(_fabric_command)" || {
		echo "Error: Fabric CLI not found. Install fabric or fabric-ai first." >&2
		return 1
	}

	if [[ "$with_timestamps" == "true" ]]; then
		command "$fabric_cmd" -y "$video_link" --transcript-with-timestamps
	else
		command "$fabric_cmd" -y "$video_link"
	fi
}

# Create directory and enter it (mkdir + cd combined)
# Usage: mkd project/src/components
mkd() {
    mkdir -pv "$@" && cd "$@" || return
}

# Create file with directory structure (creates parent dirs if needed)
# Usage: mkf project/src/index.ts
mkf() {
    local file="$1"
    local dir
    dir="$(dirname "$file")"
    [[ "$dir" != "." ]] && mkdir -pv "$dir"
    touch "$file"
}

# Find files by name (uses fd/fdfind if available, falls back to find)
# Usage: f "*.ts"
f() {
    local _fd_cmd
    if _fd_cmd="$(first_command fd fdfind)"; then
        "$_fd_cmd" "$@"
    else
        find . -name "*$1*" -type f
    fi
}

# Find directories by name (uses fd/fdfind if available)
# Usage: fd_dir "node_modules"
fd_dir() {
    local _fd_cmd
    if _fd_cmd="$(first_command fd fdfind)"; then
        "$_fd_cmd" --type d "$@"
    else
        find . -name "*$1*" -type d
    fi
}

# cd into the frontmost Finder window (macOS) or most recent directory (Linux)
# Usage: cdf
cdf() {
    if is_macos; then
        cd "$(osascript -e 'tell app "Finder" to POSIX path of (insertion location as alias)')" || return
    else
        # Linux/WSL: cd to most recently accessed directory
        if [[ -f ~/.local/share/recently-used.xbel ]]; then
            local recent_dir
            recent_dir=$(grep -o 'file://[^"]*' ~/.local/share/recently-used.xbel | head -1 | sed 's|file://||' | xargs dirname 2>/dev/null)
            if [[ -d "$recent_dir" ]]; then
                cd "$recent_dir" || return
            else
                cd ~ || return
            fi
        else
            cd ~ || return
        fi
    fi
}

# Get file size in human-readable format
filesize() {
    if is_macos; then
        stat -f"%z" "$1" | numfmt --to=iec
    else
        stat --printf="%s" "$1" | numfmt --to=iec
    fi
}

# Recursively delete files by pattern
cleanup_files() {
    local pattern="${1:-.DS_Store}"
    local search_path="${2:-.}"

    echo "Cleaning up $pattern files in $search_path..."
    find "$search_path" -name "$pattern" -type f -print -delete
}

# Read a single key into REPLY after printing a prompt, in bash or zsh.
# zsh's `read -p` reads from a coprocess, and its `read -k` reads the terminal
# unless told to use stdin (-u0), so each shell gets its own form (YAN-1424).
# Usage: _read_key "Continue? (y/n) "; [[ $REPLY =~ ^[Yy]$ ]]
_read_key() {
    REPLY=
    if [[ -n "${ZSH_VERSION:-}" ]]; then
        read -r -k1 -u0 "REPLY?$1"
    else
        read -r -n1 -p "$1"
    fi
}

# ================================================================
# ARCHIVE OPERATIONS
# ================================================================
# Universal archive extraction and creation.

# Run "$@" and write its stdout to $1; remove the partial output on failure.
# Usage: _extract_to_file out.txt gunzip -c in.txt.gz
_extract_to_file() {
    local out="$1"
    shift
    if ! "$@" >"$out"; then
        rm -f "$out"
        return 1
    fi
}

# Extract archives - auto-detects format and extracts to named folder
# Supports: tar.gz, tar.bz2, tar.xz, zip, rar, 7z, gz, bz2, xz, Z
# Usage: extract archive.tar.gz
extract() {
    if [[ ! -f "$1" ]]; then
        echo "Error: '$1' is not a valid file"
        return 1
    fi

    local filename
    filename=$(basename "$1")
    local foldername="${filename%%.*}"
    local fullpath
    fullpath=$(realpath "$1")
    local didfolderexist=false

    if [[ -d "$foldername" ]]; then
        didfolderexist=true
        _read_key "$foldername already exists, overwrite it? (y/n) "
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            return 0
        fi
        rm -rf "$foldername"
    fi

    mkdir -p "$foldername" && cd "$foldername" || return 1

    case "$1" in
        *.tar.bz2|*.tbz2|*.tbz) tar xjf "$fullpath" ;;
        *.tar.gz|*.tgz) tar xzf "$fullpath" ;;
        *.tar.xz|*.txz) tar Jxf "$fullpath" ;;
        *.tar.Z|*.taz) tar xzf "$fullpath" ;;
        *.tar) tar xf "$fullpath" ;;
        *.zip) unzip -q "$fullpath" ;;
        *.rar) unrar x "$fullpath" ;;
        *.7z) 7z x "$fullpath" ;;
        # Single-file formats: decompress to stdout so the output lands in
        # the new folder and the original archive is kept (YAN-2490).
        *.gz) _extract_to_file "${filename%.*}" gunzip -c "$fullpath" ;;
        *.bz2) _extract_to_file "${filename%.*}" bunzip2 -c "$fullpath" ;;
        *.xz) _extract_to_file "${filename%.*}" unxz -c "$fullpath" ;;
        *.Z) _extract_to_file "${filename%.*}" uncompress -c "$fullpath" ;;
        *)
            echo "Error: '$1' cannot be extracted via extract()"
            cd ..
            [[ $didfolderexist == false ]] && rm -rf "$foldername"
            return 1
            ;;
    esac
}

# Create archive - format determined by extension
# Usage: archive backup.tar.gz file1 file2 dir/
archive() {
    local archive_name="$1"
    shift

    case "$archive_name" in
        *.tar.gz|*.tgz) tar czf "$archive_name" "$@" ;;
        *.tar.bz2|*.tbz2) tar cjf "$archive_name" "$@" ;;
        *.tar.xz|*.txz) tar cJf "$archive_name" "$@" ;;
        *.tar) tar cf "$archive_name" "$@" ;;
        *.zip) zip -r "$archive_name" "$@" ;;
        *.7z) 7z a "$archive_name" "$@" ;;
        *) echo "Error: unsupported archive format for '$archive_name'" && return 1 ;;
    esac
}

# ================================================================
# TEXT PROCESSING
# ================================================================
# Text transformation, encoding, and formatting utilities.

# JSON pretty print with syntax highlighting (uses jq + bat/batcat when available)
# Usage: echo '{"a":1}' | json   OR   json '{"a":1}'
json() {
    local _bat_cmd=""
    _bat_cmd="$(first_command bat batcat)" || _bat_cmd=""
    if [[ -p /dev/stdin ]]; then
        # Input from pipe
        if has_command jq; then
            jq '.'
        elif has_command python3; then
            python3 -m json.tool
        else
            cat
        fi
    else
        # Input from argument
        if has_command jq; then
            echo "$*" | jq '.'
        elif has_command python3; then
            echo "$*" | python3 -m json.tool
        else
            echo "$*"
        fi
    fi | (if [[ -n "$_bat_cmd" ]]; then "$_bat_cmd" -l json --paging=never; else cat; fi)
}

# URL encode a string (spaces become %20, etc.)
# Usage: urlencode "hello world"
urlencode() {
    if has_command python3; then
        python3 -c "import urllib.parse, sys; print(urllib.parse.quote_plus(sys.argv[1]))" "$1"
    else
        echo "$1" | sed 's/ /%20/g'
    fi
}

# URL decode a string (%20 becomes space, etc.)
# Usage: urldecode "hello%20world"
urldecode() {
    if has_command python3; then
        python3 -c "import urllib.parse, sys; print(urllib.parse.unquote_plus(sys.argv[1]))" "$1"
    else
        echo "$1" | sed 's/%20/ /g'
    fi
}

# Base64 encode (from string or stdin)
# Usage: b64encode "hello"   OR   cat file | b64encode
b64encode() {
    if [[ -p /dev/stdin ]]; then
        base64 -i -
    else
        echo -n "$1" | base64
    fi
}

# Base64 decode (from string or stdin)
# Usage: b64decode "aGVsbG8="   OR   echo "aGVsbG8=" | b64decode
b64decode() {
    if [[ -p /dev/stdin ]]; then
        base64 -d -i -
    else
        echo "$1" | base64 -d
    fi
}

# ROT13 encode/decode (removed - conflicts with alias)

# Escape UTF-8 characters to hex representation
# Usage: escape "emoji"
escape() {
    local hex
    hex=$(printf "%s" "$*" | xxd -p -c1 -u)
    # Word-split intentional: each line is one hex byte to feed printf.
    # shellcheck disable=SC2046,SC2086
    printf "\\\x%s" $(echo "$hex")
    echo
}

# Decode Unicode escape sequences (e.g., \u00e9 -> e)
# Usage: unidecode '\u00e9'
unidecode() {
    perl -CSD -e 'print @ARGV' -- "$@"
    echo
}

# ================================================================
# NETWORK FUNCTIONS
# ================================================================
# HTTP testing, DNS lookups, port scanning, and IP address utilities.

# Test if a URL supports HTTP compression (gzip/deflate)
# Usage: httpcompression https://example.com
httpcompression() {
    local url="$1"
    local encoding
    encoding=$(curl -LIs -H 'User-Agent: Mozilla/5.0' -H 'Accept-Encoding: gzip,deflate,compress,sdch' "$url" | grep '^Content-Encoding:')

    if [[ -n "$encoding" ]]; then
        echo "$url is encoded using ${encoding#* }"
    else
        echo "$url is not using any encoding"
    fi
}

# Get HTTP status code for a URL (e.g., 200, 404, 500)
# Usage: httpstatus https://example.com
httpstatus() {
    curl -o /dev/null -s -w "%{http_code}" "$1"
}

# Get HTTP response headers for a URL
# Usage: httpheaders https://example.com
httpheaders() {
    curl -I -s "$1"
}

# Simple port scanner using bash /dev/tcp
# Usage: portscan localhost 1 1000  (scans ports 1-1000)
portscan() {
    local host="${1:-localhost}"
    local start_port="${2:-1}"
    local end_port="${3:-1000}"

    echo "Scanning ports $start_port-$end_port on $host..."
    for port in $(seq $start_port $end_port); do
        if timeout 1 bash -c "echo >/dev/tcp/$host/$port" 2>/dev/null; then
            echo "Port $port: Open"
        fi
    done
}

# Get all IP addresses (public and local)
# Usage: myips
myips() {
    echo "Public IP: $(curl -s https://ipinfo.io/ip)"
    echo "Local IPs:"
    if is_macos; then
        ifconfig | grep -E 'inet [0-9]' | grep -v 127.0.0.1 | awk '{print "  " $2}'
    else
        ip addr | grep -oP '(?<=inet )\d+(\.\d+){3}' | grep -v 127.0.0.1 | sed 's/^/  /'
    fi
}

# DNS lookup for all common record types (A, AAAA, CNAME, MX, TXT, NS)
# Usage: dig_all example.com
dig_all() {
    local domain="$1"
    for type in A AAAA CNAME MX TXT NS; do
        echo "=== $type records for $domain ==="
        dig +short "$domain" "$type"
        echo
    done
}

# ================================================================
# GIT FUNCTIONS
# ================================================================
# Git workflow helpers for cloning, exporting, and cleanup.

# Git log with clickable GitHub URLs (requires iTerm or similar)
# Shows commits with links to files on GitHub
git_log_github() {
    local remote user_repo
    if ! remote="$(git remote get-url --push origin 2>/dev/null)"; then
        echo "No origin remote found" >&2
        return 1
    fi

    case "$remote" in
        git@*:*) user_repo="${remote#*:}" ;;
        https://*/*)
            user_repo="${remote#https://}"
            user_repo="${user_repo#*/}"
            ;;
        *)
            echo "Unsupported origin remote format" >&2
            return 1
            ;;
    esac
    git log "$@" --name-status --color | awk -v repo="${user_repo%.git}" '
        /^.*commit [0-9a-f]{40}/ {sha=substr($2,1,7)}
        /^[MA]\t/ {printf "%s\thttps://github.com/%s/blob/%s/%s\n", $1, repo, sha, $2; next}
        {print}
    ' | ${PAGER:-less}
}

# Clone a repo and cd into it
# Usage: gitclone https://github.com/user/repo.git
gitclone() {
    git clone "$1" && cd "$(basename "$1" .git)" || return
}

# Export git repository without .git directory
# Usage: gitexport ./dist
gitexport() {
    local target="${1:-.}"
    mkdir -p "$target"
    git archive HEAD | tar -x -C "$target"
}

# Delete local branches that have been merged (excludes main/master/develop)
# Usage: gitclean
gitclean() {
    echo "Cleaning merged branches..."
    git branch --merged | grep -v '\*\|main\|master\|develop' | xargs -n 1 git branch -d
}

# ================================================================
# FILE COMPRESSION AND COMPARISON
# ================================================================
# File size analysis and diff utilities.

# Show gzipped size of a file and compression ratio
# Usage: gz large-file.js
gz() {
    local file="$1"
    echo "Original size: $(wc -c < "$file") bytes"
    echo "Gzipped size:  $(gzip -c "$file" | wc -c) bytes"
    echo "Compression:   $(echo "scale=2; $(gzip -c "$file" | wc -c) * 100 / $(wc -c < "$file")" | bc)%"
}

# Compare two files with syntax-highlighted diff (uses delta if available)
# Usage: compare file1.txt file2.txt
compare() {
    local _bat_cmd=""
    _bat_cmd="$(first_command bat batcat)" || _bat_cmd=""
    if has_command delta; then
        delta "$1" "$2"
    elif has_command diff; then
        diff -u "$1" "$2" | (if [[ -n "$_bat_cmd" ]]; then "$_bat_cmd" -l diff --paging=never; else cat; fi)
    else
        echo "No diff tool available"
    fi
}

# ================================================================
# PROCESS MANAGEMENT
# ================================================================
# Process finding and termination utilities.

# Find processes by name and optionally kill them (with confirmation)
# Usage: killall_name node
killall_name() {
    local name="$1"
    local pid
    local -a pids=()
    # One array element per PID. zsh does not word-split an unquoted string,
    # so a newline-joined PID list would reach ps and kill as one word (YAN-1424).
    while IFS= read -r pid; do
        [[ -n "$pid" ]] && pids+=("$pid")
    done < <(pgrep -f -- "$name")

    if ((${#pids[@]} > 0)); then
        echo "Found processes matching '$name':"
        # ps -p takes a single comma-separated list on both procps and BSD.
        ps -p "$(IFS=,; echo "${pids[*]}")" -o pid,comm,args
        _read_key "Kill these processes? (y/n) "
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            kill "${pids[@]}"
        fi
    else
        echo "No processes found matching '$name'"
    fi
}

# Kill the process using a specific port
# Usage: killport 3000
killport() {
    local port="$1"
    local pids pid rc=0
    # One PID per line, deduplicated: a process listening on both IPv4 and
    # IPv6 reports one line per socket (YAN-1434).
    if is_macos; then
        pids=$(lsof -ti "tcp:$port" | sort -u)
    else
        pids=$(ss -tulpn | grep ":$port " | grep -o 'pid=[0-9][0-9]*' | cut -d= -f2 | sort -u)
    fi

    if [[ -z "$pids" ]]; then
        echo "No process found using port $port"
        return 0
    fi

    # Kill each PID on its own; never pass the multi-line string to kill.
    while IFS= read -r pid; do
        [[ -n "$pid" ]] || continue
        echo "Killing process $pid using port $port"
        kill "$pid" || rc=1
    done <<<"$pids"
    return "$rc"
}

# Show what process is using a specific port
# Usage: whoisport 3000
whoisport() {
    local port="$1"
    if is_macos; then
        lsof -i tcp:$port
    else
        ss -tulpn | grep ":$port "
    fi
}

# ================================================================
# DEVELOPMENT FUNCTIONS
# ================================================================
# Development utilities: HTTP server, password generation, QR codes.

# Start a simple HTTP server in current directory
# Usage: serve 8080 ./dist  (defaults to port 8000, current dir)
serve() {
    local port="${1:-8000}"
    local dir="${2:-.}"

    echo "Starting HTTP server on http://localhost:$port"
    echo "Serving: $(realpath "$dir")"

    if has_command python3; then
        cd "$dir" && python3 -m http.server "$port"
    elif has_command python; then
        cd "$dir" && python -m SimpleHTTPServer "$port"
    elif has_command node; then
        npx serve -p "$port" "$dir"
    else
        echo "No HTTP server available"
        return 1
    fi
}

# Generate a random password (alphanumeric, URL-safe)
# Usage: genpass 24  (defaults to 16 characters)
genpass() {
    local length="${1:-16}" password='' chunk
    if [[ ! "$length" =~ ^[1-9][0-9]*$ ]]; then
        echo "Usage: genpass [positive length]" >&2
        return 1
    fi

    # Filter finite chunks so tr ends at EOF rather than relying on SIGPIPE.
    while (( ${#password} < length )); do
        if ! chunk=$(set -o pipefail; head -c 4096 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9') || [[ -z "$chunk" ]]; then
            echo "Unable to generate password" >&2
            return 1
        fi
        password+="$chunk"
    done
    printf '%s\n' "${password:0:$length}"
}

# Generate QR code in terminal (uses qrencode or web API)
# Usage: qr "https://example.com"
qr() {
    if has_command qrencode; then
        qrencode -t UTF8 "$1"
    elif has_command curl; then
        curl -s "https://qr-server.com/api/qr?size=200x200&data=$(urlencode "$1")"
    else
        echo "QR code generation not available"
    fi
}

# ================================================================
# SYSTEM INFORMATION
# ================================================================
# System diagnostics and information display.

# Display system information summary (OS, hardware, disk usage)
# Usage: sysinfo
sysinfo() {
    echo "=== System Information ==="
    echo "OS: $DOTFILES_OS"
    echo "Distro: $DOTFILES_DISTRO${DOTFILES_CODENAME:+ ($DOTFILES_CODENAME)}"
    echo "Architecture: $DOTFILES_ARCH"
    echo "Container: $DOTFILES_CONTAINER"
    echo "Package Manager: $DOTFILES_PKG_MGR"
    echo "Homebrew Path: $DOTFILES_HOMEBREW_PATH"
    echo
    echo "=== Hardware ==="
    if is_macos; then
        system_profiler SPHardwareDataType | grep -E "(Model Name|Chip|Memory)"
    else
        echo "CPU: $(cat /proc/cpuinfo | grep 'model name' | head -1 | cut -d: -f2 | xargs)"
        echo "Memory: $(free -h | grep '^Mem:' | awk '{print $2}')"
    fi
    echo
    echo "=== Disk Usage ==="
    df -h | grep -E '^/|^tmpfs'
}

# Check if system reboot is required (Linux only)
# Usage: needreboot
needreboot() {
    if is_linux; then
        if [[ -f /var/run/reboot-required ]]; then
            echo "*** System restart required ***"
            [[ -f /var/run/reboot-required.pkgs ]] && cat /var/run/reboot-required.pkgs
        else
            echo "No reboot required"
        fi
    else
        echo "Reboot check only available on Linux"
    fi
}

# ================================================================
# DOCKER FUNCTIONS
# ================================================================
# Docker container, image, and volume management utilities.

# Clean up Docker: remove stopped containers, unused images, volumes, networks
# WARNING: This removes all unused Docker resources!
# Usage: docker-clean
docker-clean() {
    echo "Cleaning Docker containers, images, and volumes..."
    docker container prune -f
    docker image prune -a -f
    docker volume prune -f
    docker network prune -f
}

# Docker Compose up with 1Password secrets injection
# Runs docker compose with secrets from 1Password if available
# Usage: op-dc-up
op-dc-up() {
    if has_command op; then
        op run --no-masking -- docker compose up -d --remove-orphans
    else
        docker compose up -d --remove-orphans
    fi
}

# Docker Compose down with cleanup
# Usage: op-dc-down
op-dc-down() {
    docker compose down --remove-orphans
    docker network prune -f
}

# Get a shell inside a running Docker container
# Lists available containers if called without arguments
# Usage: dsh container-name [shell]  (shell defaults to sh)
dsh() {
    local container="$1"
    local shell="${2:-sh}"

    if [[ -z "$container" ]]; then
        echo "Available containers:"
        docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
        return 1
    fi

    docker exec -it "$container" "$shell"
}

# ================================================================
# CLIPBOARD FUNCTIONS
# ================================================================
# Cross-platform clipboard operations using DOTFILES_COPY_CMD/PASTE_CMD.
# Automatically uses OSC 52 for SSH sessions when terminal supports it.

# Copy to clipboard from stdin or argument
# Usage: echo "text" | clip   OR   clip "text"
# shellcheck disable=SC2120  # args optional when reading from stdin
clip() {
    if [[ -p /dev/stdin ]]; then
        eval "$DOTFILES_COPY_CMD"
    else
        echo -n "$*" | eval "$DOTFILES_COPY_CMD"
    fi
}

# Paste clipboard contents to stdout
# Usage: unclip   OR   unclip > file.txt
unclip() {
    eval "$DOTFILES_PASTE_CMD"
}

# Copy current working directory path to clipboard
# Usage: copypath
copypath() {
    pwd | clip
    echo "Current directory path copied to clipboard"
}

# ================================================================
# TMUX FUNCTIONS
# ================================================================
# Tmux session management. These are functions (not aliases) to
# enable proper tab completion for session names.

# Fix tmux PATH ordering when Homebrew tools aren't being found
# Call this if wrong version of a tool is being used in tmux
fix_tmux_path() {
    if [[ -z "$TMUX" ]]; then
        echo "Not in tmux session"
        return 0
    fi

    local dotfiles_root="${DOTFILES_ROOT:-$HOME/.config/dotfiles}"
    source "$dotfiles_root/common/platform.sh" || return 1

    # paths.sh only rebuilds PATH while its load guard is clear, and the guard is
    # always set in an already-configured shell. Clear it so the re-source runs;
    # paths.sh sets it again only if setup_all_paths actually ran.
    unset DOTFILES_PATHS_LOADED
    source "$dotfiles_root/common/paths.sh" || return 1
    if [[ -z "${DOTFILES_PATHS_LOADED:-}" ]]; then
        echo "Tmux PATH not updated: paths.sh skipped PATH setup (DOTFILES_NO_AUTO_PATH_SETUP is set)" >&2
        return 1
    fi

    tmux set-environment -g PATH "$PATH"
    echo "Tmux PATH updated with Homebrew tools preferred"
}

# Create or attach to a tmux session by name
# Usage: tmux_session my-project
tmux_session() {
    local session="$1"
    if [[ -z "$session" ]]; then
        echo "Usage: tmux_session <session_name>"
        return 1
    fi

    # Update PATH first
    tmux set-environment -g PATH "$PATH" 2>/dev/null || true

    # Create or attach to session
    if tmux has-session -t "$session" 2>/dev/null; then
        tmux attach-session -t "$session"
    else
        tmux new-session -s "$session"
    fi
}

# Tmux attach to session (ta = tmux attach)
# Usage: ta session-name
ta() {
    # Update PATH in tmux environment
    tmux set-environment -g PATH "$PATH" 2>/dev/null || true
    # Attach to session with all arguments passed through
    tmux attach-session -t "$@"
}

# Tmux new session (ts = tmux session)
# Usage: ts session-name
ts() {
    # Update PATH in tmux environment
    tmux set-environment -g PATH "$PATH" 2>/dev/null || true
    # Create new session with all arguments passed through
    tmux new-session -s "$@"
}

# Tmux kill session (tk = tmux kill)
# Usage: tk session-name
tk() {
    tmux kill-session -t "$@"
}

# ================================================================
# HERDR
# ================================================================

# Broadcast a command to many Herdr panes — the analogue of tmux
# `synchronize-panes`, which Herdr has no built-in equivalent for.
# Portable bash/zsh (no zsh-only expansions), so both shells get it.
#
#   herdr-run-all [-a|-t] [-s] [-A] [-n] <command> [args...]
#
#   -a, --all-workspaces  every pane in every workspace (default: current workspace)
#   -t, --tab             only panes in the current tab
#   -s, --include-self    also send to the calling pane (default: skipped)
#   -A, --include-agents  also send to panes hosting a coding agent (default: skipped)
#   -n, --dry-run         print target pane IDs instead of sending
#
# Agent panes are skipped by default because `herdr pane run` writes into the
# agent TUI, not a shell prompt. Use `herdr agent prompt` for those.
herdr-run-all() {
    local scope=workspace include_self=0 include_agents=0 dry_run=0

    while (($#)); do
        case "$1" in
        -a | --all-workspaces) scope=all ;;
        -t | --tab) scope=tab ;;
        -s | --include-self) include_self=1 ;;
        -A | --include-agents) include_agents=1 ;;
        -n | --dry-run) dry_run=1 ;;
        --)
            shift
            break
            ;;
        -*)
            printf 'herdr-run-all: unknown option: %s\n' "$1" >&2
            return 2
            ;;
        *) break ;;
        esac
        shift
    done

    if (($# == 0)); then
        echo "usage: herdr-run-all [-a|-t] [-s] [-A] [-n] <command> [args...]" >&2
        return 2
    fi

    if [[ "${HERDR_ENV:-}" != 1 ]]; then
        echo "herdr-run-all: not inside a Herdr-managed pane (HERDR_ENV != 1)" >&2
        return 1
    fi

    local dep
    for dep in herdr jq; do
        if ! command -v "$dep" >/dev/null 2>&1; then
            printf 'herdr-run-all: required command not found: %s\n' "$dep" >&2
            return 1
        fi
    done

    local tab_only=0
    [[ $scope == tab ]] && tab_only=1

    local -a list_args=()
    if [[ $scope != all ]]; then
        if [[ -z "${HERDR_WORKSPACE_ID:-}" ]]; then
            echo "herdr-run-all: HERDR_WORKSPACE_ID is unset" >&2
            return 1
        fi
        list_args=(--workspace "$HERDR_WORKSPACE_ID")
    fi

    # Captured in two steps (not one pipeline) so a `herdr pane list` failure
    # is reported without needing a function-local pipefail in both shells.
    local json ids
    json="$(herdr pane list "${list_args[@]}")" || {
        echo "herdr-run-all: herdr pane list failed" >&2
        return 1
    }
    ids="$(jq -r \
        --arg tab "${HERDR_TAB_ID:-}" \
        --arg self "${HERDR_PANE_ID:-}" \
        --argjson tab_only "$tab_only" \
        --argjson keep_self "$include_self" \
        --argjson keep_agents "$include_agents" \
        '
            .result.panes[]
            | select($tab_only == 0 or .tab_id == $tab)
            | select($keep_self == 1 or .pane_id != $self)
            | select($keep_agents == 1 or (.agent // null) == null)
            | .pane_id
        ' <<<"$json")" || return 1

    local -a panes=()
    local pane
    while IFS= read -r pane; do
        [[ -n "$pane" ]] && panes+=("$pane")
    done <<<"$ids"

    if ((${#panes[@]} == 0)); then
        echo "herdr-run-all: no matching panes" >&2
        return 1
    fi

    local result_status=0
    for pane in "${panes[@]}"; do
        if ((dry_run)); then
            printf '%s\n' "$pane"
            continue
        fi
        if ! herdr pane run "$pane" "$@" >/dev/null; then
            printf 'herdr-run-all: failed to send to %s\n' "$pane" >&2
            result_status=1
        fi
    done

    return $result_status
}

# ================================================================
# LOAD PLATFORM-SPECIFIC FUNCTIONS
# ================================================================

# Load platform-specific functions if they exist: family file first, distro
# overlay second (see os_module_dirs in common/platform.sh). Only
# common/macos/functions.sh exists today, so most platforms load nothing here.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd)"
while IFS= read -r os_module_dir; do
    [[ -z "$os_module_dir" ]] && continue
    if [[ -f "$SCRIPT_DIR/$os_module_dir/functions.sh" ]]; then
        source "$SCRIPT_DIR/$os_module_dir/functions.sh"
    fi
done <<EOF
$(os_module_dirs)
EOF
unset os_module_dir
