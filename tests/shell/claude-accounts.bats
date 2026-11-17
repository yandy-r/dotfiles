# shellcheck shell=bash
# Tests for bin/claude-accounts.sh: sharing one Claude Code config dir across
# several account dirs (CLAUDE_CONFIG_DIR) while each keeps its own login.
# Hermetic: HOME, the main dir, the accounts root and the backup dir all live in
# the test tmpdir.

load test_helper

SCRIPT="$REPO_ROOT/bin/claude-accounts.sh"

setup() {
	export HOME="$BATS_TEST_TMPDIR/home"
	export CLAUDE_MAIN_DIR="$HOME/.claude"
	export CLAUDE_ACCOUNTS_ROOT="$HOME/.config"
	export CLAUDE_ACCOUNTS_BACKUP_DIR="$BATS_TEST_TMPDIR/backups"
	export NO_COLOR=1
	MAIN="$CLAUDE_MAIN_DIR"
	ACCT="$CLAUDE_ACCOUNTS_ROOT/claude-work"
	mkdir -p "$MAIN/plugins/cache" "$MAIN/skills/a" "$MAIN/projects/-p/" "$CLAUDE_ACCOUNTS_ROOT"
	echo '{"main":true}' >"$MAIN/settings.json"
	echo main-rules >"$MAIN/CLAUDE.md"
	echo '{}' >"$MAIN/plugins/installed_plugins.json"
	echo '{"display":"m1","timestamp":10}' >"$MAIN/history.jsonl"
	echo '{"display":"m2","timestamp":30}' >>"$MAIN/history.jsonl"
	echo main-session >"$MAIN/projects/-p/s1.jsonl"
}

# make_account creates a cloned account dir like a manual `cp -a ~/.claude`.
make_account() {
	mkdir -p "$ACCT/projects/-p" "$ACCT/plugins"
	echo '{"acct":true}' >"$ACCT/settings.json"
	echo '{"token":"secret"}' >"$ACCT/.credentials.json"
	echo '{"oauthAccount":{"emailAddress":"w@x"}}' >"$ACCT/.claude.json"
	echo '{"old":true}' >"$ACCT/plugins/installed_plugins.json"
	echo '{"display":"m1","timestamp":10}' >"$ACCT/history.jsonl"
	echo '{"display":"a1","timestamp":20}' >>"$ACCT/history.jsonl"
	echo acct-session >"$ACCT/projects/-p/s2.jsonl"
}

@test "link creates a new account with symlinks into main" {
	run "$SCRIPT" link fresh
	[ "$status" -eq 0 ]
	local d="$CLAUDE_ACCOUNTS_ROOT/claude-fresh"
	[ "$(readlink "$d/settings.json")" = "$MAIN/settings.json" ]
	[ "$(readlink "$d/plugins")" = "$MAIN/plugins" ]
	[ "$(readlink "$d/projects")" = "$MAIN/projects" ]
	[ "$(readlink "$d/history.jsonl")" = "$MAIN/history.jsonl" ]
	# session dirs missing in main are created there so the link is valid
	[ -d "$MAIN/todos" ] && [ -L "$d/todos" ]
	# config missing in main is not linked
	[ ! -e "$d/commands" ] && [ ! -L "$d/commands" ]
	[[ "$output" == *"not logged in yet"* ]]
}

@test "link on an existing account keeps the login, merges sessions, backs up config" {
	make_account
	run "$SCRIPT" link work
	[ "$status" -eq 0 ]
	# login untouched
	[ "$(cat "$ACCT/.credentials.json")" = '{"token":"secret"}' ]
	[ ! -L "$ACCT/.claude.json" ]
	# config linked, old copy backed up
	[ -L "$ACCT/settings.json" ]
	grep -q acct "$CLAUDE_ACCOUNTS_BACKUP_DIR"/work-*/settings.json
	grep -q old "$CLAUDE_ACCOUNTS_BACKUP_DIR"/work-*/plugins/installed_plugins.json
	# account sessions merged into main
	[ "$(cat "$MAIN/projects/-p/s2.jsonl")" = acct-session ]
	[ "$(cat "$MAIN/projects/-p/s1.jsonl")" = main-session ]
	# history merged, de-duplicated, ordered by timestamp
	run jq -r .display "$MAIN/history.jsonl"
	[ "$output" = "$(printf 'm1\na1\nm2')" ]
}

