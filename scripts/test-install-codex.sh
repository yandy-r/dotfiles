#!/usr/bin/env bash
# Hermetic lifecycle tests: never download or modify the real user's Codex.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO_ROOT/bin/install-codex.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/test-install-codex.XXXXXX")"
trap 'rm -rf -- "$TEST_ROOT"' EXIT
export HOME="$TEST_ROOT/home"
# Keep host Codex and package managers unreachable, even when installed.
mkdir -p "$TEST_ROOT/tools"
for tool in /usr/bin/*; do
	case "${tool##*/}" in codex | npm | pnpm | bun | brew | rpm | dpkg | pacman | curl) continue ;; esac
	ln -s "$tool" "$TEST_ROOT/tools/${tool##*/}"
done
export PATH="$TEST_ROOT/bin:$TEST_ROOT/tools"
export TEST_LOG="$TEST_ROOT/log"
export CODEX_INSTALL_DIR="$HOME/.local/bin"
export CODEX_HOME="$HOME/.codex"
export NON_INTERACTIVE=true
unset ACTION CODEX_VERSION FORCE_INSTALL DRY_RUN NONINTERACTIVE BUN_INSTALL
mkdir -p "$CODEX_HOME" "$TEST_ROOT/bin" "$TEST_ROOT/tmp" "$TEST_ROOT/dotfiles"
export TMPDIR="$TEST_ROOT/tmp"
export NPM_STATE="$TEST_ROOT/npm-installed" PNPM_STATE="$TEST_ROOT/pnpm-installed"
printf 'model = "gpt-5"\n' >"$CODEX_HOME/config.toml"
printf '{"token":"secret"}\n' >"$CODEX_HOME/auth.json"
printf 'export EDITOR=nvim\n' >"$TEST_ROOT/dotfiles/zshrc"
ln -s "$TEST_ROOT/dotfiles/zshrc" "$HOME/.zshrc"
: >"$TEST_LOG"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
contains() { grep -Fq -- "$1" "$2" || fail "Missing $1 in $2"; }
expect_failure() {
	if "$@" >"$TEST_ROOT/error" 2>&1; then fail "Unexpected success: $*"; fi
}

cat >"$TEST_ROOT/bin/npm" <<'NPM'
#!/usr/bin/env bash
printf 'npm %s\n' "$*" >>"$TEST_LOG"
case "$*" in
"ls -g --depth=0 @openai/codex")
	[[ -e "$NPM_STATE" ]] || exit 1
	printf '/usr/local/lib\n`-- @openai/codex@0.1.0\n'
	;;
"uninstall -g @openai/codex")
	rm -f -- "$NPM_STATE"
	;;
*) exit 2 ;;
esac
NPM

cat >"$TEST_ROOT/bin/pnpm" <<'PNPM'
#!/usr/bin/env bash
printf 'pnpm %s\n' "$*" >>"$TEST_LOG"
case "$*" in
"ls -g --depth=0 --json @openai/codex")
	if [[ -e "$PNPM_STATE" ]]; then
		printf '[{"dependencies":{"@openai/codex":{"version":"0.1.0"}}}]\n'
	else
		printf '[{}]\n'
	fi
	;;
"remove -g @openai/codex")
	rm -f -- "$PNPM_STATE"
	;;
*) exit 2 ;;
esac
PNPM
chmod +x "$TEST_ROOT/bin/npm" "$TEST_ROOT/bin/pnpm"

