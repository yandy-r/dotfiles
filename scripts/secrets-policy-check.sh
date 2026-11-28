#!/usr/bin/env bash
# Secrets policy (YAN-2498; docs/features/environment-management.md#secrets-with-1password).
# Secrets live in 1Password; repos commit only op:// reference templates.
# Fails when:
#   1. a tracked file outside the historical record mentions the retired
#      encrypted-env tool, its key variables, or its key file;
#   2. an env file (other than *.example) or the retired key file is tracked;
#   3. a tracked op/*.env template holds anything but comments, blank lines and
#      NAME=op://... references (a literal value may be a pasted secret).
# Exits 1 on a policy failure and 2 when a check cannot run.
# Usage: scripts/secrets-policy-check.sh [REPO_DIR]   (default: this repository)
set -euo pipefail
repo=${1:-$(cd "$(dirname "$0")/.." && pwd)}
cd "$repo"

failures=0
fail() {
    echo "FAIL $*"
    failures=$((failures + 1))
}

tracked_grep() { # git grep args -> matches; exits 2 on a git error, not "no match"
    local out rc=0
    out=$(git grep "$@") || rc=$?
    ((rc <= 1)) || {
        echo "error: git grep failed ($rc)" >&2
        exit 2
    }
    printf '%s' "$out"
}

each_line() { # text -> calls "$2" per non-empty line
    local line
    while IFS= read -r line; do
        [[ -z $line ]] || "$2" "$line"
    done <<<"$1"
}

git rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
    echo "error: $repo is not a git work tree" >&2
    exit 2
}

# --- 1. retired tooling ------------------------------------------------------

# Released notes, plans and research describe what was true when written. The
# pattern is assembled so this script does not match itself.
retired_tool='dotenv''x'
historical=(':!CHANGELOG.md' ':!docs/prps/**' ':!docs/plans/**' ':!docs/internal/**'
    ':!docs/research/**' ':!package-lock.json')
retired_hit() { fail "retired env tooling referenced: $1"; }
# -o prints file:line:match only, never the rest of the line: a hit may be a
# committed key assignment, and CI logs must not copy its value.
hits=$(tracked_grep -n -o -I -i -E "${retired_tool}|DOTENV_(PRIVATE|PUBLIC)_KEY|\\.env\\.keys" -- . "${historical[@]}")
each_line "$hits" retired_hit

# --- 2. tracked env files ----------------------------------------------------

tracked_env() { fail "tracked env or key file: $1"; }
files=$(git ls-files | grep -E '(^|/)\.env(\.[^/]*)?$' | grep -v -E '\.example$' || true)
each_line "$files" tracked_env

# --- 3. op:// reference templates --------------------------------------------

templates=$(git ls-files -- 'op/*.env' '*/op/*.env')
ref_value='(op://[^[:space:]"'\'']+|"op://[^"]+"|'\''op://[^'\'']+'\'')'
ref_line_re="^[[:space:]]*(export[[:space:]]+)?[A-Za-z_][A-Za-z0-9_]*[[:space:]]*=[[:space:]]*${ref_value}[[:space:]]*\$"
check_template() {
    local file=$1 line lineno=0
    while IFS= read -r line || [[ -n $line ]]; do
        lineno=$((lineno + 1))
        [[ $line =~ ^[[:space:]]*(#.*)?$ ]] && continue
        # NAME=op://..., optionally `export`ed, spaced around `=`, or quoted:
        # the env-file forms `op run --env-file` accepts.
        if [[ $line =~ $ref_line_re ]]; then
            continue
        elif [[ $line =~ ^[[:space:]]*([A-Za-z_][A-Za-z0-9_]*)= ]]; then
            # Print only the name: the value might be a pasted secret.
            fail "$file:$lineno ${BASH_REMATCH[1]} is not an op:// reference"
        else
            fail "$file:$lineno is not a NAME=op:// reference (line not shown)"
        fi
    done <"$file"
}
each_line "$templates" check_template

if ((failures > 0)); then
    echo "secrets policy: $failures problem(s); keep secrets in 1Password (op://), see docs/features/environment-management.md"
    exit 1
fi
echo "secrets policy: ok"
