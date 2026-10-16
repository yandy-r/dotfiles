#!/usr/bin/env bash
# Hermetic lifecycle tests: never download or modify the real user's omp.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO_ROOT/bin/install-omp.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/test-install-omp.XXXXXX")"
trap 'rm -rf -- "$TEST_ROOT"' EXIT
export HOME="$TEST_ROOT/home"
# Keep host Python and package managers unreachable, even when installed.
mkdir -p "$TEST_ROOT/tools"
for tool in /usr/bin/*; do
	case "${tool##*/}" in python* | apt* | sudo | dnf | pacman) continue ;; esac
	ln -s "$tool" "$TEST_ROOT/tools/${tool##*/}"
done
export PATH="$TEST_ROOT/bin:$TEST_ROOT/tools"
export TEST_LOG="$TEST_ROOT/log"
export PI_INSTALL_DIR="$HOME/.local/bin"
export NON_INTERACTIVE=true
unset ACTION OMP_VERSION FORCE_INSTALL DRY_RUN NONINTERACTIVE
mkdir -p "$HOME/.omp" "$TEST_ROOT/bin" "$TEST_ROOT/tmp"
export TMPDIR="$TEST_ROOT/tmp"
export PY_LOG="$TEST_ROOT/python.log" PY_STATE="$TEST_ROOT/debugpy-installed"
: >"$PY_LOG"
cat >"$TEST_ROOT/bin/apt-get" <<'APT'
#!/usr/bin/env bash
printf 'apt-get %s\n' "$*" >>"$PY_LOG"
[[ "${FAIL_APT:-false}" != true ]] || exit 1
touch "$PY_STATE"
if [[ ! -x "$(dirname "$0")/python" ]]; then
    cp "$(dirname "$0")/python3" "$(dirname "$0")/python"
fi
APT
cat >"$TEST_ROOT/bin/sudo" <<'SUDO'
#!/usr/bin/env bash
exec "$@"
SUDO
chmod +x "$TEST_ROOT/bin/apt-get" "$TEST_ROOT/bin/sudo"
# Fake interpreter: never let tests touch a real Python environment.
cat >"$TEST_ROOT/bin/python" <<'PYTHON'
#!/usr/bin/env bash
printf 'python %s\n' "$*" >>"$PY_LOG"
case "$*" in
"-c import debugpy.adapter") [[ -e "$PY_STATE" ]] ;;
-) cat >/dev/null; printf '%s\n' "${PY_MODE:-site}" ;;
"-m pip --version") [[ "${NO_PIP:-false}" != true ]] ;;
"-m pip install"*)
	[[ "${FAIL_PIP:-false}" != true ]] || exit 1
	[[ "${PIP_NOOP:-false}" == true ]] || touch "$PY_STATE"
	;;
*) exit 2 ;;
esac
PYTHON
chmod +x "$TEST_ROOT/bin/python"
printf 'keep credentials\n' >"$HOME/.omp/auth.json"
printf 'keep shell\n' >"$HOME/.bashrc"
: >"$TEST_LOG"

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
contains() { grep -Fq -- "$1" "$2" || fail "Missing $1 in $2"; }
expect_failure() {
	if "$@" >"$TEST_ROOT/error" 2>&1; then fail "Unexpected success: $*"; fi
}

# Simulate the upstream script, including its --binary flag and version smoke test.
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
[ "$1" = --binary ]
if [ "${FAIL_INSTALL:-false}" = true ]; then exit 1; fi
mkdir -p "$PI_INSTALL_DIR"
if [ "${BROKEN_INSTALL:-false}" = true ]; then
	printf '#!/bin/sh\nexit 1\n' >"$PI_INSTALL_DIR/omp"
else
	printf '#!/bin/sh\nprintf "omp 18.8.9\\n"\n' >"$PI_INSTALL_DIR/omp"
fi
chmod +x "$PI_INSTALL_DIR/omp"
UPSTREAM
CURL
chmod +x "$TEST_ROOT/bin/curl"

bash "$INSTALLER" --help >"$TEST_ROOT/help"
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

