# shellcheck shell=bash
# Tests for bin/install-claude.sh (YAN-1391):
#   - --uninstall removes the native install the official installer creates
#     (~/.local/bin/claude -> ~/.local/share/claude), the npm global package,
#     and the manual /usr/local/bin binary, and preserves ~/.claude config;
#   - --uninstall reports "not installed" honestly and fails when claude is
#     still on PATH or a location cannot be removed;
#   - --version is passed through to the installer and rejected when invalid.
# Hermetic: HOME is the test tmpdir, curl/npm/node/sudo are stubs that log to
# STUB_LOG, and CLAUDE_INSTALL_PATH points into the tmpdir. Nothing touches the
# network or a real Claude install.

load test_helper

setup() {
	FAKEHOME="$BATS_TEST_TMPDIR/home"
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	STUB_LOG="$BATS_TEST_TMPDIR/stub.log"
	SYSTEM_BIN="$BATS_TEST_TMPDIR/usr-local-bin/claude"
	mkdir -p "$FAKEHOME/.local/bin" "$FAKEBIN" "$(dirname "$SYSTEM_BIN")"
	: >"$STUB_LOG"

	# curl: the official installer URL yields a script that records its
	# target argument and creates the native layout; anything else fails
	# unless CURL_OK=1.
	cat >"$FAKEBIN/curl" <<'STUB'
#!/bin/bash
echo "curl $*" >>"$STUB_LOG"
if [[ "$*" == *"claude.ai/install.sh"* && "${CURL_FAIL:-0}" != 1 ]]; then
	cat <<'INSTALLER'
target="${1:-<none>}"
echo "official-installer target=$target skip_shell=${SKIP_SHELL_INTEGRATION:-}" >>"$STUB_LOG"
ver="$target"; [[ "$ver" =~ ^[0-9] ]] || ver=9.9.9
mkdir -p "$HOME/.local/share/claude/versions" "$HOME/.local/bin"
printf '#!/bin/sh\necho "%s (Claude Code)"\n' "$ver" >"$HOME/.local/share/claude/versions/$ver"
chmod +x "$HOME/.local/share/claude/versions/$ver"
ln -sfn "$HOME/.local/share/claude/versions/$ver" "$HOME/.local/bin/claude"
INSTALLER
	exit 0
fi
exit 22
STUB

	# npm: `ls -g` succeeds when NPM_STATE exists; `install -g` records the
	# spec and creates a claude bin; `uninstall -g` removes both.
	cat >"$FAKEBIN/npm" <<'STUB'
#!/bin/bash
echo "npm $*" >>"$STUB_LOG"
state="$BATS_TMP/npm-installed"
case "$1" in
ls) [[ -e "$state" ]] ;;
install)
	echo "${@: -1}" >"$state"
	mkdir -p "$BATS_TMP/npm-prefix/bin"
	ver="${3#*@*@}"; [[ "$ver" =~ ^[0-9] ]] || ver=9.9.9
	printf '#!/bin/sh\necho "%s (Claude Code)"\n' "$ver" >"$BATS_TMP/npm-prefix/bin/claude"
	chmod +x "$BATS_TMP/npm-prefix/bin/claude"
	;;
