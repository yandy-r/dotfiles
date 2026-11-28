# shellcheck shell=bash
# bin/init-envrc scaffolds a project for 1Password secrets (YAN-2498): an
# op/local.env template of op:// references only, and .gitignore entries that
# ignore real env files but keep .env.example tracked.

load test_helper

setup() {
	INIT="$REPO_ROOT/bin/init-envrc"
	PROJECT="$BATS_TEST_TMPDIR/demo"
	mkdir -p "$PROJECT"
	git -C "$PROJECT" init -q
}

scaffold() {
	cd "$PROJECT" && "$INIT" demo </dev/null
}

@test "scaffolds an op:// template that passes the secrets policy" {
	run scaffold
	[[ $status -eq 0 ]] || fail "status $status: $output"
	[[ -f $PROJECT/op/local.env && -f $PROJECT/.envrc ]] || fail "missing files"
	git -C "$PROJECT" add -A
	run "$REPO_ROOT/scripts/secrets-policy-check.sh" "$PROJECT"
	[[ $status -eq 0 ]] || fail "policy: $output"
	git -C "$PROJECT" ls-files --error-unmatch op/local.env .env.example >/dev/null || fail "template not tracked"
	! git -C "$PROJECT" ls-files --error-unmatch .env >/dev/null 2>&1 || fail ".env tracked"
}

@test "an existing .env then !.env.example order keeps .env.example tracked" {
	printf 'node_modules\n.env\n!.env.example' >"$PROJECT/.gitignore"
	run scaffold
	[[ $status -eq 0 ]] || fail "status $status: $output"
	git -C "$PROJECT" check-ignore -q .env.example && fail ".env.example is ignored: $(cat "$PROJECT/.gitignore")"
	git -C "$PROJECT" check-ignore -q .env.local || fail ".env.local not ignored"
}

@test "a correctly ordered .gitignore is left unchanged" {
	printf '.env\n.env.*\n!.env.example\n' >"$PROJECT/.gitignore"
	cp "$PROJECT/.gitignore" "$BATS_TEST_TMPDIR/before"
	run scaffold
	[[ $status -eq 0 ]] || fail "status $status: $output"
	cmp -s "$PROJECT/.gitignore" "$BATS_TEST_TMPDIR/before" || fail "changed: $(cat "$PROJECT/.gitignore")"
}
