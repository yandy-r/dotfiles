#!/usr/bin/env bash
# Hyprland clients helper: show class/title blocks (default) or raw output (-f).

set -euo pipefail

readonly SCRIPT_NAME="${0##*/}"

usage() {
	cat <<USAGE
Usage: $SCRIPT_NAME [-h | --help]
       $SCRIPT_NAME [<pattern>...]
       $SCRIPT_NAME [-f | --full] [<pattern>...]

Default output (no -f): window blocks with class info (no line numbers).
With pattern: fixed, case-insensitive substring match against class/title fields.

-f, --full: raw hyprctl clients output. If a pattern is given, filters
lines using rg (preferred) or grep -F.

Note:
  Default mode ignores this script's own terminal title
  (titles starting with "hypr-clients ") to reduce noise.
USAGE
}

case "${1:-}" in
-h|--help)
	usage
	exit 0
	;;
esac

full_output=false
if [[ "${1:-}" == "-f" || "${1:-}" == "--full" ]]; then
	full_output=true
	shift
fi

if ! command -v hyprctl &>/dev/null; then
	echo "$SCRIPT_NAME: hyprctl not found (Hyprland may not be running)" >&2
	exit 1
fi

if [[ "$full_output" == true ]]; then
	tmp=$(mktemp)
	trap 'rm -f "$tmp"' EXIT

	if ! hyprctl clients >"$tmp" 2>&1; then
		cat "$tmp" >&2
		exit 1
	fi

	if [[ $# -eq 0 ]]; then
		cat "$tmp"
		exit 0
	fi

	pattern="$*"
	set +e
	if command -v rg &>/dev/null; then
		rg -i -F -- "$pattern" "$tmp"
		rc=$?
	else
		grep -i -F -- "$pattern" "$tmp"
		rc=$?
	fi
	set -e

	# rg/grep exit 1 = no matches (not an error); 2+ = real error
	if ((rc > 1)); then
		exit "$rc"
	fi
	exit 0
fi

if ! command -v jq &>/dev/null; then
	echo "$SCRIPT_NAME: jq not found; install jq or use $SCRIPT_NAME -f" >&2
	exit 1
fi

json_out=$(hyprctl clients -j 2>&1) || {
	echo "$SCRIPT_NAME: hyprctl clients -j failed:" >&2
	echo "$json_out" >&2
	exit 1
}

if ! echo "$json_out" | jq -e . >/dev/null 2>&1; then
	echo "$SCRIPT_NAME: unexpected hyprctl output, try $SCRIPT_NAME -f" >&2
	echo "$json_out" >&2
	exit 1
fi

pattern="${*:-}"
echo "$json_out" | jq -r --arg p "$pattern" '
  .[]
  | select(((.title // "") | ascii_downcase | startswith("hypr-clients ")) | not)
  | select(
      ($p == "") or
      (((.class // "") + "\n" + (.title // "") + "\n" + (.initialClass // "") + "\n" + (.initialTitle // ""))
        | ascii_downcase
        | contains($p | ascii_downcase))
    )
  | "Window \(.address // "?") -> \(.title // ""):\n"
    + "    title: \(.title // "")\n"
    + "    initialTitle: \(.initialTitle // "")\n"
    + "    class: \(.class // "")\n"
    + "    initialClass: \(.initialClass // "")\n"
'
