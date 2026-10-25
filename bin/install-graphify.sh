#!/usr/bin/env bash

# Install graphify (PyPI: graphifyy) as a uv tool together with its optional
# extras. graphify only *warns* when an optional extractor or LLM-backend
# dependency is missing ("10 .tf file(s) contributed nothing to the graph ...
# tree_sitter_hcl not installed"), so installing the bare package, or adding one
# extra at a time, keeps surfacing a new gap on every repo. This installs the
# upstream `all` extra by default and verifies each extra actually imports.
#
# It also migrates away from pipx installs: pipx and uv both link
# ~/.local/bin/graphify, so a stale pipx venv silently shadows the uv tool and
# extras installed into one never reach the binary that runs.

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[GRAPHIFY-INSTALL]"
readonly PACKAGE_NAME="graphifyy"
readonly EXECUTABLE="graphify"

ACTION="${ACTION:-install}"
GRAPHIFY_VERSION="${GRAPHIFY_VERSION:-latest}"
GRAPHIFY_EXTRAS="${GRAPHIFY_EXTRAS:-all}"
GRAPHIFY_PYTHON="${GRAPHIFY_PYTHON:-}"
FORCE_INSTALL="${FORCE_INSTALL:-false}"
DRY_RUN="${DRY_RUN:-false}"

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Install, update, or uninstall graphify (PyPI package: $PACKAGE_NAME) as a uv
tool with its optional extras, so 'graphify update|extract|label' stops
skipping files for missing grammars (sql, terraform, ...) or LLM backends.

OPTIONS:
    --install           Install (default); skip when present with the requested extras
    --update            Upgrade to the latest/requested version with the requested extras
    --uninstall         Remove the uv tool and any pipx install; keep graphs and skills
    --check             Show the install, its extras, shadowing installs, and import status
    --force             Reinstall even when present; approve prompts
    --extras LIST       Comma-separated extras (default: all)
    --version VERSION   Package version (e.g. 0.9.95; default: latest)
    --python VERSION    Python interpreter for the tool venv (default: uv's choice)
    --dry-run           Print actions without changing anything
    -h, --help          Show this help

ENVIRONMENT:
    ACTION              install, update, uninstall, or check
    GRAPHIFY_EXTRAS     Comma-separated extras (default: all)
    GRAPHIFY_VERSION    Package version (default: latest)
    GRAPHIFY_PYTHON     Python interpreter for the tool venv
    FORCE_INSTALL       true to reinstall and approve prompts
    DRY_RUN             true to preview changes
    NON_INTERACTIVE     true to approve prompts (used by dotf)
    NONINTERACTIVE      1 to approve prompts

EXTRAS:
    all is the upstream superset. Individual extras include: sql, terraform,
    postgres, ocaml, commonlisp, pascal, dm, robot, vbnet, r, erlang, solidity
    (grammars); openai, gemini, ollama, kimi, anthropic, bedrock (LLM backends);
    mcp, pdf, office, google, watch, svg, video, leiden, neo4j, falkordb, chinese.

EXAMPLES:
    $SCRIPT_NAME                          # graphify with every extra
    $SCRIPT_NAME --update                 # upgrade, keeping every extra
    $SCRIPT_NAME --extras sql,terraform,openai
    $SCRIPT_NAME --check
    $SCRIPT_NAME --uninstall

Agent skills ('graphify install') and graphify-out/ directories are never
touched; run 'graphify uninstall' before --uninstall to remove the skills.
EOF
}

log() { printf '%s %s\n' "$LOG_PREFIX" "$*" >&2; }
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

confirm() {
  local response
  if [[ "$DRY_RUN" == "true" || "$FORCE_INSTALL" == "true" || "${NON_INTERACTIVE:-false}" == "true" || "${NONINTERACTIVE:-0}" == "1" ]]; then
    return 0
  fi
  [[ -t 0 && -t 1 ]] || error_exit "Set NON_INTERACTIVE=true or pass --force to approve: $1"
  printf '%s %s [y/N]: ' "$LOG_PREFIX" "$1" >&2
  read -r response
  case "$response" in
  [yY] | [yY][eE][sS]) return 0 ;;
  *) return 1 ;;
  esac
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
    --install) ACTION="install" ;;
    --update) ACTION="update" ;;
    --uninstall) ACTION="uninstall" ;;
    --check) ACTION="check" ;;
    --force) FORCE_INSTALL="true" ;;
    --dry-run) DRY_RUN="true" ;;
    --extras | --version | --python)
      [[ $# -ge 2 && -n "$2" && "$2" != --* ]] || error_exit "$1 requires a value"
      case "$1" in
      --extras) GRAPHIFY_EXTRAS="$2" ;;
      --version) GRAPHIFY_VERSION="$2" ;;
      --python) GRAPHIFY_PYTHON="$2" ;;
      esac
      shift
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *) error_exit "Unknown option: $1. Use --help for usage information." ;;
    esac
    shift
  done

  case "$ACTION" in
  install | update | uninstall | check) ;;
  *) error_exit "Invalid action: $ACTION" ;;
  esac

  GRAPHIFY_EXTRAS="$(printf '%s' "$GRAPHIFY_EXTRAS" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')"
  GRAPHIFY_EXTRAS="${GRAPHIFY_EXTRAS#,}"
  GRAPHIFY_EXTRAS="${GRAPHIFY_EXTRAS%,}"
  [[ "$GRAPHIFY_EXTRAS" =~ ^[a-z0-9-]+(,[a-z0-9-]+)*$ ]] ||
    error_exit "Expected comma-separated extras, got: '$GRAPHIFY_EXTRAS'"

  if [[ "$GRAPHIFY_VERSION" != "latest" ]]; then
    GRAPHIFY_VERSION="${GRAPHIFY_VERSION#v}"
    [[ "$GRAPHIFY_VERSION" =~ ^[0-9]+(\.[0-9]+)*([a-z0-9.+-]*)$ ]] ||
      error_exit "Expected a release version, got: $GRAPHIFY_VERSION"
  fi
}

