#!/usr/bin/env bash
# Smoke test for targeted-research call-surface extraction helper.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
HELPER_SCRIPT="$SCRIPT_DIR/targeted-research-call-surface.sh"

if [[ ! -x "$HELPER_SCRIPT" ]]; then
    echo "Error: helper script is missing or not executable: $HELPER_SCRIPT" >&2
    exit 1
fi

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/targeted-research-call-surface-smoke.XXXXXX")"
trap 'rm -rf "$TMP_ROOT"' EXIT

FIXTURE_DIR="$TMP_ROOT/fixture"
OUTPUT_DIR="$TMP_ROOT/output"
mkdir -p "$FIXTURE_DIR/src" "$FIXTURE_DIR/pkg" "$FIXTURE_DIR/loop"

cat > "$FIXTURE_DIR/src/app.ts" << 'EOF'
export function greet(name: string) {
    return `hi ${name}`;
}

class InternalGreeter {}
EOF

cat > "$FIXTURE_DIR/pkg/module.py" << 'EOF'
def run():
    return "ok"
EOF

cat > "$FIXTURE_DIR/README.md" << 'EOF'
# Fixture
This file should be ignored by the extractor.
EOF

# Symlink loop sentinel; helper should not follow this.
ln -s "$FIXTURE_DIR" "$FIXTURE_DIR/loop/self"

RUN_OUTPUT="$("$HELPER_SCRIPT" --code-root "$FIXTURE_DIR" --output-dir "$OUTPUT_DIR" 2>&1)"

FILE_LIST="$OUTPUT_DIR/call-surface-files.txt"
MAP_FILE="$OUTPUT_DIR/call-surface-map.txt"

[[ -f "$FILE_LIST" ]] || { echo "Missing file list output" >&2; exit 1; }
[[ -f "$MAP_FILE" ]] || { echo "Missing call-surface map output" >&2; exit 1; }
[[ -s "$FILE_LIST" ]] || { echo "Empty file list output" >&2; exit 1; }
[[ -s "$MAP_FILE" ]] || { echo "Empty call-surface map output" >&2; exit 1; }

grep -q '/src/app.ts' "$FILE_LIST" || { echo "Expected TypeScript fixture path missing" >&2; exit 1; }
grep -q '/pkg/module.py' "$FILE_LIST" || { echo "Expected Python fixture path missing" >&2; exit 1; }

grep -q 'export function greet' "$MAP_FILE" || { echo "Expected TypeScript symbol missing" >&2; exit 1; }
grep -q 'def run' "$MAP_FILE" || { echo "Expected Python symbol missing" >&2; exit 1; }

if grep -q 'File system loop found' <<< "$RUN_OUTPUT"; then
    echo "Unexpected symlink loop warning in helper output" >&2
    exit 1
fi

echo "Smoke test passed"
echo "$RUN_OUTPUT"
