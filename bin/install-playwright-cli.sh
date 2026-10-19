#!/usr/bin/env bash

# Install Microsoft's Playwright CLI and its agent skill.
# The CLI is installed globally through npm. Skills default to user-global scope;
# pass --local to install them in the current project instead.

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[PLAYWRIGHT-CLI-INSTALL]"
readonly PACKAGE_NAME="@playwright/cli"
readonly SKILL_NAME="playwright-cli"

ACTION="${ACTION:-install}"
PLAYWRIGHT_CLI_VERSION="${PLAYWRIGHT_CLI_VERSION:-latest}"
SKILL_SCOPE="${SKILL_SCOPE:-global}"
PROJECT_DIR="${PROJECT_DIR:-$PWD}"
SKILLS_ONLY="${SKILLS_ONLY:-false}"
DRY_RUN="${DRY_RUN:-false}"

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall Microsoft's Playwright CLI and agent skill.
The CLI package is installed globally. Skills are installed globally by default.

OPTIONS:
    --install           Install Playwright CLI and skills (default)
    --update            Update Playwright CLI and refresh skills
    --uninstall         Uninstall Playwright CLI and remove installed skills
    --global            Install skills for the current user (default)
    --local             Install skills in the current project
    --project DIR       Use DIR as the project for --local (default: current directory)
    --version VERSION   Install a specific package version (default: latest)
    --skills-only       Skip npm package installation and refresh skills only
    --dry-run           Print actions without changing files
    -h, --help          Show this help message

SKILL TARGETS:
    Global:
      Shared (.agents readers)  ~/.agents/skills/playwright-cli
      Claude                    ~/.claude/skills/playwright-cli
      Codex                     ~/.codex/skills/playwright-cli
      Cursor/Cursor Agent       ~/.cursor/skills/playwright-cli
      OpenCode                  ~/.config/opencode/skills/playwright-cli
      Antigravity               ~/.gemini/antigravity/skills/playwright-cli

    Local:
      Shared (.agents readers)  <project>/.agents/skills/playwright-cli
      Claude                    <project>/.claude/skills/playwright-cli
      Codex                     <project>/.codex/skills/playwright-cli
      Cursor/Cursor Agent       <project>/.cursor/skills/playwright-cli
      OpenCode                  <project>/.opencode/skills/playwright-cli

Every agent gets its own native skill directory. The shared .agents directory is
written too, but it is not sufficient on its own: installed Codex, Cursor, and
OpenCode builds do not all discover skills there, so relying on it alone leaves
agents without the skill. Antigravity reads <project>/.agents/skills for project
skills, so it needs no separate local directory.

ENVIRONMENT VARIABLES:
    ACTION                       install, update, or uninstall
    PLAYWRIGHT_CLI_VERSION       npm package version (default: latest)
    SKILL_SCOPE                  global or local (default: global)
    PROJECT_DIR                  target project for local skills
    SKILLS_ONLY                  true to skip npm package installation
    DRY_RUN                      true to print actions only
    PLAYWRIGHT_CLI_SKILL_SOURCE  override bundled skill source directory
    CODEX_HOME                   Codex config home (default: ~/.codex)
    XDG_CONFIG_HOME              config home for OpenCode (default: ~/.config)

EXAMPLES:
    $SCRIPT_NAME
    $SCRIPT_NAME --local
    $SCRIPT_NAME --local --project ~/src/my-app
    $SCRIPT_NAME --update
    $SCRIPT_NAME --skills-only --global
EOF
}

log() {
  printf '%s %s\n' "$LOG_PREFIX" "$*" >&2
}

error_exit() {
  log "ERROR: $1"
  exit "${2:-1}"
}

run() {
  if [[ "$DRY_RUN" == "true" ]]; then
    printf '%s DRY-RUN:' "$LOG_PREFIX" >&2
    printf ' %q' "$@" >&2
    printf '\n' >&2
    return 0
  fi
  "$@"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
    --install)
      ACTION="install"
      ;;
    --update)
      ACTION="update"
      ;;
    --uninstall)
      ACTION="uninstall"
      ;;
    --global | -g)
      SKILL_SCOPE="global"
      ;;
    --local)
      SKILL_SCOPE="local"
      ;;
    --project)
      [[ $# -ge 2 ]] || error_exit "--project requires a directory"
      PROJECT_DIR="$2"
      shift
      ;;
    --version)
      [[ $# -ge 2 ]] || error_exit "--version requires a value"
      PLAYWRIGHT_CLI_VERSION="$2"
      shift
      ;;
    --skills-only)
      SKILLS_ONLY="true"
      ;;
    --dry-run)
      DRY_RUN="true"
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      error_exit "Unknown option: $1. Use --help for usage information."
      ;;
    esac
    shift
  done

  case "$ACTION" in
  install | update | uninstall) ;;
  *) error_exit "Invalid action: $ACTION" ;;
  esac

  case "$SKILL_SCOPE" in
  global | local) ;;
  *) error_exit "Invalid skill scope: $SKILL_SCOPE (expected global or local)" ;;
  esac
}

