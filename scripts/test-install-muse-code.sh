#!/usr/bin/env bash
# Hermetic lifecycle tests: never download or modify the real user's muse.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO_ROOT/bin/install-muse-code.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/test-install-muse-code.XXXXXX")"
trap 'rm -rf -- "$TEST_ROOT"' EXIT
export HOME="$TEST_ROOT/home"
# Only host tools via symlinks; the stubs in $TEST_ROOT/bin win.
mkdir -p "$TEST_ROOT/tools"
for tool in /usr/bin/* /bin/*; do
	[[ -e "$TEST_ROOT/tools/${tool##*/}" || -L "$TEST_ROOT/tools/${tool##*/}" ]] && continue
	case "${tool##*/}" in muse | curl | sleep) continue ;; esac
	ln -s "$tool" "$TEST_ROOT/tools/${tool##*/}"
done
export PATH="$TEST_ROOT/bin:$TEST_ROOT/tools"
export TEST_LOG="$TEST_ROOT/log"
export NON_INTERACTIVE=true
unset ACTION FORCE_INSTALL DRY_RUN NONINTERACTIVE MUSE_CODE_INSTALL_DIR MUSE_INSTALLER_URL MUSE_NO_MODIFY_PATH MUSE_INSTALL_DIR
readonly DIR="$HOME/.local/bin"
readonly BIN="$DIR/muse"
mkdir -p "$HOME/.config/muse" "$TEST_ROOT/bin" "$TEST_ROOT/tmp"
export TMPDIR="$TEST_ROOT/tmp"
printf 'keep auth\n' >"$HOME/.config/muse/auth.json"
printf 'plain bashrc\n' >"$HOME/.bashrc"
: >"$TEST_LOG"

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}
contains() { grep -Fq -- "$1" "$2" || fail "Missing $1 in $2"; }
expect_failure() {
	if "$@" >"$TEST_ROOT/error" 2>&1; then fail "Unexpected success: $*"; fi
}

cat >"$TEST_ROOT/bin/sleep" <<'SLEEP'
#!/usr/bin/env bash
printf 'sleep %s\n' "$*" >>"$TEST_LOG"
SLEEP
# Simulate the official installer: a launcher plus a versioned binary next to
# it; it edits ~/.bashrc unless MUSE_NO_MODIFY_PATH is set.
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
set -euo pipefail
printf 'installer no_modify=%s dir=%s\n' "${MUSE_NO_MODIFY_PATH:-}" "${MUSE_INSTALL_DIR:-}" >>"$TEST_LOG"
if [[ "${FAIL_INSTALL:-false}" == true ]]; then exit 1; fi
dir="${MUSE_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$dir"
version="${MUSE_FAKE_VERSION:-1.4.25-R9361.1}"
if [[ "${BROKEN_INSTALL:-false}" == true ]]; then
	printf '#!/bin/sh\nexit 1\n' >"$dir/muse"
else
	printf '#!/bin/sh\nexec "$(dirname "$0")/muse-bin-$(cat "$(dirname "$0")/.muse-version")" "$@"\n' >"$dir/muse"
	printf '#!/bin/sh\necho "muse %s"\n' "$version" >"$dir/muse-bin-$version"
	chmod +x "$dir/muse-bin-$version"
	printf '%s\n' "$version" >"$dir/.muse-version"
	: >"$dir/.muse-update-checked-at"
	mkdir -p "$dir/.muse-update-lock"
fi
chmod +x "$dir/muse"
[[ -n "${MUSE_NO_MODIFY_PATH:-}" ]] || printf 'export PATH="%s:$PATH"\n' "$dir" >>"$HOME/.bashrc"
UPSTREAM
CURL
chmod +x "$TEST_ROOT/bin/sleep" "$TEST_ROOT/bin/curl"

bash "$INSTALLER" --help >"$TEST_ROOT/help"
contains --force "$TEST_ROOT/help"
contains --uninstall "$TEST_ROOT/help"
contains 'https://dev.meta.ai/install.sh' "$TEST_ROOT/help"
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

