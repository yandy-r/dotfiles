# shellcheck shell=bash

load test_helper

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$FAKEBIN"

	# Allow only the commands needed by the dry-run path, so an installed
	# hadolint cannot short-circuit it and no requests can reach the network.
	local tool
	for tool in dirname grep sed; do
		ln -s "$(command -v "$tool")" "$FAKEBIN/$tool"
	done

	cat >"$FAKEBIN/curl" <<'STUB'
#!/bin/sh
[ "$2" = https://api.github.com/repos/hadolint/hadolint/releases/latest ] || exit 22
printf '%s\n' '{"tag_name":"v2.15.1"}'
STUB
	cat >"$FAKEBIN/uname" <<'STUB'
#!/bin/sh
[ "$1" = -m ] || exit 1
printf '%s\n' "$FAKE_ARCH"
STUB
	chmod +x "$FAKEBIN/curl" "$FAKEBIN/uname"
}

assert_hadolint_asset() {
	local os="$1" machine="$2" asset="$3"
	local url="https://github.com/hadolint/hadolint/releases/download/v2.15.1/$asset"
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="$FAKEBIN" \
		DOTFILES_NO_AUTO_SETUP=1 DOTFILES_OS="$os" DOTFILES_ARCH="$machine" \
		FAKE_ARCH="$machine" "${TEST_BASH:-$BASH}" \
		"$REPO_ROOT/bin/install-container-tools.sh" --dry-run hadolint
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	[[ "$output" == *"Would download: $url"$'\n'* ]] || fail "expected $url: $output"
}

@test "hadolint uses the macos arm64 asset on Apple Silicon" {
	assert_hadolint_asset macos arm64 hadolint-macos-arm64
}

@test "hadolint uses the macos x86_64 asset on Intel Macs" {
	assert_hadolint_asset macos x86_64 hadolint-macos-x86_64
}

@test "hadolint uses the linux arm64 asset on aarch64 Linux" {
	assert_hadolint_asset linux aarch64 hadolint-linux-arm64
}

@test "hadolint uses the linux x86_64 asset on x86_64 Linux" {
	assert_hadolint_asset linux x86_64 hadolint-linux-x86_64
}
