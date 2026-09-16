#!/usr/bin/env sh
# Screenshot helper: capture → annotate in Satty → save + clipboard on Enter.
# Satty behavior (early-exit, actions-on-enter, output path) is configured
# in ~/.config/satty/config.toml — this script only handles capture mode.
# Usage: screenshot.sh [region|screen|window|quick-region]
set -e

mode="${1:-region}"
screenshots_dir="${HYPR_SCREENSHOT_DIR:-$HOME/Pictures/Screenshots}"
output_pattern="${screenshots_dir}/screenshot-%Y-%m-%d_%H-%M-%S.png"

mkdir -p "$screenshots_dir"

case "$mode" in
  region) grim -g "$(slurp)" - | satty --filename - --output-filename "$output_pattern" ;;
  screen) grim - | satty --filename - --output-filename "$output_pattern" ;;
  window) grim -g "$(hyprctl -j activewindow | jq -r '"\(.at[0]),\(.at[1]) \(.size[0])x\(.size[1])"')" - | satty --filename - --output-filename "$output_pattern" ;;
  quick-region)
    output_file="${screenshots_dir}/screenshot-$(date +%F_%H-%M-%S).png"
    grim -g "$(slurp)" "$output_file" && wl-copy < "$output_file"
    ;;
  *) echo "Usage: $0 [region|screen|window|quick-region]" >&2; exit 1 ;;
esac
