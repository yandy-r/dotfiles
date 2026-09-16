#!/usr/bin/env bash
# Sync local private env dirs (~/.config/env, ~/.config/environment.d) to remote hosts.
# Host list: ~/.config/sync-private-env/hosts (see sync-private-env.hosts.example).

set -euo pipefail

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
readonly LOG_PREFIX="[sync-private-env]"

CONFIG_BASE="${XDG_CONFIG_HOME:-${HOME}/.config}"
readonly CONFIG_DIR="${CONFIG_BASE}/sync-private-env"
readonly HOSTS_FILE="${CONFIG_DIR}/hosts"
readonly EXAMPLE_HOSTS="${DOTFILES_ROOT:-${HOME}/.config/dotfiles}/bin/sync-private-env.hosts.example"

SRC_ENV="${CONFIG_BASE}/env"
SRC_ENVIRONMENT_D="${CONFIG_BASE}/environment.d"
REMOTE_DEST=".config/"

DRY_RUN=false
LIST_ONLY=false
SINGLE_HOST=""
ADD_HOST=""

usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Rsync private environment directories to every host in:
  ${HOSTS_FILE}

Local paths (must exist):
  ${SRC_ENV}
  ${SRC_ENVIRONMENT_D}

Remote destination (per host): ~/${REMOTE_DEST}

Options:
  -n, --dry-run       Show rsync commands without running them
  -l, --list          Print configured hosts and exit
  -H, --host HOST     Sync only this host (must be in the hosts file)
  -a, --add HOST      Append HOST to the hosts file (creates file from example if missing)
  -e, --edit          Open the hosts file in \$EDITOR
  -h, --help          Show this help

Examples:
  $SCRIPT_NAME
  $SCRIPT_NAME --dry-run
  $SCRIPT_NAME --host yrmbp14
  $SCRIPT_NAME --add myserver
  $SCRIPT_NAME --edit
EOF
    exit "${1:-0}"
}

log() {
    echo "${LOG_PREFIX} $*" >&2
}

error_exit() {
    log "ERROR: $1"
    exit "${2:-1}"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

ensure_hosts_file() {
    if [[ -f "$HOSTS_FILE" ]]; then
        return 0
    fi

    mkdir -p "$CONFIG_DIR"
    if [[ -f "$EXAMPLE_HOSTS" ]]; then
        cp "$EXAMPLE_HOSTS" "$HOSTS_FILE"
        log "Created ${HOSTS_FILE} from example — edit it to add your servers."
    else
        cat >"$HOSTS_FILE" <<'EOF'
# One SSH host alias or user@host per line
EOF
        log "Created empty ${HOSTS_FILE} — add your servers."
    fi
}

read_hosts() {
    ensure_hosts_file
    local line host
    while IFS= read -r line || [[ -n "$line" ]]; do
        line="${line%%#*}"
        line="${line#"${line%%[![:space:]]*}"}"
        line="${line%"${line##*[![:space:]]}"}"
        [[ -n "$line" ]] || continue
        host="$line"
        if [[ -n "$SINGLE_HOST" && "$host" != "$SINGLE_HOST" ]]; then
            continue
        fi
        printf '%s\n' "$host"
    done <"$HOSTS_FILE"
}

validate_sources() {
    local missing=()
    [[ -d "$SRC_ENV" ]] || missing+=("$SRC_ENV")
    [[ -d "$SRC_ENVIRONMENT_D" ]] || missing+=("$SRC_ENVIRONMENT_D")
    if ((${#missing[@]} > 0)); then
        error_exit "Missing local directories: ${missing[*]}"
    fi
}

add_host() {
    ensure_hosts_file
    local host="$ADD_HOST"
    if grep -qxF "$host" "$HOSTS_FILE" 2>/dev/null; then
        log "Host already listed: ${host}"
        return 0
    fi
    printf '\n%s\n' "$host" >>"$HOSTS_FILE"
    log "Added ${host} to ${HOSTS_FILE}"
}

edit_hosts() {
    ensure_hosts_file
    local editor="${EDITOR:-vi}"
    command_exists "$editor" || error_exit "Editor not found: ${editor}"
    "$editor" "$HOSTS_FILE"
}

run_rsync() {
    local host="$1"
    local -a rsync_args=(-av)

    if [[ "$DRY_RUN" == "true" ]]; then
        rsync_args+=(--dry-run)
    fi

    log "→ ${host}:${REMOTE_DEST}"
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY-RUN: rsync ${rsync_args[*]} ${SRC_ENV} ${SRC_ENVIRONMENT_D} ${host}:${REMOTE_DEST}"
    fi

    rsync "${rsync_args[@]}" "$SRC_ENV" "$SRC_ENVIRONMENT_D" "${host}:${REMOTE_DEST}"
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            -n | --dry-run)
                DRY_RUN=true
                shift
                ;;
            -l | --list)
                LIST_ONLY=true
                shift
                ;;
            -H | --host)
                [[ $# -ge 2 ]] || error_exit "--host requires a value"
                SINGLE_HOST="$2"
                shift 2
                ;;
            -a | --add)
                [[ $# -ge 2 ]] || error_exit "--add requires a value"
                ADD_HOST="$2"
                shift 2
                ;;
            -e | --edit)
                edit_hosts
                exit 0
                ;;
            -h | --help)
                usage 0
                ;;
            *)
                error_exit "Unknown option: $1 (try --help)"
                ;;
        esac
    done
}

main() {
    parse_args "$@"

    command_exists rsync || error_exit "rsync not found in PATH"

    if [[ -n "$ADD_HOST" ]]; then
        add_host
        exit 0
    fi

    mapfile -t hosts < <(read_hosts)

    if ((${#hosts[@]} == 0)); then
        if [[ -n "$SINGLE_HOST" ]]; then
            error_exit "Host not in ${HOSTS_FILE}: ${SINGLE_HOST}"
        fi
        error_exit "No hosts in ${HOSTS_FILE}. Add one with: ${SCRIPT_NAME} --add HOST"
    fi

    if [[ "$LIST_ONLY" == "true" ]]; then
        printf '%s\n' "${hosts[@]}"
        exit 0
    fi

    validate_sources

    local failed=()
    local host
    for host in "${hosts[@]}"; do
        if ! run_rsync "$host"; then
            failed+=("$host")
        fi
    done

    if ((${#failed[@]} > 0)); then
        error_exit "Failed for: ${failed[*]}"
    fi

    log "Done (${#hosts[@]} host(s))."
}

main "$@"
