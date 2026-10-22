#!/usr/bin/env bash
# Regression tests for common/local.sh, the machine-local overrides hook that
# bash/bashrc and zsh/zshrc source last. For bash -i, bash -l -i and zsh -i it
# checks: load order, PATH priority, raw secret token exports, umask, aliases,
# top-level sourcing (typeset/setopt persist), zsh fpath pickup, the ownership
# and permission guards, a custom XDG_CONFIG_HOME, login bash's ~/.extra still
# overriding bashrc, and that a machine without overrides gets no hook output.
# Fixture files are written with literal $VAR text, expanded by the shell under test.
# shellcheck disable=SC2016
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMP_DIR="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_DIR"' EXIT
# Fixtures must not be group-writable, or the permission guard skips them.
umask 022

USER_NAME="${USER:-$(id -un)}"

# Minimal PATH: the shells under test plus the base system, never the host's
# tool dirs, so the results do not depend on what this machine has installed.
TEST_PATH="/usr/bin:/bin:$(dirname "$(command -v bash)")"
if command -v zsh >/dev/null 2>&1; then
	TEST_PATH="$TEST_PATH:$(dirname "$(command -v zsh)")"
fi

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# Probe run inside each shell after startup. Values are printed between
# delimiters so unrelated startup noise on stdout is ignored.
PROBE='
printf "@@ORDER=%s\n" "${DOTF_TEST_ORDER:-}"
printf "@@PATH1=%s\n" "${PATH%%:*}"
printf "@@OP=%s|\n" "${OP_SERVICE_ACCOUNT_TOKEN:-}"
printf "@@GEMINI=%s|\n" "${GEMINI_API_KEY:-}"
printf "@@OPENAI=%s|\n" "${OPENAI_API_KEY:-}"
printf "@@ANTHROPIC=%s|\n" "${ANTHROPIC_API_KEY:-}"
u="$(umask)"; while [[ ${#u} -lt 4 ]]; do u="0$u"; done; printf "@@UMASK=%s\n" "$u"
printf "@@ALIAS=%s\n" "$(alias dotftestalias 2>/dev/null)"
printf "@@TYPESET=%s\n" "${DOTF_TEST_TYPESET:-}"
printf "@@SETOPT=%s\n" "$( [[ -n "${ZSH_VERSION:-}" ]] && [[ -o interactive_comments ]] && [[ ! -o auto_cd ]] && echo yes || echo no)"
printf "@@HELPERS=%s\n" "$(typeset -f _dotf_local_add _dotf_local_collect 2>/dev/null | wc -l | tr -d " ")${_dotf_local_files+set}${_dotf_local_fpath+set}"
printf "@@EXTRA=%s\n" "${DOTF_TEST_EXTRA:-}"
printf "@@EXTRAALIAS=%s\n" "$(alias dotfextraalias 2>/dev/null)"
printf "@@COMP=%s\n" "$( [[ -n "${ZSH_VERSION:-}" ]] && printf "%s" "${_comps[dotftest]:-}")"
printf "@@PATHALL=%s\n" "$PATH"
'

# make_home <dir>: temp HOME whose rc files point at this checkout.
make_home() {
	local home="$1"
	mkdir -p "$home"
	ln -s "$REPO_ROOT/bash/bashrc" "$home/.bashrc"
	ln -s "$REPO_ROOT/bash/bash_profile" "$home/.bash_profile"
	ln -s "$REPO_ROOT/zsh/zshrc" "$home/.zshrc"
}

# populate <home> <config_dir>: every override file appends its name to
# DOTF_TEST_ORDER so the final value records the load order.
populate() {
	local home="$1" config="$2" name
	mkdir -p "$config/dotfiles.local" "$config/secrets" "$home/localbin"
	for name in env paths aliases secrets; do
		printf 'DOTF_TEST_ORDER="${DOTF_TEST_ORDER:-}%s,"\n' "$name" >"$config/dotfiles.local/$name.sh"
	done
	# A global declaration must survive: proves top-level (not function) sourcing.
	printf 'typeset -x DOTF_TEST_TYPESET=kept\n' >>"$config/dotfiles.local/env.sh"
	printf 'export PATH="%s/localbin:$PATH"\n' "$home" >>"$config/dotfiles.local/paths.sh"
	printf "alias dotftestalias='echo hi'\n" >>"$config/dotfiles.local/aliases.sh"
	for name in a b; do
		printf 'DOTF_TEST_ORDER="${DOTF_TEST_ORDER:-}secrets/%s,"\n' "$name" >"$config/secrets/$name.sh"
	done
	printf 'op-token\n' >"$config/secrets/1password-service-account"
	printf 'gemini-key\n' >"$config/secrets/gemini_api_key"
	printf 'openai-key' >"$config/secrets/openai_api_key"
	printf 'anthropic-key\n' >"$config/secrets/anthropic_api_key"
	# zsh: a completion dir added to fpath by an override must be picked up.
	mkdir -p "$config/comp"
	printf '#compdef dotftest\n_message hi\n' >"$config/comp/_dotftest"
	printf '[[ -n "${ZSH_VERSION:-}" ]] && fpath=("%s/comp" $fpath)\n' "$config" >>"$config/dotfiles.local/env.sh"
	# Legacy per-shell files load last; the zsh one also flips options.
	printf 'DOTF_TEST_ORDER="${DOTF_TEST_ORDER:-}legacy,"\n' >"$home/.bashrc.local"
	printf 'DOTF_TEST_ORDER="${DOTF_TEST_ORDER:-}legacy,"\nsetopt interactive_comments\nunsetopt auto_cd\n' >"$home/.zshrc.local"
	# Login bash sources ~/.extra after bashrc, so it must still win.
	printf 'export DOTF_TEST_EXTRA=extra\nalias dotfextraalias="echo x"\n' >"$home/.extra"
}

# run_shell <home> <out_prefix> <xdg-or-empty> <shell args...>
# Writes <out_prefix>.out / .err. Starts from umask 077 to prove the reset.
run_shell() {
	local home="$1" out="$2" xdg="$3"
	shift 3
	local -a envs=(HOME="$home" PATH="$TEST_PATH" TERM=xterm-256color DOTFILES_ROOT="$REPO_ROOT" USER="$USER_NAME")
	[[ -n "$xdg" ]] && envs+=(XDG_CONFIG_HOME="$xdg")
	(
		cd "$home"
		umask 077
		env -i "${envs[@]}" "$@" "$PROBE" </dev/null >"$out.out" 2>"$out.err"
	) || fail "$* exited non-zero (stderr: $(<"$out.err"))"
}

field() {
	local out="$1" key="$2" line
	line="$(grep -a "^@@$key=" "$out.out" | tail -n 1)" || fail "missing @@$key in $out.out"
	printf '%s' "${line#@@"$key"=}"
}

expect() {
	local label="$1" key="$2" want="$3" out="$4" got
	got="$(field "$out" "$key")"
	[[ "$got" == "$want" ]] || fail "$label: $key = '$got', want '$want'"
}

SHELLS=("bash -i -c" "bash -l -i -c")
if command -v zsh >/dev/null 2>&1; then
	SHELLS+=("zsh -i -c")
else
	echo "SKIP: zsh not installed; zsh cases skipped"
fi

# A root-owned file for the ownership guard (impossible to test as root).
ROOT_FILE=""
if [[ "$(id -u)" -ne 0 ]]; then
	for candidate in /etc/hostname /etc/passwd; do
		if [[ -f "$candidate" && ! -O "$candidate" ]]; then
			ROOT_FILE="$candidate"
			break
		fi
	done
fi
[[ -n "$ROOT_FILE" ]] || echo "SKIP: no foreign-owned file available; ownership guard case skipped"

test_full() {
	local label="$1" xdg_mode="$2" shell_cmd="$3"
	local home="$TMP_DIR/$label/home" config xdg="" out="$TMP_DIR/$label/run"
	mkdir -p "$TMP_DIR/$label"
	make_home "$home"
	if [[ "$xdg_mode" == custom ]]; then
		xdg="$TMP_DIR/$label/xdg"
		config="$xdg"
		# Decoy in the default location must be ignored when XDG_CONFIG_HOME is set.
		mkdir -p "$home/.config/dotfiles.local"
		printf 'DOTF_TEST_ORDER="${DOTF_TEST_ORDER:-}decoy,"\n' >"$home/.config/dotfiles.local/env.sh"
	else
		config="$home/.config"
	fi
	populate "$home" "$config"
	if [[ -n "$ROOT_FILE" ]]; then
		ln -s "$ROOT_FILE" "$config/secrets/zz.sh"
	fi

	# shellcheck disable=SC2086 # intentional word split of the shell command
	run_shell "$home" "$out" "$xdg" $shell_cmd

	local ctx="$label ($shell_cmd)"
	expect "$ctx" ORDER "env,paths,aliases,secrets,secrets/a,secrets/b,legacy," "$out"
	expect "$ctx" PATH1 "$home/localbin" "$out"
	expect "$ctx" OP "op-token|" "$out"
	expect "$ctx" GEMINI "gemini-key|" "$out"
	expect "$ctx" OPENAI "openai-key|" "$out"
	expect "$ctx" ANTHROPIC "anthropic-key|" "$out"
	expect "$ctx" UMASK "0022" "$out"
	[[ "$(field "$out" ALIAS)" == *dotftestalias* ]] || fail "$ctx: alias from aliases.sh not defined"
	expect "$ctx" TYPESET "kept" "$out"
	if [[ "$shell_cmd" == zsh* ]]; then
		expect "$ctx" SETOPT "yes" "$out"
		expect "$ctx" COMP "_dotftest" "$out"
	fi
	if [[ "$shell_cmd" == zsh* && "$xdg_mode" == default ]]; then
		# The hook's compinit uses its own dump; a second startup with the same
		# fpath must reuse it rather than rewrite it.
		local dump="$home/.zcompdump-local"
		[[ -f "$dump" ]] || fail "$ctx: $dump not created"
		sleep 1
		touch "$TMP_DIR/$label/marker"
		# shellcheck disable=SC2086
		run_shell "$home" "$out.2" "$xdg" $shell_cmd
		[[ "$dump" -nt "$TMP_DIR/$label/marker" ]] && fail "$ctx: $dump regenerated on an unchanged startup"
	fi
	if [[ "$shell_cmd" == "bash -l -i -c" ]]; then
		expect "$ctx" EXTRA "extra" "$out"
		[[ "$(field "$out" EXTRAALIAS)" == *dotfextraalias* ]] || fail "$ctx: alias from ~/.extra lost"
	fi
	expect "$ctx" HELPERS "0" "$out"
	if [[ -n "$ROOT_FILE" ]]; then
		grep -qF "dotfiles: skipping $config/secrets/zz.sh (not owned by $USER_NAME)" "$out.err" ||
			fail "$ctx: ownership warning missing (stderr: $(<"$out.err"))"
		# Sourcing it would surface as an error naming zz.sh (e.g. the hostname
		# run as a command); only the guard's own warning may mention it.
		if grep -a 'zz\.sh' "$out.err" | grep -qv '^dotfiles: skipping'; then
			fail "$ctx: foreign-owned zz.sh was sourced (stderr: $(<"$out.err"))"
		fi
	fi
}

test_empty() {
	local shell_cmd="$1" label="empty-${1// /_}"
	local home="$TMP_DIR/$label/home" out="$TMP_DIR/$label/run"
	mkdir -p "$TMP_DIR/$label"
	make_home "$home"
	# shellcheck disable=SC2086
	run_shell "$home" "$out" "" $shell_cmd
	if grep -aE 'local\.sh|dotfiles:|no matches found' "$out.err"; then
		fail "empty $shell_cmd: hook wrote to stderr"
	fi
	expect "empty $shell_cmd" ORDER "" "$out"
	expect "empty $shell_cmd" UMASK "0022" "$out"
	expect "empty $shell_cmd" HELPERS "0" "$out"
}

# World-writable override file, and a world-writable secrets/ directory: both
# must be skipped with the warning, everything else still loads.
test_perms() {
	local shell_cmd="$1" label="perms-${1// /_}"
	local home="$TMP_DIR/$label/home" out="$TMP_DIR/$label/run" config
	mkdir -p "$TMP_DIR/$label"
	make_home "$home"
	config="$home/.config"
	populate "$home" "$config"
	chmod o+w "$config/dotfiles.local/aliases.sh" "$config/secrets"
	# shellcheck disable=SC2086
	run_shell "$home" "$out" "" $shell_cmd

	local ctx="$label ($shell_cmd)"
	expect "$ctx" ORDER "env,paths,secrets,legacy," "$out"
	expect "$ctx" OP "|" "$out"
	expect "$ctx" ALIAS "" "$out"
	local f
	for f in dotfiles.local/aliases.sh secrets/1password-service-account secrets/a.sh secrets/b.sh; do
		grep -qF "dotfiles: skipping $config/$f (group/world-writable)" "$out.err" ||
			fail "$ctx: no warning for $f (stderr: $(<"$out.err"))"
	done
	if grep -a 'dotfiles: skipping' "$out.err" | grep -qE '\.bashrc\.local|\.zshrc\.local|env\.sh'; then
		fail "$ctx: safe file wrongly skipped (stderr: $(<"$out.err"))"
	fi
}

# Group write: allowed when the group is the user-private group (umask 002 on
# Ubuntu gives 664 files / 775 dirs), rejected for any other group.
IS_UPG=""
[[ "$(id -gn)" == "$USER_NAME" ]] && IS_UPG=1
test_group_write() {
	local shell_cmd="$1" label="group-${1// /_}"
	local home="$TMP_DIR/$label/home" out="$TMP_DIR/$label/run" config
	mkdir -p "$TMP_DIR/$label"
	make_home "$home"
	config="$home/.config"
	populate "$home" "$config"
	chmod 664 "$home/.bashrc.local" "$home/.zshrc.local" "$config/secrets/a.sh" "$config/secrets/b.sh"
	chmod 775 "$config/secrets"
	# shellcheck disable=SC2086
	run_shell "$home" "$out" "" $shell_cmd

	local ctx="$label ($shell_cmd)"
	if [[ -n "$IS_UPG" ]]; then
		expect "$ctx" ORDER "env,paths,aliases,secrets,secrets/a,secrets/b,legacy," "$out"
		expect "$ctx" OP "op-token|" "$out"
		if grep -aq 'dotfiles: skipping' "$out.err"; then
			fail "$ctx: user-private-group file skipped (stderr: $(<"$out.err"))"
		fi
	else
		expect "$ctx" ORDER "env,paths,aliases,secrets," "$out"
		grep -qF "dotfiles: skipping $config/secrets/a.sh (group/world-writable)" "$out.err" ||
			fail "$ctx: shared-group write not rejected (stderr: $(<"$out.err"))"
	fi
}

# Non-interactive login bash: bashrc returns early, so bash_profile's own
# installer PATH lines (hard-coded CodeRabbit ~/.local/bin) must still apply.
test_login_noninteractive() {
	local home="$TMP_DIR/login-c/home" out="$TMP_DIR/login-c/run"
	mkdir -p "$TMP_DIR/login-c"
	make_home "$home"
	run_shell "$home" "$out" "" bash -l -c
	[[ ":$(field "$out" PATHALL):" == *":/home/yandy/.local/bin:"* ]] ||
		fail "bash -l -c: bash_profile CodeRabbit PATH line not effective (PATH=$(field "$out" PATHALL))"
}

[[ -n "$IS_UPG" ]] || echo "NOTE: primary group is not user-private; testing group-write rejection instead"
test_login_noninteractive
for shell_cmd in "${SHELLS[@]}"; do
	test_full "default-${shell_cmd// /_}" default "$shell_cmd"
	test_full "xdg-${shell_cmd// /_}" custom "$shell_cmd"
	test_empty "$shell_cmd"
	test_perms "$shell_cmd"
	test_group_write "$shell_cmd"
done

echo "PASS: shell local hook tests"
