#!/usr/bin/env bash

set -euo pipefail

if ! command -v rofi >/dev/null 2>&1; then
  notify-send "Calculator" "rofi is not installed"
  exit 1
fi

if ! command -v qalc >/dev/null 2>&1; then
  notify-send "Calculator" "qalc is not installed"
  exit 1
fi

if ! command -v wl-copy >/dev/null 2>&1; then
  notify-send "Calculator" "wl-copy is not installed"
  exit 1
fi

config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/qalculate"
mkdir -p "$config_dir"

expression="$(
  printf '' | rofi -dmenu -p 'Calc copy' \
    -theme-str '
      window { width: 24%; }
      listview { lines: 0; }
    '
)"

if [[ -z "${expression}" ]]; then
  exit 0
fi

result="$(
  qalc -t "$expression" 2>/dev/null \
    | awk 'NF { line = $0 } END { print line }'
)"

if [[ -z "${result}" ]]; then
  notify-send "Calculator" "No result for: ${expression}"
  exit 1
fi

printf '%s' "$result" | wl-copy
notify-send "Calculator" "${expression} = ${result}"
