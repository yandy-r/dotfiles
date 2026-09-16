# Plan: Fabric CLI Dotfiles Integration

## Summary

Add Daniel Miessler's Fabric CLI to the dotfiles as a first-class AI/developer tool: installable through the existing bootstrap flow, discoverable through tool health checks, and usable from bash/zsh with Fabric pattern aliases and direct prompt helpers. The implementation must mirror the current AI CLI installer patterns and avoid unsafe shell startup behavior when Fabric is not installed or has not run `fabric --setup`.

## User Story

As a command-line-heavy dotfiles user, I want Fabric installed and wired into my shell aliases, so that I can run Fabric patterns and quick ad hoc AI questions directly from the terminal.

## Problem -> Solution

Fabric is not represented anywhere in the dotfiles today, so installing it requires manual commands and its pattern aliases are not available in new shells. Add a repo-owned installer, bootstrap hook, tool registry entry, completions support, shared shell functions, generated pattern aliases, direct question helpers, and docs that match the existing bash/zsh/common system.

## Metadata

- **Complexity**: Medium
- **Source PRD**: N/A
- **PRD Phase**: standalone
- **Estimated Files**: 7

## Batches

Tasks grouped by dependency for parallel execution. Tasks within the same batch run concurrently; batches run in order.

| Batch | Tasks         | Depends On | Parallel Width |
| ----- | ------------- | ---------- | -------------- |
| B1    | 1.1, 1.2, 1.3 | none       | 3              |
| B2    | 2.1, 2.2, 2.3 | B1         | 3              |
| B3    | 3.1           | B2         | 1              |

- **Total tasks**: 7
- **Total batches**: 3
- **Max parallel width**: 3

---

## UX Design

### Before

```text
shell -> no Fabric installer in bootstrap
      -> no Fabric tool health entry
      -> no Fabric pattern aliases
      -> user manually runs install/setup/aliases
```

### After

```text
bootstrap.sh
  -> bin/install-fabric.sh
  -> fabric or fabric-ai is available in PATH

new shell
  -> common/paths.sh already exposes ~/.local/bin and GOPATH/bin
  -> common/functions.sh exposes fabric helpers
  -> common/aliases.sh exposes Fabric command aliases
  -> optional generated pattern aliases from ~/.config/fabric/patterns
```

### Interaction Changes

| Touchpoint                | Before                       | After                                                      | Notes                                                                |
| ------------------------- | ---------------------------- | ---------------------------------------------------------- | -------------------------------------------------------------------- |
| Bootstrap                 | Fabric skipped               | Fabric installed beside Claude, OpenCode, Codex            | Installer failures tracked as non-fatal like other optional AI tools |
| Shell startup             | No Fabric commands           | `fabric` works when binary is `fabric` or `fabric-ai`      | Homebrew/AUR install names are normalized                            |
| Pattern usage             | `fabric --pattern summarize` | Pattern aliases generated from `~/.config/fabric/patterns` | Prefix support via `FABRIC_ALIAS_PREFIX`                             |
| Quick questions           | Manual stdin piping          | `qq`, `askai`, `fabric-ask` helpers                        | Bare `??` is unsafe in zsh with `NOMATCH`; Bash-only or opt-in       |
| YouTube transcript helper | Manual `fabric -y ...`       | `yt [-t] URL` helper                                       | Mirrors Fabric README behavior                                       |

---

## Mandatory Reading

Files that MUST be read before implementing:

