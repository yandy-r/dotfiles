#!/usr/bin/env bash

# standardize-filenames.sh - Automated file naming validation and standardization
# Created: 2025-10-16
# Purpose: Enforce kebab-case naming convention for markdown files in any directory
# Usage: ./standardize-filenames.sh [DIRECTORY] [--execute] [--verbose]

set -euo pipefail

# Configuration
TARGET_DIR=""
DRY_RUN=true
VERBOSE=false
EXIT_CODE=0

# ANSI color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Exception patterns - files that MUST be uppercase
UPPERCASE_EXCEPTIONS=(
  "README.md"
  "CLAUDE.md"
  "CHANGELOG.md"
  "LICENSE.md"
  "TODO.md"
  "DEPLOYMENT.md"
  "AGENTS.md"
  "CONTRIBUTING.md"
  "ROADMAP.md"
)

# Additional exception patterns (relative paths or glob patterns)
PATH_EXCEPTIONS=(
  ".doc.md" # Allow .doc.md convention in features/ directory if documented
)

# Arrays to store non-compliant files
declare -a NON_COMPLIANT_FILES=()

# Parse command line arguments
parse_args() {
  while [[ $# -gt 0 ]]; do
    case $1 in
    --execute)
      DRY_RUN=false
      shift
      ;;
    --verbose)
      VERBOSE=true
      shift
      ;;
    -h | --help)
      show_help
      exit 0
      ;;
    -*)
      echo -e "${RED}Error: Unknown option: $1${NC}" >&2
      show_help
      exit 1
      ;;
    *)
      # First non-option argument is the target directory
      if [[ -z "$TARGET_DIR" ]]; then
        TARGET_DIR="$1"
      else
        echo -e "${RED}Error: Multiple directories specified${NC}" >&2
        show_help
        exit 1
      fi
      shift
      ;;
    esac
  done

  # Default to current directory if no directory specified
  if [[ -z "$TARGET_DIR" ]]; then
    TARGET_DIR="$(pwd)"
  fi

  # Convert to absolute path and validate
  TARGET_DIR="$(cd "$TARGET_DIR" && pwd)" || {
    echo -e "${RED}Error: Directory does not exist: $TARGET_DIR${NC}" >&2
    exit 1
  }
}

# Show help message
show_help() {
  cat <<EOF
Usage: standardize-filenames.sh [DIRECTORY] [OPTIONS]

Validate and standardize markdown file naming conventions in a directory tree.

ARGUMENTS:
    DIRECTORY       Target directory to scan (default: current directory)

OPTIONS:
    --execute       Actually perform the renames (default: dry-run mode)
    --verbose       Show detailed output during execution
    -h, --help      Show this help message

NAMING RULES:
    - Lowercase letters only
    - Hyphens for word separation (not underscores or spaces)
    - Valid: kebab-case-example.md
    - Invalid: SCREAMING_SNAKE.md, camelCase.md, snake_case.md, Mixed-Case.md

EXCEPTIONS:
    - Files that must be UPPERCASE: README.md, CLAUDE.md, CHANGELOG.md, etc.
    - Files matching patterns in PATH_EXCEPTIONS array (e.g., .doc.md)

EXAMPLES:
    # Dry-run mode on current directory (safe, shows what would change)
    standardize-filenames.sh

    # Dry-run mode on specific directory
    standardize-filenames.sh /path/to/docs

    # Execute renames in current directory
    standardize-filenames.sh --execute

    # Execute renames in specific directory with verbose output
    standardize-filenames.sh ~/projects/myapp --execute --verbose

    # Verbose dry-run on parent directory
    standardize-filenames.sh .. --verbose
EOF
}

# Check if filename is in uppercase exceptions list
is_uppercase_exception() {
  local filename="$1"
  for exception in "${UPPERCASE_EXCEPTIONS[@]}"; do
    if [[ "$filename" == "$exception" ]]; then
      return 0
    fi
  done
  return 1
}

# Check if file path matches path exceptions
is_path_exception() {
  local filepath="$1"
  for pattern in "${PATH_EXCEPTIONS[@]}"; do
    if [[ "$filepath" == *"$pattern" ]]; then
      return 0
    fi
  done
  return 1
}

