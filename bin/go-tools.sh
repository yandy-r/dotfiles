#!/usr/bin/env bash

# go-tools.sh - Unified Go development tools script
# Supports: tidy, fmt, vet, test, build, clean, check, all
# Follows best practices for Go project maintenance

set -e

# Unset any cd aliases that might interfere (e.g., zoxide)
unalias cd 2>/dev/null || true

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m' # No Color

# Global variables
TARGET_DIR=""
VERBOSE=false
PARALLEL=false
SKIP_TESTS=false
SERVICE=""

# Counters for summary (compatible with Bash 3.2+)
# Use simple variables instead of associative arrays for macOS Bash 3.2 compatibility
COUNTER_TOTAL=0
COUNTER_SUCCESS=0
COUNTER_FAILED=0
COUNTER_FORMATTED=0
COUNTER_BUILT=0
COUNTER_TESTED=0
COUNTER_VETTED=0

# Function to print colored output
print_header() {
    echo -e "${CYAN}================================${NC}"
    echo -e "${CYAN}  $1${NC}"
    echo -e "${CYAN}================================${NC}"
}

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_debug() {
    if [[ "$VERBOSE" == "true" ]]; then
        echo -e "${MAGENTA}[DEBUG]${NC} $1"
    fi
}

# Function to generate .golangci.yml config file
generate_golangci_config() {
    local config_file="$TARGET_DIR/.golangci.yml"
    local force_overwrite="${GO_TOOLS_FORCE:-false}"
    local assume_yes="${GO_TOOLS_ASSUME_YES:-false}"
    local dry_run="${GO_TOOLS_DRY_RUN:-false}"

    if [[ -f "$config_file" ]]; then
        print_warning "Config file already exists: $config_file"
        if [[ "$force_overwrite" == "true" ]]; then
            :
        elif [[ "$assume_yes" == "true" ]]; then
            print_status "Skipping config generation"
            return 0
        else
            read -p "Overwrite existing file? (y/N): " -n 1 -r
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                print_status "Skipping config generation"
                return 0
            fi
        fi
    fi

    print_status "Generating golangci-lint config: $config_file"

    if [[ "$dry_run" == "true" ]]; then
        print_status "[dry-run] Would write: $config_file"
        return 0
    fi

    # Create a modern golangci-lint v2 config that works for generic Go projects.
    cat > "$config_file" << 'EOF'
# golangci-lint configuration
# See: https://golangci-lint.run/usage/configuration/

version: "2"

run:
  timeout: 5m
  tests: true
  relative-path-mode: gomod

linters:
  default: standard
  enable:
    - revive

issues:
  max-issues-per-linter: 0
  max-same-issues: 0
  exclude-rules:
    - path: _test\.go
      linters:
        - errcheck
EOF

    print_success "Created: $config_file"
    echo
    print_status "Next steps:"
    print_status "  1. Install golangci-lint: https://golangci-lint.run/usage/install/"
    print_status "  2. Run linter: go-tools.sh --lint-advanced"
    print_status "  3. Customize config: edit $config_file"
}

print_progress() {
    local current=$1
    local total=$2
    local service=$3
    local operation=$4

    local percentage=$((current * 100 / total))
    local bar_length=30
    local filled_length=$((percentage * bar_length / 100))

    # Create progress bar
    local bar=""
    for ((i=0; i<filled_length; i++)); do
        bar+="█"
    done
    for ((i=filled_length; i<bar_length; i++)); do
        bar+="░"
    done

    printf "\r${BLUE}[%3d%%]${NC} [%s] %s - %s" "$percentage" "$bar" "$service" "$operation"
    if [[ $current -eq $total ]]; then
        echo # New line when complete
    fi
}