@test "link is idempotent and repairs a link replaced by a regular file" {
	"$SCRIPT" link work >/dev/null
	run "$SCRIPT" link work
	[ "$status" -eq 0 ]
	[[ "$output" == *"ok     settings.json"* ]]
	rm "$ACCT/settings.json"
	echo '{"drift":true}' >"$ACCT/settings.json"
	run "$SCRIPT" status work
	[[ "$output" == *"settings.json"*"local (differs)"* ]]
	"$SCRIPT" link work >/dev/null
	[ -L "$ACCT/settings.json" ]
	grep -q drift "$CLAUDE_ACCOUNTS_BACKUP_DIR"/work-*/settings.json
}

@test "--no-sessions leaves session data per account" {
	make_account
	"$SCRIPT" link --no-sessions work >/dev/null
	[ -L "$ACCT/settings.json" ]
	[ ! -L "$ACCT/projects" ] && [ -f "$ACCT/projects/-p/s2.jsonl" ]
	[ ! -L "$ACCT/history.jsonl" ]
	[ ! -e "$MAIN/projects/-p/s2.jsonl" ]
}

@test "--copy copies main into the account and keeps account extras" {
	make_account
	"$SCRIPT" link --copy work >/dev/null
	[ ! -L "$ACCT/settings.json" ]
	[ "$(cat "$ACCT/settings.json")" = '{"main":true}' ]
	[ -f "$ACCT/projects/-p/s1.jsonl" ] && [ -f "$ACCT/projects/-p/s2.jsonl" ]
	[ ! -L "$ACCT/plugins" ] && [ -d "$ACCT/plugins/cache" ]
}

@test "--dry-run changes nothing" {
	make_account
	before="$(cd "$CLAUDE_ACCOUNTS_ROOT" && find . -printf '%p %y\n' | sort)"
	run "$SCRIPT" link --dry-run work
	[ "$status" -eq 0 ]
	after="$(cd "$CLAUDE_ACCOUNTS_ROOT" && find . -printf '%p %y\n' | sort)"
	[ "$before" = "$after" ]
	[ ! -e "$CLAUDE_ACCOUNTS_BACKUP_DIR" ]
	[ "$(wc -l <"$MAIN/history.jsonl")" -eq 2 ]
}

@test "unlink replaces links with real copies" {
	"$SCRIPT" link work >/dev/null
	"$SCRIPT" unlink work >/dev/null
	[ ! -L "$ACCT/settings.json" ] && [ -f "$ACCT/settings.json" ]
	[ ! -L "$ACCT/projects" ] && [ -f "$ACCT/projects/-p/s1.jsonl" ]
}

@test "--all covers every account dir and skips a bare prefix dir" {
	make_account
	mkdir -p "$CLAUDE_ACCOUNTS_ROOT/claude-other" "$CLAUDE_ACCOUNTS_ROOT/claude-"
	run "$SCRIPT" link --all
	[ "$status" -eq 0 ]
	[ -L "$ACCT/settings.json" ]
	[ -L "$CLAUDE_ACCOUNTS_ROOT/claude-other/settings.json" ]
	[ ! -e "$CLAUDE_ACCOUNTS_ROOT/claude-/settings.json" ]
}

@test "--mcp merges user-scope MCP servers into the account .claude.json" {
	make_account
	echo '{"mcpServers":{"gh":{"command":"gh-mcp"}}}' >"$HOME/.claude.json"
	"$SCRIPT" link --mcp work >/dev/null
	[ "$(jq -r '.mcpServers.gh.command' "$ACCT/.claude.json")" = gh-mcp ]
	[ "$(jq -r '.oauthAccount.emailAddress' "$ACCT/.claude.json")" = "w@x" ]
}

@test "rejects bad names and the main dir" {
	run "$SCRIPT" link ../evil
	[ "$status" -ne 0 ]
	ln -s "$MAIN" "$CLAUDE_ACCOUNTS_ROOT/claude-alias"
	run "$SCRIPT" link alias
	[ "$status" -ne 0 ]
	[[ "$output" == *"main config dir"* ]]
}

