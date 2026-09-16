#!/usr/bin/env bash
# tmux-session-manager.sh — reliable start/stop for tmux under systemd
# Handles save-file validation, server health checks, and restore sequencing.
set -euo pipefail

TMUX_BIN="${TMUX_BIN:-/usr/bin/tmux}"
RESURRECT_DIR="${HOME}/.local/share/tmux/resurrect"
RESURRECT_SCRIPTS="${HOME}/.tmux/plugins/tmux-resurrect/scripts"
MAX_WAIT=10  # seconds to wait for server readiness

log() { echo "[tmux-session-manager] $*"; }

wait_for_server() {
    local i=0
    while [ $i -lt "$MAX_WAIT" ]; do
        if "$TMUX_BIN" list-sessions &>/dev/null; then
            return 0
        fi
        sleep 1
        i=$((i + 1))
    done
    return 1
}

# Validate that the "last" save file is non-empty; fall back to the most
# recent non-empty save if it isn't.
fix_last_symlink() {
    local last_link="${RESURRECT_DIR}/last"
    [ -d "$RESURRECT_DIR" ] || return 0

    # If the symlink target is non-empty, nothing to do
    if [ -L "$last_link" ] && [ -s "$last_link" ]; then
        return 0
    fi

    log "last save file is empty or missing — searching for a valid one"
    local candidate
    candidate=$(command ls -t "${RESURRECT_DIR}"/tmux_resurrect_*.txt 2>/dev/null \
        | while read -r f; do [ -s "$f" ] && echo "$f" && break; done)

    if [ -n "$candidate" ]; then
        ln -sf "$(basename "$candidate")" "$last_link"
        log "re-linked last → $(basename "$candidate")"
    else
        log "no valid save files found — nothing to restore"
    fi
}

do_start() {
    log "starting tmux server"

    # Ensure save directory exists
    mkdir -p "$RESURRECT_DIR"

    # Fix any corrupted last-save symlink before restore
    fix_last_symlink

    # Start a detached session (creates the server)
    "$TMUX_BIN" new-session -d -s __boot__ 2>/dev/null || true

    if ! wait_for_server; then
        log "ERROR: tmux server did not start within ${MAX_WAIT}s"
        exit 1
    fi
    log "server is ready"

    # Run resurrect restore if the script and a valid save exist
    local restore_script="${RESURRECT_SCRIPTS}/restore.sh"
    local last_link="${RESURRECT_DIR}/last"
    if [ -x "$restore_script" ] && [ -L "$last_link" ] && [ -s "$last_link" ]; then
        log "restoring sessions from $(readlink "$last_link")"
        "$restore_script" || log "WARNING: restore exited with $?"
    else
        log "skipping restore (no script or no valid save)"
    fi
}

do_save() {
    # Only save if the server is actually alive — prevents 0-byte files
    if ! "$TMUX_BIN" list-sessions &>/dev/null; then
        log "server not running — skipping save (preserving existing save files)"
        return 0
    fi

    local save_script="${RESURRECT_SCRIPTS}/save.sh"
    if [ -x "$save_script" ]; then
        log "saving sessions"
        "$save_script" || log "WARNING: save exited with $?"
    fi
}

do_stop() {
    do_save
    if "$TMUX_BIN" list-sessions &>/dev/null; then
        log "killing tmux server"
        "$TMUX_BIN" kill-server || true
    fi
}

case "${1:-}" in
    start)   do_start ;;
    stop)    do_stop  ;;
    save)    do_save  ;;
    *)       echo "Usage: $0 {start|stop|save}" >&2; exit 1 ;;
esac
