#!/bin/bash
set -euo pipefail
sleep 0.3
. /etc/acpi/hypr-utils.sh || exit 0
log "Lid open event received."

if ! export_hypr_env; then
  log "Hyprland not found, exiting."
  exit 0
fi

# Let Hyprland settle after env export
sleep 0.5
detect_external_monitor || true

if external_connected_retry 40 0.10; then
  log "External ${EXTERNAL_MONITOR} connected. Moving workspaces 1-9 to external."
  # Switch to workspace 10 before moving workspaces
  hypr dispatch workspace 10
  sleep 0.2
  # Move workspaces 1-9 to external monitor
  # shellcheck disable=SC2086
  move_ws_to_monitor "$EXTERNAL_MONITOR" $EXTERNAL_WS
  sleep 0.25
  # Focus external monitor and switch to workspace 1
  reassert_primary_external
  notify "Display" "Lid opened: workspaces 1-9 on external"
  log "Workspaces moved to external monitor."
else
  log "No external monitor detected. All workspaces on laptop."
  notify "Display" "Lid opened: laptop-only mode"
fi
