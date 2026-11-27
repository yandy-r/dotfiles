# shellcheck shell=bash
# Tests for bin/install-fabric.sh (YAN-1398): on Arch, --update and
# --uninstall pass the headless flags to paru/yay when NON_INTERACTIVE is set
# (--noconfirm, plus --needed for -S), matching the install path, and leave the
# argv unchanged for interactive runs.
# Hermetic: paru/yay/brew/fabric are stubs that record argv to STUB_LOG, the script
# is sourced with detect_os pinned to arch and verify_installation stubbed, and
# HOME/FABRIC_INSTALL_DIR/GOBIN point into the test tmpdir.

load test_helper

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	STUB_LOG="$BATS_TEST_TMPDIR/stub.log"
	mkdir -p "$FAKEBIN" "$BATS_TEST_TMPDIR/home" "$BATS_TEST_TMPDIR/install" "$BATS_TEST_TMPDIR/gobin"
	: >"$STUB_LOG"

	printf '#!/bin/sh\necho "fabric 1.0.0"\n' >"$FAKEBIN/fabric"
	# brew and both AUR helpers are always stubbed so a host install is never
	# reached: brew and the unselected helper report fabric-ai as absent.
	printf '#!/bin/sh\nexit 1\n' >"$FAKEBIN/brew"
	write_helper paru 1
	write_helper yay 1
	chmod +x "$FAKEBIN/fabric" "$FAKEBIN/brew"
}

# write_helper <name> <-Q exit status>: an AUR helper stub whose -Q query
# exits with the given status and which records every other invocation's argv.
write_helper() {
	cat >"$FAKEBIN/$1" <<STUB
#!/bin/sh
[ "\$1" = "-Q" ] && exit $2
echo "$1 \$*" >>"\$STUB_LOG"
STUB
	chmod +x "$FAKEBIN/$1"
}

# make_helper <name>: select the helper that reports fabric-ai installed.
make_helper() {
	write_helper "$1" 0
}

# run_fabric <NON_INTERACTIVE value or ""> [args...]
run_fabric() {
	local ni="$1"
	shift
	local -a envs=(HOME="$BATS_TEST_TMPDIR/home" PATH="$FAKEBIN:/usr/bin:/bin"
		STUB_LOG="$STUB_LOG" FABRIC_INSTALL_DIR="$BATS_TEST_TMPDIR/install"
		GOBIN="$BATS_TEST_TMPDIR/gobin")
	if [[ -n "$ni" ]]; then
		envs+=(NON_INTERACTIVE="$ni")
	fi
	# shellcheck disable=SC2016 # expanded by the child shell
	run env -i "${envs[@]}" bash -c '
		source "$1"
		shift
		detect_os() { echo arch; }
		verify_installation() { :; }
		main "$@"
	' _ "$REPO_ROOT/bin/install-fabric.sh" "$@" </dev/null
}

@test "--update via paru passes --needed --noconfirm when NON_INTERACTIVE=true" {
	make_helper paru
	run_fabric true --update
	[[ "$status" -eq 0 ]] || fail "status=$status output=$output"
	[[ "$(cat "$STUB_LOG")" == "paru -S --needed --noconfirm fabric-ai" ]] || fail "argv: $(cat "$STUB_LOG")"
}

@test "--uninstall via paru passes --noconfirm when NON_INTERACTIVE=true" {
	make_helper paru
	run_fabric true --uninstall
	[[ "$status" -eq 0 ]] || fail "status=$status output=$output"
	[[ "$(cat "$STUB_LOG")" == "paru -R --noconfirm fabric-ai" ]] || fail "argv: $(cat "$STUB_LOG")"
}

@test "--update via yay passes --needed --noconfirm when NON_INTERACTIVE=1" {
	make_helper yay
	run_fabric 1 --update
	[[ "$status" -eq 0 ]] || fail "status=$status output=$output"
	[[ "$(cat "$STUB_LOG")" == "yay -S --needed --noconfirm fabric-ai" ]] || fail "argv: $(cat "$STUB_LOG")"
}

@test "--uninstall via yay passes --noconfirm when NON_INTERACTIVE=true" {
	make_helper yay
	run_fabric true --uninstall
	[[ "$status" -eq 0 ]] || fail "status=$status output=$output"
	[[ "$(cat "$STUB_LOG")" == "yay -R --noconfirm fabric-ai" ]] || fail "argv: $(cat "$STUB_LOG")"
}

@test "interactive --update and --uninstall leave paru argv unchanged" {
	make_helper paru
	run_fabric "" --update
	[[ "$status" -eq 0 ]] || fail "update status=$status output=$output"
	run_fabric "" --uninstall
	[[ "$status" -eq 0 ]] || fail "uninstall status=$status output=$output"
	[[ "$(cat "$STUB_LOG")" == $'paru -S fabric-ai\nparu -R fabric-ai' ]] || fail "argv: $(cat "$STUB_LOG")"
}

@test "interactive --update and --uninstall leave yay argv unchanged" {
	make_helper yay
	run_fabric "" --update
	[[ "$status" -eq 0 ]] || fail "update status=$status output=$output"
	run_fabric "" --uninstall
	[[ "$status" -eq 0 ]] || fail "uninstall status=$status output=$output"
	[[ "$(cat "$STUB_LOG")" == $'yay -S fabric-ai\nyay -R fabric-ai' ]] || fail "argv: $(cat "$STUB_LOG")"
}
