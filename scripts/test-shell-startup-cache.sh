#!/usr/bin/env bash
# Regression tests for shell startup cost (YAN-1339, YAN-1451):
#   - common/cached-eval.sh: tool init output (zoxide, atuin, starship, direnv,
#     thefuck) is generated once, sourced from the cache on later starts, and
#     regenerated only for the tool whose binary or config changed. Failures
#     are never cached; DOTFILES_NO_EVAL_CACHE=1 bypasses the cache.
#   - Cache trust: a group/world-writable cache dir or file is never sourced or
#     written; noclobber (zsh -o noclobber, bash -C) does not block writes.
#   - zsh runs exactly one compinit per start (counted by a wrapper installed
#     from the temp HOME's ~/.zshenv); ~/.zcompdump is not rewritten on an
#     unchanged startup; completion dirs from installers (~/.zfunc,
#     ~/.docker/completions, ~/.grok/completions/zsh) are registered and win.
#     A second home simulates Debian/Ubuntu's system-rc compinit: the dotfiles
#     re-run it once into ~/.zcompdump-dotfiles, which then stays stable.
# Every tool is a stub on a temp HOME's PATH that logs its argv, zsh runs with
# NO_GLOBAL_RCS, and `brew --prefix` is stubbed to an empty dir, so the results
# never depend on what this machine has installed or its /etc/zsh files.
# shellcheck disable=SC2016
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMP_DIR="$(cd "$(mktemp -d)" && pwd -P)"
trap '[[ -n "${KEEP_TMP:-}" ]] || rm -rf "$TMP_DIR"' EXIT
umask 022

USER_NAME="${USER:-$(id -un)}"
TOOLS_BASH=(zoxide atuin starship direnv)
TOOLS_ZSH=(zoxide atuin starship direnv thefuck)

# The stubs live in $HOME/bin, which common/paths.sh prepends ahead of the
# system dirs (including /usr/local/bin), so they shadow any real install. It is
# deliberately NOT on the starting PATH: prepend_path skips dirs already present.
# A `brew` stub (first on PATH) makes common/platform.sh's `brew --prefix`
# report an empty dir, so no host Homebrew completion dir lands on fpath.
mkdir -p "$TMP_DIR/fakebrew/bin" "$TMP_DIR/fakebrew/prefix"
printf '#!/bin/sh\necho "%s"\n' "$TMP_DIR/fakebrew/prefix" >"$TMP_DIR/fakebrew/bin/brew"
chmod 755 "$TMP_DIR/fakebrew/bin/brew"
TEST_PATH="$TMP_DIR/fakebrew/bin:/usr/bin:/bin:$(dirname "$(command -v bash)")"
if command -v zsh >/dev/null 2>&1; then
	TEST_PATH="$TEST_PATH:$(dirname "$(command -v zsh)")"
fi

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# Probe run inside each shell after startup; values between @@ delimiters.
PROBE='
for t in zoxide atuin starship direnv thefuck; do
	eval "printf \"@@MARK_%s=%s\n\" \"\$t\" \"\${dotf_stub_$t:-}\""
done
printf "@@LEAK=%s\n" "$(set | grep -c "^__dotf_ce")"
if [[ -n "${ZSH_VERSION:-}" ]]; then
	printf "@@COMP_z=%s\n" "${_comps[z]:-}"
	printf "@@COMP_%s=%s\n" rfmedia "${_comps[rfmedia]:-}" docker "${_comps[docker]:-}" \
		grok "${_comps[grok]:-}" op "${_comps[op]:-}" branchlet "${_comps[branchlet]:-}" \
		dotfnew2 "${_comps[dotfnew2]:-}"
	autoload +X _docker 2>/dev/null
	printf "@@DOCKERSRC=%s\n" "${functions_source[_docker]:-}"
fi
'

# write_stub <home> <tool> [fail]: logs "<tool> <argv>" and prints an init line
# defining dotf_stub_<tool>; "fail" prints nothing and exits 1.
write_stub() {
	local home="$1" tool="$2" mode="${3:-ok}" stub="$1/bin/$2"
	{
		printf '#!/bin/sh\n'
		printf 'printf "%%s %%s\\n" %s "$*" >>"%s/stub.log"\n' "$tool" "$home"
		if [[ "$mode" == fail ]]; then
			printf 'exit 1\n'
		else
			printf 'echo "dotf_stub_%s=loaded"\n' "$tool"
		fi
	} >"$stub"
	chmod 755 "$stub"
}

