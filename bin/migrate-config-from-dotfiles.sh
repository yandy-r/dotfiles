#!/usr/bin/env bash
# Manual use only. Not wired into bootstrap, playbooks, or CI.
#
# Promote a config directory away from the dotfiles-managed symlink.
#
# On machines set up with an older playbook, directories like ~/.claude,
# ~/.codex, and ~/.config/opencode are symlinks into ~/.config/dotfiles/<src>.
# This script makes the home-side path a real directory by:
#   1. If the home path is a symlink into dotfiles AND dotfiles/<src> still
#      exists, moving dotfiles/<src> → home path.
#   2. If dotfiles/<src> was already removed (repo pulled clean), restoring
#      its last tracked contents from git history into the home path.
#   3. Leaving the home path as a regular user-owned directory.
#
# Safe to re-run: idempotent and refuses to touch unrelated symlinks.
#
# Entries use the form `<dotfiles-src>[:<home-relative-dest>]`:
#   .claude                    → dotfiles/.claude           → ~/.claude
#   .codex                     → dotfiles/.codex            → ~/.codex
#   .opencode:.config/opencode → dotfiles/.opencode         → ~/.config/opencode
#
# Usage:
#   bin/migrate-config-from-dotfiles.sh                         # all defaults
#   bin/migrate-config-from-dotfiles.sh .claude                 # subset
#   bin/migrate-config-from-dotfiles.sh .opencode:.config/opencode
#   CONFIG_RESTORE_REF=<sha> bin/migrate-config-from-dotfiles.sh .codex
#
# When CONFIG_RESTORE_REF is unset, the restore ref is auto-resolved per
# entry: the script locates the commit that deleted <src> and uses its
# parent. This handles stacked removal commits correctly (where HEAD~1
# may already be past the deletion).

set -euo pipefail

DOTFILES_DIR="${DOTFILES_DIR:-$HOME/.config/dotfiles}"
# Explicit override. If unset, the script resolves a ref per entry by
# finding the commit that deleted the dir and using its parent.
RESTORE_REF="${CONFIG_RESTORE_REF:-}"

DEFAULT_DIRS=(.claude .codex .opencode:.config/opencode)
TARGETS=("$@")
[[ ${#TARGETS[@]} -eq 0 ]] && TARGETS=("${DEFAULT_DIRS[@]}")

log()  { printf '==> %s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }
die()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

[[ -d "$DOTFILES_DIR/.git" ]] || die "Dotfiles repo not found at $DOTFILES_DIR"

migrate_one() {
    local entry="$1"
    local src_name="${entry%%:*}"
    local dest_rel="${entry#*:}"
    [[ "$dest_rel" == "$entry" ]] && dest_rel="$src_name"
    local name="$src_name"
    local home_path="$HOME/$dest_rel"
    local src_path="$DOTFILES_DIR/$src_name"

    log "[$name] target: $home_path"

    if [[ -d "$home_path" && ! -L "$home_path" ]]; then
        log "[$name] already a real directory. Skipping."
        [[ -e "$src_path" ]] && warn "[$name] stale $src_path still in dotfiles; remove manually if intentional."
        return 0
    fi

    if [[ -L "$home_path" ]]; then
        local target
        target="$(readlink -f "$home_path" || true)"
        log "[$name] symlink -> $target"

        if [[ "$target" != "$src_path" ]]; then
            warn "[$name] symlink points to '$target', not '$src_path'. Skipping."
            return 0
        fi

        if [[ -d "$src_path" ]]; then
            log "[$name] moving $src_path -> $home_path"
            rm "$home_path"
            mv "$src_path" "$home_path"
        else
            log "[$name] symlink dangles; removing."
            rm "$home_path"
        fi
    fi

    if [[ ! -e "$home_path" ]]; then
        local ref="$RESTORE_REF"
        if [[ -z "$ref" ]]; then
            local delete_commit
            delete_commit="$(cd "$DOTFILES_DIR" && git log --diff-filter=D --format=%H -1 -- "$src_name" 2>/dev/null || true)"
            [[ -n "$delete_commit" ]] \
                || die "[$name] cannot locate a commit that deleted $src_name/. Set CONFIG_RESTORE_REF to a commit that still contains it."
            ref="${delete_commit}~1"
        fi
        log "[$name] missing. Restoring from $ref"
        (
            cd "$DOTFILES_DIR"
            git rev-parse --verify "$ref" >/dev/null 2>&1 \
                || die "[$name] git ref '$ref' not found. Set CONFIG_RESTORE_REF."
            git ls-tree -d --name-only "$ref" -- "$src_name" >/dev/null 2>&1 \
                || die "[$name] ref '$ref' does not contain $src_name/. Pick an older CONFIG_RESTORE_REF."

            local tmp
            tmp="$(mktemp -d)"
            trap 'rm -rf "$tmp"' EXIT
            git archive "$ref" "$src_name" | tar -x -C "$tmp"
            [[ -d "$tmp/$src_name" ]] || die "[$name] archive extraction produced no $src_name/."
            mkdir -p "$(dirname "$home_path")"
            mv "$tmp/$src_name" "$home_path"
        )
        log "[$name] restored from $ref"
    fi

    [[ -d "$home_path" && ! -L "$home_path" ]] \
        || die "[$name] post-check failed: $home_path is not a real directory."
    log "[$name] done."
}

for entry in "${TARGETS[@]}"; do
    migrate_one "$entry"
done
