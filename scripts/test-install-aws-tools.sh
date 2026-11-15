#!/usr/bin/env bash
# Hermetic failure-propagation tests for bin/install-aws-tools.sh (YAN-1388).
# Never downloads anything or touches the host: curl, unzip, sudo and uname are
# stubs on a PATH that excludes any real aws/sam/ecs-cli and package managers.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALLER="$REPO_ROOT/bin/install-aws-tools.sh"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/test-install-aws-tools.XXXXXX")"
trap 'rm -rf -- "$TEST_ROOT"' EXIT

# Build dotf (for the end-to-end case) with the caller's real Go environment,
# before HOME and PATH are sandboxed. DOTF_BIN overrides; no Go skips the case.
DOTF_E2E="${DOTF_BIN:-}"
if [[ -z "$DOTF_E2E" ]] && command -v go >/dev/null 2>&1; then
	DOTF_E2E="$TEST_ROOT/dotf"
	(cd "$REPO_ROOT" && go build -o "$DOTF_E2E" ./cmd/dotf) || {
		echo "FAIL: go build ./cmd/dotf" >&2
		exit 1
	}
fi

export HOME="$TEST_ROOT/home"
export XDG_CACHE_HOME="$HOME/.cache"
export TMPDIR="$TEST_ROOT/tmp"
STUBS="$TEST_ROOT/stubs"
TOOLS="$TEST_ROOT/tools"
INSTALL_DIR="$TEST_ROOT/install-bin"
mkdir -p "$HOME" "$TMPDIR" "$STUBS" "$TOOLS" "$INSTALL_DIR"

# Host userland minus anything that could perform or fake a real install.
for tool in /usr/bin/* /bin/*; do
	name="${tool##*/}"
	case "$name" in
	aws | sam | ecs-cli | curl | unzip | sudo | uname | installer | apt* | dnf | pacman | zypper | brew) continue ;;
	esac
	# /bin may be a symlink to /usr/bin (merged-usr): skip names already linked,
	# including dangling links, which -e alone would miss.
	[[ -e "$TOOLS/$name" || -L "$TOOLS/$name" ]] || ln -s "$tool" "$TOOLS/$name"
done

