#!/usr/bin/env bash
# Hermetic shell startup smoke tests (YAN-1346):
#   1. syntax: bash -n over the bash rc files, zsh -n over the zsh rc files.
#   2. startup stderr is empty for zsh -i, bash -i and bash -l -i under env -i
#      with a temp HOME whose rc files symlink this checkout (the same links
#      `dotf link` creates). Only bash's no-tty notices and bash 3.2's
#      ESC[?1034h meta-mode escape are filtered (same filter as run_shell in
#      scripts/test-shell-parity.sh).
#   3. login shells export DOTFILES_OS: `bash -l -c 'echo $DOTFILES_OS'` and
#      `zsh -l -c 'echo $DOTFILES_OS'` both print a non-empty value.
#   4. startup time budget: best of N (default 5, after one warm-up) runs of
#      zsh -i -c exit / bash -i -c exit in the hermetic HOME stays under
#      SHELL_STARTUP_BUDGET_MS (default 1000). Measured with $EPOCHREALTIME
#      (bash 5); when the driver's bash lacks it the budget check is skipped
#      with a message (macOS bash 3.2 has no sub-second clock that is portable
#      across the tools there). The shells under test are unaffected.
# zsh cases are skipped when zsh is absent.
#
# TEST_BASH (optional): absolute path of the bash binary under test; defaults
# to `command -v bash`. Only the shells under test use it, the driver itself
# needs bash >= 4. CI sets TEST_BASH=/bin/bash on macOS to cover bash 3.2.
# shellcheck disable=SC2016
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMP_DIR="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_DIR"' EXIT
umask 022

USER_NAME="${USER:-$(id -un)}"

