# shellcheck shell=bash
# Tests for guarded tool aliases and Debian/Ubuntu tool names (YAN-1345):
#   - kubectl aliases exist only when kubectl is on PATH;
#   - bat/fd resolve through batcat/fdfind when only those names exist;
#   - with real bat/fd present the alias text and env values are unchanged,
#     and with neither present nothing references them;
#   - vault/cdktf/carapace/thefuck setup is a no-op when the tool is absent.
# Every case runs under env -i with a sandbox PATH of symlinked coreutils
# plus per-case stubs, so host tools (/usr/bin/batcat, /usr/bin/fdfind, ...)
# can never leak in.

load test_helper

TEST_BASH="${TEST_BASH:-$(command -v bash)}"
[[ "$TEST_BASH" == /* ]] || TEST_BASH="$(command -v "$TEST_BASH")"
TEST_ZSH="$(command -v zsh 2>/dev/null || true)"
[[ -z "$TEST_ZSH" || "$TEST_ZSH" == /* ]] || TEST_ZSH="$(command -v "$TEST_ZSH")"

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	FAKE_HOME="$BATS_TEST_TMPDIR/home"
	STUB_LOG="$BATS_TEST_TMPDIR/stub.log"
	BASH_PROBE="$BATS_TEST_TMPDIR/probe.sh"
	ZSH_PROBE="$BATS_TEST_TMPDIR/probe.zsh"
	ZSH_FUNCS_PROBE="$BATS_TEST_TMPDIR/probe-funcs.zsh"
	TOOLS_PROBE="$BATS_TEST_TMPDIR/probe-tools.sh"
	RC_PROBE="$BATS_TEST_TMPDIR/probe-rc.sh"
	RC_ZSH_PROBE="$BATS_TEST_TMPDIR/probe-rc.zsh"
	RESOURCE_PROBE="$BATS_TEST_TMPDIR/probe-resource.sh"
	RESOURCE_ZSH_PROBE="$BATS_TEST_TMPDIR/probe-resource.zsh"
	mkdir -p "$FAKE_HOME"
	write_probes
}

# Symlink only the coreutils the sourced files need into the sandbox bin.
make_core_bin() {
	mkdir -p "$FAKEBIN"
	local tool path
	for tool in cat ls mkdir dirname basename tty locale tr grep awk sed id uname hostname stat find diff env sh; do
		path="$(type -P "$tool" 2>/dev/null || true)"
		[[ -n "$path" ]] && ln -sf "$path" "$FAKEBIN/$tool"
	done
}

stub() {
	printf '#!/bin/sh\nexit 0\n' >"$FAKEBIN/$1"
	chmod 755 "$FAKEBIN/$1"
}

# Recording stub: logs "<name> <args>" so tests can prove which binary a
# helper actually invoked.
recording_stub() {
	printf '#!/bin/sh\nprintf "%%s %%s\\n" "${0##*/}" "$*" >>"$STUB_LOG"\nexit 0\n' >"$FAKEBIN/$1"
	chmod 755 "$FAKEBIN/$1"
}

reset_bin() {
	rm -rf "$FAKEBIN"
	make_core_bin
	local tool
	for tool in "$@"; do
		case "$tool" in
		batcat-record) recording_stub batcat ;;
		fdfind-record) recording_stub fdfind ;;
		bat-record) recording_stub bat ;;
		fd-record) recording_stub fd ;;
		*) stub "$tool" ;;
		esac
	done
	: >"$STUB_LOG"
}

