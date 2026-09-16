#!/usr/bin/env bash
#
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[RESOLVED-NM]"

# Configuration
DRY_RUN="${DRY_RUN:-false}"
readonly NM_DROPIN_DIR="/etc/NetworkManager/conf.d"
readonly NM_DROPIN_FILE="${NM_DROPIN_DIR}/dns.conf"
readonly STUB_RESOLV="/run/systemd/resolve/stub-resolv.conf"

# Color codes for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m'

# ================================================================
# USAGE AND HELP
# ================================================================

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Fix systemd-resolved and NetworkManager DNS integration so that
Tailscale MagicDNS (and split-DNS in general) works correctly.

This script configures NetworkManager to delegate DNS management to
systemd-resolved via D-Bus, and sets /etc/resolv.conf as a symlink
to the resolved stub resolver.

Reference: https://tailscale.com/s/resolved-nm

OPTIONS:
    --dry-run           Show what would be changed without applying
    --check             Check current state and exit (no changes)
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DRY_RUN             Enable dry-run mode (true/false)

EXAMPLES:
    $SCRIPT_NAME                    # Apply the fix
    $SCRIPT_NAME --check            # Diagnose without changing anything
    $SCRIPT_NAME --dry-run          # Show what would be done
    sudo $SCRIPT_NAME               # Run with sudo already

WHAT IT DOES:
    1. Creates ${NM_DROPIN_FILE} with [main] dns=systemd-resolved
    2. Replaces /etc/resolv.conf with a symlink to ${STUB_RESOLV}
    3. Restarts NetworkManager and systemd-resolved

SUPPORTED PLATFORMS:
    Linux systems running both NetworkManager and systemd-resolved.
    Tested on CachyOS, Arch, Fedora, Ubuntu.

EOF
	exit "${1:-0}"
}

# ================================================================
# LOGGING FUNCTIONS
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

log_section() {
	echo "" >&2
	echo -e "${BLUE}${LOG_PREFIX} ── $* ──${NC}" >&2
}

error_exit() {
	log_error "$1"
	exit "${2:-1}"
}

# ================================================================
# PREFLIGHT CHECKS
# ================================================================

check_linux() {
	if [[ "$(uname -s)" != "Linux" ]]; then
		error_exit "This script is for Linux systems only (detected: $(uname -s))"
	fi
}

check_systemd_resolved() {
	if ! command -v resolvectl &>/dev/null; then
		error_exit "systemd-resolved is not installed (resolvectl not found)"
	fi

	if ! systemctl is-active --quiet systemd-resolved 2>/dev/null; then
		log_warning "systemd-resolved is not running"
		return 1
	fi

	return 0
}

check_networkmanager() {
	if ! command -v nmcli &>/dev/null; then
		error_exit "NetworkManager is not installed (nmcli not found)"
	fi

	if ! systemctl is-active --quiet NetworkManager 2>/dev/null; then
		log_warning "NetworkManager is not running"
		return 1
	fi

	return 0
}

# ================================================================
# STATE DETECTION
# ================================================================

get_resolv_mode() {
	resolvectl status 2>/dev/null | grep -oP 'resolv\.conf mode:\s+\K\S+' || echo "unknown"
}

is_resolv_symlinked() {
	[[ -L /etc/resolv.conf ]] && [[ "$(readlink -f /etc/resolv.conf)" == "$(readlink -f "$STUB_RESOLV")" ]]
}

