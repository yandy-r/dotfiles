#!/usr/bin/env bash
# Regression tests for the bash prompt and terminal title:
#   - bash/prompt.sh must render untrusted values (git branch, venv name)
#     literally and never execute them during PS1 expansion.
#   - bash/bashrc's terminal title must follow $PWD and survive a startup
#     directory whose name contains a single quote.
set -euo pipefail

# ${PS1@P} (prompt expansion of a string) needs bash 4.4+.
if ((BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 4))); then
	echo "SKIP: bash prompt tests need bash >= 4.4"
	exit 0
fi

# Keep the developer's git config (signing, hooks) out of the fixture repo.
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
# Physical path: env -i drops PWD, so bash -i reports the resolved directory
# (e.g. macOS /var -> /private/var) and the title assertion must match it.
TMP_DIR="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# Render the fallback prompt the way bash does at display time: run
# PROMPT_COMMAND, then apply prompt expansion to PS1.
render_prompt() {
	local repo="$1" venv="$2"
	(
		cd "$repo"
		env -i HOME="$TMP_DIR" PATH="$PATH" TERM=xterm-256color VIRTUAL_ENV="$venv" \
			bash --norc --noprofile -c '
				source "$1/bash/prompt.sh"
				eval "$PROMPT_COMMAND"
				printf "%s" "${PS1@P}"
			' _ "$REPO_ROOT"
	)
}

test_prompt_values_are_literal() {
	local repo="$TMP_DIR/repo" branch rendered
	git init -q "$repo"
	git -C "$repo" -c user.name=t -c user.email=t@t commit -q --allow-empty -m init

	# Harmless expansions: if bash re-expanded these, the rendered prompt would
	# show 42 / an empty substitution instead of the literal text.
	# shellcheck disable=SC2016 # literal fixtures, must not expand here
	for branch in 'x$((6+36))' 'y`echo`z'; do
		git -C "$repo" checkout -q -b "$branch"
		# shellcheck disable=SC2016
		rendered="$(render_prompt "$repo" "$TMP_DIR/"'v$((6+36))')"
		[[ "$rendered" == *"git:${branch}"* ]] || fail "branch not rendered literally: $branch -> $rendered"
		# shellcheck disable=SC2016
		[[ "$rendered" == *'(v$((6+36)))'* ]] || fail "venv not rendered literally: $rendered"
	done
}

test_title_tracks_pwd() {
	local start="$TMP_DIR/it's" out
	mkdir -p "$start"
	out="$(
		cd "$start"
		printf 'cd /usr\nexit\n' |
			env -i HOME="$TMP_DIR" PATH=/usr/bin:/bin TERM=xterm USER=tester HOSTNAME=box \
				DOTFILES_ROOT="$REPO_ROOT" bash --noprofile --rcfile "$REPO_ROOT/bash/bashrc" -i 2>&1
	)"
	[[ "$out" != *"unexpected EOF"* ]] || fail "PROMPT_COMMAND broke in a directory containing a quote"
	[[ "$out" == *$'\e]0;tester@box: /usr\a'* ]] || fail "title did not follow cd /usr"
	[[ "$out" == *$'\e]0;tester@box: '"$start"$'\a'* ]] || fail "title missing for startup dir"
}

test_prompt_values_are_literal
test_title_tracks_pwd
echo "PASS: bash prompt tests"
