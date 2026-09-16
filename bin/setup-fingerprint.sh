#!/usr/bin/env bash
#
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[FINGERPRINT]"

# PAM files and the line to insert
readonly PAM_LINE="auth sufficient pam_fprintd.so"
readonly PAM_SUDO="/etc/pam.d/sudo"
readonly PAM_POLKIT="/etc/pam.d/polkit-1"
readonly PAM_POLKIT_SRC="/usr/lib/pam.d/polkit-1"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

NON_INTERACTIVE="${NON_INTERACTIVE:-false}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

# ================================================================
# LOGGING
# ================================================================

log() {
    echo -e "${CYAN}${LOG_PREFIX}${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}${LOG_PREFIX} ✓ $*${NC}" >&2
}

log_warning() {
    echo -e "${YELLOW}${LOG_PREFIX} ⚠ $*${NC}" >&2
}

log_error() {
    echo -e "${RED}${LOG_PREFIX} ✗ $*${NC}" >&2
}

error_exit() {
    log_error "$1"
    exit "${2:-1}"
}

# ================================================================
# UTILITIES
# ================================================================

is_interactive() {
    [[ -t 0 ]] && [[ -t 1 ]]
}

prompt_confirmation() {
    local prompt_message="$1"
    local response

    if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${FORCE_INSTALL:-false}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]; then
        log "Non-interactive mode: auto-approving prompt"
        return 0
    fi

    if ! is_interactive; then
        log_error "Non-interactive shell detected - cannot prompt for confirmation"
        return 1
    fi

    echo -n -e "${YELLOW}${LOG_PREFIX} ${prompt_message} [y/N]: ${NC}" >&2
    read -r response

    response="${response:-n}"
    case "$response" in
        [yY]|[yY][eE][sS]) return 0 ;;
        *) return 1 ;;
    esac
}

# Returns the real (non-root) user when invoked via sudo
get_actual_user() {
    echo "${SUDO_USER:-$USER}"
}

detect_os() {
    case "$(uname -s)" in
        Darwin*) echo "macos" ;;
        Linux*)
            if [[ -f /etc/os-release ]]; then
                # shellcheck source=/dev/null
                source /etc/os-release
                case "${ID:-unknown}" in
                    arch|cachyos|manjaro|endeavouros) echo "arch" ;;
                    *)
                        case "${ID_LIKE:-}" in
                            *arch*) echo "arch" ;;
                            *) echo "linux" ;;
                        esac
                        ;;
                esac
            else
                echo "linux"
            fi
            ;;
        *) echo "unknown" ;;
    esac
}

# ================================================================
# VALIDATION
# ================================================================

validate_arch() {
    local os_type
    os_type="$(detect_os)"
    if [[ "$os_type" != "arch" ]]; then
        error_exit "This script requires an Arch-based system (detected: $os_type). fprintd is available via pacman."
    fi
}

require_root() {
    if [[ $EUID -ne 0 ]]; then
        error_exit "PAM configuration requires root privileges. Run with: sudo $SCRIPT_NAME"
    fi
}

# ================================================================
# FPRINTD INSTALLATION
# ================================================================

install_fprintd() {
    if command -v fprintd-list >/dev/null 2>&1; then
        log "fprintd is already installed"
        return 0
    fi

    log "Installing fprintd (libfprint pulled as dependency)..."
    if ! pacman -S --noconfirm --needed fprintd; then
        error_exit "Failed to install fprintd"
    fi
    log_success "fprintd installed"
}

# ================================================================
# HARDWARE DETECTION
# ================================================================

check_fingerprint_hardware() {
    local output
    output="$(fprintd-list "$(get_actual_user)" 2>&1)" || true

    if echo "$output" | grep -qi "no devices available"; then
        return 1
    fi
    return 0
}

# ================================================================
# PAM CONFIGURATION
# ================================================================

pam_has_fprintd() {
    local pam_file="$1"
    [[ -f "$pam_file" ]] && grep -qF "$PAM_LINE" "$pam_file" 2>/dev/null
}

