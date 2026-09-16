#!/usr/bin/env bash
set -euo pipefail

# Monitor Auto-Switch Uninstaller
# - Removes ACPI lid/monitor scripts from /etc/acpi
# - Removes ACPI event files from /etc/acpi/events
# - Cleans up runtime files and logs
# - Removes Hyprland exec hooks from the repo-owned generic Hypr config
# - Optionally restarts acpid
# Works on any system with acpid + systemd (service name: acpid)

# Defaults (can be overridden by flags)
DEST_DIR="/etc/acpi"
EVENTS_DIR="/etc/acpi/events"
BACKUP_DIR="/etc/acpi/backup-$(date +%Y%m%d-%H%M%S)"
NO_RESTART=0
DRY_RUN=0

# Files to remove
ACPI_SCRIPTS=(
  "hypr-utils.sh"
  "lid-open.sh"
  "lid-close.sh"
  "check-lid-on-startup.sh"
)

EVENT_FILES=(
  "lid-close"
  "lid-open"
)

RUNTIME_FILES=(
  "/run/lid-switch.lock"
  "/var/lib/hypr-monitor-lid.conf"
  "/var/log/lid-events.log"
)

usage() {
  cat <<EOF
Usage: sudo $(basename "$0") [options]

Options:
  -d, --dest DIR           Destination directory for scripts (default: /etc/acpi)
      --events-dir DIR     Destination directory for ACPI events (default: /etc/acpi/events)
  -n, --dry-run            Print what would be done, do not remove
      --no-restart         Do not restart acpid after uninstalling
  -h, --help               Show this help and exit

This script will:
  1. Create backups of all files before removal
  2. Remove ACPI scripts from /etc/acpi/
  3. Remove ACPI event files from /etc/acpi/events/
  4. Clean up runtime files and logs
  5. Remove Hyprland exec hooks from config files
  6. Restart acpid service (unless --no-restart)

Examples:
  sudo $(basename "$0")
  sudo $(basename "$0") --no-restart
  sudo $(basename "$0") -n   # dry-run to preview changes
EOF
}

# Parse args early (before sudo checks so --help works without root)
parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      -d|--dest)
        DEST_DIR="${2:-}"; shift 2 || { echo "Missing value for $1" >&2; exit 2; };
        ;;
      --events-dir)
        EVENTS_DIR="${2:-}"; shift 2 || { echo "Missing value for $1" >&2; exit 2; };
        ;;
      -n|--dry-run)
        DRY_RUN=1; shift ;;
      --no-restart)
        NO_RESTART=1; shift ;;
      -h|--help)
        usage; exit 0 ;;
      *)
        echo "Unknown option: $1" >&2
        usage
        exit 2
        ;;
    esac
  done
}

need_sudo() {
  if [ "${EUID:-$(id -u)}" -ne 0 ]; then
    echo "Please run as root: sudo $0 [options]" >&2
    exit 1
  fi
}

say() { printf '%s\n' "$*"; }

run_as_user() {
  local user="$1"; shift
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$user" -- "$@"
  else
    sudo -u "$user" "$@"
  fi
}

backup_file() {
  local file="$1"
  if [ ! -f "$file" ]; then
    return 0
  fi

  if [ "$DRY_RUN" = 1 ]; then
    say "Would back up $file -> ${BACKUP_DIR}/$(basename "$file")"
  else
    mkdir -p "$BACKUP_DIR"
    cp -av "$file" "${BACKUP_DIR}/$(basename "$file")"
  fi
}

remove_file() {
  local file="$1"
  if [ ! -f "$file" ] && [ ! -e "$file" ]; then
    return 0
  fi

  backup_file "$file"

  if [ "$DRY_RUN" = 1 ]; then
    say "Would remove $file"
  else
    rm -f "$file"
    say "Removed: $file"
  fi
}

