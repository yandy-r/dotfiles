#!/usr/bin/env bash

# Install OpenCode 2 while preserving configuration used by OpenCode 1.
# Supports macOS, Linux (Ubuntu, Fedora, Arch, openSUSE, WSL), and best-effort
# installation on other Unix-like systems.

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[OPENCODE-INSTALL]"
readonly V2_INSTALL_URL="https://opencode.ai/v2/install"
readonly V2_BREW_FORMULA="anomalyco/tap/opencode-v2"
readonly V2_NPM_PACKAGE="@opencode/cli"
readonly V2_AUR_PACKAGE="opencode-beta"
# dotf's installed-state probe. A bare `opencode` on PATH cannot distinguish V1
# from V2, so `dotf apply` would report a V1 host as converged and never run this
# migration. This marker is a symlink to the verified V2 binary: if an external
# package-manager uninstall removes that binary, os.Stat sees a dangling link and
# dotf correctly reports OpenCode as absent.
OPENCODE_V2_MARKER="${OPENCODE_V2_MARKER:-$HOME/.local/state/dotf/opencode-v2}"

ACTION="${ACTION:-install}"
OPENCODE_VERSION="${OPENCODE_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"

usage() {
	cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall OpenCode 2.

OPTIONS:
    --install           Install OpenCode 2 (default)
    --uninstall         Uninstall OpenCode binaries but keep config and data
    --update            Update OpenCode 2
    --version VERSION   Install a specific V2 version (default: latest)
    --force             Reinstall even when OpenCode 2 is already installed
    -h, --help          Show this help message

ENVIRONMENT VARIABLES:
    ACTION              Action to perform (install, uninstall, update)
    OPENCODE_VERSION    Version to install (default: latest)
    FORCE_INSTALL       Force installation (true/false)
    NON_INTERACTIVE     Auto-approve prompts when true

OFFICIAL V2 METHODS (tried in order per platform):
    macOS:
      1. Homebrew: brew install anomalyco/tap/opencode-v2
      2. V2 installer: curl -fsSL https://opencode.ai/v2/install | bash
      3. Node package manager (see below)

    Arch Linux:
      1. AUR: paru -S opencode-beta
      2. V2 installer: curl -fsSL https://opencode.ai/v2/install | bash
      3. Node package manager (see below)

    Other Linux, WSL, and Unix-like systems:
      1. V2 installer: curl -fsSL https://opencode.ai/v2/install | bash
      2. npm: npm install -g --allow-scripts=@opencode/cli @opencode/cli@latest
      3. Bun: bun install -g --trust @opencode/cli@latest
      4. pnpm: pnpm add -g --allow-build=@opencode/cli @opencode/cli@latest

NPM POSTINSTALL:
    @opencode/cli installs a stub binary until its postinstall runs, and npm 12
    skips that script unless the package is allowlisted. The installer adds
    allow-scripts=@opencode/cli to the user npmrc (so later reinstalls, including
    \`opencode upgrade\`, keep working) and runs the skipped postinstall for any
    stub copy it finds (npm/pnpm/Bun globals and every mise Node version).

MIGRATION:
    OpenCode 1 package installations are removed before V2 is installed.
    Existing configuration and data are preserved for V2 migration.
EOF
	exit "${1:-0}"
}

log() {
	echo "${LOG_PREFIX} $*" >&2
}

error_exit() {
	log "ERROR: $1"
	exit "${2:-1}"
}

is_interactive() {
	[[ -t 0 && -t 1 ]]
}

prompt_confirmation() {
	local prompt_message="$1"
	local response

	if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "$FORCE_INSTALL" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]]; then
		log "Non-interactive mode: auto-approving prompt"
		return 0
	fi

	if ! is_interactive; then
		log "ERROR: Non-interactive shell detected; set NON_INTERACTIVE=true to approve"
		return 1
	fi

	printf '%s %s [y/N]: ' "$LOG_PREFIX" "$prompt_message" >&2
	read -r response
	case "${response:-n}" in
	[yY] | [yY][eE][sS]) return 0 ;;
	*) return 1 ;;
	esac
}

detect_os() {
	case "$(uname -s)" in
	Darwin) echo "macos" ;;
	Linux)
		if [[ ! -f /etc/os-release ]]; then
			echo "linux"
			return
		fi
		local os_id
		os_id="$(sed -n 's/^ID=//p' /etc/os-release | head -n1 | tr -d '"')"
		case "$os_id" in
		ubuntu) echo "ubuntu" ;;
		fedora) echo "fedora" ;;
		arch | manjaro | endeavouros) echo "arch" ;;
		opensuse* | sles) echo "opensuse" ;;
		*) echo "linux" ;;
		esac
		;;
	*) echo "unix" ;;
	esac
}

is_wsl() {
	[[ -n "${WSL_DISTRO_NAME:-}" ]] || grep -qi microsoft /proc/version 2>/dev/null
}

opencode_version() {
	command -v opencode >/dev/null 2>&1 || return 1
	opencode --version 2>/dev/null || return 1
}

version_major() {
	local version="$1"
	if [[ "$version" =~ ([0-9]+)\. ]]; then
		printf '%s\n' "${BASH_REMATCH[1]}"
		return 0
	fi
	return 1
}

binary_generation() {
	local binary="$1" version major
	version="$("$binary" --version 2>/dev/null || true)"
	major="$(version_major "$version" || true)"
	if [[ -z "$major" ]]; then
		echo "unknown"
	elif ((major >= 2)); then
		echo "v2"
	else
		echo "v1"
	fi
}

# Every `opencode` on PATH, so a V1 binary shadowed by a V2 install is still
# found and removed.
opencode_binaries() {
	type -a -P opencode 2>/dev/null || true
}

first_v1_binary() {
	local binary
	while IFS= read -r binary; do
		[[ -n "$binary" ]] || continue
		if [[ "$(binary_generation "$binary")" == "v1" ]]; then
			printf '%s\n' "$binary"
			return 0
		fi
	done < <(opencode_binaries)
	return 1
}

opencode_generation() {
	if ! command -v opencode >/dev/null 2>&1; then
		echo "absent"
		return
	fi

	local version major
	version="$(opencode_version || true)"
	major="$(version_major "$version" || true)"
	if [[ -z "$major" ]]; then
		echo "unknown"
	elif ((major >= 2)); then
		echo "v2"
	else
		echo "v1"
	fi
}

npm_package_installed() {
	local package="$1"
	command -v npm >/dev/null 2>&1 && npm list -g --depth=0 "$package" >/dev/null 2>&1
}

# pnpm stores global packages under a hashed virtual-store directory, so probe
# its own listing rather than guessing a node_modules path.
pnpm_package_installed() {
	local package="$1"
	command -v pnpm >/dev/null 2>&1 || return 1
	pnpm ls -g --depth=0 --json "$package" 2>/dev/null | command grep -Fq "\"$package\""
}

bun_package_installed() {
	local package="$1"
	command -v bun >/dev/null 2>&1 || return 1
	[[ -f "${BUN_INSTALL:-$HOME/.bun}/install/global/node_modules/$package/package.json" ]]
}

yarn_package_installed() {
	local package="$1" root
	command -v yarn >/dev/null 2>&1 || return 1
	root="$(yarn global dir 2>/dev/null)" || return 1
	[[ -f "$root/node_modules/$package/package.json" ]]
}

brew_formula_installed() {
	local formula="$1"
	command -v brew >/dev/null 2>&1 && brew list --formula "$formula" >/dev/null 2>&1
}

pacman_package_installed() {
	local package="$1"
	command -v pacman >/dev/null 2>&1 && pacman -Q "$package" >/dev/null 2>&1
}

legacy_v1_installed() {
	first_v1_binary >/dev/null || \
		brew_formula_installed "opencode" || \
		npm_package_installed "opencode-ai" || \
		pnpm_package_installed "opencode-ai" || \
		bun_package_installed "opencode-ai" || \
		yarn_package_installed "opencode-ai" || \
		pacman_package_installed "opencode-bin"
}

remove_with_privilege() {
	local path="$1"

	[[ -n "$path" && "$path" != "/" ]] || error_exit "Refusing to remove unsafe path: $path"
	[[ ! -d "$path" || -L "$path" ]] || error_exit "Refusing to remove directory: $path"

	if [[ -w "$path" || ( ! -e "$path" && -w "$(dirname "$path")" ) ]]; then
		rm -f -- "$path"
		return
	fi

	log "Elevated privileges required to remove legacy binary: $path"
	if ! prompt_confirmation "Remove $path with sudo?"; then
		return 1
	fi
	sudo rm -f -- "$path"
}

remove_v1_packages() {
	if brew_formula_installed "opencode"; then
		log "Removing OpenCode 1 Homebrew formula; configuration remains untouched..."
		brew uninstall opencode
	fi

	if npm_package_installed "opencode-ai"; then
		log "Removing OpenCode 1 npm package; configuration remains untouched..."
		npm uninstall -g opencode-ai
	fi
	if pnpm_package_installed "opencode-ai"; then
		log "Removing OpenCode 1 pnpm package; configuration remains untouched..."
		pnpm remove -g opencode-ai
	fi
	if bun_package_installed "opencode-ai"; then
		log "Removing OpenCode 1 Bun package; configuration remains untouched..."
		bun remove -g opencode-ai
	fi
	if yarn_package_installed "opencode-ai"; then
		log "Removing OpenCode 1 Yarn package; configuration remains untouched..."
		yarn global remove opencode-ai
	fi

	if pacman_package_installed "opencode-bin"; then
		log "Removing OpenCode 1 AUR package; configuration remains untouched..."
		if command -v paru >/dev/null 2>&1; then
			paru -Rns --noconfirm opencode-bin
		else
			prompt_confirmation "Remove opencode-bin with sudo pacman?" || return 1
			sudo pacman -Rns --noconfirm opencode-bin
		fi
	fi
}

remove_remaining_v1_binaries() {
	local binary
	local attempts=0

	hash -r
	while binary="$(first_v1_binary)"; do
		if ((attempts >= 10)); then
			error_exit "Too many legacy OpenCode binaries remain on PATH"
		fi
		log "Removing legacy OpenCode 1 binary: $binary"
		remove_with_privilege "$binary"
		attempts=$((attempts + 1))
		hash -r
	done
}

migrate_from_v1_if_needed() {
	legacy_v1_installed || return 0

	log "OpenCode 1 detected. Removing V1 installation before installing V2..."
	log "Preserving OpenCode configuration and data for V2 migration."
	remove_v1_packages
	remove_remaining_v1_binaries

	if legacy_v1_installed; then
		error_exit "OpenCode 1 removal did not complete"
	fi
	log "OpenCode 1 removed; settings and data preserved."
}

install_opencode_homebrew() {
	command -v brew >/dev/null 2>&1 || return 1
	[[ "$OPENCODE_VERSION" == "latest" ]] || return 1

	# The documented V2 formula is not published in every tap state; skip to the
	# next official method instead of failing the run when it is missing.
	if ! brew info --formula "$V2_BREW_FORMULA" >/dev/null 2>&1; then
		log "Homebrew formula $V2_BREW_FORMULA unavailable; trying next official method."
		return 1
	fi

	log "Installing OpenCode 2 via official Homebrew tap..."
	brew install "$V2_BREW_FORMULA"
}

install_opencode_aur() {
	command -v paru >/dev/null 2>&1 || return 1
	[[ "$OPENCODE_VERSION" == "latest" ]] || return 1

	log "Installing OpenCode 2 via official AUR package..."
	paru -S --needed --noconfirm "$V2_AUR_PACKAGE"
}

install_opencode_official_script() {
	command -v curl >/dev/null 2>&1 || return 1
	command -v bash >/dev/null 2>&1 || return 1

	log "Installing OpenCode 2 via official V2 installer..."
	if [[ "$FORCE_INSTALL" != "true" ]]; then
		log "Security notice: this downloads and executes $V2_INSTALL_URL"
		if ! prompt_confirmation "Run the official OpenCode 2 installer?"; then
			return 1
		fi
	fi

	local -a installer_args=(--no-modify-path)
	if [[ "$OPENCODE_VERSION" != "latest" ]]; then
		installer_args+=(--version "$OPENCODE_VERSION")
	fi

	curl -fsSL "$V2_INSTALL_URL" | bash -s -- "${installer_args[@]}"
}

# @opencode/cli selects its platform binary in a postinstall script. npm 11.16+
# gates dependency install scripts behind an allowlist (and npm 12 blocks them by
# default), so a plain `npm install -g` yields a CLI with no binary.
npm_supports_allow_scripts() {
	npm install --help 2>/dev/null | command grep -q -- '--allow-scripts'
}

# Persist the allowlist in the user npmrc. A per-command flag only covers this
# run: OpenCode's own `opencode upgrade`, a later manual `npm i -g`, or a global
# install under another mise-managed Node version would otherwise reinstall
# @opencode/cli with its postinstall skipped and leave the stub binary behind.
ensure_npm_allow_scripts() {
	command -v npm >/dev/null 2>&1 || return 0
	npm_supports_allow_scripts || return 0

	local current
	current="$(npm config get allow-scripts --location=user 2>/dev/null || true)"
	case ",${current// /}," in
	*",${V2_NPM_PACKAGE},"*) return 0 ;;
	esac
	if [[ -z "$current" || "$current" == "null" || "$current" == "undefined" ]]; then
		current="$V2_NPM_PACKAGE"
	else
		current="${current},${V2_NPM_PACKAGE}"
	fi

	log "Allowing $V2_NPM_PACKAGE install scripts in the user npmrc (allow-scripts=$current)..."
	npm config set --location=user allow-scripts "$current" ||
		log "WARNING: could not persist allow-scripts; later npm reinstalls may skip the postinstall"
}

# Every @opencode/cli copy a Node package manager has installed: the active npm
# global root, each mise-managed Node version (each keeps its own globals), and
# the pnpm/Bun global stores.
node_package_dirs() {
	local root dir
	{
		if command -v npm >/dev/null 2>&1; then
			root="$(npm root -g 2>/dev/null || true)"
			[[ -n "$root" ]] && printf '%s\n' "$root/$V2_NPM_PACKAGE"
		fi
		for dir in "${MISE_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/mise}"/installs/node/*/lib/node_modules/"$V2_NPM_PACKAGE"; do
			printf '%s\n' "$dir"
		done
		if command -v pnpm >/dev/null 2>&1; then
			root="$(pnpm root -g 2>/dev/null || true)"
			[[ -n "$root" ]] && printf '%s\n' "$root/$V2_NPM_PACKAGE"
		fi
		printf '%s\n' "${BUN_INSTALL:-$HOME/.bun}/install/global/node_modules/$V2_NPM_PACKAGE"
	} | while IFS= read -r dir; do
		[[ -f "$dir/package.json" ]] || continue
		# Resolve symlinked version aliases (mise's 26 -> 26.12.0) to one entry.
		(cd -P -- "$dir" 2>/dev/null && pwd)
	done | sort -u
}

# The package ships a tiny shell placeholder as its binary until the postinstall
# swaps in the real platform build; the placeholder only prints an error.
is_postinstall_stub() {
	local binary="$1"
	[[ -f "$binary" ]] || return 1
	(($(wc -c <"$binary") < 4096)) || return 1
	command grep -Fqs "postinstall script was not run" "$binary"
}

# Run the skipped postinstall in place for any stub copy, so a shadowing stub on
# PATH cannot break `opencode` even when it was installed outside this script.
repair_postinstall_stubs() {
	local dir binary
	command -v node >/dev/null 2>&1 || return 0
	while IFS= read -r dir; do
		[[ -n "$dir" ]] || continue
		binary="$dir/bin/opencode.exe"
		is_postinstall_stub "$binary" || continue
		if [[ ! -f "$dir/postinstall.mjs" ]]; then
			log "WARNING: $dir has a stub binary but no postinstall.mjs; reinstall $V2_NPM_PACKAGE"
			continue
		fi
		log "Running skipped $V2_NPM_PACKAGE postinstall in $dir..."
		if ! (cd -- "$dir" && node ./postinstall.mjs); then
			log "WARNING: postinstall failed in $dir"
			continue
		fi
		if is_postinstall_stub "$binary"; then
			log "WARNING: postinstall ran but $binary is still the stub"
		fi
	done < <(node_package_dirs)
	return 0
}

install_opencode_npm() {
	command -v node >/dev/null 2>&1 || return 1
	command -v npm >/dev/null 2>&1 || return 1

	local package="${V2_NPM_PACKAGE}@${OPENCODE_VERSION}"

	log "Installing OpenCode 2 via official npm package..."
	ensure_npm_allow_scripts
	if npm_supports_allow_scripts; then
		npm install -g --allow-scripts="$V2_NPM_PACKAGE" "$package" || return 1
	else
		npm install -g "$package" || return 1
	fi
	repair_postinstall_stubs
}

# pnpm (v10+) and Bun block dependency build scripts by default, so both need an
# explicit trust flag for the same postinstall binary selection.
install_opencode_pnpm() {
	command -v pnpm >/dev/null 2>&1 || return 1

	local package="${V2_NPM_PACKAGE}@${OPENCODE_VERSION}"

	log "Installing OpenCode 2 via official pnpm package..."
	pnpm add -g --allow-build="$V2_NPM_PACKAGE" "$package" || return 1
	repair_postinstall_stubs
}

install_opencode_bun() {
	command -v bun >/dev/null 2>&1 || return 1

	local package="${V2_NPM_PACKAGE}@${OPENCODE_VERSION}"

	log "Installing OpenCode 2 via official Bun package..."
	bun install -g --trust "$package" || return 1
	repair_postinstall_stubs
}

install_opencode_yarn() {
	command -v yarn >/dev/null 2>&1 || return 1

	local package="${V2_NPM_PACKAGE}@${OPENCODE_VERSION}"

	log "Installing OpenCode 2 via official Yarn package..."
	yarn global add "$package" || return 1
	repair_postinstall_stubs
}

# Node-toolchain fallback chain, in the order the official docs list them.
install_opencode_node_toolchain() {
	install_opencode_npm || install_opencode_bun || install_opencode_pnpm || install_opencode_yarn
}

install_for_platform() {
	local os_type="$1"

	# Exact versions use the official installer because rolling package managers
	# cannot reliably satisfy a requested release.
	if [[ "$OPENCODE_VERSION" != "latest" ]]; then
		install_opencode_official_script || install_opencode_node_toolchain || \
			error_exit "No official OpenCode 2 method could install version $OPENCODE_VERSION"
		return
	fi

	case "$os_type" in
	macos)
		install_opencode_homebrew || install_opencode_official_script || install_opencode_node_toolchain || \
			error_exit "All official OpenCode 2 installation methods failed for macOS"
		;;
	arch)
		install_opencode_aur || install_opencode_official_script || install_opencode_node_toolchain || \
			error_exit "All official OpenCode 2 installation methods failed for Arch Linux"
		;;
	ubuntu | fedora | opensuse | linux)
		install_opencode_official_script || install_opencode_node_toolchain || \
			error_exit "All official OpenCode 2 installation methods failed for Linux"
		;;
	unix)
		install_opencode_official_script || install_opencode_node_toolchain || \
			error_exit "OpenCode 2 has no working official method for this Unix platform"
		;;
	esac
}

refresh_opencode_path() {
	if [[ -x "$HOME/.opencode/bin/opencode" ]]; then
		export PATH="$HOME/.opencode/bin:$PATH"
	fi
	hash -r
}

verify_installation() {
	refresh_opencode_path
	# Whatever method ran, a stub copy elsewhere (another Node version, an
	# `opencode upgrade` via npm) can still shadow the real binary on PATH.
	ensure_npm_allow_scripts
	repair_postinstall_stubs
	hash -r
	if ! command -v opencode >/dev/null 2>&1; then
		log "ERROR: opencode not found in PATH"
		log "Expected user-install path: $HOME/.opencode/bin"
		return 1
	fi

	local version generation
	version="$(opencode_version || true)"
	generation="$(opencode_generation)"
	if [[ "$generation" != "v2" ]]; then
		log "ERROR: expected OpenCode 2, found '${version:-unknown}' at $(command -v opencode)"
		return 1
	fi

	mkdir -p "$(dirname "$OPENCODE_V2_MARKER")"
	ln -sfn "$(command -v opencode)" "$OPENCODE_V2_MARKER"
	log "OpenCode 2 verified: $version"
	log "Location: $(command -v opencode)"
}

v2_installation_method() {
	# Prefer the active curl-installed binary over hidden package-manager copies.
	# Otherwise an update could change a shadowed package while leaving the
	# command the user actually runs untouched.
	if [[ "$(opencode_generation)" == "v2" && "$(command -v opencode)" == "$HOME/.opencode/bin/opencode" ]]; then
		echo "standalone"
	elif brew_formula_installed "opencode-v2"; then
		echo "homebrew"
	elif pacman_package_installed "$V2_AUR_PACKAGE"; then
		echo "aur"
	elif npm_package_installed "$V2_NPM_PACKAGE"; then
		echo "npm"
	elif pnpm_package_installed "$V2_NPM_PACKAGE"; then
		echo "pnpm"
	elif bun_package_installed "$V2_NPM_PACKAGE"; then
		echo "bun"
	elif yarn_package_installed "$V2_NPM_PACKAGE"; then
		echo "yarn"
	elif [[ "$(opencode_generation)" == "v2" ]]; then
		echo "standalone"
	else
		echo "absent"
	fi
}

update_opencode() {
	migrate_from_v1_if_needed

	local method
	method="$(v2_installation_method)"
	if [[ "$method" == "absent" ]]; then
		log "OpenCode 2 is not installed; installing it now."
		install_for_platform "$(detect_os)"
		verify_installation
		return
	fi

	log "Updating OpenCode 2 (detected method: $method)..."
	if [[ "$OPENCODE_VERSION" != "latest" ]]; then
		install_opencode_official_script || install_opencode_node_toolchain || \
			error_exit "Could not install OpenCode 2 version $OPENCODE_VERSION"
	else
		case "$method" in
		homebrew) brew upgrade "$V2_BREW_FORMULA" ;;
		aur) paru -S --needed --noconfirm "$V2_AUR_PACKAGE" ;;
		npm) install_opencode_npm ;;
		pnpm) install_opencode_pnpm ;;
		bun) install_opencode_bun ;;
		yarn) install_opencode_yarn ;;
		standalone) install_opencode_official_script ;;
		esac
	fi
	verify_installation
}

uninstall_opencode() {
	log "Uninstalling OpenCode binaries while preserving configuration and data..."

	remove_v1_packages
	if brew_formula_installed "opencode-v2"; then
		brew uninstall "$V2_BREW_FORMULA"
	fi
	if npm_package_installed "$V2_NPM_PACKAGE"; then
		npm uninstall -g "$V2_NPM_PACKAGE"
	fi
	if pnpm_package_installed "$V2_NPM_PACKAGE"; then
		pnpm remove -g "$V2_NPM_PACKAGE"
	fi
	if bun_package_installed "$V2_NPM_PACKAGE"; then
		bun remove -g "$V2_NPM_PACKAGE"
	fi
	if yarn_package_installed "$V2_NPM_PACKAGE"; then
		yarn global remove "$V2_NPM_PACKAGE"
	fi
	if pacman_package_installed "$V2_AUR_PACKAGE"; then
		if command -v paru >/dev/null 2>&1; then
			paru -Rns --noconfirm "$V2_AUR_PACKAGE"
		else
			prompt_confirmation "Remove $V2_AUR_PACKAGE with sudo pacman?" || return 1
			sudo pacman -Rns --noconfirm "$V2_AUR_PACKAGE"
		fi
	fi

	hash -r
	local binary attempts=0
	while command -v opencode >/dev/null 2>&1; do
		if ((attempts >= 10)); then
			error_exit "Too many OpenCode binaries remain on PATH"
		fi
		binary="$(command -v opencode)"
		log "Removing remaining OpenCode binary: $binary"
		remove_with_privilege "$binary"
		attempts=$((attempts + 1))
		hash -r
	done

	# V2's curl installer creates this compatibility shim beside its binary.
	rm -f -- "$HOME/.opencode/bin/opencode2" "$HOME/.opencode/bin/opencode2.cmd"
	rm -f -- "$OPENCODE_V2_MARKER"
	log "OpenCode binaries removed. Configuration and data were not changed."
}

install_opencode_main() {
	migrate_from_v1_if_needed

	if [[ "$(opencode_generation)" == "v2" && "$FORCE_INSTALL" != "true" ]]; then
		update_opencode
		return
	fi

	local os_type
	os_type="$(detect_os)"
	log "Installing OpenCode 2 on $os_type..."
	if is_wsl; then
		log "WSL detected; using Linux installation method."
	fi
	install_for_platform "$os_type"
	verify_installation
}

parse_args() {
	while (($# > 0)); do
		case "$1" in
		--install) ACTION="install"; shift ;;
		--uninstall) ACTION="uninstall"; shift ;;
		--update) ACTION="update"; shift ;;
		--version)
			(($# >= 2)) || error_exit "--version requires an argument"
			OPENCODE_VERSION="$2"
			shift 2
			;;
		--force) FORCE_INSTALL="true"; shift ;;
		-h | --help) usage 0 ;;
		*) error_exit "Unknown option: $1. Use --help for usage information." ;;
		esac
	done
}

main() {
	parse_args "$@"
	case "$ACTION" in
	install) install_opencode_main ;;
	uninstall) uninstall_opencode ;;
	update) update_opencode ;;
	*) error_exit "Invalid action: $ACTION. Use --help for usage information." ;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	main "$@"
fi
