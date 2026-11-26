#!/usr/bin/env bash
# Shared Proton version definitions — sourced by wine-app.sh and other game helpers.
# Requires bash 4+ (associative arrays).

[[ -n "${BASH_VERSION:-}" ]] || return 0

# Default when no -p/--proton is passed (override before sourcing, or via env).
PROTON_DEFAULT="${PROTON_DEFAULT:-cachyos}"

declare -gA PROTON_VERSIONS
_proton_registry_built=0

_proton_dir_has_proton() {
  [[ -d "$1" && -x "$1/proton" ]]
}

# Lowercase, spaces -> hyphens, drop single quotes (Steam folder names).
_proton_norm_key() {
  local n="${1,,}"
  n="${n// /-}"
  n="${n//\'/}"
  printf '%s' "$n"
}

# Register path under key if key is non-empty and not already set (first wins).
_proton_register() {
  local path="$1"
  local key="$2"
  [[ -n "${key}" ]] || return 0
  [[ -n "${PROTON_VERSIONS[${key}]:-}" ]] && return 0
  PROTON_VERSIONS["${key}"]="${path}"
}

_proton_register_dir() {
  local path="$1"
  _proton_dir_has_proton "${path}" || return 0
  local base nk
  base="$(basename "${path}")"
  nk="$(_proton_norm_key "${base}")"
  _proton_register "${path}" "${nk}"
}

_proton_collect_ge_dirs() {
  local -n _out="${1}"
  local root d
  for root in "${HOME}/.steam/root/compatibilitytools.d" "${XDG_DATA_HOME:-${HOME}/.local/share}/Steam/compatibilitytools.d"; do
    [[ -d "${root}" ]] || continue
    shopt -s nullglob
    for d in "${root}"/GE-Proton*; do
      [[ -d "${d}" ]] || continue
      _proton_dir_has_proton "${d}" || continue
      _out+=("${d}")
    done
    shopt -u nullglob
  done
}