remove_acpi_scripts() {
  say ""
  say "==> Removing ACPI scripts from $DEST_DIR"
  
  local found=0
  for script in "${ACPI_SCRIPTS[@]}"; do
    local path="${DEST_DIR}/${script}"
    if [ -f "$path" ]; then
      remove_file "$path"
      found=1
      
      # Also remove .backup files if they exist
      if [ -f "${path}.backup" ]; then
        if [ "$DRY_RUN" = 1 ]; then
          say "Would remove backup: ${path}.backup"
        else
          rm -f "${path}.backup"
        fi
      fi
    fi
  done

  if [ "$found" -eq 0 ]; then
    say "No ACPI scripts found in $DEST_DIR"
  fi
}

remove_event_files() {
  say ""
  say "==> Removing ACPI event files from $EVENTS_DIR"
  
  local found=0
  for event in "${EVENT_FILES[@]}"; do
    local path="${EVENTS_DIR}/${event}"
    if [ -f "$path" ]; then
      remove_file "$path"
      found=1
    fi
  done

  if [ "$found" -eq 0 ]; then
    say "No ACPI event files found in $EVENTS_DIR"
  fi
}

clean_runtime_files() {
  say ""
  say "==> Cleaning up runtime files"
  
  local found=0
  for file in "${RUNTIME_FILES[@]}"; do
    if [ -f "$file" ] || [ -e "$file" ]; then
      remove_file "$file"
      found=1
    fi
  done

  if [ "$found" -eq 0 ]; then
    say "No runtime files found to clean up"
  fi
}