write_probes() {
	cat >"$BASH_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
# shellcheck disable=SC1091
source "$REPO_ROOT/common/platform.sh"
# shellcheck disable=SC1091
source "$REPO_ROOT/common/exports.sh"
shopt -s expand_aliases 2>/dev/null || true
# shellcheck disable=SC1091
source "$REPO_ROOT/common/aliases.sh"
# shellcheck disable=SC1091
source "$REPO_ROOT/common/functions.sh"

fact() { printf '@@%s=%s\n' "$1" "$2"; }
alias_fact() {
	local name="$1" key="$2" value
	if value="$(alias "$name" 2>/dev/null)"; then
		value="${value#alias }"
		value="${value#"$name"=}"
		value="${value#\'}"
		value="${value%\'}"
		fact "$key" "$value"
	else
		fact "$key" "none"
	fi
}
alias_fact bat BAT_ALIAS
alias_fact less LESS_ALIAS
alias_fact fd FD_ALIAS
alias_fact k K_ALIAS
alias_fact pods PODS_ALIAS
fact PAGER "${PAGER:-}"
fact MANPAGER "${MANPAGER:-}"
fact FZF_DEFAULT_COMMAND "${FZF_DEFAULT_COMMAND:-}"
fact FZF_ALT_C_COMMAND "${FZF_ALT_C_COMMAND:-}"
fact BAT_CMD "${DOTFILES_BAT_CMD:-}"
fact FD_CMD "${DOTFILES_FD_CMD:-}"

if [[ -n "${PROBE_FUNCS:-}" ]]; then
	printf 'a\n' >"$HOME/file-a"
	printf 'b\n' >"$HOME/file-b"
	(cd "$HOME" && json '{"a":1}' </dev/null >/dev/null 2>&1)
	(cd "$HOME" && compare file-a file-b >/dev/null 2>&1)
	f "file" >/dev/null 2>&1
	fd_dir "file" >/dev/null 2>&1
fi
EOF

	cat >"$ZSH_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/aliases.sh"

fact() { printf '@@%s=%s\n' "$1" "$2"; }
alias_fact() {
	local name="$1" key="$2" value
	if value="$(alias "$name" 2>/dev/null)"; then
		value="${value#alias }"
		value="${value#"$name"=}"
		value="${value#\'}"
		value="${value%\'}"
		fact "$key" "$value"
	else
		fact "$key" "none"
	fi
}
alias_fact bat BAT_ALIAS
alias_fact fd FD_ALIAS
alias_fact k K_ALIAS
alias_fact pods PODS_ALIAS
fact PAGER "${PAGER:-}"
fact FZF_DEFAULT_COMMAND "${FZF_DEFAULT_COMMAND:-}"
EOF

	cat >"$ZSH_FUNCS_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/aliases.sh"
source "$REPO_ROOT/common/functions.sh"
printf 'a\n' >"$HOME/file-a"
printf 'b\n' >"$HOME/file-b"
(cd "$HOME" && json '{"a":1}' </dev/null >/dev/null 2>&1)
(cd "$HOME" && compare file-a file-b >/dev/null 2>&1)
f "file" >/dev/null 2>&1
EOF

	cat >"$TOOLS_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/tools.sh"
source "$REPO_ROOT/common/completions.sh"

fact() { printf '@@%s=%s\n' "$1" "$2"; }
setup_vault_completions
fact VAULT_STATUS "$?"
setup_cdktf_completions
fact CDKTF_STATUS "$?"
setup_carapace_completions
fact CARAPACE_STATUS "$?"
setup_thefuck
fact THEFUCK_STATUS "$?"
if complete -p vault >/dev/null 2>&1; then
	fact VAULT_COMPLETE "yes"
else
	fact VAULT_COMPLETE "no"
fi
fact CARAPACE_BRIDGES "${CARAPACE_BRIDGES:-unset}"
setup_modern_alternatives
fact MODERN_MANPAGER "${MANPAGER:-}"
fact BAT_THEME "${BAT_THEME:-}"
fact BAT_PAGER "${BAT_PAGER:-}"
EOF

	# Real rc order: platform -> exports -> paths (auto) -> aliases -> functions.
	# paths.sh must run without DOTFILES_NO_AUTO_PATH_SETUP so tools that only
	# become reachable via $HOME/.cargo/bin are picked up on re-resolution.
	cat >"$RC_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/paths.sh"
source "$REPO_ROOT/common/aliases.sh"
source "$REPO_ROOT/common/functions.sh"
fact() { printf '@@%s=%s\n' "$1" "$2"; }
alias_fact() {
	local name="$1" key="$2" value
	if value="$(alias "$name" 2>/dev/null)"; then
		value="${value#alias }"
		value="${value#"$name"=}"
		value="${value#\'}"
		value="${value%\'}"
		fact "$key" "$value"
	else
		fact "$key" "none"
	fi
}
alias_fact bat BAT_ALIAS
alias_fact fd FD_ALIAS
fact PAGER "${PAGER:-}"
fact FZF_DEFAULT_COMMAND "${FZF_DEFAULT_COMMAND:-}"
fact FZF_CTRL_T_COMMAND "${FZF_CTRL_T_COMMAND:-}"
EOF

	cat >"$RC_ZSH_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/paths.sh"
source "$REPO_ROOT/common/aliases.sh"
fact() { printf '@@%s=%s\n' "$1" "$2"; }
alias_fact() {
	local name="$1" key="$2" value
	if value="$(alias "$name" 2>/dev/null)"; then
		value="${value#alias }"
		value="${value#"$name"=}"
		value="${value#\'}"
		value="${value%\'}"
		fact "$key" "$value"
	else
		fact "$key" "none"
	fi
}
alias_fact bat BAT_ALIAS
alias_fact fd FD_ALIAS
fact PAGER "${PAGER:-}"
fact FZF_DEFAULT_COMMAND "${FZF_DEFAULT_COMMAND:-}"
EOF

	# Re-source regression: source exports+aliases twice; the second pass sees
	# the aliases from the first, and must still resolve batcat/fdfind.
	cat >"$RESOURCE_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
shopt -s expand_aliases 2>/dev/null || true
source "$REPO_ROOT/common/aliases.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/aliases.sh"
fact() { printf '@@%s=%s\n' "$1" "$2"; }
alias_fact() {
	local name="$1" key="$2" value
	if value="$(alias "$name" 2>/dev/null)"; then
		value="${value#alias }"
		value="${value#"$name"=}"
		value="${value#\'}"
		value="${value%\'}"
		fact "$key" "$value"
	else
		fact "$key" "none"
	fi
}
alias_fact bat BAT_ALIAS
alias_fact less LESS_ALIAS
alias_fact fd FD_ALIAS
fact PAGER "${PAGER:-}"
fact BAT_CMD "${DOTFILES_BAT_CMD:-}"
fact FD_CMD "${DOTFILES_FD_CMD:-}"
fact FZF_DEFAULT_COMMAND "${FZF_DEFAULT_COMMAND:-}"
EOF

	cat >"$RESOURCE_ZSH_PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/aliases.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/aliases.sh"
fact() { printf '@@%s=%s\n' "$1" "$2"; }
alias_fact() {
	local name="$1" key="$2" value
	if value="$(alias "$name" 2>/dev/null)"; then
		value="${value#alias }"
		value="${value#"$name"=}"
		value="${value#\'}"
		value="${value%\'}"
		fact "$key" "$value"
	else
		fact "$key" "none"
	fi
}
alias_fact bat BAT_ALIAS
alias_fact fd FD_ALIAS
fact PAGER "${PAGER:-}"
fact BAT_CMD "${DOTFILES_BAT_CMD:-}"
fact FD_CMD "${DOTFILES_FD_CMD:-}"
EOF
}