# make_home <dir>: temp HOME whose rc files point at this checkout, with every
# stub, an atuin config and stub completion files from three installers.
make_home() {
	local home="$1" tool
	mkdir -p "$home/bin" "$home/.config/atuin" "$home/.zfunc" \
		"$home/.docker/completions" "$home/.grok/completions/zsh"
	ln -s "$REPO_ROOT/bash/bashrc" "$home/.bashrc"
	ln -s "$REPO_ROOT/bash/bash_profile" "$home/.bash_profile"
	ln -s "$REPO_ROOT/zsh/zshrc" "$home/.zshrc"
	# ~/.zshenv (read even under NO_GLOBAL_RCS) wraps compinit so every call is
	# logged to ~/compinit.log with its arguments.
	cat >"$home/.zshenv" <<'EOF'
_dotf_test_wrap_compinit() {
	compinit() {
		print -r -- "compinit $*" >>"$HOME/compinit.log"
		unfunction compinit
		autoload -Uz compinit
		compinit "$@"
		local s=$?
		_dotf_test_wrap_compinit
		return $s
	}
}
_dotf_test_wrap_compinit
EOF
	for tool in "${TOOLS_ZSH[@]}"; do
		write_stub "$home" "$tool"
	done
	printf 'style = "compact"\n' >"$home/.config/atuin/config.toml"
	printf '#compdef rfmedia\n_message rfmedia\n' >"$home/.zfunc/_rfmedia"
	printf '#compdef docker\n_message docker\n' >"$home/.docker/completions/_docker"
	printf '#compdef grok\n_message grok\n' >"$home/.grok/completions/zsh/_grok"
}

# run_shell <home> <out> <shell args...>: one startup; stub.log is reset first
# so it records exactly this startup's tool runs. Extra env via RUN_ENV.
RUN_ENV=()
run_shell() {
	local home="$1" out="$2"
	shift 2
	: >"$home/stub.log"
	: >"$home/compinit.log"
	(
		cd "$home"
		env -i HOME="$home" PATH="$TEST_PATH" TERM=xterm-256color \
			DOTFILES_ROOT="$REPO_ROOT" USER="$USER_NAME" ${RUN_ENV[@]+"${RUN_ENV[@]}"} \
			"$@" "$PROBE" </dev/null >"$out.out" 2>"$out.err"
	) || fail "$* exited non-zero (stderr: $(<"$out.err"))"
	cp "$home/stub.log" "$out.log"
	cp "$home/compinit.log" "$out.compinit"
}

# expect_compinit <label> <out> <line...>: the exact compinit calls of a start.
expect_compinit() {
	local label="$1" out="$2" got want
	shift 2
	got="$(cat "$out.compinit")"
	want="$(printf '%s\n' "$@")"
	[[ "$got" == "$want" ]] || fail "$label: compinit calls = '$(tr '\n' ';' <"$out.compinit")', want '$(printf '%s;' "$@")'"
}

# Repo completions ($REPO_ROOT/zsh/completions) load only when compaudit
# accepts that dir: a checkout group-writable by a shared group (umask 002 with
# primary group staff/docker, as on CI runners) is skipped, never loaded.
REPO_COMP_OP=""
if command -v zsh >/dev/null 2>&1 &&
	[[ -z "$(zsh -fc 'autoload -Uz compaudit; compaudit "$1" 2>/dev/null' zsh "$REPO_ROOT/zsh/completions")" ]]; then
	REPO_COMP_OP=_op
fi

# expect_completions <label> <out> <home>
expect_completions() {
	local label="$1" out="$2" home="$3"
	expect "$label" COMP_rfmedia _rfmedia "$out"
	expect "$label" COMP_docker _docker "$out"
	expect "$label" COMP_grok _grok "$out"
	expect "$label" COMP_op "$REPO_COMP_OP" "$out"
	expect "$label" COMP_branchlet _branchlet "$out"
	# The user's ~/.docker/completions copy is the one that loads.
	expect "$label" DOCKERSRC "$home/.docker/completions/_docker" "$out"
}

