# shellcheck shell=bash
# Tests for the codex() wrapper in common/functions.sh:
#   - a missing or older app-server daemon runs bin/install-codex.sh --install
#     before launching codex from a terminal (script(1) supplies the tty), and
#     only prints a hint when not interactive;
#   - prereleases rank before their release;
#   - a matching or newer daemon launches codex without the installer;
#   - codex always runs with the caller's arguments, even when repair fails.
# The installer and codex are stubs that log to STUB_LOG; CODEX_HOME and
# DOTFILES_ROOT point into the test tmpdir.

load test_helper

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	STUB_LOG="$BATS_TEST_TMPDIR/stub.log"
	FAKE_CODEX_HOME="$BATS_TEST_TMPDIR/codex-home"
	FAKE_DOTFILES="$BATS_TEST_TMPDIR/dotfiles"
	mkdir -p "$FAKEBIN" "$FAKE_DOTFILES/bin"
	: >"$STUB_LOG"
	printf '#!/bin/sh\nprintf "codex %%s\\n" "$*" >>"%s"\n' "$STUB_LOG" >"$FAKEBIN/codex"
	printf '#!/bin/sh\nprintf "installer %%s\\n" "$*" >>"%s"\nexit "${INSTALLER_STATUS:-0}"\n' "$STUB_LOG" \
		>"$FAKE_DOTFILES/bin/install-codex.sh"
	chmod +x "$FAKEBIN/codex" "$FAKE_DOTFILES/bin/install-codex.sh"
}

# select_release <package> <release>: point a package's current link at a release.
select_release() {
	mkdir -p "$FAKE_CODEX_HOME/packages/$1/releases/$2"
	ln -sfn "$FAKE_CODEX_HOME/packages/$1/releases/$2" "$FAKE_CODEX_HOME/packages/$1/current"
}

# run_codex [args...]; CODEX_TTY=1 runs it under script(1) for a real tty.
run_codex() {
	local -a wrap=()
	if [[ "${CODEX_TTY:-0}" == 1 ]]; then
		# util-linux script(1); BSD/macOS script takes different arguments.
		script --version 2>/dev/null | grep -q util-linux || skip "needs util-linux script(1) for a tty"
		printf '%s\n' "$*" >"$BATS_TEST_TMPDIR/args"
		wrap=(script -qec "bash $BATS_TEST_TMPDIR/tty.sh" /dev/null)
		cat >"$BATS_TEST_TMPDIR/tty.sh" <<'TTY'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/functions.sh"
read -r -a args <"$ARGS_FILE" || true
codex "${args[@]}"
TTY
		run env -i HOME="$BATS_TEST_TMPDIR" PATH="$FAKEBIN:/usr/bin:/bin" \
			CODEX_HOME="$FAKE_CODEX_HOME" DOTFILES_ROOT="$FAKE_DOTFILES" \
			INSTALLER_STATUS="${INSTALLER_STATUS:-0}" REPO_ROOT="$REPO_ROOT" \
			ARGS_FILE="$BATS_TEST_TMPDIR/args" "${wrap[@]}"
		return
	fi
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="$FAKEBIN:/usr/bin:/bin" \
		CODEX_HOME="$FAKE_CODEX_HOME" DOTFILES_ROOT="$FAKE_DOTFILES" \
		INSTALLER_STATUS="${INSTALLER_STATUS:-0}" REPO_ROOT="$REPO_ROOT" \
		bash -c '
			export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
			export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
			source "$REPO_ROOT/common/platform.sh"
			source "$REPO_ROOT/common/functions.sh"
			codex "$@"
		' codex "$@"
}

@test "matching daemon launches codex without the installer" {
	select_release standalone 0.167.1-x86_64-unknown-linux-musl
	select_release app-server-daemon 0.167.1-x86_64-unknown-linux-musl
	run_codex exec hi
	[[ "$status" -eq 0 ]]
	run cat "$STUB_LOG"
	[[ "$output" == "codex exec hi" ]]
}

@test "newer daemon is left alone" {
	select_release standalone 0.167.1-x86_64-unknown-linux-musl
	select_release app-server-daemon 0.168.0-x86_64-unknown-linux-musl
	run_codex
	run cat "$STUB_LOG"
	[[ "$output" != *installer* ]]
}

@test "older daemon is repaired before codex runs" {
	select_release standalone 0.167.10-x86_64-unknown-linux-musl
	select_release app-server-daemon 0.167.9-x86_64-unknown-linux-musl
	CODEX_TTY=1 run_codex resume
	[[ "$output" == *"daemon is 0.167.9"* ]]
	run cat "$STUB_LOG"
	[[ "${lines[0]}" == "installer --install" ]]
	[[ "${lines[1]}" == "codex resume" ]]
}

@test "missing daemon is repaired before codex runs" {
	select_release standalone 0.167.1-x86_64-unknown-linux-musl
	CODEX_TTY=1 run_codex
	[[ "$output" == *"daemon is missing"* ]]
	run cat "$STUB_LOG"
	[[ "${lines[0]}" == "installer --install" ]]
}

@test "failed repair still launches codex" {
	select_release standalone 0.167.1-x86_64-unknown-linux-musl
	INSTALLER_STATUS=1 CODEX_TTY=1 run_codex
	[[ "$output" == *"Daemon repair failed"* ]]
	run cat "$STUB_LOG"
	[[ "${lines[0]}" == "installer --install" ]]
	[[ "${lines[1]}" == codex* ]]
}

@test "non-interactive launch only prints a hint" {
	select_release standalone 0.167.1-x86_64-unknown-linux-musl
	run_codex exec hi
	[[ "$output" == *"--install to update it"* ]]
	run cat "$STUB_LOG"
	[[ "$output" == "codex exec hi" ]]
}

@test "a release is newer than its prerelease" {
	select_release standalone 0.168.0-alpha.1-x86_64-unknown-linux-musl
	select_release app-server-daemon 0.168.0-x86_64-unknown-linux-musl
	run_codex
	run cat "$STUB_LOG"
	[[ "$output" != *installer* ]]
	select_release standalone 0.168.0-x86_64-unknown-linux-musl
	select_release app-server-daemon 0.168.0-alpha.1-x86_64-unknown-linux-musl
	run_codex
	[[ "$output" == *"daemon is 0.168.0-alpha.1"* ]]
}

@test "no standalone install skips the check" {
	run_codex
	run cat "$STUB_LOG"
	[[ "$output" != *installer* ]]
}
