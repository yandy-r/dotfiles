#!/usr/bin/env bash
# Tests for bin/cursor-export-secrets.sh.
# Uses synthetic SQLite fixture databases under an isolated HOME/XDG_CONFIG_HOME.
# Never touches real Cursor state or real credentials.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
EXPORTER="$REPO_ROOT/bin/cursor-export-secrets.sh"
FIXTURE_PARENT="/tmp/opencode"
if [[ ! -d "$FIXTURE_PARENT" || ! -w "$FIXTURE_PARENT" ]]; then
	FIXTURE_PARENT="${TMPDIR:-/tmp}"
fi
TMP_DIR=""
ORIGINAL_PATH="$PATH"
REAL_UNAME="$(command -v uname)"

# Synthetic secret values. These exist only inside the temporary fixture DB and
# must never appear on the exporter's stdout or stderr.
ACCESS_TOKEN="synthetic-token-00112233445566778899aabbccddeeff"
MACHINE_ID="0123456789abcdef0123456789abcdef"
UPDATED_ACCESS_TOKEN="synthetic-token-ffeeddccbbaa99887766554433221100"

cleanup() {
	[[ -z "$TMP_DIR" ]] || rm -rf "$TMP_DIR"
}

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}

assert_file() {
	[[ -f "$1" ]] || fail "expected file: $1"
}

assert_missing() {
	[[ ! -e "$1" ]] || fail "expected path to be absent: $1"
}

assert_eq() {
	[[ "$1" == "$2" ]] || fail "expected '$1' == '$2'"
}

assert_contains() {
	local needle="$1"
	local file="$2"
	grep -Fq -- "$needle" "$file" || fail "expected '$needle' in $file"
}

assert_mode() {
	local expected="$1"
	local path="$2"
	local actual
	if actual="$(stat -c '%a' "$path" 2>/dev/null)"; then
		:
	elif actual="$(stat -f '%Lp' "$path" 2>/dev/null)"; then
		:
	else
		fail "cannot stat mode of $path"
	fi
	assert_eq "$expected" "${actual: -3}"
}

# Secret material must never leak into captured output. Do not echo the values
# on failure; report only the artifact that leaked.
assert_hides_secrets() {
	local file="$1"
	if grep -Fq -- "$ACCESS_TOKEN" "$file" ||
		grep -Fq -- "$UPDATED_ACCESS_TOKEN" "$file" ||
		grep -Fq -- "$MACHINE_ID" "$file"; then
		fail "secret material leaked into $file"
	fi
}

sql_literal() {
	local value="$1"
	value="${value//\'/\'\'}"
	printf "'%s'" "$value"
}

# Insert or replace a key/value row in the synthetic Cursor state database.
db_put() {
	local db="$1"
	local key="$2"
	local value="$3"
	sqlite3 "$db" "INSERT OR REPLACE INTO ItemTable (key, value) VALUES ($(sql_literal "$key"), $(sql_literal "$value"));" ||
		fail "failed to seed fixture key $key"
}

create_db() {
	local db="$1"
	mkdir -p "$(dirname "$db")"
	sqlite3 "$db" "CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT);" ||
		fail "failed to create fixture database $db"
}

# Populate the default (primary) keys.
seed_primary_keys() {
	local db="$1"
	db_put "$db" "cursorAuth/accessToken" "$ACCESS_TOKEN"
	db_put "$db" "storage.serviceMachineId" "$MACHINE_ID"
}

# Fake uname so the exporter sees a controlled OS. FAKE_UNAME_S selects the
# `uname -s` result; every other invocation defers to the real uname.
write_fake_uname() {
	local bin_dir="$1"
	cat >"$bin_dir/uname" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-s" ]]; then
	printf '%s\n' "${FAKE_UNAME_S:-Linux}"
	exit 0
fi
exec "$REAL_UNAME" "$@"
EOF
	chmod +x "$bin_dir/uname"
}

