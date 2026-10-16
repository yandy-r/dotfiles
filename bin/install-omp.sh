#!/usr/bin/env bash

# Install oh-my-pi's standalone omp binary using the official installer.
# Explicit --binary avoids implicit Bun installation and shell config changes.
set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[OMP-INSTALL]"
readonly INSTALL_URL="https://omp.sh/install"
ACTION="${ACTION:-install}"
OMP_VERSION="${OMP_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"
OMP_SKIP_DEBUGGER="${OMP_SKIP_DEBUGGER:-false}"
PI_INSTALL_DIR="${PI_INSTALL_DIR:-$HOME/.local/bin}"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install oh-my-pi (omp) on macOS or Linux, including WSL.

OPTIONS:
    --install           Install (default); skip when already present
    --update            Install the latest/requested version, even when present
    --uninstall         Remove the standalone binary; keep configuration and data
    --force             Reinstall even when already present; approve prompts
    --version VERSION   Release version (e.g. 18.8.9 or v18.8.9; default: latest)
    --dir PATH          Installation directory (default: ~/.local/bin)
    --debugger-only     Only ensure the Python debugger (debugpy); never touch omp
    --skip-debugger     Install/update omp without checking debugpy
    --check             Print the installed version and debugger status
    --dry-run           Preview without downloads or changes
    -h, --help          Show this help

ENVIRONMENT:
    ACTION              install, update, uninstall, debugger, or check
    OMP_VERSION         Release version (default: latest)
    OMP_SKIP_DEBUGGER   true to skip the debugpy check during install/update
    PI_INSTALL_DIR      Standalone binary directory (default: ~/.local/bin)
    FORCE_INSTALL       true to reinstall and approve prompts
    DRY_RUN             true to preview changes
    NON_INTERACTIVE     true to approve prompts (used by dotf)
    NONINTERACTIVE      1 to approve prompts

Uses https://omp.sh/install --binary (upstream verifies the binary SHA256).
Only standalone installs in PI_INSTALL_DIR are updated/removed. For an omp
installed elsewhere (e.g. Bun or Homebrew), use its original package manager.
No shell files, ~/.omp, or XDG configuration/data directories are modified.

DEBUGGER:
    omp debugs Python with 'python -m debugpy.adapter'. Install/update and
    --debugger-only check that 'python' on PATH can import it (falling back to
    python3 when python is absent). If anything is missing, interactive runs
    explain and ask; non-interactive runs install it with a notice.
    Externally managed system Python uses distro packages (Debian/Ubuntu:
    python3-debugpy + python-is-python3; Fedora: python3-debugpy +
    python-unversioned-command; Arch: python-debugpy), via sudo when needed.
    Other Pythons use pip (virtualenv, writable site, or --user); a python3-only
    install also gets ~/.local/bin/python, never overwriting an existing file.
    Debugger failures warn during omp install/update but fail --debugger-only.
    Uninstall leaves debugpy in place because it is a shared Python package.
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

# Approval for optional work. Unlike confirm, a missing TTY means "headless", which
# approves with a notice; an interactive "no" returns 1 instead of exiting.
approve_optional() {
	local response
	if [[ "$FORCE_INSTALL" == true || "${NON_INTERACTIVE:-false}" == true || "${NONINTERACTIVE:-0}" == 1 || ! -t 0 || ! -t 1 ]]; then
		log "NOTICE: auto-approved (non-interactive or --force); $2"
		return 0
	fi
	printf '%s %s [y/N]: ' "$LOG_PREFIX" "$1" >&2
	read -r response || return 1
	case "$response" in
	[yY] | [yY][eE][sS]) return 0 ;;
	*) return 1 ;;
	esac
}

DEBUG_PYTHON=python
resolve_debug_python() {
	if command -v python >/dev/null 2>&1; then
		DEBUG_PYTHON=python
	# Only fall back when no `python` resolves. Keep python3 explicit.
	elif command -v python3 >/dev/null 2>&1; then
		DEBUG_PYTHON=python3
	else
		log "WARNING: Python is not installed; install Python 3 then rerun --debugger-only"
		return 1
	fi
}

debugpy_installed() {
	"$DEBUG_PYTHON" -c 'import debugpy.adapter' >/dev/null 2>&1
}

# System Python is owned by the OS, not pip. Ubuntu/Debian also need the
# unversioned command because omp's default adapter launches `python`.
install_system_debugger() {
	local -a command_args
	if [[ -f /etc/debian_version ]] && command -v apt-get >/dev/null 2>&1; then
		command_args=(apt-get install -y python3-debugpy)
		if [[ "$DEBUG_PYTHON" == python3 ]]; then command_args+=(python-is-python3); fi
	elif command -v dnf >/dev/null 2>&1; then
		command_args=(dnf install -y python3-debugpy)
		if [[ "$DEBUG_PYTHON" == python3 ]]; then command_args+=(python-unversioned-command); fi
	elif command -v pacman >/dev/null 2>&1; then
		command_args=(pacman -S --needed --noconfirm python-debugpy)
	else
		log "WARNING: install your distro's debugpy package and ensure python is on PATH; no supported system package manager found"
		return 1
	fi
	if ((EUID != 0)); then command_args=(sudo "${command_args[@]}"); fi
	log "System Python needs distro packages: ${command_args[*]}"
	if [[ "$DRY_RUN" == true ]]; then return 0; fi
	approve_optional "Install debugger dependencies with ${command_args[*]}?" "installing debugger dependencies: ${command_args[*]}" || return 1
	"${command_args[@]}" || return 1
	hash -r
	resolve_debug_python || return 1
	if [[ "$DEBUG_PYTHON" != python ]] || ! debugpy_installed; then
		log "WARNING: system packages installed, but python cannot import debugpy.adapter; check PATH/interpreter selection"
		return 1
	fi
	log "Python debugger ready (system packages)"
}

