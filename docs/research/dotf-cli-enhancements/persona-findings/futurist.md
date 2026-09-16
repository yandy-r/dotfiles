# Futurist Findings — Where provisioning/dotfiles tooling is heading (2026–2030+)

Persona: **The Futurist** | Squad: Asymmetric Research | Date: 2026-07-04
Scope: emerging directions worth `dotf` betting on vs deliberately ignoring, grounded
in the actual `dotf` code + release setup.

---

## Executive Summary

The center of gravity in provisioning tooling for 2026–2028 is **"verify what you
install"** — supply-chain provenance (Sigstore/cosign keyless signing, SLSA
attestations, SBOMs, checksum lockfiles) has crossed from aspirational to baseline,
pushed hard by tooling maturity (GitHub-native attestations, GoReleaser one-liners) and
by regulation (EU CRA reporting obligations begin **Sept 2026**; SBOM mandate lands
**Dec 2027**). `dotf`'s architecture is already pointed the right way — it pins
`sha256` on `github-release` and `script-url` and its bootstrap shim verifies a
`checksums.txt` — but it stops one step short of the emerging norm: **no signing, no
SBOM, no attestation-verify, no lockfile.** These are the safest, highest-leverage
near-term bets and most are near-trivial for a GoReleaser-based Go binary.

The second force is **declarative-reproducible-environment pressure** (Nix/flakes,
home-manager, devenv/devbox/flox, mise, Dev Containers). This is real and growing, but
it does **not** obsolete a lightweight, imperative-friendly, single-host symlink+install
tool — it validates the _middle ground_ `dotf` occupies. The lesson to adopt is
**pinning + lockfiles + reproducible re-apply**; the thing to reject is a Nix rewrite or
full declarative reconciliation.

The third force — **universal binary auto-resolution** (ubi, eget, and mise's `github`
backend) — is the most directly consequential for the `custom-script`→native migration.
The ecosystem is converging on "just point at `owner/repo`, auto-detect the asset." But
critically, mise in 2026 **deprecated its `ubi` backend in favor of `github`** and now
**rejects new `ubi`/`asdf` registry entries for supply-chain reasons**, steering
contributors to `aqua` (checksum-verified) or `github`. That is a strong signal:
auto-resolution is the future _only when paired with checksum/attestation verification_ —
exactly the axis `dotf`'s `sha256`-pinned `github-release` backend should double down on.

AI-assisted config is mostly hype for a solo tool's core loop, but has one genuinely
low-effort, durable win: **structured JSON output** (`--json`) so agents can consume
`plan`/`status`/`diff` — the repo is already saturated with AI-agent tooling.

---

## Supply-Chain / Reproducibility Trajectory

