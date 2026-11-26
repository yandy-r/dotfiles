# shellcheck shell=bash
# Probe code expands in the child shell, not in Bats.
# shellcheck disable=SC2016
# YAN-1437: system_health, list_missing_tools and install_modern_tools iterate
# the associative-array tool tables in common/tools.sh. The bash-only
# ${!ARRAY[@]} key expansion is a "bad substitution" in zsh, so each function
# must list every table key in both bash and zsh through _tools_keys.
# Hermetic: tool_exists/tool_version/install_tool are stubbed in the child
# shell, so no real tool is probed or installed.
load test_helper

setup() {
	TEST_BASH="${TEST_BASH:-$(command -v bash)}"
	TEST_ZSH="$(command -v zsh || true)"
	FAKE_HOME="$BATS_TEST_TMPDIR/home"
	mkdir -p "$FAKE_HOME"
}

# Run code in a clean child shell with common/tools.sh sourced (no auto setup)
# and the tool probes stubbed. Output lands in $BATS_TEST_TMPDIR/probe.log.
probe() {
	local shell="$1" code="$2"
	PROBE_LOG="$BATS_TEST_TMPDIR/probe.log"
	if ! env -i HOME="$FAKE_HOME" PATH="/usr/bin:/bin" TERM=dumb \
		DOTFILES_ROOT="$REPO_ROOT" DOTFILES_OS=linux \
		DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_TOOL_SETUP=1 "$shell" -c '
			source "$DOTFILES_ROOT/common/platform.sh"
			source "$DOTFILES_ROOT/common/tools.sh"
			tool_exists() { [[ "$1" == eza || "$1" == git || "$1" == node ]]; }
			tool_version() { echo 9.9.9; }
			install_tool() { echo "install:$1"; }
			eval "$1"
		' _ "$code" >"$PROBE_LOG" 2>&1; then
		cat "$PROBE_LOG" >&2
		fail "${shell##*/} probe failed"
	fi
	if grep -qi 'bad substitution' "$PROBE_LOG"; then
		cat "$PROBE_LOG" >&2
		fail "${shell##*/} reported a bad substitution"
	fi
}

# True when the shell under test has associative arrays (zsh, bash >= 4).
# macOS CI also runs the suite under system bash 3.2, where tools.sh skips
# the tables and the functions must still run cleanly with nothing to list.
has_assoc() {
	"$1" -c '[[ -n "${ZSH_VERSION:-}" ]] || [[ "${BASH_VERSION%%.*}" -ge 4 ]]'
}

@test "_tools_keys lists every key in bash and zsh and rejects bad names" {
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" '
			_tools_keys "bad;name" && exit 1
			_tools_keys NO_SUCH_TABLE
		'
		[ ! -s "$PROBE_LOG" ] || fail "${shell##*/} printed keys for a missing table"
		has_assoc "$shell" || continue

		probe "$shell" '
			typeset -A T=(["a b"]=1 [c]=2)
			n=0
			while IFS= read -r k; do
				[[ "$k" == "a b" || "$k" == c ]] || exit 1
				n=$((n + 1))
			done < <(_tools_keys T)
			[[ $n -eq 2 ]] || exit 1
			typeset -A E=()
			[[ -z "$(_tools_keys E)" ]]
		'
	done
}

@test "system_health reports every modern and language tool in bash and zsh" {
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" 'system_health; true'
		grep -q '^=== Language Runtimes ===$' "$PROBE_LOG" ||
			fail "${shell##*/} system_health stopped early"
		has_assoc "$shell" || continue
		grep -qx '✓ eza 9.9.9' "$PROBE_LOG" || fail "${shell##*/}: eza missing"
		grep -qx '✗ bat (not installed)' "$PROBE_LOG" || fail "${shell##*/}: bat missing"
		grep -qx '✓ node 9.9.9' "$PROBE_LOG" || fail "${shell##*/}: node missing"
		grep -qx '✗ cargo (not installed)' "$PROBE_LOG" || fail "${shell##*/}: cargo missing"
		probe "$shell" '
			[[ $(system_health | sed -n "/^=== Modern CLI Tools ===$/,/^$/p" | grep -cE "^(✓|✗) ") -eq ${#MODERN_TOOLS[@]} ]] || exit 1
			[[ $(system_health | sed -n "/^=== Language Runtimes ===$/,\$p" | grep -cE "^(✓|✗) ") -eq ${#LANG_TOOLS[@]} ]]
		'
	done
}

@test "list_missing_tools lists missing modern and dev tools in bash and zsh" {
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" list_missing_tools
		has_assoc "$shell" || continue
		grep -qx 'bat' "$PROBE_LOG" || fail "${shell##*/}: modern tool bat not listed"
		grep -qx 'nvim' "$PROBE_LOG" || fail "${shell##*/}: dev tool nvim not listed"
		if grep -qx 'eza' "$PROBE_LOG" || grep -qx 'git' "$PROBE_LOG"; then
			fail "${shell##*/}: installed tool listed as missing"
		fi
		probe "$shell" '
			[[ $(list_missing_tools | grep -c "^[a-z]") -gt 0 ]] || exit 1
			want=$(( ${#MODERN_TOOLS[@]} + ${#DEV_TOOLS[@]} - 2 ))
			[[ $(list_missing_tools | sed -n "2,/^$/p" | grep -c .) -eq $want ]]
		'
	done
}

@test "install_modern_tools installs every modern tool in bash and zsh" {
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" install_modern_tools
		has_assoc "$shell" || continue
		grep -qx 'install:eza' "$PROBE_LOG" || fail "${shell##*/}: eza not installed"
		grep -qx 'install:atuin' "$PROBE_LOG" || fail "${shell##*/}: atuin not installed"
		probe "$shell" '[[ $(install_modern_tools | grep -c "^install:") -eq ${#MODERN_TOOLS[@]} ]]'
	done
}
