#!/usr/bin/env bash
# Manual use only. Not wired into bootstrap, playbooks, or CI.
# Export Cursor authentication state to a private JSON file.

set -euo pipefail
umask 077
# Never leak secret values through xtrace if invoked with `bash -x`.
{ set +x; } 2>/dev/null

SCRIPT_NAME="$(basename "${BASH_SOURCE[0]}")"
readonly SCRIPT_NAME
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR

DEFAULT_DOTFILES_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
if [[ ! -f "${DEFAULT_DOTFILES_ROOT}/common/platform.sh" ]]; then
    DEFAULT_DOTFILES_ROOT="${HOME}/.config/dotfiles"
fi
readonly DEFAULT_DOTFILES_ROOT
DOTFILES_ROOT="${DOTFILES_ROOT:-${DEFAULT_DOTFILES_ROOT}}"
readonly DOTFILES_ROOT

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

if [[ ! -f "${DOTFILES_ROOT}/common/platform.sh" ]]; then
    log_error "Platform helper not found: ${DOTFILES_ROOT}/common/platform.sh"
    exit 1
fi

# shellcheck source=../common/platform.sh
source "${DOTFILES_ROOT}/common/platform.sh"

readonly -a ACCESS_TOKEN_KEYS=(
    "cursorAuth/accessToken"
    "cursorAuth/token"
)
readonly -a MACHINE_ID_KEYS=(
    "storage.serviceMachineId"
    "storage.machineId"
    "telemetry.machineId"
)

DRY_RUN=false
CHECK_ONLY=false
OUTPUT_IS_DEFAULT=true
OUTPUT_PATH="${XDG_CONFIG_HOME:-${HOME}/.config}/secrets/cursor.json"

usage() {
    cat <<EOF
Usage: $SCRIPT_NAME [OPTIONS]

Export Cursor's access token and machine ID from its local state database.
Secret values are never printed. The output file is written with mode 600.

Options:
  --dry-run        Check state and report paths and key presence without writing
  --check          Verify dependencies, database, keys, and value formats
  --output PATH    Write to PATH instead of the default location
  -h, --help       Show this help

Default output:
  ${XDG_CONFIG_HOME:-${HOME}/.config}/secrets/cursor.json
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --check)
                CHECK_ONLY=true
                shift
                ;;
            --output)
                if [[ $# -lt 2 || -z "$2" ]]; then
                    log_error "--output requires a path"
                    exit 1
                fi
                OUTPUT_PATH="$2"
                OUTPUT_IS_DEFAULT=false
                shift 2
                ;;
            -h | --help)
                usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1 (try --help)"
                exit 1
                ;;
        esac
    done

    if [[ "$DRY_RUN" == "true" && "$CHECK_ONLY" == "true" ]]; then
        log_error "--dry-run and --check cannot be used together"
        exit 1
    fi
}

cursor_user_directory() {
    local os="$1"

    case "$os" in
        macos)
            printf '%s\n' "${HOME}/Library/Application Support/Cursor/User"
            ;;
        debian | redhat | arch | suse | linux | wsl)
            printf '%s\n' "${HOME}/.config/Cursor/User"
            ;;
        windows)
            log_error "Windows is unsupported: POSIX modes cannot enforce private file ACLs there."
            return 1
            ;;
        *)
            log_error "Unsupported operating system: ${os}"
            return 1
            ;;
    esac
}

check_dependencies() {
    local dependency
    local missing=false

    for dependency in sqlite3 jq; do
        if ! command -v "$dependency" >/dev/null 2>&1; then
            log_error "Required command not found: ${dependency}. Install it with your system package manager."
            missing=true
        fi
    done

    [[ "$missing" == "false" ]]
}

query_value() {
    local database="$1"
    local key="$2"
    local value

    if ! value="$(sqlite3 -readonly "$database" \
        "SELECT value FROM ItemTable WHERE key = '${key}' LIMIT 1;" 2>/dev/null)"; then
        log_error "Failed to query Cursor state database in read-only mode."
        return 1
    fi

    printf '%s' "$value"
}