bash "$INSTALLER" --dry-run 2>"$TEST_ROOT/dry"
[[ ! -e "$PI_INSTALL_DIR" && ! -s "$TEST_LOG" && ! -e "$PY_STATE" ]] || fail 'dry-run changed state'
contains 'Would run: python -m pip install --upgrade debugpy' "$TEST_ROOT/dry"
! grep -Fq 'pip install' "$PY_LOG" || fail 'dry-run ran pip'
expect_failure env NON_INTERACTIVE=false bash "$INSTALLER" </dev/null
[[ ! -s "$TEST_LOG" ]] || fail 'unapproved install downloaded a script'

bash "$INSTALLER" 2>"$TEST_ROOT/install"
[[ -x "$PI_INSTALL_DIR/omp" ]] || fail 'install did not create omp'
[[ -e "$PY_STATE" ]] || fail 'headless install did not install debugpy'
contains 'NOTICE: auto-approved' "$TEST_ROOT/install"
contains 'python -m pip install --upgrade debugpy' "$PY_LOG"
contains 'https://omp.sh/install' "$TEST_LOG"
contains 'installer --binary' "$TEST_LOG"
bash "$INSTALLER" --check >"$TEST_ROOT/version" 2>&1
contains 'omp 18.8.9' "$TEST_ROOT/version"
contains 'Python debugger: ready' "$TEST_ROOT/version"
cp "$TEST_LOG" "$TEST_ROOT/previous"
bash "$INSTALLER"
cmp "$TEST_LOG" "$TEST_ROOT/previous" || fail 'install was not idempotent'

# --debugger-only never downloads omp; covers present, missing, user, failures.
: >"$TEST_LOG"
: >"$PY_LOG"
bash "$INSTALLER" --debugger-only
! grep -Fq 'pip install' "$PY_LOG" || fail 'present debugpy was reinstalled'
rm "$PY_STATE"
bash "$INSTALLER" --check >"$TEST_ROOT/check" 2>&1
contains 'Python debugger: missing' "$TEST_ROOT/check"
env NON_INTERACTIVE=false PY_MODE=user bash "$INSTALLER" --debugger-only </dev/null 2>"$TEST_ROOT/notice"
contains 'NOTICE: auto-approved' "$TEST_ROOT/notice"
contains 'python -m pip install --user --upgrade debugpy' "$PY_LOG"
[[ ! -s "$TEST_LOG" ]] || fail '--debugger-only ran the omp installer'
rm "$PY_STATE"
# Real pseudo-terminal exercises yes/no prompts without touching the host.
if command -v script >/dev/null 2>&1; then
	: >"$PY_LOG"
	printf 'n\n' | env NON_INTERACTIVE=false script -q -e -c "bash '$INSTALLER' --debugger-only" /dev/null >"$TEST_ROOT/decline" 2>&1 && fail 'decline succeeded'
	[[ ! -e "$PY_STATE" ]] || fail 'decline installed debugpy'
	! grep -Fq 'pip install' "$PY_LOG" || fail 'decline ran pip'
	printf 'y\n' | env NON_INTERACTIVE=false script -q -e -c "bash '$INSTALLER' --debugger-only" /dev/null >"$TEST_ROOT/approve" 2>&1
	[[ -e "$PY_STATE" ]] || fail 'interactive approval did not install debugpy'
	rm "$PY_STATE"
fi
: >"$PY_LOG"
env PY_MODE=managed bash "$INSTALLER" --debugger-only
contains 'apt-get install -y python3-debugpy' "$PY_LOG"
! grep -Fq 'pip install' "$PY_LOG" || fail 'bypassed externally managed Python'
rm "$PY_STATE"
expect_failure env PY_MODE=managed FAIL_APT=true bash "$INSTALLER" --debugger-only

