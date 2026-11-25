# shellcheck shell=bash
# Regression tests for terminal color exports and redirected jsonpp output (YAN-1428):
#   - common/exports.sh exports CLICOLOR=1 for TTY color and does not export
#     FORCE_COLOR globally (bash and zsh);
#   - redirected `jsonpp` (`python3 -m json.tool`) produces valid, ANSI-free JSON
#     after sourcing common/exports.sh and common/aliases.sh (bash and zsh);
#   - full interactive bash/zsh rc startup keeps FORCE_COLOR unset;
#   - on Python 3.14+, explicitly setting FORCE_COLOR=1 reproduces ANSI escape
#     pollution in redirected `python3 -m json.tool`, proving the regression guard.
# Every case runs under env -i with a sandbox PATH so host env vars (including a
# host-level FORCE_COLOR) cannot leak in.

load test_helper

TEST_BASH="${TEST_BASH:-$(command -v bash)}"
[[ "$TEST_BASH" == /* ]] || TEST_BASH="$(command -v "$TEST_BASH")"
TEST_ZSH="$(command -v zsh 2>/dev/null || true)"
[[ -z "$TEST_ZSH" || "$TEST_ZSH" == /* ]] || TEST_ZSH="$(command -v "$TEST_ZSH")"
PYTHON3_BIN="$(command -v python3 2>/dev/null || true)"
[[ -z "$PYTHON3_BIN" || "$PYTHON3_BIN" == /* ]] || PYTHON3_BIN="$(command -v "$PYTHON3_BIN")"

write_python3_wrapper() {
	local dest="$1" q_py
	q_py="'${PYTHON3_BIN//\'/\'\\\'\'}'"
	printf '#!/bin/sh\nexec %s "$@"\n' "$q_py" >"$dest"
	chmod 755 "$dest"
}

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	FAKE_HOME="$BATS_TEST_TMPDIR/home"
	OUT_JSON="$BATS_TEST_TMPDIR/out.json"
	mkdir -p "$FAKEBIN" "$FAKE_HOME"
	local tool path
	for tool in cat ls mkdir dirname basename tty locale tr grep awk sed id uname hostname stat find diff env sh; do
		path="$(type -P "$tool" 2>/dev/null || true)"
		[[ -n "$path" ]] && ln -sf "$path" "$FAKEBIN/$tool"
	done
	if [[ -n "$PYTHON3_BIN" ]]; then
		write_python3_wrapper "$FAKEBIN/python3"
	fi
}

# Verify a JSON file contains no ANSI escape sequences and parses to {"a": 1}.
assert_clean_json() {
	local file="$1"
	[ -s "$file" ] || fail "expected non-empty JSON output at $file"
	"$PYTHON3_BIN" - "$file" <<'PY'
import json
import sys

path = sys.argv[1]
raw = open(path, "rb").read()
assert b"\x1b" not in raw, f"ANSI escape sequence found in redirected JSON: {raw!r}"
parsed = json.loads(raw.decode("utf-8"))
assert parsed == {"a": 1}, f"unexpected JSON payload: {parsed!r}"
PY
}

field() {
	local esc=$'\e[?1034h'
	grep -a "^@@$1=" <<<"${2//"$esc"/}" | tail -n 1 | cut -d= -f2-
}

expect_field() {
	local key="$1" want="$2" out="$3" got
	got="$(field "$key" "$out")"
	[ "$got" = "$want" ] || fail "$key is '$got', want '$want':
$out"
}

@test "common/exports.sh exports CLICOLOR=1 and leaves FORCE_COLOR unset (bash)" {
	local probe="$BATS_TEST_TMPDIR/exports-bash.sh" out
	cat >"$probe" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
printf '@@CLICOLOR_VAL=%s\n' "${CLICOLOR:-}"
if env | grep -qx 'CLICOLOR=1'; then
	printf '@@CLICOLOR_EXPORTED=yes\n'
else
	printf '@@CLICOLOR_EXPORTED=no\n'
fi
if [ -n "${FORCE_COLOR+x}" ]; then
	printf '@@FORCE_COLOR_SET=yes\n'
else
	printf '@@FORCE_COLOR_SET=no\n'
fi
if env | grep -q '^FORCE_COLOR='; then
	printf '@@FORCE_COLOR_EXPORTED=yes\n'
else
	printf '@@FORCE_COLOR_EXPORTED=no\n'
fi
EOF
	out="$(env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		"$TEST_BASH" "$probe" 2>&1)"
	expect_field CLICOLOR_VAL 1 "$out"
	expect_field CLICOLOR_EXPORTED yes "$out"
	expect_field FORCE_COLOR_SET no "$out"
	expect_field FORCE_COLOR_EXPORTED no "$out"
}

@test "common/exports.sh exports CLICOLOR=1 and leaves FORCE_COLOR unset (zsh)" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	local probe="$BATS_TEST_TMPDIR/exports-zsh.zsh" out
	cat >"$probe" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
printf '@@CLICOLOR_VAL=%s\n' "${CLICOLOR:-}"
if env | grep -qx 'CLICOLOR=1'; then
	printf '@@CLICOLOR_EXPORTED=yes\n'
else
	printf '@@CLICOLOR_EXPORTED=no\n'
fi
if [ -n "${FORCE_COLOR+x}" ]; then
	printf '@@FORCE_COLOR_SET=yes\n'
else
	printf '@@FORCE_COLOR_SET=no\n'
fi
if env | grep -q '^FORCE_COLOR='; then
	printf '@@FORCE_COLOR_EXPORTED=yes\n'
else
	printf '@@FORCE_COLOR_EXPORTED=no\n'
fi
EOF
	out="$(env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		"$TEST_ZSH" -f "$probe" 2>&1)"
	expect_field CLICOLOR_VAL 1 "$out"
	expect_field CLICOLOR_EXPORTED yes "$out"
	expect_field FORCE_COLOR_SET no "$out"
	expect_field FORCE_COLOR_EXPORTED no "$out"
}

@test "redirected jsonpp produces clean JSON with exports.sh and aliases.sh sourced (bash)" {
	[[ -n "$PYTHON3_BIN" ]] || skip "python3 not installed"
	local probe="$BATS_TEST_TMPDIR/jsonpp-bash.sh" out
	cat >"$probe" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
shopt -s expand_aliases
source "$REPO_ROOT/common/aliases.sh"
printf '@@JSONPP_ALIAS=%s\n' "$(alias jsonpp 2>/dev/null || echo none)"
printf '{"a":1}\n' | jsonpp >"$OUT_JSON"
EOF
	out="$(env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" OUT_JSON="$OUT_JSON" \
		"$TEST_BASH" "$probe" 2>&1)"
	[[ "$(field JSONPP_ALIAS "$out")" == *'python3 -m json.tool'* ]] ||
		fail "jsonpp alias missing or unexpected: $out"
	assert_clean_json "$OUT_JSON"
}

@test "redirected jsonpp produces clean JSON with exports.sh and aliases.sh sourced (zsh)" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	[[ -n "$PYTHON3_BIN" ]] || skip "python3 not installed"
	local probe="$BATS_TEST_TMPDIR/jsonpp-zsh.zsh" out
	cat >"$probe" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/aliases.sh"
printf '@@JSONPP_ALIAS=%s\n' "$(alias jsonpp 2>/dev/null || echo none)"
printf '{"a":1}\n' | jsonpp >"$OUT_JSON"
EOF
	out="$(env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" OUT_JSON="$OUT_JSON" \
		"$TEST_ZSH" -f "$probe" 2>&1)"
	[[ "$(field JSONPP_ALIAS "$out")" == *'python3 -m json.tool'* ]] ||
		fail "jsonpp alias missing or unexpected: $out"
	assert_clean_json "$OUT_JSON"
}

@test "interactive bash and zsh startup leave FORCE_COLOR unset and jsonpp clean" {
	[[ -n "$PYTHON3_BIN" ]] || skip "python3 not installed"
	local hermetic_home="$BATS_TEST_TMPDIR/rc-home"
	local stub_bin="$BATS_TEST_TMPDIR/rc-stubs"
	mkdir -p "$hermetic_home/bin" "$stub_bin" "$BATS_TEST_TMPDIR/fakebrew"
	local tool
	for tool in starship direnv atuin zoxide mise fzf thefuck carapace eza; do
		printf '#!/bin/sh\nexit 0\n' >"$stub_bin/$tool"
	done
	printf '#!/bin/sh\necho "%s"\n' "$BATS_TEST_TMPDIR/fakebrew" >"$stub_bin/brew"
	write_python3_wrapper "$stub_bin/python3"
	chmod 755 "$stub_bin/"*
	cp "$stub_bin/"* "$hermetic_home/bin/"
	ln -s "$REPO_ROOT/bash/bashrc" "$hermetic_home/.bashrc"
	ln -s "$REPO_ROOT/bash/bash_profile" "$hermetic_home/.bash_profile"
	ln -s "$REPO_ROOT/bash/profile" "$hermetic_home/.profile"
	ln -s "$REPO_ROOT/zsh/zshrc" "$hermetic_home/.zshrc"
	ln -s "$REPO_ROOT/zsh/zprofile" "$hermetic_home/.zprofile"
	printf 'skip_global_compinit=1\n' >"$hermetic_home/.zshenv"

	local rc_path
	rc_path="$stub_bin:/usr/bin:/bin:$(dirname "$TEST_BASH")"
	local out_bash="$BATS_TEST_TMPDIR/rc-bash.json" out
	out="$(env -i HOME="$hermetic_home" PATH="$rc_path" TERM=xterm-256color \
		DOTFILES_ROOT="$REPO_ROOT" USER="${USER:-$(id -un)}" OUT_JSON="$out_bash" \
		"$TEST_BASH" --noediting -i -c '
			printf "@@FORCE_COLOR_SET=%s\n" "${FORCE_COLOR+yes}"
			printf "@@CLICOLOR=%s\n" "${CLICOLOR:-}"
			printf "{\"a\":1}\n" | jsonpp >"$OUT_JSON"
		' 2>&1)"
	expect_field FORCE_COLOR_SET "" "$out"
	expect_field CLICOLOR 1 "$out"
	assert_clean_json "$out_bash"

	if [[ -n "$TEST_ZSH" ]]; then
		local out_zsh="$BATS_TEST_TMPDIR/rc-zsh.json"
		out="$(env -i HOME="$hermetic_home" PATH="$rc_path:$(dirname "$TEST_ZSH")" TERM=xterm-256color \
			DOTFILES_ROOT="$REPO_ROOT" USER="${USER:-$(id -un)}" OUT_JSON="$out_zsh" \
			"$TEST_ZSH" -i -c '
				printf "@@FORCE_COLOR_SET=%s\n" "${FORCE_COLOR+yes}"
				printf "@@CLICOLOR=%s\n" "${CLICOLOR:-}"
				printf "{\"a\":1}\n" | jsonpp >"$OUT_JSON"
			' 2>&1)"
		expect_field FORCE_COLOR_SET "" "$out"
		expect_field CLICOLOR 1 "$out"
		assert_clean_json "$out_zsh"
	fi
}

@test "control: Python 3.14+ injects ANSI escapes into redirected json.tool when FORCE_COLOR=1" {
	[[ -n "$PYTHON3_BIN" ]] || skip "python3 not installed"
	if ! "$PYTHON3_BIN" -c 'import sys; sys.exit(0 if sys.version_info >= (3, 14) else 1)'; then
		skip "requires Python 3.14+ (colorized json.tool)"
	fi
	local polluted="$BATS_TEST_TMPDIR/polluted.json"
	printf '{"a":1}\n' | env -i FORCE_COLOR=1 "$PYTHON3_BIN" -m json.tool >"$polluted"
	"$PYTHON3_BIN" - "$polluted" <<'PY'
import json
import sys

raw = open(sys.argv[1], "rb").read()
assert b"\x1b" in raw, f"expected ANSI escapes under FORCE_COLOR=1, got: {raw!r}"
try:
    json.loads(raw.decode("utf-8"))
except json.JSONDecodeError:
    pass
else:
    raise AssertionError("expected JSONDecodeError when FORCE_COLOR=1 pollutes redirected json.tool")
PY
}