normalize_value() {
    local value="$1"

    if [[ ${#value} -ge 2 && "$value" == \"*\" ]]; then
        printf '%s' "$value" | jq -Rrs 'try fromjson catch .'
    else
        printf '%s' "$value"
    fi
}

read_candidates() {
    local database="$1"
    local value_type="$2"
    shift 2

    local key raw_value normalized_value
    local selected_value=""

    for key in "$@"; do
        raw_value="$(query_value "$database" "$key")" || return 1
        normalized_value="$(normalize_value "$raw_value")" || {
            log_error "Failed to normalize ${value_type} from Cursor state database."
            return 1
        }

        if [[ -n "$normalized_value" ]]; then
            if [[ "$DRY_RUN" == "true" ]]; then
                log_info "Key ${key}: found" >&2
            fi
            if [[ -z "$selected_value" ]]; then
                selected_value="$normalized_value"
            fi
        elif [[ "$DRY_RUN" == "true" ]]; then
            log_info "Key ${key}: missing" >&2
        fi
    done

    printf '%s' "$selected_value"
}

validate_values() {
    local access_token="$1"
    local machine_id="$2"
    local compact_machine_id="${machine_id//-/}"
    local valid=true

    if [[ -z "$access_token" ]]; then
        log_error "Cursor access token not found. Sign in to Cursor, then try again."
        valid=false
    fi
    if [[ -z "$machine_id" ]]; then
        log_error "Cursor machine ID not found. Open Cursor once, then try again."
        valid=false
    elif [[ ! "$compact_machine_id" =~ ^[a-f0-9]{32,}$ ]]; then
        log_error "Cursor machine ID has an unexpected format; refusing to export it."
        valid=false
    fi

    [[ "$valid" == "true" ]]
}

directory_mode() {
    local directory="$1"
    local mode

    if mode="$(stat -c '%a' "$directory" 2>/dev/null)"; then
        printf '%s\n' "$mode"
    elif mode="$(stat -f '%Lp' "$directory" 2>/dev/null)"; then
        printf '%s\n' "$mode"
    else
        log_error "Cannot determine permissions for output directory: ${directory}"
        return 1
    fi
}

secure_output_directory() {
    local directory="$1"
    local is_default="$2"
    local mode permission_bits

    if [[ -L "$directory" ]]; then
        log_error "Output directory must not be a symlink: ${directory}"
        return 1
    fi

    if [[ ! -d "$directory" ]]; then
        install -d -m 700 "$directory"
        return
    fi

    if [[ ! -O "$directory" ]]; then
        log_error "Output directory is not owned by the current user: ${directory}"
        return 1
    fi

    mode="$(directory_mode "$directory")"
    mode="${mode: -3}"
    permission_bits=$((8#$mode))
    if (( (permission_bits & 077) != 0 )); then
        if [[ "$is_default" == "true" ]]; then
            log_warn "Output directory permissions are broader than 700; changing them: ${directory}"
            chmod 700 "$directory"
        else
            log_error "Custom output directory must already be private (700): ${directory}"
            return 1
        fi
    fi
}

write_output() {
    local output_path="$1"
    local access_token="$2"
    local machine_id="$3"
    local os="$4"
    local is_default="$5"
    local output_directory temp_file exported_at

    if [[ -L "$output_path" ]]; then
        log_error "Refusing to write through a symlink: ${output_path}"
        return 1
    fi

    if [[ -d "$output_path" ]]; then
        log_error "Output path is a directory: ${output_path}"
        return 1
    fi

    output_directory="$(dirname -- "$output_path")"
    secure_output_directory "$output_directory" "$is_default"

    temp_file="$(mktemp "${output_directory}/.cursor.json.tmp.XXXXXX")"
    cleanup_temp_file() {
        if [[ -n "${temp_file:-}" && -f "$temp_file" ]]; then
            rm -f -- "$temp_file"
        fi
    }
    trap cleanup_temp_file EXIT

    exported_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    # Pass secrets through stdin, never external process arguments.
    printf '%s\0%s' "$access_token" "$machine_id" | jq -Rs \
        --arg os "$os" \
        --arg exportedAt "$exported_at" \
        'split("\u0000") | {accessToken: .[0], machineId: .[1], os: $os, exportedAt: $exportedAt}' \
        >"$temp_file"
    chmod 600 "$temp_file"
    mv -- "$temp_file" "$output_path"
    temp_file=""
    trap - EXIT
}

main() {
    parse_args "$@"

    local os cursor_user_dir database access_token machine_id
    os="$(detect_os)"
    cursor_user_dir="$(cursor_user_directory "$os")" || exit 1
    database="${cursor_user_dir}/globalStorage/state.vscdb"

    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "Detected OS: ${os}"
        log_info "Cursor database: ${database}"
        log_info "Output path: ${OUTPUT_PATH}"
    fi

    check_dependencies || exit 1

    if [[ ! -f "$database" ]]; then
        log_error "Cursor state database not found: ${database}. Cursor may not be installed or opened yet."
        exit 1
    fi

    access_token="$(read_candidates "$database" "access token" "${ACCESS_TOKEN_KEYS[@]}")" || exit 1
    machine_id="$(read_candidates "$database" "machine ID" "${MACHINE_ID_KEYS[@]}")" || exit 1
    validate_values "$access_token" "$machine_id" || exit 1

    if [[ "$CHECK_ONLY" == "true" ]]; then
        log_success "Cursor secret state is available and valid."
        exit 0
    fi
    if [[ "$DRY_RUN" == "true" ]]; then
        log_success "Dry run passed; no file written."
        exit 0
    fi

    write_output "$OUTPUT_PATH" "$access_token" "$machine_id" "$os" "$OUTPUT_IS_DEFAULT"
    log_success "Cursor secrets exported successfully."
    printf '%s\n' "$OUTPUT_PATH"
}

main "$@"
