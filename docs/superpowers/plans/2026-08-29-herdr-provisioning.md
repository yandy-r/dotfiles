# Herdr Provisioning Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install checksum-pinned Herdr on every supported Linux/WSL and macOS target and manage a collision-free, tmux-compatible Herdr configuration.

**Architecture:** Two OS-gated `github-release` manifest entries install one `herdr` binary without exposing the package on native Windows. Operators run `dotf install --intent dev`, then `dotf link --backup` so any existing `~/.config/herdr/config.toml` is preserved before the single `FacetUnix` symlink-table entry manages it, leaving Herdr's runtime logs outside the repository; focused manifest and symlink contract tests pin the supported platform matrix.

**Tech Stack:** Go 1.26, TOML, the native `dotf` manifest/GitHub Release/symlink engines, YAML for the historical rollback mirror, Markdown, and Herdr 0.8.2.

**Spec:** `docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md`

## Global Constraints

- Supported targets are Linux/WSL and macOS on x86_64 and ARM64; native Windows is out of scope and must remain unaffected.
- Pin Herdr to `v0.8.2` from `herdrdev/herdr`; upgrades require an explicit tag and checksum review.
- Use only the native `github-release` backend. Do not add a custom installer or change `customScriptBaseline`.
- Use checksum keys with canonical architecture names: `linux/x86_64`, `linux/arm64`, `darwin/x86_64`, and `darwin/arm64`.
- Manage only `herdr/config.toml` → `~/.config/herdr/config.toml`; never link the whole Herdr directory.
- Preserve `Ctrl+B` and the approved tmux-compatible bindings. Rebind Herdr Settings, notification targeting, and sidebar actions where tmux bindings displace defaults.
- Add only the dedicated Herdr manifest invariant and updates to existing symlink contract tests.
- Do not modify `manifest/dotf.lock`: inline per-target checksums are authoritative, missing package lock entries deliberately fall back to the manifest, and the existing host-only `dotf lock --check` already reports drift against the committed multi-target lock.
- In worktree verification, set `DOTFILES_ROOT="$PWD"` so the feature binary cannot resolve the main checkout.
- Do not run the historical Ansible symlink playbook against a real home. The native `dotf link` path is authoritative; an isolated temporary-home run with explicit `dotfiles_dir` is allowed only for rollback behavior verification.

---

## File Map

- `manifest/packages.toml`: Linux and macOS Herdr providers, checksums, and `dev` intent membership.
- `internal/manifest/manifest_test.go`: exact release metadata and applicability matrix.
- `herdr/config.toml`: portable theme, layout, and keybinding contract.
- `internal/symlink/table.go`: reusable `FacetUnix` gate and active single-file link.
- `internal/symlink/table_test.go`: facet, count, backup, and golden-table invariants.
- `internal/symlink/symlink_test.go`: per-OS creation matrix, including SUSE, WSL, generic Linux, and the Windows exclusion.
- `playbooks/symlinks.yml`: historical rollback parent-directory creation.
- `playbooks/vars/symlinks.yml`: macOS/Linux historical rollback mirror.
- `playbooks/tasks/backup.yml`: focused historical rollback backup coverage for the Herdr file mapping.
- `README.md`: user-facing provisioning and managed-config documentation.

---

### Task 1: Add the Pinned Cross-Platform Herdr Packages

**Files:**

- Modify: `internal/manifest/manifest_test.go`
- Modify: `manifest/packages.toml`

**Interfaces:**

- Consumes: existing `manifest.Package`, `backend.Engine.Applicable`, `platform.Info`, and `dev` intent contracts.
- Produces: OS-gated packages `herdr` and `herdr-macos`, each installing/probing the `herdr` binary at release `v0.8.2`.

- [ ] **Step 1: Write the failing embedded-manifest test**

Add this focused invariant after `TestEmbeddedManifest_CoreToolsetProvisioned` in `internal/manifest/manifest_test.go`:

```go
func TestEmbeddedManifest_HerdrRelease(t *testing.T) {
 m := loadEmbeddedManifest(t)
 dev, ok := m.Intents["dev"]
 if !ok {
  t.Fatal(`Intents["dev"] missing from embedded manifest`)
 }

 names := []string{"herdr", "herdr-macos"}
 byName := make(map[string]manifest.Package, len(m.Packages))
 for _, pkg := range m.Packages {
  byName[pkg.Name] = pkg
 }

 want := map[string]struct {
  os        string
  checksums map[string]string
 }{
  "herdr": {
   os: "linux",
   checksums: map[string]string{
    "linux/x86_64": "976150a14d490c94b243ea2e1a7eb2dfb67f12e36b182db90936f6728e6aecf4",
    "linux/arm64":  "f55610658e1c2e0d2aaef730b4b2ab885f7f8ba00285ab372bfb14f2e3d5b40d",
   },
  },
  "herdr-macos": {
   os: "macos",
   checksums: map[string]string{
    "darwin/x86_64": "ab50262c8190cd7aa9056d249d255c08c328c3e8716de9cfa29db4f131b8e2c1",
    "darwin/arm64":  "a5d4f4d504d8b309c91f811050559300faba31258425f53c50852fc96f6ae574",
   },
  },
 }

 for _, name := range names {
  if !slices.Contains(dev.Packages, name) {
   t.Errorf("dev intent does not include %q", name)
  }
  pkg, ok := byName[name]
  if !ok {
   t.Fatalf("embedded manifest has no [[package]] named %q", name)
  }
  if pkg.Backend != manifest.BackendGitHubRelease {
   t.Errorf("%s backend = %q, want github-release", name, pkg.Backend)
  }
  if pkg.Repo != "herdrdev/herdr" || pkg.Tag != "v0.8.2" || pkg.Binary != "herdr" {
   t.Errorf("%s release = repo %q tag %q binary %q", name, pkg.Repo, pkg.Tag, pkg.Binary)
  }
  if pkg.OS != want[name].os {
   t.Errorf("%s os = %q, want %q", name, pkg.OS, want[name].os)
  }
  if len(pkg.Checksums) != len(want[name].checksums) {
   t.Errorf("%s checksums = %v, want exactly %v", name, pkg.Checksums, want[name].checksums)
  }
  for target, sum := range want[name].checksums {
   if pkg.Checksums[target] != sum {
    t.Errorf("%s checksum[%q] = %q, want %q", name, target, pkg.Checksums[target], sum)
   }
  }
  if got := pkg.EffectiveClass(); got != manifest.ClassPinned {
   t.Errorf("%s effective class = %q, want pinned", name, got)
  }
 }

 engine := backend.Default(exec.NewFake(), privilege.NewSudo(), privilege.NewFakeDropper(), t.TempDir(), nil)
 targets := []struct {
  name string
  info platform.Info
  want []string
 }{
  {"debian-x86_64", platform.Info{OS: platform.OSDebian, Arch: platform.ArchX86_64}, []string{"herdr"}},
  {"redhat-arm64", platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM64}, []string{"herdr"}},
  {"arch-x86_64", platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}, []string{"herdr"}},
  {"suse-x86_64", platform.Info{OS: platform.OSSUSE, Arch: platform.ArchX86_64}, []string{"herdr"}},
  {"generic-linux-arm64", platform.Info{OS: platform.OSLinux, Arch: platform.ArchARM64}, []string{"herdr"}},
  {"wsl-x86_64", platform.Info{OS: platform.OSWSL, Arch: platform.ArchX86_64}, []string{"herdr"}},
  {"darwin-x86_64", platform.Info{OS: platform.OSMacOS, Arch: platform.ArchX86_64}, []string{"herdr-macos"}},
  {"darwin-arm64", platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64}, []string{"herdr-macos"}},
  {"windows-excluded", platform.Info{OS: platform.OSWindows, Arch: platform.ArchX86_64}, nil},
 }
 for _, target := range targets {
  t.Run(target.name, func(t *testing.T) {
   var got []string
   for _, name := range names {
    applicable, err := engine.Applicable(byName[name], target.info)
    if err != nil {
     t.Fatalf("Applicable(%q) error = %v", name, err)
    }
    if applicable {
     got = append(got, name)
    }
   }
   if !slices.Equal(got, target.want) {
    t.Errorf("applicable Herdr packages = %v, want %v", got, target.want)
   }
  })
 }
}
```

- [ ] **Step 2: Run the test and verify it fails for the absent providers**

Run:

