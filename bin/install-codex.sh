#!/usr/bin/env bash

# Install the OpenAI Codex CLI with the official standalone installer.
# Migrates away from npm/pnpm/Bun/Homebrew installs, which keep breaking
# because the JS wrapper and its bundled server drift out of sync.
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[CODEX-INSTALL]"
readonly INSTALL_URL="https://chatgpt.com/codex/install.sh"
readonly NPM_PACKAGE="@openai/codex"
ACTION="${ACTION:-install}"
CODEX_VERSION="${CODEX_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"
CODEX_INSTALL_DIR="${CODEX_INSTALL_DIR:-$HOME/.local/bin}"
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install the Codex CLI on macOS or Linux, including WSL, with the official
standalone installer, migrating away from npm/pnpm/Bun/Homebrew installs.

OPTIONS:
    --install           Install (default); skip when already present
    --update            Install the latest/requested version, even when present
    --uninstall         Remove the standalone install; keep configuration and data
    --force             Reinstall even when already present; approve prompts
    --version VERSION   Release version (e.g. 0.163.0 or v0.163.0; default: latest)
    --dir PATH          Installation directory (default: ~/.local/bin)
    --check             Print the installed version and foreign installs
    --dry-run           Preview without downloads or changes
    -h, --help          Show this help

ENVIRONMENT:
    ACTION              install, update, uninstall, or check
    CODEX_VERSION       Release version (default: latest)
    CODEX_INSTALL_DIR   Standalone binary directory (default: ~/.local/bin)
    CODEX_HOME          Codex home directory (default: ~/.codex)
    FORCE_INSTALL       true to reinstall and approve prompts
    DRY_RUN             true to preview changes
    NON_INTERACTIVE     true to approve prompts (used by dotf)
    NONINTERACTIVE      1 to approve prompts

Uses $INSTALL_URL (upstream verifies the release SHA256). Only standalone
installs under CODEX_HOME/packages/standalone are updated or removed.
Configuration, authentication, and sessions in CODEX_HOME (config.toml,
auth.json, sessions, skills, packages/app-server-daemon) are preserved:
uninstall removes only the standalone package directory, the install-dir
symlinks, and the PATH blocks the upstream installer appends to shell files
(dotfiles manage PATH, so those blocks are always stripped again).
EOF
}

