#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TOOL="$REPO_ROOT/bin/tea-tools"
TMP_DIR=""

cleanup() {
  if [[ -n "$TMP_DIR" ]]; then
    rm -rf "$TMP_DIR"
  fi
}

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_log_contains() {
  local expected="$1"
  if ! grep -Fqx "$expected" "$TEA_TOOLS_TEST_LOG"; then
    echo "Expected log line not found:" >&2
    echo "  $expected" >&2
    echo "Actual log:" >&2
    sed 's/^/  /' "$TEA_TOOLS_TEST_LOG" >&2
    exit 1
  fi
}

assert_log_not_contains() {
  local unexpected="$1"
  if grep -Fqx "$unexpected" "$TEA_TOOLS_TEST_LOG"; then
    echo "Unexpected log line found:" >&2
    echo "  $unexpected" >&2
    echo "Actual log:" >&2
    sed 's/^/  /' "$TEA_TOOLS_TEST_LOG" >&2
    exit 1
  fi
}

write_fakes() {
  local bin_dir="$1"

  cat >"$bin_dir/tea" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

log() {
  printf 'tea %s\n' "$*" >>"$TEA_TOOLS_TEST_LOG"
}

if [[ "${1:-}" == "api" ]]; then
  shift
  method="GET"
  args=()
  while (($#)); do
    case "$1" in
      --method|-X)
        method="$2"
        shift 2
        ;;
      *)
        args+=("$1")
        shift
        ;;
    esac
  done
  endpoint="${args[-1]}"
  log "api --method ${method} ${endpoint}"
  if [[ "$method" == "GET" && "$endpoint" == "/repos/{owner}/{repo}/pulls/7" ]]; then
    cat <<'JSON'
{
  "number": 7,
  "state": "open",
  "head": {
    "label": "feat/demo-branch",
    "ref": "refs/pull/7/head",
    "repo": {"full_name": "yandy/dotfiles"}
  },
  "base": {
    "ref": "main",
    "repo": {"full_name": "yandy/dotfiles", "default_branch": "main"}
  }
}
JSON
    exit 0
  fi
  if [[ "$method" == "GET" && "$endpoint" == "/repos/{owner}/{repo}/pulls/8" ]]; then
    cat <<'JSON'
{
  "number": 8,
  "state": "open",
  "head": {
    "label": "feat/failing-merge",
    "ref": "refs/pull/8/head",
    "repo": {"full_name": "yandy/dotfiles"}
  },
  "base": {
    "ref": "main",
    "repo": {"full_name": "yandy/dotfiles", "default_branch": "main"}
  }
}
JSON
    exit 0
  fi
  exit 0
fi

if [[ "${1:-}" == "pr" && "${2:-}" == "merge" ]]; then
  log "$*"
  if [[ "${*: -1}" == "8" ]]; then
    echo "merge failed" >&2
    exit 2
  fi
  exit 0
fi

log "$*"
EOF
  chmod +x "$bin_dir/tea"

  cat >"$bin_dir/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

log() {
  printf 'git %s\n' "$*" >>"$TEA_TOOLS_TEST_LOG"
}

case "${1:-}" in
  branch)
    if [[ "${2:-}" == "--show-current" ]]; then
      if [[ -n "${TEA_TOOLS_FAKE_STATE_DIR:-}" && -f "$TEA_TOOLS_FAKE_STATE_DIR/current-branch" ]]; then
		cat "$TEA_TOOLS_FAKE_STATE_DIR/current-branch"
		exit 0
      fi
      printf '%s\n' "${TEA_TOOLS_FAKE_CURRENT_BRANCH:-main}"
      exit 0
    fi
    ;;
  show-ref)
    log "$*"
    if [[ "${*: -1}" == "refs/heads/missing" ]]; then
      exit 1
    fi
    exit 0
    ;;
  status)
    if [[ "${2:-}" == "--porcelain" ]]; then
      if [[ "${TEA_TOOLS_FAKE_DIRTY:-0}" == "1" ]]; then
        echo " M dirty-file"
      fi
      exit 0
    fi
    ;;
  switch)
    log "$*"
    if [[ -n "${TEA_TOOLS_FAKE_STATE_DIR:-}" ]]; then
      mkdir -p "$TEA_TOOLS_FAKE_STATE_DIR"
      printf '%s\n' "$2" >"$TEA_TOOLS_FAKE_STATE_DIR/current-branch"
    fi
    exit 0
    ;;
  fetch)
    log "$*"
    exit 0
    ;;
  merge)
    log "$*"
    exit 0
    ;;
  branch)
    log "$*"
    exit 0
    ;;
esac

log "$*"
exit 0
EOF
  chmod +x "$bin_dir/git"
}

