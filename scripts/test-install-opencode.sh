#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALLER="$REPO_ROOT/bin/install-opencode.sh"
TMP_DIR=""
# Captured before setup_case narrows PATH, for tests that need a real node.
REAL_NODE="$(command -v node 2>/dev/null || true)"

cleanup() {
	[[ -z "$TMP_DIR" ]] || rm -rf "$TMP_DIR"
}

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

assert_eq() {
	local want="$1" got="$2"
	[[ "$got" == "$want" ]] || fail "got '$got', want '$want'"
}

assert_contains() {
	local needle="$1" file="$2"
	grep -Fq -- "$needle" "$file" || fail "'$needle' missing from $file"
}

setup_case() {
	cleanup
	TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/test-install-opencode.XXXXXX")"
	trap cleanup EXIT
	export HOME="$TMP_DIR/home"
	export TEST_BIN="$TMP_DIR/bin"
	export TEST_LOG="$TMP_DIR/commands.log"
	mkdir -p "$HOME/.config/opencode" "$TEST_BIN"
	export OPENCODE_V2_MARKER="$TMP_DIR/state/opencode-v2"
	unset MISE_DATA_DIR XDG_DATA_HOME BUN_INSTALL NPM_CONFIG_USERCONFIG
	: >"$TEST_LOG"
	export PATH="$TEST_BIN:/usr/bin:/bin"
	hash -r
}

write_opencode() {
	local version="$1"
	cat >"$TEST_BIN/opencode" <<EOF
#!/usr/bin/env bash
printf '%s\n' '$version'
EOF
	chmod +x "$TEST_BIN/opencode"
}

test_generation_detection() {
	setup_case
	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"

	assert_eq "absent" "$(opencode_generation)"
	write_opencode "opencode 1.1.45"
	assert_eq "v1" "$(opencode_generation)"
	write_opencode "2.0.6"
	assert_eq "v2" "$(opencode_generation)"
}

test_v1_npm_migration_preserves_settings() {
	setup_case
	write_opencode "1.1.45"
	printf '%s\n' '{"model":"preserve-me"}' >"$HOME/.config/opencode/opencode.json"

	touch "$TMP_DIR/npm-opencode-ai"
	cat >"$TEST_BIN/npm" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'npm %s\n' "$*" >>"$TEST_LOG"
marker="$(dirname "$TEST_BIN")/npm-opencode-ai"
case "$*" in
  "list -g --depth=0 opencode-ai") [[ -e "$marker" ]] ;;
  "uninstall -g opencode-ai") rm -f "$marker" "$TEST_BIN/opencode" ;;
  *) exit 1 ;;
esac
EOF
	chmod +x "$TEST_BIN/npm"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	migrate_from_v1_if_needed

	assert_contains "npm uninstall -g opencode-ai" "$TEST_LOG"
	assert_contains '"model":"preserve-me"' "$HOME/.config/opencode/opencode.json"
	assert_eq "absent" "$(opencode_generation)"
}

test_shadowed_v1_binary_is_removed() {
	setup_case
	local second_dir="$TMP_DIR/legacy-bin"
	mkdir -p "$second_dir"
	export PATH="$TEST_BIN:$second_dir:/usr/bin:/bin"

	write_opencode "2.0.6"
	cat >"$second_dir/opencode" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' '1.18.31'
EOF
	chmod +x "$second_dir/opencode"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	migrate_from_v1_if_needed

	[[ -x "$TEST_BIN/opencode" ]] || fail "V2 binary was removed"
	[[ ! -e "$second_dir/opencode" ]] || fail "shadowed V1 binary was not removed"
}

test_official_v2_installer_arguments() {
	setup_case
	cat >"$TEST_BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'curl %s\n' "$*" >>"$TEST_LOG"
cat <<'INSTALL'
#!/usr/bin/env bash
printf 'installer %s\n' "$*" >>"$TEST_LOG"
INSTALL
EOF
	chmod +x "$TEST_BIN/curl"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	FORCE_INSTALL=true
	OPENCODE_VERSION=2.0.6
	install_opencode_official_script

	assert_contains "curl -fsSL https://opencode.ai/v2/install" "$TEST_LOG"
	assert_contains "installer --no-modify-path --version 2.0.6" "$TEST_LOG"
}