nm_dns_setting() {
	# Check the effective dns= value from NM
	if [[ -f "$NM_DROPIN_FILE" ]] && grep -q 'dns=systemd-resolved' "$NM_DROPIN_FILE" 2>/dev/null; then
		echo "systemd-resolved"
		return
	fi

	# Check main config
	if grep -q 'dns=systemd-resolved' /etc/NetworkManager/NetworkManager.conf 2>/dev/null; then
		echo "systemd-resolved"
		return
	fi

	# Check all conf.d files
	local conf_files=()
	if compgen -G "${NM_DROPIN_DIR}/*.conf" &>/dev/null; then
		for f in "${NM_DROPIN_DIR}"/*.conf; do
			if [[ -f "$f" ]] && grep -q 'dns=systemd-resolved' "$f" 2>/dev/null; then
				echo "systemd-resolved"
				return
			fi
		done
	fi

	echo "default"
}

is_already_fixed() {
	local mode nm_dns
	mode="$(get_resolv_mode)"
	nm_dns="$(nm_dns_setting)"

	[[ "$mode" == "stub" ]] && [[ "$nm_dns" == "systemd-resolved" ]]
}

# ================================================================
# DIAGNOSIS
# ================================================================

diagnose() {
	log_section "Current DNS Configuration"

	local mode nm_dns
	mode="$(get_resolv_mode)"
	nm_dns="$(nm_dns_setting)"

	# resolv.conf state
	if [[ -L /etc/resolv.conf ]]; then
		log "resolv.conf: symlink → $(readlink /etc/resolv.conf)"
	else
		log "resolv.conf: regular file (not a symlink)"
	fi
	log "resolv.conf mode: ${mode}"
	log "NetworkManager dns=: ${nm_dns}"

	# Check for the dual search-line bug
	local search_count
	search_count="$(grep -c '^search ' /etc/resolv.conf 2>/dev/null || echo 0)"
	if [[ "$search_count" -gt 1 ]]; then
		log_warning "resolv.conf has ${search_count} 'search' lines (only the last one takes effect)"
	fi

	# Verdict
	echo "" >&2
	if [[ "$mode" == "stub" ]] && [[ "$nm_dns" == "systemd-resolved" ]]; then
		log_success "DNS is correctly wired: NM → systemd-resolved → stub resolv.conf"
		return 0
	else
		log_warning "DNS is misconfigured — MagicDNS/split-DNS may not work reliably"
		[[ "$mode" != "stub" ]] && log "  → resolv.conf mode should be 'stub', got '${mode}'"
		[[ "$nm_dns" != "systemd-resolved" ]] && log "  → NM dns= should be 'systemd-resolved', got '${nm_dns}'"
		return 1
	fi
}

# ================================================================
# FIX FUNCTIONS
# ================================================================

ensure_sudo() {
	if [[ $EUID -ne 0 ]]; then
		if ! command -v sudo &>/dev/null; then
			error_exit "This script requires root privileges. Run with sudo or as root."
		fi
		SUDO="sudo"
	else
		SUDO=""
	fi
}

create_nm_dropin() {
	log_section "NetworkManager Drop-in Configuration"

	if [[ -f "$NM_DROPIN_FILE" ]] && grep -q 'dns=systemd-resolved' "$NM_DROPIN_FILE" 2>/dev/null; then
		log_success "${NM_DROPIN_FILE} already exists with correct setting"
		return 0
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would create ${NM_DROPIN_FILE} with:"
		log "  [main]"
		log "  dns=systemd-resolved"
		return 0
	fi

	log "Creating ${NM_DROPIN_FILE}..."
	$SUDO mkdir -p "$NM_DROPIN_DIR"

	# Back up existing file if it has different content
	if [[ -f "$NM_DROPIN_FILE" ]]; then
		local backup
		backup="${NM_DROPIN_FILE}.bak.$(date +%Y%m%d_%H%M%S)"
		log_warning "Existing file found — backing up to ${backup}"
		$SUDO cp "$NM_DROPIN_FILE" "$backup"
	fi

	printf '[main]\ndns=systemd-resolved\n' | $SUDO tee "$NM_DROPIN_FILE" >/dev/null
	log_success "Created ${NM_DROPIN_FILE}"
}

fix_resolv_symlink() {
	log_section "resolv.conf Symlink"

	if is_resolv_symlinked; then
		log_success "/etc/resolv.conf already symlinked to stub resolver"
		return 0
	fi

	if [[ ! -f "$STUB_RESOLV" ]]; then
		error_exit "Stub resolver file not found at ${STUB_RESOLV} — is systemd-resolved running?"
	fi

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would replace /etc/resolv.conf with symlink → ${STUB_RESOLV}"
		return 0
	fi

	# Back up current resolv.conf if it's a regular file
	if [[ -f /etc/resolv.conf ]] && [[ ! -L /etc/resolv.conf ]]; then
		local backup
		backup="/etc/resolv.conf.bak.$(date +%Y%m%d_%H%M%S)"
		log "Backing up current /etc/resolv.conf → ${backup}"
		$SUDO cp /etc/resolv.conf "$backup"
	fi

	log "Creating symlink: /etc/resolv.conf → ${STUB_RESOLV}"
	$SUDO ln -sf "$STUB_RESOLV" /etc/resolv.conf
	log_success "resolv.conf symlink created"
}

restart_services() {
	log_section "Restarting Services"

	if [[ "$DRY_RUN" == "true" ]]; then
		log "DRY-RUN: Would restart NetworkManager and systemd-resolved"
		return 0
	fi

	log "Restarting NetworkManager..."
	$SUDO systemctl restart NetworkManager

	log "Restarting systemd-resolved..."
	$SUDO systemctl restart systemd-resolved

	# Brief pause for services to settle
	sleep 1

	log_success "Services restarted"
}

# ================================================================
# VERIFICATION
# ================================================================

verify() {
	log_section "Verification"

	local failures=0

	# Check resolv.conf mode
	local mode
	mode="$(get_resolv_mode)"
	if [[ "$mode" == "stub" ]]; then
		log_success "resolv.conf mode: stub"
	else
		log_error "resolv.conf mode: ${mode} (expected: stub)"
		((failures++))
	fi

	# Check symlink
	if is_resolv_symlinked; then
		log_success "/etc/resolv.conf → ${STUB_RESOLV}"
	else
		log_error "/etc/resolv.conf is not correctly symlinked"
		((failures++))
	fi

	# Check NM setting
	local nm_dns
	nm_dns="$(nm_dns_setting)"
	if [[ "$nm_dns" == "systemd-resolved" ]]; then
		log_success "NetworkManager dns=systemd-resolved"
	else
		log_error "NetworkManager dns=${nm_dns} (expected: systemd-resolved)"
		((failures++))
	fi

	# Check search domains are merged correctly
	local search_count
	search_count="$(grep -c '^search ' /etc/resolv.conf 2>/dev/null || echo 0)"
	if [[ "$search_count" -le 1 ]]; then
		log_success "Search domains correctly merged on single line"
	else
		log_warning "Multiple search lines detected (${search_count})"
		((failures++))
	fi

	# Test basic resolution
	if host google.com &>/dev/null 2>&1 || nslookup google.com &>/dev/null 2>&1; then
		log_success "External DNS resolution working"
	else
		log_warning "External DNS resolution test failed (may be transient)"
	fi

	echo "" >&2
	if [[ "$failures" -eq 0 ]]; then
		log_success "All checks passed — MagicDNS should work correctly"
		log "Test with: ping <tailscale-hostname>"
	else
		log_error "${failures} check(s) failed — review output above"
		return 1
	fi
}

# ================================================================
# COMMAND LINE ARGUMENT PARSING
# ================================================================

CHECK_ONLY=false

while [[ $# -gt 0 ]]; do
	case "$1" in
	--dry-run)
		DRY_RUN="true"
		shift
		;;
	--check)
		CHECK_ONLY=true
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

# ================================================================
# MAIN EXECUTION
# ================================================================

main() {
	log_section "Tailscale MagicDNS — resolved + NetworkManager Fix"

	[[ "$DRY_RUN" == "true" ]] && log_warning "DRY-RUN MODE: No changes will be made"

	# Preflight
	check_linux
	check_systemd_resolved
	check_networkmanager

	# Check-only mode
	if [[ "$CHECK_ONLY" == "true" ]]; then
		diagnose
		exit $?
	fi

	# Already fixed?
	if is_already_fixed; then
		log_success "DNS is already correctly configured"
		diagnose
		exit 0
	fi

	# Show current state
	diagnose || true
	echo "" >&2

	# Apply fixes
	ensure_sudo
	create_nm_dropin
	fix_resolv_symlink
	restart_services

	# Verify
	if [[ "$DRY_RUN" != "true" ]]; then
		verify
	else
		log_warning "DRY-RUN: Skipping verification (no changes were made)"
		log "Run without --dry-run to apply changes"
	fi
}

main "$@"
