#!/usr/bin/env bash
# Extract repository call surface without external Node dependencies.

set -euo pipefail

usage() {
    cat << EOF
Usage: $(basename "$0") --output-dir DIR [--code-root DIR] [--file-list FILE] [--map-file FILE]

Generate call-surface artifacts:
  - call-surface-files.txt (candidate source files)
  - call-surface-map.txt (top-level exported/declared symbols)

Arguments:
  --output-dir DIR   Output directory for generated artifacts (required)
  --code-root DIR    Repository/code root to scan (default: current directory)
  --file-list FILE   Override output file list path (default: OUTPUT_DIR/call-surface-files.txt)
  --map-file FILE    Override output map path (default: OUTPUT_DIR/call-surface-map.txt)
EOF
}

CODE_ROOT="$(pwd)"
OUTPUT_DIR=""
FILE_LIST=""
MAP_FILE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output-dir)
            OUTPUT_DIR="${2:-}"
            shift 2
            ;;
        --code-root)
            CODE_ROOT="${2:-}"
            shift 2
            ;;
        --file-list)
            FILE_LIST="${2:-}"
            shift 2
            ;;
        --map-file)
            MAP_FILE="${2:-}"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

if [[ -z "$OUTPUT_DIR" ]]; then
    echo "Error: --output-dir is required" >&2
    usage >&2
    exit 1
fi

if [[ ! -d "$CODE_ROOT" ]]; then
    echo "Error: code root does not exist: $CODE_ROOT" >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR"
FILE_LIST="${FILE_LIST:-$OUTPUT_DIR/call-surface-files.txt}"
MAP_FILE="${MAP_FILE:-$OUTPUT_DIR/call-surface-map.txt}"

LANG_REGEX='\.(js|jsx|ts|tsx|mjs|cjs|py|go|rs|java|kt|swift|rb|php)$'
SYMBOL_REGEX='^(export[[:space:]]+(async[[:space:]]+)?function|export[[:space:]]+class|function[[:space:]]+[A-Za-z_][A-Za-z0-9_]*|class[[:space:]]+[A-Za-z_][A-Za-z0-9_]*|def[[:space:]]+[A-Za-z_][A-Za-z0-9_]*|func[[:space:]]+[A-Za-z_][A-Za-z0-9_]*)'

if command -v rg >/dev/null 2>&1; then
    rg --no-follow --files "$CODE_ROOT" | rg "$LANG_REGEX" > "$FILE_LIST"
else
    find -P "$CODE_ROOT" -type f \
        \( -name '*.js' -o -name '*.jsx' -o -name '*.ts' -o -name '*.tsx' -o -name '*.mjs' -o -name '*.cjs' -o -name '*.py' -o -name '*.go' -o -name '*.rs' -o -name '*.java' -o -name '*.kt' -o -name '*.swift' -o -name '*.rb' -o -name '*.php' \) \
        > "$FILE_LIST"
fi

: > "$MAP_FILE"
if [[ -s "$FILE_LIST" ]]; then
    if command -v rg >/dev/null 2>&1; then
        while IFS= read -r file; do
            rg -n --no-heading "$SYMBOL_REGEX" "$file" || true
        done < "$FILE_LIST" > "$MAP_FILE"
    else
        while IFS= read -r file; do
            grep -nE "$SYMBOL_REGEX" "$file" || true
        done < "$FILE_LIST" > "$MAP_FILE"
    fi
fi

FILE_COUNT=$(wc -l < "$FILE_LIST" | tr -d ' ')
SYMBOL_COUNT=$(wc -l < "$MAP_FILE" | tr -d ' ')

echo "Call-surface extraction complete"
echo "Code root: $CODE_ROOT"
echo "Files scanned list: $FILE_LIST ($FILE_COUNT entries)"
echo "Symbol map: $MAP_FILE ($SYMBOL_COUNT entries)"
