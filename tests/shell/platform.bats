# shellcheck shell=bash
# Tests for common/platform.sh: os_module_dirs resolution, the detect_os
# mapping (fixture os-release files, ID_LIKE derivatives, WSL via
# /proc/version, uname fallbacks). The Go mapping in
# internal/platform/platform.go (osFamilyFromID) stays canonical; these tests
# only pin the shell side to the same answers.

load test_helper

@test "os_module_dirs: every OS token" {
	platform_load
	[ "$(DOTFILES_OS=macos os_module_dirs macos)" = "macos" ]
	[ "$(os_module_dirs debian)" = $'linux\ndebian' ]
	[ "$(os_module_dirs redhat)" = $'linux\nredhat' ]
	[ "$(os_module_dirs arch)" = $'linux\narch' ]
	[ "$(os_module_dirs suse)" = $'linux\nsuse' ]
	[ "$(os_module_dirs wsl)" = $'linux\nwsl' ]
	[ "$(os_module_dirs linux)" = "linux" ]
	# No module directory for these platforms.
	[ -z "$(os_module_dirs windows)" ]
	[ -z "$(os_module_dirs unknown)" ]
}

@test "os_module_dirs: default comes from \$DOTFILES_OS" {
	platform_load
	[ "$(DOTFILES_OS=macos os_module_dirs)" = "macos" ]
	[ "$(DOTFILES_OS=arch os_module_dirs)" = $'linux\narch' ]
	[ "$(DOTFILES_OS=debian os_module_dirs)" = $'linux\ndebian' ]
	[ "$(DOTFILES_OS=linux os_module_dirs)" = "linux" ]
	[ "$(DOTFILES_OS=wsl os_module_dirs)" = $'linux\nwsl' ]
}

@test "os_module_dirs: linux token does not add an overlay dir" {
	platform_load
	[ "$(os_module_dirs linux | wc -l)" -eq 1 ]
	[ "$(os_module_dirs debian | wc -l)" -eq 2 ]
}

@test "detect_os: ubuntu and debian map to debian" {
	local f
	f="$(make_os_release "$BATS_TEST_TMPDIR/ubuntu" 'ID=ubuntu')"
	[ "$(detect_os_result Linux "$f")" = debian ]
	f="$(make_os_release "$BATS_TEST_TMPDIR/debian" 'ID=debian')"
	[ "$(detect_os_result Linux "$f")" = debian ]
}

@test "detect_os: fedora family maps to redhat" {
	local f
	f="$(make_os_release "$BATS_TEST_TMPDIR/fedora" 'ID=fedora')"
	[ "$(detect_os_result Linux "$f")" = redhat ]
	f="$(make_os_release "$BATS_TEST_TMPDIR/asahi" 'ID=fedora-asahi-remix')"
	[ "$(detect_os_result Linux "$f")" = redhat ]
}

@test "detect_os: arch family maps to arch" {
	local f
	f="$(make_os_release "$BATS_TEST_TMPDIR/arch" 'ID=arch')"
	[ "$(detect_os_result Linux "$f")" = arch ]
	f="$(make_os_release "$BATS_TEST_TMPDIR/cachyos" 'ID=cachyos')"
	[ "$(detect_os_result Linux "$f")" = arch ]
}

@test "detect_os: opensuse maps to suse" {
	local f
	f="$(make_os_release "$BATS_TEST_TMPDIR/tumbleweed" 'ID=opensuse-tumbleweed')"
	[ "$(detect_os_result Linux "$f")" = suse ]
}

@test "detect_os: ID_LIKE derivatives" {
	local f
	f="$(make_os_release "$BATS_TEST_TMPDIR/pop" 'ID=pop' 'ID_LIKE="ubuntu debian"')"
	[ "$(detect_os_result Linux "$f")" = debian ]
	f="$(make_os_release "$BATS_TEST_TMPDIR/nobara" 'ID=nobara' 'ID_LIKE=fedora')"
	[ "$(detect_os_result Linux "$f")" = redhat ]
}

@test "detect_os: unknown ID without ID_LIKE falls back to linux" {
	local f
	f="$(make_os_release "$BATS_TEST_TMPDIR/unknown-id" 'ID=coreos')"
	[ "$(detect_os_result Linux "$f")" = linux ]
}

@test "detect_os: missing os-release + Microsoft /proc/version means wsl" {
	local pv="$BATS_TEST_TMPDIR/proc-version"
	printf 'Linux version 5.15.153.1-microsoft-standard-WSL2\n' >"$pv"
	[ "$(detect_os_result Linux none "$pv")" = wsl ]
	# No Microsoft marker: plain linux.
	printf 'Linux version 6.8.0\n' >"$pv"
	[ "$(detect_os_result Linux none "$pv")" = linux ]
}

@test "detect_os: uname fallbacks" {
	[ "$(detect_os_result Darwin none none)" = macos ]
	[ "$(detect_os_result CYGWIN_NT-10.0 none none)" = windows ]
	[ "$(detect_os_result MINGW64_NT-10.0 none none)" = windows ]
	[ "$(detect_os_result MSYS_NT-10.0-22621 none none)" = windows ]
	[ "$(detect_os_result FreeBSD none none)" = unknown ]
}