run_with_fakes() {
  local tmp_dir="$1"
  shift
  if [[ -n "${TEA_TOOLS_FAKE_STATE_DIR:-}" ]]; then
    mkdir -p "$TEA_TOOLS_FAKE_STATE_DIR"
    printf '%s\n' "${TEA_TOOLS_FAKE_CURRENT_BRANCH:-main}" >"$TEA_TOOLS_FAKE_STATE_DIR/current-branch"
  fi
  PATH="$tmp_dir/bin:$PATH" "$@"
}

test_merge_delete_branch_flow() {
  local tmp_dir="$1"
  : >"$TEA_TOOLS_TEST_LOG"

  run_with_fakes "$tmp_dir" bash "$TOOL" pr merge --style squash --delete-branch 7

  assert_log_contains "tea api --method GET /repos/{owner}/{repo}/pulls/7"
  assert_log_contains "tea pr merge --style squash 7"
  assert_log_contains "tea api --method DELETE /repos/yandy/dotfiles/branches/feat%2Fdemo-branch"
  assert_log_contains "git branch -d feat/demo-branch"
}

test_dry_run_skips_mutations() {
  local tmp_dir="$1"
  : >"$TEA_TOOLS_TEST_LOG"

  run_with_fakes "$tmp_dir" bash "$TOOL" pr merge --delete-branch --dry-run 7 >/dev/null

  assert_log_contains "tea api --method GET /repos/{owner}/{repo}/pulls/7"
  assert_log_not_contains "tea pr merge 7"
  assert_log_not_contains "tea api --method DELETE /repos/yandy/dotfiles/branches/feat%2Fdemo-branch"
  assert_log_not_contains "git branch -d feat/demo-branch"
}

test_merge_failure_skips_cleanup() {
  local tmp_dir="$1"
  : >"$TEA_TOOLS_TEST_LOG"

  if run_with_fakes "$tmp_dir" bash "$TOOL" pr merge --delete-branch 8 >/dev/null 2>&1; then
    fail "merge failure should make tea-tools fail"
  fi

  assert_log_contains "tea pr merge 8"
  assert_log_not_contains "tea api --method DELETE /repos/yandy/dotfiles/branches/feat%2Ffailing-merge"
  assert_log_not_contains "git branch -d feat/failing-merge"
}

test_current_branch_switches_to_base_before_delete() {
  local tmp_dir="$1"
  : >"$TEA_TOOLS_TEST_LOG"

  TEA_TOOLS_FAKE_CURRENT_BRANCH="feat/demo-branch" \
    run_with_fakes "$tmp_dir" bash "$TOOL" pr merge --delete-branch 7

  assert_log_contains "git switch main"
  assert_log_contains "git branch -d feat/demo-branch"
}

test_current_branch_fast_forwards_base_before_delete() {
  local tmp_dir="$1"
  : >"$TEA_TOOLS_TEST_LOG"

  TEA_TOOLS_FAKE_CURRENT_BRANCH="feat/demo-branch" \
    run_with_fakes "$tmp_dir" bash "$TOOL" pr merge --remote origin --delete-branch 7

  assert_log_contains "git switch main"
  assert_log_contains "git fetch origin main"
  assert_log_contains "git merge --ff-only origin/main"
  assert_log_contains "git branch -d feat/demo-branch"
}

test_dirty_current_branch_leaves_local_branch() {
  local tmp_dir="$1"
  : >"$TEA_TOOLS_TEST_LOG"

  if TEA_TOOLS_FAKE_CURRENT_BRANCH="feat/demo-branch" TEA_TOOLS_FAKE_DIRTY=1 \
    run_with_fakes "$tmp_dir" bash "$TOOL" pr merge --delete-branch 7 >/dev/null 2>&1; then
    fail "dirty current branch should report incomplete cleanup"
  fi

  assert_log_contains "tea pr merge 7"
  assert_log_contains "tea api --method DELETE /repos/yandy/dotfiles/branches/feat%2Fdemo-branch"
  assert_log_not_contains "git switch main"
  assert_log_not_contains "git branch -d feat/demo-branch"
}