# Convert filename to kebab-case
to_kebab_case() {
  local filename="$1"
  local basename="${filename%.*}"
  local extension="${filename##*.}"

  # If filename has no extension, treat entire name as basename
  if [[ "$basename" == "$extension" ]]; then
    extension=""
  fi

  # Convert to kebab-case:
  # 1. Replace whitespace and separators with hyphens
  # 2. Replace underscores with hyphens
  # 3. Convert to lowercase
  # 4. Replace multiple consecutive hyphens with single hyphen
  # 5. Trim leading/trailing hyphens
  local kebab_basename
  kebab_basename=$(echo "$basename" | sed 's/[ \t.,+]/-/g' | sed 's/_/-/g' | tr '[:upper:]' '[:lower:]' | sed 's/-\+/-/g' | sed 's/^-\+//;s/-\+$//')

  if [[ -n "$extension" ]]; then
    echo "${kebab_basename}.${extension}"
  else
    echo "${kebab_basename}"
  fi
}

# Check if filename is kebab-case compliant
is_kebab_case_compliant() {
  local filename="$1"
  local basename="${filename%.*}"

  # Check if basename contains only lowercase letters, numbers, and hyphens
  if [[ "$basename" =~ ^[a-z0-9-]+$ ]]; then
    return 0
  fi
  return 1
}

# Validate filename naming convention
validate_filename() {
  local filepath="$1"
  local filename
  local dirname

  # Use bash parameter expansion instead of external commands to avoid stdin issues
  filename="${filepath##*/}"
  dirname="${filepath%/*}"

  # Check if this is an uppercase exception
  if is_uppercase_exception "$filename"; then
    [[ "$VERBOSE" == true ]] && echo -e "${GREEN}✓ ${filepath} (uppercase exception)${NC}"
    return 0
  fi

  # Check if this matches path exceptions
  if is_path_exception "$filepath"; then
    [[ "$VERBOSE" == true ]] && echo -e "${GREEN}✓ ${filepath} (path exception)${NC}"
    return 0
  fi

  # Check if filename is kebab-case compliant
  if is_kebab_case_compliant "$filename"; then
    [[ "$VERBOSE" == true ]] && echo -e "${GREEN}✓ ${filepath} (compliant)${NC}"
    return 0
  fi

  # File is non-compliant - generate suggested name
  local suggested_name
  suggested_name=$(to_kebab_case "$filename")
  local suggested_path="${dirname}/${suggested_name}"

  # Add to non-compliant files array
  NON_COMPLIANT_FILES+=("$filepath")

  [[ "$VERBOSE" == true ]] && echo -e "${RED}✗ ${filepath} → ${suggested_path}${NC}"

  return 1
}

# Find all markdown files in the target directory
find_markdown_files() {
  # Find all .md files, excluding common build/dependency directories
  find "$TARGET_DIR" -type f -name "*.md" \
    -not -path "*/node_modules/*" \
    -not -path "*/.git/*" \
    -not -path "*/dist/*" \
    -not -path "*/build/*" \
    -not -path "*/.next/*" \
    -not -path "*/vendor/*" \
    -not -path "*/.venv/*" \
    -not -path "*/venv/*" |
    sort
}

# Generate report of non-compliant files
generate_report() {
  local count="${#NON_COMPLIANT_FILES[@]}"

  if [[ $count -eq 0 ]]; then
    echo -e "${GREEN}✓ All markdown files follow kebab-case naming convention!${NC}"
    return 0
  fi

  echo ""
  echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${YELLOW}File Naming Standardization Report${NC}"
  echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo -e "${RED}Non-compliant files found: ${count}${NC}"
  echo ""

  for i in "${!NON_COMPLIANT_FILES[@]}"; do
    local original_path="${NON_COMPLIANT_FILES[$i]}"
    local original_file
    original_file=$(basename "$original_path")
    local suggested_file
    suggested_file=$(to_kebab_case "$original_file")
    local suggested_path="${original_path%/*}/${suggested_file}"

    # Make paths relative to target directory for cleaner output
    local rel_original="${original_path#$TARGET_DIR/}"
    local rel_suggested="${suggested_path#$TARGET_DIR/}"

    echo -e "${BLUE}${rel_original}${NC}"
    echo -e "  ${GREEN}→${NC} Suggested: ${rel_suggested}"
    echo -e "  ${YELLOW}→${NC} Command: git mv -- \"${rel_original}\" \"${rel_suggested}\""
    echo ""
  done

  echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

  if [[ "$DRY_RUN" == true ]]; then
    echo ""
    echo -e "${BLUE}This was a dry-run. No files were modified.${NC}"
    echo -e "${BLUE}Run with --execute to perform renames.${NC}"
    echo ""
  fi
}