# Function to show usage
show_usage() {
    cat << EOF
Usage: go-tools.sh [DIRECTORY] [COMMANDS] [OPTIONS]

Go Development Tools - Unified script for Go project maintenance

ARGUMENTS:
    DIRECTORY       Target directory to process (default: current directory)

COMMANDS:
    --generate      Generate a .golangci.yml config file in target directory
    --tidy          Run 'go mod tidy' in all Go modules
    --fmt           Run 'go fmt' to format Go code
    --vet           Run 'go vet' to check for issues
    --lint-advanced Run 'golangci-lint' for comprehensive linting
    --test          Run 'go test' in all modules
    --build         Run 'go build' to compile all modules
    --clean         Run 'go clean' to clean build artifacts
    --check         Run 'go mod verify' and 'go list -m all'
    --all           Run all commands (tidy, fmt, vet, test, build, check)

OPTIONS:
    -v, --verbose       Enable verbose output
    -p, --parallel      Run operations in parallel where possible
    -s, --skip-test     Skip running tests
    --service <name>    Run operations on a specific service/module only
    -h, --help          Show this help message

EXAMPLES:
    # Generate golangci-lint config in current directory
    go-tools.sh --generate

    # Format Go code in current directory
    go-tools.sh --fmt

    # Run advanced linting with golangci-lint
    go-tools.sh --lint-advanced

    # Run all tools on specific directory
    go-tools.sh ~/my-go-project --all

    # Run multiple commands on parent directory
    go-tools.sh .. --tidy --fmt --vet

    # Run tests with verbose output
    go-tools.sh --test -v

    # Build with verbose output in specific directory
    go-tools.sh ~/projects/api --build -v

    # Format only a specific service/module
    go-tools.sh --fmt --service auth-api

EOF
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to run operation on a single module
run_operation() {
    local service_dir="$1"
    local operation="$2"
    local relative_path="$3"
    local current="$4"
    local total="$5"

    local result=""
    local success=false
    local output=""

    # Change to service directory
    if ! cd "$service_dir" 2>/dev/null; then
        print_error "Failed to change to directory: $relative_path"
        return 1
    fi

    print_debug "Running $operation in $relative_path"

    case "$operation" in
        "tidy")
            if output=$(go mod tidy 2>&1); then
                result="completed successfully"
                success=true
            else
                result="failed"
                success=false
            fi
            ;;

        "fmt")
            # Check if there are any Go files
            if ! find . -name "*.go" -type f | head -1 | grep -q .; then
                result="no Go files found"
                success=true
                output=""
            elif output=$(go fmt ./... 2>&1); then
                if [[ -n "$output" ]]; then
                    result="formatted $(echo "$output" | wc -l) files"
                    COUNTER_FORMATTED=$((COUNTER_FORMATTED + 1))
                else
                    result="no files needed formatting"
                fi
                success=true
            else
                result="failed"
                success=false
            fi
            ;;

        "vet")
            if output=$(go vet ./... 2>&1); then
                result="completed successfully"
                success=true
            else
                result="found issues"
                success=false
            fi
            ;;

        "test")
            if [[ "$SKIP_TESTS" == "true" ]]; then
                result="skipped"
                success=true
            else
                local -a test_args=("./...")
                if [[ "$VERBOSE" == "true" ]]; then
                    test_args=("-v" "./...")
                fi

                if output=$(go test "${test_args[@]}" 2>&1); then
                    result="all tests passed"
                    success=true
                    COUNTER_TESTED=$((COUNTER_TESTED + 1))
                else
                    result="some tests failed"
                    success=false
                fi
            fi
            ;;

        "build")
            if output=$(go build -v -o /dev/null ./... 2>&1); then
                result="built successfully"
                success=true
                COUNTER_BUILT=$((COUNTER_BUILT + 1))
            else
                result="build failed"
                success=false
            fi
            ;;

        "clean")
            if output=$(go clean ./... 2>&1); then
                result="cleaned successfully"
                success=true
            else
                result="clean failed"
                success=false
            fi
            ;;

        "check")
            local check_output=""
            local check_success=true

            # Run go mod verify
            if ! check_output=$(go mod verify 2>&1); then
                check_success=false
                output+="go mod verify failed: $check_output\n"
            fi

            # Run go list -m all
            if ! check_output=$(go list -m all 2>&1); then
                check_success=false
                output+="go list -m all failed: $check_output\n"
            fi

            if [[ "$check_success" == "true" ]]; then
                result="modules verified successfully"
                success=true
            else
                result="module verification failed"
                success=false
            fi
            ;;

        "lint-advanced")
            if ! command_exists golangci-lint; then
                result="golangci-lint not installed"
                output="Install from: https://golangci-lint.run/usage/install/"
                success=false
            else
                local -a lint_args=("run")
                if [[ "$VERBOSE" == "true" ]]; then
                    lint_args=("run" "-v")
                fi

                if output=$(golangci-lint "${lint_args[@]}" ./... 2>&1); then
                    result="no issues found"
                    success=true
                else
                    # golangci-lint returns non-zero when issues are found
                    if echo "$output" | grep -q "File is not"; then
                        result="linting failed"
                        success=false
                    else
                        result="found issues"
                        success=false
                    fi
                fi
            fi
            ;;
    esac

    # Update counters
    COUNTER_TOTAL=$((COUNTER_TOTAL + 1))
    if [[ "$success" == "true" ]]; then
        COUNTER_SUCCESS=$((COUNTER_SUCCESS + 1))
        case "$operation" in
            "vet") COUNTER_VETTED=$((COUNTER_VETTED + 1)) ;;
        esac
    else
        COUNTER_FAILED=$((COUNTER_FAILED + 1))
    fi

    # Show progress
    print_progress "$current" "$total" "$relative_path" "$result"

    # Show detailed output in verbose mode or on failure
    if [[ "$VERBOSE" == "true" && -n "$output" ]]; then
        echo
        echo "    Output: $output"
    elif [[ "$success" == "false" && -n "$output" ]]; then
        echo
        echo "    Error: $output"
    fi

    # Return to target directory
    cd "$TARGET_DIR"

    if [[ "$success" == "true" ]]; then
        return 0
    fi

    return 1
}

