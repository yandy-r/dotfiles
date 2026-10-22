#!/usr/bin/env bash
# Regression tests for the cached platform predicates (YAN-1340):
#   - common/platform.sh's is_macos, is_linux, is_wsl, is_container, is_arm64
#     and is_x86_64 answer from DOTFILES_OS / DOTFILES_CONTAINER / DOTFILES_ARCH
#     (exported by setup_platform_env) without calling any detect_* function.
#   - When a var is unset or empty they fall back to detect_* and return the
#     answer the detected value implies.
#   - After a normal (auto-setup) source, is_linux / is_macos agree with uname.
# Every case runs under bash and, when installed, zsh. detect_* are replaced
# with stubs that log each call to a file, so the results never depend on the
# host platform (except the final real-detection sanity case).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TMP_RAW="$(mktemp -d)"
TMP_DIR="$(cd "$TMP_RAW" && pwd -P)"
trap '[[ -n "${KEEP_TMP:-}" ]] || rm -rf "$TMP_DIR"' EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

SHELLS=(bash)
if command -v zsh >/dev/null 2>&1; then
	SHELLS+=(zsh)
fi

# Driver sourced by each shell. It is written in the bash/zsh common subset:
# word lists are literal (zsh does not split unquoted parameters).
DRIVER="$TMP_DIR/driver.sh"
cat >"$DRIVER" <<'EOF'
DOTFILES_NO_AUTO_SETUP=1
. "$PLATFORM_SH"
unset DOTFILES_NO_AUTO_SETUP

CALLS="$TMP_DIR/calls"
STUB_OS=unknown STUB_CONTAINER=none STUB_ARCH=unknown
detect_os() { echo os >>"$CALLS"; echo "$STUB_OS"; }
detect_container() { echo container >>"$CALLS"; echo "$STUB_CONTAINER"; }
detect_arch() { echo arch >>"$CALLS"; echo "$STUB_ARCH"; }

die() { echo "FAIL [$SHELL_NAME]: $*" >&2; exit 1; }
reset_calls() { : >"$CALLS"; }
calls() { wc -l <"$CALLS" | tr -d ' '; }

# expect <predicate> <0|1> <context>
expect() {
	if "$1"; then got=0; else got=1; fi
	[ "$got" = "$2" ] || die "$1 returned $got, want $2 ($3)"
}

# Cached path: every value, no detect_* call.
export DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
for os in macos debian redhat arch suse linux wsl unknown; do
	export DOTFILES_OS="$os"
	mac=1 lin=1 wsl=1
	case "$os" in
		macos) mac=0 ;;
		debian|redhat|arch|suse|linux) lin=0 ;;
		wsl) wsl=0 ;;
	esac
	reset_calls
	expect is_macos "$mac" "DOTFILES_OS=$os"
	expect is_linux "$lin" "DOTFILES_OS=$os"
	expect is_wsl "$wsl" "DOTFILES_OS=$os"
	[ "$(calls)" = 0 ] || die "detect_* called with DOTFILES_OS=$os: $(tr '\n' ' ' <"$CALLS")"
done

export DOTFILES_OS=debian
for ctr in wsl docker none; do
	export DOTFILES_CONTAINER="$ctr"
	wsl=1 cont=0
	case "$ctr" in
		wsl) wsl=0 ;;
		none) cont=1 ;;
	esac
	reset_calls
	expect is_wsl "$wsl" "DOTFILES_CONTAINER=$ctr"
	expect is_container "$cont" "DOTFILES_CONTAINER=$ctr"
	[ "$(calls)" = 0 ] || die "detect_* called with DOTFILES_CONTAINER=$ctr: $(tr '\n' ' ' <"$CALLS")"
done

for arch in arm64 x86_64; do
	export DOTFILES_ARCH="$arch"
	arm=1 x86=1
	case "$arch" in
		arm64) arm=0 ;;
		x86_64) x86=0 ;;
	esac
	reset_calls
	expect is_arm64 "$arm" "DOTFILES_ARCH=$arch"
	expect is_x86_64 "$x86" "DOTFILES_ARCH=$arch"
	[ "$(calls)" = 0 ] || die "detect_* called with DOTFILES_ARCH=$arch: $(tr '\n' ' ' <"$CALLS")"
done

