# shellcheck shell=bash
# Shared setup for the tests/shell bats suite (YAN-1346). Loaded by every
# *.bats file via `load test_helper`.
#
# The suite is hermetic: it never depends on the host OS. Platform behaviour is
# driven through DOTFILES_OS (the cached value is_macos/is_linux read), and
# detect_os is pointed at fixture files through the DOTFILES_OS_RELEASE_FILE /
# DOTFILES_PROC_VERSION_FILE seams in common/platform.sh, with uname stubbed as
# a shell function inside the child shell that runs detect_os.

REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd -P)"
PLATFORM_SH="$REPO_ROOT/common/platform.sh"
PATHS_SH="$REPO_ROOT/common/paths.sh"
fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# Source common/paths.sh for function-only use (no auto PATH setup, no
# platform auto setup), leaving DOTFILES_OS untouched for the test to set.
paths_load() {
	DOTFILES_NO_AUTO_PATH_SETUP=1
	DOTFILES_NO_AUTO_SETUP=1
	# shellcheck disable=SC1090
	source "$PATHS_SH"
	unset DOTFILES_NO_AUTO_PATH_SETUP DOTFILES_NO_AUTO_SETUP
}

# Write an os-release fixture; each remaining argument is a KEY=VALUE line.
# Prints the fixture path.
make_os_release() {
	local file="$1"
	shift
	: >"$file"
	local line
	for line in "$@"; do
		printf '%s\n' "$line" >>"$file"
	done
	printf '%s\n' "$file"
}

# Run detect_os hermetically and print its result.
#
# detect_os sources the os-release fixture into its caller's scope, so it is
# run in a child bash inside $( ); `uname` is stubbed there as a shell function
# (the stub falls back to the real uname for other flags), and the
# DOTFILES_*_FILE seams point at fixtures. A `none` argument means "file does
# not exist" (the seam is pointed at a path that is never created).
#
# Usage: detect_os_result <uname-s> <os_release_file|none> [proc_version_file|none]
detect_os_result() {
	local uname_s="$1" os_release="$2" proc_version="${3:-}"
	local missing_os="$BATS_TEST_TMPDIR/definitely-missing-os-release"
	local missing_proc="$BATS_TEST_TMPDIR/definitely-missing-proc-version"
	local stub="$BATS_TEST_TMPDIR/uname-stub.sh"
	[[ "$os_release" == none ]] && os_release="$missing_os"
	[[ "$proc_version" == none ]] && proc_version="$missing_proc"

	printf 'uname() { [ "$1" = -s ] && printf "%%s\\n" "%s" && return 0; command uname "$@"; }\n' \
		"$uname_s" >"$stub"

	DOTFILES_NO_AUTO_SETUP=1 \
		HOME="$BATS_TEST_TMPDIR" \
		DOTFILES_OS_RELEASE_FILE="$os_release" \
		DOTFILES_PROC_VERSION_FILE="$proc_version" \
		"${TEST_BASH:-bash}" -c '. "$1"; . "$2"; detect_os' _ "$stub" "$PLATFORM_SH"
}

# Source platform.sh quietly (no auto setup) for predicate/dirs tests.
platform_load() {
	DOTFILES_NO_AUTO_SETUP=1
	# shellcheck disable=SC1090
	source "$PLATFORM_SH"
	unset DOTFILES_NO_AUTO_SETUP
}
