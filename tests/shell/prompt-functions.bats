# shellcheck shell=bash
# extract and killall_name prompt and act the same way in bash and zsh:
# zsh's `read -p` means "read from the coprocess", and zsh does not
# word-split an unquoted PID list (YAN-1424).

load test_helper

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$FAKEBIN"

	# pgrep reports three matches, one per line, like the real tool.
	cat >"$FAKEBIN/pgrep" <<'STUB'
#!/bin/sh
printf '%s\n' 101 202 303
STUB
	# ps records its arguments so the test can check the PID list.
	cat >"$FAKEBIN/ps" <<'STUB'
#!/bin/sh
printf '%s\n' "$@" >"$TEST_TMPDIR/ps-args"
echo "PID COMMAND"
STUB
	chmod +x "$FAKEBIN/pgrep" "$FAKEBIN/ps"
}

# run_fn <shell> <stdin> <snippet>
run_fn() {
	local tested_shell
	tested_shell="$(command -v "$1")" || fail "$1 not found"
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="$FAKEBIN:/usr/bin:/bin" \
		REPO_ROOT="$REPO_ROOT" TEST_TMPDIR="$BATS_TEST_TMPDIR" \
		DOTFILES_OS=linux DOTFILES_NO_AUTO_SETUP=1 \
		"$tested_shell" -f -c '
			source "$REPO_ROOT/common/platform.sh" &&
			source "$REPO_ROOT/common/functions.sh" || exit 1
			kill() { printf "%s\n" "$@" >"$TEST_TMPDIR/killed"; }
			'"$3" <<<"$2"
}

make_archive() {
	local work="$BATS_TEST_TMPDIR/work"
	mkdir -p "$work/again"
	echo stale >"$work/again/old"
	mkdir -p "$work/src"
	echo fresh >"$work/src/new"
	tar czf "$work/again.tar.gz" -C "$work/src" new
}

check_extract_overwrites_on_yes() {
	make_archive
	run_fn "$1" y 'cd "$TEST_TMPDIR/work" && extract again.tar.gz'
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ "$output" != *coprocess* ]] || fail "$output"
	local work="$BATS_TEST_TMPDIR/work"
	[[ ! -e "$work/again/old" ]] || fail "old dir kept: $output"
	[[ $(cat "$work/again/new") == fresh ]] || fail "not extracted: $output"
}

check_extract_keeps_on_no() {
	make_archive
	run_fn "$1" n 'cd "$TEST_TMPDIR/work" && extract again.tar.gz'
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	local work="$BATS_TEST_TMPDIR/work"
	[[ -f "$work/again/old" ]] || fail "old dir removed: $output"
	[[ ! -e "$work/again/new" ]] || fail "extracted on n: $output"
}

check_killall_name_kills_each_pid() {
	run_fn "$1" y 'killall_name node'
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ "$output" != *coprocess* ]] || fail "$output"
	[[ $(cat "$BATS_TEST_TMPDIR/ps-args") == $'-p\n101,202,303\n-o\npid,comm,args' ]] ||
		fail "ps args: $(cat "$BATS_TEST_TMPDIR/ps-args")"
	[[ $(cat "$BATS_TEST_TMPDIR/killed") == $'101\n202\n303' ]] ||
		fail "killed: $(cat "$BATS_TEST_TMPDIR/killed" 2>/dev/null)"
}

check_killall_name_spares_on_no() {
	run_fn "$1" n 'killall_name node'
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ ! -e "$BATS_TEST_TMPDIR/killed" ]] || fail "killed on no"
}

check_killall_name_no_match() {
	printf '#!/bin/sh\nexit 1\n' >"$FAKEBIN/pgrep"
	run_fn "$1" y 'killall_name nothing'
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ "$output" == "No processes found matching 'nothing'" ]] || fail "$output"
	[[ ! -e "$BATS_TEST_TMPDIR/ps-args" && ! -e "$BATS_TEST_TMPDIR/killed" ]]
}

@test "extract overwrites an existing folder on y in Bash" {
	check_extract_overwrites_on_yes "${TEST_BASH:-bash}"
}

@test "extract overwrites an existing folder on y in zsh" {
	check_extract_overwrites_on_yes zsh
}

@test "extract keeps an existing folder on n in Bash" {
	check_extract_keeps_on_no "${TEST_BASH:-bash}"
}

@test "extract keeps an existing folder on n in zsh" {
	check_extract_keeps_on_no zsh
}

@test "killall_name lists and kills every matching PID in Bash" {
	check_killall_name_kills_each_pid "${TEST_BASH:-bash}"
}

@test "killall_name lists and kills every matching PID in zsh" {
	check_killall_name_kills_each_pid zsh
}

@test "killall_name kills nothing on n in Bash" {
	check_killall_name_spares_on_no "${TEST_BASH:-bash}"
}

@test "killall_name kills nothing on n in zsh" {
	check_killall_name_spares_on_no zsh
}

@test "killall_name reports when nothing matches in Bash" {
	check_killall_name_no_match "${TEST_BASH:-bash}"
}

@test "killall_name reports when nothing matches in zsh" {
	check_killall_name_no_match zsh
}
