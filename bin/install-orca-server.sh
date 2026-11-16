#!/usr/bin/env bash
# ================================================================
# Orca server-only installer (headless `orca-ide serve`)
# ================================================================
# Installs the official Orca server-only build — orcad, its bundled Node
# runtime and the `orca-ide` CLI — for headless Linux hosts. It needs no
# desktop packages (GTK, X11). Upstream publishes it as a per-arch tarball on
# every GitHub release, with an `orca-server-sha256sums.txt` sidecar; the
# tarball is verified against that sidecar (fail closed) and, when readable,
# against GitHub's own asset digest.
#
# The tree is installed per user into ~/.local/share/orca-server and exposed as
# ~/.local/bin/orca-server (a symlink to bin/orca-ide; the launcher resolves
# symlinks). It is deliberately NOT linked as `orca-ide`, so it never shadows
# the desktop app's /usr/bin/orca-ide from the .deb/.rpm.
#
#   install-orca-server.sh             install (no-op when already installed)
#   install-orca-server.sh --update    install the latest release if newer
#   install-orca-server.sh --uninstall remove the install and the symlink
#
# ORCA_SERVER_VERSION=1.4.261 pins a release (default: latest).
#
# https://orca.dev/docs/remote-servers#install-only-the-server-on-linux
# ================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[ORCA-SERVER]"

readonly ORCA_REPO="stablyai/orca"
readonly SUMS_ASSET="orca-server-sha256sums.txt"
readonly VERSION_MARKER=".dotf-version"

DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
INSTALL_DIR="$DATA_HOME/orca-server"
BIN_DIR="$HOME/.local/bin"
LINK_PATH="$BIN_DIR/orca-server"
DRY_RUN="${DRY_RUN:-false}"
ORCA_SERVER_VERSION="${ORCA_SERVER_VERSION:-latest}"

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
    for cmd in curl tar gzip; do
        command -v "$cmd" >/dev/null 2>&1 || error_exit "required command not found: $cmd"
    done
}

# host_arch maps uname -m to upstream's asset arch token.
host_arch() {
    case "$(uname -m)" in
        x86_64 | amd64) echo "x64" ;;
        aarch64 | arm64) echo "arm64" ;;
        *) error_exit "unsupported architecture: $(uname -m) (upstream ships x64 and arm64 only)" ;;
    esac
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        shasum -a 256 "$1" | awk '{print $1}'
    fi
}

# release_json prints the GitHub release metadata for ORCA_SERVER_VERSION.
release_json() {
    local api="https://api.github.com/repos/${ORCA_REPO}/releases"
    if [[ "$ORCA_SERVER_VERSION" == "latest" ]]; then
        api="$api/latest"
    else
        api="$api/tags/v${ORCA_SERVER_VERSION#v}"
    fi
    curl -fsSL --proto '=https' --tlsv1.2 -H 'Accept: application/vnd.github+json' "$api"
}

# json_field <json> <tag|digest> [asset-name] reads the release tag, or an
# asset's digest, with jq or python3. Prints nothing when neither is available.
json_field() {
    local json="$1" field="$2" asset="${3:-}"
    if command -v jq >/dev/null 2>&1; then
        if [[ "$field" == "tag" ]]; then
            jq -r '.tag_name // empty' <<<"$json"
        else
            jq -r --arg n "$asset" '.assets[] | select(.name == $n) | .digest // empty' <<<"$json"
        fi
    elif command -v python3 >/dev/null 2>&1; then
        python3 -I -c '
import json, sys
d = json.load(sys.stdin)
if sys.argv[1] == "tag":
    print(d.get("tag_name") or "")
else:
    for a in d.get("assets", []):
        if a.get("name") == sys.argv[2]:
            print(a.get("digest") or "")
' "$field" "$asset" <<<"$json"
    fi
}

# resolve_version prints the bare version (no leading v) to install.
resolve_version() {
    if [[ "$ORCA_SERVER_VERSION" != "latest" ]]; then
        echo "${ORCA_SERVER_VERSION#v}"
        return 0
    fi
    local json tag
    json="$(release_json)" || error_exit "could not read the latest ${ORCA_REPO} release"
    tag="$(json_field "$json" tag)"
    [[ -n "$tag" ]] || error_exit "could not determine the latest release tag (install jq or python3, or set ORCA_SERVER_VERSION)"
    echo "${tag#v}"
}