backup_pam_file() {
    local pam_file="$1"
    local backup="${pam_file}.pre-fingerprint"

    # Keep the original backup - don't overwrite it once it exists
    if [[ -f "$backup" ]]; then
        log "Original backup already exists at $backup"
        return 0
    fi

    if [[ -f "$pam_file" ]]; then
        cp "$pam_file" "$backup"
        log "Backed up $pam_file → $backup"
    fi
}

configure_pam_file() {
    local pam_file="$1"

    if pam_has_fprintd "$pam_file"; then
        log "$pam_file already configured"
        return 0
    fi

    backup_pam_file "$pam_file"

    # Prepend the fprintd line atomically
    local tmp_file
    tmp_file="$(mktemp)"
    {
        echo "$PAM_LINE"
        cat "$pam_file"
    } > "$tmp_file"
    mv "$tmp_file" "$pam_file"

    log_success "Configured fingerprint auth in $pam_file"
}

configure_pam() {
    log "Configuring PAM..."

    # sudo
    if [[ ! -f "$PAM_SUDO" ]]; then
        error_exit "Expected PAM file not found: $PAM_SUDO"
    fi
    configure_pam_file "$PAM_SUDO"

    # polkit-1: copy from /usr/lib/pam.d/ if not yet in /etc/pam.d/
    if [[ ! -f "$PAM_POLKIT" ]]; then
        if [[ -f "$PAM_POLKIT_SRC" ]]; then
            log "Copying $PAM_POLKIT_SRC → $PAM_POLKIT"
            cp "$PAM_POLKIT_SRC" "$PAM_POLKIT"
        else
            log_warning "$PAM_POLKIT_SRC not found; creating minimal polkit PAM config"
            cat > "$PAM_POLKIT" << 'EOF'
#%PAM-1.0
auth       include      system-auth
account    include      system-auth
password   include      system-auth
session    include      system-auth
EOF
        fi
    fi
    configure_pam_file "$PAM_POLKIT"

    log_success "PAM configuration complete"
}

remove_pam_line() {
    local pam_file="$1"

    if [[ ! -f "$pam_file" ]]; then
        log_warning "$pam_file not found, skipping"
        return 0
    fi

    if ! pam_has_fprintd "$pam_file"; then
        log "$pam_file is not configured for fingerprint auth"
        return 0
    fi

    local tmp_file
    tmp_file="$(mktemp)"
    grep -vF "$PAM_LINE" "$pam_file" > "$tmp_file"
    mv "$tmp_file" "$pam_file"

    log_success "Removed fingerprint auth from $pam_file"
}

# ================================================================
# ENROLLMENT
# ================================================================

clear_fingerprints() {
    local actual_user
    actual_user="$(get_actual_user)"

    if ! command -v fprintd-delete >/dev/null 2>&1; then
        error_exit "fprintd-delete not found. Install fprintd first."
    fi

    log "Clearing fingerprint data for $actual_user..."

    # Standard fprintd-delete (clears fprintd's database for this user)
    fprintd-delete "$actual_user" 2>/dev/null || true

    # For sensors with on-chip storage (e.g. Goodix MOC), fprintd-delete only
    # clears fprintd's own database. Stale data on the sensor causes
    # enroll-duplicate even when fprintd-list says "no fingers enrolled".
    # Clearing /var/lib/fprint/ and restarting fprintd forces a full re-sync.
    if [[ $EUID -eq 0 ]]; then
        _purge_fprint_storage
    else
        log_warning "Run with sudo for a full sensor reset: sudo $SCRIPT_NAME --clear-fingerprints"
    fi

    log_success "Fingerprint data cleared"
}

