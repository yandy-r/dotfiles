# shellcheck shell=bash
# ================================================================
# FZF Setup (bash) - Platform-Aware Configuration
# ================================================================
# Linked to ~/.fzf.bash by `dotf link` and sourced by setup_fzf in
# common/tools.sh (which falls back to this repo file when the link is absent).
# Mirrors zsh/fzf.zsh. Bash 3.2 compatible. Silent when fzf is absent or old.

_dotf_fzf_setup() {
  local brew_prefix="${DOTFILES_HOMEBREW_PATH:-}" dir out

  # Detect Homebrew path for cross-platform compatibility
  if [[ -z "$brew_prefix" ]]; then
    if [[ "$(uname -s)" == "Darwin" ]]; then
      if [[ "$(uname -m)" == "arm64" ]]; then
        brew_prefix="/opt/homebrew"
      else
        brew_prefix="/usr/local"
      fi
    else
      brew_prefix="/home/linuxbrew/.linuxbrew"
    fi
    DOTFILES_HOMEBREW_PATH="$brew_prefix"
  fi

  # Add fzf to PATH if installed via Homebrew
  if [[ -d "$brew_prefix/opt/fzf/bin" && ":$PATH:" != *":$brew_prefix/opt/fzf/bin:"* ]]; then
    PATH="${PATH:+${PATH}:}$brew_prefix/opt/fzf/bin"
  fi

  command -v fzf >/dev/null 2>&1 || return 0

  # fzf >= 0.48 prints its own integration. An older fzf rejects --bash, which
  # makes both paths below fail without output, so fall through to the packaged
  # scripts.
  if declare -F _cached_eval >/dev/null 2>&1; then
    _cached_eval fzf fzf --bash 2>/dev/null && return 0
  else
    out="$(fzf --bash 2>/dev/null)" && [[ -n "$out" ]] && eval "$out" && return 0
  fi

  # Packaged key-bindings/completion: Homebrew, Debian/Ubuntu, Arch/Fedora.
  for dir in \
    "$brew_prefix/opt/fzf/shell" \
    /usr/share/doc/fzf/examples \
    /usr/share/fzf \
    /usr/share/fzf/shell; do
    if [[ -f "$dir/key-bindings.bash" ]]; then
      # shellcheck source=/dev/null
      source "$dir/key-bindings.bash"
      # shellcheck source=/dev/null
      [[ -f "$dir/completion.bash" ]] && source "$dir/completion.bash"
      return 0
    fi
  done
  return 0
}

_dotf_fzf_setup
unset -f _dotf_fzf_setup