# For non-system Python3-only installs, provide the command omp expects without
# changing /usr/bin or shell config. Never overwrite an existing local command.
ensure_python_command() {
	[[ "$DEBUG_PYTHON" == python3 ]] || return 0
	local target="$HOME/.local/bin/python" interpreter
	interpreter="$(command -v python3)"
	log "omp needs the unversioned python command; would link $target to $interpreter"
	if [[ "$DRY_RUN" == true ]]; then return 0; fi
	[[ ! -e "$target" && ! -L "$target" ]] || { log "WARNING: refusing to overwrite $target"; return 1; }
	case ":$PATH:" in
	*:"$HOME/.local/bin":*) ;;
	*) log "WARNING: add $HOME/.local/bin to PATH, then rerun --debugger-only"; return 1 ;;
	esac
	approve_optional "Create $target -> $interpreter for omp?" "creating $target -> $interpreter" || return 1
	mkdir -p "$HOME/.local/bin"
	ln -s "$interpreter" "$target" || return 1
	hash -r
	DEBUG_PYTHON=python
}

# Prints how pip may install into `python`: venv, site, user, or managed.
python_install_mode() {
	"$DEBUG_PYTHON" - <<'PY'
import os, sys, sysconfig
if sys.prefix != sys.base_prefix:
    print("venv")
elif os.path.exists(os.path.join(sysconfig.get_path("stdlib"), "EXTERNALLY-MANAGED")):
    print("managed")
elif os.access(sysconfig.get_path("purelib"), os.W_OK):
    print("site")
else:
    print("user")
PY
}

# Returns 0 when debugpy is ready (or dry-run), 1 when absent or failed.
ensure_debugger() {
	resolve_debug_python || return 1
	local interpreter mode
	interpreter="$(command -v "$DEBUG_PYTHON")"
	if debugpy_installed; then
		if [[ "$DEBUG_PYTHON" == python3 ]]; then
			log "debugpy is available, but omp calls the unversioned python command"
			mode="$(python_install_mode)" || mode=unknown
			if [[ "$mode" == managed ]]; then install_system_debugger; return; fi
			ensure_python_command || return 1
		fi
		debugpy_installed || return 1
		log "Python debugger ready: debugpy found for $(command -v python)"
		return 0
	fi
	log "omp's builtin Python debugger needs debugpy, which is missing for $interpreter"
	mode="$(python_install_mode)" || { log "WARNING: could not inspect Python environment"; return 1; }
	if [[ "$mode" == managed ]]; then install_system_debugger; return; fi
	local -a pip_args=(-m pip install --upgrade debugpy)
	if [[ "$mode" == user ]]; then pip_args=(-m pip install --user --upgrade debugpy); fi
	if [[ "$DRY_RUN" == true ]]; then
		log "Would run: $DEBUG_PYTHON ${pip_args[*]}"
		if [[ "$DEBUG_PYTHON" == python3 ]]; then log "Would link ~/.local/bin/python to $interpreter"; fi
		return 0
	fi
	if ! approve_optional "Install debugpy with $DEBUG_PYTHON ${pip_args[*]}?" "installing debugpy with $DEBUG_PYTHON ${pip_args[*]}"; then
		log "Skipped debugpy. Install it later with: $SCRIPT_NAME --debugger-only"
		return 1
	fi
	if ! "$DEBUG_PYTHON" -m pip --version >/dev/null 2>&1; then
		log "WARNING: pip is unavailable for $interpreter; install pip, then rerun $SCRIPT_NAME --debugger-only"
		return 1
	fi
	"$DEBUG_PYTHON" "${pip_args[@]}" || { log "WARNING: debugpy installation failed"; return 1; }
	ensure_python_command || return 1
	debugpy_installed || { log "WARNING: pip installed debugpy, but python cannot import it"; return 1; }
	log "Installed debugpy; omp can now debug Python programs"
}

# The debugger is optional for omp itself, so install/update only warn.
ensure_debugger_optional() {
	if [[ "$OMP_SKIP_DEBUGGER" == true ]]; then return; fi
	ensure_debugger || log "WARNING: omp is installed, but its Python debugger is not ready"
}

