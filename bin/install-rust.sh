#!/usr/bin/env bash
# ================================================================
# Rust toolchain (rustup + cargo) installer
# ================================================================
# Bootstraps the Rust toolchain via the official rustup installer so the
# manifest's toolchain-backend crates become installable, mirroring the legacy
# bootstrap.sh install_rust() step. Idempotent: a host that already has rustup
# is left untouched (a self-update is best-effort). Non-interactive by design —
# the installer is driven with `-y`, independent of NON_INTERACTIVE.
# https://rustup.rs/
# ================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[RUST]"

# Extra crates the legacy install_rust() provisioned. Installed best-effort so a
# single crate's build failure never fails the toolchain bootstrap.
readonly CARGO_CRATES=(neocmakelsp bws)

DRY_RUN="${DRY_RUN:-false}"

log() { echo "${LOG_PREFIX} $*" >&2; }
log_success() { echo "${LOG_PREFIX} ✓ $*" >&2; }
log_warning() { echo "${LOG_PREFIX} ⚠ $*" >&2; }
log_error() { echo "${LOG_PREFIX} ✗ $*" >&2; }

error_exit() {
    log_error "$1"
    exit "${2:-1}"
}

# source_cargo_env makes cargo/rustc visible in the current shell after a fresh
# rustup install, before the user's shell profile is reloaded.
source_cargo_env() {
    if [[ -f "$HOME/.cargo/env" ]]; then
        # shellcheck disable=SC1091
        source "$HOME/.cargo/env"
    else
        export PATH="$HOME/.cargo/bin:$PATH"
    fi
}

# install_rustup installs the toolchain via the official installer, or reports
# an existing install (best-effort self-update). Returns non-zero only when a
# fresh install fails.
install_rustup() {
    if command -v rustup >/dev/null 2>&1; then
        log "rustup already installed: $(rustup --version 2>/dev/null || echo unknown)"
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY-RUN: Would run: rustup self update"
            return 0
        fi
        rustup self update >/dev/null 2>&1 || log_warning "rustup self-update failed (continuing)"
        return 0
    fi

    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: Would run: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y ..."
        return 0
    fi

    log "Installing Rust toolchain via rustup..."
    if ! curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs |
        sh -s -- -y --no-modify-path --default-toolchain stable --profile default; then
        error_exit "rustup installation failed"
    fi
    source_cargo_env
    log_success "Rust toolchain installed: $(rustc --version 2>/dev/null || echo unknown)"
}

# install_crates installs CARGO_CRATES best-effort (a converged host is a fast
# no-op via `cargo install`'s own up-to-date check).
install_crates() {
    if ! command -v cargo >/dev/null 2>&1; then
        source_cargo_env
    fi
    if ! command -v cargo >/dev/null 2>&1; then
        log_warning "cargo not on PATH; skipping crate installation"
        return 0
    fi

    local crate
    for crate in "${CARGO_CRATES[@]}"; do
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY-RUN: Would run: cargo install $crate"
            continue
        fi
        log "Installing crate: $crate"
        cargo install "$crate" || log_warning "cargo install $crate failed (continuing)"
    done
}

show_usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [--dry-run] [--help]

Bootstrap the Rust toolchain (rustup + cargo) and provision a small set of
cargo crates. Idempotent and non-interactive.
EOF
}

main() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run) DRY_RUN=true; shift ;;
            --help | -h) show_usage; exit 0 ;;
            *) error_exit "Unknown option: $1. Use --help for usage." ;;
        esac
    done

    install_rustup
    install_crates
    log_success "Rust toolchain provisioning complete"
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
