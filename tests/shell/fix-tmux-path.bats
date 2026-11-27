# shellcheck shell=bash
# fix_tmux_path really rebuilds PATH in an already-configured shell (where the
# paths.sh load guard DOTFILES_PATHS_LOADED is set), resolves the repo through
# DOTFILES_ROOT, and never reports success when paths.sh did not run (YAN-1438).

load test_helper

setup() {
	# Hermetic HOME with one user bin dir that setup_user_paths prepends, so a
	# real rebuild is observable regardless of the host.
	mkdir -p "$BATS_TEST_TMPDIR/.local/bin"
}

# run_fix <shell> [extra env...]: start from a configured shell (platform.sh +
# paths.sh + functions.sh sourced, so the guard is set), then inside tmux reset
# PATH to /usr/bin:/bin:X and call fix_tmux_path. tmux is a shell function that
# records its arguments. Prints "rc=<n>", the function output, "PATH=<path>" and
# the recorded tmux call (or "tmux=none"). SKIP_PATH_SETUP_AT_FIX=1 sets
# DOTFILES_NO_AUTO_PATH_SETUP just before the call; TMUX_FAILS=1 makes the
# tmux stub exit non-zero (server gone).
run_fix() {
	local tested_shell
	tested_shell="$(command -v "$1")" || fail "$1 not found"
	shift
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="/usr/bin:/bin" \
		REPO_ROOT="$REPO_ROOT" TEST_TMPDIR="$BATS_TEST_TMPDIR" \
		DOTFILES_OS=linux DOTFILES_NO_AUTO_SETUP=1 "$@" \
		"$tested_shell" -f -c '
			source "$REPO_ROOT/common/platform.sh" &&
			source "$REPO_ROOT/common/paths.sh" &&
			source "$REPO_ROOT/common/functions.sh" || exit 1
			[ -n "$DOTFILES_PATHS_LOADED" ] || { echo "guard not set by initial load"; exit 1; }
			tmux() { printf "%s\n" "$*" >"$TEST_TMPDIR/tmux-call"; [ -z "$TMUX_FAILS" ]; }
			TMUX=/tmp/tmux-test/default,1,0
			PATH=/usr/bin:/bin:X
			[ -n "$SKIP_PATH_SETUP_AT_FIX" ] && DOTFILES_NO_AUTO_PATH_SETUP=1
			fix_tmux_path
			echo "rc=$?"
			echo "PATH=$PATH"
			if [ -f "$TEST_TMPDIR/tmux-call" ]; then
				echo "tmux=$(cat "$TEST_TMPDIR/tmux-call")"
			else
				echo "tmux=none"
			fi
		'
}

assert_rebuilt() {
	[ "$status" -eq 0 ] || fail "shell exited $status: $output"
	[[ "$output" == *"rc=0"* ]] || fail "fix_tmux_path did not succeed: $output"
	[[ "$output" == *"Tmux PATH updated"* ]] || fail "no success message: $output"
	local new_path
	new_path="$(printf '%s\n' "$output" | sed -n 's/^PATH=//p')"
	[ "$new_path" != "/usr/bin:/bin:X" ] || fail "PATH was not rebuilt: $output"
	[[ ":$new_path:" == *":$BATS_TEST_TMPDIR/.local/bin:"* ]] ||
		fail "rebuilt PATH lacks ~/.local/bin: $new_path"
	[[ "$output" == *"tmux=set-environment -g PATH $new_path"* ]] ||
		fail "tmux global PATH not set to the rebuilt PATH: $output"
}

@test "bash: rebuilds PATH despite the paths.sh guard and pushes it to tmux" {
	run_fix bash DOTFILES_ROOT="$REPO_ROOT"
	assert_rebuilt
}

@test "zsh: rebuilds PATH despite the paths.sh guard and pushes it to tmux" {
	command -v zsh >/dev/null || skip "zsh not installed"
	run_fix zsh DOTFILES_ROOT="$REPO_ROOT"
	assert_rebuilt
}

@test "falls back to ~/.config/dotfiles when DOTFILES_ROOT is unset" {
	mkdir -p "$BATS_TEST_TMPDIR/.config"
	ln -s "$REPO_ROOT" "$BATS_TEST_TMPDIR/.config/dotfiles"
	run_fix bash
	assert_rebuilt
}

@test "no-op: when paths.sh setup stays skipped, PATH is unchanged and no success is reported" {
	# Setting DOTFILES_NO_AUTO_PATH_SETUP after the initial load keeps the
	# paths.sh load guard closed, so the re-source cannot rebuild PATH; the
	# function must say so, not claim success.
	run_fix bash DOTFILES_ROOT="$REPO_ROOT" SKIP_PATH_SETUP_AT_FIX=1
	[ "$status" -eq 0 ] || fail "shell exited $status: $output"
	[[ "$output" == *"PATH=/usr/bin:/bin:X"* ]] || fail "PATH changed: $output"
	[[ "$output" != *"Tmux PATH updated"* ]] || fail "false success reported: $output"
	[[ "$output" != *"rc=0"* ]] || fail "returned success on a no-op: $output"
	[[ "$output" == *"tmux=none"* ]] || fail "tmux PATH was overwritten on a no-op: $output"
}

@test "tmux set-environment failure: returns non-zero without reporting success" {
	run_fix bash DOTFILES_ROOT="$REPO_ROOT" TMUX_FAILS=1
	[ "$status" -eq 0 ] || fail "shell exited $status: $output"
	[[ "$output" != *"Tmux PATH updated"* ]] || fail "false success reported: $output"
	[[ "$output" != *"rc=0"* ]] || fail "returned success after tmux failed: $output"
}

@test "outside tmux: does nothing and says so" {
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="/usr/bin:/bin" REPO_ROOT="$REPO_ROOT" \
		DOTFILES_OS=linux DOTFILES_NO_AUTO_SETUP=1 bash -f -c '
			source "$REPO_ROOT/common/platform.sh" &&
			source "$REPO_ROOT/common/functions.sh" || exit 1
			PATH=/usr/bin:/bin:X
			fix_tmux_path
			echo "PATH=$PATH"
		'
	[[ "$output" == *"Not in tmux session"* ]] || fail "$output"
	[[ "$output" == *"PATH=/usr/bin:/bin:X"* ]] || fail "$output"
}
