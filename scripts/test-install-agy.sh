#!/usr/bin/env bash
# Hermetic lifecycle tests: never download or modify the real user's agy.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO_ROOT/bin/install-agy.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/test-install-agy.XXXXXX")"
trap 'rm -rf -- "$TEST_ROOT"' EXIT
export HOME="$TEST_ROOT/home"
# Only host tools via symlinks; the stubs in $TEST_ROOT/bin win.
mkdir -p "$TEST_ROOT/tools"
for tool in /usr/bin/* /bin/*; do
	[[ -e "$TEST_ROOT/tools/${tool##*/}" || -L "$TEST_ROOT/tools/${tool##*/}" ]] && continue
	case "${tool##*/}" in agy | curl | sleep) continue ;; esac
	ln -s "$tool" "$TEST_ROOT/tools/${tool##*/}"
done
export PATH="$TEST_ROOT/bin:$TEST_ROOT/tools"
export TEST_LOG="$TEST_ROOT/log"
export NON_INTERACTIVE=true
unset ACTION FORCE_INSTALL DRY_RUN NONINTERACTIVE AGY_INSTALL_DIR AGY_INSTALL_URL
readonly BIN="$HOME/.local/bin/agy"
mkdir -p "$HOME/.gemini/antigravity-cli" "$TEST_ROOT/bin" "$TEST_ROOT/tmp" "$TEST_ROOT/repo"
export TMPDIR="$TEST_ROOT/tmp"
printf 'keep settings\n' >"$HOME/.gemini/antigravity-cli/settings.json"
# ~/.bashrc is a symlink into the "dotfiles repo", as `dotf link` makes it.
printf 'managed bashrc\n' >"$TEST_ROOT/repo/bashrc"
ln -s "$TEST_ROOT/repo/bashrc" "$HOME/.bashrc"
printf 'plain profile\n' >"$HOME/.profile"
: >"$TEST_LOG"

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}
contains() { grep -Fq -- "$1" "$2" || fail "Missing $1 in $2"; }
expect_failure() {
	if "$@" >"$TEST_ROOT/error" 2>&1; then fail "Unexpected success: $*"; fi
}
shell_files_intact() {
	[[ -L "$HOME/.bashrc" ]] || fail 'HOME/.bashrc is no longer a symlink'
	[[ "$(cat "$TEST_ROOT/repo/bashrc")" == 'managed bashrc' ]] || fail 'repo bashrc was modified'
	[[ "$(cat "$HOME/.profile")" == 'plain profile' ]] || fail 'HOME/.profile was modified'
	[[ ! -e "$HOME/.zshrc" ]] || fail 'installer-created HOME/.zshrc was kept'
}

cat >"$TEST_ROOT/bin/sleep" <<'SLEEP'
#!/usr/bin/env bash
printf 'sleep %s\n' "$*" >>"$TEST_LOG"
SLEEP
# Simulate the official installer: no-op when present, then the `agy install`
# step editing shell profiles.
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
printf 'installer %s\n' "$*" >>"$TEST_LOG"
dir="$HOME/.local/bin"
if [ "${1:-}" = --dir ]; then dir="$2"; fi
if [ -f "$dir/agy" ]; then echo "already installed"; exit 0; fi
mkdir -p "$dir"
if [ "${BROKEN_INSTALL:-false}" = true ]; then
	printf '#!/bin/sh\nexit 1\n' >"$dir/agy"
else
	printf '#!/bin/sh\nprintf "agy %s\\n"\n' "${AGY_FAKE_VERSION:-1.5.16}" >"$dir/agy"
fi
chmod +x "$dir/agy"
printf 'export PATH="%s:$PATH"\n' "$dir" >>"$HOME/.bashrc"
printf 'export PATH="%s:$PATH"\n' "$dir" >>"$HOME/.profile"
printf 'export PATH="%s:$PATH"\n' "$dir" >>"$HOME/.zshrc"
if [ "${INTERRUPT_INSTALL:-false}" = true ]; then kill -TERM "$PPID"; exit 0; fi
if [ "${FAIL_INSTALL:-false}" = true ]; then exit 1; fi
UPSTREAM
CURL
chmod +x "$TEST_ROOT/bin/sleep" "$TEST_ROOT/bin/curl"

bash "$INSTALLER" --help >"$TEST_ROOT/help"
contains --force "$TEST_ROOT/help"
contains --uninstall "$TEST_ROOT/help"
contains 'https://antigravity.google/cli/install.sh' "$TEST_ROOT/help"
expect_failure bash "$INSTALLER" --dir
expect_failure bash "$INSTALLER" --dir relative
expect_failure bash "$INSTALLER" --dir /
expect_failure bash "$INSTALLER" --unknown
expect_failure env ACTION=bad bash "$INSTALLER"
expect_failure bash "$INSTALLER" --check

bash "$INSTALLER" --dry-run 2>/dev/null
[[ ! -e "$BIN" && ! -s "$TEST_LOG" ]] || fail 'dry-run changed state'
expect_failure env NON_INTERACTIVE=false bash "$INSTALLER" </dev/null
[[ ! -s "$TEST_LOG" ]] || fail 'unapproved install downloaded a script'

# A fresh install whose binary cannot start leaves nothing behind.
expect_failure env BROKEN_INSTALL=true bash "$INSTALLER"
[[ ! -e "$BIN" ]] || fail 'failed fresh install left a broken binary'
shell_files_intact
: >"$TEST_LOG"