run_bash_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" PROBE_FUNCS="${PROBE_FUNCS:-}" \
		"$TEST_BASH" "$BASH_PROBE" 2>&1
}

run_rc_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" \
		"$TEST_BASH" "$RC_PROBE" 2>&1
}

run_rc_zsh_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" \
		"$TEST_ZSH" -f "$RC_ZSH_PROBE" 2>&1
}

run_resource_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" \
		"$TEST_BASH" "$RESOURCE_PROBE" 2>&1
}

run_resource_zsh_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" \
		"$TEST_ZSH" -f "$RESOURCE_ZSH_PROBE" 2>&1
}

run_zsh_funcs_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" \
		"$TEST_ZSH" -f "$ZSH_FUNCS_PROBE" 2>&1
}

run_tools_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" \
		"$TEST_BASH" "$TOOLS_PROBE" 2>&1
}

run_zsh_probe() {
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		STUB_LOG="$STUB_LOG" \
		"$TEST_ZSH" -f "$ZSH_PROBE" 2>&1
}

# field <key> <output>: print the last @@key=value line, value only.
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

@test "kubectl absent: kubectl aliases are undefined (bash)" {
	reset_bin bat fd less
	local out
	out="$(run_bash_probe)"
	expect_field K_ALIAS none "$out"
	expect_field PODS_ALIAS none "$out"
}