setup_case() {
	cleanup
	mkdir -p "$FIXTURE_PARENT"
	TMP_DIR="$(mktemp -d "$FIXTURE_PARENT/test-cursor-export-secrets.XXXXXX")"
	trap cleanup EXIT

	export TEST_HOME="$TMP_DIR/home"
	export TEST_XDG_CONFIG_HOME="$TMP_DIR/xdg-config"
	export TEST_XDG_DATA_HOME="$TMP_DIR/xdg-data"
	export TEST_XDG_CACHE_HOME="$TMP_DIR/xdg-cache"
	export TEST_XDG_STATE_HOME="$TMP_DIR/xdg-state"
	export TEST_BIN="$TMP_DIR/bin"
	export CURSOR_USER_DIR="$TEST_HOME/.config/Cursor/User"
	export CURSOR_DB="$CURSOR_USER_DIR/globalStorage/state.vscdb"
	export OUTPUT_PATH="$TEST_XDG_CONFIG_HOME/secrets/cursor.json"
	export OUT_STDOUT="$TMP_DIR/stdout.log"
	export OUT_STDERR="$TMP_DIR/stderr.log"
	mkdir -p "$TEST_HOME" "$TEST_XDG_CONFIG_HOME" "$TEST_XDG_DATA_HOME" "$TEST_XDG_CACHE_HOME" "$TEST_XDG_STATE_HOME" "$TEST_BIN"
	write_fake_uname "$TEST_BIN"
}

# Run the exporter with isolated HOME/XDG roots and fake uname.
# Captures stdout/stderr separately; returns the exporter's exit status without
# aborting the test (set -e safe via the `if !` guard).
run_export() {
	local fake_uname_s="${1:-Linux}"
	shift
	local status=0
	env \
		DOTFILES_ROOT="$REPO_ROOT" \
		HOME="$TEST_HOME" \
		XDG_CONFIG_HOME="$TEST_XDG_CONFIG_HOME" \
		XDG_DATA_HOME="$TEST_XDG_DATA_HOME" \
		XDG_CACHE_HOME="$TEST_XDG_CACHE_HOME" \
		XDG_STATE_HOME="$TEST_XDG_STATE_HOME" \
		PATH="$TEST_BIN:$ORIGINAL_PATH" \
		FAKE_UNAME_S="$fake_uname_s" \
		REAL_UNAME="$REAL_UNAME" \
		"$EXPORTER" "$@" >"$OUT_STDOUT" 2>"$OUT_STDERR" || status=$?
	assert_hides_secrets "$OUT_STDOUT"
	assert_hides_secrets "$OUT_STDERR"
	return "$status"
}

assert_run_ok() {
	local fake_uname_s="$1"
	shift
	if ! run_export "$fake_uname_s" "$@"; then
		sed 's/^/  /' "$OUT_STDERR" >&2
		fail "expected exporter to succeed: $*"
	fi
}

assert_run_fails() {
	local fake_uname_s="$1"
	shift
	if run_export "$fake_uname_s" "$@"; then
		fail "expected exporter to fail: $*"
	fi
}

test_success_json_and_permissions() {
	setup_case
	create_db "$CURSOR_DB"
	seed_primary_keys "$CURSOR_DB"

	assert_run_ok Linux

	assert_file "$OUTPUT_PATH"
	assert_mode 700 "$(dirname "$OUTPUT_PATH")"
	assert_mode 600 "$OUTPUT_PATH"

	jq -e --arg token "$ACCESS_TOKEN" --arg machine "$MACHINE_ID" \
		'.accessToken == $token and .machineId == $machine and (.os | length > 0) and (.exportedAt | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$"))' \
		"$OUTPUT_PATH" >/dev/null || fail "output JSON missing expected fields"

	# Final stdout line prints only the output path.
	assert_eq "$OUTPUT_PATH" "$(tail -n 1 "$OUT_STDOUT")"
	# No temporary leftover files.
	local leftover
	leftover="$(find "$(dirname "$OUTPUT_PATH")" -name '.cursor.json.tmp.*' -print)"
	assert_eq "" "$leftover"
	printf 'PASS: success JSON + permissions\n'
}

test_custom_output_path() {
	setup_case
	create_db "$CURSOR_DB"
	seed_primary_keys "$CURSOR_DB"
	local custom_out="$TMP_DIR/custom-dir/custom-secrets.json"

	assert_run_ok Linux --output "$custom_out"

	assert_file "$custom_out"
	assert_mode 700 "$(dirname "$custom_out")"
	assert_mode 600 "$custom_out"
	assert_eq "$custom_out" "$(tail -n 1 "$OUT_STDOUT")"
	printf 'PASS: custom output path\n'
}

