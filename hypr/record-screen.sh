#!/usr/bin/env sh
# Toggle screen recording with a codec that matches the current machine.
set -eu

mode="${1:-screen}"

notify() {
  if command -v notify-send >/dev/null 2>&1; then
    notify-send "Screen recording" "$1"
  fi
}

has_ffmpeg_encoder() {
  encoder_name="$1"

  command -v ffmpeg >/dev/null 2>&1 || return 1
  ffmpeg -hide_banner -encoders 2>/dev/null | grep -Eq "[[:space:]]${encoder_name}([[:space:]]|$)"
}

build_codec_candidates() {
  if [ -n "${HYPR_WF_RECORDER_CODEC:-}" ]; then
    printf '%s\n' "$HYPR_WF_RECORDER_CODEC"
    return 0
  fi

  if has_ffmpeg_encoder "h264_nvenc"; then
    printf '%s\n' "h264_nvenc"
  fi

  if has_ffmpeg_encoder "h264_vaapi"; then
    printf '%s\n' "h264_vaapi"
  fi

  if has_ffmpeg_encoder "libx264"; then
    printf '%s\n' "libx264"
  fi
}

start_recording() {
  codec="$1"
  output_file="$2"
  log_file="$3"
  geometry="${4:-}"

  : >"$log_file"
  if [ -n "$geometry" ]; then
    wf-recorder -g "$geometry" -c "$codec" -f "$output_file" 2>"$log_file" &
  else
    wf-recorder -c "$codec" -f "$output_file" 2>"$log_file" &
  fi
  recorder_pid=$!

  sleep 1
  if kill -0 "$recorder_pid" 2>/dev/null; then
    if [ -n "$geometry" ]; then
      notify "Started region (${codec})"
    else
      notify "Started screen (${codec})"
    fi
    wait "$recorder_pid"
    return $?
  fi

  wait "$recorder_pid"
}

pick_geometry() {
  case "$mode" in
    screen)
      return 0
      ;;
    region)
      if ! command -v slurp >/dev/null 2>&1; then
        notify "Failed: slurp is required for region recording"
        exit 1
      fi

      geometry="$(slurp)" || exit 0
      if [ -z "$geometry" ]; then
        exit 0
      fi

      printf '%s\n' "$geometry"
      ;;
    *)
      echo "Usage: $0 [screen|region]" >&2
      exit 1
      ;;
  esac
}

recordings_dir="${HYPR_SCREENRECORD_DIR:-$HOME/Pictures/Recordings}"

if pgrep -x wf-recorder >/dev/null 2>&1; then
  pkill -INT wf-recorder
  notify "Stopped"
  exit 0
fi

mkdir -p "$recordings_dir"

codecs="$(build_codec_candidates || true)"
if [ -z "$codecs" ]; then
  notify "Failed: no supported ffmpeg encoder found for wf-recorder"
  exit 1
fi

geometry="$(pick_geometry)"

file_prefix="screen"
if [ "$mode" = "region" ]; then
  file_prefix="region"
fi

output_file="${recordings_dir}/${file_prefix}-$(date +%F_%H-%M-%S).mp4"
log_file="$(mktemp -t wf-recorder.XXXXXX.log)"
trap 'rm -f "$log_file"' EXIT HUP INT TERM

last_error=""
for codec in $codecs; do
  if start_recording "$codec" "$output_file" "$log_file" "$geometry"; then
    exit 0
  fi

  last_error="$(tail -n 1 "$log_file" 2>/dev/null || true)"

  if [ -n "${HYPR_WF_RECORDER_CODEC:-}" ]; then
    break
  fi
done

if [ -n "$last_error" ]; then
  notify "Failed: ${last_error}"
else
  notify "Failed to start wf-recorder"
fi

exit 1