@test "kubectl present: kubectl aliases are defined (bash)" {
	reset_bin kubectl less
	local out
	out="$(run_bash_probe)"
	expect_field K_ALIAS kubectl "$out"
	expect_field PODS_ALIAS "kubectl get pods" "$out"
}

@test "only batcat: bat/less aliases and pager use batcat (bash)" {
	reset_bin batcat less
	local out
	out="$(run_bash_probe)"
	expect_field BAT_ALIAS "batcat --paging=never" "$out"
	expect_field LESS_ALIAS "batcat --paging=always" "$out"
	expect_field PAGER "batcat --paging=always" "$out"
	expect_field MANPAGER "batcat --language=man --paging=always" "$out"
	expect_field BAT_CMD batcat "$out"
}

@test "only bat: bat/less aliases and pager keep exact bat strings (bash)" {
	reset_bin bat less
	local out
	out="$(run_bash_probe)"
	expect_field BAT_ALIAS "bat --paging=never" "$out"
	expect_field LESS_ALIAS "bat --paging=always" "$out"
	expect_field PAGER "bat --paging=always" "$out"
	expect_field MANPAGER "bat --language=man --paging=always" "$out"
}

@test "no bat or batcat: no bat aliases, pager falls back to less (bash)" {
	reset_bin less
	local out
	out="$(run_bash_probe)"
	expect_field BAT_ALIAS none "$out"
	expect_field LESS_ALIAS none "$out"
	expect_field PAGER less "$out"
	expect_field BAT_CMD "" "$out"
}

@test "only batcat: json and compare helpers invoke batcat (bash)" {
	reset_bin batcat-record less
	PROBE_FUNCS=1
	local out
	out="$(run_bash_probe)"
	grep -q '^batcat -l json --paging=never$' "$STUB_LOG" || fail "json did not invoke batcat:
$(cat "$STUB_LOG")"
	grep -q '^batcat -l diff --paging=never$' "$STUB_LOG" || fail "compare did not invoke batcat:
$(cat "$STUB_LOG")"
}

@test "only fdfind: fd alias and FZF commands use fdfind (bash)" {
	reset_bin fdfind less
	local out fzf
	out="$(run_bash_probe)"
	expect_field FD_ALIAS fdfind "$out"
	expect_field FD_CMD fdfind "$out"
	fzf="$(field FZF_DEFAULT_COMMAND "$out")"
	[[ "$fzf" == fdfind\ * ]] || fail "FZF_DEFAULT_COMMAND does not start with 'fdfind ': $fzf"
	[[ "$(field FZF_ALT_C_COMMAND "$out")" == fdfind\ * ]] || fail "FZF_ALT_C_COMMAND does not start with 'fdfind ':
$out"
}

@test "only fdfind: f and fd_dir invoke fdfind (bash)" {
	reset_bin fdfind-record less
	PROBE_FUNCS=1
	local out
	out="$(run_bash_probe)"
	grep -q '^fdfind file$' "$STUB_LOG" || fail "f did not invoke fdfind:
$(cat "$STUB_LOG")"
	grep -q '^fdfind --type d file$' "$STUB_LOG" || fail "fd_dir did not invoke fdfind:
$(cat "$STUB_LOG")"
}

