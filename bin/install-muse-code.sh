#!/usr/bin/env bash

# Install Meta's Muse Code CLI (`muse`) on Linux with Meta's official installer
# (https://dev.meta.ai/install.sh), the vendor's native Linux route. macOS uses
# the official Homebrew cask instead (the `muse-code-macos` manifest entry).
# There is deliberately no fallback to any other method (AUR, npm, ...): if the
# official installer fails, this script fails.
#
# The upstream installer drops a self-updating launcher at ~/.local/bin/muse,
# which downloads the checksum-verified release binary next to itself
# (muse-bin-<version>, .muse-version). It is run with MUSE_NO_MODIFY_PATH=1:
# these dotfiles manage PATH, and ~/.local/bin is already on it.
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[MUSE-INSTALL]"
readonly DEFAULT_INSTALL_DIR="$HOME/.local/bin"
MUSE_INSTALLER_URL="${MUSE_INSTALLER_URL:-https://dev.meta.ai/install.sh}"
ACTION="${ACTION:-install}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"
MUSE_CODE_INSTALL_DIR="${MUSE_CODE_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install Meta's Muse Code CLI (muse) with the official installer.

OPTIONS:
    --install           Install (default); skip when already present
    --update            Re-run the official installer to fetch the latest release
    --uninstall         Remove the launcher and its binaries; keep config and auth
    --force             Reinstall even when already present; approve prompts
    --dir PATH          Installation directory (default: ~/.local/bin)
    --check             Print the installed version
    --dry-run           Preview without downloads or changes
    -h, --help          Show this help

ENVIRONMENT:
    ACTION                 install, update, uninstall, or check
    MUSE_CODE_INSTALL_DIR  Launcher directory (default: ~/.local/bin)
    FORCE_INSTALL          true to reinstall and approve prompts
    DRY_RUN                true to preview changes
    NON_INTERACTIVE        true to approve prompts (used by dotf)
    NONINTERACTIVE         1 to approve prompts

Uses the official installer only: $MUSE_INSTALLER_URL
(upstream verifies the launcher and release binary SHA256). Shell startup files
are never edited (MUSE_NO_MODIFY_PATH=1). A muse installed another way (e.g. a
Homebrew cask) is refused, not shadowed. The launcher self-updates hourly; set
MUSE_NO_AUTO_UPDATE=1 to pin. ~/.config/muse (settings, auth) and
~/.local/share/muse are never touched. Run 'muse login' after installing.
EOF
}

log() { printf '%s %s\n' "$LOG_PREFIX" "$*" >&2; }
error_exit() {
	log "ERROR: $*"
	exit 1
}

confirm() {
	local response
	if [[ "$FORCE_INSTALL" == true || "${NON_INTERACTIVE:-false}" == true || "${NONINTERACTIVE:-0}" == 1 ]]; then
		return 0
	fi
	[[ -t 0 && -t 1 ]] || error_exit "Set NON_INTERACTIVE=true to approve this action, or use --dry-run"
	printf '%s %s [y/N]: ' "$LOG_PREFIX" "$1" >&2
	read -r response
	case "$response" in
	[yY] | [yY][eE][sS]) return 0 ;;
	*) error_exit "Cancelled" ;;
	esac
}

