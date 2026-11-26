#!/usr/bin/env bash
# Headless GE-Proton manager: download, sha512-verify, and unpack GE-Proton
# builds into Steam's compatibilitytools.d, where Steam, Lutris, umu-run and
# bin/wine-app.sh (--proton ge / ge-<major>-<minor>) discover them.
#
#   proton-install.sh install [latest|GE-ProtonX-Y]   (default: latest)
#   proton-install.sh list                             installed builds
#   proton-install.sh releases [count]                 recent upstream tags
#   proton-install.sh remove GE-ProtonX-Y
#
# Env: PROTON_COMPAT_DIR overrides the install root.

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
GE_REPO="GloriousEggroll/proton-ge-custom"
GE_URL="https://github.com/${GE_REPO}"

die() {
  printf '%s: error: %s\n' "${SCRIPT_NAME}" "$*" >&2
  exit 1
}

log() {
  printf '%s: %s\n' "${SCRIPT_NAME}" "$*" >&2
}

usage() {
  cat <<EOF
Usage:
  ${SCRIPT_NAME} install [latest|GE-ProtonX-Y]   Install a GE-Proton build (default: latest)
  ${SCRIPT_NAME} list                             List installed Proton builds
  ${SCRIPT_NAME} releases [count]                 Show recent GE-Proton release tags (default: 10)
  ${SCRIPT_NAME} remove GE-ProtonX-Y              Remove an installed GE-Proton build

Install root: $(compat_dir)
  (override with PROTON_COMPAT_DIR)
EOF
}

compat_dir() {
  if [[ -n "${PROTON_COMPAT_DIR:-}" ]]; then
    printf '%s\n' "${PROTON_COMPAT_DIR}"
  elif [[ -d "${STEAM_ROOT:-${HOME}/.steam/root}" ]]; then
    printf '%s\n' "${STEAM_ROOT:-${HOME}/.steam/root}/compatibilitytools.d"
  else
    printf '%s\n' "${XDG_DATA_HOME:-${HOME}/.local/share}/Steam/compatibilitytools.d"
  fi
}

require_tools() {
  local t
  for t in "$@"; do
    command -v "${t}" >/dev/null 2>&1 || die "'${t}' is required but not found in PATH"
  done
}

host_arch() {
  case "$(uname -m)" in
  x86_64 | amd64) printf 'x86_64\n' ;;
  aarch64 | arm64) printf 'aarch64\n' ;;
  *) die "unsupported architecture: $(uname -m)" ;;
  esac
}

valid_tag() {
  [[ "$1" =~ ^GE-Proton[0-9]+-[0-9]+(-rc[0-9]+)?$ ]]
}

# Installed directory for tag, if any: newer releases unpack as
# GE-ProtonX-Y-<arch>, older ones as GE-ProtonX-Y.
installed_dir() {
  local root="$1" tag="$2" d
  for d in "${root}/${tag}-$(host_arch)" "${root}/${tag}"; do
    if [[ -x "${d}/proton" ]]; then
      printf '%s\n' "${d}"
      return 0
    fi
  done
  return 1
}

latest_tag() {
  # The /releases/latest redirect carries the tag; no API token or rate limit.
  local location
  location="$(curl -fsSI "${GE_URL}/releases/latest" | tr -d '\r' | awk 'tolower($1)=="location:" {print $2}' | tail -n1)"
  [[ -n "${location}" ]] || die "could not resolve the latest GE-Proton release"
  printf '%s\n' "${location##*/}"
}

# Echo the first asset base name (without extension) that exists for tag.
# Newer releases suffix the arch (GE-Proton11-14-x86_64.tar.gz); older x86_64
# releases do not (GE-Proton9-27.tar.gz).
resolve_asset_base() {
  local tag="$1" arch="$2" base
  local -a candidates=("${tag}-${arch}")
  [[ "${arch}" == "x86_64" ]] && candidates+=("${tag}")
  for base in "${candidates[@]}"; do
    if curl -fsIL -o /dev/null "${GE_URL}/releases/download/${tag}/${base}.tar.gz"; then
      printf '%s\n' "${base}"
      return 0
    fi
  done
  return 1
}