```bash
go test ./internal/manifest -run TestEmbeddedManifest_HerdrRelease -count=1
```

Expected: FAIL because the `dev` intent and embedded package map contain neither `herdr` nor `herdr-macos`.

- [ ] **Step 3: Add the manifest entries**

Add both names immediately after `tmux` in `[intents.dev].packages`, then add these packages after the existing tmux package:

```toml
[[package]]
name = "herdr"
backend = "github-release"
os = "linux"
repo = "herdrdev/herdr"
tag = "v0.8.2"
binary = "herdr"
checksums = { "linux/arm64" = "f55610658e1c2e0d2aaef730b4b2ab885f7f8ba00285ab372bfb14f2e3d5b40d", "linux/x86_64" = "976150a14d490c94b243ea2e1a7eb2dfb67f12e36b182db90936f6728e6aecf4" }

[[package]]
name = "herdr-macos"
backend = "github-release"
os = "macos"
repo = "herdrdev/herdr"
tag = "v0.8.2"
binary = "herdr"
checksums = { "darwin/arm64" = "a5d4f4d504d8b309c91f811050559300faba31258425f53c50852fc96f6ae574", "darwin/x86_64" = "ab50262c8190cd7aa9056d249d255c08c328c3e8716de9cfa29db4f131b8e2c1" }
```

Precede the entries with a short comment explaining that the sibling split is the existing one-token OS-gate pattern and prevents native Windows applicability.

- [ ] **Step 4: Format and run the focused manifest suite**

Run:

```bash
gofmt -w internal/manifest/manifest_test.go
go test ./internal/manifest -run 'TestEmbeddedManifest_(HerdrRelease|CoreToolsetProvisioned)' -count=1
```

Expected: PASS; the Herdr subtests select one provider on each supported target and zero on Windows.

- [ ] **Step 5: Commit the package contract**

```bash
git add manifest/packages.toml internal/manifest/manifest_test.go
git commit -m "feat(provisioning): install Herdr from pinned releases"
```

---

### Task 2: Add the Tmux-Compatible Config and Unix-Only Link

**Files:**

- Create: `herdr/config.toml`
- Modify: `internal/symlink/table.go`
- Modify: `internal/symlink/table_test.go`
- Modify: `internal/symlink/symlink_test.go`
- Modify: `playbooks/symlinks.yml`
- Modify: `playbooks/vars/symlinks.yml`
- Modify: `playbooks/tasks/backup.yml`

**Interfaces:**

- Consumes: `platform.IsLinuxFamily`, `symlink.LinkSpec`, existing dry-run/backup behavior, and the approved Herdr v0.8.2 key names.
- Produces: `FacetUnix`, one backup-eligible `herdr/config.toml` link, and a collision-free active Herdr keymap.

- [ ] **Step 1: Change the symlink expectations first**

Make these exact expectation changes in `internal/symlink/table_test.go`:

```go
// TestTable_Count
if got, want := len(DefaultTable()), 42; got != want {

// TestTable_FacetCounts addition; existing counts stay unchanged
FacetUnix: 1,

// TestTable_BackupEligibleSet addition
".config/herdr/config.toml",

// TestTable_MatchesPlaybook addition after the macOS app entry
{Src: "herdr/config.toml", Dest: ".config/herdr/config.toml", Facet: FacetUnix, BackupEligible: true},
```

Add these rows to `TestApplies`:

```go
{"unix-on-macos", FacetUnix, platform.OSMacOS, false, true},
{"unix-on-debian", FacetUnix, platform.OSDebian, false, true},
{"unix-on-generic-linux", FacetUnix, platform.OSLinux, false, true},
{"unix-on-wsl", FacetUnix, platform.OSWSL, false, true},
{"unix-on-windows", FacetUnix, platform.OSWindows, false, false},
```

Update `TestPlan_IdenticalSet_PerOS` counts and required destinations as follows:

```go
// Existing rows: add ".config/herdr/config.toml" to mustHave.
// macos 30; arch-no-desktop 30; arch-desktop 41;
// debian-no-desktop 30; debian-desktop 40;
// generic-linux 29; redhat-no-desktop 30.

{
 name: "suse-no-desktop", os: platform.OSSUSE, wantCreated: 30,
 mustHave: []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
 mustNotHave: []string{".config/aerospace", ".config/hypr"},
},
{
 name: "wsl", os: platform.OSWSL, wantCreated: 30,
 mustHave: []string{".config/herdr/config.toml", ".config/systemd/user/tmux.service"},
 mustNotHave: []string{".config/aerospace", ".config/hypr"},
},
{
 name: "windows-excludes-herdr", os: platform.OSWindows, wantCreated: 28,
 mustNotHave: []string{".config/herdr/config.toml", ".config/aerospace", ".config/hypr"},
},
```

- [ ] **Step 2: Run the focused suite and verify the new contract fails**

Run:

```bash
go test ./internal/symlink -run 'Test(Table|Applies|Plan_IdenticalSet_PerOS)' -count=1
```

Expected: compilation fails because `FacetUnix` is undefined. This confirms the expectations precede implementation.

- [ ] **Step 3: Implement `FacetUnix` and the active link**

Add the new facet beside `FacetLinux` in `internal/symlink/table.go`:

```go
// FacetUnix applies on macOS and on every canonical Linux-family host,
// including generic Linux and WSL. It excludes native Windows and unknown OSes.
FacetUnix
```

Add its applicability branch:

```go
case FacetUnix:
 return info.OS == platform.OSMacOS || platform.IsLinuxFamily(info.OS)
```

Add the link after the macOS application entry and before desktop-only entries:

```go
// unix_configs (macOS and all Linux families; single file so runtime logs stay local)
{Src: "herdr/config.toml", Dest: ".config/herdr/config.toml", Facet: FacetUnix, BackupEligible: true},
```

Update the `BackupEligible` comment from 30 to 31 entries and from 14 to 15 single-file entries. Extend the documented `DefaultTable`/playbook divergences to state that one native `FacetUnix` entry is represented in the rollback YAML under both macOS and Linux groups.

- [ ] **Step 4: Add the historical rollback mappings**

Add this file mapping to both `macos_configs` and `linux_configs` in `playbooks/vars/symlinks.yml`:

```yaml
- { src: 'herdr/config.toml', dest: '.config/herdr/config.toml' }
```

This keeps the historical playbook scoped away from `windows_configs`. Extend the
existing backup inventory only with the in-scope managed Herdr mapping so a real
config is copied before unified cleanup; do not broaden rollback backup behavior
for unrelated application or platform mappings.

Add the Herdr parent to the existing `Create required directories` loop in `playbooks/symlinks.yml`:

```yaml
- '{{ config_dir }}/herdr'
```

Run the rollback playbook in an isolated temporary home with `backup=true` and
explicit `dotfiles_dir`. Seed a real `.config/herdr/config.toml`, then assert that
the timestamped backup contains the original bytes and the destination is the
intended repository symlink. Never point this behavior check at the real home.

- [ ] **Step 5: Create the approved Herdr config**

Create `herdr/config.toml` with exactly:

```toml
onboarding = false

[theme]
name = "catppuccin"

[ui]
tab_bar_position = "bottom"

[keys]
prefix = "ctrl+b"
goto = "prefix+g"
new_tab = "prefix+c"
next_tab = "prefix+n"
previous_tab = "prefix+p"
switch_tab = "prefix+1..9"
focus_pane_left = "prefix+h"
focus_pane_down = "prefix+j"
focus_pane_up = "prefix+k"
focus_pane_right = "prefix+l"
split_vertical = ["prefix+v", "prefix+|"]
split_horizontal = ["prefix+s", "prefix+minus"]
close_pane = "prefix+x"
close_tab = "prefix+shift+x"
last_pane = "prefix+o"
zoom = "prefix+z"
copy_mode = "prefix+["
detach = ["prefix+d", "prefix+q"]
reload_config = "prefix+shift+r"
toggle_sidebar = "prefix+shift+b"
open_notification_target = "prefix+shift+o"
settings = "prefix+shift+s"
```

- [ ] **Step 6: Format and run the symlink suite**

Run:

```bash
gofmt -w internal/symlink/table.go internal/symlink/table_test.go internal/symlink/symlink_test.go
go test ./internal/symlink -count=1
```

Expected: PASS, including macOS, Debian, RedHat, Arch, SUSE, generic Linux, WSL, and Windows-exclusion rows.

