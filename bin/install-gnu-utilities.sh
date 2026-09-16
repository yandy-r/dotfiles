#!/usr/bin/env bash
# ================================================================
# GNU utilities (macOS) installer
# ================================================================
# Installs GNU coreutils/findutils and friends via Homebrew on macOS so the
# shell config's GNU-tool assumptions hold, mirroring the legacy bootstrap.sh
# install_gnu_utilities() step. A no-op on non-macOS / non-Homebrew hosts, where
# these tools are already the system default. Idempotent: already-installed
# formulae are skipped.
# ================================================================

set -euo pipefail

readonly LOG_PREFIX="[GNU-UTILS]"

# The GNU formulae the shell layer expects on macOS (g-prefixed binaries).
readonly GNU_FORMULAE=(
    coreutils findutils gnu-tar gnu-sed gawk grep make
    diffutils gzip less wget bash
)

log() { echo "${LOG_PREFIX} $*" >&2; }

main() {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        log "Not macOS; GNU utilities are the system default here. Skipping."
        return 0
    fi
    if ! command -v brew >/dev/null 2>&1; then
        log "Homebrew not found; cannot install GNU utilities. Skipping."
        return 0
    fi

    local formula
    for formula in "${GNU_FORMULAE[@]}"; do
        if brew list "$formula" >/dev/null 2>&1; then
            log "$formula already installed; skipping"
            continue
        fi
        log "Installing $formula..."
        brew install "$formula" || log "Warning: failed to install $formula (continuing)"
    done
    log "GNU utilities provisioning complete."
}

[[ "${BASH_SOURCE[0]}" == "${0}" ]] && main "$@"
