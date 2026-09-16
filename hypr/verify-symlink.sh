#!/usr/bin/env bash
# Verification script for the generic Linux desktop symlink surface.
# This helps verify Hyprland plus optional Waybar/SwayOSD/Mako/Fuzzel symlinks after rollout.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOTFILES_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_DIR="${HOME}/.config"
HYPR_SOURCE_DIR="${DOTFILES_DIR}/hypr"

failures=0

echo "=== Generic Linux Desktop Verification ==="
echo

# Check if source exists
if [[ -d "$HYPR_SOURCE_DIR" ]]; then
    echo "✓ Source directory exists: ${HYPR_SOURCE_DIR}"
    echo "  Ecosystem .conf files: $(find "$HYPR_SOURCE_DIR" -maxdepth 1 \( -name 'hypridle.conf' -o -name 'hyprlock.conf' -o -name 'hyprpaper.conf' -o -name 'hyprsunset.conf' -o -name 'xdph.conf' \) | wc -l | tr -d ' ')"
    echo "  Compositor .lua files: $(find "$HYPR_SOURCE_DIR" -maxdepth 1 -name '*.lua' | wc -l | tr -d ' ')"
else
    echo "✗ Source directory missing: ${HYPR_SOURCE_DIR}"
    exit 1
fi

echo

check_symlink_target() {
    local label="$1"
    local dest="$2"
    local expected="$3"

    if [[ -L "$dest" ]]; then
        local target
        target="$(readlink "$dest")"
        echo "✓ ${label} is a symlink: ${dest}"
        echo "  Points to: ${target}"

        if [[ ! -e "$dest" ]]; then
            echo "  ✗ Symlink target does not exist"
            failures=$((failures + 1))
        else
            local resolved_target expected_canonical
            resolved_target="$(readlink -f "$dest")"
            expected_canonical="$(readlink -f "$expected")"
            if [[ "$resolved_target" == "$expected_canonical" ]]; then
                echo "  ✓ Points to correct location"
            else
                echo "  ✗ Points to WRONG location (expected ${expected})"
                failures=$((failures + 1))
            fi
        fi
    elif [[ -e "$dest" ]]; then
        echo "⚠ ${label} exists but is not symlinked: ${dest}"
        echo "  Preview activation first:"
        echo "    ansible-playbook playbooks/symlinks.yml --extra-vars 'dryrun=true linux_desktop=true'"
        echo "  Then apply live with backups:"
        echo "    ansible-playbook playbooks/symlinks.yml --extra-vars 'backup=true linux_desktop=true'"
        failures=$((failures + 1))
    else
        echo "⚠ ${label} is not deployed: ${dest}"
    fi

    echo
}

check_symlink_target "Hyprland config" "${CONFIG_DIR}/hypr" "${HYPR_SOURCE_DIR}"
check_symlink_target "Waybar config" "${CONFIG_DIR}/waybar" "${DOTFILES_DIR}/waybar"
check_symlink_target "SwayOSD config" "${CONFIG_DIR}/swayosd" "${DOTFILES_DIR}/swayosd"
check_symlink_target "Mako config" "${CONFIG_DIR}/mako" "${DOTFILES_DIR}/mako"
check_symlink_target "Fuzzel config" "${CONFIG_DIR}/fuzzel" "${DOTFILES_DIR}/fuzzel"

echo

has_lid_startup_hook() {
	local lua_conf="$1"

	awk '
		/^[[:space:]]*hl\.on\("hyprland\.start",[[:space:]]*function\(\)[[:space:]]*$/ {
			in_hook = 1
			found_exec = 0
			next
		}
		in_hook && /^[[:space:]]*hl\.exec_cmd\("\/etc\/acpi\/check-lid-on-startup\.sh"\)[[:space:]]*$/ {
			found_exec = 1
			next
		}
		in_hook && /^[[:space:]]*end\)[[:space:]]*$/ {
			if (found_exec) {
				found_hook = 1
				exit
			}
			in_hook = 0
		}
		END { exit found_hook ? 0 : 1 }
	' "$lua_conf"
}

has_lid_reload_hook() {
	local lua_conf="$1"

	awk '
		/^[[:space:]]*hl\.on\("hyprland\.start",[[:space:]]*function\(\)[[:space:]]*$/ {
			in_hook = 1
			next
		}
		in_hook && /^[[:space:]]*end\)[[:space:]]*$/ {
			in_hook = 0
			next
		}
		!in_hook && /^[[:space:]]*hl\.exec_cmd\("\/etc\/acpi\/check-lid-on-startup\.sh"\)[[:space:]]*$/ {
			found_reload = 1
			exit
		}
		END { exit found_reload ? 0 : 1 }
	' "$lua_conf"
}