- [ ] **Step 7: Commit the config and link contract**

```bash
git add herdr/config.toml internal/symlink/table.go internal/symlink/table_test.go internal/symlink/symlink_test.go playbooks/symlinks.yml playbooks/vars/symlinks.yml
git commit -m "feat(herdr): add tmux-compatible configuration"
```

---

### Task 3: Document and Verify the Integrated Feature

**Files:**

- Modify: `README.md`
- Potential generated updates: tracked files under `graphify-out/` only when `graphify update .` changes them

**Interfaces:**

- Consumes: both Herdr providers, `FacetUnix`, `herdr/config.toml`, the worktree-built `dotf`, and locally installed Herdr 0.8.2.
- Produces: user-facing documentation plus local evidence for package selection, symlink scope, config acceptance, and repository quality gates.

- [ ] **Step 1: Update the repository and provisioning summaries**

Add this entry to the repository tree near the other terminal configs:

```text
├── herdr/                # Herdr config (single file linked; runtime logs stay local)
```

Add this bullet under `### 📦 Package Management`:

```markdown
- **Pinned Herdr Runtime**: Install Herdr 0.8.2 from checksum-verified Linux/macOS release assets with `dotf install --intent dev`, then preserve any existing Herdr config while managing the repository config with `dotf link --backup`
```

Replace the Terminal Tools bullet with:

```markdown
- **Terminal Tools**: Herdr with a managed tmux-compatible config, TPM (Tmux Plugin Manager), and tmux-sessionizer
```

- [ ] **Step 2: Run Markdown and diff hygiene checks**

Run:

```bash
feature_base="$(git merge-base origin/main HEAD)"
git -c core.whitespace=blank-at-eol,blank-at-eof,space-before-tab diff --check "${feature_base}..HEAD"
herdr_product_markdown_files=(
  README.md
  docs/superpowers/plans/2026-08-29-herdr-provisioning.md
  docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md
)
npx --no -- markdownlint-cli2 "${herdr_product_markdown_files[@]}"
npx --no -- prettier --check --ignore-path .prettierignore --config .prettierrc \
  docs/superpowers/plans/2026-08-29-herdr-provisioning.md \
  docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md
```

Expected: the deterministic diff check over `${feature_base}..HEAD` is clean and
the fresh range-scoped result is recorded; markdownlint passes the explicit
product-doc gate of README plus this plan and its spec, while committed review
artifacts are validated by the review workflow. Prettier checks the newly
authored plan and spec. The unchanged README's pre-existing Prettier result is
recorded separately and is not an all-green feature gate, without adding
unrelated README table formatting. The full `npm run lint:md` command remains a
known non-green baseline of 444 pre-existing errors and must be recorded rather
than represented as a passing feature gate.

- [ ] **Step 3: Build the feature binary and verify provider selection**

Run:

```bash
DOTFILES_ROOT="$PWD" npm run go:build
DOTFILES_ROOT="$PWD" ./dotf apply --headless --dry-run --intent dev | tee /tmp/dotf-herdr-apply-plan.txt
rg '^plan\\.github-release=herdr$' /tmp/dotf-herdr-apply-plan.txt
if rg '^plan\\.github-release=.*herdr-macos' /tmp/dotf-herdr-apply-plan.txt; then
  echo "unexpected macOS provider on Linux" >&2
  exit 1
fi
```

Expected on the current Arch Linux host: the plan contains `herdr`, omits `herdr-macos`, and performs no installation because of `--dry-run`.

- [ ] **Step 4: Verify the worktree config link visibly**

Run:

```bash
DOTFILES_ROOT="$PWD" ./dotf link --dry-run | tee /tmp/dotf-herdr-link-plan.txt
rg '\.config/herdr/config\.toml' /tmp/dotf-herdr-link-plan.txt
```

Expected: output visibly plans `herdr/config.toml` → `.config/herdr/config.toml` without changing `$HOME`.

- [ ] **Step 5: Load the config with the real pinned Herdr binary**

Run the TUI in an isolated temporary home/socket under a pseudo-terminal, interrupt it after startup, and reject config warnings:

```bash
test "$(herdr --version)" = "herdr 0.8.2"
validation_root=$(mktemp -d)
trap 'rm -rf -- "$validation_root"' EXIT
set +e
HOME="$validation_root" \
XDG_CONFIG_HOME="$validation_root/.config" \
HERDR_CONFIG_PATH="$PWD/herdr/config.toml" \
HERDR_SOCKET_PATH="$validation_root/herdr.sock" \
timeout --signal=INT --kill-after=1s 2s \
  script -qec 'herdr --no-session' "$validation_root/transcript.log"
smoke_code=$?
set -e
if [ "$smoke_code" -ne 124 ] && [ "$smoke_code" -ne 130 ]; then
  echo "unexpected Herdr smoke exit: $smoke_code" >&2
  exit 1
fi
if rg -a -i 'config.*(error|warning)|unknown.*(key|field)|invalid.*(binding|config)' "$validation_root/transcript.log"; then
  echo "Herdr rejected the managed config" >&2
  exit 1
fi
```

Expected: Herdr reaches its TUI until the deliberate timeout, and the transcript has no configuration/keybinding warning.

- [ ] **Step 6: Run the complete repository gates**

Run:

```bash
npm run lint
feature_base="$(git merge-base origin/main HEAD)"
git -c core.whitespace=blank-at-eol,blank-at-eof,space-before-tab diff --check "${feature_base}..HEAD"
herdr_product_markdown_files=(
  README.md
  docs/superpowers/plans/2026-08-29-herdr-provisioning.md
  docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md
)
npx --no -- markdownlint-cli2 "${herdr_product_markdown_files[@]}"
npx --no -- prettier --check --ignore-path .prettierignore --config .prettierrc \
  docs/superpowers/plans/2026-08-29-herdr-provisioning.md \
  docs/superpowers/specs/2026-08-29-herdr-provisioning-design.md
npm run go:build
go build ./...
go vet ./...
go test -race ./...
golangci-lint version
golangci-lint run ./...
```

Expected: every command exits 0, and `golangci-lint version` reports v2.12.2,
matching CI. The deterministic diff over `${feature_base}..HEAD` and explicit
product-doc Markdown gate have the same scope as Step 2; record the fresh
range-scoped diff result, and validate committed review artifacts through the
review workflow. The unchanged README's pre-existing Prettier result is
recorded separately and is not an all-green feature gate.

- [ ] **Step 7: Refresh the knowledge graph and inspect generated changes**

Run:

```bash
graphify update .
git status --short
```

Expected: graph refresh completes. If tracked `graphify-out/` files changed, include only those generated graph updates in the documentation commit; do not stage unrelated files.

- [ ] **Step 8: Commit user-facing documentation and generated graph updates**

```bash
git add README.md
git add -u graphify-out
git commit -m "docs: document Herdr provisioning"
```

- [ ] **Step 9: Confirm the final implementation range is clean**

```bash
git status --short --branch
git log --oneline origin/main..HEAD
```

Expected: clean worktree on `feat/herdr-provisioning`, with design/plan, package, config/link, and documentation commits only.

---

## Delivery Workflow After Implementation

1. Run an independent code review against `origin/main...HEAD`, then fix every actionable finding and rerun the affected focused test plus the complete gates.
2. Push `feat/herdr-provisioning` and open a GitHub PR using `.github/pull_request_template.md`, with `Closes #37`, the New Feature and Documentation boxes checked, exact test evidence, and notes explaining that `npm test` does not exist and the Ansible playbook is rollback-only.
3. Apply existing labels `type:feature`, `area:cli`, and `priority:medium`; do not invent platform labels that are absent from the repository.
4. Run a second independent PR review, apply any review fixes, and resolve all review threads.
5. Monitor GitHub checks with `gh pr checks --watch --interval 10` until every required check is green. Investigate and fix failures rather than rerunning blindly.
6. Squash merge with branch deletion only after review and CI are green: `gh pr merge --squash --delete-branch`.
7. Verify the PR is merged and issue #37 is closed, update the main checkout with `git pull --ff-only`, remove `/home/yandy/.claude-worktrees/dotfiles-feat-herdr`, delete the exact local feature branch, prune stale worktree metadata, and verify the remote feature ref is absent.
8. Perform the requirement-by-requirement completion audit against the merged `main` state before marking the goal complete.