require_uv() {
  command -v uv >/dev/null 2>&1 ||
    error_exit "uv is required (https://docs.astral.sh/uv/); install it first, e.g. 'curl -LsSf https://astral.sh/uv/install.sh | sh'"
}

uvq() { uv --color never "$@"; }

package_spec() {
  local spec="${PACKAGE_NAME}[${GRAPHIFY_EXTRAS}]"
  [[ "$GRAPHIFY_VERSION" == "latest" ]] || spec+="==${GRAPHIFY_VERSION}"
  printf '%s\n' "$spec"
}

# The uv receipt line, e.g. "graphifyy v0.9.95 [extras: openai, sql]".
uv_receipt() {
  uvq tool list --show-extras 2>/dev/null | awk -v pkg="$PACKAGE_NAME" '$1 == pkg { print; exit }'
}

uv_installed() { [[ -n "$(uv_receipt)" ]]; }

uv_installed_version() {
  uv_receipt | awk '{ sub(/^v/, "", $2); print $2 }'
}

# Installed extras as a comma-separated list (empty when none).
uv_installed_extras() {
  uv_receipt | sed -n 's/.*\[extras: \([^]]*\)\].*/\1/p' | tr -d ' '
}

uv_tool_executable() {
  printf '%s/%s/bin/%s\n' "$(uvq tool dir)" "$PACKAGE_NAME" "$EXECUTABLE"
}

uv_tool_python() {
  printf '%s/%s/bin/python\n' "$(uvq tool dir)" "$PACKAGE_NAME"
}

extras_satisfied() {
  local installed extra
  installed=",$(uv_installed_extras),"
  [[ "$installed" == *",all,"* ]] && return 0
  local -a requested
  IFS=',' read -r -a requested <<<"$GRAPHIFY_EXTRAS"
  for extra in "${requested[@]}"; do
    [[ "$installed" == *",${extra},"* ]] || return 1
  done
}

version_satisfied() {
  [[ "$GRAPHIFY_VERSION" == "latest" || "$(uv_installed_version)" == "$GRAPHIFY_VERSION" ]]
}

pipx_installed() {
  command -v pipx >/dev/null 2>&1 &&
    pipx list --short 2>/dev/null | awk -v pkg="$PACKAGE_NAME" '$1 == pkg { found = 1 } END { exit !found }'
}

remove_pipx_install() {
  pipx_installed || return 0
  log "Found a pipx install of $PACKAGE_NAME; it shares ~/.local/bin/$EXECUTABLE with uv and shadows the uv tool"
  if confirm "Uninstall the pipx copy of $PACKAGE_NAME?"; then
    run pipx uninstall "$PACKAGE_NAME"
  else
    log "WARNING: keeping the pipx install; '$EXECUTABLE' on PATH may not be the uv tool"
  fi
}

# Importable module(s) proving each extra is present (verified against 0.9.95).
extra_modules() {
  case "$1" in
  all) extra_modules_all ;;
  sql) echo tree_sitter_sql ;;
  terraform) echo tree_sitter_hcl ;;
  postgres) echo psycopg tree_sitter_sql ;;
  ocaml) echo tree_sitter_ocaml ;;
  commonlisp) echo tree_sitter_commonlisp ;;
  pascal) echo tree_sitter_pascal ;;
  dm) echo tree_sitter_dm ;;
  robot) echo robot ;;
  vbnet) echo tree_sitter_vb_dotnet ;;
  solidity) echo tree_sitter_solidity ;;
  r | erlang) echo tree_sitter_language_pack ;;
  openai | gemini | kimi) echo openai tiktoken ;;
  ollama) echo openai ;;
  anthropic) echo anthropic ;;
  bedrock) echo boto3 ;;
  mcp) echo mcp ;;
  pdf) echo pypdf markdownify ;;
  office) echo docx openpyxl ;;
  google) echo openpyxl ;;
  watch) echo watchdog ;;
  svg) echo matplotlib ;;
  video) echo faster_whisper yt_dlp ;;
  neo4j) echo neo4j ;;
  falkordb) echo falkordb ;;
  *) ;; # Unknown or interpreter-dependent extra (leiden, chinese): not import-checked.
  esac
}

