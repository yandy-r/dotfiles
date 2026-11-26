# shellcheck shell=bash
# killport kills every unique PID on a port, one kill per PID, even when the
# process listens on both IPv4 and IPv6 (YAN-1434).

load test_helper

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$FAKEBIN"

	# Linux: one listener (PID 239218) on 127.0.0.1 and ::1, plus a second
	# process (PID 4242) on the same port and an unrelated port that must be ignored.
	cat >"$FAKEBIN/ss" <<'STUB'
#!/bin/sh
cat <<'EOF'
Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process
tcp   LISTEN 0      5          127.0.0.1:47123      0.0.0.0:*     users:(("python3",pid=239218,fd=3))
tcp   LISTEN 0      5              [::1]:47123         [::]:*     users:(("python3",pid=239218,fd=4))
tcp   LISTEN 0      5          127.0.0.1:47123      0.0.0.0:*     users:(("node",pid=4242,fd=7))
tcp   LISTEN 0      5          127.0.0.1:5000       0.0.0.0:*     users:(("other",pid=777,fd=3))
EOF
STUB
	# macOS: lsof -t prints one PID per socket, so duplicates appear too.
	cat >"$FAKEBIN/lsof" <<'STUB'
#!/bin/sh
printf '%s\n' 239218 239218 4242
STUB
	chmod +x "$FAKEBIN/ss" "$FAKEBIN/lsof"
}

run_killport() {
	local tested_shell
	tested_shell="$(command -v "$1")" || fail "$1 not found"
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="$FAKEBIN:/usr/bin:/bin" \
		REPO_ROOT="$REPO_ROOT" TEST_TMPDIR="$BATS_TEST_TMPDIR" \
		DOTFILES_OS="$2" DOTFILES_NO_AUTO_SETUP=1 \
		"$tested_shell" -f -c '
			source "$REPO_ROOT/common/platform.sh" &&
			source "$REPO_ROOT/common/functions.sh" || exit 1
			kill() {
				[ "$#" -eq 1 ] || { echo "kill got $# args" >&2; return 2; }
				case "$1" in
					*[!0-9]*|"") echo "kill got non-pid: $1" >&2; return 1 ;;
				esac
				printf "%s\n" "$1" >>"$TEST_TMPDIR/killed"
			}
			killport '"$3"'
		'
}

check_kills_unique_pids() {
	run_killport "$1" "$2" 47123
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ $(cat "$BATS_TEST_TMPDIR/killed") == $'239218\n4242' ]] ||
		fail "killed: $(cat "$BATS_TEST_TMPDIR/killed" 2>/dev/null)"
}

check_no_process() {
	run_killport "$1" linux 9999
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ "$output" == "No process found using port 9999" ]] || fail "$output"
	[[ ! -e "$BATS_TEST_TMPDIR/killed" ]]
}

@test "killport kills a dual-stack listener once per unique PID on Linux in Bash" {
	check_kills_unique_pids "${TEST_BASH:-bash}" linux
}

@test "killport kills a dual-stack listener once per unique PID on Linux in zsh" {
	check_kills_unique_pids zsh linux
}

@test "killport kills each unique lsof PID on macOS in Bash" {
	check_kills_unique_pids "${TEST_BASH:-bash}" macos
}

@test "killport kills each unique lsof PID on macOS in zsh" {
	check_kills_unique_pids zsh macos
}

@test "killport reports when nothing listens on the port in Bash" {
	check_no_process "${TEST_BASH:-bash}"
}

@test "killport reports when nothing listens on the port in zsh" {
	check_no_process zsh
}