| Priority       | File                          | Lines                | Why                                                          |
| -------------- | ----------------------------- | -------------------- | ------------------------------------------------------------ |
| P0 (critical)  | `CLAUDE.md`                   | 1-760                | Repo rules, shell validation, alias/tool conventions         |
| P0 (critical)  | `bootstrap.sh`                | 1160-1240, 1670-1745 | AI CLI installer wrapper and bootstrap sequencing patterns   |
| P0 (critical)  | `bin/install-codex.sh`        | 1-540                | Closest existing AI CLI installer contract                   |
| P0 (critical)  | `bin/install-opencode.sh`     | 1-635                | Remote installer prompt and cleanup pattern                  |
| P0 (critical)  | `common/aliases.sh`           | 1-730                | Shared aliases and AI CLI alias placement                    |
| P0 (critical)  | `common/functions.sh`         | 1-90                 | Shared CLI wrapper function pattern                          |
| P1 (important) | `common/tools.sh`             | 1-770                | Tool health and installer delegation registry                |
| P1 (important) | `common/completions.sh`       | 1-620                | Completion setup patterns and shell detection                |
| P1 (important) | `common/paths.sh`             | 300-350              | `~/.local/bin`, `~/.bin`, GOPATH/bin, and mise path handling |
| P1 (important) | `docs/reference/aliases.md`   | 416-540              | Alias reference table format                                 |
| P1 (important) | `bin/README.md`               | 1-320                | Installer documentation format                               |
| P2 (reference) | `playbooks/vars/symlinks.yml` | 1-105                | Confirms `bin/` is symlinked through `utility_dirs`          |

## External Documentation

| Topic               | Source                                                                                   | Key Takeaway                                                                                                                                                                                                |
| ------------------- | ---------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Fabric installation | https://github.com/danielmiessler/Fabric#installation                                    | Recommended Unix install is the official one-line installer; Homebrew/AUR install as `fabric-ai`; Go install is `go install github.com/danielmiessler/fabric/cmd/fabric@latest`; setup is `fabric --setup`. |
| Pattern aliases     | https://github.com/danielmiessler/Fabric#add-aliases-for-all-patterns                    | Fabric documents generating aliases from `~/.config/fabric/patterns/*` and supports `FABRIC_ALIAS_PREFIX`.                                                                                                  |
| CLI usage           | https://github.com/danielmiessler/Fabric#usage                                           | Important flags include `--pattern`, `--stream`, `--listpatterns`, `--updatepatterns`, `--listmodels`, `--listvendors`, `--youtube`, `--scrape_url`, `--scrape_question`, and `--dry-run`.                  |
| Shell completions   | https://github.com/danielmiessler/Fabric#shell-completions                               | Fabric provides zsh, bash, and fish completions plus a completion setup script.                                                                                                                             |
| Installer behavior  | https://raw.githubusercontent.com/danielmiessler/Fabric/main/scripts/installer/README.md | Official installer defaults to `~/.local/bin`, supports `INSTALL_DIR`, detects OS/arch, verifies installation, and prints PATH guidance.                                                                    |

---

## Patterns to Mirror

Code patterns discovered in the codebase. Follow these exactly.

### INSTALLER_CONTRACT

```bash
// SOURCE: bin/install-codex.sh:15-18
ACTION="${ACTION:-install}"
CODEX_VERSION="${CODEX_VERSION:-latest}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
```

### INSTALLER_LOGGING

```bash
// SOURCE: bin/install-codex.sh:51-53
log() { echo "${LOG_PREFIX} $*" >&2; }
error_exit() { log "ERROR: $1" >&2; exit "${2:-1}"; }
```

### PROMPT_CONFIRMATION

```bash
// SOURCE: bin/install-codex.sh:57-68
if [[ "${NON_INTERACTIVE:-false}" == "true" ]] || [[ "${NONINTERACTIVE:-0}" == "1" ]] || [[ "${FORCE_INSTALL:-false}" == "true" ]]; then
    log "Non-interactive mode: auto-approving prompt"
    return 0
fi
```

### REMOTE_INSTALL_GUARD

```bash
// SOURCE: bin/install-opencode.sh:389-392
if ! prompt_confirmation "Do you want to proceed with downloading and executing the remote script?"; then
    log "Installation cancelled by user"
    return 1
fi
```

### BOOTSTRAP_AI_TOOL

```bash
// SOURCE: bootstrap.sh:1217-1223
install_codex() {
  log_section "Installing OpenAI Codex CLI"
  if [[ -f "$SCRIPT_DIR/bin/install-codex.sh" ]]; then
    if ! FORCE_INSTALL=true bash "$SCRIPT_DIR/bin/install-codex.sh"; then
```

### CONDITIONAL_ALIASES

```bash
// SOURCE: common/aliases.sh:77-86
if has_command eza; then
    alias ls="eza -lg --group-directories-first --color always --icons always"
    alias la="eza -lg --all --group-directories-first --color always --icons always"
```

