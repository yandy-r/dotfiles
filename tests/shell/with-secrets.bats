# shellcheck shell=bash
# bin/with-secrets runs a command through `op run` when the env template holds
# op:// references, runs it plainly when it holds none, and fails fast when
# references exist but op is missing or signed out (YAN-2498). `op` is a stub
# that records its arguments; no real 1Password call is made.

load test_helper

setup() {
	LOADER="$REPO_ROOT/bin/with-secrets"
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	PROJECT="$BATS_TEST_TMPDIR/project"
	OP_LOG="$BATS_TEST_TMPDIR/op.log"
	mkdir -p "$FAKEBIN" "$PROJECT/op"
	export OP_LOG

	# Stub op: `whoami` succeeds unless OP_STUB_SIGNED_OUT is set; `run`
	# records its arguments, then runs the command after `--`.
	cat >"$FAKEBIN/op" <<'STUB'
#!/usr/bin/env bash
case $1 in
whoami) [[ -z ${OP_STUB_SIGNED_OUT:-} ]] ;;
run)
	printf '%s\n' "$*" >>"$OP_LOG"
	while (($# > 0)) && [[ $1 != -- ]]; do shift; done
	shift
	exec "$@"
	;;
*) exit 64 ;;
esac
STUB
	chmod +x "$FAKEBIN/op"
	# Only the basics on PATH, so "op missing" means missing.
	BASE_PATH="/usr/bin:/bin"
}

with_op() {
	cd "$PROJECT" && PATH="$FAKEBIN:$BASE_PATH" "$LOADER" "$@"
}

without_op() {
	cd "$PROJECT" && PATH="$BASE_PATH" "$LOADER" "$@"
}

@test "refs + op signed in: runs the command through op run --env-file" {
	printf '# local\nAPI_KEY=op://EnvSecrets/DEMO_LOCAL/API_KEY\n' >"$PROJECT/op/local.env"
	run with_op -- echo hello
	[[ $status -eq 0 ]] || fail "status $status: $output"
	[[ $output == hello ]] || fail "unexpected output: $output"
	grep -qx 'run --env-file=op/local.env -- echo hello' "$OP_LOG" || fail "op args: $(cat "$OP_LOG")"
}

@test "-e selects op/<env>.env and --no-masking is passed through" {
	printf 'DB_URL="op://EnvSecrets/DEMO_PROD/DB_URL"\n' >"$PROJECT/op/prod.env"
	run with_op -e prod --no-masking -- true
	[[ $status -eq 0 ]] || fail "status $status: $output"
	grep -qx 'run --env-file=op/prod.env --no-masking -- true' "$OP_LOG" || fail "op args: $(cat "$OP_LOG")"
}

@test "no refs: runs the command plainly without calling op" {
	printf '# commented only\n# API_KEY=op://EnvSecrets/DEMO/API_KEY\nAPP_ENV=development\n' >"$PROJECT/op/local.env"
	run with_op -- echo plain
	[[ $status -eq 0 && $output == plain ]] || fail "status $status: $output"
	[[ ! -e $OP_LOG ]] || fail "op was called: $(cat "$OP_LOG")"
}

@test "no template: runs the command plainly, even without op" {
	run without_op -- echo plain
	[[ $status -eq 0 && $output == plain ]] || fail "status $status: $output"
}

@test "the command's exit status is preserved" {
	run without_op -- sh -c 'exit 7'
	[[ $status -eq 7 ]] || fail "status $status: $output"
}

@test "refs + op missing: fails fast with an install hint" {
	printf 'API_KEY=op://EnvSecrets/DEMO_LOCAL/API_KEY\n' >"$PROJECT/op/local.env"
	run without_op -- echo should-not-run
	[[ $status -eq 1 ]] || fail "status $status: $output"
	[[ $output == *"op CLI is not installed"* ]] || fail "missing hint: $output"
	[[ $output != *should-not-run* ]] || fail "command ran: $output"
}

@test "refs + op signed out: fails fast without running op run" {
	printf 'API_KEY=op://EnvSecrets/DEMO_LOCAL/API_KEY\n' >"$PROJECT/op/local.env"
	OP_STUB_SIGNED_OUT=1 run with_op -- echo should-not-run
	[[ $status -eq 1 ]] || fail "status $status: $output"
	[[ $output == *"not signed in"* ]] || fail "missing hint: $output"
	[[ ! -e $OP_LOG ]] || fail "op run was called: $(cat "$OP_LOG")"
}

@test "spaced NAME = op:// references still count as references" {
	printf 'API_KEY = op://EnvSecrets/DEMO_LOCAL/API_KEY\n' >"$PROJECT/op/local.env"
	run with_op -- true
	[[ $status -eq 0 ]] || fail "status $status: $output"
	grep -qx 'run --env-file=op/local.env -- true' "$OP_LOG" || fail "op not used: $(cat "$OP_LOG" 2>/dev/null)"
}

@test "an unreadable template fails instead of running without secrets" {
	[[ $(id -u) -ne 0 ]] || skip "root reads any file"
	printf 'API_KEY=op://EnvSecrets/DEMO_LOCAL/API_KEY\n' >"$PROJECT/op/local.env"
	chmod 000 "$PROJECT/op/local.env"
	run with_op -- echo should-not-run
	chmod 600 "$PROJECT/op/local.env"
	[[ $status -eq 1 && $output == *"cannot read env template"* ]] || fail "status $status: $output"
	[[ $output != *should-not-run* ]] || fail "command ran: $output"
}

@test "an explicit -f template that does not exist is an error" {
	run with_op -f op/missing.env -- true
	[[ $status -eq 1 && $output == *"env template not found"* ]] || fail "status $status: $output"
}

@test "no command is a usage error" {
	run with_op
	[[ $status -eq 2 ]] || fail "status $status: $output"
}