TEST_BASH="${TEST_BASH:-$(command -v bash)}"
[[ "$TEST_BASH" == /* && -x "$TEST_BASH" ]] || {
	echo "FAIL: TEST_BASH must be an absolute path to an executable bash (got '$TEST_BASH')" >&2
	exit 1
}

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

for f in bash/bashrc bash/bash_profile bash/profile bash/prompt.sh; do
	"$TEST_BASH" -n "$REPO_ROOT/$f" || fail "bash -n $f"
done
if command -v zsh >/dev/null 2>&1; then
	for f in zsh/zshrc zsh/zprofile zsh/functions.zsh zsh/fzf.zsh; do
		zsh -n "$REPO_ROOT/$f" || fail "zsh -n $f"
	done
	# Completion files are extensionless shell scripts too.
	for f in "$REPO_ROOT"/zsh/completions/*; do
		[[ -f "$f" ]] || continue
		zsh -n "$f" || fail "zsh -n ${f#"$REPO_ROOT"/}"
	done
fi
echo "ok: rc file syntax"

# Hermetic temp HOME: rc files symlinked like `dotf link` does, DOTFILES_ROOT
# pointing at this checkout, and fake no-op tools so startup cannot depend on
# (or write to) anything on the host.
HOME_DIR="$TMP_DIR/home"
mkdir -p "$HOME_DIR/bin" "$TMP_DIR/fakebin"
for tool in starship direnv atuin zoxide mise fzf thefuck carapace eza; do
	printf '#!/bin/sh\nexit 0\n' >"$TMP_DIR/fakebin/$tool"
done
chmod 755 "$TMP_DIR/fakebin"/*
cp "$TMP_DIR/fakebin"/* "$HOME_DIR/bin/"
# A `brew` stub reporting an empty prefix (same as test-shell-parity.sh and
# test-shell-startup-cache.sh): otherwise get_homebrew_path falls back to the
# host's /opt/homebrew on macOS, putting real tools first on PATH and real
# completion dirs on fpath, so startup output and timing depend on the runner.
mkdir -p "$TMP_DIR/fakebrew/prefix"
printf '#!/bin/sh\necho "%s"\n' "$TMP_DIR/fakebrew/prefix" >"$TMP_DIR/fakebin/brew"
chmod 755 "$TMP_DIR/fakebin/brew"
ln -s "$REPO_ROOT/bash/bashrc" "$HOME_DIR/.bashrc"
ln -s "$REPO_ROOT/bash/bash_profile" "$HOME_DIR/.bash_profile"
ln -s "$REPO_ROOT/bash/profile" "$HOME_DIR/.profile"
ln -s "$REPO_ROOT/zsh/zshrc" "$HOME_DIR/.zshrc"
ln -s "$REPO_ROOT/zsh/zprofile" "$HOME_DIR/.zprofile"
# Debian/Ubuntu's /etc/zsh/zshrc runs a bare compinit unless this is set (same
# reason as scripts/test-shell-parity.sh's make_home).
printf 'skip_global_compinit=1\n' >"$HOME_DIR/.zshenv"

TEST_PATH="$TMP_DIR/fakebin:/usr/bin:/bin:$(dirname "$TEST_BASH")"
HAVE_ZSH=""
if command -v zsh >/dev/null 2>&1; then
	HAVE_ZSH=1
	TEST_PATH="$TEST_PATH:$(dirname "$(command -v zsh)")"
else
	echo "SKIP: zsh not installed; zsh cases skipped"
fi

# run <label> <out-prefix> <shell args...>: env -i + hermetic HOME, exit code
# must be 0, stdout captured to <out-prefix>.out, stderr to <out-prefix>.err.
run() {
	local label="$1" out="$2"
	shift 2
	env -i HOME="$HOME_DIR" PATH="$TEST_PATH" TERM=xterm-256color \
		DOTFILES_ROOT="$REPO_ROOT" USER="$USER_NAME" \
		"$@" </dev/null >"$out.out" 2>"$out.err" ||
		fail "$label: exited non-zero (stderr: $(<"$out.err"))"
}

# Filter bash's no-tty notices and the 3.2 readline meta-mode escape; anything
# else on stderr is startup noise and fails the check (same filter as
# run_shell in scripts/test-shell-parity.sh, plus the literal `exit`/`logout`
# lines bash echoes when an interactive `-c exit` session quits on EOF stdin).
check_quiet() {
	local label="$1" out="$2" err
	err="$(grep -av -e '^bash: cannot set terminal process group' \
		-e '^bash: no job control in this shell' "$out.err")" || true
	err="${err//$'\e[?1034h'/}"
	err="$(grep -av -E '^(exit|logout)$' <<<"$err")" || true
	[[ -z "$err" ]] || fail "$label: startup wrote to stderr: $err"
}

# 2. Interactive startup smoke: quiet stderr, exit 0.
if [[ -n "$HAVE_ZSH" ]]; then
	run "zsh-i" "$TMP_DIR/zsh-i" zsh -i -c exit
	check_quiet "zsh -i -c exit" "$TMP_DIR/zsh-i"
	run "zsh-i-probe" "$TMP_DIR/zsh-ip" zsh -i -c 'alias ll >/dev/null || exit 9'
	check_quiet "zsh -i -c (probe)" "$TMP_DIR/zsh-ip"
fi
run "bash-i" "$TMP_DIR/bash-i" "$TEST_BASH" -i -c exit
check_quiet "bash -i -c exit" "$TMP_DIR/bash-i"
run "bash-li" "$TMP_DIR/bash-li" "$TEST_BASH" -l -i -c exit
check_quiet "bash -l -i -c exit" "$TMP_DIR/bash-li"
echo "ok: hermetic interactive startup (zsh -i, bash -i, bash -l -i) is quiet"

# 3. Login shells export DOTFILES_OS (bash_profile's non-interactive path and
# zprofile must both set it for `bash -l -c` / `zsh -l -c`).
run "bash-login" "$TMP_DIR/bash-login" "$TEST_BASH" -l -c 'printf "%s" "${DOTFILES_OS:-}"'
[[ -s "$TMP_DIR/bash-login.out" ]] ||
	fail "bash -l -c: DOTFILES_OS is empty (non-interactive login shells must source common/platform.sh via bash_profile)"
check_quiet "bash -l -c DOTFILES_OS" "$TMP_DIR/bash-login"
if [[ -n "$HAVE_ZSH" ]]; then
	run "zsh-login" "$TMP_DIR/zsh-login" zsh -l -c 'printf "%s" "${DOTFILES_OS:-}"'
	[[ -s "$TMP_DIR/zsh-login.out" ]] || fail "zsh -l -c: DOTFILES_OS is empty"
	check_quiet "zsh -l -c DOTFILES_OS" "$TMP_DIR/zsh-login"
fi
echo "ok: bash -l and zsh -l export a non-empty DOTFILES_OS"

# 4. Startup time budget: best of N timed runs per shell in the hermetic HOME.
# $EPOCHREALTIME (bash 5) gives microsecond wall clock without forking date;
# without it there is no portable sub-second clock across macOS's bash-3.2-era
# userland, so the check degrades to a skip with a message.
BUDGET_MS="${SHELL_STARTUP_BUDGET_MS:-1000}"
RUNS="${SHELL_STARTUP_RUNS:-5}"

now_s() {
	echo "${EPOCHREALTIME:-}"
}

timed_case() {
	local label="$1" shell_bin="$2" out="$3"
	shift 3
	local i t0 t1 best="" ms
	# One warm-up run first (cache, zcompile, etc.).
	run "$label (warm-up)" "$out-warm" "$shell_bin" "$@" exit
	for ((i = 0; i < RUNS; i++)); do
		t0="$(now_s)"
		run "$label (run $i)" "$out-$i" "$shell_bin" "$@" exit
		t1="$(now_s)"
		check_quiet "$label (run $i)" "$out-$i"
		ms="$(awk -v a="$t0" -v b="$t1" 'BEGIN { printf "%.0f", (b - a) * 1000 }')"
		echo "  $label: run $((i + 1))/${RUNS} ${ms}ms"
		if [[ -z "$best" ]] || ((ms < best)); then best=$ms; fi
	done
	echo "  $label: best of ${RUNS}: ${best}ms (budget ${BUDGET_MS}ms)"
	((best <= BUDGET_MS)) || fail "$label: best startup ${best}ms exceeds budget ${BUDGET_MS}ms"
}

if [[ -n "$(now_s)" ]]; then
	[[ -z "$HAVE_ZSH" ]] ||
		timed_case "zsh -i -c exit" zsh "$TMP_DIR/t-zsh" -i -c
	timed_case "bash -i -c exit" "$TEST_BASH" "$TMP_DIR/t-bash" -i -c
	echo "ok: startup time budget (${BUDGET_MS}ms; best of ${RUNS})"
else
	echo "SKIP: \$EPOCHREALTIME unavailable (bash ${BASH_VERSION}); cannot measure sub-second startup portably"
fi

echo "PASS: shell startup smoke tests"