### CLI_WRAPPER_FUNCTION

```bash
// SOURCE: common/functions.sh:48-70
opencode() {
    if [[ -n "${TMUX:-}" ]] && has_command tmux; then
        local passthrough
        passthrough="$(tmux show-options -p -v allow-passthrough 2>/dev/null || true)"
```

### TOOL_REGISTRY

```bash
// SOURCE: common/tools.sh:76-80
        ["wget"]="File downloader"
        ["tree"]="Directory tree viewer"
        ["branchlet"]="Git worktree manager"
        ["opencode"]="AI coding agent"
```

### COMPLETION_LOADER

```bash
// SOURCE: common/completions.sh:74-80
load_completion() {
    local cmd="$1"
    local shell="${2:-$CURRENT_SHELL}"
    local completion_script=""
```

### TEST_STRUCTURE

```bash
// SOURCE: package.json:10-17
"lint": "bin/lint.sh --python --shell",
"lint:modified": "bin/lint.sh --modified --python --shell",
"lint:staged": "bin/lint.sh --staged --python --shell",
```

---

## Files to Change

| File                        | Action | Justification                                                                          |
| --------------------------- | ------ | -------------------------------------------------------------------------------------- |
| `bin/install-fabric.sh`     | CREATE | New Fabric installer following AI CLI installer conventions                            |
| `bootstrap.sh`              | UPDATE | Add Fabric to interactive and non-interactive optional tool install flow               |
| `common/functions.sh`       | UPDATE | Add Fabric command resolution, pattern alias loader, `yt`, and direct-question helpers |
| `common/aliases.sh`         | UPDATE | Add Fabric command aliases and safe direct question alias names                        |
| `common/tools.sh`           | UPDATE | Add Fabric to tool health and `install_tool fabric` delegation                         |
| `common/completions.sh`     | UPDATE | Add Fabric completion setup or completion-path support                                 |
| `bin/README.md`             | UPDATE | Document Fabric installer in AI tooling section                                        |
| `docs/reference/aliases.md` | UPDATE | Document Fabric aliases, generated pattern aliases, and direct prompt helpers          |

## NOT Building

- Do not run `fabric --setup` automatically because it requires provider choices and API credentials.
- Do not commit Fabric API keys, local Fabric config, generated patterns, or machine-specific state.
- Do not symlink `~/.config/fabric` into this repo unless a future task intentionally manages Fabric config.
- Do not disable zsh `NOMATCH` globally to make bare `??` work.
- Do not create unprefixed aliases for invalid pattern names or names that fail shell identifier checks.
- Do not add Windows PowerShell integration in this dotfiles pass.

---

## Step-by-Step Tasks

### Task 1.1: Create Fabric Installer - Depends on none

- **BATCH**: B1
- **ACTION**: Create `bin/install-fabric.sh`.
- **IMPLEMENT**: Mirror `bin/install-codex.sh` and `bin/install-opencode.sh`: strict mode, `ACTION`, `FABRIC_VERSION`, `FORCE_INSTALL`, `NON_INTERACTIVE`, `--install`, `--uninstall`, `--update`, `--version`, `--force`, `--help`, logging helpers, prompt helper, OS/arch detection, and `verify_installation`. Prefer the official Fabric Unix installer into `${FABRIC_INSTALL_DIR:-$HOME/.local/bin}` after confirmation; support Homebrew/AUR `fabric-ai` fallback; support Go install fallback when `go` exists. Normalize verification so either `fabric` or `fabric-ai` is acceptable and print `fabric --setup` as the next manual step.
- **MIRROR**: INSTALLER_CONTRACT, INSTALLER_LOGGING, PROMPT_CONFIRMATION, REMOTE_INSTALL_GUARD.
- **IMPORTS**: none.
- **GOTCHA**: If Homebrew/AUR installs `fabric-ai`, do not copy or rename binaries. Let shell aliases normalize `fabric` to `fabric-ai`.
- **VALIDATE**: `bash -n bin/install-fabric.sh` and `shellcheck --severity=warning bin/install-fabric.sh`.

### Task 1.2: Add Fabric Shell Functions - Depends on none