# not_rewritten <label> <file...>: backdates the files; check_not_rewritten
# then fails when any of them became newer than the reference.
backdate() {
	touch -t 200102030405.06 "$@"
}
check_not_rewritten() {
	local label="$1" ref="$2" file
	shift 2
	for file in "$@"; do
		[[ "$file" -nt "$ref" ]] && fail "$label: $file rewritten"
	done
	return 0
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

# runs <out> <tool>: how many times the tool's stub ran during that startup.
runs() {
	local n
	n="$(grep -c "^$2 " "$1.log" || true)"
	printf '%s' "${n:-0}"
}

# expect_runs <label> <out> <want> <tool...>
expect_runs() {
	local label="$1" out="$2" want="$3" tool got
	shift 3
	for tool in "$@"; do
		got="$(runs "$out" "$tool")"
		[[ "$got" == "$want" ]] || fail "$label: $tool ran $got time(s), want $want (log: $(tr '\n' ';' <"$out.log"))"
	done
}

expect_markers() {
	local label="$1" out="$2" tool
	shift 2
	for tool in "$@"; do
		expect "$label" "MARK_$tool" loaded "$out"
	done
	expect "$label" LEAK 0 "$out"
}

test_shell() {
	local shell_cmd="$1" ext label tools_var
	label="${shell_cmd// /_}"
	if [[ "$shell_cmd" == zsh* ]]; then
		ext=zsh
		tools_var=("${TOOLS_ZSH[@]}")
	else
		ext=bash
		tools_var=("${TOOLS_BASH[@]}")
	fi
	local home="$TMP_DIR/$label/home" out="$TMP_DIR/$label/run" cache
	cache="$home/.cache/dotfiles/eval"
	mkdir -p "$TMP_DIR/$label"
	make_home "$home"

	# a. Cold start: each tool runs once, its init is in effect, cache written.
	# shellcheck disable=SC2086 # intentional word split of the shell command
	run_shell "$home" "$out.1" $shell_cmd
	expect_runs "$label cold" "$out.1" 1 "${tools_var[@]}"
	expect_markers "$label cold" "$out.1" "${tools_var[@]}"
	local tool
	for tool in "${tools_var[@]}"; do
		[[ -f "$cache/$tool.$ext" ]] || fail "$label cold: $cache/$tool.$ext not written"
	done
	[[ "$(find "$cache" -maxdepth 0 -perm 700 | wc -l)" -eq 1 ]] || fail "$label: $cache is not mode 700"

	# b. Warm start: no tool runs, init still in effect.
	# shellcheck disable=SC2086
	run_shell "$home" "$out.2" $shell_cmd
	expect_runs "$label warm" "$out.2" 0 "${tools_var[@]}"
	expect_markers "$label warm" "$out.2" "${tools_var[@]}"

	local ref="$TMP_DIR/$label/ref"
	if [[ "$ext" == zsh ]]; then
		# g. Exactly one compinit per start: the cold start builds the dump
		# (-i), later starts load it unaudited (-C), and an unchanged startup
		# reuses ~/.zcompdump (backdated with a reference file; a rewrite makes
		# it newer).
		expect_compinit "$label cold" "$out.1" "compinit -i"
		expect_compinit "$label warm" "$out.2" "compinit -C"
		local dump="$home/.zcompdump"
		[[ -f "$dump" ]] || fail "$label: $dump not created"
		backdate "$dump" "$ref"
		# shellcheck disable=SC2086
		run_shell "$home" "$out.3" $shell_cmd
		expect_compinit "$label unchanged" "$out.3" "compinit -C"
		check_not_rewritten "$label unchanged startup" "$ref" "$dump"

		# h. Installer completion dirs and repo/zshrc completions are registered.
		expect_completions "$label" "$out.3" "$home"

		# A newly installed completion file changes the count: the dump is
		# rebuilt (-i) and the new completion loads.
		printf '#compdef dotfnew\n_message new\n' >"$home/.zfunc/_dotfnew"
		# shellcheck disable=SC2086
		run_shell "$home" "$out.3b" $shell_cmd
		expect_compinit "$label new file" "$out.3b" "compinit -i"
		[[ "$dump" -nt "$ref" ]] || fail "$label: $dump not rebuilt after a new completion file"
		grep -q "'dotfnew' '_dotfnew'" "$dump" || fail "$label: _dotfnew missing from rebuilt $dump"
		rm "$home/.zfunc/_dotfnew"
	fi

	# c. New binary metadata (an upgrade) regenerates only that tool's cache.
	touch -t 200102030405.06 "$home/bin/zoxide"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.4" $shell_cmd
	expect_runs "$label zoxide upgraded" "$out.4" 1 zoxide
	expect_runs "$label zoxide upgraded" "$out.4" 0 "${tools_var[@]:1}"
	expect_markers "$label zoxide upgraded" "$out.4" "${tools_var[@]}"

	# d. Editing the atuin config regenerates only atuin's cache.
	printf 'style = "full"\n' >"$home/.config/atuin/config.toml"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.5" $shell_cmd
	expect_runs "$label atuin config" "$out.5" 1 atuin
	expect_runs "$label atuin config" "$out.5" 0 zoxide starship direnv
	expect_markers "$label atuin config" "$out.5" "${tools_var[@]}"

	# e. A failing tool is not cached (and a stale cache is removed), so the
	# next start runs it again.
	write_stub "$home" direnv fail
	# shellcheck disable=SC2086
	run_shell "$home" "$out.6" $shell_cmd
	expect_runs "$label direnv failing" "$out.6" 1 direnv
	[[ ! -e "$cache/direnv.$ext" ]] || fail "$label: failing direnv left $cache/direnv.$ext"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.7" $shell_cmd
	expect_runs "$label direnv failing again" "$out.7" 1 direnv
	expect_runs "$label direnv failing again" "$out.7" 0 zoxide atuin starship
	[[ -z "$(find "$cache" -name '.*' -type f)" ]] || fail "$label: temp files left in $cache"
	write_stub "$home" direnv

	# f. DOTFILES_NO_EVAL_CACHE=1 runs every tool on every start.
	RUN_ENV=(DOTFILES_NO_EVAL_CACHE=1)
	local n
	for n in 8 9; do
		# shellcheck disable=SC2086
		run_shell "$home" "$out.$n" $shell_cmd
		expect_runs "$label no-cache run $n" "$out.$n" 1 "${tools_var[@]}"
		expect_markers "$label no-cache run $n" "$out.$n" "${tools_var[@]}"
	done
	RUN_ENV=()

	# i. noclobber must not block cache writes: cold start into an empty cache
	# with the option on, then a warm start runs nothing.
	rm -rf "$cache"
	local noclobber_cmd
	if [[ "$ext" == zsh ]]; then noclobber_cmd="${shell_cmd/zsh /zsh -o noclobber }"; else noclobber_cmd="${shell_cmd/bash /bash -C }"; fi
	# shellcheck disable=SC2086
	run_shell "$home" "$out.10" $noclobber_cmd
	expect_runs "$label noclobber cold" "$out.10" 1 "${tools_var[@]}"
	for tool in "${tools_var[@]}"; do
		[[ -f "$cache/$tool.$ext" ]] || fail "$label noclobber: $cache/$tool.$ext not written"
	done
	# shellcheck disable=SC2086
	run_shell "$home" "$out.11" $noclobber_cmd
	expect_runs "$label noclobber warm" "$out.11" 0 "${tools_var[@]}"
	expect_markers "$label noclobber warm" "$out.11" "${tools_var[@]}"

	# j. A world-writable cache dir is never sourced or written: every tool runs
	# on every start, init still applies, and no file in it changes.
	chmod 777 "$cache"
	backdate "$cache"/* "$ref"
	for n in 12 13; do
		# shellcheck disable=SC2086
		run_shell "$home" "$out.$n" $shell_cmd
		expect_runs "$label 777 dir run $n" "$out.$n" 1 "${tools_var[@]}"
		expect_markers "$label 777 dir run $n" "$out.$n" "${tools_var[@]}"
	done
	check_not_rewritten "$label 777 dir" "$ref" "$cache"/*
	[[ -z "$(find "$cache" -name '.*' -type f)" ]] || fail "$label: temp file written into 777 $cache"
	chmod 700 "$cache"

	# k. A group-writable cache file is never sourced or replaced; the others
	# are still served from the cache.
	chmod 664 "$cache/zoxide.$ext"
	backdate "$cache/zoxide.$ext" "$ref"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.14" $shell_cmd
	expect_runs "$label 664 file" "$out.14" 1 zoxide
	expect_runs "$label 664 file" "$out.14" 0 "${tools_var[@]:1}"
	expect_markers "$label 664 file" "$out.14" "${tools_var[@]}"
	check_not_rewritten "$label 664 file" "$ref" "$cache/zoxide.$ext"
}

# A system zshrc that already ran compinit (Debian/Ubuntu /etc/zsh/zshrc without
# skip_global_compinit), simulated from the temp HOME's ~/.zshenv: the dotfiles
# re-run compinit once into ~/.zcompdump-dotfiles, which an unchanged startup
# then reuses, and completions still resolve.
#
# The simulated call is `compinit -u`, not Debian's bare `compinit`: a bare one
# depends on the host's default fpath being secure. Where it is not (GitHub's
# Ubuntu image runs `chmod -R 777 /usr/share`), it prompts, reads EOF, aborts
# and removes compdef, so the dotfiles fall back to their own single
# `compinit -i` and this case would never reach the re-run it exists for. -u
# skips only that audit; it still initializes from the default fpath.
test_system_compinit() {
	local shell_cmd="$1" label="system-compinit"
	local home="$TMP_DIR/$label/home" out="$TMP_DIR/$label/run" ref="$TMP_DIR/$label/ref"
	mkdir -p "$TMP_DIR/$label"
	make_home "$home"
	printf 'autoload -Uz compinit && compinit -u\n' >>"$home/.zshenv"
	local own="compinit -i -d $home/.zcompdump-dotfiles" own_warm="compinit -C -d $home/.zcompdump-dotfiles"

	# shellcheck disable=SC2086
	run_shell "$home" "$out.1" $shell_cmd
	expect_compinit "$label cold" "$out.1" "compinit -u" "$own"
	[[ -f "$home/.zcompdump-dotfiles" ]] || fail "$label: ~/.zcompdump-dotfiles not created"
	expect_completions "$label cold" "$out.1" "$home"

	backdate "$home/.zcompdump-dotfiles" "$home/.zcompdump" "$ref"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.2" $shell_cmd
	expect_compinit "$label warm" "$out.2" "compinit -u" "$own_warm"
	check_not_rewritten "$label unchanged startup" "$ref" "$home/.zcompdump-dotfiles" "$home/.zcompdump"
	expect_completions "$label warm" "$out.2" "$home"

	# A completion registered between the system compinit and the dotfiles
	# re-run with an already-defined function (zoxide's real init does
	# `compdef __zoxide_z_complete z`) must survive the re-run. The dump never
	# lists it (compinit builds the dump from fpath only), so every such start
	# depends on the dotfiles putting it back.
	cat >"$home/bin/zoxide" <<EOF
#!/bin/sh
printf "%s %s\\n" zoxide "\$*" >>"$home/stub.log"
echo 'dotf_stub_zoxide=loaded'
echo '__stub_z_complete() { :; }'
echo 'compdef __stub_z_complete z'
EOF
	chmod 755 "$home/bin/zoxide"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.3" $shell_cmd
	expect "$label zoxide compdef" COMP_z __stub_z_complete "$out.3"
	if grep -q "__stub_z_complete" "$home/.zcompdump-dotfiles"; then
		fail "$label: dump unexpectedly lists z; the case no longer exercises the restore"
	fi
	expect_completions "$label zoxide compdef" "$out.3" "$home"
}

# A group-writable checkout (umask 002, as on CI runners and many machines)
# whose group is not the user-private one: compaudit flags both
# $DOTFILES_ROOT/zsh/completions and $DOTFILES_ROOT/zsh. compinit -i alone then
# rewrote ~/.zcompdump on every start (common/completions.sh:_dotfiles_compinit
# explains why). The flagged repo completions must stay unloaded, everything
# else must load, and an unchanged startup must reuse the dump.
test_insecure_root() {
	local shell_cmd="$1" label="insecure-root"
	local home="$TMP_DIR/$label/home" out="$TMP_DIR/$label/run" ref="$TMP_DIR/$label/ref"
	local root="$TMP_DIR/$label/dotfiles"
	mkdir -p "$root"
	cp -R "$REPO_ROOT/bash" "$REPO_ROOT/common" "$REPO_ROOT/zsh" "$root/"
	chmod -R g+w "$root"
	# A user-private group passes compaudit; world write fails it on any host.
	[[ "$(id -gn)" == "$USER_NAME" ]] && chmod o+w "$root/zsh" "$root/zsh/completions"
	make_home "$home"
	ln -sf "$root/zsh/zshrc" "$home/.zshrc"

	local saved_root="$REPO_ROOT"
	REPO_ROOT="$root"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.1" $shell_cmd
	expect_compinit "$label cold" "$out.1" "compinit -i"
	[[ -f "$home/.zcompdump" ]] || fail "$label: ~/.zcompdump not created"
	backdate "$home/.zcompdump" "$ref"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.2" $shell_cmd
	REPO_ROOT="$saved_root"
	expect_compinit "$label warm" "$out.2" "compinit -C"
	check_not_rewritten "$label unchanged startup" "$ref" "$home/.zcompdump"
	expect "$label" COMP_op "" "$out.2"
	expect "$label" COMP_rfmedia _rfmedia "$out.2"
	expect "$label" COMP_docker _docker "$out.2"
	expect "$label" COMP_grok _grok "$out.2"
	expect "$label" COMP_branchlet _branchlet "$out.2"
}

# A secure completion dir under a world-writable parent (Homebrew's
# group-writable share/ holding zsh-completions on an admin-user Mac; GitHub's
# Ubuntu image, whose /usr/share/zsh is 777): compaudit reports only the
# parent. The dir must keep loading as with plain compinit -i, and an unchanged
# startup must reuse the dump (compinit's own check rewrote it every start).
test_insecure_parent() {
	local shell_cmd="$1" label="insecure-parent"
	local home="$TMP_DIR/$label/home" out="$TMP_DIR/$label/run" ref="$TMP_DIR/$label/ref"
	mkdir -p "$TMP_DIR/$label"
	make_home "$home"
	chmod o+w "$home/.grok/completions"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.1" $shell_cmd
	expect_compinit "$label cold" "$out.1" "compinit -i"
	expect_completions "$label cold" "$out.1" "$home"
	[[ -f "$home/.zcompdump" ]] || fail "$label: ~/.zcompdump not created"
	backdate "$home/.zcompdump" "$ref"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.2" $shell_cmd
	expect_compinit "$label warm" "$out.2" "compinit -C"
	check_not_rewritten "$label unchanged startup" "$ref" "$home/.zcompdump"
	expect_completions "$label warm" "$out.2" "$home"

	# A new file whose count delta equals the files under insecure-parent dirs
	# (one: _grok) makes compinit's own buggy count match the stale header; the
	# rebuild must still not load that stale dump, so dotfnew2 must register.
	printf '#compdef dotfnew2\n_message new\n' >"$home/.zfunc/_dotfnew2"
	# shellcheck disable=SC2086
	run_shell "$home" "$out.3" $shell_cmd
	expect_compinit "$label new file" "$out.3" "compinit -i"
	expect "$label new file" COMP_dotfnew2 _dotfnew2 "$out.3"
}

ZSH_CMD="zsh -o NO_GLOBAL_RCS -i -c"
SHELLS=("bash -i -c")
if command -v zsh >/dev/null 2>&1; then
	SHELLS+=("$ZSH_CMD")
else
	echo "SKIP: zsh not installed; zsh cases skipped"
fi

for shell_cmd in "${SHELLS[@]}"; do
	test_shell "$shell_cmd"
done
if command -v zsh >/dev/null 2>&1; then
	test_system_compinit "$ZSH_CMD"
	test_insecure_parent "$ZSH_CMD"
	test_insecure_root "$ZSH_CMD"
fi

echo "PASS: shell startup cache tests"
