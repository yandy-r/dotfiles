#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
INSTALLER="$REPO_ROOT/bin/install-playwright-cli.sh"
TMP_DIR=""
ORIGINAL_PATH="$PATH"

cleanup() {
  [[ -z "$TMP_DIR" ]] || rm -rf "$TMP_DIR"
}

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_file() {
  [[ -f "$1" ]] || fail "expected file: $1"
}

assert_missing() {
  [[ ! -e "$1" ]] || fail "expected path to be absent: $1"
}

assert_contains() {
  local needle="$1"
  local file="$2"
  grep -Fq -- "$needle" "$file" || fail "expected '$needle' in $file"
}

setup_case() {
  cleanup
  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/test-install-playwright-cli.XXXXXX")"
  trap cleanup EXIT

  export HOME="$TMP_DIR/home"
  # The installer honors CODEX_HOME/XDG_CONFIG_HOME; clear any inherited value so
  # the default ~/.codex and ~/.config roots are what the assertions exercise.
  unset CODEX_HOME XDG_CONFIG_HOME
  export TEST_BIN="$TMP_DIR/bin"
  export TEST_LOG="$TMP_DIR/commands.log"
  export PLAYWRIGHT_CLI_SKILL_SOURCE="$TMP_DIR/source/playwright-cli"
  mkdir -p "$HOME" "$TEST_BIN" "$PLAYWRIGHT_CLI_SKILL_SOURCE/references"
  printf '%s\n' '---' 'name: playwright-cli' 'description: test skill' '---' >"$PLAYWRIGHT_CLI_SKILL_SOURCE/SKILL.md"
  printf '%s\n' 'test reference' >"$PLAYWRIGHT_CLI_SKILL_SOURCE/references/test.md"
  : >"$TEST_LOG"

  cat >"$TEST_BIN/node" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-p" ]]; then
  printf '20\n'
else
  printf 'v20.0.0\n'
fi
EOF

  cat >"$TEST_BIN/npm" <<'EOF'
#!/usr/bin/env bash
printf 'npm %s\n' "$*" >>"$TEST_LOG"
if [[ "$*" == "root --global" ]]; then
  printf '%s\n' "$HOME/.npm-global/lib/node_modules"
fi
EOF

  cat >"$TEST_BIN/playwright-cli" <<'EOF'
#!/usr/bin/env bash
printf 'Version 0.1.20\n'
EOF

  chmod +x "$TEST_BIN/node" "$TEST_BIN/npm" "$TEST_BIN/playwright-cli"
  export PATH="$TEST_BIN:$ORIGINAL_PATH"
  hash -r
}

test_global_is_default() {
  setup_case

  "$INSTALLER"

  assert_file "$HOME/.agents/skills/playwright-cli/SKILL.md"
  assert_file "$HOME/.claude/skills/playwright-cli/SKILL.md"
  assert_file "$HOME/.codex/skills/playwright-cli/SKILL.md"
  assert_file "$HOME/.cursor/skills/playwright-cli/SKILL.md"
  assert_file "$HOME/.config/opencode/skills/playwright-cli/SKILL.md"
  assert_file "$HOME/.gemini/antigravity/skills/playwright-cli/SKILL.md"
  assert_file "$HOME/.agents/skills/playwright-cli/references/test.md"
  assert_contains "npm install --global @playwright/cli@latest" "$TEST_LOG"
}

test_local_targets_project() {
  setup_case
  local project="$TMP_DIR/project"
  mkdir -p "$project"

  "$INSTALLER" --local --project "$project" --version 0.1.20

  assert_file "$project/.agents/skills/playwright-cli/SKILL.md"
  assert_file "$project/.claude/skills/playwright-cli/SKILL.md"
  assert_file "$project/.codex/skills/playwright-cli/SKILL.md"
  assert_file "$project/.cursor/skills/playwright-cli/SKILL.md"
  assert_file "$project/.opencode/skills/playwright-cli/SKILL.md"
  assert_missing "$project/.gemini"
  assert_missing "$HOME/.claude"
  assert_contains "npm install --global @playwright/cli@0.1.20" "$TEST_LOG"
}

test_uninstall_removes_selected_scope() {
  setup_case
  mkdir -p \
    "$HOME/.agents/skills/playwright-cli" \
    "$HOME/.claude/skills/playwright-cli" \
    "$HOME/.codex/skills/playwright-cli" \
    "$HOME/.cursor/skills/playwright-cli" \
    "$HOME/.config/opencode/skills/playwright-cli" \
    "$HOME/.gemini/antigravity/skills/playwright-cli"
  touch \
    "$HOME/.agents/skills/playwright-cli/SKILL.md" \
    "$HOME/.claude/skills/playwright-cli/SKILL.md" \
    "$HOME/.codex/skills/playwright-cli/SKILL.md" \
    "$HOME/.cursor/skills/playwright-cli/SKILL.md" \
    "$HOME/.config/opencode/skills/playwright-cli/SKILL.md" \
    "$HOME/.gemini/antigravity/skills/playwright-cli/SKILL.md"

  "$INSTALLER" --uninstall

  assert_missing "$HOME/.agents/skills/playwright-cli"
  assert_missing "$HOME/.claude/skills/playwright-cli"
  assert_missing "$HOME/.codex/skills/playwright-cli"
  assert_missing "$HOME/.cursor/skills/playwright-cli"
  assert_missing "$HOME/.config/opencode/skills/playwright-cli"
  assert_missing "$HOME/.gemini/antigravity/skills/playwright-cli"
  assert_contains "npm uninstall --global @playwright/cli" "$TEST_LOG"
}

# Regression fence for the .agents-only bug: installed Codex, Cursor, and
# OpenCode builds did not discover the skill from ~/.agents/skills, so every
# agent must get its own native directory rather than relying on the shared one.
test_every_agent_gets_a_native_root() {
  setup_case

  "$INSTALLER"

  local root
  for root in \
    "$HOME/.claude/skills" \
    "$HOME/.codex/skills" \
    "$HOME/.cursor/skills" \
    "$HOME/.config/opencode/skills" \
    "$HOME/.gemini/antigravity/skills"; do
    assert_file "$root/playwright-cli/SKILL.md"
    diff -r "$HOME/.agents/skills/playwright-cli" "$root/playwright-cli" >/dev/null ||
      fail "skill at $root/playwright-cli differs from the shared .agents copy"
  done
}

test_respects_codex_home_and_xdg_config_home() {
  setup_case
  export CODEX_HOME="$TMP_DIR/codex-home"
  export XDG_CONFIG_HOME="$TMP_DIR/xdg-config"

  "$INSTALLER"

  assert_file "$CODEX_HOME/skills/playwright-cli/SKILL.md"
  assert_file "$XDG_CONFIG_HOME/opencode/skills/playwright-cli/SKILL.md"
  assert_missing "$HOME/.codex"
  assert_missing "$HOME/.config/opencode"
}

test_dry_run_changes_nothing() {
  setup_case

  "$INSTALLER" --dry-run

  assert_missing "$HOME/.claude"
  [[ ! -s "$TEST_LOG" ]] || fail "dry-run executed npm"
}

for test_case in \
  test_global_is_default \
  test_local_targets_project \
  test_uninstall_removes_selected_scope \
  test_every_agent_gets_a_native_root \
  test_respects_codex_home_and_xdg_config_home \
  test_dry_run_changes_nothing; do
  "$test_case"
  printf 'PASS: %s\n' "$test_case"
done
