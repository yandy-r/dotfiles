#!/usr/bin/env bash
#
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#
# Repairs VMware Workstation networking on Arch / CachyOS after a fresh
# install or kernel upgrade. Walks the same fix path used to recover from
# the "Could not connect 'EthernetN' to virtual network '/dev/vmnetX'" /
# "Network configuration is missing" failure.
#

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[VMWARE-NET]"

DRY_RUN="${DRY_RUN:-false}"
CHECK_ONLY="${CHECK_ONLY:-false}"

readonly VMWARE_PKG="vmware-workstation"
readonly VMWARE_ETC_DIR="/etc/vmware"
readonly NETWORKING_FILE="${VMWARE_ETC_DIR}/networking"
readonly REQUIRED_MODULES=(vmmon vmnet)
readonly REQUIRED_DEVICES=(/dev/vmmon /dev/vmnet0 /dev/vmnet1 /dev/vmnet8)
readonly REQUIRED_SERVICES=(vmware-networks.service vmware-usbarbitrator.service)

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m'

# ================================================================
# USAGE
# ================================================================

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Repair VMware Workstation host networking on Arch / CachyOS.

OPTIONS:
    --check             Diagnose only, do not modify anything
    --dry-run           Show what would be changed without applying
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    DRY_RUN             Enable dry-run mode (true/false)
    CHECK_ONLY          Enable check-only mode (true/false)

EXAMPLES:
    sudo $SCRIPT_NAME                # Apply the fix
    sudo $SCRIPT_NAME --check        # Diagnose without changing anything
    sudo $SCRIPT_NAME --dry-run      # Show what would be done

WHAT IT DOES:
    1. Verify ${VMWARE_PKG} (AUR) is installed
    2. Ensure DKMS modules are built for the running kernel
    3. modprobe vmmon and vmnet
    4. Seed a default ${NETWORKING_FILE} if missing
    5. vmware-networks --start (creates /dev/vmnetN, dhcpd, NAT, bridge)
    6. systemctl enable --now ${REQUIRED_SERVICES[*]}
    7. Verify /dev/vmnet0, /dev/vmnet1, /dev/vmnet8 and --status output

EXIT CODES:
    0  Success / state already healthy
    1  Generic failure
    2  Required prerequisite missing (package, DKMS, kernel headers)
EOF
}

# ================================================================
# LOGGING
# ================================================================

log_info()  { echo -e "${BLUE}${LOG_PREFIX}${NC} $*"; }
log_ok()    { echo -e "${GREEN}${LOG_PREFIX}${NC} ✓ $*"; }
log_warn()  { echo -e "${YELLOW}${LOG_PREFIX}${NC} ⚠ $*"; }
log_err()   { echo -e "${RED}${LOG_PREFIX}${NC} ✗ $*" >&2; }
log_step()  { echo -e "${CYAN}${LOG_PREFIX}${NC} ▶ $*"; }

run_cmd() {
	if [[ "$DRY_RUN" == "true" ]]; then
		echo -e "${YELLOW}[dry-run]${NC} $*"
		return 0
	fi
	"$@"
}

# ================================================================
# PRE-FLIGHT
# ================================================================

require_root() {
	if [[ $EUID -ne 0 ]]; then
		log_err "This script needs root (re-run with sudo)."
		exit 1
	fi
}

require_pkg() {
	if ! pacman -Qq "$VMWARE_PKG" >/dev/null 2>&1; then
		log_err "${VMWARE_PKG} is not installed (install from AUR first)."
		exit 2
	fi
	log_ok "${VMWARE_PKG} is installed"
}

ensure_dkms() {
	local kernel
	kernel="$(uname -r)"

	if ! command -v dkms >/dev/null 2>&1; then
		log_err "dkms is not installed; install with: pacman -S dkms"
		exit 2
	fi

	# Match: vmware-workstation/<ver>, <kernel>, <arch>: installed
	if dkms status 2>/dev/null | grep -q "vmware-workstation.*${kernel}.*installed"; then
		log_ok "DKMS modules built for ${kernel}"
		return 0
	fi

	log_warn "DKMS modules not built for ${kernel}"
	if [[ "$CHECK_ONLY" == "true" ]]; then return 0; fi

	# Find the package version dkms knows about and rebuild for the running kernel.
	local dkms_ver
	dkms_ver="$(dkms status 2>/dev/null | awk -F'[/,]' '/vmware-workstation/ {print $2; exit}' | tr -d ' ')"
	if [[ -z "$dkms_ver" ]]; then
		log_err "No vmware-workstation source registered with DKMS (check /usr/src)."
		exit 2
	fi

	log_step "Building vmware-workstation/${dkms_ver} for ${kernel}"
	run_cmd dkms install "vmware-workstation/${dkms_ver}" -k "${kernel}" --force
	run_cmd depmod -a "${kernel}"
}

# ================================================================
# FIX STEPS
# ================================================================

