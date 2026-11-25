# shellcheck shell=bash
# Regression test for YAN-1439: zsh/zprofile must call fix_homebrew_path_order
# on macOS only (matching finalize_path in common/paths.sh). On Linux its
# reordering pushes prepended entries such as mise shims behind /usr/bin in
# every login shell, and children inherit that PATH.
#
# zprofile is run against a fixture DOTFILES_ROOT whose platform.sh/paths.sh
# stubs pin is_macos and record whether fix_homebrew_path_order ran, so the
# result does not depend on the host OS.

load test_helper

TEST_ZSH="$(command -v zsh 2>/dev/null || true)"

# zprofile_calls_fix <is_macos-exit-status>: print "called" or "skipped".
zprofile_calls_fix() {
	local root="$BATS_TEST_TMPDIR/root-$1"
	mkdir -p "$root/common"
	printf 'is_macos() { return %s; }\n' "$1" >"$root/common/platform.sh"
	printf 'fix_homebrew_path_order() { FIX_CALLED=1; }\n' >"$root/common/paths.sh"
	# shellcheck disable=SC2016 # expanded by the child zsh
	env -i HOME="$BATS_TEST_TMPDIR" PATH=/usr/bin:/bin DOTFILES_ROOT="$root" \
		"$TEST_ZSH" -f -c '
			source "$1"
			if [[ -n "${FIX_CALLED:-}" ]]; then echo called; else echo skipped; fi
		' zsh "$REPO_ROOT/zsh/zprofile"
}

@test "zprofile: fix_homebrew_path_order is skipped on Linux" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	run zprofile_calls_fix 1
	[ "$status" -eq 0 ]
	[ "$output" = skipped ]
}

@test "zprofile: fix_homebrew_path_order still runs on macOS" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	run zprofile_calls_fix 0
	[ "$status" -eq 0 ]
	[ "$output" = called ]
}
