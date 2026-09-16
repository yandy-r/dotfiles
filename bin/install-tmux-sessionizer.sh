#!/usr/bin/env bash
# ================================================================
# tmux-sessionizer (tms) installer
# ================================================================
# Installs the `tms` tmux-sessionizer binary via cargo, mirroring the legacy
# bootstrap.sh install_tmux_sessionizer() step. Requires the Rust toolchain
# (provisioned by the `runtimes` intent / bin/install-rust.sh). Idempotent: a
# present `tms` is left untouched.
# https://github.com/jrmoulton/tmux-sessionizer
# ================================================================

set -euo pipefail

readonly LOG_PREFIX="[TMS]"
readonly TMS_REPO="https://github.com/jrmoulton/tmux-sessionizer"

log() { echo "${LOG_PREFIX} $*" >&2; }

# ensure_cargo makes a rustup-installed cargo visible when it is not yet on PATH.
ensure_cargo() {
    if command -v cargo >/dev/null 2>&1; then
        return 0
    fi
    if [[ -f "$HOME/.cargo/env" ]]; then
        # shellcheck disable=SC1091
        source "$HOME/.cargo/env"
    fi
    command -v cargo >/dev/null 2>&1
}

main() {
    if command -v tms >/dev/null 2>&1; then
        log "tmux-sessionizer (tms) already installed; skipping"
        return 0
    fi

    if ! ensure_cargo; then
        log "cargo not found; install the Rust toolchain first (--intent runtimes). Skipping."
        return 0
    fi

    log "Installing tmux-sessionizer via cargo (from git)..."
    cargo install --git "$TMS_REPO" tmux-sessionizer
    log "tmux-sessionizer installed as 'tms'."
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