test_hardens_permissive_directory() {
	setup_case
	create_db "$CURSOR_DB"
	seed_primary_keys "$CURSOR_DB"
	local secrets_dir="$TEST_XDG_CONFIG_HOME/secrets"
	mkdir -m 755 "$secrets_dir"
	assert_mode 755 "$secrets_dir"

	assert_run_ok Linux

	assert_mode 700 "$secrets_dir"
	assert_contains "Output directory permissions are broader than 700" "$OUT_STDERR"
	printf 'PASS: hardens permissive directory\n'
}

test_help_writes_nothing() {
	setup_case

	assert_run_ok Linux --help
	assert_contains "Usage:" "$OUT_STDOUT"
	assert_missing "$TEST_XDG_CONFIG_HOME/secrets"
	printf 'PASS: --help writes nothing\n'
}

test_check_writes_nothing() {
	setup_case
	create_db "$CURSOR_DB"
	seed_primary_keys "$CURSOR_DB"

	assert_run_ok Linux --check
	assert_contains "available and valid" "$OUT_STDOUT"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: --check writes nothing\n'
}

test_dry_run_writes_nothing() {
	setup_case
	create_db "$CURSOR_DB"
	seed_primary_keys "$CURSOR_DB"

	assert_run_ok Linux --dry-run
	assert_contains "Dry run passed" "$OUT_STDOUT"
	assert_contains "Key cursorAuth/accessToken: found" "$OUT_STDERR"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: --dry-run writes nothing\n'
}

test_dry_run_and_check_conflict() {
	setup_case

	assert_run_fails Linux --dry-run --check
	assert_contains "cannot be used together" "$OUT_STDERR"
	printf 'PASS: --dry-run + --check conflict\n'
}

test_missing_database() {
	setup_case

	assert_run_fails Linux
	assert_contains "state database not found" "$OUT_STDERR"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: missing database\n'
}

test_missing_keys() {
	setup_case
	create_db "$CURSOR_DB"

	assert_run_fails Linux
	assert_contains "access token not found" "$OUT_STDERR"
	assert_contains "machine ID not found" "$OUT_STDERR"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: missing keys\n'
}

test_malformed_database() {
	setup_case
	mkdir -p "$(dirname "$CURSOR_DB")"
	printf 'this is not a sqlite database' >"$CURSOR_DB"

	assert_run_fails Linux
	assert_contains "Failed to query" "$OUT_STDERR"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: malformed database\n'
}

test_invalid_machine_id_format() {
	setup_case
	create_db "$CURSOR_DB"
	db_put "$CURSOR_DB" "cursorAuth/accessToken" "$ACCESS_TOKEN"
	db_put "$CURSOR_DB" "storage.serviceMachineId" "not-a-hex-machine-id"

	assert_run_fails Linux
	assert_contains "unexpected format" "$OUT_STDERR"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: invalid machine ID format\n'
}

test_fallback_keys() {
	setup_case
	create_db "$CURSOR_DB"
	# Test token fallback key cursorAuth/token and machineId fallback storage.machineId
	db_put "$CURSOR_DB" "cursorAuth/token" "$ACCESS_TOKEN"
	db_put "$CURSOR_DB" "storage.machineId" "$MACHINE_ID"

	assert_run_ok Linux

	jq -e --arg token "$ACCESS_TOKEN" --arg machine "$MACHINE_ID" \
		'.accessToken == $token and .machineId == $machine' \
		"$OUTPUT_PATH" >/dev/null || fail "fallback keys (cursorAuth/token, storage.machineId) failed"

	# Test third machineId fallback telemetry.machineId
	sqlite3 "$CURSOR_DB" "DELETE FROM ItemTable WHERE key = 'storage.machineId';"
	db_put "$CURSOR_DB" "telemetry.machineId" "$MACHINE_ID"

	assert_run_ok Linux

	jq -e --arg token "$ACCESS_TOKEN" --arg machine "$MACHINE_ID" \
		'.accessToken == $token and .machineId == $machine' \
		"$OUTPUT_PATH" >/dev/null || fail "fallback key telemetry.machineId failed"
	printf 'PASS: fallback keys\n'
}

test_quoted_strings_normalized() {
	setup_case
	create_db "$CURSOR_DB"
	db_put "$CURSOR_DB" "cursorAuth/accessToken" "\"$ACCESS_TOKEN\""
	db_put "$CURSOR_DB" "storage.serviceMachineId" "\"$MACHINE_ID\""

	assert_run_ok Linux

	jq -e --arg token "$ACCESS_TOKEN" --arg machine "$MACHINE_ID" \
		'.accessToken == $token and .machineId == $machine' \
		"$OUTPUT_PATH" >/dev/null || fail "quoted values were not normalized"
	printf 'PASS: quoted strings normalized\n'
}

