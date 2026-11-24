#!/usr/bin/env bash

# Install the Antigravity CLI (agy) with Google's official installer, the native
# install route on both macOS and Linux. There is deliberately no fallback to any
# other method (Homebrew cask, npm, ...): if the official installer fails, this
# script fails.
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[AGY-INSTALL]"
readonly DEFAULT_INSTALL_DIR="$HOME/.local/bin"
AGY_INSTALL_URL="${AGY_INSTALL_URL:-https://antigravity.google/cli/install.sh}"
ACTION="${ACTION:-install}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"
AGY_INSTALL_DIR="${AGY_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"

# Shell startup files the upstream `agy install` step may edit (PATH line,
# legacy alias cleanup). They are usually symlinks into this dotfiles repo, which
# manages PATH itself, so they are snapshotted before the installer runs and put
# back afterwards.
readonly SHELL_FILES=(
	.bashrc
	.bash_profile
	.bash_login
	.profile
	.zshrc
	.zprofile
	.zshenv
	.zlogin
	.config/fish/config.fish
)

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install the Antigravity CLI (agy) on macOS or Linux, including WSL.

OPTIONS:
    --install           Install (default); skip when already present
    --update            Reinstall the latest release, even when present
    --uninstall         Remove the agy binary; keep configuration and data
    --force             Reinstall even when already present; approve prompts
    --dir PATH          Installation directory (default: ~/.local/bin)
    --check             Print the installed version
    --dry-run           Preview without downloads or changes
    -h, --help          Show this help

ENVIRONMENT:
    ACTION              install, update, uninstall, or check
    AGY_INSTALL_DIR     Binary directory (default: ~/.local/bin)
    FORCE_INSTALL       true to reinstall and approve prompts
    DRY_RUN             true to preview changes
    NON_INTERACTIVE     true to approve prompts (used by dotf)
    NONINTERACTIVE      1 to approve prompts

Uses the official installer only: $AGY_INSTALL_URL
(upstream verifies the release SHA512). Only the binary in AGY_INSTALL_DIR is
managed; an agy installed another way (e.g. a Homebrew cask) is refused, not
shadowed. agy also self-updates in the background while it runs.

