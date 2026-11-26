# shellcheck shell=bash
# Regression tests for YAN-1421: an empty Fabric patterns directory must not
# trigger zsh NOMATCH or abort the rest of common/aliases.sh (helper cleanup
# and Linux/macOS alias loading). Also guards bash nullglob/failglob handling.

load test_helper

TEST_BASH="${TEST_BASH:-$(command -v bash)}"
[[ "$TEST_BASH" == /* ]] || TEST_BASH="$(command -v "$TEST_BASH")"
TEST_ZSH="$(command -v zsh 2>/dev/null || true)"
[[ -z "$TEST_ZSH" || "$TEST_ZSH" == /* ]] || TEST_ZSH="$(command -v "$TEST_ZSH")"

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	FAKE_HOME="$BATS_TEST_TMPDIR/home"
	mkdir -p "$FAKEBIN" "$FAKE_HOME"
	local tool path
	for tool in cat ls mkdir dirname basename tty locale tr grep awk sed id uname hostname stat find diff env sh; do
		path="$(type -P "$tool" 2>/dev/null || true)"
		[[ -n "$path" ]] && ln -sf "$path" "$FAKEBIN/$tool"
	done
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

run_zsh_aliases() {
	local os="$1" pkg="$2" extra_pre="${3:-}"
	local err_file="$BATS_TEST_TMPDIR/zsh.err"
	local script="$BATS_TEST_TMPDIR/probe.zsh"
	cat >"$script" <<EOF
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=$os DOTFILES_PKG_MGR=$pkg DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
$extra_pre
source "\$REPO_ROOT/common/platform.sh"
source "\$REPO_ROOT/common/aliases.sh"

fact() { printf '@@%s=%s\n' "\$1" "\$2"; }
alias_fact() {
	local name="\$1" key="\$2" value
	if value="\$(alias "\$name" 2>/dev/null)"; then
		value="\${value#alias }"
		value="\${value#"\$name"=}"
		value="\${value#\'}"
		value="\${value%\'}"
		fact "\$key" "\$value"
	else
		fact "\$key" "none"
	fi
}

alias_fact listinstalled LISTINSTALLED
alias_fact cpuinfo CPUINFO
alias_fact flushdns FLUSHDNS
alias_fact summarize SUMMARIZE
alias_fact ls LS_ALIAS
alias_fact fabric FABRIC_ALIAS
alias_fact bad-name BAD_NAME
alias_fact not_a_dir NOT_A_DIR
if typeset -f _fabric_load_pattern_aliases >/dev/null 2>&1; then
	fact HELPER_LOADER present
else
	fact HELPER_LOADER unset
fi
if typeset -f _fabric_alias_reserved >/dev/null 2>&1; then
	fact HELPER_RESERVED present
else
	fact HELPER_RESERVED unset
fi
[[ -o nomatch ]] && fact OPT_NOMATCH on || fact OPT_NOMATCH off
[[ -o null_glob ]] && fact OPT_NULLGLOB on || fact OPT_NULLGLOB off
EOF
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		"$TEST_ZSH" -f "$script" 2>"$err_file"
}

run_bash_aliases() {
	local os="$1" pkg="$2" extra_pre="${3:-}"
	local err_file="$BATS_TEST_TMPDIR/bash.err"
	local script="$BATS_TEST_TMPDIR/probe.bash"
	cat >"$script" <<EOF
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=$os DOTFILES_PKG_MGR=$pkg DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
$extra_pre
source "\$REPO_ROOT/common/platform.sh"
source "\$REPO_ROOT/common/aliases.sh"

fact() { printf '@@%s=%s\n' "\$1" "\$2"; }
alias_fact() {
	local name="\$1" key="\$2" value
	if value="\$(alias "\$name" 2>/dev/null)"; then
		value="\${value#alias }"
		value="\${value#"\$name"=}"
		value="\${value#\'}"
		value="\${value%\'}"
		fact "\$key" "\$value"
	else
		fact "\$key" "none"
	fi
}

alias_fact listinstalled LISTINSTALLED
alias_fact cpuinfo CPUINFO
alias_fact flushdns FLUSHDNS
alias_fact summarize SUMMARIZE
alias_fact ls LS_ALIAS
alias_fact fabric FABRIC_ALIAS
alias_fact bad-name BAD_NAME
alias_fact not_a_dir NOT_A_DIR
if declare -F _fabric_load_pattern_aliases >/dev/null 2>&1; then
	fact HELPER_LOADER present
else
	fact HELPER_LOADER unset
fi
if declare -F _fabric_alias_reserved >/dev/null 2>&1; then
	fact HELPER_RESERVED present
else
	fact HELPER_RESERVED unset
fi
shopt -q nullglob && fact OPT_NULLGLOB on || fact OPT_NULLGLOB off
shopt -q failglob && fact OPT_FAILGLOB on || fact OPT_FAILGLOB off
EOF
	env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=xterm-256color \
		REPO_ROOT="$REPO_ROOT" DOTFILES_ROOT="$REPO_ROOT" \
		"$TEST_BASH" "$script" 2>"$err_file"
}

@test "zsh: empty Fabric patterns dir does not abort aliases.sh and loads Linux aliases" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	mkdir -p "$FAKE_HOME/.config/fabric/patterns"
	local out
	out="$(run_zsh_aliases linux apt)"
	[ ! -s "$BATS_TEST_TMPDIR/zsh.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/zsh.err")"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field CPUINFO "lscpu" "$out"
	expect_field HELPER_LOADER unset "$out"
	expect_field HELPER_RESERVED unset "$out"
	expect_field OPT_NOMATCH on "$out"
	expect_field OPT_NULLGLOB off "$out"
}

@test "zsh: empty Fabric patterns dir still loads macOS aliases" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	mkdir -p "$FAKE_HOME/.config/fabric/patterns"
	local out
	out="$(run_zsh_aliases macos homebrew)"
	[ ! -s "$BATS_TEST_TMPDIR/zsh.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/zsh.err")"
	expect_field FLUSHDNS "sudo dscacheutil -flushcache && sudo killall -HUP mDNSResponder" "$out"
	expect_field CPUINFO "sysctl -n machdep.cpu.brand_string" "$out"
	expect_field HELPER_LOADER unset "$out"
	expect_field HELPER_RESERVED unset "$out"
}

@test "zsh: Fabric patterns dir with only dotfiles does not abort aliases.sh" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	mkdir -p "$FAKE_HOME/.config/fabric/patterns"
	: >"$FAKE_HOME/.config/fabric/patterns/.gitkeep"
	local out
	out="$(run_zsh_aliases linux apt)"
	[ ! -s "$BATS_TEST_TMPDIR/zsh.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/zsh.err")"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field HELPER_LOADER unset "$out"
	expect_field HELPER_RESERVED unset "$out"
}

@test "zsh: preserves caller null_glob option when already enabled" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	mkdir -p "$FAKE_HOME/.config/fabric/patterns"
	local out
	out="$(run_zsh_aliases linux apt "setopt null_glob")"
	[ ! -s "$BATS_TEST_TMPDIR/zsh.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/zsh.err")"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field OPT_NULLGLOB on "$out"
}

@test "zsh: non-empty Fabric patterns dir loads valid patterns and platform aliases" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	mkdir -p \
		"$FAKE_HOME/.config/fabric/patterns/summarize" \
		"$FAKE_HOME/.config/fabric/patterns/ls" \
		"$FAKE_HOME/.config/fabric/patterns/fabric" \
		"$FAKE_HOME/.config/fabric/patterns/bad-name"
	: >"$FAKE_HOME/.config/fabric/patterns/not_a_dir"
	local out ls_alias
	out="$(run_zsh_aliases linux apt)"
	[ ! -s "$BATS_TEST_TMPDIR/zsh.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/zsh.err")"
	expect_field SUMMARIZE "fabric --pattern summarize" "$out"
	ls_alias="$(field LS_ALIAS "$out")"
	[[ -n "$ls_alias" && "$ls_alias" != *'--pattern'* ]] || fail "pre-existing ls alias overwritten: '$ls_alias'"
	expect_field FABRIC_ALIAS none "$out"
	expect_field BAD_NAME none "$out"
	expect_field NOT_A_DIR none "$out"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field HELPER_LOADER unset "$out"
	expect_field HELPER_RESERVED unset "$out"
}

@test "zsh -i: interactive startup with empty Fabric patterns dir is quiet and loads platform aliases" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	local tool
	mkdir -p "$FAKE_HOME/bin" "$FAKE_HOME/.config/fabric/patterns" "$BATS_TEST_TMPDIR/fakebrew/prefix"
	for tool in starship direnv atuin zoxide mise fzf thefuck carapace eza; do
		printf '#!/bin/sh\nexit 0\n' >"$FAKEBIN/$tool"
		chmod 755 "$FAKEBIN/$tool"
	done
	printf '#!/bin/sh\necho "%s"\n' "$BATS_TEST_TMPDIR/fakebrew/prefix" >"$FAKEBIN/brew"
	chmod 755 "$FAKEBIN/brew"
	ln -s "$REPO_ROOT/zsh/zshrc" "$FAKE_HOME/.zshrc"
	ln -s "$REPO_ROOT/zsh/zprofile" "$FAKE_HOME/.zprofile"
	printf 'skip_global_compinit=1\n' >"$FAKE_HOME/.zshenv"

	local err_file="$BATS_TEST_TMPDIR/zsh-i.err"
	local out
	# shellcheck disable=SC2016 # expanded inside the child zsh
	out="$(env -i HOME="$FAKE_HOME" PATH="$FAKEBIN:/usr/bin:/bin:$(dirname "$TEST_ZSH")" \
		TERM=xterm-256color DOTFILES_ROOT="$REPO_ROOT" USER="${USER:-test}" \
		"$TEST_ZSH" -i -c '
			alias cpuinfo >/dev/null && echo "@@CPUINFO_OK=1" || echo "@@CPUINFO_OK=0"
			typeset -f _fabric_load_pattern_aliases >/dev/null 2>&1 && echo "@@HELPER_LOADER=present" || echo "@@HELPER_LOADER=unset"
		' </dev/null 2>"$err_file")"
	[ ! -s "$err_file" ] || fail "zsh -i wrote to stderr: $(cat "$err_file")"
	expect_field CPUINFO_OK 1 "$out"
	expect_field HELPER_LOADER unset "$out"
}

@test "bash: empty Fabric patterns dir loads Linux and macOS aliases and preserves nullglob off" {
	mkdir -p "$FAKE_HOME/.config/fabric/patterns"
	local out
	out="$(run_bash_aliases linux apt)"
	[ ! -s "$BATS_TEST_TMPDIR/bash.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/bash.err")"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field CPUINFO "lscpu" "$out"
	expect_field HELPER_LOADER unset "$out"
	expect_field HELPER_RESERVED unset "$out"
	expect_field OPT_NULLGLOB off "$out"
	expect_field OPT_FAILGLOB off "$out"

	out="$(run_bash_aliases macos homebrew)"
	[ ! -s "$BATS_TEST_TMPDIR/bash.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/bash.err")"
	expect_field FLUSHDNS "sudo dscacheutil -flushcache && sudo killall -HUP mDNSResponder" "$out"
	expect_field HELPER_LOADER unset "$out"
}

@test "bash: preserves nullglob on and tolerates failglob on when Fabric patterns dir is empty" {
	mkdir -p "$FAKE_HOME/.config/fabric/patterns"
	local out
	out="$(run_bash_aliases linux apt "shopt -s nullglob")"
	[ ! -s "$BATS_TEST_TMPDIR/bash.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/bash.err")"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field OPT_NULLGLOB on "$out"

	out="$(run_bash_aliases linux apt "shopt -s failglob")"
	[ ! -s "$BATS_TEST_TMPDIR/bash.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/bash.err")"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field HELPER_LOADER unset "$out"
	expect_field OPT_FAILGLOB on "$out"
	expect_field OPT_NULLGLOB off "$out"
}

@test "bash: non-empty Fabric patterns dir loads valid patterns and platform aliases" {
	mkdir -p \
		"$FAKE_HOME/.config/fabric/patterns/summarize" \
		"$FAKE_HOME/.config/fabric/patterns/ls" \
		"$FAKE_HOME/.config/fabric/patterns/fabric" \
		"$FAKE_HOME/.config/fabric/patterns/bad-name"
	: >"$FAKE_HOME/.config/fabric/patterns/not_a_dir"
	local out ls_alias
	out="$(run_bash_aliases linux apt)"
	[ ! -s "$BATS_TEST_TMPDIR/bash.err" ] || fail "unexpected stderr: $(cat "$BATS_TEST_TMPDIR/bash.err")"
	expect_field SUMMARIZE "fabric --pattern summarize" "$out"
	ls_alias="$(field LS_ALIAS "$out")"
	[[ -n "$ls_alias" && "$ls_alias" != *'--pattern'* ]] || fail "pre-existing ls alias overwritten: '$ls_alias'"
	expect_field FABRIC_ALIAS none "$out"
	expect_field BAD_NAME none "$out"
	expect_field NOT_A_DIR none "$out"
	expect_field LISTINSTALLED "apt list --installed" "$out"
	expect_field HELPER_LOADER unset "$out"
	expect_field HELPER_RESERVED unset "$out"
}