@test "only fd: no fd alias, FZF commands keep exact fd strings (bash)" {
	reset_bin fd less
	local out
	out="$(run_bash_probe)"
	expect_field FD_ALIAS none "$out"
	expect_field FZF_DEFAULT_COMMAND 'fd --type f --hidden --follow --exclude .git 2>/dev/null || find . -type f -not -path "*/\.git/*"' "$out"
	expect_field FZF_ALT_C_COMMAND 'fd --type d --hidden --follow --exclude .git 2>/dev/null || find . -type d -not -path "*/\.git/*"' "$out"
}

@test "no fd or fdfind: FZF commands use only the find fallback (bash)" {
	reset_bin less
	local out
	out="$(run_bash_probe)"
	expect_field FD_ALIAS none "$out"
	expect_field FD_CMD "" "$out"
	expect_field FZF_DEFAULT_COMMAND 'find . -type f -not -path "*/\.git/*"' "$out"
	expect_field FZF_ALT_C_COMMAND 'find . -type d -not -path "*/\.git/*"' "$out"
}

@test "absent vault/cdktf/carapace/thefuck: setup is a no-op (bash)" {
	reset_bin less
	local out
	out="$(run_tools_probe)"
	expect_field VAULT_STATUS 0 "$out"
	expect_field CDKTF_STATUS 0 "$out"
	expect_field CARAPACE_STATUS 0 "$out"
	expect_field THEFUCK_STATUS 0 "$out"
	expect_field VAULT_COMPLETE no "$out"
	expect_field CARAPACE_BRIDGES unset "$out"
}

@test "only batcat: setup_modern_alternatives uses batcat (bash)" {
	reset_bin batcat less
	local out
	out="$(run_tools_probe)"
	expect_field MODERN_MANPAGER "sh -c 'col -bx | batcat -l man -p'" "$out"
	expect_field BAT_THEME "Sublime Snazzy" "$out"
	expect_field BAT_PAGER "less -RF" "$out"
}

@test "only bat: setup_modern_alternatives keeps exact bat strings (bash)" {
	reset_bin bat less
	local out
	out="$(run_tools_probe)"
	expect_field MODERN_MANPAGER "sh -c 'col -bx | bat -l man -p'" "$out"
}

@test "zsh: kubectl absent and only batcat present" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	reset_bin batcat less
	local out
	out="$(run_zsh_probe)"
	expect_field K_ALIAS none "$out"
	expect_field PODS_ALIAS none "$out"
	expect_field BAT_ALIAS "batcat --paging=never" "$out"
	expect_field PAGER "batcat --paging=always" "$out"
}

@test "zsh: kubectl present and only batcat present" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	reset_bin kubectl batcat less
	local out
	out="$(run_zsh_probe)"
	expect_field K_ALIAS kubectl "$out"
	expect_field PODS_ALIAS "kubectl get pods" "$out"
	expect_field BAT_ALIAS "batcat --paging=never" "$out"
}

@test "rc order: bat/fd only under .cargo/bin are picked up after paths.sh (bash)" {
	reset_bin less
	mkdir -p "$FAKE_HOME/.cargo/bin"
	printf '#!/bin/sh\nexit 0\n' >"$FAKE_HOME/.cargo/bin/bat"
	printf '#!/bin/sh\nexit 0\n' >"$FAKE_HOME/.cargo/bin/fd"
	chmod 755 "$FAKE_HOME/.cargo/bin/bat" "$FAKE_HOME/.cargo/bin/fd"
	local out
	out="$(run_rc_probe)"
	expect_field BAT_ALIAS "bat --paging=never" "$out"
	expect_field FD_ALIAS none "$out"
	expect_field PAGER "bat --paging=always" "$out"
	expect_field FZF_DEFAULT_COMMAND 'fd --type f --hidden --follow --exclude .git 2>/dev/null || find . -type f -not -path "*/\.git/*"' "$out"
	expect_field FZF_CTRL_T_COMMAND 'fd --type f --hidden --follow --exclude .git 2>/dev/null || find . -type f -not -path "*/\.git/*"' "$out"
}