# Fallback path: unset and empty vars both defer to detect_*.
for mode in unset empty; do
	if [ "$mode" = unset ]; then
		unset DOTFILES_OS DOTFILES_CONTAINER DOTFILES_ARCH
	else
		export DOTFILES_OS= DOTFILES_CONTAINER= DOTFILES_ARCH=
	fi

	STUB_OS=macos STUB_CONTAINER=none
	reset_calls; expect is_macos 0 "$mode, detect_os=macos"
	[ "$(calls)" = 1 ] || die "is_macos ($mode): want 1 detect call, got $(calls)"
	reset_calls; expect is_linux 1 "$mode, detect_os=macos"
	[ "$(calls)" = 1 ] || die "is_linux ($mode): want 1 detect call, got $(calls)"

	STUB_OS=arch
	reset_calls; expect is_linux 0 "$mode, detect_os=arch"
	reset_calls; expect is_macos 1 "$mode, detect_os=arch"

	# is_wsl consults the container only when the OS check fails.
	STUB_OS=wsl
	reset_calls; expect is_wsl 0 "$mode, detect_os=wsl"
	[ "$(tr '\n' ' ' <"$CALLS")" = "os " ] || die "is_wsl ($mode, os=wsl): calls $(tr '\n' ' ' <"$CALLS")"
	STUB_OS=debian STUB_CONTAINER=wsl
	reset_calls; expect is_wsl 0 "$mode, detect_container=wsl"
	[ "$(tr '\n' ' ' <"$CALLS")" = "os container " ] || die "is_wsl ($mode, container=wsl): calls $(tr '\n' ' ' <"$CALLS")"
	STUB_CONTAINER=none
	reset_calls; expect is_wsl 1 "$mode, debian/none"

	STUB_CONTAINER=docker
	reset_calls; expect is_container 0 "$mode, detect_container=docker"
	[ "$(calls)" = 1 ] || die "is_container ($mode): want 1 detect call, got $(calls)"
	STUB_CONTAINER=none
	reset_calls; expect is_container 1 "$mode, detect_container=none"

	STUB_ARCH=arm64
	reset_calls; expect is_arm64 0 "$mode, detect_arch=arm64"
	[ "$(calls)" = 1 ] || die "is_arm64 ($mode): want 1 detect call, got $(calls)"
	reset_calls; expect is_x86_64 1 "$mode, detect_arch=arm64"
	STUB_ARCH=x86_64
	reset_calls; expect is_x86_64 0 "$mode, detect_arch=x86_64"
	reset_calls; expect is_arm64 1 "$mode, detect_arch=x86_64"
done
echo ok
EOF

# Real detection: normal source runs setup_platform_env; compare with uname.
REAL="$TMP_DIR/real.sh"
cat >"$REAL" <<'EOF'
. "$PLATFORM_SH"
[ -n "${DOTFILES_OS:-}" ] || { echo "FAIL [$SHELL_NAME]: DOTFILES_OS not exported by auto setup" >&2; exit 1; }
if is_macos; then mac=0; else mac=1; fi
if is_linux || is_wsl; then lin=0; else lin=1; fi
case "$(uname -s)" in
	Darwin*) want_mac=0 want_lin=1 ;;
	Linux*) want_mac=1 want_lin=0 ;;
	*) want_mac=1 want_lin=1 ;;
esac
[ "$mac" = "$want_mac" ] || { echo "FAIL [$SHELL_NAME]: is_macos=$mac, uname=$(uname -s)" >&2; exit 1; }
[ "$lin" = "$want_lin" ] || { echo "FAIL [$SHELL_NAME]: is_linux/is_wsl=$lin, uname=$(uname -s)" >&2; exit 1; }
echo ok
EOF

run_shell() {
	local sh="$1" script="$2" out
	local -a argv=("$sh")
	[[ "$sh" == zsh ]] && argv+=(-f)
	out="$(env -i PATH="$PATH" HOME="$TMP_DIR" TMP_DIR="$TMP_DIR" \
		PLATFORM_SH="$REPO_ROOT/common/platform.sh" SHELL_NAME="$sh" \
		"${argv[@]}" "$script" 2>&1)" || fail "$sh $(basename "$script"): $out"
	[[ "$out" == ok ]] || fail "$sh $(basename "$script"): unexpected output: $out"
}

for sh in "${SHELLS[@]}"; do
	run_shell "$sh" "$DRIVER"
	run_shell "$sh" "$REAL"
done

echo "PASS: platform predicates read cached DOTFILES_* vars and fall back to detect_* (${SHELLS[*]})"