extra_modules_all() {
  local extra
  for extra in sql terraform postgres ocaml commonlisp pascal dm robot vbnet solidity r \
    openai anthropic bedrock mcp pdf office watch svg video neo4j falkordb; do
    extra_modules "$extra"
  done
}

requested_modules() {
  local -a requested
  local extra
  IFS=',' read -r -a requested <<<"$GRAPHIFY_EXTRAS"
  for extra in "${requested[@]}"; do
    extra_modules "$extra"
  done | tr ' ' '\n' | sed '/^$/d' | sort -u
}

# Prints the modules that fail to import in the tool venv; returns 1 if any do.
missing_modules() {
  local python module status=0
  python="$(uv_tool_python)"
  [[ -x "$python" ]] || return 1
  while IFS= read -r module; do
    if ! "$python" -c "import $module" >/dev/null 2>&1; then
      printf '%s\n' "$module"
      status=1
    fi
  done < <(requested_modules)
  return "$status"
}

warn_if_shadowed() {
  local on_path expected
  on_path="$(type -P "$EXECUTABLE" || true)"
  expected="$(uv_tool_executable)"
  if [[ -z "$on_path" ]]; then
    log "WARNING: '$EXECUTABLE' is not on PATH; add $(uvq tool dir --bin) to PATH"
  elif [[ "$(readlink -f "$on_path")" != "$(readlink -f "$expected")" ]]; then
    log "WARNING: '$EXECUTABLE' on PATH resolves to $(readlink -f "$on_path"), not the uv tool ($expected)"
  fi
}

verify_installation() {
  local bin missing
  bin="$(uv_tool_executable)"
  [[ -x "$bin" ]] || error_exit "uv reports $PACKAGE_NAME installed but $bin is missing"
  "$bin" --help >/dev/null 2>&1 || error_exit "Installed $EXECUTABLE cannot start: $bin --help failed"
  if ! missing="$(missing_modules)"; then
    error_exit "Extras '$GRAPHIFY_EXTRAS' installed but these modules do not import: $(printf '%s' "$missing" | tr '\n' ' ')"
  fi
  warn_if_shadowed
  log "Verified: $(uv_receipt)"
}

install_graphify() {
  require_uv

  if [[ "$ACTION" == "install" && "$FORCE_INSTALL" != "true" ]] && uv_installed &&
    extras_satisfied && version_satisfied && ! pipx_installed; then
    log "Already installed: $(uv_receipt)"
    log "Use --update to upgrade or --force to reinstall"
    verify_installation
    return 0
  fi

  remove_pipx_install

  # --force lets uv replace executables another installer left in the bin dir.
  local -a args=(tool install --force)
  [[ "$ACTION" == "update" ]] && args+=(--upgrade)
  [[ "$FORCE_INSTALL" == "true" ]] && args+=(--reinstall)
  [[ -n "$GRAPHIFY_PYTHON" ]] && args+=(--python "$GRAPHIFY_PYTHON")
  args+=("$(package_spec)")

  log "Installing $(package_spec) with uv"
  run uv "${args[@]}"

  [[ "$DRY_RUN" == "true" ]] || verify_installation
}

uninstall_graphify() {
  local removed=false
  if command -v uv >/dev/null 2>&1 && uv_installed; then
    log "Uninstalling the uv tool $PACKAGE_NAME"
    run uv tool uninstall "$PACKAGE_NAME"
    removed=true
  fi
  if pipx_installed; then
    log "Uninstalling the pipx install of $PACKAGE_NAME"
    run pipx uninstall "$PACKAGE_NAME"
    removed=true
  fi
  [[ "$removed" == "true" ]] || log "$PACKAGE_NAME is not installed with uv or pipx"
  log "Agent skills and graphify-out/ directories were left in place"
}

check_graphify() {
  require_uv
  local status=0 missing

  if uv_installed; then
    log "uv tool: $(uv_receipt)"
    if missing="$(missing_modules)"; then
      log "Extras '$GRAPHIFY_EXTRAS': all modules import"
    else
      log "Extras '$GRAPHIFY_EXTRAS': missing $(printf '%s' "$missing" | tr '\n' ' ')"
      status=1
    fi
    warn_if_shadowed
  else
    log "uv tool: $PACKAGE_NAME is not installed"
    status=1
  fi

  if pipx_installed; then
    log "pipx: $PACKAGE_NAME is installed (shadows or duplicates the uv tool; rerun --install to migrate)"
    status=1
  fi
  return "$status"
}

main() {
  parse_args "$@"

  case "$ACTION" in
  install | update) install_graphify ;;
  uninstall) uninstall_graphify ;;
  check) check_graphify ;;
  esac
}

main "$@"
