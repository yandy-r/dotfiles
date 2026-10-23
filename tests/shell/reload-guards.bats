# shellcheck shell=bash
# Tests for the per-process rc reload guards in bash/bashrc, bash/bash_profile
# and bash/profile (YAN-1346):
#   - a nested `bash -i` (or `bash -l`) started from an interactive bash that
#     already ran the rc files still loads its own config (the guards are
#     per-PID and never exported, so no inherited value can block a child);
#   - re-sourcing the same rc file inside one shell stays a no-op;
#   - an inherited *exported* stale value from an older dotfiles release does
#     not block the child either.
# Every case runs under env -i with a temp HOME whose rc files symlink the
# repo (the links `dotf link` creates), so nothing depends on the host.

load test_helper

TEST_BASH="${TEST_BASH:-$(command -v bash)}"

setup() {
	HERMETIC_HOME="$BATS_TEST_TMPDIR/home"
	FAKEBIN="$BATS_TEST_TMPDIR/fakebin"
	mkdir -p "$HERMETIC_HOME/bin" "$FAKEBIN"
	local tool
	for tool in starship direnv atuin zoxide mise fzf thefuck carapace eza; do
		printf '#!/bin/sh\nexit 0\n' >"$FAKEBIN/$tool"
	done
	# brew stub with an empty prefix: no host Homebrew dirs on PATH (macOS).
	mkdir -p "$BATS_TEST_TMPDIR/fakebrew"
	printf '#!/bin/sh\necho "%s"\n' "$BATS_TEST_TMPDIR/fakebrew" >"$FAKEBIN/brew"
	chmod 755 "$FAKEBIN/"*
	# Same layout as scripts/test-shell-startup-smoke.sh: paths.sh prepends
	# /usr/local/bin ahead of the starting PATH, but then $HOME/bin (not on the
	# starting PATH) ahead of that, so the fakes there win over host tools.
	cp "$FAKEBIN"/* "$HERMETIC_HOME/bin/"
	ln -s "$REPO_ROOT/bash/bashrc" "$HERMETIC_HOME/.bashrc"
	ln -s "$REPO_ROOT/bash/bash_profile" "$HERMETIC_HOME/.bash_profile"
	ln -s "$REPO_ROOT/bash/profile" "$HERMETIC_HOME/.profile"
	TEST_ENV=(HOME="$HERMETIC_HOME" PATH="$FAKEBIN:/usr/bin:/bin" \
		TERM=xterm-256color DOTFILES_ROOT="$REPO_ROOT" USER="${USER:-$(id -un)}" \
		TEST_BASH="$TEST_BASH")
}

# hermetic_bash <script>: run the bash under test interactively under env -i;
# merges stderr in (bash's no-tty notices are harmless here).
hermetic_bash() {
	env -i "${TEST_ENV[@]}" "$TEST_BASH" --noediting -i "$1" 2>&1
}

# field <key> <output>: print the last @@key=value line, value only. bash 3.2's
# readline writes its meta-mode escape (ESC[?1034h) to stdout ahead of the
# first line, so strip it before matching the line anchor.
field() {
	local esc=$'\e[?1034h'
	grep -a "^@@$1=" <<<"${2//"$esc"/}" | tail -n 1 | cut -d= -f2-
}

@test "nested bash -i from an interactive bash loads its aliases" {
	local script="$BATS_TEST_TMPDIR/nested.sh" out
	cat >"$script" <<'EOF'
"$TEST_BASH" -i -c 'alias ll >/dev/null 2>&1 && echo "@@CHILD_LL=yes" || echo "@@CHILD_LL=no"
echo "@@CHILD_ALIASES=$(alias | wc -l)"'
EOF
	out="$(hermetic_bash "$script")"
	[ "$(field CHILD_LL "$out")" = yes ] || fail "nested bash -i has no ll alias:
$out"
	local child_aliases
	child_aliases="$(field CHILD_ALIASES "$out" | tr -d '[:space:]')"
	[ "${child_aliases:-0}" -gt 0 ] || fail "nested bash -i loaded ${child_aliases:-0} aliases, want > 0:
$out"
}

@test "nested bash -l from an interactive bash still gets DOTFILES_OS" {
	local script="$BATS_TEST_TMPDIR/nested-login.sh" out os
	cat >"$script" <<'EOF'
env -u DOTFILES_OS "$TEST_BASH" -l -c '[ -n "${DOTFILES_OS:-}" ] && echo "@@CHILD_OS=$DOTFILES_OS" || echo "@@CHILD_OS=empty"'
EOF
	out="$(hermetic_bash "$script")"
	os="$(field CHILD_OS "$out")"
	[ -n "$os" ] && [ "$os" != empty ] || fail "nested bash -l DOTFILES_OS empty:
$out"
}

@test "inherited exported guards from an older release do not block the child" {
	local script="$BATS_TEST_TMPDIR/stale.sh" out
	cat >"$script" <<'EOF'
export DOTFILES_BASHRC_LOADED=1
export DOTFILES_BASH_PROFILE_LOADED=1
export DOTFILES_PROFILE_LOADED=1
"$TEST_BASH" -i -c 'alias ll >/dev/null 2>&1 && echo "@@CHILD_LL=yes" || echo "@@CHILD_LL=no"'
EOF
	out="$(hermetic_bash "$script")"
	[ "$(field CHILD_LL "$out")" = yes ] || fail "stale exported guard blocked the child:
$out"
}

@test "exported stale guard does not block exec bash -i in the same process" {
	local stage2="$HERMETIC_HOME/stage2.sh" script="$BATS_TEST_TMPDIR/exec.sh" out
	cat >"$stage2" <<'EOF'
exec "$TEST_BASH" -i -c 'alias ll >/dev/null 2>&1 && echo "@@EXEC_LL=yes" || echo "@@EXEC_LL=no"'
EOF
	cat >"$script" <<'EOF'
DOTFILES_BASHRC_LOADED=1 "$TEST_BASH" --noediting -i "$HOME/stage2.sh"
EOF
	out="$(hermetic_bash "$script")"
	[ "$(field EXEC_LL "$out")" = yes ] || fail "exported stale guard blocked exec bash -i:
$out"
}

@test "re-sourcing ~/.bashrc in the same shell is still a no-op" {
	local script="$BATS_TEST_TMPDIR/resource.sh" out
	cat >"$script" <<'EOF'
before=$PROMPT_COMMAND
. "$HOME/.bashrc"
[ "$PROMPT_COMMAND" = "$before" ] && echo "@@NOOP=yes" || echo "@@NOOP=no"
EOF
	out="$(hermetic_bash "$script")"
	[ "$(field NOOP "$out")" = yes ] || fail "re-sourcing ~/.bashrc re-ran setup:
$out"
}

@test "re-sourcing the login rc files in the same shell adds nothing" {
	local script="$BATS_TEST_TMPDIR/resource-login.sh" out
	cat >"$script" <<'EOF'
. "$HOME/.bash_profile"
. "$HOME/.profile"
p1=$PATH
. "$HOME/.bash_profile"
. "$HOME/.profile"
[ "$PATH" = "$p1" ] && echo "@@PATH_STABLE=yes" || echo "@@PATH_STABLE=no"
EOF
	out="$(hermetic_bash "$script")"
	[ "$(field PATH_STABLE "$out")" = yes ] || fail "re-sourcing login rc files re-ran setup:
$out"
}
