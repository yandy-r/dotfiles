# shellcheck shell=bash
# Tests for common/paths.sh:cleanup_unwanted_paths. The OS is driven through
# the cached DOTFILES_OS (is_linux/is_macos read it; live detect_* only runs
# when it is unset), so no case depends on the host OS. Each case runs in a
# subshell with a fixed per-OS HOME: the host HOME (e.g. /Users/runner on a
# macOS runner) must not decide what the Linux rules strip, and bats' own PATH
# must survive the test.

load test_helper
setup() {
	platform_load
	paths_load
}

# cleaned <os> <path>: print PATH after cleanup_unwanted_paths with
# DOTFILES_OS=<os>, HOME=/home/tester (linux) or /Users/tester (macos), and
# PATH=<path>. <path> may use $H for that HOME.
cleaned() {
	(
		DOTFILES_OS="$1"
		if [ "$1" = macos ]; then H=/Users/tester; else H=/home/tester; fi
		HOME="$H"
		PATH="${2//\$H/$H}"
		cleanup_unwanted_paths
		printf '%s' "$PATH"
	)
}

# assert_cleaned <os> <path> <want>
assert_cleaned() {
	local got
	got="$(cleaned "$1" "$2")"
	[ "$got" = "$3" ] || {
		echo "PATH mismatch ($1):
  want: $3
  got:  $got" >&2
		return 1
	}
}

@test "linux: drops /opt/homebrew, /Users, Nix and old Python user bins" {
	assert_cleaned linux '/usr/local/bin:/opt/homebrew/bin:/usr/bin:/Users/yandy/.turso/bin:$H/.nix-profile/bin:/nix/var/nix/profiles/default/bin:$H/Library/Python/3.9/bin:/bin' \
		/usr/local/bin:/usr/bin:/bin
}

@test "linux: macOS sbin and 3.7/3.8 Python bins go too" {
	assert_cleaned linux '/opt/homebrew/sbin:$H/Library/Python/3.8/bin:$H/Library/Python/3.7/bin:/usr/bin' /usr/bin
}

@test "linux: keeps normal dirs and their order" {
	assert_cleaned linux '$H/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:$H/.cargo/bin' \
		/home/tester/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/home/tester/.cargo/bin
}

@test "linux: keeps /home/linuxbrew (Linux Homebrew is valid there)" {
	assert_cleaned linux /home/linuxbrew/.linuxbrew/bin:/usr/bin /home/linuxbrew/.linuxbrew/bin:/usr/bin
}

@test "macos: drops /home/linuxbrew/*" {
	assert_cleaned macos /home/linuxbrew/.linuxbrew/bin:/home/linuxbrew/.linuxbrew/sbin:/opt/homebrew/bin:/usr/bin:/bin \
		/opt/homebrew/bin:/usr/bin:/bin
}

@test "macos: keeps /opt/homebrew, /Users and ~/Library/Python of a current version" {
	assert_cleaned macos '/opt/homebrew/sbin:/opt/homebrew/bin:$H/.local/bin:/usr/local/bin:/usr/bin:/bin:$H/Library/Python/3.13/bin' \
		/opt/homebrew/sbin:/opt/homebrew/bin:/Users/tester/.local/bin:/usr/local/bin:/usr/bin:/bin:/Users/tester/Library/Python/3.13/bin
}

@test "nix removal applies on macOS too" {
	assert_cleaned macos '$H/.nix-profile/bin:/nix/var/nix/profiles/default/bin:/usr/bin' /usr/bin
}