# Ubuntu case: only python3 + managed system Python installs both apt packages.
cp "$TEST_ROOT/bin/python" "$TEST_ROOT/bin/python3"
rm "$TEST_ROOT/bin/python"
: >"$PY_LOG"
env PY_MODE=managed bash "$INSTALLER" --debugger-only
contains 'apt-get install -y python3-debugpy python-is-python3' "$PY_LOG"
[[ -x "$TEST_ROOT/bin/python" ]] || fail 'python3-only case did not provision python'
rm "$PY_STATE" "$TEST_ROOT/bin/python3"
expect_failure env NO_PIP=true bash "$INSTALLER" --debugger-only
expect_failure env FAIL_PIP=true bash "$INSTALLER" --debugger-only
expect_failure env PIP_NOOP=true bash "$INSTALLER" --debugger-only
# Non-system python3-only setup creates an unversioned user-local link.
cp "$TEST_ROOT/bin/python" "$TEST_ROOT/bin/python3"
rm "$TEST_ROOT/bin/python"
PATH="$HOME/.local/bin:$PATH" bash "$INSTALLER" --debugger-only
[[ -L "$HOME/.local/bin/python" ]] || fail 'missing user-local python link'
rm "$HOME/.local/bin/python"
cp "$TEST_ROOT/bin/python3" "$TEST_ROOT/bin/python"
rm "$TEST_ROOT/bin/python3" "$PY_STATE"
# Optional debugger failures warn but never fail or undo omp.
env FAIL_PIP=true bash "$INSTALLER" --update 2>"$TEST_ROOT/warn"
contains 'Python debugger is not ready' "$TEST_ROOT/warn"
: >"$PY_LOG"
bash "$INSTALLER" --update --skip-debugger 2>/dev/null
[[ ! -s "$PY_LOG" ]] || fail '--skip-debugger checked Python'
touch "$PY_STATE"

: >"$TEST_LOG"
bash "$INSTALLER" --update --version 18.8.9
contains 'installer --binary --ref v18.8.9' "$TEST_LOG"
: >"$TEST_LOG"
env NON_INTERACTIVE=false bash "$INSTALLER" --force --version v18.8.9
contains 'installer --binary --ref v18.8.9' "$TEST_LOG"

cp "$PI_INSTALL_DIR/omp" "$TEST_ROOT/original"
expect_failure env FAIL_DOWNLOAD=true bash "$INSTALLER" --update
[[ ! -e "$HOME/partial-script-executed" ]] || fail 'partial download was executed'
cmp "$PI_INSTALL_DIR/omp" "$TEST_ROOT/original" || fail 'failed download changed existing binary'
expect_failure env FAIL_INSTALL=true bash "$INSTALLER" --update
cmp "$PI_INSTALL_DIR/omp" "$TEST_ROOT/original" || fail 'failed install deleted existing binary'
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary script leaked on failure'

bash "$INSTALLER" --uninstall --dry-run
[[ -x "$PI_INSTALL_DIR/omp" ]] || fail 'dry-run removed binary'
expect_failure env NON_INTERACTIVE=false bash "$INSTALLER" --uninstall </dev/null
bash "$INSTALLER" --uninstall
[[ ! -e "$PI_INSTALL_DIR/omp" ]] || fail 'uninstall retained binary'
bash "$INSTALLER" --uninstall
contains 'keep credentials' "$HOME/.omp/auth.json"
contains 'keep shell' "$HOME/.bashrc"
[[ -e "$PY_STATE" ]] || fail 'uninstall removed shared debugpy'

# Unsupported ownership must not be overwritten or removed, even with --force.
cp "$TEST_ROOT/original" "$TEST_ROOT/bin/omp"
expect_failure bash "$INSTALLER" --update
expect_failure bash "$INSTALLER" --force
expect_failure bash "$INSTALLER" --uninstall
[[ -x "$TEST_ROOT/bin/omp" ]] || fail 'removed foreign binary'
rm "$TEST_ROOT/bin/omp"
ln -s "$TEST_ROOT/original" "$PI_INSTALL_DIR/omp"
expect_failure bash "$INSTALLER" --force
expect_failure bash "$INSTALLER" --uninstall
[[ -L "$PI_INSTALL_DIR/omp" ]] || fail 'removed symlink'
rm "$PI_INSTALL_DIR/omp"

# Custom directory with spaces is passed literally; both update and removal work.
export PI_INSTALL_DIR="$HOME/custom bin"
bash "$INSTALLER" --update
[[ -x "$PI_INSTALL_DIR/omp" ]] || fail 'update-on-absent/custom directory failed'
bash "$INSTALLER" --uninstall
expect_failure env BROKEN_INSTALL=true bash "$INSTALLER"
[[ -z "$(ls -A "$TMPDIR")" ]] || fail 'temporary script leaked on verification failure'

printf 'PASS: install-omp lifecycle tests\n'