Shell startup files (~/.bashrc, ~/.zshrc, ~/.profile, fish config, ...) are
restored if the installer edits them: these dotfiles manage PATH, and
~/.local/bin is already on it. ~/.gemini (settings, auth) is never touched.
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
			AGY_INSTALL_DIR="$2"
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
	[[ "$AGY_INSTALL_DIR" == /* && "$AGY_INSTALL_DIR" != / ]] || error_exit "Installation directory must be an absolute path other than /"
	AGY_INSTALL_DIR="${AGY_INSTALL_DIR%/}"
}

target_path() { printf '%s/agy\n' "$AGY_INSTALL_DIR"; }

# Prints an agy on PATH that is NOT the managed binary (e.g. a Homebrew cask).
foreign_agy() {
	local found
	found="$(command -v agy 2>/dev/null)" || return 1
	[[ "$found" != "$(target_path)" ]] || return 1
	printf '%s\n' "$found"
}

TEMP_DIR=""
BACKUP_BINARY=""
SHELL_SNAPSHOT_PENDING=false
cleanup() {
	# An interrupted installer (Ctrl+C, SIGTERM) never reaches the normal
	# restore, so put the shell startup files back before dropping the snapshot.
	if [[ "$SHELL_SNAPSHOT_PENDING" == true ]]; then restore_shell_files || true; fi
	# Any unsuccessful update (failed, interrupted, or a new binary that fails
	# validation) puts the previous binary back. A successful update deletes the
	# backup before getting here.
	if [[ -n "$BACKUP_BINARY" && -f "$BACKUP_BINARY" ]]; then
		rm -f -- "$(target_path)" || true
		mv -f -- "$BACKUP_BINARY" "$(target_path)" || true
	fi
	if [[ -n "$TEMP_DIR" ]]; then rm -rf -- "$TEMP_DIR"; fi
	TEMP_DIR=""
	BACKUP_BINARY=""
}

# Record each shell startup file's content (or absence) before the installer.
snapshot_shell_files() {
	local rel file
	mkdir -p "$TEMP_DIR/shell"
	for rel in "${SHELL_FILES[@]}"; do
		file="$HOME/$rel"
		if [[ -f "$file" ]]; then
			cp -- "$file" "$TEMP_DIR/shell/${rel//\//__}"
		fi
	done
	SHELL_SNAPSHOT_PENDING=true
}

# Undo any installer edit to a shell startup file. Content is written back
# through the existing path, so a symlink into the dotfiles repo stays a symlink
# and its target regains the exact bytes it had before.
restore_shell_files() {
	local rel file saved
	for rel in "${SHELL_FILES[@]}"; do
		file="$HOME/$rel"
		saved="$TEMP_DIR/shell/${rel//\//__}"
		if [[ -f "$saved" ]]; then
			if ! cmp -s -- "$saved" "$file"; then
				log "Restoring $file (the upstream installer edited it; these dotfiles manage PATH)"
				cat -- "$saved" >"$file"
			fi
		elif [[ -f "$file" && ! -L "$file" ]]; then
			log "Removing $file (created by the upstream installer)"
			rm -f -- "$file"
		fi
	done
	SHELL_SNAPSHOT_PENDING=false
}

run_official_installer() {
	local -a args=()
	if [[ "$AGY_INSTALL_DIR" != "$DEFAULT_INSTALL_DIR" ]]; then args=(--dir "$AGY_INSTALL_DIR"); fi
	local attempt rc=0
	for attempt in 1 2 3; do
		snapshot_shell_files
		# The install dir goes first on PATH so the installer's setup step sees
		# agy as already reachable.
		PATH="$AGY_INSTALL_DIR:$PATH" bash "$TEMP_DIR/install.sh" ${args[@]+"${args[@]}"} && rc=0 || rc=$?
		restore_shell_files
		((rc != 0)) || return 0
		log "WARNING: official installer failed (exit $rc, attempt $attempt/3)"
		# A failed attempt must not leave a half-installed binary that the next
		# attempt would mistake for a finished install.
		rm -f -- "$(target_path)"
		if ((attempt < 3)); then sleep $((attempt * 5)); fi
	done
	return "$rc"
}

install_binary() {
	local target foreign
	target="$(target_path)"
	if foreign="$(foreign_agy)"; then
		error_exit "agy is installed at $foreign; remove it with the tool that installed it, then rerun (only the official installer is supported)"
	fi
	if [[ -e "$target" || -L "$target" ]]; then
		[[ -f "$target" && ! -L "$target" ]] || error_exit "Refusing to replace a symlink/directory at $target"
		if [[ "$ACTION" == install && "$FORCE_INSTALL" != true ]]; then
			log "Already installed at $target; use --update or --force to reinstall"
			return
		fi
	fi
	case "$(uname -s)" in
	Darwin | Linux) ;;
	*) error_exit "Only macOS and Linux are supported" ;;
	esac
	log "Installing agy to $target using the official installer $AGY_INSTALL_URL"
	if [[ "$DRY_RUN" == true ]]; then return; fi
	command -v curl >/dev/null 2>&1 || error_exit "curl is required"
	confirm "Download and execute the official Antigravity CLI installer?"
	TEMP_DIR="$(mktemp -d)"
	trap cleanup EXIT
	# Turn signals into a normal exit so the EXIT trap restores everything.
	trap 'exit 130' INT
	trap 'exit 143' TERM
	# Download completely before execution; a failed/partial download must not run.
	curl -fsSL --connect-timeout 10 --max-time 120 "$AGY_INSTALL_URL" -o "$TEMP_DIR/install.sh" ||
		error_exit "Failed to download $AGY_INSTALL_URL"
	# Upstream no-ops when the binary exists, so an update moves it aside first
	# and puts it back if the new install fails.
	if [[ -f "$target" ]]; then
		BACKUP_BINARY="$TEMP_DIR/agy.previous"
		mv -f -- "$target" "$BACKUP_BINARY"
	fi
	if ! run_official_installer; then
		error_exit "Official installer failed after 3 attempts${BACKUP_BINARY:+; the previous agy was restored}"
	fi
	[[ -x "$target" ]] || error_exit "The official installer did not create $target${BACKUP_BINARY:+; the previous agy was restored}"
	"$target" --version >&2 || error_exit "Installed agy cannot start${BACKUP_BINARY:+; the previous agy was restored}"
	if [[ -n "$BACKUP_BINARY" ]]; then rm -f -- "$BACKUP_BINARY"; fi
	cleanup
	trap - EXIT INT TERM
	case ":$PATH:" in
	*:"$AGY_INSTALL_DIR":*) ;;
	*) log "WARNING: $AGY_INSTALL_DIR is not on PATH; open a new shell or add it to run agy" ;;
	esac
	log "Installed. Run agy to sign in and start a session."
}

uninstall_binary() {
	local target foreign
	target="$(target_path)"
	if [[ ! -e "$target" && ! -L "$target" ]]; then
		if foreign="$(foreign_agy)"; then
			error_exit "agy is installed at $foreign; remove it with the tool that installed it"
		fi
		log "agy is not installed at $target"
		return
	fi
	[[ -f "$target" && ! -L "$target" ]] || error_exit "Refusing to remove a symlink/directory at $target"
	log "Removing $target; configuration and data (~/.gemini) will be preserved"
	if [[ "$DRY_RUN" == true ]]; then return; fi
	confirm "Uninstall the Antigravity CLI binary?"
	rm -f -- "$target"
}

main() {
	parse_args "$@"
	case "$ACTION" in
	install | update) install_binary ;;
	uninstall) uninstall_binary ;;
	check)
		local target
		target="$(target_path)"
		[[ -x "$target" ]] || error_exit "agy is not installed at $target"
		"$target" --version
		;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