# Wipe fprintd storage and restart the service to force sensor re-sync.
# Handles Goodix MOC and similar match-on-chip sensors that keep their
# own template store separate from fprintd's database.
_purge_fprint_storage() {
    local fprint_dir="/var/lib/fprint"
    if [[ -d "$fprint_dir" ]] && [[ -n "$(ls -A "$fprint_dir" 2>/dev/null)" ]]; then
        log "Purging fprintd storage at $fprint_dir/..."
        rm -rf "${fprint_dir:?}"/*
    fi
    if systemctl is-active --quiet fprintd.service 2>/dev/null; then
        log "Restarting fprintd service..."
        systemctl restart fprintd.service
        sleep 1
    fi
}

enroll_fingerprint() {
    local actual_user
    actual_user="$(get_actual_user)"

    # If fprintd already knows about enrolled fingers, offer to clear first
    if check_enrolled_fingerprints 2>/dev/null; then
        log_warning "Fingerprints already enrolled for $actual_user"
        if prompt_confirmation "Clear existing fingerprints and re-enroll?"; then
            sudo -u "$actual_user" fprintd-delete "$actual_user" 2>/dev/null || true
            _purge_fprint_storage
            log_success "Existing fingerprints cleared"
        else
            log "Keeping existing fingerprints, skipping enrollment"
            return 0
        fi
    fi

    log "Enrolling fingerprint for user: $actual_user"
    log "Follow the prompts to scan your finger multiple times..."
    echo

    # Capture output to detect enroll-duplicate (sensor has stale data
    # that fprintd-list doesn't report — common with Goodix MOC sensors)
    local enroll_output
    enroll_output="$(sudo -u "$actual_user" fprintd-enroll "$actual_user" 2>&1)" && {
        echo "$enroll_output"
        log_success "Fingerprint enrolled"
    } || {
        echo "$enroll_output"
        if echo "$enroll_output" | grep -q "enroll-duplicate"; then
            echo
            log_warning "Sensor has stale fingerprint data from a previous enrollment"
            if prompt_confirmation "Clear sensor data and retry enrollment?"; then
                sudo -u "$actual_user" fprintd-delete "$actual_user" 2>/dev/null || true
                _purge_fprint_storage
                log_success "Sensor data cleared"
                echo
                log "Retrying enrollment..."
                log "Follow the prompts to scan your finger multiple times..."
                echo
                if ! sudo -u "$actual_user" fprintd-enroll "$actual_user"; then
                    error_exit "Fingerprint enrollment failed on retry"
                fi
                log_success "Fingerprint enrolled"
            else
                error_exit "Enrollment aborted — clear stale data with: $SCRIPT_NAME --clear-fingerprints"
            fi
        else
            error_exit "Fingerprint enrollment failed"
        fi
    }
    echo

    log "Verifying enrollment..."
    if sudo -u "$actual_user" fprintd-verify "$actual_user"; then
        log_success "Fingerprint verification successful"
    else
        log_warning "Verification failed — try re-running enrollment"
    fi
}

check_enrolled_fingerprints() {
    local actual_user
    actual_user="$(get_actual_user)"

    if ! command -v fprintd-list >/dev/null 2>&1; then
        return 1
    fi

    local output
    output="$(fprintd-list "$actual_user" 2>&1)" || true

    # Enrolled fingers show up as named entries like "right-index-finger"
    echo "$output" | grep -qE "(left|right)-(index|middle|ring|little|thumb)-finger"
}

# ================================================================
# STATUS
# ================================================================

show_status() {
    local actual_user
    actual_user="$(get_actual_user)"

    echo
    log "Fingerprint authentication status for: $actual_user"
    echo

    if command -v fprintd-list >/dev/null 2>&1; then
        log_success "fprintd: installed"
    else
        log_error  "fprintd: not installed"
    fi

    if command -v fprintd-list >/dev/null 2>&1 && check_fingerprint_hardware 2>/dev/null; then
        log_success "Hardware: fingerprint reader detected"
    else
        log_warning "Hardware: not detected (reader absent or fprintd not started)"
    fi

    if check_enrolled_fingerprints 2>/dev/null; then
        log_success "Enrolled fingerprints: found for $actual_user"
    else
        log_warning "Enrolled fingerprints: none for $actual_user"
    fi

    echo
    if pam_has_fprintd "$PAM_SUDO"; then
        log_success "PAM sudo    ($PAM_SUDO): configured"
    else
        log_warning "PAM sudo    ($PAM_SUDO): not configured"
    fi

    if pam_has_fprintd "$PAM_POLKIT"; then
        log_success "PAM polkit  ($PAM_POLKIT): configured"
    else
        log_warning "PAM polkit  ($PAM_POLKIT): not configured"
    fi

    echo
}

# ================================================================
# REMOVE
# ================================================================

remove_fingerprint_auth() {
    log "Removing fingerprint authentication..."

    remove_pam_line "$PAM_SUDO"
    remove_pam_line "$PAM_POLKIT"

    local actual_user
    actual_user="$(get_actual_user)"

    if command -v fprintd-delete >/dev/null 2>&1; then
        log "Clearing fingerprint data for $actual_user..."
        sudo -u "$actual_user" fprintd-delete "$actual_user" 2>/dev/null || true
        _purge_fprint_storage
        log_success "Fingerprint data cleared"
    fi

    if command -v pacman >/dev/null 2>&1 && pacman -Qi fprintd &>/dev/null; then
        if prompt_confirmation "Uninstall fprintd package?"; then
            if pacman -Rs --noconfirm fprintd; then
                log_success "fprintd uninstalled"
            else
                log_warning "Failed to uninstall fprintd"
            fi
        fi
    fi

    log_success "Fingerprint authentication disabled"
}

# ================================================================
# ENABLE
# ================================================================

enable_fingerprint_auth() {
    validate_arch
    install_fprintd

    log "Checking for fingerprint hardware..."
    if ! check_fingerprint_hardware; then
        error_exit "No fingerprint hardware detected. Verify your reader is supported by libfprint: https://fprint.freedesktop.org/supported-devices.html"
    fi
    log_success "Fingerprint hardware detected"

    configure_pam
    enroll_fingerprint

    echo
    log_success "Fingerprint authentication setup complete!"
    echo
    log "Verify it works:"
    log "  sudo -K && sudo whoami       # should prompt for fingerprint (sudo path)"
    log "  pkexec whoami                # should prompt for fingerprint (polkit path)"
    echo
    log "For 1Password: Settings → Security → 'Unlock using system authentication'"
    log "For Hyprlock: already enabled (auth.fingerprint:enabled = true in hyprlock.conf)"
    echo
    log "If fingerprint auth breaks: password still works as fallback (auth sufficient)"
    log "To undo: sudo $SCRIPT_NAME --remove"
    log "Backups: ${PAM_SUDO}.pre-fingerprint and ${PAM_POLKIT}.pre-fingerprint"
}

# ================================================================
# USAGE
# ================================================================

show_usage() {
    cat << EOF
Usage: $SCRIPT_NAME [OPTIONS]

Configure fingerprint authentication for sudo and polkit on Arch-based systems.
Only modifies /etc/pam.d/sudo and /etc/pam.d/polkit-1 — never system-auth.

OPTIONS:
    --remove              Remove fingerprint auth from PAM (optionally uninstall fprintd)
    --clear-fingerprints  Delete all enrolled fingerprints and purge sensor data
    --status              Show current fingerprint auth status (no root required)
    -h, --help            Show this help message

ENVIRONMENT VARIABLES:
    NON_INTERACTIVE     Skip confirmation prompts (true/false)
    FORCE_INSTALL       Force non-interactive mode (true/false)

EXAMPLES:
    sudo $SCRIPT_NAME                   # Install fprintd, enroll finger, configure PAM
    $SCRIPT_NAME --status               # Check what is/isn't configured (no sudo needed)
    sudo $SCRIPT_NAME --clear-fingerprints  # Remove enrolled fingerprints + sensor data
    sudo $SCRIPT_NAME --remove          # Remove all fingerprint auth config

RECOVERY:
    If fingerprint auth causes issues, password auth still works as fallback.
    Original PAM files are backed up with .pre-fingerprint suffix.
    TTY login is never affected (system-auth is not modified).

EOF
}

# ================================================================
# MAIN
# ================================================================

main() {
    local mode="enable"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --remove)
                mode="remove"
                shift
                ;;
            --status)
                mode="status"
                shift
                ;;
            --clear-fingerprints)
                mode="clear"
                shift
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
        esac
    done

    case "$mode" in
        status)
            show_status
            ;;
        clear)
            clear_fingerprints
            ;;
        remove)
            require_root
            remove_fingerprint_auth
            ;;
        enable)
            require_root
            enable_fingerprint_auth
            ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