test_node_toolchain_allows_postinstall_scripts() {
	setup_case
	for tool in npm bun pnpm; do
		cat >"$TEST_BIN/$tool" <<EOF
#!/usr/bin/env bash
if [[ "\$1" == "install" && "\$2" == "--help" ]]; then
	echo "  --allow-scripts"
	exit 0
fi
printf '$tool %s\n' "\$*" >>"\$TEST_LOG"
exit 0
EOF
		chmod +x "$TEST_BIN/$tool"
	done
	cat >"$TEST_BIN/node" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
	chmod +x "$TEST_BIN/node"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	OPENCODE_VERSION=latest
	install_opencode_npm
	install_opencode_bun
	install_opencode_pnpm

	assert_contains "npm install -g --allow-scripts=@opencode/cli @opencode/cli@latest" "$TEST_LOG"
	assert_contains "bun install -g --trust @opencode/cli@latest" "$TEST_LOG"
	assert_contains "pnpm add -g --allow-build=@opencode/cli @opencode/cli@latest" "$TEST_LOG"
}

test_npm_falls_back_when_allow_scripts_unsupported() {
	setup_case
	cat >"$TEST_BIN/npm" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "install" && "$2" == "--help" ]]; then
  echo "Usage: npm install"
  exit 0
fi
printf 'npm %s\n' "$*" >>"$TEST_LOG"
exit 0
EOF
	chmod +x "$TEST_BIN/npm"
	cat >"$TEST_BIN/node" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
	chmod +x "$TEST_BIN/node"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	OPENCODE_VERSION=latest
	install_opencode_npm

	assert_contains "npm install -g @opencode/cli@latest" "$TEST_LOG"
	if grep -Fq -- "--allow-scripts" "$TEST_LOG"; then
		fail "old npm was passed --allow-scripts"
	fi
}

test_npm_allow_scripts_is_persisted() {
	setup_case
	export NPMRC_STATE="$TMP_DIR/npmrc-allow-scripts"
	cat >"$TEST_BIN/npm" <<'EOF'
#!/usr/bin/env bash
if [[ "$1" == "install" && "$2" == "--help" ]]; then
	echo "  --allow-scripts"
	exit 0
fi
case "$1 $2" in
  "config get") cat "$NPMRC_STATE" 2>/dev/null; exit 0 ;;
  "config set") printf '%s' "$5" >"$NPMRC_STATE"; printf 'npm %s\n' "$*" >>"$TEST_LOG"; exit 0 ;;
esac
exit 0
EOF
	chmod +x "$TEST_BIN/npm"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	ensure_npm_allow_scripts
	assert_eq "@opencode/cli" "$(cat "$NPMRC_STATE")"

	# Idempotent, and an existing allowlist is extended rather than replaced.
	ensure_npm_allow_scripts
	assert_eq "1" "$(grep -c "config set" "$TEST_LOG")"
	printf 'esbuild' >"$NPMRC_STATE"
	ensure_npm_allow_scripts
	assert_eq "esbuild,@opencode/cli" "$(cat "$NPMRC_STATE")"
}

