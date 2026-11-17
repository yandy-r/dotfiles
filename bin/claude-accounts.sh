#!/usr/bin/env bash
# ================================================================
# claude-accounts — share one Claude Code setup across several accounts
# ================================================================
# Each extra subscription lives in its own CLAUDE_CONFIG_DIR
# (~/.config/claude-<name>) so it keeps its own login. Everything that is not
# tied to the login (settings, CLAUDE.md, agents, skills, hooks, plugins) and,
# by default, the session data (projects/, history.jsonl, file-history/, ...)
# is symlinked to the main config dir (~/.claude). Install a plugin or change a
# setting once and every account sees it; `claude --resume` in a project shows
# the sessions from every account.
#
# .claude.json (login, onboarding, trust) stays per account, but its MCP config
# is synced from the main one: user-scope servers (mcpServers) and local-scope
# servers (projects[<dir>].mcpServers, what a plain `claude mcp add` writes).
# Main wins; a server removed from main is removed from the accounts, servers
# added inside an account are kept. The sync runs on `link` and before every
# `run`, so add MCP servers in the main config.
#
# Never touched (per account): .credentials.json, the rest of .claude.json,
# policy-limits*, remote-settings*, caches, live-session state (sessions/,
# session-env/, shell-snapshots/, ide/, daemon/, ...).
#
#   claude-accounts.sh link apple iot     symlink shared items (creates dirs)
#   claude-accounts.sh link --all --copy  copy instead (one-way main -> account)
#   claude-accounts.sh status [name...]   show what is linked / copied / local
#   claude-accounts.sh unlink apple       detach: replace links with copies
#   claude-accounts.sh mcp --all          only sync MCP servers
#   claude-accounts.sh run apple [args]   sync MCP, then run claude as that account
#
# Run `claude-accounts.sh --help` for every option.
# ================================================================

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME

MAIN_DIR="${CLAUDE_MAIN_DIR:-$HOME/.claude}"
ACCOUNTS_ROOT="${CLAUDE_ACCOUNTS_ROOT:-${XDG_CONFIG_HOME:-$HOME/.config}}"
PREFIX="${CLAUDE_ACCOUNTS_PREFIX:-claude-}"
BACKUP_ROOT="${CLAUDE_ACCOUNTS_BACKUP_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/claude-accounts}"

# Config shared by default: everything you set up once.
CONFIG_ITEMS=(
    settings.json
    keybindings.json
    CLAUDE.md
    AGENTS.md
    statusline-command.sh
    agents
    commands
    skills
    hooks
    output-styles
    themes
    plugins
)

# Session data shared by default (--no-sessions keeps it per account).
SESSION_ITEMS=(
    projects
    history.jsonl
    file-history
    todos
    plans
    tasks
    session-data
    transcripts
)

MODE="link" # link | copy
SESSIONS=true
MERGE=true
SYNC_MCP=true
DRY_RUN=false
ALL=false
BACKUP_DIR=""      # set lazily, one per run and account

# Per-account record of the MCP server names synced last time, so servers
# removed from main can be removed from the account too.
MCP_STATE=".claude-accounts-mcp.json"

