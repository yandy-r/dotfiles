# Plan: SSH-Server Sysconfig Domain

## Summary

Add an `ssh-server` domain to `internal/sysconfig` so `dotf apply` stands up an OpenSSH **server** on Linux/WSL the way the legacy `bootstrap.sh configure_ssh_server`/`configure_ssh_firewall` did: install `openssh-server`, generate host keys if missing (`ssh-keygen -A`), `systemctl enable --now` the correct service unit (`ssh` vs `sshd`), and open the SSH port via the existing firewall backend. The domain mirrors the existing per-OS Configurer/Plan/Execute pattern (the `firewall` domain is the closest analog), is idempotent/plan-first (a converged host yields an empty plan), and is a no-op on macOS. This closes the final provisioning-parity gap of #31 (Phase D) after Phases A–C.

## User Story

As an operator provisioning a fresh headless Linux/WSL host, I want `dotf apply` (and `dotf sysconfig --ssh-server on`) to install, key, enable, and firewall-open the OpenSSH server, so that I can log in remotely over SSH immediately after a headless provision — the same guarantee the legacy `bootstrap.sh` gave — without manual post-install SSH setup.

## Problem → Solution

Today `internal/sysconfig` manages five domains (hostname, timezone, locale, shell, firewall); the firewall domain only _opens_ the SSH port, and no domain installs `openssh-server`, generates host keys, or enables `sshd` — so a headless `dotf apply` opens port 22 to a host with no SSH server listening. → Add an idempotent, plan-first `ssh-server` domain that on Linux/WSL emits install → `ssh-keygen -A` → `systemctl enable --now <unit>` → firewall-open steps (empty plan when converged), is a no-op empty plan on macOS, and is toggled by a new `--ssh-server` flag + `[sysconfig] ssh_server` manifest field + `sysconfig.Config.SSHServer`.

## Metadata