@test "run sets CLAUDE_CONFIG_DIR and passes args through" {
	mkdir -p "$BATS_TEST_TMPDIR/bin" "$ACCT"
	printf '#!/bin/sh\necho "dir=$CLAUDE_CONFIG_DIR args=$*"\n' >"$BATS_TEST_TMPDIR/bin/claude"
	chmod +x "$BATS_TEST_TMPDIR/bin/claude"
	PATH="$BATS_TEST_TMPDIR/bin:$PATH" run "$SCRIPT" run work --resume -c
	[ "$status" -eq 0 ]
	[ "$output" = "dir=$ACCT args=--resume -c" ]
}

# --------------------------------------------------------------- platform

@test "macOS puts accounts and backups under Application Support" {
	unset CLAUDE_ACCOUNTS_ROOT CLAUDE_ACCOUNTS_BACKUP_DIR XDG_CONFIG_HOME XDG_STATE_HOME
	local root="$HOME/Library/Application Support/claude-accounts"
	CLAUDE_ACCOUNTS_OS=Darwin run "$SCRIPT" link mac
	[ "$status" -eq 0 ]
	[ "$(readlink "$root/claude-mac/settings.json")" = "$MAIN/settings.json" ]
	CLAUDE_ACCOUNTS_OS=Darwin run "$SCRIPT" list
	[[ "$output" == *"$root/claude-mac"* ]]
	CLAUDE_ACCOUNTS_OS=Darwin run "$SCRIPT" --help
	[[ "$output" == *"backups in $root/backups"* ]]
}

@test "macOS keeps using accounts that already live in ~/.config" {
	unset CLAUDE_ACCOUNTS_ROOT XDG_CONFIG_HOME
	make_account
	CLAUDE_ACCOUNTS_OS=Darwin run "$SCRIPT" link --all
	[ "$status" -eq 0 ]
	[ -L "$ACCT/settings.json" ]
	[ ! -e "$HOME/Library/Application Support/claude-accounts/claude-work" ]
}

@test "Linux defaults to XDG dirs" {
	unset CLAUDE_ACCOUNTS_ROOT CLAUDE_ACCOUNTS_BACKUP_DIR XDG_CONFIG_HOME XDG_STATE_HOME
	CLAUDE_ACCOUNTS_OS=Linux run "$SCRIPT" --help
	[[ "$output" == *"accounts in $HOME/.config,"* ]]
	[[ "$output" == *"backups in $HOME/.local/state/claude-accounts."* ]]
}

# --------------------------------------------------------------- remotes

# fake_ssh installs an ssh stand-in: host H runs locally with HOME set to
# $BATS_TEST_TMPDIR/remote-H, a clean environment and the given OS.
fake_ssh() {
	mkdir -p "$BATS_TEST_TMPDIR/bin"
	cat >"$BATS_TEST_TMPDIR/bin/fake-ssh" <<-'SH'
		#!/usr/bin/env bash
		host="$1"; shift
		echo "$host" >>"$FAKE_SSH_LOG"
		export HOME="$FAKE_SSH_ROOT/remote-$host"
		mkdir -p "$HOME" && cd "$HOME" || exit 255
		unset CLAUDE_MAIN_DIR CLAUDE_ACCOUNTS_ROOT CLAUDE_ACCOUNTS_BACKUP_DIR XDG_CONFIG_HOME XDG_STATE_HOME
		export CLAUDE_ACCOUNTS_OS="${FAKE_SSH_OS:-Linux}"
		exec bash -c "$*"
	SH
	chmod +x "$BATS_TEST_TMPDIR/bin/fake-ssh"
	export CLAUDE_ACCOUNTS_SSH="$BATS_TEST_TMPDIR/bin/fake-ssh"
	export FAKE_SSH_ROOT="$BATS_TEST_TMPDIR" FAKE_SSH_LOG="$BATS_TEST_TMPDIR/ssh.log"
}

# local main with a symlinked item, home-absolute plugin paths and MCP servers
seed_upstream() {
	echo agents-rules >"$BATS_TEST_TMPDIR/AGENTS.src"
	ln -s "$BATS_TEST_TMPDIR/AGENTS.src" "$MAIN/AGENTS.md"
	printf '{"m":{"installLocation":"%s/.claude/plugins/marketplaces/m"}}\n' "$HOME" \
		>"$MAIN/plugins/known_marketplaces.json"
	jq -n --arg h "$HOME" '{
		oauthAccount: {emailAddress: "local@x"},
		mcpServers: {gh: {command: "gh-mcp"}},
		projects: {($h + "/proj"): {mcpServers: {db: {command: "db-mcp"}}, allowedTools: ["x"]}}
	}' >"$HOME/.claude.json"
}

