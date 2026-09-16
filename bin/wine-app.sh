#!/usr/bin/env bash

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROTON_HELPERS="${SCRIPT_DIR}/proton-versions.sh"

if [[ -f "${PROTON_HELPERS}" ]]; then
  source "${PROTON_HELPERS}"
fi

PROTON_DEFAULT="${PROTON_DEFAULT:-system}"

if ! declare -f parse_proton_flag >/dev/null 2>&1; then
  parse_proton_flag() {
    local args=("$@")
    local remaining=()
    local selected_proton=""
    local i=0

    while [[ ${i} -lt ${#args[@]} ]]; do
      case "${args[$i]}" in
      -p)
        i=$((i + 1))
        [[ ${i} -lt ${#args[@]} ]] || die "-p requires a value"
        selected_proton="${args[$i]}"
        ;;
      --proton)
        i=$((i + 1))
        [[ ${i} -lt ${#args[@]} ]] || die "--proton requires a value"
        selected_proton="${args[$i]}"
        ;;
      --proton=*)
        selected_proton="${args[$i]#--proton=}"
        ;;
      *)
        remaining+=("${args[$i]}")
        ;;
      esac
      i=$((i + 1))
    done

    if [[ -n "${selected_proton}" ]]; then
      PROTON_PATH="${selected_proton}"
    else
      PROTON_PATH="${PROTON_PATH:-${PROTONPATH:-}}"
    fi

    REMAINING_ARGS=("${remaining[@]}")
  }
fi

if ! declare -f resolve_proton_executable >/dev/null 2>&1; then
  resolve_proton_executable() {
    local selection="${1:-}"

    if [[ -z "${selection}" ]]; then
      if command -v proton >/dev/null 2>&1; then
        command -v proton
        return 0
      fi
      return 1
    fi

    if [[ -x "${selection}" ]]; then
      printf '%s\n' "${selection}"
      return 0
    fi

    if [[ -d "${selection}" && -x "${selection}/proton" ]]; then
      printf '%s\n' "${selection}/proton"
      return 0
    fi

    if command -v "${selection}" >/dev/null 2>&1; then
      command -v "${selection}"
      return 0
    fi

    return 1
  }
fi

DEFAULT_PREFIX_ROOT="${WINE_PREFIX_ROOT:-${XDG_DATA_HOME:-${HOME}/.local/share}/wine-prefixes}"
DEFAULT_STEAM_ROOT="${STEAM_ROOT:-${HOME}/.steam/root}"
DEFAULT_STEAM_COMPAT_DATA_ROOT="${STEAM_COMPAT_DATA_ROOT:-${HOME}/.local/share/Steam/steamapps/compatdata}"

if [[ -z "${STEAM_COMPAT_DATA_ROOT:-}" && ! -d "${DEFAULT_STEAM_COMPAT_DATA_ROOT}" ]]; then
  DEFAULT_STEAM_COMPAT_DATA_ROOT="/mnt/sdb/SteamLibrary/steamapps/compatdata"
fi

declare -A APP_IDS=(
  [mgsv]="287700"
)

print_main_usage() {
  cat <<EOF
Usage:
  ${SCRIPT_NAME} <subcommand> [options]

Subcommands:
  install    Install or update an app in a Wine prefix using umu-run
  run        Launch an app in a Wine prefix using umu-run
  trainer    Launch a helper app with Proton run inside Steam compatdata
  help       Show help for all subcommands or one subcommand

Examples:
  Prefix + umu-run (standalone Wine prefix):
  ${SCRIPT_NAME} install --prefix my-app --prefix-root "${XDG_DATA_HOME:-$HOME/.local/share}/wine-prefixes" ./setup.exe
  ${SCRIPT_NAME} install --prefix mgsv-mods --proton ge ./ModManagerSetup.exe
  ${SCRIPT_NAME} run --prefix my-app ./Installed/MyApp/App.exe
  ${SCRIPT_NAME} run --prefix mgsv-mods --proton cachyos ./ModTools/Launcher.exe --game-id 287700

  Trainer inside Steam compatdata (uses game id or app key):
  ${SCRIPT_NAME} trainer --trainer 'D:\\Tools\\Trainer.exe' --game mgsv --proton ge
  ${SCRIPT_NAME} trainer --trainer 'D:\\Tools\\Trainer.exe' --game 287700 --proton experimental
  ${SCRIPT_NAME} trainer --trainer 'D:\\Tools\\Trainer.exe' --game 287700 --proton hotfix

  ${SCRIPT_NAME} help install
  ${SCRIPT_NAME} help trainer
EOF
}

print_install_usage() {
  cat <<EOF
Usage:
  ${SCRIPT_NAME} install [--proton <key|path>] [--prefix-root <dir>] [--prefix <name|path>] [--game-id <id>] [--steam-deck|--no-steam-deck] <installer-path>
  ${SCRIPT_NAME} install [--proton <key|path>] [--prefix-root <dir>] <prefix-name|prefix-path> <installer-path> [game-id]

Install or update any Wine/Proton-compatible app in a prefix using umu-run.

Defaults:
  Prefix root: ${DEFAULT_PREFIX_ROOT}
  GAMEID: 0
  SteamDeck mode: enabled

Examples:
  ${SCRIPT_NAME} install --prefix my-game --proton ge ./setup.exe
  ${SCRIPT_NAME} install --proton experimental my-prefix ./installer.exe 287700

Proton (--proton / -p): short keys from bin/proton-versions.sh (e.g. ge, cachyos, experimental, hotfix),
  a GE shorthand like ge-10-34, any discovered install name, or a full path to a Proton directory.
EOF
}

print_run_usage() {
  cat <<EOF
Usage:
  ${SCRIPT_NAME} run [--proton <key|path>] [--prefix-root <dir>] [--prefix <name|path>] [--game-id <id>] [--steam-deck|--no-steam-deck] <app-exe-path>
  ${SCRIPT_NAME} run [--proton <key|path>] [--prefix-root <dir>] <prefix-name|prefix-path> <app-exe-path> [game-id]

Launch any Wine/Proton-compatible app from a prefix using umu-run.

Defaults:
  Prefix root: ${DEFAULT_PREFIX_ROOT}
  GAMEID: 0
  SteamDeck mode: enabled

Examples:
  ${SCRIPT_NAME} run --prefix my-game --proton ge ./Binaries/Win64/Game.exe
  ${SCRIPT_NAME} run --proton hotfix my-prefix ./launcher.exe --game-id 287700

Proton (--proton / -p): see install help; same resolution as bin/proton-versions.sh.
EOF
}

print_trainer_usage() {
  cat <<EOF
Usage:
  ${SCRIPT_NAME} trainer --trainer <path> [--game <key|app-id|compatdata-path>|--compat-data <path>] [--proton <key|path>] [--compat-root <dir>] [--steam-root <dir>]

Launch helper/trainer apps with Proton's "run" command in Steam compatdata.

Defaults:
  Compatdata root: ${DEFAULT_STEAM_COMPAT_DATA_ROOT}
  Steam root: ${DEFAULT_STEAM_ROOT}
  Proton default: ${PROTON_DEFAULT}

App keys:
$(for key in "${!APP_IDS[@]}"; do printf "  %s -> %s\n" "$key" "${APP_IDS[$key]}"; done | sort)

Examples:
  ${SCRIPT_NAME} trainer --trainer 'D:\\Tools\\Trainer.exe' --game mgsv --proton ge
  ${SCRIPT_NAME} trainer --trainer 'D:\\Tools\\Trainer.exe' --game 287700 --proton cachyos
  ${SCRIPT_NAME} trainer --trainer 'D:\\Tools\\Trainer.exe' --compat-data "${DEFAULT_STEAM_COMPAT_DATA_ROOT}/287700" --proton experimental

Proton (--proton / -p): ge (latest GE-Proton), cachyos, experimental, hotfix, ge-10-32 / ge-10-34,
  or a path to a Proton folder (must contain a proton executable). Default: ${PROTON_DEFAULT}
EOF
}

die() {
  echo "Error: $*" >&2
  exit 1
}

resolve_path() {
  local input_path="$1"

  if [[ "${input_path}" == /* ]]; then
    printf '%s\n' "${input_path}"
    return 0
  fi

  realpath -m -- "${PWD}/${input_path}"
}

resolve_existing_file() {
  local resolved_path
  resolved_path="$(resolve_path "$1")" || return 1

  [[ -f "${resolved_path}" ]] || return 1
  printf '%s\n' "${resolved_path}"
}

resolve_prefix_path() {
  local prefix_arg="$1"
  local prefix_root="$2"

  if [[ "${prefix_arg}" == /* || "${prefix_arg}" == */* || "${prefix_arg}" == .* ]]; then
    resolve_path "${prefix_arg}"
    return 0
  fi

  realpath -m -- "${prefix_root}/${prefix_arg}"
}

normalize_executable_arg() {
  local input_arg="$1"

  if [[ "${input_arg}" == *\\* || "${input_arg}" == *:* ]]; then
    printf '%s\n' "${input_arg}"
    return 0
  fi

  if [[ "${input_arg}" == /* || "${input_arg}" == ./* || "${input_arg}" == ../* ]]; then
    resolve_path "${input_arg}"
    return 0
  fi

  if [[ -e "${input_arg}" ]]; then
    resolve_path "${input_arg}"
    return 0
  fi

  printf '%s\n' "${input_arg}"
}

normalize_proton_flags() {
  local normalized=()
  local arg
  local expect_value=0

  for arg in "$@"; do
    if [[ "${expect_value}" -eq 1 ]]; then
      normalized+=("${arg}")
      expect_value=0
      continue
    fi

    case "${arg}" in
    --proton)
      normalized+=("-p")
      expect_value=1
      ;;
    --proton=*)
      normalized+=("-p" "${arg#--proton=}")
      ;;
    *)
      normalized+=("${arg}")
      ;;
    esac
  done

  if [[ "${expect_value}" -eq 1 ]]; then
    die "--proton requires a value"
  fi

  NORMALIZED_ARGS=("${normalized[@]}")
}

run_umu_subcommand() {
  local mode="$1"
  shift

  normalize_proton_flags "$@"
  parse_proton_flag "${NORMALIZED_ARGS[@]}"
  set -- "${REMAINING_ARGS[@]}"

  local prefix_root="${DEFAULT_PREFIX_ROOT}"
  local prefix_arg=""
  local app_path=""
  local game_id=""
  local steam_deck=1
  local positional=()

  while [[ $# -gt 0 ]]; do
    case "$1" in
    -h | --help)
      if [[ "${mode}" == "install" ]]; then
        print_install_usage
      else
        print_run_usage
      fi
      return 0
      ;;
    --prefix-root)
      [[ -n "${2:-}" ]] || die "--prefix-root requires a value"
      prefix_root="$(resolve_path "$2")"
      shift 2
      ;;
    --prefix)
      [[ -n "${2:-}" ]] || die "--prefix requires a value"
      prefix_arg="$2"
      shift 2
      ;;
    --game-id)
      [[ -n "${2:-}" ]] || die "--game-id requires a value"
      game_id="$2"
      shift 2
      ;;
    --steam-deck)
      steam_deck=1
      shift
      ;;
    --no-steam-deck)
      steam_deck=0
      shift
      ;;
    --)
      shift
      while [[ $# -gt 0 ]]; do
        positional+=("$1")
        shift
      done
      ;;
    -*)
      die "unknown option '$1'"
      ;;
    *)
      positional+=("$1")
      shift
      ;;
    esac
  done

  if [[ -z "${prefix_arg}" ]]; then
    [[ ${#positional[@]} -ge 2 ]] || die "missing required positional args"
    prefix_arg="${positional[0]}"
    app_path="${positional[1]}"
    if [[ -z "${game_id}" && ${#positional[@]} -ge 3 ]]; then
      game_id="${positional[2]}"
    fi
  else
    [[ ${#positional[@]} -ge 1 ]] || die "missing app path"
    app_path="${positional[0]}"
    if [[ -z "${game_id}" && ${#positional[@]} -ge 2 ]]; then
      game_id="${positional[1]}"
    fi
  fi

  local wine_prefix
  wine_prefix="$(resolve_prefix_path "${prefix_arg}" "${prefix_root}")"

  local target_path
  if [[ "${mode}" == "install" ]]; then
    target_path="$(resolve_existing_file "${app_path}")" || die "installer not found at '${app_path}'"
    mkdir -p "${wine_prefix}" || die "failed to create prefix directory '${wine_prefix}'"
  else
    [[ -d "${wine_prefix}" ]] || die "prefix '${wine_prefix}' not found"
    target_path="$(normalize_executable_arg "${app_path}")"
  fi

  command -v umu-run >/dev/null 2>&1 || die "umu-run is required but not found in PATH"

  local resolved_game_id="${game_id:-0}"
  local -a env_vars=(
    "WINEPREFIX=${wine_prefix}"
    "GAMEID=${resolved_game_id}"
    "PROTONPATH=${PROTON_PATH:-${PROTONPATH:-}}"
  )

  if [[ "${steam_deck}" -eq 1 ]]; then
    env_vars+=("SteamDeck=1")
  fi

  env "${env_vars[@]}" umu-run "${target_path}"
}

resolve_compat_data_path() {
  local selection="$1"
  local compat_root="$2"
  local normalized_key="${selection,,}"

  if [[ -n "${APP_IDS[${normalized_key}]:-}" ]]; then
    selection="${APP_IDS[${normalized_key}]}"
  fi

  if [[ "${selection}" =~ ^[0-9]+$ ]]; then
    printf '%s\n' "${compat_root}/${selection}"
    return 0
  fi

  if [[ "${selection}" == /* ]]; then
    printf '%s\n' "${selection}"
    return 0
  fi

  return 1
}

run_trainer_subcommand() {
  local trainer_path=""
  local compat_data_path=""
  local game_selection=""
  local proton_selection=""
  local compat_root="${DEFAULT_STEAM_COMPAT_DATA_ROOT}"
  local steam_root="${DEFAULT_STEAM_ROOT}"

  while [[ $# -gt 0 ]]; do
    case "$1" in
    -t | --trainer)
      [[ -n "${2:-}" ]] || die "--trainer requires a value"
      trainer_path="$2"
      shift 2
      ;;
    -g | --game)
      [[ -n "${2:-}" ]] || die "--game requires a value"
      game_selection="$2"
      shift 2
      ;;
    -c | --compat-data)
      [[ -n "${2:-}" ]] || die "--compat-data requires a value"
      compat_data_path="$2"
      shift 2
      ;;
    --compat-root)
      [[ -n "${2:-}" ]] || die "--compat-root requires a value"
      compat_root="$(resolve_path "$2")"
      shift 2
      ;;
    --steam-root)
      [[ -n "${2:-}" ]] || die "--steam-root requires a value"
      steam_root="$(resolve_path "$2")"
      shift 2
      ;;
    -p | --proton)
      [[ -n "${2:-}" ]] || die "--proton requires a value"
      proton_selection="$2"
      shift 2
      ;;
    -h | --help)
      print_trainer_usage
      return 0
      ;;
    *)
      die "unknown option '$1'"
      ;;
    esac
  done

  [[ -n "${trainer_path}" ]] || die "--trainer is required"

  if [[ -z "${compat_data_path}" ]]; then
    [[ -n "${game_selection}" ]] || die "provide either --game or --compat-data"
    compat_data_path="$(resolve_compat_data_path "${game_selection}" "${compat_root}")" \
      || die "unknown game/app id/path '${game_selection}'"
  fi

  [[ -d "${compat_data_path}" ]] || die "compatdata path '${compat_data_path}' not found"

  local wine_prefix="${compat_data_path}/pfx"
  [[ -d "${wine_prefix}" ]] || die "Wine prefix '${wine_prefix}' not found"
  [[ -d "${steam_root}" ]] || die "Steam root '${steam_root}' not found"

  local proton_executable
  proton_executable="$(resolve_proton_executable "${proton_selection}")" \
    || die "failed to resolve proton executable"

  STEAM_COMPAT_DATA_PATH="${compat_data_path}" \
  STEAM_COMPAT_CLIENT_INSTALL_PATH="${steam_root}" \
  WINEPREFIX="${wine_prefix}" \
    "${proton_executable}" run "${trainer_path}"
}

main() {
  local subcommand="${1:-}"

  case "${subcommand}" in
  install)
    shift
    run_umu_subcommand "install" "$@"
    ;;
  run)
    shift
    run_umu_subcommand "run" "$@"
    ;;
  trainer)
    shift
    run_trainer_subcommand "$@"
    ;;
  help)
    case "${2:-}" in
    install) print_install_usage ;;
    run) print_run_usage ;;
    trainer) print_trainer_usage ;;
    "" | all) print_main_usage ;;
    *) die "unknown help topic '${2}'" ;;
    esac
    ;;
  -h | --help | "")
    print_main_usage
    ;;
  *)
    die "unknown subcommand '${subcommand}'"
    ;;
  esac
}

main "$@"
