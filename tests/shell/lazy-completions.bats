# shellcheck shell=bash
# Regression tests for the lazy kubectl wrappers in common/lazy-completions.sh
# (YAN-1426): the `k` shortcut from common/aliases.sh must keep working as
# kubectl after its first use, whichever of `k` or `kubectl` runs first, in
# both bash and zsh. Before the fix, `k` unset itself after the first call and
# the alias was already gone, so the second `k` was "command not found" (127).
#
# Every case runs under env -i with a sandbox PATH of symlinked coreutils plus
# a fake kubectl, sourcing the real rc order: platform -> exports -> aliases ->
# lazy-completions.

load test_helper

TEST_BASH="${TEST_BASH:-$(command -v bash)}"
[[ "$TEST_BASH" == /* ]] || TEST_BASH="$(command -v "$TEST_BASH")"
TEST_ZSH="$(command -v zsh 2>/dev/null || true)"
[[ -z "$TEST_ZSH" || "$TEST_ZSH" == /* ]] || TEST_ZSH="$(command -v "$TEST_ZSH")"

setup() {
	FAKEBIN="$BATS_TEST_TMPDIR/bin"
	FAKE_HOME="$BATS_TEST_TMPDIR/home"
	PROBE="$BATS_TEST_TMPDIR/probe.sh"
	mkdir -p "$FAKEBIN" "$FAKE_HOME"

	local tool path
	for tool in cat ls mkdir dirname basename tty locale tr grep awk sed id uname hostname stat find diff env sh; do
		path="$(type -P "$tool" 2>/dev/null || true)"
		[[ -n "$path" ]] && ln -sf "$path" "$FAKEBIN/$tool"
	done

	# Fake kubectl: emits a tiny completion script, otherwise echoes its args.
	cat >"$FAKEBIN/kubectl" <<'EOF'
#!/bin/sh
if [ "$1" = completion ]; then
	echo '__start_kubectl() { :; }'
	exit 0
fi
echo "real-kubectl $*"
EOF
	chmod 755 "$FAKEBIN/kubectl"

	# Body is shared by bash and zsh. $1 = first shortcut to call (k|kubectl).
	cat >"$PROBE" <<'EOF'
export DOTFILES_NO_AUTO_SETUP=1 DOTFILES_NO_AUTO_COMPLETIONS=1 DOTFILES_NO_AUTO_TOOL_SETUP=1
export DOTFILES_OS=linux DOTFILES_PKG_MGR=apt DOTFILES_CONTAINER=none DOTFILES_ARCH=x86_64
[[ -n "${BASH_VERSION:-}" ]] && shopt -s expand_aliases
source "$REPO_ROOT/common/platform.sh"
source "$REPO_ROOT/common/exports.sh"
source "$REPO_ROOT/common/aliases.sh"
source "$REPO_ROOT/common/lazy-completions.sh"
first="$1"
"$first" get first; echo "@@rc1=$?"
k get pods; echo "@@rc2=$?"
k get svc; echo "@@rc3=$?"
kubectl get ns; echo "@@rc4=$?"
echo "@@loaded=${LAZY_LOADED_KUBECTL:-}"
EOF
}

run_probe() {
	local shell="$1" first="$2"
	run env -i HOME="$FAKE_HOME" PATH="$FAKEBIN" TERM=dumb REPO_ROOT="$REPO_ROOT" \
		"$shell" "$PROBE" "$first"
}

assert_k_survives() {
	[ "$status" -eq 0 ] || fail "probe exited $status: $output"
	[[ "$output" == *"real-kubectl get first"* ]] || fail "first call did not reach kubectl: $output"
	[[ "$output" == *"real-kubectl get pods"* ]] || fail "second k call did not reach kubectl: $output"
	[[ "$output" == *"real-kubectl get svc"* ]] || fail "third k call did not reach kubectl: $output"
	[[ "$output" == *"real-kubectl get ns"* ]] || fail "kubectl after k did not reach kubectl: $output"
	local n
	for n in 1 2 3 4; do
		[[ "$output" == *"@@rc$n=0"* ]] || fail "call $n failed: $output"
	done
	[[ "$output" == *"@@loaded=1"* ]] || fail "completions not marked loaded: $output"
	[[ "$output" != *"not found"* ]] || fail "a call was not found: $output"
}

@test "bash: k keeps working after its first use" {
	run_probe "$TEST_BASH" k
	assert_k_survives
}

@test "bash: k keeps working after kubectl loads completions first" {
	run_probe "$TEST_BASH" kubectl
	assert_k_survives
}

@test "zsh: k keeps working after its first use" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	run_probe "$TEST_ZSH" k
	assert_k_survives
}

@test "zsh: k keeps working after kubectl loads completions first" {
	[[ -n "$TEST_ZSH" ]] || skip "zsh not installed"
	run_probe "$TEST_ZSH" kubectl
	assert_k_survives
}