- **BATCH**: B1
- **ACTION**: Update `common/functions.sh` with Fabric helper functions.
- **IMPLEMENT**: Add a small command resolver such as `_fabric_command` that returns `fabric` or `fabric-ai` without executing a function recursively. Add `fabric-ask`/`fabric_ask` to send joined arguments or stdin to Fabric with `--stream`, plus aliases-friendly wrappers `askai` and `qq` through `common/aliases.sh` in Task 2.2. Add `yt` as a function with `-t|--timestamps` support, quoting URLs and calling Fabric with `-y "$video_link"`.
- **MIRROR**: CLI_WRAPPER_FUNCTION.
- **IMPORTS**: none.
- **GOTCHA**: Bare `??` is not zsh-safe because `zsh/zshrc` sets `NOMATCH`; implement `qq`/`askai` as the zsh-safe default and only add `??` in bash or behind an explicit opt-in that does not change global glob behavior.
- **VALIDATE**: `bash -n common/functions.sh` and `zsh -n common/functions.sh`.

### Task 1.3: Register Fabric Tool and Completions - Depends on none

- **BATCH**: B1
- **ACTION**: Update `common/tools.sh` and `common/completions.sh`.
- **IMPLEMENT**: Add Fabric to `DEV_TOOLS` with a description. Special-case `tool_exists fabric` and `tool_version fabric` so `fabric-ai` counts. Add `install_tool fabric` delegation to `bin/install-fabric.sh`, following the `opencode` custom installer case. Add Fabric completion handling: first try the generic `load_completion fabric`; if Fabric's upstream completion files are installed by the installer, ensure the user-local zsh and bash completion directories Fabric uses are included or documented without slowing shell startup.
- **MIRROR**: TOOL_REGISTRY, COMPLETION_LOADER.
- **IMPORTS**: none.
- **GOTCHA**: The current Fabric README documents static completion files and a setup script, not necessarily a `fabric completion zsh` subcommand. Keep `common/completions.sh` graceful when no generated completion is available.
- **VALIDATE**: `bash -n common/tools.sh common/completions.sh` and `zsh -n common/tools.sh common/completions.sh`.

### Task 2.1: Hook Fabric into Bootstrap - Depends on 1.1

- **BATCH**: B2
- **ACTION**: Update `bootstrap.sh`.
- **IMPLEMENT**: Add `install_fabric()` beside `install_claude`, `install_opencode`, and `install_codex`. Delegate to `bin/install-fabric.sh` with `FORCE_INSTALL=true`, track failures as non-fatal, and add it to both the interactive "Additional tools" section and the non-interactive linear install sequence after Codex or near the other AI CLIs.
- **MIRROR**: BOOTSTRAP_AI_TOOL.
- **IMPORTS**: none.
- **GOTCHA**: Do not call `fabric --setup` during bootstrap; print a next-step message from the installer instead.
- **VALIDATE**: `bash -n bootstrap.sh`.

### Task 2.2: Add Fabric Aliases and Pattern Alias Loading - Depends on 1.2, 1.3

- **BATCH**: B2
- **ACTION**: Update `common/aliases.sh`.
- **IMPLEMENT**: Add a `FABRIC ALIASES` block near the Claude/Codex aliases. Normalize `fabric` to `fabric-ai` only when `fabric` is absent and `fabric-ai` exists. Add explicit aliases/functions for common Fabric commands: `fab`, `fabric-setup`, `fabric-patterns`, `fpatterns`, `fabric-update-patterns`, `fupatterns`, `fabric-models`, `fmodels`, `fabric-vendors`, `fvendors`, `fabric-serve`, and `fabric-serve-ollama`. Wire `qq` and `askai` to the function from Task 1.2. Load all pattern aliases from `~/.config/fabric/patterns` when `FABRIC_ENABLE_PATTERN_ALIASES` is not `false`, applying optional `FABRIC_ALIAS_PREFIX`, validating alias names, skipping collisions unless `FABRIC_ALIAS_OVERWRITE=true`, and avoiding raw unsanitized `eval`.
- **MIRROR**: CONDITIONAL_ALIASES.
- **IMPORTS**: none.
- **GOTCHA**: Pattern aliases can collide with existing shell commands. Default behavior should be conservative: skip existing commands/aliases/functions and optionally allow overwrite with an env var.
- **VALIDATE**: `bash -n common/aliases.sh`, `zsh -n common/aliases.sh`, and a smoke test that sources common files with a temporary fake Fabric patterns directory.