load_modules() {
	for mod in "${REQUIRED_MODULES[@]}"; do
		if lsmod | awk '{print $1}' | grep -qx "$mod"; then
			log_ok "module ${mod} is loaded"
			continue
		fi
		log_step "modprobe ${mod}"
		run_cmd modprobe "$mod"
	done
}

seed_networking_config() {
	if [[ -s "$NETWORKING_FILE" ]] && grep -q '^VERSION=' "$NETWORKING_FILE"; then
		log_ok "${NETWORKING_FILE} already populated"
		return 0
	fi

	log_step "Seeding default ${NETWORKING_FILE}"

	if [[ "$DRY_RUN" == "true" ]]; then
		echo -e "${YELLOW}[dry-run]${NC} would write default networking config"
		return 0
	fi

	run_cmd install -d -m 0755 "$VMWARE_ETC_DIR"

	# Minimal default: vmnet1 host-only, vmnet8 NAT. Subnets match VMware's
	# typical first-run defaults; vmware-networks --start regenerates the
	# dhcpd / nat configs and re-stamps the hashes on first launch.
	cat >"$NETWORKING_FILE" <<'EOF'
VERSION=1,0
answer VNET_1_DHCP yes
answer VNET_1_HOSTONLY_NETMASK 255.255.255.0
answer VNET_1_HOSTONLY_SUBNET 172.16.25.0
answer VNET_1_VIRTUAL_ADAPTER yes
answer VNET_8_DHCP yes
answer VNET_8_HOSTONLY_NETMASK 255.255.255.0
answer VNET_8_HOSTONLY_SUBNET 172.16.228.0
answer VNET_8_NAT yes
answer VNET_8_VIRTUAL_ADAPTER yes
EOF
	chmod 0644 "$NETWORKING_FILE"
	log_ok "wrote default ${NETWORKING_FILE}"
}

start_networks() {
	log_step "vmware-networks --start"
	if ! run_cmd vmware-networks --start; then
		log_err "vmware-networks --start failed"
		log_warn "If this is a first-time install with a display, run: sudo vmware-netcfg"
		log_warn "to seed /etc/vmware/networking interactively, then re-run this script."
		exit 1
	fi
}

enable_services() {
	for svc in "${REQUIRED_SERVICES[@]}"; do
		if ! systemctl list-unit-files | grep -q "^${svc}"; then
			log_warn "${svc} not present (skipping)"
			continue
		fi
		log_step "systemctl enable --now ${svc}"
		run_cmd systemctl enable --now "$svc"
	done
}

# ================================================================
# VERIFY
# ================================================================

verify() {
	local failed=0

	echo
	log_info "Verification:"

	for dev in "${REQUIRED_DEVICES[@]}"; do
		if [[ -e "$dev" ]]; then
			log_ok "device ${dev}"
		else
			log_err "missing device ${dev}"
			failed=1
		fi
	done

	if vmware-networks --status 2>&1 | tee /tmp/vmware-net-status.$$ | grep -q "All the services configured"; then
		log_ok "vmware-networks --status reports all services running"
	else
		log_err "vmware-networks --status did not report a healthy state"
		sed 's/^/    /' /tmp/vmware-net-status.$$ >&2 || true
		failed=1
	fi
	rm -f /tmp/vmware-net-status.$$

	for svc in "${REQUIRED_SERVICES[@]}"; do
		if systemctl is-enabled "$svc" >/dev/null 2>&1; then
			log_ok "${svc} enabled"
		else
			log_warn "${svc} not enabled at boot"
		fi
	done

	return $failed
}

# ================================================================
# MAIN
# ================================================================

parse_args() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
			--check)   CHECK_ONLY=true ;;
			--dry-run) DRY_RUN=true ;;
			-h|--help) usage; exit 0 ;;
			*) log_err "Unknown option: $1"; usage; exit 1 ;;
		esac
		shift
	done
}

main() {
	parse_args "$@"

	echo -e "${CYAN}╔════════════════════════════════════════════════════════════╗${NC}"
	echo -e "${CYAN}║  VMware Workstation Networking Repair (${SCRIPT_NAME})   ║${NC}"
	echo -e "${CYAN}╚════════════════════════════════════════════════════════════╝${NC}"
	echo

	if [[ "$DRY_RUN" == "true" ]]; then log_warn "DRY-RUN mode (no changes will be applied)"; fi
	if [[ "$CHECK_ONLY" == "true" ]]; then log_warn "CHECK-ONLY mode (diagnostic only)"; fi

	require_root
	require_pkg
	ensure_dkms

	if [[ "$CHECK_ONLY" == "true" ]]; then
		verify || true
		exit 0
	fi

	load_modules
	seed_networking_config
	start_networks
	enable_services

	echo
	if verify; then
		log_ok "VMware networking is healthy. Power-cycle any running VMs."
		exit 0
	else
		log_err "Verification reported issues — see messages above."
		exit 1
	fi
}

main "$@"