# Pin the platform to Linux x86_64 regardless of the runner.
cat >"$STUBS/uname" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
-m) echo x86_64 ;;
*) echo Linux ;;
esac
EOF
# sudo passes through (no privilege change).
cat >"$STUBS/sudo" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == "-v" || "${1:-}" == "-n" ]] && exit 0
exec "$@"
EOF
# curl: exit CURL_EXIT (default 0); on success write a placeholder to -o.
cat >"$STUBS/curl" <<'EOF'
#!/usr/bin/env bash
code="${CURL_EXIT:-0}"
[[ "$code" == 0 ]] || { echo "curl: (6) Could not resolve host" >&2; exit "$code"; }
while [[ $# -gt 0 ]]; do
	if [[ "$1" == "-o" ]]; then : >"$2"; shift; fi
	shift
done
EOF
# unzip: exit UNZIP_EXIT (default 0); on success lay down aws/install, which
# exits AWS_INSTALL_EXIT and, when AWS_INSTALL_CREATES=true, creates aws in
# --bin-dir.
cat >"$STUBS/unzip" <<'EOF'
#!/usr/bin/env bash
code="${UNZIP_EXIT:-0}"
[[ "$code" == 0 ]] || { echo "unzip: cannot find zipfile" >&2; exit "$code"; }
dest=.
while [[ $# -gt 0 ]]; do
	if [[ "$1" == "-d" ]]; then dest="$2"; shift; fi
	shift
done
mkdir -p "$dest/aws"
cat >"$dest/aws/install" <<'INNER'
#!/usr/bin/env bash
bin_dir=""
while [[ $# -gt 0 ]]; do
	if [[ "$1" == "--bin-dir" ]]; then bin_dir="$2"; shift; fi
	shift
done
[[ "${AWS_INSTALL_EXIT:-0}" == 0 ]] || { echo "aws/install: failed" >&2; exit "$AWS_INSTALL_EXIT"; }
if [[ "${AWS_INSTALL_CREATES:-false}" == true ]]; then
	printf '#!/usr/bin/env bash\necho "aws-cli/2.0.0 Python/3 Linux"\n' >"$bin_dir/aws"
	chmod +x "$bin_dir/aws"
fi
INNER
chmod +x "$dest/aws/install"
EOF
chmod +x "$STUBS"/*

export PATH="$INSTALL_DIR:$STUBS:$TOOLS"
OUT="$TEST_ROOT/out"

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	[[ -f "$OUT" ]] && sed 's/^/  | /' "$OUT" >&2
	exit 1
}

# run_case NAME EXPECTED_EXIT EXPECTED_TEXT [ENV=VAL...]
run_case() {
	local name="$1" want_exit="$2" want_text="$3"
	shift 3
	rm -f "$INSTALL_DIR/aws"
	local rc=0
	env "$@" bash "$INSTALLER" --dir "$INSTALL_DIR" aws >"$OUT" 2>&1 || rc=$?
	[[ "$rc" == "$want_exit" ]] || fail "$name: exit=$rc, want $want_exit"
	grep -Fq -- "$want_text" "$OUT" || fail "$name: missing '$want_text'"
	if [[ "$want_exit" != 0 ]]; then
		! grep -q 'SUCCESS.*AWS CLI v2 installed' "$OUT" ||
			fail "$name: reported SUCCESS on a failed install"
		grep -Fq "aws installation failed" "$OUT" || fail "$name: missing per-tool failure line"
	fi
	printf 'ok - %s\n' "$name"
}

run_case "download failure (curl exit 6)" 1 "Failed to download AWS CLI v2" CURL_EXIT=6
run_case "extract failure (unzip exit 9)" 1 "Failed to extract AWS CLI v2 installer" UNZIP_EXIT=9
run_case "installer failure (aws/install exit 1)" 1 "AWS CLI v2 installer failed" AWS_INSTALL_EXIT=1
run_case "installer exits 0 but aws missing" 1 "aws is not on PATH" AWS_INSTALL_CREATES=false
run_case "successful install" 0 "AWS CLI v2 installed" AWS_INSTALL_CREATES=true

# End to end: dotf's custom-script backend must report the installer as failed
# and exit non-zero. dotf prepends system tool dirs (/usr/local/bin, ...) to
# PATH before running a script, and CI runners ship a real aws there, so the
# real installer is driven through a user-overlay package (HOME is sandboxed)
# whose wrapper restores the sandbox PATH. Skipped when Go is unavailable.
if [[ -n "$DOTF_E2E" ]]; then
	overlay="$HOME/.config/dotf"
	mkdir -p "$overlay/packages.d" "$overlay/bin"
	cat >"$overlay/bin/aws-tools-e2e.sh" <<EOF
#!/usr/bin/env bash
PATH="$PATH" exec bash "$INSTALLER" --dir "$INSTALL_DIR" aws
EOF
	cat >"$overlay/packages.d/aws-tools-e2e.toml" <<'EOF'
[[package]]
name = "aws-tools-e2e"
backend = "custom-script"
script = "bin/aws-tools-e2e.sh"
provides = ["aws-tools-e2e-never-on-path"]
class = "rolling"
justification = "hermetic test wrapper around bin/install-aws-tools.sh"
EOF
	rm -f "$INSTALL_DIR/aws"
	rc=0
	(cd "$REPO_ROOT" && XDG_CONFIG_HOME="$HOME/.config" DOTFILES_ROOT="$REPO_ROOT" CURL_EXIT=6 \
		"$DOTF_E2E" install aws-tools-e2e) >"$OUT" 2>&1 || rc=$?
	[[ "$rc" != 0 ]] || fail "dotf install: exit=0 on a failed AWS CLI install"
	grep -q 'Failed to download AWS CLI v2' "$OUT" || fail "dotf install: installer did not hit the stubbed failure"
	grep -Eq 'aws-tools-e2e:failed' "$OUT" || fail "dotf install: status not reported as failed"
	printf 'ok - dotf install reports the failed AWS CLI install as failed (exit %s)\n' "$rc"
else
	printf 'skip - dotf end-to-end (go not found; set DOTF_BIN to run)\n'
fi