# MCP_JQ rewrites an account's .claude.json ($m[0] = main .claude.json, $s =
# state) so its MCP config matches main. Only MCP keys change: the account's
# own servers stay, main's servers overwrite same-named ones, servers synced
# before but gone from main are dropped, and .mcp.json approvals are unioned.
read -r -d '' MCP_JQ <<'JQ' || true
def strip($prev): with_entries(select(.key | IN($prev[]) | not));
def put($k; $v): if ($v | length) > 0 or has($k) then .[$k] = $v else . end;
def mcp_paths:
    (.projects // {}) | to_entries[]
    | select([.value.mcpServers, .value.enabledMcpjsonServers, .value.disabledMcpjsonServers]
             | map(. // [] | length) | add > 0)
    | .key;
$m[0] as $main
| put("mcpServers"; (.mcpServers // {} | strip($s.user // [])) + ($main.mcpServers // {}))
| reduce ([($main | mcp_paths), (($s.projects // {}) | keys[])] | unique | .[]) as $p (.;
    ($main.projects[$p] // {}) as $mp
    | (.projects[$p] // {}) as $cur
    | ($cur
        | put("mcpServers"; (.mcpServers // {} | strip($s.projects[$p] // [])) + ($mp.mcpServers // {}))
        | put("enabledMcpjsonServers"; (.enabledMcpjsonServers // []) + ($mp.enabledMcpjsonServers // []) | unique)
        | put("disabledMcpjsonServers"; (.disabledMcpjsonServers // []) + ($mp.disabledMcpjsonServers // []) | unique)
      ) as $new
    | if $new == $cur then . else .projects[$p] = $new end)
JQ

# MCP_STATE_JQ lists the server names main defines, per scope.
read -r -d '' MCP_STATE_JQ <<'JQ' || true
{
    user: (.mcpServers // {} | keys),
    projects: ((.projects // {})
        | map_values(.mcpServers // {} | keys)
        | with_entries(select(.value | length > 0)))
}
JQ

c_ok="" c_warn="" c_err="" c_dim="" c_off=""
if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
    c_ok=$'\e[32m' c_warn=$'\e[33m' c_err=$'\e[31m' c_dim=$'\e[2m' c_off=$'\e[0m'
fi

log() { printf '%s\n' "$*"; }
warn() { printf '%s⚠ %s%s\n' "$c_warn" "$*" "$c_off" >&2; }
die() {
    printf '%s✗ %s%s\n' "$c_err" "$*" "$c_off" >&2
    exit "${2:-1}"
}

# run prints the action and executes it unless --dry-run.
run() {
    if [[ "$DRY_RUN" == true ]]; then
        printf '%s  [dry-run] %s%s\n' "$c_dim" "$*" "$c_off"
    else
        "$@"
    fi
}

usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [options] <command> [name...]

Share one Claude Code setup (main: $MAIN_DIR) with account config dirs
named $ACCOUNTS_ROOT/${PREFIX}<name>, each keeping its own login.

Commands:
  link <name...>|--all     Share config + sessions into the account(s). Creates
                           missing account dirs. Re-run any time: it repairs
                           links that were replaced by regular files.
  unlink <name...>|--all   Detach: replace every link with a real copy of the
                           main item, so the account stands alone again.
  status [name...]         Show each shared item: linked, copy, local, missing.
  mcp <name...>|--all      Only sync MCP servers from the main .claude.json.
  list                     List account dirs and whether they are logged in.
  run <name> [args...]     Sync MCP servers, then run claude with
                           CLAUDE_CONFIG_DIR set to the account. Everything
                           after <name> is passed to claude.

MCP sync (link, mcp, run): user-scope servers (mcpServers) and local-scope
servers (projects[<dir>].mcpServers) from $(main_json)
are written into each account's .claude.json. Main wins on name clashes;
servers removed from main are removed from the account; servers added only in
the account are kept. Add servers in the main config.

Options (link):
  --copy           Copy instead of symlinking (one-way main -> account; newer
                   files in the account are kept, nothing is deleted).
  --no-sessions    Keep session data (${SESSION_ITEMS[*]})
                   per account.
  --no-merge       Do not merge the account's own sessions into main before
                   linking (they are still backed up).
  --no-mcp         Skip the MCP sync (link, run).
  --all            Every existing ${PREFIX}* dir.
  -n, --dry-run    Show what would change.
  -h, --help       Show this help.

Anything replaced is moved to $BACKUP_ROOT/<name>-<timestamp>/ first.
Close running sessions of an account before linking it.

Environment: CLAUDE_MAIN_DIR, CLAUDE_ACCOUNTS_ROOT, CLAUDE_ACCOUNTS_PREFIX,
CLAUDE_ACCOUNTS_BACKUP_DIR.
EOF
}

# ---------------------------------------------------------------- helpers

is_session_item() {
    local item="$1" s
    for s in "${SESSION_ITEMS[@]}"; do
        [[ "$s" == "$item" ]] && return 0
    done
    return 1
}

selected_items() {
    printf '%s\n' "${CONFIG_ITEMS[@]}"
    if [[ "$SESSIONS" == true ]]; then
        printf '%s\n' "${SESSION_ITEMS[@]}"
    fi
}

account_dir() { printf '%s/%s%s\n' "$ACCOUNTS_ROOT" "$PREFIX" "$1"; }

# main_json is the .claude.json of the main config: ~/.claude.json for the
# default ~/.claude, otherwise inside the dir (CLAUDE_CONFIG_DIR behaviour).
main_json() {
    if [[ "$MAIN_DIR" == "$HOME/.claude" ]]; then
        printf '%s\n' "$HOME/.claude.json"
    else
        printf '%s\n' "$MAIN_DIR/.claude.json"
    fi
}

validate_name() {
    local name="$1"
    [[ -n "$name" ]] || die "empty account name"
    [[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || die "invalid account name: '$name'"
}

all_accounts() {
    local d
    for d in "$ACCOUNTS_ROOT/$PREFIX"*; do
        [[ -d "$d" && ! -L "$d" ]] || continue
        local name="${d##*/"$PREFIX"}"
        [[ -n "$name" ]] || continue # a bare "claude-" dir
        printf '%s\n' "$name"
    done
}

resolve() {
    # Portable "readlink -f" (macOS ships it since 12.3; fall back to python3).
    readlink -f "$1" 2>/dev/null || python3 -I -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$1"
}

points_to_main() {
    local path="$1" item="$2"
    [[ -L "$path" ]] || return 1
    [[ "$(readlink "$path")" == "$MAIN_DIR/$item" ]] && return 0
    [[ -e "$path" && "$(resolve "$path")" == "$(resolve "$MAIN_DIR/$item")" ]]
}

ensure_backup_dir() {
    local name="$1"
    if [[ -z "$BACKUP_DIR" ]]; then
        BACKUP_DIR="$BACKUP_ROOT/$name-$(date +%Y%m%d-%H%M%S)"
        run mkdir -p "$BACKUP_DIR"
    fi
}

# backup moves an account item out of the way (real files/dirs only).
backup() {
    local name="$1" path="$2" item="$3"
    ensure_backup_dir "$name"
    run mv "$path" "$BACKUP_DIR/$item"
    log "    backed up -> $BACKUP_DIR/$item"
}

# merge_history appends the account's prompt history to main, de-duplicated and
# ordered by timestamp, written atomically next to the main file.
merge_history() {
    local src="$1" dst="$2"
    [[ -s "$src" ]] || return 0
    if [[ "$DRY_RUN" == true ]]; then
        printf '%s  [dry-run] merge %s into %s%s\n' "$c_dim" "$src" "$dst" "$c_off"
        return 0
    fi
    local tmp="$dst.claude-accounts.$$"
    touch "$dst"
    if command -v jq >/dev/null 2>&1; then
        awk '!seen[$0]++' "$dst" "$src" |
            jq -c -s 'sort_by(.timestamp // 0) | .[]' >"$tmp" 2>/dev/null ||
            awk '!seen[$0]++' "$dst" "$src" >"$tmp"
    else
        awk '!seen[$0]++' "$dst" "$src" >"$tmp"
    fi
    chmod 600 "$tmp"
    mv "$tmp" "$dst"
}

# merge_dir copies the account's files into main, keeping whichever copy of a
# file is newer (a session continued in the account wins over the stale copy
# main still has from when the dir was cloned).
merge_dir() {
    local src="$1" dst="$2"
    if command -v rsync >/dev/null 2>&1; then
        run rsync -a --update "$src/" "$dst/"
    else
        run cp -Rp -u "$src/." "$dst/" 2>/dev/null || run cp -Rpn "$src/." "$dst/"
    fi
}

merge_into_main() {
    local acct_path="$1" item="$2"
    local main_path="$MAIN_DIR/$item"
    if [[ -d "$acct_path" ]]; then
        run mkdir -p "$main_path"
        merge_dir "$acct_path" "$main_path"
    elif [[ -f "$acct_path" && "$item" == history.jsonl ]]; then
        merge_history "$acct_path" "$main_path"
    fi
    log "    merged sessions into main"
}

copy_from_main() {
    local main_path="$1" acct_path="$2"
    if [[ -d "$main_path" ]]; then
        run mkdir -p "$acct_path"
        if command -v rsync >/dev/null 2>&1; then
            run rsync -a --update "$main_path/" "$acct_path/"
        else
            run cp -Rp "$main_path/." "$acct_path/"
        fi
    else
        run cp -p "$main_path" "$acct_path"
    fi
}

# ---------------------------------------------------------------- link

link_item() {
    local name="$1" acct="$2" item="$3"
    local main_path="$MAIN_DIR/$item" acct_path="$acct/$item"

    if [[ ! -e "$main_path" && ! -L "$main_path" ]]; then
        if ! is_session_item "$item"; then
            if [[ -e "$acct_path" || -L "$acct_path" ]]; then
                log "  ${c_dim}skip   $item (not in main; left as is)${c_off}"
            fi
            return 0
        fi
        # Session data always lands in main, so create it there first.
        if [[ "$item" == *.jsonl ]]; then
            run touch "$main_path"
        else
            run mkdir -p "$main_path"
        fi
    fi

    if [[ "$MODE" == link ]]; then
        if points_to_main "$acct_path" "$item"; then
            log "  ${c_ok}ok${c_off}     $item"
            return 0
        fi
        if [[ -L "$acct_path" ]]; then
            log "  relink $item (was -> $(readlink "$acct_path"))"
            run rm "$acct_path"
        elif [[ -e "$acct_path" ]]; then
            log "  link   $item (replacing local copy)"
            if is_session_item "$item" && [[ "$MERGE" == true ]]; then
                merge_into_main "$acct_path" "$item"
            fi
            if [[ -f "$acct_path" ]] && cmp -s "$acct_path" "$main_path"; then
                run rm "$acct_path" # identical to main; nothing to keep
            else
                backup "$name" "$acct_path" "$item"
            fi
        else
            log "  link   $item"
        fi
        run ln -s "$main_path" "$acct_path"
        return 0
    fi

    # copy mode
    if [[ -L "$acct_path" ]]; then
        log "  copy   $item (replacing link)"
        run rm "$acct_path"
    elif [[ -f "$acct_path" ]]; then
        if cmp -s "$acct_path" "$main_path"; then
            log "  ${c_ok}ok${c_off}     $item"
            return 0
        fi
        log "  copy   $item (local copy differs)"
        backup "$name" "$acct_path" "$item"
    else
        log "  copy   $item"
    fi
    copy_from_main "$main_path" "$acct_path"
}

# ---------------------------------------------------------------- mcp

# mcp_render prints the account's .claude.json with main's MCP config applied.
mcp_render() {
    local acct="$1" state="$1/$MCP_STATE" s='{}'
    if [[ -s "$state" ]] && jq -e 'type == "object"' "$state" >/dev/null 2>&1; then
        s="$(<"$state")"
    fi
    jq --slurpfile m "$(main_json)" --argjson s "$s" "$MCP_JQ" "$acct/.claude.json"
}

# mcp_skip_reason prints why an account cannot be synced (empty if it can).
mcp_skip_reason() {
    local acct="$1" src
    src="$(main_json)"
    if ! command -v jq >/dev/null 2>&1; then
        printf 'needs jq'
    elif [[ ! -f "$src" ]]; then
        printf 'no %s' "$src"
    elif [[ ! -f "$acct/.claude.json" ]]; then
        printf 'no .claude.json yet; log in first'
    elif [[ "$(resolve "$acct/.claude.json")" == "$(resolve "$src")" ]]; then
        printf '.claude.json is the main one'
    fi
}

# sync_mcp writes main's MCP servers into the account's .claude.json. Never
# fails: problems are reported and the account is left as it was. With quiet
# (used by `run`) only problems are printed.
sync_mcp() {
    local acct="$1" quiet="${2:-false}"
    local dst="$acct/.claude.json" state="$acct/$MCP_STATE"
    local reason src
    src="$(main_json)"
    reason="$(mcp_skip_reason "$acct")"
    if [[ -n "$reason" ]]; then
        [[ "$quiet" == true ]] || log "  ${c_dim}skip   mcp ($reason)${c_off}"
        return 0
    fi

    local state_json counts tmp="$dst.claude-accounts.$$"
    if ! state_json="$(jq -c "$MCP_STATE_JQ" "$src" 2>/dev/null)" ||
        ! mcp_render "$acct" >"$tmp" 2>/dev/null || [[ ! -s "$tmp" ]]; then
        rm -f "$tmp"
        warn "mcp: could not parse $src or $dst; MCP sync skipped"
        return 0
    fi
    counts="$(jq -r '"\(.user | length) user, \([.projects[] | length] | add // 0) local"' <<<"$state_json")"

    if jq -e --slurpfile a "$dst" '. == $a[0]' "$tmp" >/dev/null 2>&1; then
        rm -f "$tmp"
        [[ "$quiet" == true ]] || log "  ${c_ok}ok${c_off}     mcp ($counts server(s))"
    elif [[ "$DRY_RUN" == true ]]; then
        rm -f "$tmp"
        log "  mcp    sync $counts server(s) into .claude.json"
        printf '%s  [dry-run] write %s%s\n' "$c_dim" "$dst" "$c_off"
        return 0
    else
        [[ "$quiet" == true ]] || log "  mcp    sync $counts server(s) into .claude.json"
        chmod 600 "$tmp"
        if ! mv "$tmp" "$dst"; then
            rm -f "$tmp"
            warn "mcp: could not write $dst"
            return 0
        fi
    fi

    [[ "$DRY_RUN" == true ]] && return 0
    if printf '%s\n' "$state_json" >"$state.$$" && chmod 600 "$state.$$"; then
        mv "$state.$$" "$state" || rm -f "$state.$$"
    else
        rm -f "$state.$$"
    fi
    return 0
}

# mcp_state is the status column for an account's MCP config.
mcp_state() {
    local acct="$1" reason
    reason="$(mcp_skip_reason "$acct")"
    if [[ -n "$reason" ]]; then
        printf '%s-%s (%s)' "$c_dim" "$c_off" "$reason"
    elif mcp_render "$acct" 2>/dev/null | jq -e --slurpfile a "$acct/.claude.json" '. == $a[0]' >/dev/null 2>&1; then
        printf '%sin sync%s' "$c_ok" "$c_off"
    else
        printf '%sout of sync%s (run: %s mcp %s)' "$c_warn" "$c_off" "$SCRIPT_NAME" "${acct##*/"$PREFIX"}"
    fi
}

cmd_mcp() {
    local name acct
    for name in "$@"; do
        validate_name "$name"
        acct="$(account_dir "$name")"
        [[ -d "$acct" ]] || die "no such account dir: $acct"
        log "${c_ok}==>${c_off} $name ($acct) [mcp]"
        sync_mcp "$acct"
    done
}

cmd_link() {
    local names=("$@") name acct item
    [[ -d "$MAIN_DIR" ]] || die "main config dir not found: $MAIN_DIR"
    for name in "${names[@]}"; do
        validate_name "$name"
        acct="$(account_dir "$name")"
        [[ "$(resolve "$acct" 2>/dev/null || true)" != "$(resolve "$MAIN_DIR")" ]] ||
            die "$acct is the main config dir"
        BACKUP_DIR=""
        log "${c_ok}==>${c_off} $name ($acct) [$MODE$([[ "$SESSIONS" == true ]] || printf ', no sessions')]"
        if [[ ! -d "$acct" ]]; then
            log "  create $acct"
            run mkdir -p "$acct"
            [[ "$DRY_RUN" == true ]] || chmod 700 "$acct"
        fi
        while IFS= read -r item; do
            link_item "$name" "$acct" "$item"
        done < <(selected_items)
        if [[ "$SYNC_MCP" == true ]]; then
            sync_mcp "$acct"
        fi
        if [[ ! -f "$acct/.credentials.json" && ! -f "$acct/.claude.json" ]]; then
            log "  ${c_warn}not logged in yet:${c_off} $SCRIPT_NAME run $name  (then /login)"
        fi
    done
}

# ---------------------------------------------------------------- unlink

cmd_unlink() {
    local name acct item
    for name in "$@"; do
        validate_name "$name"
        acct="$(account_dir "$name")"
        [[ -d "$acct" ]] || die "no such account dir: $acct"
        log "${c_ok}==>${c_off} $name ($acct) [detach]"
        for item in "${CONFIG_ITEMS[@]}" "${SESSION_ITEMS[@]}"; do
            local acct_path="$acct/$item"
            points_to_main "$acct_path" "$item" || continue
            log "  copy   $item"
            run rm "$acct_path"
            copy_from_main "$MAIN_DIR/$item" "$acct_path"
        done
    done
}

# ---------------------------------------------------------------- status

item_state() {
    local acct_path="$1" item="$2"
    local main_path="$MAIN_DIR/$item"
    if points_to_main "$acct_path" "$item"; then
        printf '%slinked%s' "$c_ok" "$c_off"
    elif [[ -L "$acct_path" ]]; then
        printf '%slink -> %s%s' "$c_warn" "$(readlink "$acct_path")" "$c_off"
    elif [[ ! -e "$acct_path" ]]; then
        if [[ -e "$main_path" ]]; then
            printf '%smissing%s' "$c_warn" "$c_off"
        else
            printf '%s-%s' "$c_dim" "$c_off"
        fi
    elif [[ -f "$acct_path" && -f "$main_path" ]] && cmp -s "$acct_path" "$main_path"; then
        printf 'copy (same)'
    elif [[ -e "$main_path" ]]; then
        printf '%slocal (differs)%s' "$c_warn" "$c_off"
    else
        printf 'local'
    fi
}

cmd_status() {
    local name acct item
    for name in "$@"; do
        validate_name "$name"
        acct="$(account_dir "$name")"
        if [[ ! -d "$acct" ]]; then
            log "$name: ${c_err}no dir${c_off} ($acct)"
            continue
        fi
        log "${c_ok}==>${c_off} $name ($acct)"
        for item in "${CONFIG_ITEMS[@]}" "${SESSION_ITEMS[@]}"; do
            printf '  %-22s %s\n' "$item" "$(item_state "$acct/$item" "$item")"
        done
        printf '  %-22s %s\n' "mcp (.claude.json)" "$(mcp_state "$acct")"
    done
}

cmd_list() {
    local name acct login
    while IFS= read -r name; do
        acct="$(account_dir "$name")"
        if [[ -f "$acct/.credentials.json" ]] || jq -e '.oauthAccount' "$acct/.claude.json" >/dev/null 2>&1; then
            login="logged in"
        else
            login="${c_warn}not logged in${c_off}"
        fi
        printf '%-10s %s  (%s)\n' "$name" "$acct" "$login"
    done < <(all_accounts)
}

cmd_run() {
    local name="${1:-}"
    [[ -n "$name" ]] || die "usage: $SCRIPT_NAME run <name> [claude args...]"
    shift
    validate_name "$name"
    local acct
    acct="$(account_dir "$name")"
    [[ -d "$acct" ]] || die "no such account dir: $acct (create it with: $SCRIPT_NAME link $name)"
    if [[ "$SYNC_MCP" == true ]]; then
        sync_mcp "$acct" true
    fi
    CLAUDE_CONFIG_DIR="$acct" exec claude "$@"
}

# ---------------------------------------------------------------- main

main() {
    local cmd="" args=()
    while (($#)); do
        # After `run <name>` everything belongs to claude, even our own flags.
        if [[ "$cmd" == run && ${#args[@]} -ge 1 ]]; then
            args+=("$@")
            break
        fi
        case "$1" in
        -h | --help)
            usage
            return 0
            ;;
        --copy) MODE="copy" ;;
        --link) MODE="link" ;;
        --no-sessions) SESSIONS=false ;;
        --sessions) SESSIONS=true ;;
        --no-merge) MERGE=false ;;
        --mcp) SYNC_MCP=true ;;
        --no-mcp) SYNC_MCP=false ;;
        --all) ALL=true ;;
        -n | --dry-run) DRY_RUN=true ;;
        --)
            shift
            args+=("$@")
            break
            ;;
        -*)
            # `run` passes unknown flags straight through to claude
            if [[ "$cmd" == run ]]; then
                args+=("$@")
                break
            fi
            die "unknown option: $1 (see --help)" 2
            ;;
        *)
            if [[ -z "$cmd" ]]; then
                cmd="$1"
            else
                args+=("$1")
            fi
            ;;
        esac
        shift
    done

    case "$cmd" in
    link | unlink | status | mcp)
        if [[ "$ALL" == true || ( ${#args[@]} -eq 0 && "$cmd" == status ) ]]; then
            args=()
            local n
            while IFS= read -r n; do args+=("$n"); done < <(all_accounts)
            ((${#args[@]})) || die "no ${PREFIX}* dirs under $ACCOUNTS_ROOT"
        elif ((${#args[@]} == 0)); then
            die "usage: $SCRIPT_NAME $cmd <name...>|--all" 2
        fi
        "cmd_$cmd" "${args[@]}"
        ;;
    list) cmd_list ;;
    run) cmd_run "${args[@]}" ;;
    "")
        usage
        return 2
        ;;
    *) die "unknown command: $cmd (see --help)" 2 ;;
    esac
}

main "$@"
