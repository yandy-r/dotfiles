#!/usr/bin/env bash
# Open NetworkManager Wi-Fi UI: prefer nmtui, else show nmcli status and hints.
# Intended for Hypr keybind / launcher; pairs with nmcli aliases in common/linux/aliases.sh.

set -euo pipefail

readonly SCRIPT_NAME="${0##*/}"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [--help]

Opens NetworkManager Wi-Fi management:
  - If nmtui is available, runs it (full-screen TUI).
  - Otherwise prints nmcli status, Wi-Fi scan, and connection examples.

Requires NetworkManager (nmcli). On Arch/CachyOS:
  sudo pacman -S networkmanager
  # TUI (if split package): sudo pacman -S network-manager-nmtui
EOF
}

nmcli_fallback() {
	echo "$SCRIPT_NAME: nmtui not found; showing nmcli summary." >&2
	echo "Install network-manager-nmtui (or equivalent) for a full TUI." >&2
	echo >&2

	nmcli general status 2>/dev/null || true
	nmcli dev status 2>/dev/null || true
	echo >&2
	echo "Wi-Fi networks:" >&2
	if ! nmcli -f IN-USE,SSID,SIGNAL,SECURITY,BARS dev wifi list 2>/dev/null; then
		nmcli dev wifi list 2>/dev/null || true
	fi
	echo >&2
	cat <<'EOF'
Shell aliases: wifilist, wificonnect, wifidisconnect, connections

Connect:
  nmcli dev wifi connect "SSID"
  nmcli dev wifi connect "SSID" password "PASSWORD"

Disconnect Wi-Fi device (replace wlan0 if needed):
  nmcli dev disconnect wlan0
EOF
	echo >&2
	read -r -p "Press Enter to close… " _
}

main() {
	case "${1:-}" in
		-h | --help)
			usage
			exit 0
			;;
	esac

	if command -v nmtui >/dev/null 2>&1; then
		exec nmtui
	fi

	if command -v nmcli >/dev/null 2>&1; then
		nmcli_fallback
		exit 0
	fi

	cat <<EOF >&2
$SCRIPT_NAME: neither nmcli nor nmtui found.

Install NetworkManager on Arch/CachyOS:
  sudo pacman -S networkmanager
  sudo systemctl enable --now NetworkManager

Optional TUI:
  sudo pacman -S network-manager-nmtui
EOF
	read -r -p "Press Enter to close… " _
	exit 1
}

main "$@"