_proton_apply_well_known_aliases() {
  local -a ge_dirs=()
  _proton_collect_ge_dirs ge_dirs

  if [[ ${#ge_dirs[@]} -gt 0 ]]; then
    local -a sorted=()
    readarray -t sorted < <(printf '%s\n' "${ge_dirs[@]}" | sort -V)
    local latest_ge="${sorted[-1]}"
    PROTON_VERSIONS[ge]="${latest_ge}"
    local d bn k
    for d in "${sorted[@]}"; do
      bn="$(basename "${d}")"
      k="$(_proton_norm_key "${bn}")"
      _proton_register "${d}" "${k}"
      # Shorthand: ge-10-34 from GE-Proton10-34 (newer builds unpack as
      # GE-Proton11-14-x86_64, so the arch suffix is optional).
      if [[ "${bn}" =~ ^GE-Proton([0-9]+)-([0-9]+)(-(x86_64|aarch64))?$ ]]; then
        _proton_register "${d}" "ge-${BASH_REMATCH[1]}-${BASH_REMATCH[2]}"
      fi
    done
  fi

  # cachyos: prefer system path, then newest user proton-cachyos-* (sort -V).
  local cand="/usr/share/steam/compatibilitytools.d/proton-cachyos-slr"
  if _proton_dir_has_proton "${cand}"; then
    PROTON_VERSIONS[cachyos]="${cand}"
  fi
  if [[ -z "${PROTON_VERSIONS[cachyos]:-}" ]]; then
    local -a cachy=()
    local root
    for root in "${HOME}/.steam/root/compatibilitytools.d" "${XDG_DATA_HOME:-${HOME}/.local/share}/Steam/compatibilitytools.d"; do
      [[ -d "${root}" ]] || continue
      shopt -s nullglob
      for cand in "${root}"/proton-cachyos-*; do
        [[ -d "${cand}" ]] || continue
        _proton_dir_has_proton "${cand}" || continue
        cachy+=("${cand}")
      done
      shopt -u nullglob
    done
    if [[ ${#cachy[@]} -gt 0 ]]; then
      readarray -t sorted_c < <(printf '%s\n' "${cachy[@]}" | sort -V)
      PROTON_VERSIONS[cachyos]="${sorted_c[-1]}"
    fi
  fi

  # Steam-bundled names in steamapps/common
  local k path bn
  for k in "${!PROTON_VERSIONS[@]}"; do
    path="${PROTON_VERSIONS[${k}]}"
    bn="$(basename "${path}")"
    case "${bn,,}" in
    *experimental*)
      [[ -z "${PROTON_VERSIONS[experimental]:-}" ]] && PROTON_VERSIONS[experimental]="${path}"
      ;;
    *hotfix*)
      [[ -z "${PROTON_VERSIONS[hotfix]:-}" ]] && PROTON_VERSIONS[hotfix]="${path}"
      ;;
    esac
  done
}

_build_proton_registry() {
  [[ "${_proton_registry_built}" -eq 1 ]] && return 0
  PROTON_VERSIONS=()

  local root d

  # Optional extra roots (colon-separated), e.g. secondary Steam libraries.
  if [[ -n "${PROTON_EXTRA_SEARCH_DIRS:-}" ]]; then
    local IFS=':'
    local extra
    for extra in ${PROTON_EXTRA_SEARCH_DIRS}; do
      [[ -z "${extra}" ]] && continue
      [[ -d "${extra}" ]] || continue
      if [[ -d "${extra}/steamapps/common" ]]; then
        shopt -s nullglob
        for d in "${extra}/steamapps/common"/*; do
          [[ -d "${d}" ]] || continue
          _proton_register_dir "${d}"
        done
        shopt -u nullglob
      fi
      case "${extra}" in
      */compatibilitytools.d | compatibilitytools.d)
        shopt -s nullglob
        for d in "${extra}"/*; do
          [[ -d "${d}" ]] || continue
          _proton_register_dir "${d}"
        done
        shopt -u nullglob
        ;;
      esac
    done
  fi

  for root in \
    "${HOME}/.steam/root/compatibilitytools.d" \
    "${XDG_DATA_HOME:-${HOME}/.local/share}/Steam/compatibilitytools.d" \
    "/usr/share/steam/compatibilitytools.d"; do
    [[ -d "${root}" ]] || continue
    shopt -s nullglob
    for d in "${root}"/*; do
      [[ -d "${d}" ]] || continue
      case "$(basename "${d}")" in
      LegacyRuntime) continue ;;
      esac
      _proton_register_dir "${d}"
    done
    shopt -u nullglob
  done

  local steam_root
  for steam_root in "${STEAM_ROOT:-${HOME}/.steam/root}" "${XDG_DATA_HOME:-${HOME}/.local/share}/Steam"; do
    [[ -d "${steam_root}/steamapps/common" ]] || continue
    shopt -s nullglob
    for d in "${steam_root}/steamapps/common"/*; do
      [[ -d "${d}" ]] || continue
      _proton_register_dir "${d}"
    done
    shopt -u nullglob
  done

  _proton_apply_well_known_aliases
  _proton_registry_built=1
}

_proton_list_keys() {
  _build_proton_registry
  printf '%s\n' "${!PROTON_VERSIONS[@]}" | sort -u | tr '\n' ' '
}

# Resolve a selection to a Proton root directory (for PROTONPATH / umu-run).
# Empty selection uses PROTON_DEFAULT. "system" yields empty string (no PROTONPATH).
resolve_proton() {
  _build_proton_registry

  local raw="${1-}"
  raw="${raw:-${PROTON_DEFAULT}}"

  if [[ -z "${raw}" || "${raw,,}" == "system" ]]; then
    printf '%s\n' ""
    return 0
  fi

  if [[ -f "${raw}" ]]; then
    printf '%s\n' "$(cd -- "$(dirname -- "${raw}")" && pwd)"
    return 0
  fi

  if [[ -d "${raw}" ]]; then
    if _proton_dir_has_proton "${raw}"; then
      printf '%s\n' "$(cd -- "${raw}" && pwd)"
      return 0
    fi
  fi

  local key
  key="$(_proton_norm_key "${raw}")"

  if [[ -n "${PROTON_VERSIONS[${key}]:-}" ]]; then
    printf '%s\n' "${PROTON_VERSIONS[${key}]}"
    return 0
  fi

  echo "Error: unknown proton version '${raw}'" >&2
  echo "Available: $(_proton_list_keys)" >&2
  case "${key}" in
  ge | ge-*) echo "Install GE-Proton with: wine-app.sh proton install [latest|GE-ProtonX-Y]" >&2 ;;
  esac
  return 1
}

resolve_proton_executable() {
  local selection="${1-}"
  _build_proton_registry

  if [[ -z "${selection}" ]]; then
    selection="${PROTON_DEFAULT}"
  fi

  if [[ -z "${selection}" || "${selection,,}" == "system" ]]; then
    if command -v proton >/dev/null 2>&1; then
      command -v proton
      return 0
    fi
    return 1
  fi

  if [[ -f "${selection}" && -x "${selection}" ]]; then
    printf '%s\n' "${selection}"
    return 0
  fi

  if [[ -d "${selection}" && -x "${selection}/proton" ]]; then
    printf '%s\n' "${selection}/proton"
    return 0
  fi

  if command -v "${selection}" >/dev/null 2>&1; then
    command -v "${selection}"
    return 0
  fi

  local proton_dir
  proton_dir="$(resolve_proton "${selection}")" || return 1

  if [[ -z "${proton_dir}" ]]; then
    if command -v proton >/dev/null 2>&1; then
      command -v proton
      return 0
    fi
    return 1
  fi

  local proton_executable="${proton_dir}/proton"
  if [[ -x "${proton_executable}" ]]; then
    printf '%s\n' "${proton_executable}"
    return 0
  fi

  echo "Error: Proton executable not found at '${proton_executable}'" >&2
  return 1
}

# Parse -p/--proton from args; sets PROTON_PATH and REMAINING_ARGS.
parse_proton_flag() {
  PROTON_SELECTION=""
  REMAINING_ARGS=()

  while [[ $# -gt 0 ]]; do
    case "$1" in
    -p | --proton)
      [[ -n "${2:-}" ]] || {
        echo "Error: $1 requires a value" >&2
        exit 1
      }
      PROTON_SELECTION="$2"
      shift 2
      ;;
    *)
      REMAINING_ARGS+=("$1")
      shift
      ;;
    esac
  done

  local resolved
  resolved="$(resolve_proton "${PROTON_SELECTION}")" || exit 1
  PROTON_PATH="${resolved}"
}