# A partial download is never executed.
expect_failure env FAIL_DOWNLOAD=true bash "$INSTALLER"
[[ ! -e "$HOME/partial-script-executed" && ! -e "$BIN" ]] || fail 'partial download was executed'
# A fresh install whose launcher cannot start leaves nothing behind.
expect_failure env BROKEN_INSTALL=true bash "$INSTALLER"
[[ ! -e "$BIN" ]] || fail 'failed fresh install left a broken launcher'
: >"$TEST_LOG"

# Fresh install: official installer only, never edits shell profiles.
bash "$INSTALLER" 2>"$TEST_ROOT/install"
[[ -x "$BIN" ]] || fail 'install did not create muse'
contains "--proto =https --tlsv1.2 --connect-timeout 10 --max-time 120 https://dev.meta.ai/install.sh" "$TEST_LOG"
contains "installer no_modify=1 dir=$DIR" "$TEST_LOG"
[[ "$(cat "$HOME/.bashrc")" == 'plain bashrc' ]] || fail 'installer edited ~/.bashrc'
contains 'muse login' "$TEST_ROOT/install"
bash "$INSTALLER" --check >"$TEST_ROOT/version" 2>&1
contains 'muse 1.4.25-R9361.1' "$TEST_ROOT/version"
cp "$TEST_LOG" "$TEST_ROOT/previous"
bash "$INSTALLER" 2>/dev/null
cmp "$TEST_LOG" "$TEST_ROOT/previous" || fail 'install was not idempotent'

# Update re-runs the official installer.
MUSE_FAKE_VERSION=1.5.0-R1 bash "$INSTALLER" --update 2>/dev/null
bash "$INSTALLER" --check 2>&1 | grep -Fq 'muse 1.5.0-R1' || fail 'update did not install the new release'
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary files leaked'

# A failed update retries 3 times and keeps the working install.
: >"$TEST_LOG"
expect_failure env FAIL_INSTALL=true bash "$INSTALLER" --update
[[ "$(grep -c '^installer' "$TEST_LOG")" == 3 ]] || fail 'installer was not retried 3 times'
bash "$INSTALLER" --check 2>&1 | grep -Fq 'muse 1.5.0-R1' || fail 'failed update broke the install'
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary files leaked on failure'

# A foreign muse (e.g. Homebrew cask, AUR) is refused, never shadowed.
mkdir -p "$TEST_ROOT/brew"
printf '#!/bin/sh\necho brew muse\n' >"$TEST_ROOT/brew/muse"
chmod +x "$TEST_ROOT/brew/muse"
expect_failure env PATH="$TEST_ROOT/brew:$PATH" bash "$INSTALLER" --update
contains 'only the official installer is supported' "$TEST_ROOT/error"

# Custom directory is passed through to the official installer.
: >"$TEST_LOG"
bash "$INSTALLER" --dir "$TEST_ROOT/custom" 2>/dev/null
[[ -x "$TEST_ROOT/custom/muse" ]] || fail 'custom --dir install missing'
contains "installer no_modify=1 dir=$TEST_ROOT/custom" "$TEST_LOG"
bash "$INSTALLER" --dir "$TEST_ROOT/custom" --uninstall 2>/dev/null
[[ -z "$(ls -A "$TEST_ROOT/custom")" ]] || fail 'custom uninstall left files behind'

# Uninstall removes the launcher and its state, never config/auth or others.
printf 'other\n' >"$DIR/other-tool"
bash "$INSTALLER" --uninstall --dry-run 2>/dev/null
[[ -x "$BIN" ]] || fail 'dry-run removed muse'
bash "$INSTALLER" --uninstall 2>/dev/null
[[ ! -e "$BIN" ]] || fail 'uninstall left muse behind'
leftover="$(find "$DIR" -name 'muse-bin-*' -o -name '.muse-*' | head -n 1)"
[[ -z "$leftover" ]] || fail "uninstall left $leftover behind"
[[ -f "$DIR/other-tool" ]] || fail 'uninstall removed an unrelated file'
contains 'keep auth' "$HOME/.config/muse/auth.json"
bash "$INSTALLER" --uninstall 2>/dev/null
ln -s "$DIR/other-tool" "$BIN"
expect_failure bash "$INSTALLER" --uninstall
[[ -L "$BIN" ]] || fail 'uninstall removed a symlink'

printf 'PASS: install-muse-code lifecycle\n'
