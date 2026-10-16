#!/usr/bin/env bash
#
# Push agent configs (opencode, omp) to the other hosts in parallel.
#
# Usage: sync-agent-configs.sh [-n] [host ...]
#   -n      dry run (rsync --dry-run)
#   host    override the default host list
#
# Paths are relative to $HOME and sent with `rsync --relative`, so a single
# rsync (one ssh connection) per host carries everything and recreates the
# directory layout remotely. Missing paths are skipped, so optional omp
# directories can be listed before they exist.

set -uo pipefail

hosts=(
  ubdevsrv
  ubsrv
  g7
  xps
  m1plasma
  yrpls-mbp16
)

paths=(
  # opencode
  .config/opencode/opencode.json
  .config/opencode/opencode-mem.jsonc
  # omp: user config only. Everything else under ~/.omp is per-host state
  # (sessions, *.db, memories, logs, cache, natives, run) and must not sync.
  .omp/agent/config.yml
  .omp/agent/models.yml
  .omp/agent/AGENTS.md
  .omp/agent/SYSTEM.md
  .omp/agent/agents
  .omp/agent/commands
  .omp/agent/extensions
  .omp/agent/hooks
  .omp/agent/rules
  .omp/agent/skills
  .omp/agent/tools
  .omp/plugins/package.json
)

rsync_opts=(-azR --no-implied-dirs --exclude=node_modules -e "ssh -o BatchMode=yes -o ConnectTimeout=5")
if [[ "${1:-}" == "-n" ]]; then
  rsync_opts+=(--dry-run)
  shift
fi
(($#)) && hosts=("$@")

cd "$HOME" || exit 1
src=()
for p in "${paths[@]}"; do
  [[ -e "$p" ]] && src+=("$p")
done
((${#src[@]})) || {
  echo "nothing to sync" >&2
  exit 1
}

self="$(hostname -s)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
trap 'echo "interrupted" >&2; kill 0 2>/dev/null; exit 130' INT TERM

declare -A pids
for h in "${hosts[@]}"; do
  [[ "$h" == "$self" ]] && continue
  rsync "${rsync_opts[@]}" --itemize-changes "${src[@]}" "$h": >"$tmp/$h" 2>&1 &
  pids[$h]=$!
done

failed=0
for h in "${hosts[@]}"; do
  [[ -v "pids[$h]" ]] || continue
  if wait "${pids[$h]}"; then
    changes="$(grep -c '^[<>c]' "$tmp/$h")"
    echo "ok    $h ($changes changed)"
  else
    echo "FAIL  $h (rc=$?)" >&2
    sed 's/^/      /' "$tmp/$h" >&2
    failed=1
  fi
done
exit "$failed"