parse_args() {
	while (($# > 0)); do
		case "$1" in
		--install) ACTION=install ;;
		--update) ACTION=update ;;
		--uninstall) ACTION=uninstall ;;
		--check) ACTION=check ;;
		--force) FORCE_INSTALL=true ;;
		--dry-run) DRY_RUN=true ;;
		--dir)
			[[ $# -ge 2 && -n "$2" && "$2" != --* ]] || error_exit "$1 requires a value"
			MUSE_CODE_INSTALL_DIR="$2"
			shift
			;;
		-h | --help)
			usage
			exit 0
			;;
		*) error_exit "Unknown option: $1" ;;
		esac
		shift
	done
	case "$ACTION" in
	install | update | uninstall | check) ;;
	*) error_exit "Invalid action: $ACTION" ;;
	esac
	[[ "$MUSE_CODE_INSTALL_DIR" == /* && "$MUSE_CODE_INSTALL_DIR" != / ]] || error_exit "Installation directory must be an absolute path other than /"
	MUSE_CODE_INSTALL_DIR="${MUSE_CODE_INSTALL_DIR%/}"
}

target_path() { printf '%s/muse\n' "$MUSE_CODE_INSTALL_DIR"; }

# Prints a muse on PATH that is NOT the managed launcher (e.g. a Homebrew cask
# or the AUR package).
foreign_muse() {
	local found
	found="$(command -v muse 2>/dev/null)" || return 1
	[[ "$found" != "$(target_path)" ]] || return 1
	printf '%s\n' "$found"
}

TEMP_DIR=""
INSTALL_PENDING=false
cleanup() {
	# A fresh install that failed must not leave a launcher that the next run
	# (and dotf's path probe) would treat as installed. An update keeps the
	# previous release: upstream only switches .muse-version after the new
	# binary is downloaded and verified.
	if [[ "$INSTALL_PENDING" == true ]]; then rm -f -- "$(target_path)" || true; fi
	if [[ -n "$TEMP_DIR" ]]; then rm -rf -- "$TEMP_DIR"; fi
	TEMP_DIR=""
	INSTALL_PENDING=false
}

run_official_installer() {
	local attempt rc=0
	for attempt in 1 2 3; do
		MUSE_NO_MODIFY_PATH=1 MUSE_INSTALL_DIR="$MUSE_CODE_INSTALL_DIR" \
			bash "$TEMP_DIR/install.sh" && rc=0 || rc=$?
		((rc != 0)) || return 0
		log "WARNING: official installer failed (exit $rc, attempt $attempt/3)"
		if ((attempt < 3)); then sleep $((attempt * 5)); fi
	done
	return "$rc"
}

install_muse() {
	local target foreign fresh=false
	target="$(target_path)"
	if foreign="$(foreign_muse)"; then
		error_exit "muse is installed at $foreign; remove it with the tool that installed it, then rerun (only the official installer is supported)"
	fi
	if [[ -e "$target" || -L "$target" ]]; then
		[[ -f "$target" && ! -L "$target" ]] || error_exit "Refusing to replace a symlink/directory at $target"
		if [[ "$ACTION" == install && "$FORCE_INSTALL" != true ]]; then
			log "Already installed at $target; use --update or --force to reinstall"
			return
		fi
	else
		fresh=true
	fi
	case "$(uname -s)" in
	Darwin | Linux) ;;
	*) error_exit "Only macOS and Linux are supported" ;;
	esac
	log "Installing muse to $target using the official installer $MUSE_INSTALLER_URL"
	if [[ "$DRY_RUN" == true ]]; then return; fi
	command -v curl >/dev/null 2>&1 || error_exit "curl is required"
	confirm "Download and execute the official Muse Code installer?"
	TEMP_DIR="$(mktemp -d)"
	trap cleanup EXIT
	trap 'exit 130' INT
	trap 'exit 143' TERM
	# Download completely before execution; a failed/partial download must not run.
	curl -fsSL --proto '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 \
		"$MUSE_INSTALLER_URL" -o "$TEMP_DIR/install.sh" ||
		error_exit "Failed to download $MUSE_INSTALLER_URL"
	bash -n "$TEMP_DIR/install.sh" || error_exit "Downloaded installer is not a valid bash script"
	INSTALL_PENDING="$fresh"
	run_official_installer || error_exit "Official installer failed after 3 attempts"
	[[ -x "$target" ]] || error_exit "The official installer did not create $target"
	MUSE_NO_AUTO_UPDATE=1 "$target" --version >&2 || error_exit "Installed muse cannot start"
	INSTALL_PENDING=false
	cleanup
	trap - EXIT INT TERM
	case ":$PATH:" in
	*:"$MUSE_CODE_INSTALL_DIR":*) ;;
	*) log "WARNING: $MUSE_CODE_INSTALL_DIR is not on PATH; open a new shell or add it to run muse" ;;
	esac
	log "Installed. Run 'muse login' to sign in, then 'muse' to start a session."
}

uninstall_muse() {
	local target foreign f
	target="$(target_path)"
	if [[ ! -e "$target" && ! -L "$target" ]]; then
		if foreign="$(foreign_muse)"; then
			error_exit "muse is installed at $foreign; remove it with the tool that installed it"
		fi
		log "muse is not installed at $target"
		return
	fi
	[[ -f "$target" && ! -L "$target" ]] || error_exit "Refusing to remove a symlink/directory at $target"
	log "Removing $target and its release binaries; configuration and auth (~/.config/muse) will be preserved"
	if [[ "$DRY_RUN" == true ]]; then return; fi
	confirm "Uninstall the Muse Code CLI?"
	rm -f -- "$target"
	# Launcher-managed state next to the launcher (see the upstream launcher).
	for f in "$MUSE_CODE_INSTALL_DIR"/muse-bin-* \
		"$MUSE_CODE_INSTALL_DIR"/.muse-version \
		"$MUSE_CODE_INSTALL_DIR"/.muse-release-info.json \
		"$MUSE_CODE_INSTALL_DIR"/.muse-update-checked-at \
		"$MUSE_CODE_INSTALL_DIR"/.muse-update-notice* \
		"$MUSE_CODE_INSTALL_DIR"/.muse-launcher.previous; do
		[[ -f "$f" && ! -L "$f" ]] && rm -f -- "$f"
	done
	rm -rf -- "$MUSE_CODE_INSTALL_DIR"/.muse-update-lock \
		"$MUSE_CODE_INSTALL_DIR"/.muse-updater \
		"$MUSE_CODE_INSTALL_DIR"/.muse-install-lock
	return 0
}

main() {
	parse_args "$@"
	case "$ACTION" in
	install | update) install_muse ;;
	uninstall) uninstall_muse ;;
	check)
		local target
		target="$(target_path)"
		[[ -x "$target" ]] || error_exit "muse is not installed at $target"
		MUSE_NO_AUTO_UPDATE=1 "$target" --version
		;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
