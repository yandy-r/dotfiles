#!/usr/bin/env bash
#
# Push private env dirs (~/.config/env, ~/.config/environment.d) to the other
# hosts in parallel.
#
# Usage: sync-private-env.sh [-n] [-d] [host ...]
#   -n      dry run (rsync --dry-run)
#   -d      delete remote files missing locally (rsync --delete)
#   host    override the default host list

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
  .config/env
  .config/environment.d
)

rsync_opts=(-azR --no-implied-dirs -e "ssh -o BatchMode=yes -o ConnectTimeout=5")
while getopts nd opt; do
  case "$opt" in
    n) rsync_opts+=(--dry-run) ;;
    d) rsync_opts+=(--delete) ;;
    *) exit 2 ;;
  esac
done
shift $((OPTIND - 1))
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
    changes="$(grep -c '^[<>c*]' "$tmp/$h")"
    echo "ok    $h ($changes changed)"
  else
    echo "FAIL  $h (rc=$?)" >&2
    sed 's/^/      /' "$tmp/$h" >&2
    failed=1
  fi
done
exit "$failed"