installed_version() {
    [[ -f "$INSTALL_DIR/$VERSION_MARKER" ]] && cat "$INSTALL_DIR/$VERSION_MARKER"
    return 0
}

# link_is_ours succeeds when LINK_PATH is a symlink into INSTALL_DIR.
link_is_ours() {
    [[ -L "$LINK_PATH" ]] && [[ "$(readlink "$LINK_PATH")" == "$INSTALL_DIR/"* ]]
}

# ensure_link (re)creates the managed launcher. dotf probes it to decide whether
# the package is installed, so an intact runtime with a missing link must be
# repaired rather than reported as a no-op. A file this installer does not own
# is left alone (warning only).
ensure_link() {
    [[ -x "$INSTALL_DIR/bin/orca-ide" ]] || return 0
    if link_is_ours && [[ "$(readlink "$LINK_PATH")" == "$INSTALL_DIR/bin/orca-ide" ]]; then
        return 0
    fi
    if [[ -e "$LINK_PATH" || -L "$LINK_PATH" ]] && ! link_is_ours; then
        log_warning "$LINK_PATH exists and is not managed by this installer; leaving it alone"
        return 0
    fi
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would link $LINK_PATH -> $INSTALL_DIR/bin/orca-ide"
        return 0
    fi
    mkdir -p "$BIN_DIR" || error_exit "failed to create $BIN_DIR"
    ln -sfn "$INSTALL_DIR/bin/orca-ide" "$LINK_PATH" || error_exit "failed to link $LINK_PATH"
    log "Linked $LINK_PATH -> $INSTALL_DIR/bin/orca-ide"
}

# acquire_lock serializes install/update/uninstall for one INSTALL_DIR: they
# share the .orca-server.new/.old staging paths. The lock fd is released when
# the process exits.
acquire_lock() {
    [[ "$DRY_RUN" == "true" ]] && return 0
    mkdir -p "$DATA_HOME" || error_exit "failed to create $DATA_HOME"
    command -v flock >/dev/null 2>&1 || error_exit "required command not found: flock (util-linux)"
    exec 9>"$DATA_HOME/.orca-server.lock" || error_exit "failed to open the install lock"
    flock -w 600 9 || error_exit "another orca-server install is still running (lock: $DATA_HOME/.orca-server.lock)"
}

