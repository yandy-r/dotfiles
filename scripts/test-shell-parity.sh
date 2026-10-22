#!/usr/bin/env bash
# Regression tests for bash/zsh parity: both shells must get the same aliases,
# the same tool hooks and a quiet startup from the shared common/ files.
# For bash -i, bash -l -i and zsh -i (zsh cases skipped when zsh is absent)
# it checks:
#   1. startup stderr is empty, with linked and fallback fzf integration
#   2. the edit / mtr / rg aliases exist in both shells
#   3. the thefuck alias exists and carapace's `_carapace` ran exactly once
#   4. fzf key-bindings load via ~/.fzf.<shell> and via the repo fallback
#   5. the Python venv auto-activation hook (cd in/out, nested dir, switching
#      between auto venvs, manual venvs survive, unsafe activate file ignored,
#      DOTFILES_NO_VENV_AUTO=1, first-prompt activation, local PATH survives,
#      manual reactivation survives leaving the tree, bash keeps $?)
#   6. Fabric pattern aliases (YAN-1548: no `declare -A` on bash 3.2) without
#      overwriting an existing alias
#   7. ~/.cargo/env is sourced exactly once per shell
# External tools are fakes copied into each temp HOME/bin; setup_user_paths
# prepends them ahead of host tools. A brew stub hides host Homebrew prefixes.
#
# TEST_BASH (optional): absolute path of the bash binary under test; defaults
# to `command -v bash`. Only the shells under test use it, the driver itself
# needs bash >= 4. CI sets TEST_BASH=/bin/bash on macOS to cover bash 3.2.
# shellcheck disable=SC2016
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMP_DIR="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_DIR"' EXIT
# Fixtures must not be group-writable, or the venv permission guard skips them.
umask 022

USER_NAME="${USER:-$(id -un)}"

