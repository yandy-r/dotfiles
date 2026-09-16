#!/usr/bin/env bash

set -euo pipefail

log() {
  if [[ -n "${HYPR_NOTIFY_DEBUG:-}" ]]; then
    printf '[hypr-focus-app-window] %s\n' "$*" >&2
  fi
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1
}

if ! require_cmd hyprctl || ! require_cmd jq; then
  log "hyprctl and jq are required"
  exit 1
fi

desktop_entry="${NOTIFY_DESKTOP_ENTRY:-${SWAYNC_DESKTOP_ENTRY:-}}"
app_name="${NOTIFY_APP_NAME:-${SWAYNC_APP_NAME:-}}"
clients_json="$(hyprctl clients -j 2>/dev/null || printf '[]')"
active_workspace_id="$(hyprctl activeworkspace -j 2>/dev/null | jq -r '.id // empty' 2>/dev/null || true)"

if [[ "${clients_json}" == "[]" ]]; then
  log "no Hypr clients available"
  exit 1
fi

normalize() {
  local value="${1:-}"
  value="${value##*/}"
  value="${value%.desktop}"
  value="${value,,}"
  value="${value//[^a-z0-9]/}"
  printf '%s\n' "${value}"
}

declare -a raw_candidates=()

if [[ -n "${desktop_entry}" ]]; then
  raw_candidates+=("${desktop_entry}")
fi

if [[ -n "${app_name}" ]]; then
  raw_candidates+=("${app_name}")
fi

declare -a normalized_candidates=()
declare -A seen_candidates=()

append_candidate() {
  local value
  value="$(normalize "${1:-}")"

  if [[ -z "${value}" ]]; then
    return
  fi

  if [[ -n "${seen_candidates["$value"]:-}" ]]; then
    return
  fi

  normalized_candidates+=("${value}")
  seen_candidates["$value"]=1
}

expand_aliases() {
  case "${1:-}" in
    firefox | orgmozillafirefox)
      printf '%s\n' "firefox"
      ;;
    googlechrome | comgooglechrome | googlechromestable)
      printf '%s\n' "googlechrome"
      printf '%s\n' "chromium"
      ;;
    chromium | orgchromiumchromium)
      printf '%s\n' "chromium"
      printf '%s\n' "googlechrome"
      ;;
    visualstudiocode | vscode | comvisualstudiocode)
      printf '%s\n' "code"
      printf '%s\n' "visualstudiocode"
      ;;
    vscodium | comvscodiumcodium)
      printf '%s\n' "codium"
      printf '%s\n' "vscodium"
      ;;
    orgtelegramdesktop | telegramdesktop)
      printf '%s\n' "telegram"
      printf '%s\n' "telegramdesktop"
      ;;
    *)
      printf '%s\n' "${1:-}"
      ;;
  esac
}

for candidate in "${raw_candidates[@]}"; do
  normalized="$(normalize "${candidate}")"
  while IFS= read -r alias_candidate; do
    append_candidate "${alias_candidate}"
  done < <(expand_aliases "${normalized}")
done

if (( ${#normalized_candidates[@]} == 0 )); then
  log "no notification metadata available"
  exit 1
fi

candidates_json="$(printf '%s\n' "${normalized_candidates[@]}" | jq -R . | jq -s .)"
workspace_arg="${active_workspace_id:-null}"

target="$(
  jq -r \
    --argjson candidates "${candidates_json}" \
    --argjson active_workspace "${workspace_arg}" \
    '
    def norm($value):
      ($value // "") | ascii_downcase | gsub("[^a-z0-9]+"; "");

    def score_client($client):
      [
        $candidates[] as $candidate
        | (
            if norm($client.class) == $candidate then 200
            elif norm($client.initialClass) == $candidate then 180
            elif (norm($client.class) | contains($candidate)) or ($candidate | contains(norm($client.class))) then 120
            elif (norm($client.initialClass) | contains($candidate)) or ($candidate | contains(norm($client.initialClass))) then 100
            else empty
            end
          )
      ] | max // empty;

    [
      .[]
      | . as $client
      | (score_client($client)) as $base
      | select($base != null)
      | {
          address,
          workspace_id: (.workspace.id // -1),
          score: ($base + (if (.workspace.id // -1) == $active_workspace then 25 else 0 end))
        }
    ]
    | sort_by(.score, .workspace_id)
    | reverse
    | .[0]
    | if . == null then empty else "\(.workspace_id)\t\(.address)" end
    ' <<<"${clients_json}"
)"

if [[ -z "${target}" ]]; then
  log "no matching client found for ${normalized_candidates[*]}"
  exit 1
fi

target_workspace="${target%%$'\t'*}"
target_address="${target#*$'\t'}"

if [[ -n "${target_workspace}" && "${target_workspace}" != "-1" ]]; then
  hyprctl dispatch workspace "${target_workspace}" >/dev/null 2>&1 || true
fi

if [[ -n "${target_address}" ]]; then
  hyprctl dispatch focuswindow "address:${target_address}" >/dev/null 2>&1 || true
  hyprctl dispatch bringactivetotop >/dev/null 2>&1 || true
  log "focused ${target_address} on workspace ${target_workspace}"
  exit 0
fi

exit 1
