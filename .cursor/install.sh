#!/usr/bin/env bash
# Cloud Agent install script for the dotfiles Go + shell mono-repo.
#
# Idempotent: safe to run repeatedly and against a warm/snapshotted filesystem.
# Installs the toolchain the repository's own verification suite expects
# (CLAUDE.md -> Testing & Verification): Go 1.26 (module go directive), the Go
# dev tools (golangci-lint built with the matching Go, goimports), shellcheck +
# zsh for the shell verification, the Python linters from requirements.txt, and
# the Node dev dependencies. Finishes by building the `dotf` CLI as a smoke test.

set -euo pipefail

GO_VERSION="1.26.0"
GOLANGCI_VERSION="v2.6.1"
GOARCH_TARBALL="linux-amd64"

log() { printf '\n=== %s ===\n' "$*"; }

# Elevate only when not already root; the default Cloud Agent user has
# passwordless sudo. Fall back to running the command directly if sudo is absent.
as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    "$@"
  fi
}

# --- System packages ------------------------------------------------------
log "System packages (shellcheck, zsh)"
if command -v apt-get >/dev/null 2>&1; then
  as_root apt-get update -qq
  as_root apt-get install -y -qq --no-install-recommends shellcheck zsh ca-certificates curl git
fi

# --- Go toolchain (module requires 'go 1.26') -----------------------------
log "Go ${GO_VERSION}"
need_go=1
if [ -x /usr/local/go/bin/go ] && /usr/local/go/bin/go version 2>/dev/null | grep -q "go${GO_VERSION} "; then
  need_go=0
fi
if [ "$need_go" -eq 1 ]; then
  tmp="$(mktemp -d)"
  curl -fsSL --retry 4 --retry-delay 4 \
    -o "$tmp/go.tgz" \
    "https://dl.google.com/go/go${GO_VERSION}.${GOARCH_TARBALL}.tar.gz"
  as_root rm -rf /usr/local/go
  as_root tar -C /usr/local -xzf "$tmp/go.tgz"
  rm -rf "$tmp"
fi
# Expose go on the default PATH regardless of shell type.
as_root ln -sf /usr/local/go/bin/go /usr/local/bin/go
as_root ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

export PATH="/usr/local/go/bin:${HOME}/go/bin:${HOME}/.local/bin:${PATH}"
# Pin to the just-installed toolchain; never try to download go1.26 via GOTOOLCHAIN.
export GOTOOLCHAIN=local

# --- Go dev tools ---------------------------------------------------------
# golangci-lint refuses to run when built with an older Go than the module
# target, so build it from source with the Go 1.26 toolchain above.
log "Go dev tools (golangci-lint ${GOLANGCI_VERSION}, goimports)"
go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_VERSION}"
go install golang.org/x/tools/cmd/goimports@latest
as_root ln -sf "${HOME}/go/bin/golangci-lint" /usr/local/bin/golangci-lint
as_root ln -sf "${HOME}/go/bin/goimports" /usr/local/bin/goimports

# --- Python lint tools ----------------------------------------------------
log "Python tools (ruff, sqlfluff, black, ansible-lint, ...)"
if [ -f requirements.txt ]; then
  # PEP 668 external environment: prefer --break-system-packages, fall back to --user.
  pip install --quiet --break-system-packages -r requirements.txt \
    || pip install --quiet --user -r requirements.txt \
    || echo "warning: python tool install reported issues (non-fatal)"
fi

# --- Node dev dependencies ------------------------------------------------
log "Node dev dependencies"
if [ -f package-lock.json ]; then
  npm ci
elif [ -f package.json ]; then
  npm install
fi

# --- Persist PATH + GOTOOLCHAIN for interactive/login shells --------------
log "Persist PATH for login shells"
marker="# --- cloud-agent dev env PATH (managed by .cursor/install.sh) ---"
if ! grep -qF "$marker" "${HOME}/.bashrc" 2>/dev/null; then
  {
    printf '\n%s\n' "$marker"
    printf 'export GOTOOLCHAIN=local\n'
    printf 'export PATH="/usr/local/go/bin:$HOME/go/bin:$HOME/.local/bin:$PATH"\n'
  } >>"${HOME}/.bashrc"
fi

# --- Smoke test: build the CLI -------------------------------------------
log "Build dotf (smoke test)"
go build -o dotf ./cmd/dotf
./dotf version

log "install complete"