install_server() {
    local version="$1"
    local arch asset base
    arch="$(host_arch)"
    asset="orca-server-linux-${arch}.tar.gz"
    base="https://github.com/${ORCA_REPO}/releases/download/v${version}"

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would download $base/$asset, verify it against $SUMS_ASSET,"
        log "DRY-RUN: install it into $INSTALL_DIR and link $LINK_PATH -> $INSTALL_DIR/bin/orca-ide"
        return 0
    fi

    require_cmds

    local tmp
    tmp="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '$tmp'" EXIT

    log "Downloading Orca server v${version} (linux-${arch})"
    curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/$asset" "$base/$asset" ||
        error_exit "download failed: $base/$asset"
    curl -fsSL --proto '=https' --tlsv1.2 -o "$tmp/$SUMS_ASSET" "$base/$SUMS_ASSET" ||
        error_exit "download failed: $base/$SUMS_ASSET"

    # Fail closed on the upstream checksum sidecar.
    local want got
    want="$(awk -v n="$asset" '$2 == n || $2 == "*"n {print $1}' "$tmp/$SUMS_ASSET")"
    [[ "$want" =~ ^[0-9a-f]{64}$ ]] || error_exit "no sha256 for $asset in $SUMS_ASSET"
    got="$(sha256_of "$tmp/$asset")"
    [[ "$got" == "$want" ]] || error_exit "sha256 mismatch for $asset (want $want, got $got)"
    log "sha256 verified against $SUMS_ASSET: $got"

    # Cross-check GitHub's asset digest when the metadata is readable.
    local json digest=""
    if json="$(ORCA_SERVER_VERSION="$version" release_json 2>/dev/null)"; then
        digest="$(json_field "$json" digest "$asset" 2>/dev/null || true)"
    fi
    if [[ "$digest" == sha256:* ]]; then
        [[ "${digest#sha256:}" == "$got" ]] ||
            error_exit "GitHub digest mismatch for $asset (want ${digest#sha256:}, got $got)"
        log "sha256 matches the GitHub release digest"
    fi

    mkdir -p "$tmp/tree"
    tar -xzf "$tmp/$asset" -C "$tmp/tree" --strip-components=1 ||
        error_exit "failed to extract $asset"
    [[ -x "$tmp/tree/bin/orca-ide" ]] || error_exit "unexpected archive layout: bin/orca-ide missing"
    echo "$version" >"$tmp/tree/$VERSION_MARKER"

    # Stage next to the target, then swap, so a failed install never leaves a
    # half-written tree. A running server keeps its open files across the swap.
    mkdir -p "$DATA_HOME" "$BIN_DIR"
    local staged="$DATA_HOME/.orca-server.new" old="$DATA_HOME/.orca-server.old"
    rm -rf -- "$staged" "$old"
    mv "$tmp/tree" "$staged" || error_exit "failed to stage $staged"
    if [[ -e "$INSTALL_DIR" ]]; then
        mv "$INSTALL_DIR" "$old" || error_exit "failed to move aside $INSTALL_DIR"
    fi
    if ! mv "$staged" "$INSTALL_DIR"; then
        [[ -e "$old" ]] && mv "$old" "$INSTALL_DIR"
        error_exit "failed to activate $INSTALL_DIR"
    fi
    rm -rf -- "$old"

    ensure_link

    log_success "Orca server v${version} installed to $INSTALL_DIR"
    log "Start it with: orca-server serve --pairing-address <tailscale-ip-or-hostname>"
    log "Restart any running 'orca-server serve' to pick up a new version."
}

uninstall_server() {
    if [[ "$DRY_RUN" == "true" ]]; then
        [[ -e "$INSTALL_DIR" ]] && log "DRY-RUN: Would remove $INSTALL_DIR"
        link_is_ours && log "DRY-RUN: Would remove $LINK_PATH"
        return 0
    fi
    if link_is_ours; then
        rm -f -- "$LINK_PATH"
        log "Removed $LINK_PATH"
    fi
    if [[ -e "$INSTALL_DIR" ]]; then
        rm -rf -- "$INSTALL_DIR"
        log "Removed $INSTALL_DIR"
    fi
    rm -rf -- "$DATA_HOME/.orca-server.new" "$DATA_HOME/.orca-server.old"
    log_warning "Orca settings/data and any service set up with 'serve service install' were left in place"
    log_success "Orca server uninstalled"
}

show_usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [--update | --uninstall] [--dry-run] [--help]

Install the official Orca server-only build (orcad + bundled Node + orca-ide
CLI) into ${INSTALL_DIR} and link it as ${LINK_PATH}.
Linux x64/arm64 only. Idempotent and non-interactive.

  --update     Install the latest release when it is newer than the installed one
  --uninstall  Remove the install and the orca-server symlink
  --dry-run    Print what would be done

Environment:
  ORCA_SERVER_VERSION   Release to install, e.g. 1.4.261 (default: latest)
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

    if [[ "$action" != "uninstall" && "$(uname -s)" != "Linux" ]]; then
        error_exit "the Orca server-only build is Linux-only; on macOS run 'orca serve' from the desktop app"
    fi

    acquire_lock

    local current target
    case "$action" in
        install)
            current="$(installed_version)"
            if [[ -n "$current" && -x "$INSTALL_DIR/bin/orca-ide" ]]; then
                ensure_link
                log "Orca server v${current} already installed at $INSTALL_DIR (use --update to refresh)"
                return 0
            fi
            install_server "$(resolve_version)"
            ;;
        update)
            current="$(installed_version)"
            target="$(resolve_version)"
            if [[ -n "$current" && "$current" == "$target" && -x "$INSTALL_DIR/bin/orca-ide" ]]; then
                ensure_link
                log_success "Orca server v${current} is already the requested release"
                return 0
            fi
            install_server "$target"
            ;;
        uninstall) uninstall_server ;;
    esac
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