test_postinstall_stub_is_repaired() {
	setup_case
	[[ -n "$REAL_NODE" ]] || { echo "SKIP: node not on PATH" >&2; return 0; }
	local node_dir pkg
	node_dir="$(dirname "$REAL_NODE")"
	export PATH="$TEST_BIN:$node_dir:/usr/bin:/bin"
	pkg="$HOME/.local/share/mise/installs/node/26.0.0/lib/node_modules/@opencode/cli"
	mkdir -p "$pkg/bin"
	ln -s 26.0.0 "$HOME/.local/share/mise/installs/node/26"
	printf '{"name":"@opencode/cli"}\n' >"$pkg/package.json"
	cat >"$pkg/bin/opencode.exe" <<'EOF'
echo "Error: @opencode/cli's postinstall script was not run." >&2
exit 1
EOF
	cat >"$pkg/postinstall.mjs" <<'EOF'
import fs from "node:fs"
fs.writeFileSync("bin/opencode.exe", "#!/bin/sh\necho 2.4.2\n")
fs.appendFileSync(process.env.TEST_LOG, "postinstall ran\n")
EOF

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	is_postinstall_stub "$pkg/bin/opencode.exe" || fail "stub not detected"
	repair_postinstall_stubs

	is_postinstall_stub "$pkg/bin/opencode.exe" && fail "stub not repaired"
	# The version alias symlink must not cause a second run.
	assert_eq "1" "$(grep -c "postinstall ran" "$TEST_LOG")"
}

test_verification_writes_v2_marker() {
	setup_case
	write_opencode "2.0.6"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	verify_installation

	[[ -L "$OPENCODE_V2_MARKER" ]] || fail "marker is not a symlink"
	[[ -e "$OPENCODE_V2_MARKER" ]] || fail "marker does not resolve to the V2 binary"
	assert_eq "$TEST_BIN/opencode" "$(readlink "$OPENCODE_V2_MARKER")"

	# An external uninstall must make the marker dangle so dotf reports absent.
	rm -f "$TEST_BIN/opencode"
	if [[ -e "$OPENCODE_V2_MARKER" ]]; then
		fail "marker still resolves after the V2 binary was removed"
	fi
}

test_uninstall_removes_all_binaries_and_keeps_config() {
	setup_case
	local second_dir="$TMP_DIR/extra-bin"
	mkdir -p "$second_dir"
	export PATH="$TEST_BIN:$second_dir:/usr/bin:/bin"

	write_opencode "2.0.6"
	cp "$TEST_BIN/opencode" "$second_dir/opencode"
	printf '%s\n' '{"model":"keep"}' >"$HOME/.config/opencode/opencode.json"

	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	verify_installation
	uninstall_opencode

	[[ ! -e "$TEST_BIN/opencode" ]] || fail "first binary not removed"
	[[ ! -e "$second_dir/opencode" ]] || fail "second binary not removed"
	[[ ! -e "$OPENCODE_V2_MARKER" ]] || fail "marker not cleared"
	assert_contains '"model":"keep"' "$HOME/.config/opencode/opencode.json"
}

test_platform_method_order() {
	setup_case
	# shellcheck source=../bin/install-opencode.sh
	source "$INSTALLER"
	OPENCODE_VERSION=latest

	# Test doubles are called indirectly by install_for_platform.
	# shellcheck disable=SC2317
	install_opencode_homebrew() { echo homebrew >>"$TEST_LOG"; }
	# shellcheck disable=SC2317
	install_opencode_aur() { echo aur >>"$TEST_LOG"; }
	# shellcheck disable=SC2317
	install_opencode_official_script() { echo official >>"$TEST_LOG"; }
	# shellcheck disable=SC2317
	install_opencode_npm() { echo npm >>"$TEST_LOG"; }

	install_for_platform macos
	install_for_platform arch
	install_for_platform ubuntu

	mapfile -t methods <"$TEST_LOG"
	assert_eq "homebrew" "${methods[0]}"
	assert_eq "aur" "${methods[1]}"
	assert_eq "official" "${methods[2]}"
}

(test_generation_detection)
(test_v1_npm_migration_preserves_settings)
(test_shadowed_v1_binary_is_removed)
(test_official_v2_installer_arguments)
(test_node_toolchain_allows_postinstall_scripts)
(test_npm_falls_back_when_allow_scripts_unsupported)
(test_npm_allow_scripts_is_persisted)
(test_postinstall_stub_is_repaired)
(test_verification_writes_v2_marker)
(test_uninstall_removes_all_binaries_and_keeps_config)
(test_platform_method_order)

echo "PASS: install-opencode V2 tests"
