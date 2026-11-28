# shellcheck shell=bash
# scripts/secrets-policy-check.sh fails on the retired encrypted-env tool, its
# key variables and key file, tracked env files, and op/*.env templates holding
# literal values; it passes on op:// references (YAN-2498). Each case runs the
# check against a throwaway git repository.

load test_helper

setup() {
	CHECK="$REPO_ROOT/scripts/secrets-policy-check.sh"
	FIXTURE="$BATS_TEST_TMPDIR/repo"
	mkdir -p "$FIXTURE/op"
	git -C "$FIXTURE" init -q
	printf 'API_KEY=op://EnvSecrets/DEMO_LOCAL/API_KEY\n' >"$FIXTURE/op/local.env"
	printf 'APP_ENV=development\n' >"$FIXTURE/.env.example"
	printf '# readme\n' >"$FIXTURE/README.md"
	# Retired names are assembled so this file does not match the check.
	RETIRED="dotenv""x"
}

track() {
	git -C "$FIXTURE" add -A
}

@test "a clean repo with op:// templates passes" {
	track
	run "$CHECK" "$FIXTURE"
	[[ $status -eq 0 && $output == *"secrets policy: ok"* ]] || fail "status $status: $output"
}

@test "a reference to the retired tool fails" {
	printf 'brew install %s\n' "$RETIRED" >"$FIXTURE/install.sh"
	track
	run "$CHECK" "$FIXTURE"
	[[ $status -eq 1 && $output == *"retired env tooling referenced: install.sh"* ]] || fail "status $status: $output"
}

@test "a retired key variable or key file name fails" {
	printf '%s_PRIVATE_KEY_PRODUCTION=not-a-real-key-value\n' "DOTENV" >"$FIXTURE/ci.yml"
	printf '.env.%s\n' "keys" >"$FIXTURE/.gitignore"
	track
	run "$CHECK" "$FIXTURE"
	[[ $status -eq 1 ]] || fail "status $status: $output"
	[[ $output == *"ci.yml:1:"* && $output == *".gitignore:1:"* ]] || fail "missing hits: $output"
	[[ $output != *not-a-real-key-value* ]] || fail "value leaked: $output"
}

@test "historical records may mention the retired tool" {
	mkdir -p "$FIXTURE/docs/plans"
	printf 'Removed %s.\n' "$RETIRED" >"$FIXTURE/CHANGELOG.md"
	printf 'Migrate off %s.\n' "$RETIRED" >"$FIXTURE/docs/plans/migrate.md"
	track
	run "$CHECK" "$FIXTURE"
	[[ $status -eq 0 ]] || fail "status $status: $output"
}

@test "a tracked env file fails" {
	printf 'APP_ENV=development\n' >"$FIXTURE/.env.local"
	git -C "$FIXTURE" add -f .env.local
	track
	run "$CHECK" "$FIXTURE"
	[[ $status -eq 1 && $output == *"tracked env or key file: .env.local"* ]] || fail "status $status: $output"
}

@test "a literal value in an op template fails without printing the value" {
	printf 'API_KEY=op://EnvSecrets/DEMO_LOCAL/API_KEY\nDB_PASSWORD=hunter2-not-real\n' >"$FIXTURE/op/local.env"
	track
	run "$CHECK" "$FIXTURE"
	[[ $status -eq 1 && $output == *"op/local.env:2 DB_PASSWORD is not an op:// reference"* ]] || fail "status $status: $output"
	[[ $output != *hunter2* ]] || fail "value leaked: $output"
}

@test "a directory that is not a git work tree cannot be checked" {
	run "$CHECK" "$BATS_TEST_TMPDIR"
	[[ $status -eq 2 ]] || fail "status $status: $output"
}
