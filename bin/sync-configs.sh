#!/usr/bin/env bash
#
# Push live core configs to the other hosts in parallel: private env dirs and
# the core config files of each agent harness (claude, codex, opencode,
# antigravity, omp, cursor) and zed. Meant for carrying in-flight changes across
# machines during testing; skills, agents, commands, plugins and MCP servers
# live in their own repo and are deliberately not synced.
#
# Usage: sync-configs.sh [-n] [-d] [-k] [-g group]... [-a agent[,agent]...]... [-l] [host ...]
#   -n        dry run (rsync --dry-run)
#   -k        accept host keys of hosts not yet in known_hosts
#             (StrictHostKeyChecking=accept-new; changed keys still fail)
#   -d        delete remote files missing locally (rsync --delete; only
#             affects synced directories, i.e. the env group)
#   -g group  sync only this group (repeatable): env, agents
#   -a list   sync only these agents (comma-separated, repeatable): claude,
#             codex, opencode, antigravity, omp, cursor, zed; default all.
#             Without -g, implies -g agents
#   -l        list the paths that would be sent, then exit
#   host      override the default host list
#
# Paths are relative to $HOME and sent with `rsync --relative`, so a single
# rsync (one ssh connection) per host carries everything and recreates the
# directory layout remotely. Missing paths are skipped, and so are symlinks:
# a symlinked config is owned by a repo checkout, not by this script.

set -uo pipefail

hosts=(
  ubsrv
  g7
  xps
  m1plasma
  yrpls-mbp16
  hstr-bosub
  hstr-bosub2
  eve # use -a omp (to only sync omp configs)
)

env_paths=(
  .config/env
  .config/environment.d
)

# Per-host state (sessions, history, *.db, caches, logs, auth/credentials,
# settings.local.json) must never be listed here. Keys are the -a names;
# all_agents fixes the default order.
all_agents=(claude codex opencode antigravity omp cursor zed)
declare -A agent_paths=(
  [claude]=".claude/settings.json .claude/CLAUDE.md"
  [codex]=".codex/config.toml"
  [opencode]=".config/opencode/opencode.json .config/opencode/opencode-mem.jsonc
    .config/opencode/tui.jsonc .config/opencode/cli.json"
  [antigravity]=".gemini/antigravity-cli/settings.json .gemini/GEMINI.md"
  [omp]=".omp/agent/config.yml .omp/agent/models.yml .omp/agent/AGENTS.md
    .omp/agent/SYSTEM.md .omp/agent/APPEND_SYSTEM.md"
  [cursor]=".cursor/cli-config.json"
  # keymap.json is a dotfiles symlink, so only settings.json is sent
  [zed]=".config/zed/settings.json"
)

usage() { sed -n '9,20s/^# \{0,1\}//p' "$0"; }

rsync_opts=(-azR --no-implied-dirs)
ssh_cmd="ssh -o BatchMode=yes -o ConnectTimeout=5"
groups=()
agents=()
list=0
while getopts ndkg:a:lh opt; do
  case "$opt" in
  n) rsync_opts+=(--dry-run) ;;
  d) rsync_opts+=(--delete) ;;
  k) ssh_cmd+=" -o StrictHostKeyChecking=accept-new" ;;
  g)
    case "$OPTARG" in
    env | agents) groups+=("$OPTARG") ;;
    *)
      echo "unknown group: $OPTARG (expected env or agents)" >&2
      exit 2
      ;;
    esac
    ;;
  a)
    IFS=, read -ra names <<<"$OPTARG"
    for a in "${names[@]}"; do
      [[ -v "agent_paths[$a]" ]] || {
        echo "unknown agent: $a (expected one of: ${all_agents[*]})" >&2
        exit 2
      }
      agents+=("$a")
    done
    ;;
  l) list=1 ;;
  h)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
  esac
done
shift $((OPTIND - 1))
rsync_opts+=(-e "$ssh_cmd")
(($#)) && hosts=("$@")
if ((${#agents[@]})); then
  ((${#groups[@]})) || groups=(agents)
else
  agents=("${all_agents[@]}")
fi
((${#groups[@]})) || groups=(env agents)

paths=()
for g in "${groups[@]}"; do
  case "$g" in
  env) paths+=("${env_paths[@]}") ;;
  agents)
    for a in "${agents[@]}"; do
      read -ra ap <<<"${agent_paths[$a]//$'\n'/ }"
      paths+=("${ap[@]}")
    done
    ;;
  esac
done

cd "$HOME" || exit 1
src=()
for p in "${paths[@]}"; do
  [[ -e "$p" && ! -L "$p" ]] && src+=("$p")
done
((${#src[@]})) || {
  echo "nothing to sync" >&2
  exit 1
}

if ((list)); then
  printf '%s\n' "${src[@]}"
  exit 0
fi

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