test_repeated_export_overwrites() {
	setup_case
	create_db "$CURSOR_DB"
	seed_primary_keys "$CURSOR_DB"

	assert_run_ok Linux
	assert_mode 600 "$OUTPUT_PATH"

	sqlite3 "$CURSOR_DB" "UPDATE ItemTable SET value = $(sql_literal "$UPDATED_ACCESS_TOKEN") WHERE key = 'cursorAuth/accessToken';" ||
		fail "failed to update fixture token"

	assert_run_ok Linux

	jq -e --arg token "$UPDATED_ACCESS_TOKEN" \
		'.accessToken == $token' \
		"$OUTPUT_PATH" >/dev/null || fail "second export did not overwrite output"
	assert_mode 700 "$(dirname "$OUTPUT_PATH")"
	assert_mode 600 "$OUTPUT_PATH"
	printf 'PASS: repeated export overwrites\n'
}

test_macos_user_directory() {
	setup_case
	local mac_user_dir="$TEST_HOME/Library/Application Support/Cursor/User"
	local mac_db="$mac_user_dir/globalStorage/state.vscdb"
	create_db "$mac_db"
	seed_primary_keys "$mac_db"

	assert_run_ok Darwin

	assert_file "$OUTPUT_PATH"
	jq -e --arg token "$ACCESS_TOKEN" --arg machine "$MACHINE_ID" \
		'.accessToken == $token and .machineId == $machine and .os == "macos"' \
		"$OUTPUT_PATH" >/dev/null || fail "macOS export failed"
	printf 'PASS: macOS user directory\n'
}

test_missing_dependencies() {
	setup_case
	create_db "$CURSOR_DB"
	seed_primary_keys "$CURSOR_DB"

	# Strip sqlite3 and jq by pointing PATH only to a stub directory with uname, date, etc.
	local minimal_bin="$TMP_DIR/minimal-bin"
	mkdir -p "$minimal_bin"
	write_fake_uname "$minimal_bin"
	for cmd in bash dirname basename cat stat chmod install rm date env grep sed tail mkdir; do
		local cmd_path
		cmd_path="$(command -v "$cmd" || true)"
		if [[ -n "$cmd_path" ]]; then
			ln -sf "$cmd_path" "$minimal_bin/$cmd"
		fi
	done

	local status=0
	env \
		DOTFILES_ROOT="$REPO_ROOT" \
		HOME="$TEST_HOME" \
		XDG_CONFIG_HOME="$TEST_XDG_CONFIG_HOME" \
		XDG_DATA_HOME="$TEST_XDG_DATA_HOME" \
		XDG_CACHE_HOME="$TEST_XDG_CACHE_HOME" \
		XDG_STATE_HOME="$TEST_XDG_STATE_HOME" \
		PATH="$minimal_bin" \
		REAL_UNAME="$REAL_UNAME" \
		"$EXPORTER" >"$OUT_STDOUT" 2>"$OUT_STDERR" || status=$?

	assert_hides_secrets "$OUT_STDOUT"
	assert_hides_secrets "$OUT_STDERR"
	assert_eq 1 "$status"
	assert_contains "Required command not found" "$OUT_STDERR"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: missing dependencies\n'
}

test_unsupported_os() {
	setup_case

	assert_run_fails FreeBSD
	assert_contains "Unsupported operating system" "$OUT_STDERR"
	assert_missing "$OUTPUT_PATH"
	printf 'PASS: unsupported OS\n'
}

main() {
	test_success_json_and_permissions
	test_custom_output_path
	test_hardens_permissive_directory
	test_help_writes_nothing
	test_check_writes_nothing
	test_dry_run_writes_nothing
	test_dry_run_and_check_conflict
	test_missing_database
	test_missing_keys
	test_malformed_database
	test_invalid_machine_id_format
	test_fallback_keys
	test_quoted_strings_normalized
	test_repeated_export_overwrites
	test_macos_user_directory
	test_missing_dependencies
	test_unsupported_os
	cleanup
	printf 'PASS: all cursor-export-secrets tests passed\n'
}

main "$@"
