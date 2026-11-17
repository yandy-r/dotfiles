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
