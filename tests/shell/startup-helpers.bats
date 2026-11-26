# shellcheck shell=bash
# Probe code expands in the child shell, not in Bats.
# shellcheck disable=SC2016
load test_helper

setup() {
	TEST_BASH="${TEST_BASH:-$(command -v bash)}"
	TEST_ZSH="$(command -v zsh || true)"
	FAKE_HOME="$BATS_TEST_TMPDIR/home"
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	mkdir -p "$FAKE_HOME" "$FAKEBIN"
}

probe() {
	local shell="$1" os="$2" code="$3" output_file
	shift 3
	output_file="$BATS_TEST_TMPDIR/probe.log"
	if ! env -i HOME="$FAKE_HOME" PATH="$FAKEBIN:/usr/bin:/bin" TERM=dumb \
		DOTFILES_ROOT="$REPO_ROOT" DOTFILES_OS="$os" DOTFILES_HOMEBREW_PATH= \
		DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_TOOL_SETUP=1 "$shell" -c '
			source "$DOTFILES_ROOT/common/platform.sh"
			probe_code=$1; shift
			eval "$probe_code"
		' _ "$code" "$@" >"$output_file" 2>&1; then
		cat "$output_file" >&2
		fail "${shell##*/} probe failed"
	fi
}

@test "CMake defaults to Ninja only when available and preserves an explicit generator" {
	local check='source "$DOTFILES_ROOT/common/exports.sh"; sh -c '\''test "${CMAKE_GENERATOR-unset}" = "$1" && test "${CMAKE_MAKE_PROGRAM-unset}" = "$2"'\'' _ "$@"'
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" linux 'has_command() { [[ "$1" != ninja ]] && command -v "$1" >/dev/null; }; '"$check" unset unset
		probe "$shell" linux 'has_command() { [[ "$1" != ninja ]] && command -v "$1" >/dev/null; }; CMAKE_GENERATOR=Xcode; '"$check" Xcode unset
		printf '#!/bin/sh\nexit 0\n' >"$FAKEBIN/ninja"
		chmod +x "$FAKEBIN/ninja"
		probe "$shell" linux 'has_command() { command -v "$1" >/dev/null; }; '"$check" Ninja "$FAKEBIN/ninja"
		probe "$shell" linux 'has_command() { command -v "$1" >/dev/null; }; export CMAKE_GENERATOR="Unix Makefiles"; '"$check" "Unix Makefiles" "$FAKEBIN/ninja"
	done
}

@test "macOS exports the Java home reported by the system helper and preserves it on lookup failure" {
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" macos '
			has_command() { return 1; }
			function /usr/libexec/java_home { printf "%s\n" "$HOME/temurin-21.jdk/Contents/Home"; }
			source "$DOTFILES_ROOT/common/exports.sh"
			sh -c '\''test "$JAVA_HOME" = "$HOME/temurin-21.jdk/Contents/Home"'\'' || exit 1
			function /usr/libexec/java_home { printf "%s\n" /unusable; return 1; }
			source "$DOTFILES_ROOT/common/exports.sh"
			[[ "$JAVA_HOME" == "$HOME/temurin-21.jdk/Contents/Home" ]]
		'
	done
}

@test "Atuin only prepends its path when its install directory is present" {
	printf '#!/bin/sh\nexit 0\n' >"$FAKEBIN/atuin"
	chmod +x "$FAKEBIN/atuin"
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" linux '
			source "$DOTFILES_ROOT/common/tools.sh"
			has_command() { command -v "$1" >/dev/null; }
			original_path=$PATH
			setup_atuin
			[[ "$PATH" == "$original_path" ]] || exit 1
			mkdir -p "$HOME/.atuin/bin"
			setup_atuin
			[[ "$PATH" == "$HOME/.atuin/bin:$original_path" ]]
		'
		rmdir "$FAKE_HOME/.atuin/bin" "$FAKE_HOME/.atuin"
	done
}

@test "genpass emits the requested number of characters with pipefail enabled" {
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" linux '
			has_command() { command -v "$1" >/dev/null; }
			source "$DOTFILES_ROOT/common/functions.sh"
			set -o pipefail
			for length in 1 64 4096; do
				value=$(genpass "$length") || exit 1
				[[ ${#value} == "$length" && "$value" != *[!A-Za-z0-9]* ]] || exit 1
			done
			value=$(genpass) && [[ ${#value} == 16 ]]
		'
	done
}

@test "git_log_github emits the same links for HTTPS and SSH origins" {
	for shell in "$TEST_BASH" ${TEST_ZSH:+"$TEST_ZSH"}; do
		probe "$shell" linux '
			cd "$HOME" || exit 1
			git init -q && touch example.txt && git add example.txt &&
				git -c user.name=Test -c user.email=test@example.invalid commit -qm fixture || exit 1
			source "$DOTFILES_ROOT/common/functions.sh"
			export PAGER=cat
			sha=$(git rev-parse HEAD) || exit 1
			for remote in https://github.com/example/repo.git git@github.com:example/repo.git; do
				git remote add origin "$remote" || exit 1
				git_log_github | grep -Fq "https://github.com/example/repo/blob/${sha%${sha#???????}}/example.txt" || exit 1
				git remote remove origin || exit 1
			done
			rm -r .git
		'
	done
}

@test "Zsh startup respects a reduced hard limit and raises only the soft limit when allowed" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	printf 'skip_global_compinit=1\n' >"$FAKE_HOME/.zshenv"
	probe "$TEST_ZSH" linux '
		ulimit -Sn 128 && ulimit -Hn 1024 || exit 1
		"$1" -i -c '\''source "$DOTFILES_ROOT/zsh/zshrc"
			[[ $(ulimit -Sn) == 128 && $(ulimit -Hn) == 1024 ]]'\''
	' "$TEST_ZSH"
	if grep -q 'ulimit:' "$BATS_TEST_TMPDIR/probe.log"; then
		cat "$BATS_TEST_TMPDIR/probe.log" >&2
		fail "Zsh emitted an error with a reduced hard limit"
	fi

	# Simulate a high hard limit rather than assuming the host allows 65536.
	probe "$TEST_ZSH" linux '
		ulimit() {
			case "$*" in
				-Sn) echo 128 ;;
				-Hn) echo unlimited ;;
				"-Sn 65536") raised=yes ;;
				*) raised=unexpected; return 1 ;;
			esac
		}
		source "$DOTFILES_ROOT/zsh/zshrc"
		[[ "$raised" == yes ]]
	'
}