# Simulate the upstream script, including --release, CODEX_NON_INTERACTIVE,
# CODEX_INSTALL_DAEMON_ONLY, release directory layout, and its shell profile
# PATH block.
cat >"$TEST_ROOT/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >>"$TEST_LOG"
while [[ $# -gt 0 ]]; do
	if [[ "$1" == -o ]]; then output="$2"; break; fi
	shift
done
if [[ "${FAIL_DOWNLOAD:-false}" == true ]]; then
	printf 'touch "$HOME/partial-script-executed"\n' >"$output"
	exit 22
fi
cat >"$output" <<'UPSTREAM'
set -eu
ver="0.163.0"
if [ "${1:-}" = "--release" ]; then ver="$2"; fi
if [ "${CODEX_INSTALL_DAEMON_ONLY:-0}" = 1 ]; then
	printf 'daemon env=%s args=%s\n' "${CODEX_NON_INTERACTIVE:-unset}" "$*" >>"$TEST_LOG"
	if [ "${FAIL_DAEMON:-false}" = true ]; then exit 1; fi
	rel="$CODEX_HOME/packages/app-server-daemon/releases/${ver}-x"
	mkdir -p "$rel/bin"
	printf '#!/bin/sh\nprintf "codex-cli %s\\n"\n' "$ver" >"$rel/bin/codex"
	chmod +x "$rel/bin/codex"
	ln -sfn "$rel" "$CODEX_HOME/packages/app-server-daemon/current"
	exit 0
fi
printf 'installer env=%s args=%s\n' "${CODEX_NON_INTERACTIVE:-unset}" "$*" >>"$TEST_LOG"
if [ "${FAIL_INSTALL:-false}" = true ]; then exit 1; fi
rel="$CODEX_HOME/packages/standalone/releases/${ver}-x"
mkdir -p "$rel/bin" "$CODEX_INSTALL_DIR"
cat >"$rel/bin/codex" <<EOF
#!/bin/sh
printf 'codex-cli %s\n' "$ver"
EOF
chmod +x "$rel/bin/codex"
ln -sfn "$rel" "$CODEX_HOME/packages/standalone/current"
ln -sfn "$CODEX_HOME/packages/standalone/current/bin/codex" "$CODEX_INSTALL_DIR/codex"
cat >>"$HOME/.zshrc" <<'BLOCK'
# >>> Codex installer >>>
export PATH="$HOME/.local/bin:$PATH"
# <<< Codex installer <<<
BLOCK
UPSTREAM
CURL
chmod +x "$TEST_ROOT/bin/curl"

bash "$INSTALLER" --help >"$TEST_ROOT/help"
contains --update "$TEST_ROOT/help"
contains --force "$TEST_ROOT/help"
contains --uninstall "$TEST_ROOT/help"
expect_failure bash "$INSTALLER" --version
expect_failure bash "$INSTALLER" --version --force
expect_failure bash "$INSTALLER" --version bad
expect_failure bash "$INSTALLER" --dir relative
expect_failure bash "$INSTALLER" --dir /
expect_failure bash "$INSTALLER" --unknown
expect_failure env ACTION=bad bash "$INSTALLER"
expect_failure bash "$INSTALLER" --check

touch "$NPM_STATE" "$PNPM_STATE"
bash "$INSTALLER" --dry-run 2>"$TEST_ROOT/dry"
[[ ! -e "$CODEX_INSTALL_DIR/codex" && -e "$NPM_STATE" && -e "$PNPM_STATE" ]] || fail 'dry-run changed state'
! grep -Fq 'curl ' "$TEST_LOG" || fail 'dry-run ran curl'
: >"$TEST_LOG"
expect_failure env NON_INTERACTIVE=false bash "$INSTALLER" </dev/null
! grep -Fq 'curl ' "$TEST_LOG" || fail 'unapproved install downloaded a script'
[[ -e "$NPM_STATE" && -e "$PNPM_STATE" ]] || fail 'unapproved install removed packages'

: >"$TEST_LOG"
bash "$INSTALLER" 2>"$TEST_ROOT/install"
[[ -x "$CODEX_HOME/packages/standalone/current/bin/codex" ]] || fail 'missing standalone marker binary'
[[ -L "$CODEX_INSTALL_DIR/codex" ]] || fail 'missing codex symlink'
[[ ! -e "$NPM_STATE" && ! -e "$PNPM_STATE" ]] || fail 'foreign installs were not removed'
contains 'npm uninstall -g @openai/codex' "$TEST_LOG"
contains 'pnpm remove -g @openai/codex' "$TEST_LOG"
contains 'installer env=true args=' "$TEST_LOG"
[[ -L "$HOME/.zshrc" ]] || fail '.zshrc symlink was replaced with a regular file'
! grep -Fq 'Codex installer' "$HOME/.zshrc" || fail 'profile block was not stripped'
contains 'export EDITOR=nvim' "$HOME/.zshrc"
# The daemon is installed in daemon-only mode, pinned to the CLI's version.
contains 'daemon env=true args=--release 0.163.0' "$TEST_LOG"
[[ -x "$CODEX_HOME/packages/app-server-daemon/current/bin/codex" ]] || fail 'missing daemon binary'

bash "$INSTALLER" --check >"$TEST_ROOT/check" 2>&1
contains 'codex-cli 0.163.0' "$TEST_ROOT/check"
contains 'App-server daemon: ' "$TEST_ROOT/check"

# --install repairs a missing daemon without reinstalling the CLI, and clears
# a dangling control-socket link.
rm -f -- "$CODEX_HOME/packages/app-server-daemon/current"
mkdir -p "$CODEX_HOME/app-server-control"
ln -s "$TEST_ROOT/gone/daemon.sock" "$CODEX_HOME/app-server-control/app-server-control.sock"
bash "$INSTALLER" --check >"$TEST_ROOT/check" 2>&1
contains 'App-server daemon: not found' "$TEST_ROOT/check"
contains 'Dangling daemon socket link' "$TEST_ROOT/check"
: >"$TEST_LOG"
bash "$INSTALLER" --dry-run
! grep -Fq 'curl ' "$TEST_LOG" || fail 'dry-run daemon repair downloaded script'
[[ -L "$CODEX_HOME/app-server-control/app-server-control.sock" ]] || fail 'dry-run removed socket link'
bash "$INSTALLER"
contains 'daemon env=true args=--release 0.163.0' "$TEST_LOG"
! grep -Fq 'installer ' "$TEST_LOG" || fail 'daemon repair reran the CLI installer'
[[ -x "$CODEX_HOME/packages/app-server-daemon/current/bin/codex" ]] || fail 'daemon repair did not install daemon'
[[ ! -L "$CODEX_HOME/app-server-control/app-server-control.sock" ]] || fail 'dangling socket link was kept'

# A daemon older than the CLI is updated to the CLI's version; a newer one is
# never downgraded.
: >"$TEST_LOG"
bash "$INSTALLER" --update --version 0.164.0
contains 'daemon env=true args=--release 0.164.0' "$TEST_LOG"
bash "$INSTALLER" --check >"$TEST_ROOT/check" 2>&1
! grep -Fq 'older than the CLI' "$TEST_ROOT/check" || fail 'check reported drift after update'
: >"$TEST_LOG"
bash "$INSTALLER" --update --version 0.163.0
! grep -Fq 'daemon env=' "$TEST_LOG" || fail 'newer daemon was downgraded'
bash "$INSTALLER" --check >"$TEST_ROOT/check" 2>&1
contains '(codex-cli 0.164.0)' "$TEST_ROOT/check"
# Simulate the CLI self-updating past the daemon.
ln -sfn "$CODEX_HOME/packages/standalone/releases/0.164.0-x" "$CODEX_HOME/packages/standalone/current"
ln -sfn "$CODEX_HOME/packages/app-server-daemon/releases/0.163.0-x" "$CODEX_HOME/packages/app-server-daemon/current"
bash "$INSTALLER" --check >"$TEST_ROOT/check" 2>&1
contains 'older than the CLI' "$TEST_ROOT/check"
: >"$TEST_LOG"
bash "$INSTALLER"
contains 'daemon env=true args=--release 0.164.0' "$TEST_LOG"
! grep -Fq 'installer ' "$TEST_LOG" || fail 'drift repair reran the CLI installer'

# A stable daemon is not "older" than the same version's prerelease CLI.
mkdir -p "$CODEX_HOME/packages/app-server-daemon/releases/0.165.0-x/bin"
printf '#!/bin/sh\necho "codex-cli 0.165.0"\n' >"$CODEX_HOME/packages/app-server-daemon/releases/0.165.0-x/bin/codex"
chmod +x "$CODEX_HOME/packages/app-server-daemon/releases/0.165.0-x/bin/codex"
ln -sfn "$CODEX_HOME/packages/app-server-daemon/releases/0.165.0-x" "$CODEX_HOME/packages/app-server-daemon/current"
: >"$TEST_LOG"
bash "$INSTALLER" --update --version 0.165.0-alpha.1
! grep -Fq 'daemon env=' "$TEST_LOG" || fail 'stable daemon was replaced by a prerelease'
bash "$INSTALLER" --update --version 0.164.0 >/dev/null 2>&1
ln -sfn "$CODEX_HOME/packages/app-server-daemon/releases/0.164.0-x" "$CODEX_HOME/packages/app-server-daemon/current"

# A release that predates the daemon rejects daemon-only mode: warn, succeed.
rm -f -- "$CODEX_HOME/packages/app-server-daemon/current"
FAIL_DAEMON=true bash "$INSTALLER" 2>"$TEST_ROOT/nodaemon"
contains 'app-server daemon was not installed' "$TEST_ROOT/nodaemon"
bash "$INSTALLER" 2>/dev/null
[[ -x "$CODEX_HOME/packages/app-server-daemon/current/bin/codex" ]] || fail 'daemon was not restored'

: >"$TEST_LOG"
bash "$INSTALLER"
! grep -Fq 'curl ' "$TEST_LOG" || fail 'second --install downloaded script'
! grep -Fq 'installer ' "$TEST_LOG" || fail 'second --install ran upstream installer'

# Re-running --install when already installed still migrates leftover npm installs.
touch "$NPM_STATE"
: >"$TEST_LOG"
bash "$INSTALLER"
[[ ! -e "$NPM_STATE" ]] || fail 'idempotent install did not migrate leftover npm package'
! grep -Fq 'curl ' "$TEST_LOG" || fail 'idempotent install downloaded script'

: >"$TEST_LOG"
bash "$INSTALLER" --update
contains 'curl ' "$TEST_LOG"
contains 'installer env=true args=' "$TEST_LOG"

: >"$TEST_LOG"
bash "$INSTALLER" --update --version v0.163.0-alpha.1
contains 'installer env=true args=--release 0.163.0-alpha.1' "$TEST_LOG"
: >"$TEST_LOG"
bash "$INSTALLER" --update --version 0.163.0
contains 'installer env=true args=--release 0.163.0' "$TEST_LOG"

: >"$TEST_LOG"
env NON_INTERACTIVE=false bash "$INSTALLER" --force
contains 'installer env=true args=' "$TEST_LOG"
[[ -x "$CODEX_HOME/packages/standalone/current/bin/codex" ]] || fail '--force did not leave codex installed'

printf 'custom-marker\n' >"$CODEX_HOME/packages/standalone/releases/0.163.0-x/marker"
expect_failure env FAIL_DOWNLOAD=true bash "$INSTALLER" --update
[[ ! -e "$HOME/partial-script-executed" ]] || fail 'partial download was executed'
[[ -e "$CODEX_HOME/packages/standalone/releases/0.163.0-x/marker" ]] || fail 'failed download lost existing release'

expect_failure env FAIL_INSTALL=true bash "$INSTALLER" --force
[[ -e "$CODEX_HOME/packages/standalone/releases/0.163.0-x/marker" ]] || fail 'failed --force did not restore backup'
[[ -x "$CODEX_HOME/packages/standalone/current/bin/codex" ]] || fail 'failed --force broke current binary'
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary script leaked on failure'

bash "$INSTALLER" --uninstall --dry-run
[[ -L "$CODEX_INSTALL_DIR/codex" ]] || fail 'dry-run removed symlink'
expect_failure env NON_INTERACTIVE=false bash "$INSTALLER" --uninstall </dev/null
bash "$INSTALLER" --uninstall
[[ ! -e "$CODEX_INSTALL_DIR/codex" && ! -L "$CODEX_INSTALL_DIR/codex" ]] || fail 'uninstall kept symlink'
[[ ! -e "$CODEX_HOME/packages/standalone" ]] || fail 'uninstall kept standalone dir'
contains 'model = "gpt-5"' "$CODEX_HOME/config.toml"
contains '"token":"secret"' "$CODEX_HOME/auth.json"
[[ ! -e "$CODEX_HOME/packages/app-server-daemon" ]] || fail 'uninstall kept daemon package'

# Uninstall also removes a daemon left behind without the CLI.
mkdir -p "$CODEX_HOME/packages/app-server-daemon/releases"
bash "$INSTALLER" --uninstall
[[ ! -e "$CODEX_HOME/packages/app-server-daemon" ]] || fail 'uninstall kept orphaned daemon package'

# Refuses to replace or remove a non-symlink regular file at $CODEX_INSTALL_DIR/codex.
mkdir -p "$CODEX_INSTALL_DIR"
printf '#!/bin/sh\necho user-owned\n' >"$CODEX_INSTALL_DIR/codex"
chmod +x "$CODEX_INSTALL_DIR/codex"
expect_failure bash "$INSTALLER"
expect_failure bash "$INSTALLER" --force
expect_failure bash "$INSTALLER" --uninstall
[[ -f "$CODEX_INSTALL_DIR/codex" && ! -L "$CODEX_INSTALL_DIR/codex" ]] || fail 'removed user-owned regular file'
rm -f -- "$CODEX_INSTALL_DIR/codex"

printf 'PASS: install-codex lifecycle tests\n'
