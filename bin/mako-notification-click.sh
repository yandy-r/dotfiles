#!/usr/bin/env bash

set -euo pipefail

log() {
  if [[ -n "${HYPR_NOTIFY_DEBUG:-}" ]]; then
    printf '[mako-notification-click] %s\n' "$*" >&2
  fi
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1
}

if ! require_cmd makoctl || ! require_cmd busctl; then
  log "makoctl and busctl are required"
  exit 0
fi

notification_id="${id:-${1:-}}"

if [[ -z "${notification_id}" ]]; then
  log "missing notification id"
  exit 0
fi

# Fire the notification's default action (e.g., open a URL) if one exists.
# Continue regardless — we still need to focus the source app's window.
makoctl invoke -n "${notification_id}" >/dev/null 2>&1 || true
log "attempted invoke for ${notification_id}"

# Extract notification metadata so we can find the source app window.
bus_output="$(busctl --user call org.freedesktop.Notifications /fr/emersion/Mako fr.emersion.Mako ListNotifications 2>/dev/null || true)"

if [[ -n "${bus_output}" ]]; then
  mapfile -t metadata < <(
    python -c '
import re
import sys

notification_id = sys.argv[1]
payload = sys.stdin.read()
match = re.search(rf"\"id\" u {re.escape(notification_id)}(?:\s+)(.*?)(?=(?: \"id\" u \d+\s+)|$)", payload, re.S)
if not match:
    sys.exit(0)

chunk = match.group(1)

def extract(key: str) -> str:
    result = re.search(rf"\"{re.escape(key)}\" s \"([^\"]*)\"", chunk)
    return result.group(1) if result else ""

print(extract("desktop-entry"))
print(extract("app-name"))
' "${notification_id}" <<<"${bus_output}"
  )

  if NOTIFY_DESKTOP_ENTRY="${metadata[0]:-}" \
    NOTIFY_APP_NAME="${metadata[1]:-}" \
    "${HOME}/.bin/hypr-focus-app-window.sh"; then
    log "focused source app for ${notification_id}"
  else
    log "no matching window found for ${notification_id}"
  fi
else
  log "no busctl notification data available for ${notification_id}"
fi

makoctl dismiss -n "${notification_id}" >/dev/null 2>&1 || true