@test "--remote pushes main to each host's main, then links the host's accounts" {
	fake_ssh
	seed_upstream
	local r="$BATS_TEST_TMPDIR/remote-mac"
	local racct="$r/Library/Application Support/claude-accounts/claude-iot"
	mkdir -p "$racct" "$r/.claude"
	echo '{"oauthAccount":{"emailAddress":"remote@x"},"mcpServers":{"own":{"command":"o"}}}' >"$r/.claude.json"
	echo '{"oauthAccount":{"emailAddress":"iot@x"}}' >"$racct/.claude.json"

	FAKE_SSH_OS=Darwin run "$SCRIPT" link --all -r mac,nas --remote box
	[ "$status" -eq 0 ]
	[ "$(cat "$FAKE_SSH_LOG" | sort -u | tr '\n' ' ')" = "box mac nas " ]

	# step 1: remote main updated; symlinks arrive as files; home rewritten
	[ "$(cat "$r/.claude/settings.json")" = '{"main":true}' ]
	[ -f "$r/.claude/AGENTS.md" ] && [ ! -L "$r/.claude/AGENTS.md" ]
	[ "$(jq -r .m.installLocation "$r/.claude/plugins/known_marketplaces.json")" = "$r/.claude/plugins/marketplaces/m" ]
	# no sessions pushed
	[ ! -e "$r/.claude/projects/-p/s1.jsonl" ]
	# MCP merged into remote main: upstream servers in, host-only kept, login untouched
	[ "$(jq -r .mcpServers.gh.command "$r/.claude.json")" = gh-mcp ]
	[ "$(jq -r .mcpServers.own.command "$r/.claude.json")" = o ]
	[ "$(jq -r .oauthAccount.emailAddress "$r/.claude.json")" = remote@x ]
	[ "$(jq -r --arg p "$r/proj" '.projects[$p].mcpServers.db.command' "$r/.claude.json")" = db-mcp ]
	[ "$(jq -r --arg p "$r/proj" '.projects[$p].allowedTools' "$r/.claude.json")" = null ]
	[ ! -e "$r/.claude/.claude-accounts-upstream.json" ]

	# step 2: the host's account (macOS path) links to the host's main
	[ "$(readlink "$racct/settings.json")" = "$r/.claude/settings.json" ]
	[ "$(jq -r .mcpServers.gh.command "$racct/.claude.json")" = gh-mcp ]
	[ "$(jq -r .oauthAccount.emailAddress "$racct/.claude.json")" = iot@x ]
	# local still handled
	[[ "$output" == *"remote nas"* ]]
}

@test "--remote removes upstream MCP servers dropped from local main" {
	fake_ssh
	seed_upstream
	local r="$BATS_TEST_TMPDIR/remote-h"
	"$SCRIPT" mcp --all --no-local -r h >/dev/null
	[ "$(jq -r .mcpServers.gh.command "$r/.claude.json")" = gh-mcp ]
	jq 'del(.mcpServers.gh)' "$HOME/.claude.json" >"$HOME/x" && mv "$HOME/x" "$HOME/.claude.json"
	"$SCRIPT" mcp --all --no-local -r h >/dev/null
	[ "$(jq -r '.mcpServers.gh // "gone"' "$r/.claude.json")" = gone ]
}

@test "--no-local only touches the remotes; --dry-run pushes nothing" {
	fake_ssh
	seed_upstream
	run "$SCRIPT" link work --no-local -r h
	[ "$status" -eq 0 ]
	[ ! -e "$ACCT" ]
	[ -L "$BATS_TEST_TMPDIR/remote-h/.config/claude-work/settings.json" ]
	run "$SCRIPT" link work -n -r d
	[ "$status" -eq 0 ]
	[ ! -e "$BATS_TEST_TMPDIR/remote-d/.claude/settings.json" ]
	[ ! -e "$ACCT" ]
}

@test "--remote rejects run, empty hosts and --no-local alone" {
	run "$SCRIPT" run work -r h
	[ "$status" -ne 0 ]
	run "$SCRIPT" -r h run work
	[ "$status" -eq 2 ]
	[[ "$output" == *"does not work with run"* ]]
	run "$SCRIPT" status -r ,
	[ "$status" -ne 0 ]
	run "$SCRIPT" status --no-local
	[ "$status" -eq 2 ]
}