# Fresh install: official installer only, shell profiles restored.
bash "$INSTALLER" 2>"$TEST_ROOT/install"
[[ -x "$BIN" ]] || fail 'install did not create agy'
contains 'curl -fsSL --connect-timeout 10 --max-time 120 https://antigravity.google/cli/install.sh' "$TEST_LOG"
grep -Fxq 'installer ' "$TEST_LOG" || fail 'default install must not pass --dir'
contains 'Restoring' "$TEST_ROOT/install"
shell_files_intact
bash "$INSTALLER" --check >"$TEST_ROOT/version" 2>&1
contains 'agy 1.5.16' "$TEST_ROOT/version"
cp "$TEST_LOG" "$TEST_ROOT/previous"
bash "$INSTALLER" 2>/dev/null
cmp "$TEST_LOG" "$TEST_ROOT/previous" || fail 'install was not idempotent'

# Update replaces the binary through the official installer.
: >"$TEST_LOG"
AGY_FAKE_VERSION=1.6.0 bash "$INSTALLER" --update 2>/dev/null
bash "$INSTALLER" --check 2>&1 | grep -Fq 'agy 1.6.0' || fail 'update did not install the new release'
shell_files_intact
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary files leaked'

# Failures never lose the working binary or run a partial download.
cp "$BIN" "$TEST_ROOT/original"
expect_failure env FAIL_DOWNLOAD=true bash "$INSTALLER" --update
[[ ! -e "$HOME/partial-script-executed" ]] || fail 'partial download was executed'
cmp "$BIN" "$TEST_ROOT/original" || fail 'failed download changed existing binary'
: >"$TEST_LOG"
expect_failure env FAIL_INSTALL=true AGY_FAKE_VERSION=9.9.9 bash "$INSTALLER" --update
[[ "$(grep -c '^installer' "$TEST_LOG")" == 3 ]] || fail 'installer was not retried 3 times'
cmp "$BIN" "$TEST_ROOT/original" || fail 'failed update did not restore the previous binary'
shell_files_intact
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary files leaked on failure'
# An interrupted installer still gets its shell edits undone and the old binary back.
expect_failure env INTERRUPT_INSTALL=true AGY_FAKE_VERSION=9.9.9 bash "$INSTALLER" --update
shell_files_intact
cmp "$BIN" "$TEST_ROOT/original" || fail 'interrupted update did not restore the previous binary'
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary files leaked on interrupt'
# A snapshot that cannot be taken stops before the installer ever runs.
: >"$TEST_LOG"
printf '#!/usr/bin/env bash\nexit 1\n' >"$TEST_ROOT/bin/cp"
chmod +x "$TEST_ROOT/bin/cp"
expect_failure bash "$INSTALLER" --update
rm "$TEST_ROOT/bin/cp"
contains 'Could not snapshot shell startup files' "$TEST_ROOT/error"
! grep -q '^installer' "$TEST_LOG" || fail 'installer ran without a shell snapshot'
shell_files_intact
cmp "$BIN" "$TEST_ROOT/original" || fail 'snapshot failure did not restore the previous binary'
# A new binary that installs but cannot start is replaced by the previous one.
expect_failure env BROKEN_INSTALL=true bash "$INSTALLER" --update
contains 'the previous agy was restored' "$TEST_ROOT/error"
cmp "$BIN" "$TEST_ROOT/original" || fail 'broken update did not restore the previous binary'
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary files leaked on broken update'

# A foreign agy (e.g. Homebrew cask) is refused, never shadowed.
mkdir -p "$TEST_ROOT/brew"
printf '#!/bin/sh\necho brew agy\n' >"$TEST_ROOT/brew/agy"
chmod +x "$TEST_ROOT/brew/agy"
expect_failure env PATH="$TEST_ROOT/brew:$PATH" bash "$INSTALLER" --update
contains 'only the official installer is supported' "$TEST_ROOT/error"
cmp "$BIN" "$TEST_ROOT/original" || fail 'foreign agy check changed the binary'

# Custom directory is passed through to the official installer.
: >"$TEST_LOG"
bash "$INSTALLER" --dir "$TEST_ROOT/custom" 2>/dev/null
[[ -x "$TEST_ROOT/custom/agy" ]] || fail 'custom --dir install missing'
contains "installer --dir $TEST_ROOT/custom" "$TEST_LOG"
bash "$INSTALLER" --dir "$TEST_ROOT/custom" --uninstall 2>/dev/null
shell_files_intact

# Uninstall removes only the binary.
bash "$INSTALLER" --uninstall --dry-run 2>/dev/null
[[ -x "$BIN" ]] || fail 'dry-run removed binary'
bash "$INSTALLER" --uninstall 2>/dev/null
[[ ! -e "$BIN" ]] || fail 'uninstall left agy behind'
contains 'keep settings' "$HOME/.gemini/antigravity-cli/settings.json"
bash "$INSTALLER" --uninstall 2>/dev/null
ln -s "$TEST_ROOT/original" "$BIN"
expect_failure bash "$INSTALLER" --uninstall
[[ -L "$BIN" ]] || fail 'uninstall removed a symlink'

printf 'PASS: install-agy lifecycle\n'
