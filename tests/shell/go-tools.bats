# shellcheck shell=bash
# Tests for the .golangci.yml template written by bin/go-tools.sh --generate
# (also used by `style.sh init --go`) (YAN-1406):
#   - the template declares version "2" and keeps the _test.go errcheck
#     exclusion under the v2 key linters.exclusions.rules, not the v1-only
#     issues.exclude-rules that golangci-lint v2 rejects;
#   - `golangci-lint config verify` accepts the generated file. Runs when
#     golangci-lint is on PATH and skips otherwise, unless
#     GO_TOOLS_REQUIRE_GOLANGCI=1 makes a missing binary a failure. The
#     structural check above always runs, golangci-lint or not;
#   - --dry-run and an existing file without --force leave the target alone.
# Hermetic: the config is generated into the test tmpdir. Generation needs no
# Go toolchain and nothing touches the network.

load test_helper

GO_TOOLS_SH="$REPO_ROOT/bin/go-tools.sh"

setup() {
	TARGET="$BATS_TEST_TMPDIR/project"
	mkdir -p "$TARGET"
}

generate() {
	run env GO_TOOLS_ASSUME_YES=true bash "$GO_TOOLS_SH" --generate "$TARGET"
}

@test "generate writes a v2 config with the exclusion under linters.exclusions.rules" {
	generate
	[ "$status" -eq 0 ]
	cfg="$TARGET/.golangci.yml"
	[ -f "$cfg" ]

	grep -qx 'version: "2"' "$cfg"
	if grep -q 'exclude-rules' "$cfg"; then
		fail "v1-only exclude-rules key present in $cfg"
	fi

	# The exclusions block sits under top-level linters: and carries the
	# path/linters fields of the _test.go errcheck rule.
	run awk '
		/^[^ #]/ { top = $1 }
		top == "linters:" && /^  exclusions:$/ { ex = 1; next }
		ex && /^  [^ ]/ { ex = 0 }
		ex && /^    rules:$/ { rules = 1; next }
		rules && /^      - path: _test\\\.go$/ { path = 1; next }
		path && /^        linters:$/ { lin = 1; next }
		lin && /^          - errcheck$/ { ok = 1 }
		END { exit ok ? 0 : 1 }
	' "$cfg"
	[ "$status" -eq 0 ]
}

@test "golangci-lint config verify accepts the generated config" {
	if ! command -v golangci-lint >/dev/null 2>&1; then
		[[ "${GO_TOOLS_REQUIRE_GOLANGCI:-0}" != 1 ]] || fail "golangci-lint required but not on PATH"
		skip "golangci-lint not installed (set GO_TOOLS_REQUIRE_GOLANGCI=1 to fail instead)"
	fi
	generate
	[ "$status" -eq 0 ]

	run golangci-lint config verify --config "$TARGET/.golangci.yml"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "dry-run does not write the config" {
	run env GO_TOOLS_DRY_RUN=true bash "$GO_TOOLS_SH" --generate "$TARGET"
	[ "$status" -eq 0 ]
	[ ! -e "$TARGET/.golangci.yml" ]
}

@test "an existing config is kept without --force" {
	printf 'sentinel\n' >"$TARGET/.golangci.yml"
	generate
	[ "$status" -eq 0 ]
	[ "$(cat "$TARGET/.golangci.yml")" = "sentinel" ]
}