- **Complexity**: Medium (10–12 files incl. tests)
- **Source PRD**: N/A (Forgejo issue #34, child of #31, Phase D)
- **PRD Phase**: N/A
- **Estimated Files**: 11 source/impl + 4 test files
- **Research Dispatch**: Enhanced parallel sub-agents (7 researchers via `Task`)

## Batches

Tasks grouped by dependency for parallel execution. Tasks within the same batch run concurrently; batches run in order.

| Batch | Tasks      | Depends On | Parallel Width |
| ----- | ---------- | ---------- | -------------- |
| B1    | 1.1, 1.2   | —          | 2              |
| B2    | 2.1, 2.2   | B1         | 2              |
| B3    | 3.1        | B2         | 1              |
| B4    | 4.1        | B3         | 1              |
| B5    | 5.1, 5.2   | B4         | 2              |
| B6    | 6.1, 6.2, 6.3 | B5      | 3              |

- **Total tasks**: 10
- **Total batches**: 6
- **Max parallel width**: 3

---

## UX Design

`dotf` is a CLI, so "UX" is the flag surface, plan/`--dry-run` preview text, and apply result lines.

### Before

- `dotf sysconfig` flags: `--manifest --hostname --timezone --locale --shell --firewall --dry-run --yes` — no ssh flag (`cmd/dotf/sysconfig.go:145-156`).
- Help enumerates five domains: "hostname, timezone, locale, shell, firewall" (`cmd/dotf/helptext.go:198`).
- `dotf sysconfig --dry-run` prints `dry-run=true`, non-empty config echoes, `step.<domain>=<argv>`, then `skipped=` (`cmd/dotf/sysconfig.go:270-299`).
- SSH is only a firewall side effect (`ufw allow OpenSSH` / firewalld `--add-service=ssh`); no install/keygen/enable of sshd anywhere.

### After

- New `--ssh-server on|off` flag on `dotf sysconfig` (StringVar; completion auto-harvests `on|off` from the usage string).
- New `[sysconfig] ssh_server` manifest field; `sysconfig.Config.SSHServer` toggle; precedence flag > manifest > skip.
- `dotf sysconfig --ssh-server on --dry-run` on Linux emits `step.ssh-server=sudo apt-get install -y openssh-server`, `step.ssh-server=sudo ssh-keygen -A`, `step.ssh-server=sudo systemctl enable --now ssh`, and the SSH firewall-open step.
- Converged host: ssh-server emits no step → `sysconfig=converged`; the domain appears in `skipped=`.
- `dotf apply` gains a `result.ssh-server=ok|failed` line; `dotf apply --dry-run` gains `sysconfig.step.ssh-server=...` lines.
- macOS: `--ssh-server on` yields an empty plan — `ssh-server` appended to `Skipped` (silent, like `locale`).

### Interaction Changes

| Touchpoint | Before | After | Notes |
| ---------- | ------ | ----- | ----- |
| `--ssh-server` flag | absent | `fs.StringVar(&o.sshServer, "ssh-server", "", "install & enable the OpenSSH server: on\|off")` | mirrors `--firewall`; completion auto-generates |
| `[sysconfig] ssh_server` | absent | new string field, static enum validation | mirrors `firewall` field |
| sysconfig/apply help | "…shell, firewall" | add "ssh server" | `helptext.go` summary + long |
| `sysconfig --dry-run` | no ssh step | `step.ssh-server=<sudo argv>` (install/keygen/enable/firewall) | rendered by existing `sysconfigStepValue` |
| `apply` result / summary | 5 domains | adds `result.ssh-server=ok\|failed`, counted in summary | recorded per-domain via `coll.Record("sysconfig:"+domain,…)` |
| macOS | no ssh path | `ssh-server` in `skipped=`, empty plan | no per-domain "unsupported" line exists today (unchanged) |

---

## Mandatory Reading

Files that MUST be read before implementing:

| Priority | File | Lines | Why |
| -------- | ---- | ----- | --- |
| P0 (critical) | `internal/sysconfig/linux.go` | 240-387 | `planFirewall`/`planUfw`/`planFirewalld`/`ufwInstallArgv`/`ufwHasSSHRule`/`FirewallPlanNeedsSudo` — the EXACT pattern to mirror for install/enable/firewall-open + idempotency probes |
| P0 (critical) | `internal/sysconfig/sysconfig.go` | 30-142 | `Config`, `FirewallMode` enum + `validFirewallModes` + `ValidFirewallMode`, `Step`/`Plan` shapes to mirror for `SSHServerMode` |
| P0 (critical) | `cmd/dotf/sysconfig.go` | 40-197 | flag registration, `resolveSysconfig`, fail-fast validate gate (exit 2), sudo-before-plan gate, dry-run printer |
| P0 (critical) | `cmd/dotf/applyphases.go` | 108-243 | `applyIdentity`/`applyFirewall`/`identityConfig`/`mergeSysconfigResults` — the second-pass pattern for `applySSHServer` |
| P0 (critical) | `internal/sysconfig/linux_test.go` | 30-153 | `assertReadOnlyPlanCalls`, full-plan happy path, empty-plan idempotency — the test skeleton to mirror |
| P1 (important) | `cmd/dotf/apply.go` | 90-94, 176-179, 217-235 | `fwMode` resolve+validate (exit 2), `applyFirewall` call site, `validHomebrew`/`manifestFirewallMode` |
| P1 (important) | `cmd/dotf/applyphases.go` | 245-318 | `applyDryRun`/`printApplyDryRun` — where the ssh-server plan block is added |
| P1 (important) | `internal/manifest/types.go` | 36-55 | `Sysconfig` struct — add an `SSHServer` field with toml tag `ssh_server` |
| P1 (important) | `internal/manifest/validate.go` | 140-166 | `validFirewallModes` + `validateSysconfig` — add the ssh-server enum check |
| P1 (important) | `internal/sysconfig/darwin.go` | 28-42 | `Plan` no-op skip pattern (`p.Skipped = append(..., "locale")`) |
| P1 (important) | `internal/exec/fake.go` | 52-155 | `AddCommand`/`AddPath`/`RunErrors`/`CallsSnapshot` for tests |
| P2 (reference) | `internal/sysconfig/register.go` | 14-30 | per-OS Configurer selection is a RUNTIME map (no build tags) — no change needed |
| P2 (reference) | `internal/sysconfig/register_test.go` | 11-80 | empty-`Config` no-op + family-share tests the new toggle must keep green |
| P2 (reference) | `internal/backend/pkg.go` | 124-134 | full per-pkg-mgr install argv mapping to mirror for `openssh-server` names |

## External Documentation

| Topic | Source | Key Takeaway |
| ----- | ------ | ------------ |
| `ssh-keygen -A` | man ssh-keygen | Natively idempotent — generates ONLY missing host keys in `/etc/ssh`; still gate the Step behind `fs.Exists` so a converged host yields an empty plan. Needs root. |
| systemd unit name | Debian/Ubuntu vs Arch/Fedora/SUSE | Unit is `ssh.service` on Debian/Ubuntu (and typical WSL), `sshd.service` on Arch/RedHat/SUSE. Resolve by `platform.Info.OS`. |
| openssh package name | apt/dnf/zypper vs pacman | `openssh-server` on apt/dnf/zypper; `openssh` on pacman (confirmed by `bin/install-arch-pkgs.sh:338`). |
| Ubuntu 22.10+ `ssh.socket` | systemd socket activation | Newer Ubuntu can socket-activate via `ssh.socket`; reconciling `.service` vs `.socket` is **out of scope** (NOT Building). |

---

## Patterns to Mirror

Code patterns discovered in the codebase. Follow these exactly.

### NAMING_CONVENTION

```
// SOURCE: internal/sysconfig/sysconfig.go:59-97
// Mirror FirewallMode exactly as SSHServerMode: "" unset/skip, "off" explicit
// non-destructive skip, "on" enable. Add validSSHServerModes + ValidSSHServerMode.
type FirewallMode string
const ( FirewallUnset FirewallMode = ""; FirewallOff FirewallMode = "off"; ... )
var validFirewallModes = map[FirewallMode]bool{ FirewallUnset: true, ... }
func ValidFirewallMode(mode FirewallMode) bool { return validFirewallModes[mode] }
```

### SERVICE_PATTERN (install-if-absent, enable, per-distro install argv)

```
// SOURCE: internal/sysconfig/linux.go:272-305 (planUfw) + 371-377 (ufwInstallArgv)
_, lookErr := c.r.LookPath("ufw"); installed := lookErr == nil
if !installed { p.Steps = append(p.Steps, Step{Domain:"firewall", Desc:"install ufw", Argv: ufwInstallArgv(info)}) }
// ...
func ufwInstallArgv(info platform.Info) []string {
    if info.OS == platform.OSArch { return []string{"sudo","pacman","-S","--noconfirm","--needed","ufw"} }
    return []string{"sudo","apt-get","install","-y","ufw"} }
```

### ENABLE_SERVICE (the systemctl enable --now pattern — mirror for sshd/ssh)

```
// SOURCE: internal/sysconfig/linux.go:347-353 (firewalld enable, state-gated)
if out, err := c.r.Output("firewall-cmd","--state"); err != nil || !strings.Contains(string(out),"running") {
    p.Steps = append(p.Steps, Step{Domain:"firewall", Desc:"firewalld enable",
        Argv: []string{"sudo","systemctl","enable","--now","firewalld"}}) }
// For ssh-server: probe `systemctl is-active <unit>` / `is-enabled <unit>` (unprivileged), emit enable --now if not active.
```

### FIREWALL_OPEN_REUSE (do NOT hand-roll — reuse these SSH-allow steps + probes)

```
// SOURCE: internal/sysconfig/linux.go:291-297 (ufw) + 355-368 (firewalld)
if !ufwHasSSHRule(status) { p.Steps = append(p.Steps, Step{Domain:"...", Desc:"ufw allow OpenSSH",
    Argv: []string{"sudo","ufw","allow","OpenSSH"}}) }
if _, err := c.r.Output("firewall-cmd","--permanent","--query-service=ssh"); err != nil {
    p.Steps = append(p.Steps, Step{..., Argv: []string{"sudo","firewall-cmd","--permanent","--add-service=ssh"}},
                                Step{..., Argv: []string{"sudo","firewall-cmd","--reload"}}) }
```

### FILE_EXISTS_GUARD (host-key idempotency — gate ssh-keygen -A)

```
// SOURCE: internal/sysconfig/linux.go:89 (fs.Exists gate) + internal/sysconfig/fs.go (Exists seam)
if !c.fs.Exists("/etc/ssh/ssh_host_ed25519_key") {
    p.Steps = append(p.Steps, Step{Domain:"ssh-server", Desc:"generate host keys",
        Argv: []string{"sudo","ssh-keygen","-A"}}) }
// Probe sshd presence via c.fs.Exists("/usr/sbin/sshd") (robust vs bare LookPath which misses /usr/sbin).
```

### SUDO_PLAN_GATE (a privileged read-path probe needs sudo primed before Plan)

```
// SOURCE: internal/sysconfig/linux.go:324-342 (FirewallPlanNeedsSudo) + cmd/dotf/sysconfig.go:72-78
func FirewallPlanNeedsSudo(mode FirewallMode, info platform.Info) bool { ... case FirewallUfw: return true ... }
// Add SSHServerPlanNeedsSudo (true when ssh-server is on AND the ufw SSH-allow probe `sudo -n ufw status` is used).
// In cmd/dotf/sysconfig.go OR the gate to prime sudo before Plan.
```

### APPLY_SECOND_PASS (mirror applyFirewall for applySSHServer)

```
// SOURCE: cmd/dotf/applyphases.go:167-185 (applyFirewall)
func applyFirewall(eng *sysconfig.Engine, mode sysconfig.FirewallMode, info platform.Info, coll *failure.Collector) sysconfig.Result {
    cfg := sysconfig.Config{Firewall: mode, User: realUserName()}
    plan, err := eng.Plan(cfg, info); /* record; execute; record per-step */ }
// identityConfig (applyphases.go:191-200) must force cfg.SSHServer = sysconfig.SSHServerUnset (owned by applySSHServer).
```

### MACOS_NOOP (darwin skip)

```
// SOURCE: internal/sysconfig/darwin.go:35
p.Skipped = append(p.Skipped, "locale")
// For ssh-server: p.Skipped = append(p.Skipped, "ssh-server") in darwinConfigurer.Plan.
```

### TEST_STRUCTURE (table-driven + exec.Fake + read-only Plan guard)

```
// SOURCE: internal/sysconfig/linux_test.go:30-153 + sysconfig_test.go:27-35
fake := exec.NewFake(); fake.AddPath("ufw","/usr/bin/ufw")
fake.AddCommand("sudo", []string{"-n","ufw","status"}, []byte("Status: inactive\n"), nil)
assertStepArgv(t, plan.Steps[0], []string{"sudo","systemctl","enable","--now","firewalld"})
// converged case: assert len(plan.Steps)==0; assertReadOnlyPlanCalls(t, fake.CallsSnapshot())
```

---

## Files to Change

| File | Action | Justification |
| ---- | ------ | ------------- |
| `internal/sysconfig/sysconfig.go` | UPDATE | Add `SSHServerMode` enum + `validSSHServerModes` + `ValidSSHServerMode`; add `SSHServer SSHServerMode` to `Config`; extend `Step.Domain` doc + package doc to list `"ssh-server"`. |
| `internal/sysconfig/linux.go` | UPDATE | Add `planSSHServer` (install-if-absent → keygen-if-missing → enable-if-inactive → firewall-open), `sshServiceName(info)`, `sshInstallArgv(info)`, `SSHServerPlanNeedsSudo`; call `planSSHServer` from `Plan` after `planFirewall`. |
| `internal/sysconfig/darwin.go` | UPDATE | Append `"ssh-server"` to `p.Skipped` in `Plan` (macOS no-op). |
| `internal/manifest/types.go` | UPDATE | Add an `SSHServer` string field (toml tag `ssh_server`) to `Sysconfig`. |
| `internal/manifest/validate.go` | UPDATE | Add `validSSHServerModes` + ssh-server enum check in `validateSysconfig`. |
| `cmd/dotf/sysconfig.go` | UPDATE | Add `sshServer` opt + `--ssh-server on\|off` flag; extend `resolveSysconfig` signature+body; add `ValidSSHServerMode` fail-fast gate (exit 2); OR `SSHServerPlanNeedsSudo` into the sudo-before-Plan gate; emit `ssh-server=` dry-run line. |
| `cmd/dotf/apply.go` | UPDATE | Add `manifestSSHServerMode(m)` + validate gate (exit 2); call `applySSHServer` after `applyFirewall`; fold its Result into the merged sysconfig result. |
| `cmd/dotf/applyphases.go` | UPDATE | Add `applySSHServer` (mirror `applyFirewall`); force `SSHServer=SSHServerUnset` in `identityConfig`; make `mergeSysconfigResults` variadic (or add third arg); add the ssh-server plan block to `applyDryRun`/`printApplyDryRun`. |
| `cmd/dotf/helptext.go` | UPDATE | Add "ssh server" to the sysconfig/apply summary + long text + examples. |
| `CLAUDE.md` | UPDATE | Add `ssh-server` to the sysconfig domain list (the `dotf sysconfig` description near the Symlink/Stack overview). |
| `internal/sysconfig/linux_test.go` | ADD | Table-driven: per-family install argv, host-key gating, `ssh` vs `sshd` unit, converged→empty-plan idempotency, firewall-open reuse, off/unset skip, read-only Plan guard. |
| `internal/sysconfig/darwin_test.go` | ADD | Assert `ssh-server` in `Skipped`, zero steps, no probing calls on macOS. |
| `internal/manifest/validate_test.go` | ADD | Valid (`on`/`off`/empty) + invalid `ssh_server` enum cases; strict `DisallowUnknownFields` decode of `[sysconfig] ssh_server`. |
| `cmd/dotf/sysconfig_test.go` | ADD | `--ssh-server on` → `Config.SSHServer`; flag>manifest>skip precedence; dry-run line present; invalid value → exit 2. |
| `cmd/dotf/apply_test.go` | ADD | `applySSHServer` runs after firewall; `result.ssh-server=` line; `apply --dry-run` emits `sysconfig.step.ssh-server=`. |

## NOT Building

- **Generic "service manager" / package-installer abstraction** — only firewalld and ssh-server call `systemctl enable --now` (2 call sites, below rule-of-three). Inline `planSSHServer` like `planFirewalld`; do NOT add a `ServiceManager`/`enableService()` seam.
- **`sshd_config` hardening** (PermitRootLogin, PasswordAuthentication, Port, ciphers/MACs/KEX) — `sshd_config` is a symlinked dotfile owned by the `ssh` setup module, not sysconfig. This domain ensures installed/keyed/running/reachable ONLY. No `/etc/ssh/sshd_config` read or write.
- **Per-key type/size selection** — use `ssh-keygen -A` only (default types); no `-t`/`-b`, no key rotation/overwrite.
- **Non-systemd init systems** (OpenRC/runit/s6/sysvinit/dinit) — the Linux configurer targets the systemd family; non-systemd hosts fail non-fatally, no init-agnostic path.
- **WSL without systemd** (WSL1, or WSL2 `[boot] systemd=false`) — no `service ssh start` fallback; a failed `systemctl` is collected non-fatally.
- **Windows OpenSSH server** — `windows.go` stays an `ErrNotImplemented` stub; `scripts/enableSSH.ps1` is a pre-Go artifact, not wired.
- **macOS Remote Login automation** (`systemsetup -setremotelogin on`, `launchctl`) — macOS is a deliberate empty plan (needs Full Disk Access / GUI approval).
- **`ssh.socket` reconciliation and non-standard Port/ListenAddress** — the firewall backend opens the standard SSH service only.
- **Suppressing the harmless SSH-allow overlap** — the `firewall` domain also opens SSH; both emitting `ufw allow OpenSSH` is idempotent, so accept the overlap rather than coupling the two domains.

---

## Step-by-Step Tasks

### Task 1.1: `SSHServerMode` enum + `Config.SSHServer` — Depends on [none]

- **BATCH**: B1
- **ACTION**: In `internal/sysconfig/sysconfig.go`, add the `SSHServerMode` string type mirroring `FirewallMode`, add the `SSHServer` field to `Config`, and update the domain doc list.
- **IMPLEMENT**: Add consts `SSHServerUnset SSHServerMode = ""`, `SSHServerOff = "off"`, `SSHServerOn = "on"`; add `var validSSHServerModes = map[SSHServerMode]bool{...}` and `func ValidSSHServerMode(mode SSHServerMode) bool`. Add `SSHServer SSHServerMode` to `Config` with a doc comment matching `Firewall`'s (unset = unmanaged, additive-only). Add `"ssh-server"` to the `Step.Domain` doc comment (line ~101-103) and the package-doc domain list.
- **MIRROR**: NAMING_CONVENTION (`sysconfig.go:59-97`).
- **GOTCHA**: `off` is an explicit skip — it must NEVER emit a disable/stop step (sysconfig is additive-only).
- **VALIDATE**: `go build ./internal/sysconfig/`.

### Task 1.2: manifest `ssh_server` field + validation — Depends on [none]

- **BATCH**: B1
- **ACTION**: In `internal/manifest/types.go` add the field; in `internal/manifest/validate.go` add the enum check.
- **IMPLEMENT**: Add an `SSHServer` string field (toml tag `ssh_server`) to `Sysconfig` (with doc comment matching `Firewall`). Add `var validSSHServerModes = map[string]bool{"": true, "on": true, "off": true}` and, in `validateSysconfig`, append a `ValidationError` when `!validSSHServerModes[s.SSHServer]` (message: `sysconfig ssh_server %q must be one of on, off`).
- **MIRROR**: `validate.go:140-166` (`validFirewallModes` + `validateSysconfig`).
- **GOTCHA**: The manifest decoder uses `DisallowUnknownFields`; the toml tag `ssh_server` must match exactly or valid manifests break.
- **VALIDATE**: `go build ./internal/manifest/`.

### Task 2.1: Linux `planSSHServer` + helpers — Depends on [1.1]

- **BATCH**: B2
- **ACTION**: In `internal/sysconfig/linux.go`, add `planSSHServer`, `sshServiceName`, `sshInstallArgv`, `SSHServerPlanNeedsSudo`; call `planSSHServer` from `Plan` immediately after `planFirewall`.
- **IMPLEMENT**: `planSSHServer(cfg, info, p)` returns early appending `"ssh-server"` to `p.Skipped` when `cfg.SSHServer` is `SSHServerUnset` or `SSHServerOff`. Otherwise emit, in order: (1) install step via `sshInstallArgv(info)` when `!c.fs.Exists("/usr/sbin/sshd")`; (2) `sudo ssh-keygen -A` when `!c.fs.Exists("/etc/ssh/ssh_host_ed25519_key")`; (3) `sudo systemctl enable --now <sshServiceName(info)>` when `systemctl is-active <unit>` output != `active`; (4) the SSH firewall-open step reusing the ufw/firewalld helpers (detect backend like `planFirewall`'s `auto`: firewalld on RedHat/SUSE, else ufw), guarded by `ufwHasSSHRule`/`--query-service=ssh`. `sshServiceName`: `case OSDebian, OSWSL: return "ssh"; default: return "sshd"`. `sshInstallArgv`: pacman→`openssh` (Arch), dnf→`openssh-server` (RedHat), zypper→`openssh-server` (SUSE), else apt-get→`openssh-server`. `SSHServerPlanNeedsSudo(mode, info)`: true when `mode == SSHServerOn` and the resolved firewall backend is ufw (its probe needs `sudo -n ufw status`).
- **MIRROR**: SERVICE_PATTERN, ENABLE_SERVICE, FIREWALL_OPEN_REUSE, FILE_EXISTS_GUARD, SUDO_PLAN_GATE (all in `linux.go:240-387`).
- **IMPORTS**: none new (`platform`, `strings` already imported).
- **GOTCHA**: Gate `ssh-keygen -A` behind `fs.Exists` even though it is self-idempotent — an always-emitted keygen step breaks the converged→empty-plan contract. Reuse the existing firewall SSH-allow helpers; do NOT duplicate the argv. Keep every command a sudo-prefixed `[]string` through the Runner (no `sh -c`).
- **VALIDATE**: `go build ./internal/sysconfig/ && go vet ./internal/sysconfig/`.

### Task 2.2: macOS no-op skip — Depends on [1.1]

- **BATCH**: B2
- **ACTION**: In `internal/sysconfig/darwin.go`, append `"ssh-server"` to `p.Skipped` in `Plan`.
- **IMPLEMENT**: Add `p.Skipped = append(p.Skipped, "ssh-server")` alongside the existing `locale` skip so macOS emits zero ssh-server steps regardless of the toggle.
- **MIRROR**: MACOS_NOOP (`darwin.go:35`).
- **GOTCHA**: Do not branch on `cfg.SSHServer` — macOS is unconditionally a no-op (Remote Login is a manual GUI toggle).
- **VALIDATE**: `go build ./internal/sysconfig/`.

### Task 3.1: `dotf sysconfig` command wiring — Depends on [2.1]

- **BATCH**: B3
- **ACTION**: In `cmd/dotf/sysconfig.go`, register the flag, extend `resolveSysconfig`, add the validate gate, extend the sudo gate, and add the dry-run line.
- **IMPLEMENT**: Add `sshServer string` to `sysconfigOpts`; register `fs.StringVar(&o.sshServer, "ssh-server", "", "install & enable the OpenSSH server: on|off")`. Extend `resolveSysconfig` with a `flagSSHServer` param and set `SSHServer: sysconfig.SSHServerMode(firstNonEmpty(flagSSHServer, mSSHServer))` (read `m.SSHServer`). After the `ValidFirewallMode` gate, add a `ValidSSHServerMode(cfg.SSHServer)` gate returning 2 with a `want one of on, off` message. Change the sudo gate to `if !o.dryRun && (sysconfig.FirewallPlanNeedsSudo(cfg.Firewall, info) || sysconfig.SSHServerPlanNeedsSudo(cfg.SSHServer, info))`. In `printSysconfigDryRun`, echo `ssh-server=<value>` when non-empty (the per-step `step.ssh-server=` lines render automatically).
- **MIRROR**: `cmd/dotf/sysconfig.go:52-78, 145-175, 270-299`.
- **GOTCHA**: `resolveSysconfig` has one other caller — `identityConfig` in `applyphases.go` (Task 4.1); its call must add the new empty `""` argument (handled in 4.1). Keep `--hostname` flag-only.
- **VALIDATE**: `go build ./cmd/dotf/` (will fail until 4.1 updates the second caller — acceptable within the batch handoff; run after 4.1 for a clean build).

### Task 4.1: `dotf apply` pipeline pass — Depends on [3.1]

- **BATCH**: B4
- **ACTION**: In `cmd/dotf/applyphases.go` add `applySSHServer`, force the toggle off in `identityConfig`, make `mergeSysconfigResults` variadic, and extend the dry-run; in `cmd/dotf/apply.go` add `manifestSSHServerMode` + validate gate and call `applySSHServer` after `applyFirewall`.
- **IMPLEMENT**: Add `applySSHServer(eng, mode sysconfig.SSHServerMode, info, coll)` mirroring `applyFirewall` (plans `sysconfig.Config{SSHServer: mode, User: realUserName()}`, executes, records per-step). In `identityConfig`, add `cfg.SSHServer = sysconfig.SSHServerUnset` and pass the extra `""` to `resolveSysconfig`. Change `mergeSysconfigResults` to variadic `(results ...sysconfig.Result)` and update its two/three call sites. In `runApply`, add `sshMode := manifestSSHServerMode(m)` + a `ValidSSHServerMode` gate (exit 2), call `sshRes := applySSHServer(eng, sshMode, info, &coll)` right after `fwRes := applyFirewall(...)`, and pass all three results to `mergeSysconfigResults`. In `applyDryRun`/`printApplyDryRun`, plan `sysconfig.Config{SSHServer: sshMode, User: realUserName()}` and emit its `sysconfig.step.ssh-server=` lines. Add `manifestSSHServerMode(m)` beside `manifestFirewallMode` in `apply.go`.
- **MIRROR**: APPLY_SECOND_PASS (`applyphases.go:167-243`) + `apply.go:90-94, 176-179, 228-235`.
- **IMPORTS**: none new.
- **GOTCHA**: On the apply path sudo is already primed once for the whole run (`apply.go:138`), so `SSHServerPlanNeedsSudo` priming is only needed on the standalone `dotf sysconfig` path (Task 3.1). `mergeSysconfigResults` must still exclude applied domains from `Skipped` — its existing dedupe logic covers the third result automatically once variadic.
- **VALIDATE**: `go build ./... && go vet ./...`.

### Task 5.1: help text + completion — Depends on [4.1]

- **BATCH**: B5
- **ACTION**: In `cmd/dotf/helptext.go`, add ssh-server to the `sysconfig` (and `apply`) summary/long/examples.
- **IMPLEMENT**: Update the sysconfig summary string to include "ssh server", the long description domain list, and add an example like `dotf sysconfig --ssh-server on --dry-run`.
- **MIRROR**: `helptext.go:196-212`.
- **GOTCHA**: Completion values are harvested from the `on|off` token in the flag usage string — no manual completion edit needed; verify in Task 6.3.
- **VALIDATE**: `go build ./cmd/dotf/ && dotf sysconfig --help` shows `--ssh-server`.

### Task 5.2: docs (CLAUDE.md) — Depends on [4.1]

- **BATCH**: B5
- **ACTION**: Update `CLAUDE.md` so the `dotf sysconfig` domain enumeration includes `ssh-server`.
- **IMPLEMENT**: Find the sentence describing `dotf sysconfig` owning per-OS system config (hostname/timezone/locale/shell/firewall) and add `ssh-server`. Note macOS is a no-op.
- **MIRROR**: existing sysconfig prose in `CLAUDE.md`.
- **GOTCHA**: `AGENTS.md` defers to `CLAUDE.md` — do not duplicate; only edit `CLAUDE.md`.
- **VALIDATE**: `rg -n "ssh-server" CLAUDE.md`.

### Task 6.1: sysconfig domain tests — Depends on [5.1]

- **BATCH**: B6
- **ACTION**: Add table-driven tests in `internal/sysconfig/linux_test.go` and `internal/sysconfig/darwin_test.go`.
- **IMPLEMENT**: Linux: per-family install argv (`openssh`/`openssh-server`), `ssh` vs `sshd` unit per OS, `ssh-keygen -A` emitted only when host key absent, `enable --now` emitted only when not active, firewall-open step present, converged host → `len(Steps)==0`, `off`/unset → `"ssh-server"` in `Skipped`, and `assertReadOnlyPlanCalls` after Plan. macOS: `ssh-server` in `Skipped`, zero steps, no probing calls.
- **MIRROR**: TEST_STRUCTURE (`linux_test.go:30-153`, `darwin_test.go:191-214`).
- **GOTCHA**: Seed `exec.Fake` reads (`AddPath`/`AddCommand`) so the converged case reports installed+active+rule-present; the ufw SSH-allow probe expects `sudo -n ufw status`.
- **VALIDATE**: `go test -race ./internal/sysconfig/`.

### Task 6.2: manifest validation tests — Depends on [5.1]

- **BATCH**: B6
- **ACTION**: Add cases to `internal/manifest/validate_test.go`.
- **IMPLEMENT**: Valid `ssh_server` values (`on`, `off`, empty) pass; an invalid value yields a `*ValidationError`; a `[sysconfig] ssh_server = "on"` manifest decodes under `DisallowUnknownFields`.
- **MIRROR**: existing firewall validation test cases.
- **VALIDATE**: `go test -race ./internal/manifest/`.

### Task 6.3: command-layer tests — Depends on [5.1]

- **BATCH**: B6
- **ACTION**: Add cases to `cmd/dotf/sysconfig_test.go` and `cmd/dotf/apply_test.go`.
- **IMPLEMENT**: `--ssh-server on` maps to `Config.SSHServer`; flag > manifest > skip precedence; `--ssh-server bogus` → exit 2; `--dry-run` emits the `ssh-server` step lines and mutates nothing; apply runs `applySSHServer` after firewall and prints `result.ssh-server=`; `apply --dry-run` emits `sysconfig.step.ssh-server=`; completion offers `on|off`.
- **MIRROR**: `sysconfig_test.go:84-121, 297-313`; existing firewall command tests.
- **VALIDATE**: `go test -race ./cmd/dotf/`.

---

## Testing Strategy

### Unit Tests

| Test | Input | Expected Output | Edge Case? |
| ---- | ----- | --------------- | ---------- |
| install argv per family | `OSArch` + sshd absent | step `sudo pacman -S --noconfirm --needed openssh` | Yes |
| install argv per family | `OSDebian` + sshd absent | step `sudo apt-get install -y openssh-server` | Yes |
| service unit name | `OSDebian`/`OSWSL` | `systemctl enable --now ssh` | Yes |
| service unit name | `OSArch`/`OSRedHat`/`OSSUSE` | `systemctl enable --now sshd` | Yes |
| host-key gating | host key present | NO `ssh-keygen -A` step | Yes |
| host-key gating | host key missing | `sudo ssh-keygen -A` step | No |
| converged host | sshd installed, keys present, unit active, rule present | `len(Steps)==0`, `ssh-server` in `Skipped` | Yes |
| off/unset | `SSHServer=off` or `""` | `ssh-server` in `Skipped`, zero steps | Yes |
| macOS no-op | `OSMacOS`, `SSHServer=on` | `ssh-server` in `Skipped`, zero steps, no probe calls | Yes |
| read-only Plan | any on-host | Plan issues only whitelisted reads (no mutation recorded) | Yes |
| manifest validation | `ssh_server="bogus"` | `*ValidationError` | Yes |
| flag precedence | `--ssh-server on` + manifest `off` | `Config.SSHServer == on` | No |
| invalid flag | `--ssh-server bogus` | exit 2 | Yes |

### Edge Cases Checklist

- [ ] Empty input (`SSHServer=""`) → domain unmanaged/skipped
- [ ] Converged host → empty plan (idempotent)
- [ ] Invalid enum (`--ssh-server bogus`, `ssh_server="bogus"`) → exit 2 / ValidationError
- [ ] macOS → no-op empty plan
- [ ] Host keys already present → no keygen step (no overwrite)
- [ ] `--dry-run` → zero mutations, no sudo prompt
- [ ] Permission denied (ufw probe unprimed) → fails closed, does not fabricate steps

---

## Validation Commands

### Static Analysis

```bash
go build ./... && go vet ./...
```

EXPECT: Zero build/vet errors

### Unit Tests

```bash
go test -race ./internal/sysconfig/ ./internal/manifest/ ./cmd/dotf/
```

EXPECT: All tests pass

### Full Test Suite + Lint

```bash
go build ./... && go vet ./... && go test -race ./... && golangci-lint run ./...
bin/style.sh --go
```

EXPECT: No regressions, lint clean

### Manual Validation

- [ ] `dotf sysconfig --ssh-server on --dry-run` prints `step.ssh-server=` install/keygen/enable/firewall lines and mutates nothing
- [ ] `dotf apply --dry-run` includes `sysconfig.step.ssh-server=` lines
- [ ] `dotf sysconfig --ssh-server bogus` exits 2 with a clear message
- [ ] On a converged host, `dotf sysconfig --ssh-server on` prints `sysconfig=converged`

---

## Acceptance Criteria

- [ ] Linux plan emits install + `ssh-keygen -A` + `systemctl enable --now <unit>` + firewall-open steps when not converged (issue #34)
- [ ] Plan is idempotent — a converged host (sshd installed, keys present, unit active, firewall rule present) yields an empty plan (issue #34)
- [ ] macOS is a no-op — `ssh-server` in `Skipped`, zero steps (issue #34)
- [ ] `--ssh-server` flag and `[sysconfig] ssh_server` manifest field resolve (flag > manifest > skip) (issue #34)
- [ ] `ssh-keygen -A` step is emitted ONLY when a host key is missing; no existing host key is ever regenerated/overwritten
- [ ] Every command is a sudo-prefixed `[]string` argv through `exec.Runner` — no `sh -c`
- [ ] `sshd_config` is never read or written (scope guard)
- [ ] SSH firewall-open reuses the existing ufw/firewalld helpers (no hand-rolled port rules)
- [ ] Systemd unit resolves per OS family (`ssh` on Debian/WSL, `sshd` elsewhere)
- [ ] All validation commands pass; `go test -race ./...` + `golangci-lint` clean
- [ ] No type/lint errors; `bin/style.sh --go` clean

## Completion Checklist

- [ ] Code follows the firewall-domain patterns (Configurer/Plan/Execute, argv steps)
- [ ] Error handling wraps with `%w` and the `sysconfig:` prefix; per-step failures are non-fatal
- [ ] `Step.Domain` doc + package doc list `"ssh-server"`
- [ ] `CLAUDE.md` sysconfig domain list updated (`AGENTS.md` left deferring to it)
- [ ] `dotf sysconfig`/`apply` help lists ssh-server; completion offers `on|off`
- [ ] `mergeSysconfigResults` handles the third (ssh-server) result without double-listing skips
- [ ] `register_test.go` empty-`Config` no-op stays green (default `SSHServer=""` skips)
- [ ] No unnecessary scope additions (no service-manager abstraction, no sshd_config edits)
- [ ] Self-contained — no questions needed during implementation

## Risks

| Risk | Likelihood | Impact | Mitigation |
| ---- | ---------- | ------ | ---------- |
| Enabling network SSH with distro-default `sshd_config` (password auth, root login policy) exposes the host to brute-force | High | High | In-scope work is install/enable/firewall only; document the exposure in help/README and file a follow-up hardening issue. Do NOT silently ship as "hardened". |
| Regenerating existing host keys breaks client `known_hosts` trust | Low–Med | High | Gate `ssh-keygen -A` behind `fs.Exists`; never pass `-f`/overwrite |
| Non-empty plan on a converged host (unconditional keygen/enable) breaks the plan-first contract | Medium | High | Probe first (sshd present? keys present? unit active? rule present?) and append only missing steps; test the converged→empty case |
| Wrong systemd unit per distro (`ssh` vs `sshd`) → port open but service down | Medium | High | Resolve unit by `info.OS`; table-driven per-family test |
| ufw SSH-allow probe (`sudo -n ufw status`) prompts/hangs on the read-only Plan path | Low | Medium | Reuse `ufwStatus` fail-closed-to-unknown; wire `SSHServerPlanNeedsSudo` so the standalone command primes sudo before Plan |
| `sshd` on `/usr/sbin` missed by a bare `LookPath` → redundant install each run | Medium | Medium | Probe `fs.Exists("/usr/sbin/sshd")` rather than `LookPath` |
| WSL/no-systemd: `systemctl enable` errors | Medium | Low | Non-fatal via the failure collector; documented as NOT Building |
| `resolveSysconfig` signature change misses a caller | Low | Medium | Only two callers (`runSysconfig`, `identityConfig`); both updated in Tasks 3.1/4.1; `go build ./...` catches a miss |

## Notes

- **Config toggle shape — decisive**: mirror `FirewallMode` with an `SSHServerMode` enum (`""` unset/skip, `on`, `off`). This reuses the entire firewall command machinery — `firstNonEmpty` flag>manifest resolution, the exit-2 validate gate, static manifest validation, dry-run key=value rendering, and completion harvesting from the `on|off` usage token. A plain `bool` was rejected because a `--ssh-server` BoolVar cannot distinguish "absent" from "=false" for flag>manifest precedence.
- **Firewall port ownership — decisive**: the `ssh-server` domain opens its own SSH port (reusing the ufw/firewalld helpers) so it is correct even when `firewall` is `off`/unset — matching issue #34's explicit "open the SSH port via the existing firewall backend" and the legacy `configure_ssh_server` calling `configure_ssh_firewall` inline. The overlap with the `firewall` domain is idempotent and accepted.
- **Apply ordering — decisive**: `applySSHServer` runs as its own pass AFTER `applyFirewall` (which runs after packages), so the firewall backend binary exists before the SSH-allow step; the identity pass forces `SSHServer=SSHServerUnset` so ssh-server is never double-run.
- Per-OS Configurer selection is a runtime map in `register.go` (no build tags) — the new logic lives in the shared `linuxConfigurer`/`darwinConfigurer`; no new per-OS files.
- `internal/manifest/module.go` `knownSysconfigDomains` allow-list does not currently include ssh-server; it is only relevant if a setup module references the domain — out of scope for #34's acceptance criteria, noted for follow-up.
- Research backstops (7 enhanced researchers) are in `docs/prps/plans/.prp-research/ssh-server-sysconfig-domain/`.