has_hyprpaper_autostart() {
	local autostart_lua="$1"

	awk '
		/^[[:space:]]*hl\.on\("hyprland\.start",[[:space:]]*function\(\)[[:space:]]*$/ {
			in_hook = 1
			next
		}
	in_hook && /^[[:space:]]*hl\.exec_cmd\(\[\[sh -lc '\''pgrep -x hyprpaper >\/dev\/null 2>&1 \|\| exec hyprpaper'\''\]\]\)[[:space:]]*$/ {
			found_exec = 1
			next
		}
		in_hook && /^[[:space:]]*end\)[[:space:]]*$/ {
			if (found_exec) {
				found_hook = 1
				exit
			}
			in_hook = 0
		}
		END { exit found_hook ? 0 : 1 }
	' "$autostart_lua"
}

has_hyprpaper_reload_recovery() {
	local autostart_lua="$1"

	awk '
		/^[[:space:]]*hl\.on\("hyprland\.start",[[:space:]]*function\(\)[[:space:]]*$/ {
			in_hook = 1
			next
		}
		in_hook && /^[[:space:]]*end\)[[:space:]]*$/ {
			in_hook = 0
			next
		}
		!in_hook && /^[[:space:]]*hl\.exec_cmd\(\[\[sh -lc '\''pgrep -x hyprpaper >\/dev\/null 2>&1 \|\| exec hyprpaper'\''\]\]\)[[:space:]]*$/ {
			found_reload = 1
			exit
		}
		END { exit found_reload ? 0 : 1 }
	' "$autostart_lua"
}

# Check key files are accessible
echo "Checking Hyprland Lua configuration files:"
for file in hyprland.lua monitors.lua input.lua bindings.lua envs.lua looknfeel.lua windows.lua autostart.lua; do
	if [[ -f "${CONFIG_DIR}/hypr/$file" ]]; then
		echo "  ✓ $file ($(wc -l < "${CONFIG_DIR}/hypr/$file") lines)"
	else
		echo "  ✗ $file (missing)"
		failures=$((failures + 1))
	fi
done

echo
echo "Checking Hypr ecosystem configuration files:"
for file in hypridle.conf hyprlock.conf hyprpaper.conf hyprsunset.conf xdph.conf; do
	if [[ -f "${CONFIG_DIR}/hypr/$file" ]]; then
		echo "  ✓ $file ($(wc -l < "${CONFIG_DIR}/hypr/$file") lines)"
	else
		echo "  ✗ $file (missing)"
		failures=$((failures + 1))
	fi
done

lua_conf="${CONFIG_DIR}/hypr/hyprland.lua"
if [[ ! -f "$lua_conf" ]]; then
	echo "  ✗ startup hook ownership is missing from hyprland.lua"
	failures=$((failures + 1))
else
	hook_failures=0
	if has_lid_startup_hook "$lua_conf"; then
		echo "  ✓ lid startup event hook is present in hyprland.lua"
	else
		echo "  ✗ lid startup event hook is missing from hyprland.lua"
		hook_failures=$((hook_failures + 1))
	fi

	if has_lid_reload_hook "$lua_conf"; then
		echo "  ✓ lid reload-time hook is present in hyprland.lua"
	else
		echo "  ✗ lid reload-time hook is missing from hyprland.lua"
		hook_failures=$((hook_failures + 1))
	fi

	if has_hyprpaper_autostart "${CONFIG_DIR}/hypr/autostart.lua"; then
		echo "  ✓ hyprpaper startup hook is present in autostart.lua"
	else
		echo "  ✗ hyprpaper startup hook is missing from autostart.lua"
		hook_failures=$((hook_failures + 1))
	fi

	if has_hyprpaper_reload_recovery "${CONFIG_DIR}/hypr/autostart.lua"; then
		echo "  ✓ hyprpaper reload recovery hook is present in autostart.lua"
	else
		echo "  ✗ hyprpaper reload recovery hook is missing from autostart.lua"
		hook_failures=$((hook_failures + 1))
	fi

	failures=$((failures + hook_failures))
fi

echo

if (( failures > 0 )); then
    echo "=== Verification Failed (${failures} issue(s)) ==="
    exit 1
fi

echo "=== Verification Complete ==="