# Check if file is tracked by git
is_git_tracked() {
  local filepath="$1"
  git ls-files --error-unmatch "$filepath" &>/dev/null
}

# Execute rename commands
execute_renames() {
  local count="${#NON_COMPLIANT_FILES[@]}"

  if [[ $count -eq 0 ]]; then
    return 0
  fi

  # Reset exit code for rename phase (validation phase already reported)
  local rename_failed=false

  echo ""
  echo -e "${YELLOW}Executing renames...${NC}"
  echo ""

  for original_path in "${NON_COMPLIANT_FILES[@]}"; do
    local original_file
    original_file=$(basename "$original_path")
    local suggested_file
    suggested_file=$(to_kebab_case "$original_file")
    local suggested_path="${original_path%/*}/${suggested_file}"

    # Check if file is tracked by git
    if is_git_tracked "$original_path"; then
      # Use git mv for tracked files (preserves history)
      echo -e "${BLUE}Executing: git mv -- \"${original_path}\" \"${suggested_path}\"${NC}"
      if git mv -- "$original_path" "$suggested_path"; then
        echo -e "${GREEN}✓ Success (git tracked)${NC}"
      else
        echo -e "${RED}✗ Failed${NC}"
        rename_failed=true
      fi
    else
      # Use regular mv for untracked files
      echo -e "${BLUE}Executing: mv -- \"${original_path}\" \"${suggested_path}\"${NC}"
      if mv -- "$original_path" "$suggested_path"; then
        echo -e "${GREEN}✓ Success (untracked file)${NC}"
      else
        echo -e "${RED}✗ Failed${NC}"
        rename_failed=true
      fi
    fi
    echo ""
  done

  if [[ "$rename_failed" == false ]]; then
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}All renames completed successfully!${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "${BLUE}Next steps:${NC}"
    echo "  1. Review changes with: git status"
    echo "  2. Update cross-references in documentation"
    echo "  3. Commit changes with descriptive message"
    echo ""
    # Reset exit code to 0 since renames succeeded
    EXIT_CODE=0
  else
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}Some renames failed. Review errors above.${NC}"
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    # Set exit code to 1 for rename failures
    EXIT_CODE=1
  fi
}

# Main execution
main() {
  parse_args "$@"

  echo ""
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${BLUE}Markdown File Naming Standardization${NC}"
  echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo -e "${BLUE}Mode:${NC} $([ "$DRY_RUN" == true ] && echo "Dry-run (safe)" || echo "Execute (will modify files)")"
  echo -e "${BLUE}Verbose:${NC} $VERBOSE"
  echo -e "${BLUE}Target Directory:${NC} $TARGET_DIR"
  echo ""

  [[ "$VERBOSE" == true ]] && echo -e "${BLUE}Scanning for markdown files...${NC}"

  local file_count=0

  # Use process substitution to avoid stdin redirection issues
  while IFS= read -r filepath; do
    validate_filename "$filepath" || true
    file_count=$((file_count + 1))
  done < <(find_markdown_files)

  [[ "$VERBOSE" == true ]] && echo ""
  [[ "$VERBOSE" == true ]] && echo -e "${BLUE}Scanned ${file_count} markdown files${NC}"

  # Generate report (informational - not an error in dry-run mode)
  generate_report

  # If execute mode and non-compliant files found, perform renames
  # Only exit with error if renames fail
  if [[ "$DRY_RUN" == false ]] && [[ "${#NON_COMPLIANT_FILES[@]}" -gt 0 ]]; then
    execute_renames
  fi

  exit $EXIT_CODE
}

# Run main function
main "$@"

