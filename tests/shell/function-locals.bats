# shellcheck shell=bash
# Shared helpers must not shadow zsh's special parameters (YAN-1422).

load test_helper

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$FAKEBIN"
}

run_shell() {
	local tested_shell
	tested_shell="$(command -v "$1")" || fail "$1 not found"
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="$FAKEBIN:/usr/bin:/bin" \
		REPO_ROOT="$REPO_ROOT" TEST_TMPDIR="$BATS_TEST_TMPDIR" \
		DOTFILES_OS=linux DOTFILES_NO_AUTO_SETUP=1 \
		"$tested_shell" -f -c '
			source "$REPO_ROOT/common/platform.sh" &&
			source "$REPO_ROOT/common/functions.sh" || exit 1
			'"$2"
}

check_cleanup_files() {
	local search_dir="$BATS_TEST_TMPDIR/search dir"
	mkdir -p "$search_dir/nested"
	touch "$search_dir/nested/remove.tmp" "$search_dir/keep.txt" "$search_dir/.DS_Store"

	run_shell "$1" 'cleanup_files "*.tmp" "$TEST_TMPDIR/search dir"'
	[[ "$status" -eq 0 ]] || fail "$output"
	[[ ! -e "$search_dir/nested/remove.tmp" && -f "$search_dir/keep.txt" ]]

	run_shell "$1" 'cd "$TEST_TMPDIR/search dir" && cleanup_files'
	[[ "$status" -eq 0 ]] || fail "$output"
	[[ ! -e "$search_dir/.DS_Store" && -f "$search_dir/keep.txt" ]]
}

check_brew_which() {
	cat >"$FAKEBIN/brew" <<'STUB'
#!/bin/sh
case "$1" in
	--prefix) printf '%s\n' /opt/homebrew ;;
	list) printf '%s\n' other fixture-tool ;;
	*) exit 2 ;;
esac
STUB
	chmod +x "$FAKEBIN/brew"

	run_shell "$1" '
		source "$REPO_ROOT/common/macos/functions.sh"
		which() { printf "%s\n" "/opt/homebrew/bin/$1"; }
		brew_which fixture-tool
	'
	[[ "$status" -eq 0 ]] || fail "$output"
	[[ "$output" == fixture-tool ]]
}

check_opencode() {
	cat >"$FAKEBIN/opencode" <<'STUB'
#!/bin/sh
printf 'opencode %s\n' "$*" >>"$TEST_TMPDIR/tmux.log"
exit 42
STUB
	chmod +x "$FAKEBIN/opencode"

	run_shell "$1" '
		tmux() {
			case "$1" in
				show-options) printf "%s\n" on ;;
				set-option) printf "%s\n" "$*" >>"$TEST_TMPDIR/tmux.log" ;;
			esac
		}
		TMUX=fixture opencode resume
	'
	[[ "$status" -eq 42 ]] || fail "exit $status, expected 42: $output"
	[[ $(cat "$BATS_TEST_TMPDIR/tmux.log") == $'set-option -p allow-passthrough off\nopencode resume\nset-option -p allow-passthrough on' ]]
}

@test "cleanup_files finds find and removes only matching files in Bash" {
	check_cleanup_files "${TEST_BASH:-bash}"
}

@test "cleanup_files finds find and removes only matching files in zsh" {
	check_cleanup_files zsh
}

@test "brew_which finds brew and prints the matching formula in Bash" {
	check_brew_which "${TEST_BASH:-bash}"
}

@test "brew_which finds brew and prints the matching formula in zsh" {
	check_brew_which zsh
}

@test "opencode restores tmux passthrough and returns the child status in Bash" {
	check_opencode "${TEST_BASH:-bash}"
}

@test "opencode restores tmux passthrough and returns the child status in zsh" {
	check_opencode zsh
}