### Task 2.3: Update Documentation - Depends on 1.1, 1.2, 2.2

- **BATCH**: B2
- **ACTION**: Update `bin/README.md` and `docs/reference/aliases.md`.
- **IMPLEMENT**: Document `bin/install-fabric.sh` under AI tooling with install/update/uninstall examples and post-install `fabric --setup`. Add an "AI / Fabric" alias subsection that lists explicit aliases, generated pattern alias behavior, `FABRIC_ALIAS_PREFIX`, collision behavior, `yt`, `qq`, and why bare `??` is not the zsh default.
- **MIRROR**: docs/reference alias table format and bin README AI tool subsections.
- **IMPORTS**: none.
- **GOTCHA**: Keep docs aligned with actual alias names; do not document direct `??` as supported in zsh unless implementation proves it works with `NOMATCH` unchanged.
- **VALIDATE**: `npm run format:modified` or `npx prettier --write bin/README.md docs/reference/aliases.md`.

### Task 3.1: Validate Integrated Shell Behavior - Depends on 2.1, 2.2, 2.3

- **BATCH**: B3
- **ACTION**: Run repo-native validation and fix only issues caused by this feature.
- **IMPLEMENT**: Validate shell syntax, shellcheck, modified-file linting, login shell loading, and Ansible symlink dry run. Add a temporary fake Fabric command and fake patterns directory only in subshell tests to verify generated aliases without requiring a live Fabric account.
- **MIRROR**: TEST_STRUCTURE.
- **IMPORTS**: none.
- **GOTCHA**: `bash -l` currently may surface pre-existing user profile issues outside this repo. If a login-shell check fails before reaching repo code, record the exact failure and still run direct source/syntax checks for changed files.
- **VALIDATE**: Run the commands in the Validation Commands section and ensure zero new failures.

---

## Testing Strategy

### Unit Tests

| Test                     | Input                                         | Expected Output                                        | Edge Case? |
| ------------------------ | --------------------------------------------- | ------------------------------------------------------ | ---------- |
| Installer syntax         | `bash -n bin/install-fabric.sh`               | Exit 0                                                 | No         |
| Fabric command resolver  | fake `fabric-ai` in temporary PATH            | Resolver chooses `fabric-ai`                           | Yes        |
| Pattern alias generation | fake patterns `summarize`, `extract_wisdom`   | aliases/functions become available with prefix support | No         |
| Collision handling       | fake existing `summarize` command             | generated alias is skipped unless overwrite enabled    | Yes        |
| Direct question helper   | `fabric-ask "hello"` with fake Fabric command | helper pipes `hello` to Fabric with `--stream`         | No         |
| zsh question shortcut    | zsh with `NOMATCH` enabled                    | `qq` works; bare `??` is not required                  | Yes        |
| YouTube helper           | `yt -t URL` with fake Fabric command          | calls Fabric with `--transcript-with-timestamps`       | No         |

### Edge Cases Checklist

- [ ] Fabric not installed.
- [ ] Fabric installed as `fabric-ai` only.
- [ ] `~/.config/fabric/patterns` missing before `fabric --setup`.
- [ ] Empty pattern directory.
- [ ] Pattern names with characters unsafe for shell aliases.
- [ ] Alias name collides with an existing command, alias, or function.
- [ ] zsh `NOMATCH` enabled.
- [ ] Non-interactive bootstrap cannot prompt.
- [ ] Official installer download fails.
- [ ] Homebrew/AUR/Go fallback unavailable.
- [ ] Completion setup unavailable.

---

## Validation Commands

### Static Analysis

```bash
bash -n bin/install-fabric.sh bootstrap.sh common/aliases.sh common/functions.sh common/tools.sh common/completions.sh
zsh -n common/aliases.sh common/functions.sh common/tools.sh common/completions.sh zsh/zshrc
shellcheck --severity=warning bin/install-fabric.sh bootstrap.sh common/aliases.sh common/functions.sh common/tools.sh common/completions.sh
```