check_node() {
  command -v node >/dev/null 2>&1 || error_exit "Node.js 18 or newer is required"
  command -v npm >/dev/null 2>&1 || error_exit "npm is required"

  local node_major
  node_major="$(node -p 'process.versions.node.split(".")[0]')"
  [[ "$node_major" =~ ^[0-9]+$ ]] || error_exit "Could not determine Node.js version"
  ((node_major >= 18)) || error_exit "Node.js 18 or newer is required (found $(node --version))"
}

install_package() {
  check_node
  log "Installing ${PACKAGE_NAME}@${PLAYWRIGHT_CLI_VERSION} globally"
  run npm install --global "${PACKAGE_NAME}@${PLAYWRIGHT_CLI_VERSION}"
  # mise-managed node: new global bins get no shim until a reshim.
  if command -v mise >/dev/null 2>&1; then
    run mise reshim
  fi
}

uninstall_package() {
  command -v npm >/dev/null 2>&1 || error_exit "npm is required to uninstall $PACKAGE_NAME"
  log "Uninstalling $PACKAGE_NAME"
  run npm uninstall --global "$PACKAGE_NAME"
}

resolve_skill_source() {
  if [[ -n "${PLAYWRIGHT_CLI_SKILL_SOURCE:-}" ]]; then
    [[ -f "${PLAYWRIGHT_CLI_SKILL_SOURCE}/SKILL.md" ]] ||
      error_exit "Skill source has no SKILL.md: $PLAYWRIGHT_CLI_SKILL_SOURCE"
    printf '%s\n' "$PLAYWRIGHT_CLI_SKILL_SOURCE"
    return
  fi

  local npm_root
  npm_root="$(npm root --global)"
  local candidate
  for candidate in \
    "$npm_root/@playwright/cli/skills/playwright-cli" \
    "$npm_root/@playwright/cli/node_modules/playwright-core/lib/tools/skills/playwright-cli"; do
    if [[ -f "$candidate/SKILL.md" ]]; then
      printf '%s\n' "$candidate"
      return
    fi
  done

  error_exit "Could not find bundled skill. Reinstall $PACKAGE_NAME or set PLAYWRIGHT_CLI_SKILL_SOURCE."
}

# Every agent gets its own native skill directory. The shared .agents directory
# is written as well, but it is NOT sufficient by itself: installed Codex,
# Cursor, and OpenCode builds do not all discover skills there, so an
# .agents-only install leaves those agents without the skill.
skill_roots() {
  if [[ "$SKILL_SCOPE" == "global" ]]; then
    printf '%s\n' \
      "$HOME/.agents/skills" \
      "$HOME/.claude/skills" \
      "${CODEX_HOME:-$HOME/.codex}/skills" \
      "$HOME/.cursor/skills" \
      "${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills" \
      "$HOME/.gemini/antigravity/skills"
  else
    printf '%s\n' \
      "$PROJECT_DIR/.agents/skills" \
      "$PROJECT_DIR/.claude/skills" \
      "$PROJECT_DIR/.codex/skills" \
      "$PROJECT_DIR/.cursor/skills" \
      "$PROJECT_DIR/.opencode/skills"
  fi
}

install_skill_at() {
  local source="$1"
  local root="$2"
  local destination="$root/$SKILL_NAME"

  if [[ "$DRY_RUN" == "true" ]]; then
    log "Would install skill at $destination"
    return
  fi

  log "Installing skill at $destination"
  mkdir -p "$root"
  local temporary
  temporary="$(mktemp -d "$root/.${SKILL_NAME}.XXXXXX")"
  cp -R "$source/." "$temporary/"
  rm -rf "$destination"
  mv "$temporary" "$destination"
}

install_skills() {
  local source="${PLAYWRIGHT_CLI_SKILL_SOURCE:-}"
  if [[ "$DRY_RUN" != "true" ]]; then
    source="$(resolve_skill_source)"
  fi

  local root
  while IFS= read -r root; do
    install_skill_at "$source" "$root"
  done < <(skill_roots)
}

remove_skills() {
  local root destination
  while IFS= read -r root; do
    destination="$root/$SKILL_NAME"
    if [[ -e "$destination" || -L "$destination" ]]; then
      log "Removing skill at $destination"
      run rm -rf "$destination"
    else
      log "Skill not present at $destination"
    fi
  done < <(skill_roots)
}

verify_installation() {
  if [[ "$SKILLS_ONLY" != "true" ]]; then
    command -v playwright-cli >/dev/null 2>&1 ||
      error_exit "playwright-cli is installed but is not available on PATH"
    log "CLI verified: $(playwright-cli --version)"
  fi

  local root skill_file
  while IFS= read -r root; do
    skill_file="$root/$SKILL_NAME/SKILL.md"
    [[ -f "$skill_file" ]] || error_exit "Skill verification failed: $skill_file is missing"
  done < <(skill_roots)
  log "Skills verified for $SKILL_SCOPE scope"
}

main() {
  parse_args "$@"

  case "$ACTION" in
  install | update)
    if [[ "$SKILLS_ONLY" != "true" ]]; then
      install_package
    else
      command -v npm >/dev/null 2>&1 || error_exit "npm is required to locate the installed skill"
    fi
    install_skills
    if [[ "$DRY_RUN" != "true" ]]; then
      verify_installation
    fi
    ;;
  uninstall)
    remove_skills
    if [[ "$SKILLS_ONLY" != "true" ]]; then
      uninstall_package
    fi
    ;;
  esac
}

main "$@"