parse_args() {
	while (($# > 0)); do
		case "$1" in
		--install) ACTION=install ;;
		--update) ACTION=update ;;
		--uninstall) ACTION=uninstall ;;
		--check) ACTION=check ;;
		--debugger-only) ACTION=debugger ;;
		--skip-debugger) OMP_SKIP_DEBUGGER=true ;;
		--force) FORCE_INSTALL=true ;;
		--dry-run) DRY_RUN=true ;;
		--version | --dir)
			[[ $# -ge 2 && -n "$2" && "$2" != --* ]] || error_exit "$1 requires a value"
			if [[ "$1" == --version ]]; then OMP_VERSION="$2"; else PI_INSTALL_DIR="$2"; fi
			shift
			;;
		-h | --help) usage; exit 0 ;;
		*) error_exit "Unknown option: $1" ;;
		esac
		shift
	done
	case "$ACTION" in
	install | update | uninstall | check | debugger) ;;
	*) error_exit "Invalid action: $ACTION" ;;
	esac
	[[ "$PI_INSTALL_DIR" == /* && "$PI_INSTALL_DIR" != / ]] || error_exit "Installation directory must be an absolute path other than /"
	PI_INSTALL_DIR="${PI_INSTALL_DIR%/}"
	if [[ "$OMP_VERSION" != latest ]]; then
		OMP_VERSION="${OMP_VERSION#v}"
		[[ "$OMP_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || error_exit "Expected a release version, got: $OMP_VERSION"
	fi
}

installed_binary() {
	if [[ -x "$PI_INSTALL_DIR/omp" ]]; then
		printf '%s\n' "$PI_INSTALL_DIR/omp"
	else
		command -v omp 2>/dev/null
	fi
}

TEMP_DIR=""
cleanup_temp() {
	if [[ -n "$TEMP_DIR" ]]; then rm -rf -- "$TEMP_DIR"; fi
	TEMP_DIR=""
}

install_binary() {
	local binary
	binary="$(installed_binary || true)"
	if [[ "$ACTION" == install && "$FORCE_INSTALL" != true && -n "$binary" ]]; then
		log "Already installed at $binary; use --update or --force to reinstall"
		ensure_debugger_optional
		return
	fi
	# Never silently shadow or replace an install owned by a package manager.
	if [[ -n "$binary" && "$binary" != "$PI_INSTALL_DIR/omp" ]]; then
		error_exit "omp is installed at $binary; update/remove it with its package manager first"
	fi
	[[ ! -L "$PI_INSTALL_DIR/omp" && ! -d "$PI_INSTALL_DIR/omp" ]] || error_exit "Refusing to replace a symlink/directory at $PI_INSTALL_DIR/omp"
	case "$(uname -s)" in
	Darwin | Linux) ;;
	*) error_exit "Only macOS and Linux are supported" ;;
	esac
	local -a args=(--binary)
	if [[ "$OMP_VERSION" != latest ]]; then args+=(--ref "v$OMP_VERSION"); fi
	log "Installing omp $OMP_VERSION to $PI_INSTALL_DIR/omp using $INSTALL_URL ${args[*]}"
	if [[ "$DRY_RUN" == true ]]; then
		ensure_debugger_optional
		return
	fi
	command -v curl >/dev/null 2>&1 || error_exit "curl is required"
	confirm "Download and execute the official oh-my-pi installer?"
	# Download completely before execution; a failed/partial download must not run.
	TEMP_DIR="$(mktemp -d)"
	trap cleanup_temp EXIT
	curl -fsSL --connect-timeout 10 --max-time 120 "$INSTALL_URL" -o "$TEMP_DIR/install.sh"
	PI_INSTALL_DIR="$PI_INSTALL_DIR" sh "$TEMP_DIR/install.sh" "${args[@]}"
	"$PI_INSTALL_DIR/omp" --version || error_exit "Installed omp cannot start"
	cleanup_temp
	trap - EXIT
	log "Installed. Run omp to configure your provider and start a session."
	ensure_debugger_optional
}

uninstall_binary() {
	local target="$PI_INSTALL_DIR/omp"
	if [[ ! -e "$target" && ! -L "$target" ]]; then
		if command -v omp >/dev/null 2>&1; then
			error_exit "omp is installed elsewhere; use its package manager to remove it"
		fi
		log "omp is not installed at $target"
		return
	fi
	[[ ! -L "$target" && ! -d "$target" ]] || error_exit "Refusing to remove a symlink/directory at $target"
	log "Removing $target; configuration and data will be preserved"
	if [[ "$DRY_RUN" == true ]]; then return; fi
	confirm "Uninstall the standalone omp binary?"
	rm -f -- "$target"
}

main() {
	parse_args "$@"
	case "$ACTION" in
	install | update) install_binary ;;
	uninstall) uninstall_binary ;;
	debugger) ensure_debugger || error_exit "Python debugger is not ready" ;;
	check)
		local binary
		binary="$(installed_binary)" || error_exit "omp is not installed"
		"$binary" --version
		if command -v python >/dev/null 2>&1 && python -c 'import debugpy.adapter' >/dev/null 2>&1; then
			log "Python debugger: ready (debugpy)"
		else
			log "Python debugger: missing; run $SCRIPT_NAME --debugger-only"
		fi
		;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