cmd_install() {
  require_tools curl tar sha512sum
  local tag="${1:-latest}"
  [[ "${tag}" == "latest" ]] && tag="$(latest_tag)"
  valid_tag "${tag}" || die "invalid GE-Proton tag '${tag}' (expected GE-ProtonX-Y)"

  local root existing
  root="$(compat_dir)"
  if existing="$(installed_dir "${root}" "${tag}")"; then
    log "${tag} already installed at ${existing}"
    return 0
  fi

  local arch base
  arch="$(host_arch)"
  base="$(resolve_asset_base "${tag}" "${arch}")" || die "no ${arch} asset found for ${tag}"

  local tmp
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064 # expand tmp now; it is local to this function
  trap "rm -rf '${tmp}'" EXIT

  log "downloading ${base}.tar.gz"
  curl -fL --progress-bar -o "${tmp}/${base}.tar.gz" "${GE_URL}/releases/download/${tag}/${base}.tar.gz"
  curl -fsSL -o "${tmp}/${base}.sha512sum" "${GE_URL}/releases/download/${tag}/${base}.sha512sum"

  log "verifying sha512"
  local expected actual
  expected="$(awk '{print $1; exit}' "${tmp}/${base}.sha512sum")"
  actual="$(sha512sum "${tmp}/${base}.tar.gz" | awk '{print $1}')"
  [[ -n "${expected}" && "${expected}" == "${actual}" ]] || die "sha512 mismatch for ${base}.tar.gz"

  # List once into a file: piping tar into an early-exiting reader (head,
  # grep -q) SIGPIPEs tar, which pipefail + set -e turn into a silent exit.
  tar -tzf "${tmp}/${base}.tar.gz" >"${tmp}/listing" || die "could not list ${base}.tar.gz"

  # Refuse archives with absolute paths or parent traversal before extracting.
  if grep -qE '(^/|(^|/)\.\.(/|$))' "${tmp}/listing"; then
    die "refusing to extract ${base}.tar.gz: unsafe paths in archive"
  fi

  local top
  top="$(sed -n '1{s|/.*||;p;q}' "${tmp}/listing")"
  [[ -n "${top}" ]] || die "could not determine the archive's top-level directory"

  mkdir -p "${root}"
  log "extracting into ${root}"
  tar -xzf "${tmp}/${base}.tar.gz" -C "${tmp}"
  [[ -x "${tmp}/${top}/proton" ]] || die "archive did not contain ${top}/proton"
  rm -rf "${root:?}/${top}"
  mv "${tmp}/${top}" "${root}/${top}"

  log "installed ${top} -> ${root}/${top}"
  log "use it with: wine-app.sh install --proton ge ...  (or --proton $(ge_alias "${top}"))"
  log "restart Steam to see it in the compatibility tool list"
}

ge_alias() {
  if [[ "$1" =~ ^GE-Proton([0-9]+)-([0-9]+)(-(x86_64|aarch64))?$ ]]; then
    printf 'ge-%s-%s\n' "${BASH_REMATCH[1]}" "${BASH_REMATCH[2]}"
  else
    printf '%s\n' "$1"
  fi
}

cmd_list() {
  local root d found=0
  root="$(compat_dir)"
  printf 'Install root: %s\n' "${root}"
  [[ -d "${root}" ]] || {
    printf '  (none)\n'
    return 0
  }
  shopt -s nullglob
  for d in "${root}"/*; do
    [[ -x "${d}/proton" ]] || continue
    printf '  %s\n' "$(basename "${d}")"
    found=1
  done
  shopt -u nullglob
  [[ "${found}" -eq 1 ]] || printf '  (none)\n'
}

cmd_releases() {
  require_tools curl
  local count="${1:-10}"
  [[ "${count}" =~ ^[0-9]+$ ]] || die "count must be a number"
  curl -fsSL "https://api.github.com/repos/${GE_REPO}/releases?per_page=${count}" |
    grep -oE '"tag_name": *"[^"]+"' | sed -E 's/.*"([^"]+)"$/\1/'
}

cmd_remove() {
  local tag="${1:-}"
  [[ -n "${tag}" ]] || die "remove requires a GE-Proton tag"
  tag="${tag%-x86_64}"
  tag="${tag%-aarch64}"
  valid_tag "${tag}" || die "invalid GE-Proton tag '${tag}'"
  local root dir
  root="$(compat_dir)"
  dir="$(installed_dir "${root}" "${tag}")" || die "${tag} is not installed in ${root}"
  rm -rf "${dir:?}"
  log "removed ${dir}"
}

main() {
  local cmd="${1:-}"
  [[ $# -gt 0 ]] && shift
  case "${cmd}" in
  install) cmd_install "$@" ;;
  list) cmd_list ;;
  releases) cmd_releases "$@" ;;
  remove) cmd_remove "$@" ;;
  -h | --help | help | "") usage ;;
  *) die "unknown command '${cmd}' (see --help)" ;;
  esac
}

main "$@"