**Trend (near-term, 1–3yr, Confidence: High, Plausibility: Very High).** cosign
signing + SLSA provenance + SBOMs have become baseline expectation, not advanced
practice. The decisive change is _effort collapse_: "Two years ago, setting up Sigstore
and SLSA was a multi-week project. Today … you can get to SLSA Level 2 for most artifact
types in an afternoon" via GitHub-native attestations and mature `slsa-github-generator`
/ cosign tooling ([AquilaX](https://aquilax.ai/blog/supply-chain-artifact-signing-slsa)).
Keyless signing (Fulcio short-lived OIDC certs + Rekor transparency log) removes the
long-lived-key management burden that used to make signing impractical for solo
maintainers ([IntelligenceX](https://blog.intelligencex.org/secure-supply-chain-with-sigstore-cosign-slsa-framework)).

**Regulatory forcing function (Confidence: High, Plausibility: High).** The EU Cyber
Resilience Act makes machine-readable SBOMs a legal condition of market access:
vulnerability/incident reporting obligations start **11 Sept 2026**, and the SBOM
mandate (Annex I) is enforceable **11 Dec 2027**, with penalties up to €15M or 2.5% of
global turnover ([Anchore](https://anchore.com/sbom/eu-cra/),
[HeroDevs](https://www.herodevs.com/blog-posts/cra-reporting-obligations-start-september-2026-what-eol-dependencies-mean-for-your-compliance)).
This does **not** directly bind a solo hobby dotfiles tool, but it means SBOM/signing
becomes ambient infrastructure — CI actions, GoReleaser blocks, and downstream consumer
expectations all normalize around it. CycloneDX and SPDX are the two accepted formats;
teams on Syft/Trivy default to CycloneDX
([finitestate](https://finitestate.io/blog/eu-cra-sbom-technical-documentation-guide)).

**Reproducible builds (Confidence: High, Plausibility: High).** Go is the model
citizen: the toolchain is byte-for-byte reproducible since 1.21, backed by
`sum.golang.org` transparency log and independent third-party rebuild verification
([go.dev/blog/rebuild](https://go.dev/blog/rebuild),
[agwa.name](https://www.agwa.name/blog/post/verifying_go_reproducible_builds)). `dotf`
inherits this for free (pure-Go, `CGO_ENABLED=0` in `.goreleaser.yml`) — a genuine
latent asset it does not advertise. A recent lockfile study across npm/pnpm/Cargo/Go
frames lockfiles' purpose as integrity verification + reproducibility across time
([Nesbitt](https://nesbitt.io/2026/02/24/reproducible-builds-in-language-package-managers.html)).

**curl | bash is the acknowledged weak link (Confidence: High, Plausibility: High).**
Even defenders concede server-compromise is the unique unmitigated risk, and there is a
real, non-theoretical attack where a server _detects piping to a shell_ (via timing of
buffered reads) and serves a different malicious payload — which defeats the
"inspect-in-browser first" defense ([Luke Spademan](https://lukespademan.com/blog/the-dangers-of-curlbash/),
[Sysdig](https://www.sysdig.com/blog/friends-dont-let-friends-curl-bash)). The
recommended mitigations are exactly what `dotf` already partially does: **pin/cache a
known-good copy + verify checksum/signature before executing** rather than piping fresh.

### Where `dotf` actually stands (codebase-grounded)

- `.goreleaser.yml` produces `checksums.txt` (`checksum: … algorithm: sha256`) but has
  **no `signs:` (cosign) block and no `sboms:` (syft) block** — confirmed by reading the
  file. So today: checksums yes, signatures no, SBOM no, provenance no.
- `bootstrap.sh:283–294` downloads `checksums.txt` and verifies the archive with
  `sha256sum -c` / `shasum -a 256` — good, but there is **no cosign verify-blob step**.
- `internal/backend/github_release.go:55` pins `pkg.SHA256` via `fetchVerifyExtract`
  (verify-before-extract); `script_url.go:34–38` _requires_ `sha256` before executing a
  vendor installer (verify-before-execute). This is already better than most dotfiles
  tools — but pinning a **moving** vendor `curl|sh` installer to one sha256 is brittle
  (the vendor rev's the script and the pin breaks; see Contradictions).
- `internal/manifest/types.go:96–102` has `SHA256` + `Tag` fields but **no** field for
  cosign identity, GitHub attestation, or provenance policy.

### Concrete near-term `dotf` steps (ranked by value/effort)

1. **Add cosign keyless signing + SBOM to `.goreleaser.yml`** (Effort: Low, Value: High).
   GoReleaser supports this natively: a `signs:` block running `cosign sign-blob
--bundle=${signature} … --yes` over the `checksum` artifact, plus an `sboms:` block
   invoking `syft` (SPDX or CycloneDX). Signing the checksum file alone is sufficient —
   "it is generally enough to sign the checksum files only"
   ([GoReleaser sign docs](https://goreleaser.com/customization/sign/sign/),
   [GoReleaser SBOM docs](https://goreleaser.com/customization/sbom/),
   [example-supply-chain](https://github.com/goreleaser/example-supply-chain)). Requires
   `cosign-installer` + `anchore/sbom-action/download-syft` + `id-token: write` in the
   release workflow.
2. **Optional cosign verify in `bootstrap.sh`** (Effort: Low, Value: High). The shim
   already verifies sha256; add an _optional_ `cosign verify-blob --certificate-identity
… --certificate-oidc-issuer https://token.actions.githubusercontent.com --bundle
checksums.txt.sigstore.json` gated behind cosign presence (graceful skip when absent).
   This closes the "checksum is only as trustworthy as the release page that served it"
   gap.
3. **A checksum lockfile** (Effort: Medium, Value: High). Emit/commit a
   `manifest/lock.toml` (or `dotf-checksums.json`) recording resolved
   `sha256`-per-(asset×arch) for every `github-release`/`script-url` entry, regenerable
   via a `dotf lock`/`--update-checksum` command. This is precisely aqua's model —
   `aqua-checksums.json` maintained by Renovate, with `require_checksum: true` making
   unverified installs impossible
   ([aqua](https://github.com/aquaproj/aqua),
   [aqua policy-as-code](https://aquaproj.github.io/docs/guides/policy-as-code/)). It
   also makes `script-url` pin drift _visible and updatable_ instead of silently stale.
4. **GitHub Artifact Attestation verify option** on `github-release` (Effort: Medium,
   Value: Medium). For upstreams that publish `actions/attest-build-provenance`, allow an
   optional `attestation = true` in the manifest entry so `dotf` can `gh attestation
verify` before install. Note: language-ecosystem provenance verification "is still
   maturing" ([AquilaX](https://aquilax.ai/blog/supply-chain-artifact-signing-slsa)) — so
   make this opt-in, not mandatory.

---

## Declarative-Environment Pressure — adopt vs reject

**Is the world moving fully declarative? Partially — but there's durable demand for the
imperative-friendly middle.** (Confidence: Medium-High, Plausibility: High.)

- The Nix-adjacent stack (Flox, Devbox, devenv) is growing precisely _because Nix has a
  steep learning curve_; they wrap Nix reproducibility in TOML/JSON manifests and
  package-manager-like CLIs ([Flox](https://flox.dev/),
  [Alex Hyett](https://www.alexhyett.com/newsletter/thoughts-on-reproducible-environments/)).
  Flox's pitch — "Nix reproducibility with none of the headaches," 16-step onboarding
  guide collapsed to one `flox activate` — is the reproducibility ceiling `dotf` cannot
  and should not try to match.
- But the counter-current is just as real: developers are **replacing Nix with mise for
  speed**, finding it "more reliable and less flakey than … Devbox" for Rust/Go
  ([Lobsters thread](https://lobste.rs/s/zupgan/what_clever_tools_workflows_do_you_use)).
  mise is the non-Nix, imperative-friendly winner — same neighborhood as `dotf`.
- chezmoi (the closest comparator, also Go single-binary) has **not** gone declarative
  for installs: it stays declarative-for-file-contents + imperative-for-packages via
  hash-triggered `run_onchange_` scripts ([chezmoi install-packages](https://www.chezmoi.io/user-guide/advanced/install-packages-declaratively/)).
  Its 2026 trajectory is _additive_: pairing with Nix/mise, becoming AI/MCP-aware, and
  adopting **SLSA-signed releases + SOPS/age secrets** — not replacing its imperative
  core ([chezmoi trends search](https://github.com/topics/chezmoi?l=shell)).
- Dev Containers are surging (64% of devs now use non-local environments as primary, up
  from 36% in 2024; Docker at 92% adoption) — but this is _project-scoped_ isolation, a
  different job than `dotf`'s _machine-scoped_ dotfiles/tooling
  ([Docker 2026](https://www.programming-helper.com/tech/docker-2026-container-adoption-enterprise-kubernetes-python)).
  They coexist: your host still needs a provisioned shell, editor, git, and the tools to
  _drive_ containers.

**ADOPT for `dotf`:**

- **Version pinning + a lockfile** (the reproducibility feature that actually matters for
  a single-host tool — pinned tool versions + recorded checksums, à la aqua/mise).
- **Idempotent, safe re-apply with a `diff`-first workflow** (chezmoi's `apply` computes
  minimum changes; its cautionary "always `diff` first" is a UX pattern to emulate).
- **Interop, not replacement**: `dotf` should _drive_ mise/brew cleanly (it already
  bootstraps Homebrew and shells out to mise), positioning itself as the host-level
  orchestrator that hands off runtime management to mise.

**REJECT for `dotf` (Confidence: High):**

- A Nix/flakes rewrite or a Nix backend — wrong complexity budget for a solo maintainer;
  contradicts the deliberately imperative-friendly niche.
- Full declarative reconciliation / desired-state pruning as a _default_ — the PRD's
  additive-only stance matches chezmoi's own; keep pruning behind an explicit future
  `--prune`. (Contrarian persona should stress-test this; the Futurist view is that
  additive-only remains defensible for 3+ years.)
- Trying to guarantee OS-package-level reproducibility (the Nix tools' actual moat) —
  out of scope and unwinnable.

---

## AI-Assisted Config — low-effort wins vs hype

**Durable, low-effort wins (Confidence: Medium, Plausibility: Medium-High):**

- **Structured JSON output** (`dotf plan --json`, `status --json`, `diff --json`). The
  strongest 2026 signal is "CLI is the new MCP" — agents increasingly prefer driving
  plain CLIs (existing permission model, no new protocol) over bespoke MCP servers
  ([OneUptime](https://oneuptime.com/blog/post/2026-02-03-cli-is-the-new-mcp/view)). A
  clean `--json` surface lets the repo's existing Claude/Codex/opencode agents consume
  `dotf` with zero new infrastructure. This is the single best AI-adjacent bet and it's
  really just good CLI hygiene.
- **Machine-readable drift/health** (`dotf doctor --json`) so an agent can _explain_
  drift and propose manifest edits.

**Plausible but lower-priority:**

- **`dotf`-as-MCP-server** — feasible (MCP is "USB-C for tools"), but the CLI-is-the-new-MCP
  trend argues a thin CLI + JSON beats maintaining an MCP server for a solo tool
  ([modelcontextprotocol.io](https://modelcontextprotocol.io/docs/develop/build-with-agent-skills)).
  Build the JSON surface first; an MCP wrapper is trivial to add later if wanted.
- **NL "add tool X" → manifest entry** — genuinely on-trend (agent-skills scaffolding
  manifests from natural language is a 2026 pattern), but for `dotf` this is better
  delivered as _an agent skill in the repo that edits `packages.toml`_ than as CLI code.
  Low code cost, no core-loop risk.

**Hype to resist (Confidence: Medium):** LLM-generated manifests as a _runtime_ feature,
"AI-native provisioning," or embedding a model in the tool. These add dependency weight
and non-determinism to a tool whose entire value is deterministic, verifiable installs.

---

## Universal Binary Installer Trend

**The direction is unmistakable and directly relevant to the `custom-script` migration**
(Confidence: High, Plausibility: High).

- **ubi** and **eget** both auto-resolve the correct GitHub-release asset for the host
  (filter by extension → libc → arch), so you install by pointing at `owner/repo` with no
  per-asset templating ([ubi](https://github.com/houseabsolute/ubi),
  [eget](https://github.com/zyedidia/eget)). This is "per-asset templating going
  obsolete in favor of auto-resolution" in action.
- **mise** absorbed this as first-class backends and is the bellwether. Its backend
  priority is **aqua (checksum-verified, preferred) → github/ubi → cargo/npm/go/pipx →
  http** ([mise backends](https://mise.jdx.dev/dev-tools/backends/),
  [zenn declarative CLI mgmt](https://zenn.dev/ryo246912/articles/declarative-cli-management-with-mise?locale=en)).
- **The load-bearing 2026 signal:** mise **deprecated its `ubi` backend in favor of
  `github`** and now **refuses new `ubi`/`asdf` registry entries "for supply-chain
  security reasons," steering to aqua or github** ([mise backends](https://mise.jdx.dev/dev-tools/backends/)).
  Translation: auto-resolution won — _but only married to checksum/attestation
  verification._ Naïve "grab whatever asset matches" is being actively deprecated.
- Convergent evidence at the OS layer: Homebrew's bottle model (pre-built binaries via
  GitHub Packages/OCI) is now being copied by **APT (Ubuntu 26.04 installs from OCI
  registries)** and **DNF5 (experimental OCI plugin)**
  ([nttlabs](https://medium.com/nttlabs/ubuntu-26-04-can-install-apt-packages-from-github-container-registry-532412990318),
  [LWN](https://lwn.net/Articles/1046236/)). GitHub-release-as-distribution is becoming
  the universal substrate.

**Bet accordingly:** `dotf`'s `github-release` backend is on the winning side. It already
templates `{{.OS}}`/`{{.Arch}}` (`github_release.go:96–99`) and pins `sha256`. The
future-proof move is **not** to abandon per-asset config for pure auto-resolution, but to
**offer optional auto-resolution (ubi/eget-style) as a convenience while keeping the
`sha256`/lockfile pin as the trust anchor** — i.e., copy mise's "aqua-preferred, github
second, checksums always" posture. Most of the 19 `custom-script` tools that are simple
GitHub-release binaries (claude, codex, opencode, coderabbit, turso, tea, lefthook,
fabric, most of aws/azure/hashicorp/container-tools) collapse cleanly onto this backend.

---

## Emerging Technologies

- **WASM/WASI as a distribution format (Confidence: Medium, Plausibility: Low-Medium for
  `dotf`'s domain).** WASIp2 + the Component Model are production-ready for _edge/serverless
  and plugin systems_, WASI 0.3 added async I/O (Feb 2026), WASI 1.0 targeted for 2026
  ([Java Code Geeks](https://www.javacodegeeks.com/2026/04/webassembly-in-2026-where-it-has-landed-what-wasi-0-2-changes-and-why-java-and-kotlin-developers-should-pay-attention-now.html),
  [wasmRuntime](https://wasmruntime.com/en/blog/wasi-preview2-vs-wasix-2026)). But
  Go-to-WASM binaries are 8–15MB, threading is still absent, and the "single binary runs
  everywhere" vision "has narrowed but not closed." For a host-provisioning tool that must
  touch the real filesystem, shell, sudo, and package managers, native Go binaries remain
  strictly better through 2028+. **Verdict: not a distribution path for `dotf`; watch as a
  possible _plugin/extension_ mechanism only if `dotf` ever grows a plugin surface.**
- **OCI-registry distribution (Confidence: High, Plausibility: Medium).** The APT/DNF5
  OCI convergence means `dotf` _could_ one day publish/consume via OCI, but GitHub
  Releases + checksums already give it the same benefits without the ceremony. No action
  now; low regret.
- **Renovate-style automated checksum/version updates (Confidence: High, Plausibility:
  High).** aqua and mise both lean on Renovate to keep pinned versions + checksums fresh
  ([Renovate mise module](https://docs.renovatebot.com/modules/manager/mise/)). If `dotf`
  ships a lockfile, a Renovate/CI job to bump pins is the natural companion — low effort,
  high maintainability payoff.

---

## Future Scenarios for `dotf` (3)

**Scenario A — "Verified Middle Ground" (most likely, ~60%).** `dotf` adds
cosign-signed + SBOM'd releases, an optional cosign verify in the shim, and a checksum
lockfile. It migrates the ~12–14 simple `custom-script` tools onto `github-release` with
auto-resolution + pinned checksums, keeps ~5 genuine setup-steps (tpm git-clone,
tmux-sessionizer, gnu-utilities, mise/rust as rolling version managers) as scripts, and
delegates runtime version management to mise. It positions as "the verifiable,
imperative-friendly host provisioner that drives mise/brew." This is fully inside the
current scope and the evidence base.

**Scenario B — "Absorbed by mise/aqua" (~20%).** If mise's `github`/`aqua` backends +
chezmoi's `run_onchange_` cover enough of the surface, the marginal reason to maintain a
bespoke tool shrinks. `dotf` stays valuable as _personal glue_ but the ecosystem does the
heavy lifting. Mitigation: `dotf`'s value is the **symlink engine + sysconfig +
single-command host bootstrap**, which mise/aqua don't do — lean into that, don't try to
out-package-manage them.

**Scenario C — "Declarative wave crests" (~20%, longer horizon).** Nix-adjacent tooling
(Flox/devenv) or Dev Containers become so frictionless that host-level dotfiles matter
less. Even here, someone provisions the host that runs the containers; `dotf` survives as
the bootstrap layer. Low probability of full obsolescence before ~2030.

---

## Timeline Predictions

- **2026 (now→12mo):** cosign+SBOM in releases is _expected_ of any serious CLI; CRA
  reporting obligations (Sept 2026) normalize SBOM tooling industry-wide. **Near-term
  bets are all shippable now.** (Confidence: High)
- **2027–2028:** checksum lockfiles + attestation-verify become standard for
  install-from-GitHub tools (aqua/mise already there); auto-resolution backends are the
  default idiom; CRA SBOM mandate (Dec 2027) fully in force. (Confidence: Medium-High)
- **2029–2030+:** possible OCI-registry-native distribution mainstreaming; WASM
  plugin ecosystems _maybe_ viable; AI agents routinely driving provisioning CLIs via
  JSON/MCP. (Confidence: Low — speculative)

---

## Wild Cards

- **GitHub Actions itself is the soft underbelly** — "no lockfile, no integrity
  verification, mutable tags" ([Nesbitt](https://nesbitt.io/2026/02/24/reproducible-builds-in-language-package-managers.html)).
  If a high-profile Actions supply-chain incident hits, _pinned-SHA everything_ (including
  `uses:` in `.github/workflows/release.yml`) becomes table-stakes overnight. Cheap
  insurance: pin action SHAs now.
- **A cosign/Sigstore keyless UX regression or Rekor outage** could dampen keyless
  adoption; keep sha256 as the non-negotiable floor so verification never _depends_ on
  Sigstore availability.
- **mise (or chezmoi) ships a first-class symlink+sysconfig story**, eroding `dotf`'s
  differentiator. Watch mise's and chezmoi's roadmaps.
- **AI-agent-authored dotfiles as a norm** — if agents routinely generate/maintain
  dotfiles repos, machine-readable `--json` + a repo agent-skill become the primary
  interface, not the human TUI.

---

## Key Insights

1. **`dotf` is one GoReleaser block away from the 2026 supply-chain baseline.** cosign
   `signs:` + syft `sboms:` are near-trivial and the highest-leverage bets. (High)
2. **Auto-resolution won, but only _with_ verification** — mise deprecating `ubi` for
   `github` and banning unverified new entries is the canonical proof. `dotf`'s
   `sha256`-pinned `github-release` backend is on the right side; add optional
   auto-resolution as convenience, keep the pin/lockfile as trust anchor. (High)
3. **A checksum lockfile is the reproducibility feature that fits `dotf`'s scope** — it
   solves `script-url` pin-drift, matches aqua's model, and pairs with Renovate. (High)
4. **The imperative middle ground is durable, not doomed** — chezmoi and mise both thrive
   there; reject the Nix-rewrite temptation. (Medium-High)
5. **Best AI bet is `--json`, not an LLM feature** — "CLI is the new MCP." (Medium)
6. **Go reproducibility is a latent, un-advertised asset** — `CGO_ENABLED=0` pure-Go
   builds are already reproducible; lean into it in docs/attestations. (High)

---

## Evidence Quality

- **Strong / primary or authoritative:** GoReleaser docs (sign/SBOM), aqua & mise docs
  (backends, deprecation, checksum model), Go's reproducible-build blog + independent
  verification, chezmoi docs, ubi/eget repos, EU CRA analyses from compliance vendors.
  These are current (2026) and load-bearing.
- **Moderate / secondary synthesis:** the "SLSA is table-stakes" framing (vendor blogs,
  consistent across ≥4 independent sources — high confidence in the _direction_, softer
  on exact adoption %), WASM 2026 status (multiple sources, some date-variance on WASI
  0.3.0 shipping — flagged in the source itself), Docker/dev-container adoption stats
  (single-survey-derived, directionally consistent).
- **Weak / speculative:** 5yr+ predictions (OCI-native `dotf`, WASM plugins, AI-authored
  dotfiles) — explicitly tagged Low confidence.
- **Freshness note:** the reproducible-env comparison search surfaced mostly mid-2025
  community sentiment rather than 2026 adoption numbers; treat the mise-vs-Nix _momentum_
  claim as directional, not quantified.

---

## Contradictions & Uncertainties

- **Pin-a-moving-installer paradox.** `dotf`'s `script-url` _requires_ a sha256, but
  vendor `curl|sh` installers are updated frequently, so any pin is either stale or
  constantly churning. A lockfile + Renovate makes the churn _visible and managed_ rather
  than solving it. The deeper tension: for a rolling installer there is no "correct"
  permanent hash — this is an inherent limitation of the vendor-installer model, and an
  argument to migrate those tools to `github-release` (pinned artifacts) where possible.
- **Auto-resolution vs explicit-pin.** Pure auto-resolution (ubi/eget) _reduces_ manifest
  verbosity but _weakens_ verifiability if it means "install whatever asset matches."
  mise's own deprecation of `ubi` shows the ecosystem resolving this in favor of
  _verification-first_. `dotf` should not chase zero-config auto-resolution at the cost of
  its pinning discipline. (Contradicts a naïve "auto-resolution obsoletes templating" read.)
- **"Fully declarative future" is contested.** The Nix-adjacent growth and the
  Nix→mise-for-speed migration point in opposite directions; the honest read is _both_ a
  declarative high-end and an imperative pragmatic middle persist. `dotf` should not bet
  the tool on either extreme.
- **Regulation relevance is indirect.** CRA does not bind a solo dotfiles tool, so
  "table-stakes" is about _ecosystem norms and consumer expectation_, not legal
  compulsion — don't over-rotate on compliance framing for a personal tool.
- **WASM date-variance.** Sources disagree on whether WASI 0.3.0 shipped Feb 2026; the
  source itself flags this. Not decision-critical for `dotf`.

---

## Search Queries Executed

1. `SLSA provenance Sigstore cosign signing release binaries 2026 supply chain table stakes`
2. `reproducible builds Go binaries checksum lockfile supply chain verification 2026`
3. `mise devbox flox devenv reproducible development environment 2026 adoption comparison`
4. `ubi eget universal binary installer GitHub release auto-resolution Go tool`
5. `AI agent MCP server CLI tool natural language configuration LLM generated manifest 2026`
6. `Homebrew binary bottle model dnf apt install from GitHub release trend 2026`
7. `chezmoi dotfiles manager 2026 trends declarative reproducible future direction`
8. `SBOM software bill of materials CLI tool release table stakes 2026 regulation EU CRA`
9. `aqua declarative CLI version manager lockfile checksum registry security 2026`
10. `curl pipe bash install script security risk 2026 alternatives verified install`
11. `dev containers vs local development environment 2026 developer adoption mise trend`
12. `WASM WebAssembly single binary cross-compilation software distribution future 2026 wasip2`
13. `GoReleaser cosign sign checksums SLSA provenance SBOM syft 2026 release`
14. `mise backends ubi aqua cargo go npm asdf unified tool installation 2026`

(SCAMPER coverage: Substitute [github-release↔custom-script], Combine [signing+SBOM+lockfile],
Adapt [aqua/mise verification model→dotf], Modify [pin scope: per-asset→lockfile], Eliminate
[curl|bash risk], Reverse [declarative pressure vs imperative durability].)
