# shellcheck shell=bash
# extract decompresses single-file formats (.gz, .bz2, .xz, .Z) into the
# new folder and keeps the original archive (YAN-2490).

load test_helper

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	WORK="$BATS_TEST_TMPDIR/work"
	mkdir -p "$FAKEBIN" "$WORK"
}

# run_extract <shell> <stdin> <archive>
run_extract() {
	local tested_shell
	tested_shell="$(command -v "$1")" || fail "$1 not found"
	run env -i HOME="$BATS_TEST_TMPDIR" PATH="$FAKEBIN:/usr/bin:/bin" \
		REPO_ROOT="$REPO_ROOT" WORK="$WORK" ARCHIVE="$3" \
		DOTFILES_OS=linux DOTFILES_NO_AUTO_SETUP=1 \
		"$tested_shell" -f -c '
			source "$REPO_ROOT/common/platform.sh" &&
			source "$REPO_ROOT/common/functions.sh" || exit 1
			cd "$WORK" && extract "$ARCHIVE"
		' <<<"$2"
}

# make_single <ext>: compress $WORK/again.txt into $WORK/again.txt.<ext>
make_single() {
	printf 'hi\n' >"$WORK/again.txt"
	case "$1" in
		gz) gzip "$WORK/again.txt" ;;
		bz2) bzip2 "$WORK/again.txt" ;;
		xz) xz "$WORK/again.txt" ;;
		Z)
			# compress(1) is rarely installed; a stub uncompress that
			# honours -c stands in for it.
			mv "$WORK/again.txt" "$WORK/again.txt.Z"
			cat >"$FAKEBIN/uncompress" <<'STUB'
#!/bin/sh
[ "$1" = -c ] || { echo "uncompress called without -c" >&2; exit 2; }
cat "$2"
STUB
			chmod +x "$FAKEBIN/uncompress"
			;;
	esac
}

check_single_into_folder() {
	local shell="$1" ext="$2"
	make_single "$ext"
	run_extract "$shell" '' "again.txt.$ext"
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ $(cat "$WORK/again/again.txt") == hi ]] || fail "not in folder: $(ls -R "$WORK")"
	[[ -f "$WORK/again.txt.$ext" ]] || fail "archive consumed: $(ls -R "$WORK")"
	[[ ! -e "$WORK/again.txt" ]] || fail "output beside archive: $(ls -R "$WORK")"
}

check_single_overwrite_on_yes() {
	make_single gz
	mkdir -p "$WORK/again"
	touch "$WORK/again/old"
	run_extract "$1" y again.txt.gz
	[[ "$status" -eq 0 ]] || fail "exit $status: $output"
	[[ ! -e "$WORK/again/old" ]] || fail "old file kept: $output"
	[[ $(cat "$WORK/again/again.txt") == hi ]] || fail "not extracted: $output"
}

check_single_failure_cleans_output() {
	printf 'not gzip\n' >"$WORK/bad.txt.gz"
	run_extract "$1" '' bad.txt.gz
	[[ "$status" -ne 0 ]] || fail "expected failure: $output"
	[[ ! -e "$WORK/bad/bad.txt" ]] || fail "partial output kept"
	[[ -f "$WORK/bad.txt.gz" ]] || fail "archive removed"
}

@test "extract puts a .gz into the new folder in Bash" {
	check_single_into_folder "${TEST_BASH:-bash}" gz
}

@test "extract puts a .gz into the new folder in zsh" {
	check_single_into_folder zsh gz
}

@test "extract puts a .bz2 into the new folder in Bash" {
	check_single_into_folder "${TEST_BASH:-bash}" bz2
}

@test "extract puts a .bz2 into the new folder in zsh" {
	check_single_into_folder zsh bz2
}

@test "extract puts a .xz into the new folder in Bash" {
	check_single_into_folder "${TEST_BASH:-bash}" xz
}

@test "extract puts a .xz into the new folder in zsh" {
	check_single_into_folder zsh xz
}

@test "extract puts a .Z into the new folder in Bash" {
	check_single_into_folder "${TEST_BASH:-bash}" Z
}

@test "extract puts a .Z into the new folder in zsh" {
	check_single_into_folder zsh Z
}

@test "extract replaces an existing folder with a .gz on y in Bash" {
	check_single_overwrite_on_yes "${TEST_BASH:-bash}"
}

@test "extract replaces an existing folder with a .gz on y in zsh" {
	check_single_overwrite_on_yes zsh
}

@test "extract removes partial output when a .gz is corrupt in Bash" {
	check_single_failure_cleans_output "${TEST_BASH:-bash}"
}

@test "extract removes partial output when a .gz is corrupt in zsh" {
	check_single_failure_cleans_output zsh
}