uninstall) rm -f "$state" "$BATS_TMP/npm-prefix/bin/claude" ;;
esac
STUB

	printf '#!/bin/sh\necho v20.0.0\n' >"$FAKEBIN/node"
	printf '#!/bin/sh\necho "sudo $*" >>"$STUB_LOG"\nexec "$@"\n' >"$FAKEBIN/sudo"
	chmod +x "$FAKEBIN"/*
}

# run_installer [args...]: run the script in a clean env with the stubs first
# on PATH, followed by the native and npm bin dirs (as a real shell would).
run_installer() {
	run env -i HOME="$FAKEHOME" \
		PATH="$FAKEBIN:$FAKEHOME/.local/bin:$BATS_TEST_TMPDIR/npm-prefix/bin:$BATS_TEST_TMPDIR/other-bin:/usr/bin:/bin" \
		STUB_LOG="$STUB_LOG" BATS_TMP="$BATS_TEST_TMPDIR" \
		CLAUDE_INSTALL_PATH="$SYSTEM_BIN" NON_INTERACTIVE=true \
		CURL_FAIL="${CURL_FAIL:-0}" \
		bash "$REPO_ROOT/bin/install-claude.sh" "$@"
}

# make_native_install: the layout the official installer leaves behind.
make_native_install() {
	mkdir -p "$FAKEHOME/.local/share/claude/versions"
	printf '#!/bin/sh\necho "1.0.0 (Claude Code)"\n' >"$FAKEHOME/.local/share/claude/versions/1.0.0"
	chmod +x "$FAKEHOME/.local/share/claude/versions/1.0.0"
	ln -s "$FAKEHOME/.local/share/claude/versions/1.0.0" "$FAKEHOME/.local/bin/claude"
}

@test "uninstall removes the native install and preserves config" {
	make_native_install
	mkdir -p "$FAKEHOME/.claude"
	echo '{}' >"$FAKEHOME/.claude.json"
	run_installer --uninstall
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	[[ ! -e "$FAKEHOME/.local/bin/claude" && ! -L "$FAKEHOME/.local/bin/claude" ]]
	[[ ! -e "$FAKEHOME/.local/share/claude" ]]
	[[ -d "$FAKEHOME/.claude" && -f "$FAKEHOME/.claude.json" ]]
	[[ "$output" == *"Claude Code uninstalled"* ]]
	[[ "$output" != *"not installed"* ]]
}

@test "uninstall removes leftover native data without a launcher" {
	mkdir -p "$FAKEHOME/.local/share/claude/versions"
	run_installer --uninstall
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	[[ ! -e "$FAKEHOME/.local/share/claude" ]]
}

@test "uninstall with nothing installed reports it and succeeds" {
	run_installer --uninstall
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	[[ "$output" == *"Claude Code is not installed"* ]]
	[[ "$output" == *".local/bin/claude"* ]]
	[[ "$output" != *"uninstalled"* ]]
}

@test "uninstall fails when claude on PATH comes from an unmanaged install" {
	mkdir -p "$BATS_TEST_TMPDIR/other-bin"
	printf '#!/bin/sh\necho 1.0.0\n' >"$BATS_TEST_TMPDIR/other-bin/claude"
	chmod +x "$BATS_TEST_TMPDIR/other-bin/claude"
	run_installer --uninstall
	[[ "$status" -ne 0 ]] || fail "expected failure: $output"
	[[ "$output" == *"other-bin/claude"* ]]
	[[ -x "$BATS_TEST_TMPDIR/other-bin/claude" ]]
}

@test "uninstall fails when another claude is still on PATH afterwards" {
	make_native_install
	mkdir -p "$BATS_TEST_TMPDIR/other-bin"
	printf '#!/bin/sh\necho 1.0.0\n' >"$BATS_TEST_TMPDIR/other-bin/claude"
	chmod +x "$BATS_TEST_TMPDIR/other-bin/claude"
	run_installer --uninstall
	[[ "$status" -ne 0 ]] || fail "expected failure: $output"
	[[ "$output" == *"still on PATH"* ]]
	[[ ! -e "$FAKEHOME/.local/share/claude" ]]
}

@test "uninstall refuses a ~/.local/bin/claude it did not create" {
	printf '#!/bin/sh\necho mine\n' >"$FAKEHOME/.local/bin/claude"
	chmod +x "$FAKEHOME/.local/bin/claude"
	run_installer --uninstall
	[[ "$status" -ne 0 ]] || fail "expected failure: $output"
	[[ "$output" == *"Refusing to remove"* ]]
	[[ -f "$FAKEHOME/.local/bin/claude" ]]
}

@test "uninstall removes the npm global package" {
	echo "@anthropic-ai/claude-code" >"$BATS_TEST_TMPDIR/npm-installed"
	mkdir -p "$BATS_TEST_TMPDIR/npm-prefix/bin"
	printf '#!/bin/sh\necho 1.0.0\n' >"$BATS_TEST_TMPDIR/npm-prefix/bin/claude"
	chmod +x "$BATS_TEST_TMPDIR/npm-prefix/bin/claude"
	run_installer --uninstall
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	grep -q '^npm uninstall -g @anthropic-ai/claude-code$' "$STUB_LOG"
	[[ ! -e "$BATS_TEST_TMPDIR/npm-prefix/bin/claude" ]]
}

@test "uninstall removes the manual system binary via sudo" {
	printf '#!/bin/sh\necho 1.0.0\n' >"$SYSTEM_BIN"
	chmod +x "$SYSTEM_BIN"
	run_installer --uninstall
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	grep -q "^sudo rm -f $SYSTEM_BIN$" "$STUB_LOG"
	[[ ! -e "$SYSTEM_BIN" ]]
}

@test "install without --version asks the official installer for latest" {
	run_installer --install
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	grep -q '^official-installer target=latest skip_shell=1$' "$STUB_LOG"
}

@test "--version is passed through to the official installer and verified" {
	run_installer --install --version 1.0.58
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	grep -q '^official-installer target=1.0.58 ' "$STUB_LOG"
	[[ "$output" == *"1.0.58 (Claude Code)"* ]]
}

@test "--version=vX.Y.Z is normalized and honored" {
	run_installer --install --version=v1.2.3
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	grep -q '^official-installer target=1.2.3 ' "$STUB_LOG"
}

@test "--version reaches npm when the official installer fails" {
	CURL_FAIL=1 run_installer --install --version 1.0.58
	[[ "$status" -eq 0 ]] || fail "status $status: $output"
	grep -q '^npm install -g @anthropic-ai/claude-code@1.0.58$' "$STUB_LOG"
}

@test "invalid --version is rejected before anything is downloaded" {
	run_installer --install --version 'not a version'
	[[ "$status" -ne 0 ]]
	[[ "$output" == *"Invalid version 'not a version'"* ]]
	! grep -q '^curl' "$STUB_LOG"
}

@test "--version without a value is rejected" {
	run_installer --install --version
	[[ "$status" -ne 0 ]]
	[[ "$output" == *"--version requires a value"* ]]
	run_installer --version --install
	[[ "$status" -ne 0 ]]
	[[ "$output" == *"--version requires a value"* ]]
}

@test "--version with --uninstall is rejected" {
	make_native_install
	run_installer --uninstall --version 1.0.58
	[[ "$status" -ne 0 ]]
	[[ "$output" == *"applies only to --install and --update"* ]]
	[[ -L "$FAKEHOME/.local/bin/claude" ]]
}