TEST_BASH="${TEST_BASH:-$(command -v bash)}"
[[ "$TEST_BASH" == /* && -x "$TEST_BASH" ]] || {
	echo "FAIL: TEST_BASH must be an absolute path to an executable bash (got '$TEST_BASH')" >&2
	exit 1
}
echo "bash under test: $TEST_BASH ($("$TEST_BASH" -c 'echo "$BASH_VERSION"'))"
# Carapace is skipped on bash < 4 (its `source <(…)` loader reads nothing there).
TEST_BASH_MAJOR="$("$TEST_BASH" -c 'echo "${BASH_VERSINFO[0]}"')"

# Keep HOME/bin off the starting PATH: setup_user_paths only prepends new dirs.
# The brew stub hides host Homebrew tools and completion directories.
mkdir -p "$TMP_DIR/fakebrew/bin" "$TMP_DIR/fakebrew/prefix"
printf '#!/bin/sh\necho "%s"\n' "$TMP_DIR/fakebrew/prefix" >"$TMP_DIR/fakebrew/bin/brew"
chmod 755 "$TMP_DIR/fakebrew/bin/brew"
TEST_PATH="$TMP_DIR/fakebrew/bin:/usr/bin:/bin:$(dirname "$TEST_BASH")"
HAVE_ZSH=""
if command -v zsh >/dev/null 2>&1; then
	HAVE_ZSH=1
	TEST_PATH="$TEST_PATH:$(dirname "$(command -v zsh)")"
else
	echo "SKIP: zsh not installed; zsh cases skipped"
fi
FAKE_BIN="$TMP_DIR/fakebin"

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# Fake tools. Each prints just enough for the dotfiles hooks to wire them up.
mkdir -p "$FAKE_BIN"
cat >"$FAKE_BIN/fzf" <<'EOF'
#!/bin/sh
case "$1" in
--version) echo "0.60.0 (fake)" ;;
--bash) echo '__fzf_history__() { :; }' ;;
--zsh) cat <<'ZSH'
fzf-file-widget() { :; }
fzf-completion() { :; }
zle -N fzf-completion
bindkey '^I' fzf-completion
ZSH
;;
esac
EOF
cat >"$FAKE_BIN/thefuck" <<'EOF'
#!/bin/sh
[ "$1" = "--alias" ] && echo "alias fuck='echo f'"
exit 0
EOF
cat >"$FAKE_BIN/carapace" <<'EOF'
#!/bin/sh
if [ "$1" = "_carapace" ]; then
	echo run >>"$HOME/carapace-count"
	echo '_dotf_fake_carapace() { :; }'
fi
exit 0
EOF
printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/mtr"
printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/rg"
# Shadow optional host initialization tools without installing real hooks.
for tool in starship direnv atuin zoxide mise; do
	printf '#!/bin/sh\nexit 0\n' >"$FAKE_BIN/$tool"
done
chmod +x "$FAKE_BIN"/*

# Probe run inside each shell after startup. Values are printed between
# delimiters so unrelated startup noise on stdout is ignored.
PROBE='
printf "@@EDIT=%s\n" "$(alias edit 2>/dev/null)"
printf "@@MTR=%s\n" "$(alias mtr 2>/dev/null)"
printf "@@RG=%s\n" "$(alias rg 2>/dev/null)"
printf "@@FUCK=%s\n" "$(alias fuck 2>/dev/null)"
printf "@@CARAPACE=%s\n" "$(typeset -f _dotf_fake_carapace >/dev/null 2>&1 && echo yes || echo no)"
printf "@@FZFBASH=%s\n" "$(typeset -f __fzf_history__ >/dev/null 2>&1 && echo yes || echo no)"
printf "@@FZFZSH=%s\n" "$(typeset -f fzf-file-widget >/dev/null 2>&1 && echo yes || echo no)"
if [[ -n "${ZSH_VERSION:-}" ]]; then
	printf "@@FZFTAB=%s\n" "$(bindkey "^I")"
fi
printf "@@SUMMARIZE=%s\n" "$(alias summarize 2>/dev/null)"
printf "@@LS=%s\n" "$(alias ls 2>/dev/null)"
'

# make_home <dir> [fzf]: temp HOME whose rc files point at this checkout. With
# "fzf" the ~/.fzf.* links `dotf link` creates are present too.
make_home() {
	local home="$1"
	mkdir -p "$home/bin"
	cp "$FAKE_BIN"/* "$home/bin/"
	ln -s "$REPO_ROOT/bash/bashrc" "$home/.bashrc"
	ln -s "$REPO_ROOT/bash/bash_profile" "$home/.bash_profile"
	ln -s "$REPO_ROOT/zsh/zshrc" "$home/.zshrc"
	# Debian/Ubuntu's /etc/zsh/zshrc runs a bare compinit unless this is set; on
	# GitHub's Ubuntu image (/usr/share/zsh is 777) it aborts with an insecure-
	# directories prompt, which is host noise, not dotfiles startup output.
	printf 'skip_global_compinit=1\n' >"$home/.zshenv"
	if [[ "${2:-}" == fzf ]]; then
		ln -s "$REPO_ROOT/bash/.fzf.bash" "$home/.fzf.bash"
		ln -s "$REPO_ROOT/zsh/fzf.zsh" "$home/.fzf.zsh"
	fi
}

# Knobs for run_shell, reset by the caller after use.
RUN_PATH="$TEST_PATH"
RUN_CWD=""
RUN_ENV=()

# run_shell <home> <out_prefix> <stdin-file|-> <shell args...>
# Writes <out_prefix>.out / .err. With a stdin file the shell reads its
# commands from it (so bash's PROMPT_COMMAND fires between lines); otherwise
# the shell args carry their own -c and run with stdin on /dev/null.
# A leading `bash` is run as "$TEST_BASH", never resolved through the PATH.
run_shell() {
	local home="$1" out="$2" stdin="$3"
	shift 3
	local -a cmd=("$@")
	[[ "${cmd[0]}" == bash ]] && cmd[0]="$TEST_BASH"
	local -a envs=(HOME="$home" PATH="$RUN_PATH" TERM=xterm-256color DOTFILES_ROOT="$REPO_ROOT" USER="$USER_NAME")
	[[ ${#RUN_ENV[@]} -gt 0 ]] && envs+=("${RUN_ENV[@]}")
	[[ "$stdin" == - ]] && stdin=/dev/null
	(
		cd "${RUN_CWD:-$home}"
		umask 077
		env -i "${envs[@]}" "${cmd[@]}" <"$stdin" >"$out.out" 2>"$out.err"
	) || fail "${cmd[*]} exited non-zero (stderr: $(<"$out.err"))"
	# bash -i without a controlling terminal always says this; it is not startup
	# noise. Neither is bash 3.2 readline's meta-mode escape (ESC[?1034h).
	local err
	err="$(grep -av -e '^bash: cannot set terminal process group' -e '^bash: no job control in this shell' "$out.err")" || true
	err="${err//$'\e[?1034h'/}"
	if [[ -n "$err" ]]; then printf '%s\n' "$err" >"$out.err"; else : >"$out.err"; fi
}

# probe_shell <home> <out_prefix> <shell args...>: run PROBE via -c.
probe_shell() {
	local home="$1" out="$2"
	shift 2
	run_shell "$home" "$out" - "$@" "$PROBE"
}

field() {
	local out="$1" key="$2" line
	line="$(grep -a "^@@$key=" "$out.out" | tail -n 1)" || fail "missing @@$key in $out.out (stderr: $(<"$out.err"))"
	printf '%s' "${line#@@"$key"=}"
}

expect() {
	local label="$1" key="$2" want="$3" out="$4" got
	got="$(field "$out" "$key")"
	[[ "$got" == "$want" ]] || fail "$label: $key = '$got', want '$want'"
}

expect_contains() {
	local label="$1" key="$2" want="$3" out="$4" got
	got="$(field "$out" "$key")"
	[[ "$got" == *"$want"* ]] || fail "$label: $key = '$got', want it to contain '$want'"
}

# Interactive shells under test for the -c cases: label, args.
SHELLS=("bash -i -c" "bash -l -i -c")
[[ -n "$HAVE_ZSH" ]] && SHELLS+=("zsh -i -c")
# Stdin-driven shells for the venv cases.
STDIN_SHELLS=("bash -i")
[[ -n "$HAVE_ZSH" ]] && STDIN_SHELLS+=("zsh -i")

# new_case <label>: fresh HOME (with ~/.fzf.* links when $2 is "fzf"); prints
# its path.
new_case() {
	local label="$1" home="$TMP_DIR/$1/home"
	mkdir -p "$TMP_DIR/$label"
	make_home "$home" "${2:-}"
	printf '%s' "$home"
}

slug() { local s="$*"; printf '%s' "${s//[^A-Za-z0-9]/_}"; }

# 1. Whole startup is silent on stderr with linked and fallback fzf integration.
test_quiet() {
	local shell_cmd="$1" home out
	# shellcheck disable=SC2086
	set -- $shell_cmd
	for mode in linked fallback; do
		if [[ "$mode" == linked ]]; then
			home="$(new_case "quiet-$mode-$(slug "$shell_cmd")" fzf)"
		else
			home="$(new_case "quiet-$mode-$(slug "$shell_cmd")")"
		fi
		out="$TMP_DIR/quiet-$mode-$(slug "$shell_cmd")/run"
		run_shell "$home" "$out" - "$@" ':'
		[[ ! -s "$out.err" ]] || fail "$shell_cmd ($mode): startup wrote to stderr: $(<"$out.err")"
	done
}

# 2-4. Aliases, thefuck, carapace, fzf (via the ~/.fzf.* links and without).
test_tools() {
	local shell_cmd="$1" label home out
	# shellcheck disable=SC2086
	set -- $shell_cmd
	local kind="$1"
	for fzf_mode in linked fallback; do
		label="tools-$fzf_mode-$(slug "$shell_cmd")"
		if [[ "$fzf_mode" == linked ]]; then home="$(new_case "$label" fzf)"; else home="$(new_case "$label")"; fi
		out="$TMP_DIR/$label/run"
		probe_shell "$home" "$out" "$@"
		expect_contains "$label" EDIT 'EDITOR' "$out"
		expect_contains "$label" MTR 'sudo mtr' "$out"
		expect_contains "$label" RG '--smart-case' "$out"
		expect_contains "$label" RG '--hidden' "$out"
		expect_contains "$label" FUCK 'echo f' "$out"
		local want_carapace=yes want_runs=1
		if [[ "$kind" == bash && "$TEST_BASH_MAJOR" -lt 4 ]]; then
			want_carapace=no want_runs=0
		fi
		expect "$label" CARAPACE "$want_carapace" "$out"
		if [[ "$kind" == bash ]]; then
			expect "$label" FZFBASH yes "$out"
		else
			expect "$label" FZFZSH yes "$out"
			expect "$label" FZFTAB '"^I" fzf-completion' "$out"
		fi
		local runs=0
		[[ -f "$home/carapace-count" ]] && runs="$(wc -l <"$home/carapace-count" | tr -d ' ')"
		[[ "$runs" == "$want_runs" ]] || fail "$label: _carapace ran $runs times, want $want_runs"
	done
}

# 5. Python venv auto-activation. Model activate's PATH save/restore as well as
# VIRTUAL_ENV, so startup ordering and local PATH loss are observable.
make_venv() {
	local dir="$1"
	mkdir -p "$dir/.venv/bin"
	{
		printf 'VIRTUAL_ENV=%q\n' "$dir/.venv"
		cat <<'EOF'
export VIRTUAL_ENV
_OLD_VIRTUAL_PATH="$PATH"
PATH="$VIRTUAL_ENV/bin:$PATH"
export PATH
deactivate() {
	PATH="$_OLD_VIRTUAL_PATH"
	export PATH
	unset _OLD_VIRTUAL_PATH VIRTUAL_ENV
	unset -f deactivate
}
EOF
	} >"$dir/.venv/bin/activate"
}

# venv_session <label> <shell args> -- <script body>: runs the body as stdin
# of the shell started in RUN_CWD, prints the out prefix.
venv_session() {
	local label="$1" shell_cmd="$2" body="$3" home="$4"
	local out="$TMP_DIR/$label/run" script="$TMP_DIR/$label/stdin.sh"
	printf '%s\n' "$body" >"$script"
	# shellcheck disable=SC2086
	run_shell "$home" "$out" "$script" $shell_cmd
	printf '%s' "$out"
}

test_venv() {
	local shell_cmd="$1" slugged home label out
	slugged="$(slug "$shell_cmd")"
	local kind="${shell_cmd%% *}"
	# Deliberately include jj and a space: bash's vi binding must never see paths
	# in the stdin command text, only in environment variable expansions.
	local proj="$TMP_DIR/venv-$slugged/proj jj" proj2="$TMP_DIR/venv-$slugged/proj2"
	local other="$TMP_DIR/venv-$slugged/other" unsafe="$TMP_DIR/venv-$slugged/unsafe"
	mkdir -p "$proj/sub" "$proj2"
	make_venv "$proj"
	make_venv "$proj2"
	make_venv "$other"
	make_venv "$unsafe"
	chmod g+w "$unsafe/.venv/bin/activate"
	local P="$proj/.venv" Q="$proj2/.venv" O="$other/.venv"
	local -a venv_env=(T_PROJ="$proj" T_PROJ2="$proj2" T_OTHER="$other" T_UNSAFE="$unsafe")
	RUN_ENV=("${venv_env[@]}")
	# Leading \n in snapshots: the terminal-title escape leaves the cursor mid-line.

	# Walk in and out, nested dir, switching between two auto venvs.
	label="venv-walk-$slugged"
	home="$(new_case "$label")"
	out="$(venv_session "$label" "$shell_cmd" '
cd "$T_PROJ"
printf "\n@@V1=%s\n" "${VIRTUAL_ENV:-}"
cd "$T_PROJ/sub"
printf "\n@@V2=%s\n" "${VIRTUAL_ENV:-}"
cd "$T_PROJ2"
printf "\n@@V3=%s\n" "${VIRTUAL_ENV:-}"
cd /
printf "\n@@V4=%s\n" "${VIRTUAL_ENV:-}"
' "$home")"
	expect "$label" V1 "$P" "$out"
	expect "$label" V2 "$P" "$out"
	expect "$label" V3 "$Q" "$out"
	expect "$label" V4 "" "$out"

	# A venv the user activated by hand is never touched.
	label="venv-manual-$slugged"
	home="$(new_case "$label")"
	out="$(venv_session "$label" "$shell_cmd" '
. "$T_OTHER/.venv/bin/activate"
cd "$T_PROJ"
printf "\n@@V1=%s\n" "${VIRTUAL_ENV:-}"
cd /
printf "\n@@V2=%s\n" "${VIRTUAL_ENV:-}"
' "$home")"
	expect "$label" V1 "$O" "$out"
	expect "$label" V2 "$O" "$out"

	# Once the hook has observed a manual deactivate, reactivating the same venv
	# must not revive stale auto ownership when the user leaves the tree again.
	label="venv-reactivate-$slugged"
	home="$(new_case "$label")"
	out="$(venv_session "$label" "$shell_cmd" '
cd "$T_PROJ"
printf "\n@@V1=%s\n" "${VIRTUAL_ENV:-}"
deactivate
cd /
. "$T_PROJ/.venv/bin/activate"
cd "$T_PROJ/sub"
cd /
printf "\n@@V2=%s\n" "${VIRTUAL_ENV:-}"
' "$home")"
	expect "$label" V1 "$P" "$out"
	expect "$label" V2 "$P" "$out"

	# Starting inside a project waits until the first prompt. Machine-local PATH
	# additions must happen before activate saves PATH, or deactivate loses them.
	label="venv-start-$slugged"
	home="$(new_case "$label")"
	mkdir -p "$home/local-marker"
	cat >"$home/.bashrc.local" <<'EOF'
PATH="$HOME/local-marker:$PATH"
export PATH
EOF
	cp "$home/.bashrc.local" "$home/.zshrc.local"
	RUN_CWD="$proj/sub"
	out="$TMP_DIR/$label/before-prompt"
	# -c has no prompt, so it must not activate a venv at all.
	# shellcheck disable=SC2086
	run_shell "$home" "$out" - $shell_cmd -c 'printf "@@VSTART=%s\n" "${VIRTUAL_ENV:-}"'
	expect "$label" VSTART "" "$out"
	out="$(venv_session "$label" "$shell_cmd" '
printf "\n@@V1=%s\n" "${VIRTUAL_ENV:-}"
deactivate
printf "\n@@V2=%s\n" "${VIRTUAL_ENV:-}"
cd /
case ":$PATH:" in
*":$HOME/local-marker:"*) printf "\n@@MARKER=yes\n" ;;
*) printf "\n@@MARKER=no\n" ;;
esac
' "$home")"
	RUN_CWD=""
	expect "$label" V1 "$P" "$out"
	expect "$label" V2 "" "$out"
	expect "$label" MARKER yes "$out"

	# An activate file other users could have written is never sourced.
	label="venv-unsafe-$slugged"
	home="$(new_case "$label")"
	out="$(venv_session "$label" "$shell_cmd" '
cd "$T_UNSAFE"
printf "\n@@V1=%s\n" "${VIRTUAL_ENV:-}"
' "$home")"
	expect "$label" V1 "" "$out"

	# DOTFILES_NO_VENV_AUTO=1 disables the hook, at startup and on cd.
	label="venv-disabled-$slugged"
	home="$(new_case "$label")"
	RUN_ENV=("${venv_env[@]}" DOTFILES_NO_VENV_AUTO=1)
	RUN_CWD="$proj/sub"
	out="$(venv_session "$label" "$shell_cmd" '
printf "\n@@V1=%s\n" "${VIRTUAL_ENV:-}"
cd "$T_PROJ"
printf "\n@@V2=%s\n" "${VIRTUAL_ENV:-}"
' "$home")"
	RUN_CWD=""
	RUN_ENV=("${venv_env[@]}")
	expect "$label" V1 "" "$out"
	expect "$label" V2 "" "$out"

	# Capture the status immediately after the hook, not bash's restored $?.
	if [[ "$kind" == bash ]]; then
		label="venv-status-$slugged"
		home="$(new_case "$label")"
		out="$(venv_session "$label" "$shell_cmd" '
PROMPT_COMMAND="_dotf_venv_prompt_hook; _t_rc=\$?"
cd "$T_PROJ"; false
printf "\n@@RC=%s\n" "$_t_rc"
cd "$T_PROJ2"; false
printf "\n@@RC2=%s\n" "$_t_rc"
' "$home")"
		expect "$label" RC 1 "$out"
		expect "$label" RC2 1 "$out"
		# Re-sourcing the config must not stack a second hook.
		label="venv-hookonce-$slugged"
		home="$(new_case "$label")"
		out="$(venv_session "$label" "$shell_cmd" '
unset DOTFILES_BASHRC_LOADED
. "$HOME/.bashrc"
printf "\n@@PC=%s\n" "$PROMPT_COMMAND"
' "$home")"
		local pc count
		pc="$(field "$out" PC)"
		count="$(grep -o '_dotf_venv_prompt_hook' <<<"$pc" | wc -l | tr -d ' ')"
		[[ "$count" == 1 ]] || fail "$label: PROMPT_COMMAND has $count _dotf_venv_prompt_hook entries ('$pc'), want 1"
		# Without starship, bashrc falls back to bash/prompt.sh (sourced here
		# directly: a host starship may sit ahead of the fakes on PATH). Its
		# build_prompt must join PROMPT_COMMAND without dropping the venv hook.
		label="venv-noprompt-$slugged"
		home="$(new_case "$label")"
		out="$(venv_session "$label" "$shell_cmd" '
PROMPT_COMMAND="_dotf_venv_prompt_hook"
. "$DOTFILES_ROOT/bash/prompt.sh"
cd "$T_PROJ"
printf "\n@@V1=%s\n" "${VIRTUAL_ENV:-}"
printf "\n@@PC=%s\n" "$PROMPT_COMMAND"
' "$home")"
		expect "$label" V1 "$P" "$out"
		# bash 3.2 gets prompt.sh's static PS1 and no build_prompt.
		if [[ "$TEST_BASH_MAJOR" -ge 4 ]]; then
			expect "$label" PC "build_prompt; _dotf_venv_prompt_hook" "$out"
		else
			expect "$label" PC "_dotf_venv_prompt_hook" "$out"
		fi
	fi
	RUN_ENV=()
}

# 6. Fabric pattern aliases: created, existing aliases kept, silent on bash 3.2.
test_fabric() {
	local shell_cmd="$1" label home out
	# shellcheck disable=SC2086
	set -- $shell_cmd
	label="fabric-$(slug "$shell_cmd")"
	home="$(new_case "$label")"
	mkdir -p "$home/.config/fabric/patterns/summarize" "$home/.config/fabric/patterns/ls"
	out="$TMP_DIR/$label/run"
	probe_shell "$home" "$out" "$@"
	expect_contains "$label" SUMMARIZE '--pattern summarize' "$out"
	local ls_alias
	ls_alias="$(field "$out" LS)"
	[[ -n "$ls_alias" && "$ls_alias" != *'--pattern'* ]] || fail "$label: pre-existing ls alias overwritten: '$ls_alias'"
	[[ ! -s "$out.err" ]] || fail "$label: startup wrote to stderr: $(<"$out.err")"
}

# 7. ~/.cargo/env is sourced exactly once.
test_cargo() {
	local shell_cmd="$1" label home out count
	# shellcheck disable=SC2086
	set -- $shell_cmd
	label="cargo-$(slug "$shell_cmd")"
	home="$(new_case "$label")"
	mkdir -p "$home/.cargo"
	printf 'echo sourced >>"$HOME/cargo-count"\n' >"$home/.cargo/env"
	out="$TMP_DIR/$label/run"
	run_shell "$home" "$out" - "$@" ':'
	count="$(wc -l <"$home/cargo-count" 2>/dev/null | tr -d ' ' || true)"
	[[ "${count:-0}" == 1 ]] || fail "$label: ~/.cargo/env sourced ${count:-0} times, want 1"
}

for shell_cmd in "${SHELLS[@]}"; do
	test_quiet "$shell_cmd"
	test_tools "$shell_cmd"
	test_fabric "$shell_cmd"
	test_cargo "$shell_cmd"
done
for shell_cmd in "${STDIN_SHELLS[@]}"; do
	test_venv "$shell_cmd"
done

echo "PASS: shell parity tests"
