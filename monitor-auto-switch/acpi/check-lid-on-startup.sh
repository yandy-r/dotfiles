#!/bin/bash
set -euo pipefail

# On initial boot, wait for compositor to initialize sockets
if pgrep -x Hyprland >/dev/null 2>&1 && [ -n "${HYPRLAND_INSTANCE_SIGNATURE:-}" ]; then
  # Hyprland is already running (this is a reload)
  sleep 0.2
else
  # Initial boot, give compositor time to initialize
  sleep 3
fi

. /etc/acpi/hypr-utils.sh || exit 0
log "Startup workspace distribution check beginning."

if ! export_hypr_env; then
  log "Hyprland env not ready, skipping startup layout."
  exit 0
fi

detect_external_monitor || true

if external_connected; then
  log "Boot with external monitor connected. Moving workspaces 1-9 to external."
  # Switch to workspace 10 before moving workspaces
  hypr dispatch workspace 10
  sleep 0.2
  # Move workspaces 1-9 to external monitor
  # shellcheck disable=SC2086
  move_ws_to_monitor "$EXTERNAL_MONITOR" $EXTERNAL_WS
  sleep 0.25
  # Focus external monitor and workspace 1
  reassert_primary_external
  notify "Display" "Startup: workspaces 1-9 on external"
else
  log "Boot without external monitor. All workspaces on laptop."
  notify "Display" "Startup: laptop-only mode"
fi

log "Startup workspace distribution complete."
