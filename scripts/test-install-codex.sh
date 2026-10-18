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
mkdir -p "$CODEX_HOME/packages/app-server-daemon" "$TEST_ROOT/bin" "$TEST_ROOT/tmp" "$TEST_ROOT/dotfiles"
export TMPDIR="$TEST_ROOT/tmp"
export NPM_STATE="$TEST_ROOT/npm-installed" PNPM_STATE="$TEST_ROOT/pnpm-installed"
printf 'model = "gpt-5"\n' >"$CODEX_HOME/config.toml"
printf '{"token":"secret"}\n' >"$CODEX_HOME/auth.json"
printf 'keep daemon\n' >"$CODEX_HOME/packages/app-server-daemon/marker"
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
# release directory layout, and its shell profile PATH block.
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
printf 'installer env=%s args=%s\n' "${CODEX_NON_INTERACTIVE:-unset}" "$*" >>"$TEST_LOG"
if [ "${FAIL_INSTALL:-false}" = true ]; then exit 1; fi
ver="0.163.0"
if [ "${1:-}" = "--release" ]; then ver="$2"; fi
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

bash "$INSTALLER" --check >"$TEST_ROOT/check" 2>&1
contains 'codex-cli 0.163.0' "$TEST_ROOT/check"

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
contains 'keep daemon' "$CODEX_HOME/packages/app-server-daemon/marker"

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
