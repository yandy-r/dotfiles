#!/usr/bin/env bash
# ================================================================
# ble.sh (Bash Line Editor) installer
# ================================================================
# Installs ble.sh — syntax highlighting, autosuggestions and a richer vi mode
# for bash — from the upstream prebuilt nightly tarball into
# ~/.local/share/blesh, where bash/bashrc loads it. The prebuilt tarball only
# needs bash/tar/xz; building from git would add make + gawk as host deps.
# The tarball is verified against the GitHub-reported sha256 digest when
# jq or python3 can read the release metadata (HTTPS-only otherwise).
#
#   install-blesh.sh             install (no-op when already installed)
#   install-blesh.sh --update    re-download and reinstall the latest nightly
#   install-blesh.sh --uninstall remove the install, cache and state
#
# https://github.com/akinomyoga/ble.sh
# ================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[BLESH]"

readonly BLESH_REPO="akinomyoga/ble.sh"
readonly BLESH_ASSET="ble-nightly.tar.xz"
readonly BLESH_URL="https://github.com/${BLESH_REPO}/releases/download/nightly/${BLESH_ASSET}"
readonly BLESH_API="https://api.github.com/repos/${BLESH_REPO}/releases/tags/nightly"

DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
INSTALL_DIR="$DATA_HOME/blesh"
DRY_RUN="${DRY_RUN:-false}"

log() { echo "${LOG_PREFIX} $*" >&2; }
log_success() { echo "${LOG_PREFIX} ✓ $*" >&2; }
log_warning() { echo "${LOG_PREFIX} ⚠ $*" >&2; }
log_error() { echo "${LOG_PREFIX} ✗ $*" >&2; }

error_exit() {
    log_error "$1"
    exit "${2:-1}"
}

require_cmds() {
    local cmd
    for cmd in curl tar xz; do
        command -v "$cmd" >/dev/null 2>&1 || error_exit "required command not found: $cmd"
    done
}

# expected_digest prints the hex sha256 GitHub reports for the nightly asset, or
# nothing when the metadata cannot be fetched or parsed.
expected_digest() {
    local json digest=""
    json="$(curl -fsSL -H 'Accept: application/vnd.github+json' "$BLESH_API" 2>/dev/null)" || return 0
    if command -v jq >/dev/null 2>&1; then
        digest="$(jq -r --arg n "$BLESH_ASSET" '.assets[] | select(.name == $n) | .digest // empty' <<<"$json" 2>/dev/null)"
    elif command -v python3 >/dev/null 2>&1; then
        digest="$(python3 -I -c '
import json, sys
name = sys.argv[1]
for a in json.load(sys.stdin).get("assets", []):
    if a.get("name") == name:
        print(a.get("digest") or "")
' "$BLESH_ASSET" <<<"$json" 2>/dev/null)"
    fi
    [[ "$digest" == sha256:* ]] && echo "${digest#sha256:}"
    return 0
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

# install_blesh downloads, verifies and installs the nightly tarball. The
# upstream `ble.sh --install <prefix>` step copies the tree into <prefix>/blesh.
install_blesh() {
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would download $BLESH_URL and run: bash ble-nightly/ble.sh --install $DATA_HOME"
        return 0
    fi

    require_cmds

    local tmp
    tmp="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '$tmp'" EXIT

    log "Downloading $BLESH_URL"
    curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/$BLESH_ASSET" "$BLESH_URL" ||
        error_exit "download failed: $BLESH_URL"

    local want got
    want="$(expected_digest)"
    if [[ -n "$want" ]]; then
        got="$(sha256_of "$tmp/$BLESH_ASSET")"
        [[ "$got" == "$want" ]] || error_exit "sha256 mismatch for $BLESH_ASSET (want $want, got $got)"
        log "sha256 verified: $got"
    else
        log_warning "could not read the release digest (no jq/python3 or API unavailable); relying on HTTPS"
    fi

    tar -xJf "$tmp/$BLESH_ASSET" -C "$tmp" || error_exit "failed to extract $BLESH_ASSET"
    [[ -f "$tmp/ble-nightly/ble.sh" ]] || error_exit "unexpected archive layout: ble-nightly/ble.sh missing"

    mkdir -p "$DATA_HOME"
    bash "$tmp/ble-nightly/ble.sh" --install "$DATA_HOME" ||
        error_exit "ble.sh --install failed"

    [[ -f "$INSTALL_DIR/ble.sh" ]] || error_exit "install finished but $INSTALL_DIR/ble.sh is missing"
    log_success "ble.sh installed to $INSTALL_DIR"
}

uninstall_blesh() {
    local dirs=(
        "$INSTALL_DIR"
        "${XDG_CACHE_HOME:-$HOME/.cache}/blesh"
        "${XDG_STATE_HOME:-$HOME/.local/state}/blesh"
    )
    local d
    for d in "${dirs[@]}"; do
        [[ -e "$d" ]] || continue
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY-RUN: Would remove $d"
        else
            rm -rf -- "$d"
            log "Removed $d"
        fi
    done
    log_success "ble.sh uninstalled"
}

show_usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [--update | --uninstall] [--dry-run] [--help]

Install ble.sh (Bash Line Editor) from the upstream nightly tarball into
${INSTALL_DIR}. Idempotent and non-interactive.

  --update     Reinstall the latest nightly even when already installed
  --uninstall  Remove the install and ble.sh cache/state directories
  --dry-run    Print what would be done
EOF
}

main() {
    local action="install"
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --update) action="update"; shift ;;
            --uninstall) action="uninstall"; shift ;;
            --dry-run) DRY_RUN=true; shift ;;
            --help | -h) show_usage; exit 0 ;;
            *) error_exit "Unknown option: $1. Use --help for usage." ;;
        esac
    done

    case "$action" in
        install)
            if [[ -f "$INSTALL_DIR/ble.sh" ]]; then
                log "ble.sh already installed at $INSTALL_DIR (use --update to refresh)"
                return 0
            fi
            install_blesh
            ;;
        update) install_blesh ;;
        uninstall) uninstall_blesh ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