remove_hyprland_hooks() {
  local target_user
  target_user="${SUDO_USER:-$(logname 2>/dev/null || echo "${USER:-}")}"
  if [ -z "$target_user" ] || [ "$target_user" = "root" ]; then
    say ""
    say "==> Skipping Hyprland hook removal: unable to determine non-root user context"
    return
  fi

  local target_home
  target_home=$(getent passwd "$target_user" | cut -d: -f6 2>/dev/null || true)
  if [ -z "$target_home" ]; then
    target_home=$(eval echo "~$target_user")
  fi
  if [ -z "$target_home" ]; then
    say ""
    say "==> Skipping Hyprland hook removal: could not resolve home for $target_user"
    return
  fi

  local hypr_dir lua_conf
  hypr_dir="$target_home/.config/hypr"
  lua_conf="$hypr_dir/hyprland.lua"

  say ""
  say "==> Removing Hyprland exec hooks from the repo-owned generic config"

  if [ ! -f "$lua_conf" ]; then
    if [ "$DRY_RUN" = 1 ]; then
      say "Would check $lua_conf for monitor auto-switch hooks"
    else
      say "No Hyprland root config found at $lua_conf"
    fi
    return
  fi

  if ! grep -Fq '/etc/acpi/check-lid-on-startup.sh' "$lua_conf"; then
    if [ "$DRY_RUN" = 1 ]; then
      say "Would check $lua_conf for monitor auto-switch hooks"
    else
      say "No monitor auto-switch hooks found in $lua_conf"
    fi
    return
  fi

  if [ "$DRY_RUN" = 1 ]; then
    say "Would remove monitor auto-switch hooks from $lua_conf"
    return
  fi

  local backup
  backup="${lua_conf}.backup-$(date +%Y%m%d-%H%M%S)"
  run_as_user "$target_user" cp "$lua_conf" "$backup"
  say "Backed up $lua_conf -> $backup"
  local tmp_conf
  tmp_conf="${lua_conf}.tmp-$$"
  run_as_user "$target_user" sh -c '
    awk '"'"'
	function flush_buffer() {
	  if (drop_block != 1) {
	    printf "%s", block
	  }
	  block = ""
	  in_block = 0
	  drop_block = 0
	}

	/^[[:space:]]*-- The root Hypr config owns monitor-auto-switch startup hook inclusion\.[[:space:]]*$/ {
	  pending_comment = $0 ORS
	  next
	}

	pending_comment != "" && /^[[:space:]]*hl\.on\("hyprland\.start",[[:space:]]*function\(\)[[:space:]]*$/ {
	  block = pending_comment $0 ORS
	  pending_comment = ""
	  in_block = 1
	  drop_block = 0
	  next
	}

	pending_comment != "" {
	  printf "%s", pending_comment
	  pending_comment = ""
	}

	in_block == 1 {
	  block = block $0 ORS
	  if (index($0, "/etc/acpi/check-lid-on-startup.sh") > 0) {
	    drop_block = 1
	  }
	  if ($0 ~ /^[[:space:]]*end\)[[:space:]]*$/) {
	    flush_buffer()
	  }
	  next
	}

	/^[[:space:]]*hl\.on\("hyprland\.start",[[:space:]]*function\(\)[[:space:]]*$/ {
	  block = $0 ORS
	  in_block = 1
	  drop_block = 0
	  next
	}

	/^[[:space:]]*hl\.exec_cmd\("\/etc\/acpi\/check-lid-on-startup\.sh"\)[[:space:]]*$/ {
	  next
	}

	{ print }

	END {
	  if (pending_comment != "") {
	    printf "%s", pending_comment
	  }
	  if (in_block == 1) {
	    printf "%s", block
	  }
	}
    '"'"' "$1" > "$2"
    mv "$2" "$1"
  ' sh "$lua_conf" "$tmp_conf"
  say "Removed monitor auto-switch hooks from $lua_conf"
}

restart_acpid() {
  say ""
  if [ "$NO_RESTART" = 1 ]; then
    say "==> Skipping acpid restart (--no-restart)"
  else
    if [ "$DRY_RUN" = 1 ]; then
      say "==> Would restart acpid"
    else
      say "==> Restarting acpid"
      if command -v systemctl >/dev/null 2>&1; then
        systemctl restart acpid || {
          echo "Warning: could not restart acpid. Ensure it is installed and running." >&2
        }
        say "acpid restarted successfully"
      else
        echo "Note: systemctl not found; please restart acpid manually if needed." >&2
      fi
    fi
  fi
}

show_summary() {
  say ""
  say "==> Uninstallation summary"
  say ""
  
  if [ "$DRY_RUN" = 1 ]; then
    say "DRY RUN - No changes were made"
    say "Run without --dry-run to perform the uninstallation"
  else
    say "Monitor auto-switch has been uninstalled"
    
    if [ -d "$BACKUP_DIR" ]; then
      say "Backups saved to: $BACKUP_DIR"
    fi
    
    say ""
    say "The following has been removed:"
    say "  - ACPI scripts from $DEST_DIR"
    say "  - ACPI event files from $EVENTS_DIR"
    say "  - Runtime files and logs"
    say "  - Hyprland exec hooks (if found)"
    
    if [ "$NO_RESTART" = 1 ]; then
      say ""
      say "Note: acpid was not restarted. You may need to restart it manually:"
      say "  sudo systemctl restart acpid"
    fi
  fi
  
  say ""
}

check_installation() {
  local found=0
  
  # Check for ACPI scripts
  for script in "${ACPI_SCRIPTS[@]}"; do
    if [ -f "${DEST_DIR}/${script}" ]; then
      found=1
      break
    fi
  done
  
  # Check for event files
  if [ "$found" -eq 0 ]; then
    for event in "${EVENT_FILES[@]}"; do
      if [ -f "${EVENTS_DIR}/${event}" ]; then
        found=1
        break
      fi
    done
  fi
  
  if [ "$found" -eq 0 ]; then
    say "No monitor auto-switch installation found"
    say "Nothing to uninstall"
    exit 0
  fi
}

main() {
  parse_args "$@"

  # Allow --help without root
  if [ "$DRY_RUN" != 1 ]; then
    need_sudo
  fi

  say "Monitor Auto-Switch Uninstaller"
  say "================================"
  
  if [ "$DRY_RUN" = 1 ]; then
    say "DRY RUN MODE - No changes will be made"
    say ""
  fi

  check_installation
  remove_acpi_scripts
  remove_event_files
  clean_runtime_files
  remove_hyprland_hooks
  restart_acpid
  show_summary
}

main "$@"