log() { printf '%s %s\n' "$LOG_PREFIX" "$*" >&2; }
error_exit() { log "ERROR: $*"; exit 1; }

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
		--version | --dir)
			[[ $# -ge 2 && -n "$2" && "$2" != --* ]] || error_exit "$1 requires a value"
			if [[ "$1" == --version ]]; then CODEX_VERSION="$2"; else CODEX_INSTALL_DIR="$2"; fi
			shift
			;;
		-h | --help) usage; exit 0 ;;
		*) error_exit "Unknown option: $1" ;;
		esac
		shift
	done
	case "$ACTION" in
	install | update | uninstall | check) ;;
	*) error_exit "Invalid action: $ACTION" ;;
	esac
	[[ "$CODEX_INSTALL_DIR" == /* && "$CODEX_INSTALL_DIR" != / ]] || error_exit "Installation directory must be an absolute path other than /"
	CODEX_INSTALL_DIR="${CODEX_INSTALL_DIR%/}"
	[[ "$CODEX_HOME" == /* && "$CODEX_HOME" != / ]] || error_exit "Codex home must be an absolute path other than /"
	CODEX_HOME="${CODEX_HOME%/}"
	if [[ "$CODEX_VERSION" != latest ]]; then
		CODEX_VERSION="${CODEX_VERSION#rust-v}"
		CODEX_VERSION="${CODEX_VERSION#v}"
		[[ "$CODEX_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-alpha(\.[0-9]+){0,2}|-beta(\.[0-9]+)?)?$ ]] || error_exit "Expected a release version, got: $CODEX_VERSION"
	fi
}

native_binary_path() {
	local current
	current="$CODEX_HOME/packages/standalone/current"
	if [[ -x "$current/bin/codex" ]]; then
		printf '%s\n' "$current/bin/codex"
		return 0
	fi
	if [[ -x "$current/codex" ]]; then
		printf '%s\n' "$current/codex"
		return 0
	fi
	return 1
}

native_version_string() {
	local bin
	bin="$(native_binary_path)" || return 1
	"$bin" --version 2>/dev/null
}

# True when the link is a symlink whose target lies under the given root.
link_points_under() {
	local link="$1" root="$2" target
	[[ -L "$link" ]] || return 1
	target="$(readlink "$link")" || return 1
	case "$target" in
	/*) ;;
	*) target="$(dirname "$link")/$target" ;;
	esac
	[[ "$target" == "$root/"* ]]
}

# Native-installed marker the dotf manifest probes: the versioned binary must
# exist and the install-dir link must point into the standalone root.
native_installed() {
	native_binary_path >/dev/null || return 1
	link_points_under "$CODEX_INSTALL_DIR/codex" "$CODEX_HOME/packages/standalone"
}

npm_foreign() {
	local listing
	command -v npm >/dev/null 2>&1 || return 1
	listing="$(npm ls -g --depth=0 "$NPM_PACKAGE" 2>/dev/null)" || return 1
	printf '%s\n' "$listing" | command grep -Fq "$NPM_PACKAGE" || return 1
	printf 'npm\tglobal package %s\n' "$NPM_PACKAGE"
}

pnpm_foreign() {
	command -v pnpm >/dev/null 2>&1 || return 1
	pnpm ls -g --depth=0 --json "$NPM_PACKAGE" 2>/dev/null | command grep -Fq "\"$NPM_PACKAGE\"" || return 1
	printf 'pnpm\tglobal package %s\n' "$NPM_PACKAGE"
}

bun_foreign() {
	local pkg_dir
	command -v bun >/dev/null 2>&1 || return 1
	pkg_dir="${BUN_INSTALL:-$HOME/.bun}/install/global/node_modules/$NPM_PACKAGE"
	[[ -e "$pkg_dir" ]] || return 1
	printf 'bun\t%s\n' "$pkg_dir"
}

brew_foreign() {
	command -v brew >/dev/null 2>&1 || return 1
	if brew list --cask codex >/dev/null 2>&1; then
		printf 'brew\tcask codex\n'
		return 0
	fi
	if brew list --formula codex >/dev/null 2>&1; then
		printf 'brew\tformula codex\n'
		return 0
	fi
	return 1
}

# System packages are reported but never removed.
system_foreign() {
	local candidate owner found=0
	for candidate in /usr/bin/codex /usr/local/bin/codex; do
		[[ -e "$candidate" || -L "$candidate" ]] || continue
		owner=""
		if command -v rpm >/dev/null 2>&1; then
			owner="$(rpm -qf -- "$candidate" 2>/dev/null)" || owner=""
		fi
		if [[ -z "$owner" ]] && command -v dpkg >/dev/null 2>&1; then
			owner="$(dpkg -S -- "$candidate" 2>/dev/null)" || owner=""
			owner="${owner%%$'\n'*}"
		fi
		if [[ -z "$owner" ]] && command -v pacman >/dev/null 2>&1; then
			owner="$(pacman -Qo -- "$candidate" 2>/dev/null)" || owner=""
			owner="${owner%%$'\n'*}"
		fi
		case "$owner" in
		*"not owned"* | *"no path found"* | "") owner="" ;;
		esac
		if [[ -n "$owner" ]]; then
			printf 'system\t%s (%s)\n' "$candidate" "$owner"
			found=1
		fi
	done
	[[ "$found" -eq 1 ]]
}

# Prints lines of "manager<TAB>detail" for every foreign Codex install.
foreign_installs() {
	local found=0
	if npm_foreign; then found=1; fi
	if pnpm_foreign; then found=1; fi
	if bun_foreign; then found=1; fi
	if brew_foreign; then found=1; fi
	if system_foreign; then found=1; fi
	[[ "$found" -eq 1 ]]
}

log_foreign_entries() {
	local manager detail
	while IFS=$'\t' read -r manager detail; do
		[[ -n "$manager" ]] || continue
		log "  $manager: $detail"
	done
}

migrate_foreign_installs() {
	local entries manager detail
	entries="$(foreign_installs || true)"
	[[ -n "$entries" ]] || return 0
	log "Found foreign Codex installs to migrate away from:"
	log_foreign_entries <<<"$entries"
	if command grep -qv '^system'$'\t' <<<"$entries"; then
		if [[ "$DRY_RUN" == true ]]; then
			log "Dry run: npm/pnpm/Bun/Homebrew installs would be removed"
			return 0
		fi
		confirm "Remove npm/pnpm/Bun/Homebrew Codex installs and use the standalone installer instead?"
	elif [[ "$DRY_RUN" == true ]]; then
		return 0
	fi
	while IFS=$'\t' read -r manager detail; do
		[[ -n "$manager" ]] || continue
		case "$manager" in
		npm)
			log "Removing npm Codex package ($detail)..."
			npm uninstall -g "$NPM_PACKAGE" || log "WARNING: failed to remove the npm Codex package; continuing"
			;;
		pnpm)
			log "Removing pnpm Codex package ($detail)..."
			pnpm remove -g "$NPM_PACKAGE" || log "WARNING: failed to remove the pnpm Codex package; continuing"
			;;
		bun)
			log "Removing Bun Codex package ($detail)..."
			bun remove -g "$NPM_PACKAGE" || log "WARNING: failed to remove the Bun Codex package; continuing"
			;;
		brew)
			if [[ "$detail" == cask* ]]; then
				log "Removing Homebrew Codex cask..."
				brew uninstall --cask codex || log "WARNING: failed to remove the Homebrew Codex cask; continuing"
			else
				log "Removing Homebrew Codex formula..."
				brew uninstall codex || log "WARNING: failed to remove the Homebrew Codex formula; continuing"
			fi
			;;
		system)
			log "WARNING: $detail is system-managed; remove it with rpm -e, apt remove, or pacman -R as appropriate. Ensure $CODEX_INSTALL_DIR precedes it on PATH."
			;;
		esac
	done <<<"$entries"
	hash -r
}

# The upstream installer appends a PATH block to shell files, which are often
# symlinks into the dotfiles repo that manages PATH itself. Strip the block
# from the resolved target in place so symlinks survive.
strip_upstream_path_blocks() {
	local file target tmp
	for file in "$HOME/.zshrc" "$HOME/.bashrc" "$HOME/.bash_profile" "$HOME/.zprofile" "$HOME/.profile"; do
		[[ -e "$file" || -L "$file" ]] || continue
		target="$file"
		if [[ -L "$file" ]]; then
			target="$(readlink "$file")"
			case "$target" in
			/*) ;;
			*) target="$(dirname "$file")/$target" ;;
			esac
		fi
		[[ -f "$target" ]] || continue
		command grep -Fq '# >>> Codex installer >>>' "$target" || continue
		if [[ "$DRY_RUN" == true ]]; then
			log "Dry run: would remove the Codex installer PATH block from $file"
			continue
		fi
		log "Removing the Codex installer PATH block from $file"
		tmp="$(mktemp)"
		command awk 'BEGIN { skip = 0 } /# >>> Codex installer >>>/ { skip = 1; next } /# <<< Codex installer <<</ { skip = 0; next } skip == 0 { print }' "$target" >"$tmp"
		cat -- "$tmp" >"$target"
		rm -f -- "$tmp"
	done
}

TEMP_DIR=""
FORCE_BACKUP=""
FORCE_RELEASE_DIR=""

cleanup_temp() {
	if [[ -n "$TEMP_DIR" ]]; then rm -rf -- "$TEMP_DIR"; fi
	TEMP_DIR=""
}

setup_force_backup() {
	local current release_dir
	FORCE_BACKUP=""
	FORCE_RELEASE_DIR=""
	current="$CODEX_HOME/packages/standalone/current"
	[[ -L "$current" || -e "$current" ]] || return 0
	release_dir="$(cd -P "$current" 2>/dev/null && pwd)" || return 0
	[[ -n "$release_dir" && -d "$release_dir" ]] || return 0
	FORCE_RELEASE_DIR="$release_dir"
	FORCE_BACKUP="$release_dir.force-backup.$$"
	log "Saving the current release aside for --force reinstall"
	mv -- "$FORCE_RELEASE_DIR" "$FORCE_BACKUP"
}

restore_force_backup() {
	if [[ -n "$FORCE_BACKUP" && -d "$FORCE_BACKUP" && -n "$FORCE_RELEASE_DIR" ]]; then
		log "Restoring the previous release from backup"
		rm -rf -- "${FORCE_RELEASE_DIR:?}"
		mv -- "$FORCE_BACKUP" "$FORCE_RELEASE_DIR"
	fi
	FORCE_BACKUP=""
	FORCE_RELEASE_DIR=""
}

clear_force_backup() {
	if [[ -n "$FORCE_BACKUP" && -d "$FORCE_BACKUP" ]]; then
		rm -rf -- "$FORCE_BACKUP"
	fi
	FORCE_BACKUP=""
	FORCE_RELEASE_DIR=""
}

install_codex() {
	local bin_link already_version script installed_version resolved
	local -a release_args=()
	bin_link="$CODEX_INSTALL_DIR/codex"
	if [[ "$ACTION" == install && "$FORCE_INSTALL" != true ]] && native_installed; then
		already_version="$(native_version_string || true)"
		if [[ -n "$already_version" ]]; then
			log "Codex is already installed ($already_version); use --update or --force to reinstall"
		else
			log "Codex is already installed; use --update or --force to reinstall"
		fi
		migrate_foreign_installs
		return 0
	fi
	# A regular file or directory here could be a user-owned binary; only the
	# standalone installer's own symlink may be replaced.
	if [[ -e "$bin_link" || -L "$bin_link" ]] && [[ ! -L "$bin_link" ]]; then
		error_exit "Refusing to replace $bin_link; it was not installed by the standalone installer"
	fi
	if [[ -L "$bin_link" ]] && ! link_points_under "$bin_link" "$CODEX_HOME/packages/standalone"; then
		log "WARNING: replacing existing symlink $bin_link that does not point at the standalone install"
	fi
	command -v curl >/dev/null 2>&1 || error_exit "curl is required"
	migrate_foreign_installs
	if [[ "$CODEX_VERSION" != latest ]]; then release_args=(--release "$CODEX_VERSION"); fi
	if ((${#release_args[@]} > 0)); then
		log "Installing Codex CLI $CODEX_VERSION to $bin_link using $INSTALL_URL ${release_args[*]}"
	else
		log "Installing Codex CLI $CODEX_VERSION to $bin_link using $INSTALL_URL"
	fi
	if [[ "$DRY_RUN" == true ]]; then
		log "Dry run: no changes made"
		return 0
	fi
	confirm "Download and execute the official Codex installer?"
	# Download completely before execution; a failed/partial download must not run.
	TEMP_DIR="$(mktemp -d)"
	trap cleanup_temp EXIT
	script="$TEMP_DIR/install.sh"
	curl -fsSL --connect-timeout 10 --max-time 120 "$INSTALL_URL" -o "$script"
	if [[ "$FORCE_INSTALL" == true ]]; then setup_force_backup; fi
	if ! env CODEX_NON_INTERACTIVE=true CODEX_INSTALL_DIR="$CODEX_INSTALL_DIR" CODEX_HOME="$CODEX_HOME" PATH="$CODEX_INSTALL_DIR:$PATH" sh "$script" ${release_args[@]+"${release_args[@]}"}; then
		strip_upstream_path_blocks
		restore_force_backup
		error_exit "Official Codex installer failed"
	fi
	strip_upstream_path_blocks
	installed_version="$("$bin_link" --version)" || { restore_force_backup; error_exit "Installed Codex cannot start"; }
	clear_force_backup
	log "Installed Codex: $installed_version"
	resolved="$(command -v codex || true)"
	if [[ "$resolved" != "$bin_link" ]]; then
		if [[ -z "$resolved" ]]; then
			log "WARNING: $CODEX_INSTALL_DIR is not on PATH; add it to run codex"
		else
			log "WARNING: codex on PATH resolves to $resolved instead of $bin_link; check PATH order"
		fi
	fi
	cleanup_temp
	trap - EXIT
}

uninstall_codex() {
	local bin_link host_link standalone_root entries
	bin_link="$CODEX_INSTALL_DIR/codex"
	host_link="$CODEX_INSTALL_DIR/codex-code-mode-host"
	standalone_root="$CODEX_HOME/packages/standalone"
	if ! native_installed && [[ ! -e "$bin_link" && ! -L "$bin_link" ]]; then
		log "Codex is not installed at $bin_link"
		entries="$(foreign_installs || true)"
		if [[ -n "$entries" ]]; then
			log "Foreign Codex installs remain (not removed):"
			log_foreign_entries <<<"$entries"
		fi
		return 0
	fi
	if [[ -e "$bin_link" && ! -L "$bin_link" ]]; then
		error_exit "Refusing to remove $bin_link; it is not a symlink (possibly a user-owned binary)"
	fi
	if [[ -L "$bin_link" ]] && ! link_points_under "$bin_link" "$standalone_root"; then
		error_exit "Refusing to remove $bin_link; it does not point at the standalone install"
	fi
	if [[ -e "$host_link" || -L "$host_link" ]] && ! link_points_under "$host_link" "$standalone_root"; then
		error_exit "Refusing to remove $host_link; it does not point at the standalone install"
	fi
	log "Removing the Codex standalone install ($bin_link); configuration, auth, and sessions in $CODEX_HOME are preserved"
	if [[ "$DRY_RUN" == true ]]; then
		log "Dry run: no changes made"
		return 0
	fi
	confirm "Uninstall the standalone Codex CLI?"
	rm -f -- "$bin_link" "$host_link"
	rm -rf -- "$standalone_root"
	strip_upstream_path_blocks
	hash -r
	log "Uninstalled Codex; $CODEX_HOME configuration preserved"
	entries="$(foreign_installs || true)"
	if [[ -n "$entries" ]]; then
		log "Foreign Codex installs remain (not removed):"
		log_foreign_entries <<<"$entries"
	fi
}

check_codex() {
	local bin current entries status
	current="$CODEX_HOME/packages/standalone/current"
	status=0
	if bin="$(native_binary_path)"; then
		"$bin" --version || status=1
		log "Native install: $bin"
	else
		log "Native install: not found under $current"
		status=1
	fi
	if command -v codex >/dev/null 2>&1; then
		log "codex on PATH: $(command -v codex)"
	else
		log "codex on PATH: not found"
	fi
	entries="$(foreign_installs || true)"
	if [[ -n "$entries" ]]; then
		log "Foreign installs:"
		log_foreign_entries <<<"$entries"
	fi
	if [[ "$status" -ne 0 ]] && ! command -v codex >/dev/null 2>&1; then
		error_exit "codex is not installed"
	fi
}

main() {
	parse_args "$@"
	case "$ACTION" in
	install | update) install_codex ;;
	uninstall) uninstall_codex ;;
	check) check_codex ;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