@test "rc order: bat/fd only under .cargo/bin are picked up after paths.sh (zsh)" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	reset_bin less
	mkdir -p "$FAKE_HOME/.cargo/bin"
	printf '#!/bin/sh\nexit 0\n' >"$FAKE_HOME/.cargo/bin/bat"
	printf '#!/bin/sh\nexit 0\n' >"$FAKE_HOME/.cargo/bin/fd"
	chmod 755 "$FAKE_HOME/.cargo/bin/bat" "$FAKE_HOME/.cargo/bin/fd"
	local out
	out="$(run_rc_zsh_probe)"
	expect_field BAT_ALIAS "bat --paging=never" "$out"
	expect_field PAGER "bat --paging=always" "$out"
	[[ "$(field FZF_DEFAULT_COMMAND "$out")" == fd\ * ]] || fail "zsh FZF_DEFAULT_COMMAND not refreshed:
$out"
}

@test "re-source on batcat/fdfind-only host still resolves batcat/fdfind (bash)" {
	reset_bin batcat fdfind less
	local out
	out="$(run_resource_probe)"
	expect_field BAT_CMD batcat "$out"
	expect_field FD_CMD fdfind "$out"
	expect_field BAT_ALIAS "batcat --paging=never" "$out"
	expect_field LESS_ALIAS "batcat --paging=always" "$out"
	expect_field FD_ALIAS fdfind "$out"
	expect_field PAGER "batcat --paging=always" "$out"
	[[ "$(field FZF_DEFAULT_COMMAND "$out")" == fdfind\ * ]] || fail "re-sourced FZF_DEFAULT_COMMAND not fdfind:
$out"
}

@test "re-source on batcat/fdfind-only host still resolves batcat/fdfind (zsh)" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	reset_bin batcat fdfind less
	local out
	out="$(run_resource_zsh_probe)"
	expect_field BAT_CMD batcat "$out"
	expect_field FD_CMD fdfind "$out"
	expect_field BAT_ALIAS "batcat --paging=never" "$out"
	expect_field FD_ALIAS fdfind "$out"
	expect_field PAGER "batcat --paging=always" "$out"
}

@test "real bat/fd only: json/compare invoke bat and f invokes fd (bash)" {
	reset_bin bat-record fd-record less
	PROBE_FUNCS=1
	local out
	out="$(run_bash_probe)"
	grep -q '^bat -l json --paging=never$' "$STUB_LOG" || fail "json did not invoke bat:
$(cat "$STUB_LOG")"
	grep -q '^bat -l diff --paging=never$' "$STUB_LOG" || fail "compare did not invoke bat:
$(cat "$STUB_LOG")"
	grep -q '^fd file$' "$STUB_LOG" || fail "f did not invoke fd:
$(cat "$STUB_LOG")"
}

@test "real bat/fd only: json/compare invoke bat and f invokes fd (zsh)" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	reset_bin bat-record fd-record less
	local out
	out="$(run_zsh_funcs_probe)"
	grep -q '^bat -l json --paging=never$' "$STUB_LOG" || fail "zsh json did not invoke bat:
$(cat "$STUB_LOG")"
	grep -q '^bat -l diff --paging=never$' "$STUB_LOG" || fail "zsh compare did not invoke bat:
$(cat "$STUB_LOG")"
	grep -q '^fd file$' "$STUB_LOG" || fail "zsh f did not invoke fd:
$(cat "$STUB_LOG")"
}

@test "re-source on bat/fd-only host still resolves bat/fd (bash)" {
	reset_bin bat fd less
	local out
	out="$(run_resource_probe)"
	expect_field BAT_CMD bat "$out"
	expect_field FD_CMD fd "$out"
	expect_field PAGER "bat --paging=always" "$out"
	[[ "$(field FZF_DEFAULT_COMMAND "$out")" == fd\ * ]] || fail "re-sourced FZF_DEFAULT_COMMAND not fd:
$out"
}

@test "re-source on bat/fd-only host still resolves bat/fd (zsh)" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	reset_bin bat fd less
	local out
	out="$(run_resource_zsh_probe)"
	expect_field BAT_CMD bat "$out"
	expect_field FD_CMD fd "$out"
	expect_field PAGER "bat --paging=always" "$out"
}