# Function to process all modules with a given operation
process_operation() {
    local operation="$1"
    local modules=()

    print_header "Running Go $operation"

    # Find all go.mod files
    if [[ -n "$SERVICE" ]]; then
        local service_path="$TARGET_DIR/$SERVICE/go.mod"
        if [[ -f "$service_path" ]]; then
            modules+=("$service_path")
        else
            print_error "Service '$SERVICE' not found or does not have a go.mod file"
            return 1
        fi
    else
        while IFS= read -r -d '' gomod_file; do
            modules+=("$gomod_file")
        done < <(find "$TARGET_DIR" -name "go.mod" -type f -print0)
    fi

    if [[ ${#modules[@]} -eq 0 ]]; then
        print_warning "No Go modules found in project"
        return 0
    fi

    print_status "Found ${#modules[@]} Go modules"
    echo

    local current=0
    local total=${#modules[@]}
    local failed_modules=()

    # Process each module
    for gomod_file in "${modules[@]}"; do
        current=$((current + 1))
        service_dir=$(dirname "$gomod_file")
        # Use portable relative path calculation (works on macOS and Linux)
        relative_path="${service_dir#"$TARGET_DIR"/}"
        # If no subdirectory, show as "." for current directory
        [[ "$relative_path" == "$service_dir" ]] && relative_path="."

        if ! run_operation "$service_dir" "$operation" "$relative_path" "$current" "$total"; then
            failed_modules+=("$relative_path")
        fi
    done

    echo
    echo

    # Show summary for this operation
    if [[ ${#failed_modules[@]} -eq 0 ]]; then
        print_success "All modules completed $operation successfully"
    else
        print_error "${#failed_modules[@]} modules failed $operation:"
        for module in "${failed_modules[@]}"; do
            echo "  - $module"
        done
    fi

    return ${#failed_modules[@]}
}

# Function to run all operations
run_all() {
    local operations=("tidy" "fmt" "vet" "test" "build" "check")
    local total_operations=${#operations[@]}
    local current_operation=0
    local overall_failed=0

    print_header "Running All Go Tools"
    print_status "Will run $total_operations operations: ${operations[*]}"
    echo

    for operation in "${operations[@]}"; do
        current_operation=$((current_operation + 1))
        echo
        print_status "Operation $current_operation/$total_operations: $operation"

        if ! process_operation "$operation"; then
            overall_failed=$((overall_failed + 1))
        fi
    done

    echo
    print_header "Final Summary"
    print_final_summary

    return $overall_failed
}

# Function to print final summary
print_final_summary() {
    echo -e "${BLUE}Overall Statistics:${NC}"
    echo "  Total modules processed: ${COUNTER_TOTAL}"
    echo "  Successful operations: ${COUNTER_SUCCESS}"
    echo "  Failed operations: ${COUNTER_FAILED}"

    if [[ ${COUNTER_FORMATTED} -gt 0 ]]; then
        echo "  Modules with formatted files: ${COUNTER_FORMATTED}"
    fi

    if [[ ${COUNTER_BUILT} -gt 0 ]]; then
        echo "  Modules built successfully: ${COUNTER_BUILT}"
    fi

    if [[ ${COUNTER_TESTED} -gt 0 ]]; then
        echo "  Modules tested successfully: ${COUNTER_TESTED}"
    fi

    if [[ ${COUNTER_VETTED} -gt 0 ]]; then
        echo "  Modules vetted successfully: ${COUNTER_VETTED}"
    fi

    echo

    if [[ ${COUNTER_FAILED} -eq 0 ]]; then
        print_success "All operations completed successfully! 🎉"
        return 0
    else
        print_error "${COUNTER_FAILED} operations failed"
        return 1
    fi
}

# Main function
main() {
    # Parse command line arguments
    local operations=()

    while [[ $# -gt 0 ]]; do
        case $1 in
            --generate|--tidy|--fmt|--vet|--test|--build|--clean|--check|--all|--lint-advanced)
                operations+=("${1#--}")
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -p|--parallel)
                PARALLEL=true
                shift
                ;;
            -s|--skip-test)
                SKIP_TESTS=true
                shift
                ;;
            --service)
                if [[ -z "$2" || "$2" == --* ]]; then
                    print_error "--service requires a service name argument"
                    show_usage
                    exit 1
                fi
                SERVICE="$2"
                shift 2
                ;;
            -h|--help)
                show_usage
                exit 0
                ;;
            -*)
                print_error "Unknown option: $1"
                show_usage
                exit 1
                ;;
            *)
                # First non-option argument is the target directory
                if [[ -z "$TARGET_DIR" ]]; then
                    TARGET_DIR="$1"
                else
                    print_error "Multiple directories specified"
                    show_usage
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
        print_error "Directory does not exist: $TARGET_DIR"
        exit 1
    }

    # Check if any operations were specified
    if [[ ${#operations[@]} -eq 0 ]]; then
        print_error "No operations specified"
        show_usage
        exit 1
    fi

    # Handle generate operation separately (doesn't require Go modules)
    if [[ " ${operations[*]} " =~ " generate " ]]; then
        print_header "Generate golangci-lint Config"
        generate_golangci_config

        # If only generate was requested, exit here
        if [[ ${#operations[@]} -eq 1 ]]; then
            exit 0
        fi

        # Remove generate from operations array
        operations=("${operations[@]/generate}")
        echo
    fi

    # Check if Go is installed
    if ! command_exists go; then
        print_error "Go is not installed or not in PATH"
        exit 1
    fi

    # Show project info
    print_status "Go Development Tools - Directory: $(basename "$TARGET_DIR")"
    print_status "Target directory: $TARGET_DIR"
    print_status "Go version: $(go version)"

    # Warn about unimplemented parallel flag
    if [[ "$PARALLEL" == "true" ]]; then
        print_warning "Parallel mode is not yet implemented - operations will run sequentially"
    fi

    echo

    # Process operations
    local overall_failed=0

    if [[ " ${operations[*]} " =~ " all " ]]; then
        run_all
        overall_failed=$?
    else
        for operation in "${operations[@]}"; do
            # Skip empty operations (from generate removal)
            if [[ -z "$operation" ]]; then
                continue
            fi

            if ! process_operation "$operation"; then
                overall_failed=$((overall_failed + 1))
            fi
        done

        echo
        print_header "Summary"
        print_final_summary
    fi

    exit $overall_failed
}

# Run main function with all arguments
main "$@"