EXPECT: Zero syntax errors and no new shellcheck warnings.

### Unit Tests

```bash
npm run lint:modified
```

EXPECT: Modified shell and docs files pass repo linting.

### Full Test Suite

```bash
ansible-playbook playbooks/symlinks.yml --extra-vars "dryrun=true"
```

EXPECT: Symlink dry run completes without changing live files.

### Database Validation

```bash
printf 'No database validation required for dotfiles Fabric integration.\n'
```

EXPECT: Not applicable.

### Browser Validation

```bash
printf 'No browser validation required for shell/bootstrap integration.\n'
```

EXPECT: Not applicable.

### Manual Validation

- [ ] Run `bin/install-fabric.sh --help` and confirm install/update/uninstall/version options are documented.
- [ ] Run `FORCE_INSTALL=true bin/install-fabric.sh --install` only when ready to perform the live installation.
- [ ] After installation, run `fabric --version` or `fabric-ai --version`.
- [ ] Run `fabric --setup` manually to configure providers and keys.
- [ ] Restart shell or source the changed common files.
- [ ] Run `fabric --listpatterns`.
- [ ] Run `qq "Summarize what Fabric does in one paragraph"` after setup.
- [ ] Run one generated pattern alias such as `summarize` or `${FABRIC_ALIAS_PREFIX}summarize` if the pattern exists.

---

## Acceptance Criteria

- [ ] `bin/install-fabric.sh` exists and follows existing AI CLI installer conventions.
- [ ] Bootstrap installs Fabric through the same optional-tool flow as Claude, OpenCode, and Codex.
- [ ] `common/tools.sh` reports Fabric health when either `fabric` or `fabric-ai` exists.
- [ ] `common/aliases.sh` normalizes `fabric` to `fabric-ai` when only `fabric-ai` is installed, without breaking a real `fabric` binary.
- [ ] All valid Fabric patterns can be exposed as aliases, with prefix and collision controls.
- [ ] Direct question helpers work in bash and zsh without changing zsh `NOMATCH`.
- [ ] Documentation explains installation, setup, generated aliases, `yt`, `qq`/`askai`, and the `??` decision.
- [ ] Validation commands pass or any unrelated pre-existing failures are documented with evidence.

## Completion Checklist

- [ ] Code follows discovered installer, alias, function, and bootstrap patterns.
- [ ] Error handling matches codebase style.
- [ ] Logging follows installer conventions.
- [ ] Tests follow repo-native shell, lint, and Ansible validation.
- [ ] No secrets, Fabric config, or generated local state committed.
- [ ] Documentation updated.
- [ ] No worktree annotations included.
- [ ] Self-contained implementation handoff.

## Risks

| Risk                                                                 | Likelihood | Impact | Mitigation                                                                              |
| -------------------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------------------------- |
| Generated pattern aliases collide with existing commands             | Medium     | Medium | Validate alias names, skip collisions by default, support `FABRIC_ALIAS_PREFIX`         |
| Official installer changes behavior                                  | Medium     | Medium | Gate remote execution with prompt, verify binary, keep package manager and Go fallbacks |
| zsh bare `??` cannot run due `NOMATCH`                               | High       | Low    | Use `qq`/`askai` as defaults; document `??` as Bash-only or opt-in                      |
| Fabric completions are static files rather than CLI-generated output | Medium     | Low    | Keep completion setup optional and graceful                                             |
| `fabric --setup` requires interactive provider/API choices           | High       | Low    | Do not automate setup; print clear post-install next step                               |

## Notes

- The request used `--no-worktree`, so this plan intentionally omits `## Worktree Setup` and per-task worktree annotations.
- Parallel research found no existing Fabric integration in the checkout.
- The best default shortcut for direct prompts is `qq`/`askai`, not bare `??`, because this repository's zsh config enables `NOMATCH` and `??` is parsed as a two-character glob before command dispatch.
- For pattern aliases, prefer upstream-compatible behavior: read `~/.config/fabric/patterns`, apply `FABRIC_ALIAS_PREFIX` if set, and map each valid alias to `fabric --pattern <pattern>`.