test_completion_print_generation() {
  local tmp_dir="$1"
  : >"$TEA_TOOLS_TEST_LOG"

  local zsh_completion bash_completion
  zsh_completion="$(run_with_fakes "$tmp_dir" bash "$TOOL" completion print zsh)"
  bash_completion="$(run_with_fakes "$tmp_dir" bash "$TOOL" completion print bash)"

  [[ "$zsh_completion" == *"#compdef tea-tools"* ]] || fail "zsh completion should include compdef"
  [[ "$zsh_completion" == *"compdef _tea_tools tea-tools"* ]] ||
    fail "zsh completion should register tea-tools when eval-loaded"
  [[ "$zsh_completion" == *"--delete-branch"* ]] || fail "zsh completion should include --delete-branch"
  [[ "$zsh_completion" == *"completion"* ]] || fail "zsh completion should include completion command"

  [[ "$bash_completion" == *"complete -F _tea_tools_completion tea-tools"* ]] ||
    fail "bash completion should register tea-tools"
  [[ "$bash_completion" == *"--delete-branch"* ]] || fail "bash completion should include --delete-branch"
}

test_completion_install_to_explicit_dir() {
  local tmp_dir="$1"
  local install_dir="$tmp_dir/install"
  : >"$TEA_TOOLS_TEST_LOG"

  run_with_fakes "$tmp_dir" bash "$TOOL" completion zsh --dir "$install_dir" >/dev/null

  [[ -f "$install_dir/_tea-tools" ]] || fail "zsh completion install should write _tea-tools"
  grep -Fq "#compdef tea-tools" "$install_dir/_tea-tools" ||
    fail "installed zsh completion should include compdef"
  grep -Fq -- "--delete-branch" "$install_dir/_tea-tools" ||
    fail "installed zsh completion should include --delete-branch"
}

test_completion_shell_installs_without_printing_script() {
  local tmp_dir="$1"
  local install_dir="$tmp_dir/install-default"
  local output
  : >"$TEA_TOOLS_TEST_LOG"

  output="$(run_with_fakes "$tmp_dir" bash "$TOOL" completion zsh --dir "$install_dir")"

  [[ -f "$install_dir/_tea-tools" ]] || fail "completion zsh should install _tea-tools"
  [[ "$output" != *"#compdef tea-tools"* ]] || fail "completion zsh should not print script by default"
  [[ "$output" == *"Installed zsh completion"* ]] || fail "completion zsh should report installed path"
}

test_zsh_default_completion_dir_is_external() {
  local tmp_dir="$1"
  local fake_home="$tmp_dir/home"
  local output
  mkdir -p "$fake_home"

  output="$(HOME="$fake_home" run_with_fakes "$tmp_dir" bash "$TOOL" completion zsh)"

  [[ -f "$fake_home/.zsh/completions/_tea-tools" ]] ||
    fail "completion zsh should install to ~/.zsh/completions/_tea-tools by default"
  [[ "$output" == *"$fake_home/.zsh/completions/_tea-tools"* ]] ||
    fail "completion zsh should report the ~/.zsh/completions target"
}

test_zsh_startup_registers_installed_completion() {
  if ! command -v zsh >/dev/null 2>&1; then
    return 0
  fi

  local completion_dir="$TMP_DIR/zsh-completions"
  run_with_fakes "$TMP_DIR" bash "$TOOL" completion zsh --dir "$completion_dir" >/dev/null

  local result
  result="$(PATH="$REPO_ROOT/bin:$PATH" TEA_TOOLS_TEST_COMPLETION_DIR="$completion_dir" \
    zsh -fic 'fpath=("$TEA_TOOLS_TEST_COMPLETION_DIR" $fpath); autoload -Uz compinit; compinit -u -D; print ${_comps[tea-tools]-missing}' 2>/dev/null)"
  [[ "$result" == "_tea-tools" || "$result" == "_tea_tools" ]] ||
    fail "zsh login shell should register tea-tools completion, got: ${result:-empty}"
}

main() {
  [[ -x "$TOOL" ]] || fail "$TOOL is not executable"

  TMP_DIR="$(mktemp -d)"
  export TEA_TOOLS_TEST_LOG="$TMP_DIR/calls.log"
  export TEA_TOOLS_FAKE_STATE_DIR="$TMP_DIR/state"
  mkdir -p "$TMP_DIR/bin"
  write_fakes "$TMP_DIR/bin"
  trap cleanup EXIT

  test_merge_delete_branch_flow "$TMP_DIR"
  test_dry_run_skips_mutations "$TMP_DIR"
  test_merge_failure_skips_cleanup "$TMP_DIR"
  test_current_branch_switches_to_base_before_delete "$TMP_DIR"
  test_current_branch_fast_forwards_base_before_delete "$TMP_DIR"
  test_dirty_current_branch_leaves_local_branch "$TMP_DIR"
  test_completion_print_generation "$TMP_DIR"
  test_completion_install_to_explicit_dir "$TMP_DIR"
  test_completion_shell_installs_without_printing_script "$TMP_DIR"
  test_zsh_default_completion_dir_is_external "$TMP_DIR"
  test_zsh_startup_registers_installed_completion

  echo "tea-tools tests passed"
}

main "$@"
