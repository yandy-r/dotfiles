package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/privilege"
)

// lockOpts holds the parsed flag values for `dotf lock`.
type lockOpts struct {
	manifest  string
	lock      string
	only      multiFlag
	skip      multiFlag
	platforms multiFlag
	check     bool
	dryRun    bool
	jsonOut   bool
}

// setLockFlags registers lock's flags on fs and returns the opts pointer they
// parse into. It is the single source of truth for lock's flag surface: runLock
// calls it before fs.Parse, and the help/completion renderers call it on a
// throwaway flagset to derive the Flags section and completions (DRY).
func setLockFlags(fs *flag.FlagSet) *lockOpts {
	o := &lockOpts{}
	fs.StringVar(&o.manifest, "manifest", "", manifestFlagUsage)
	fs.StringVar(&o.lock, "lock", "", "path to the output lock file, overriding the manifest sibling default")
	fs.Var(&o.only, "only", "lock only these named packages, independent of class (repeatable)")
	fs.Var(&o.skip, "skip", "exclude these named packages after selection (repeatable)")
	fs.Var(&o.platforms, "platform", "additional os/arch target to lock beyond the host (repeatable)")
	fs.BoolVar(&o.check, "check", false, "re-resolve and diff against the committed lock without writing; exit 1 on drift")
	fs.BoolVar(&o.dryRun, "dry-run", false, "print the resolved artifacts without writing the lock")
	fs.BoolVar(&o.jsonOut, "json", false, jsonFlagUsage)
	return o
}

// lockFailure records a single per-(package, target) resolution failure so the
// run can collect and summarize them non-fatally instead of wedging on one dead
// asset URL.
type lockFailure struct {
	name   string
	target string
	err    error
}

// runLock resolves every pinnable package for the target platform(s), records
// their frozen checksums, and writes/checks/prints manifest/dotf.lock. Non-
// pinnable packages (rolling/managed) are silently excluded. Per-package
// resolution failures are non-fatal; the run exits 1 if any failed (or, with
// --check, on drift).
func runLock(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("lock", flag.ContinueOnError)
	fs.SetOutput(stderr)

	o := setLockFlags(fs)
	noUserConfig := setNoUserConfigFlag(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	// A scoped check would compare a filtered resolution against the full
	// committed lock and always report false drift, so the combination is a
	// usage error rather than a silently-wrong result.
	if o.check && (len(o.only) > 0 || len(o.skip) > 0) {
		_, _ = fmt.Fprintf(stderr, "%s: --check cannot be combined with --only/--skip\n", progName)
		return 2
	}

	m, err := manifest.Resolve(o.manifest, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(noUserConfigEnabled(*noUserConfig), stderr))
	if err != nil {
		return reportErr(stderr, err)
	}

	targets, err := lockTargets(r, o.platforms)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	pkgs, err := filterPackages(m, m.Packages, o.only, o.skip)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", progName, err)
		return 2
	}

	// A nil lock: resolution downloads and hashes fresh, so no lock is consulted.
	engine := backend.Default(r, privilege.NewSudo(), privilege.NewFakeDropper(), resolveRepoRoot(), nil)

	lock, failures := resolveLock(engine, pkgs, targets)
	printLockSummary(stderr, lock, failures)
	failed := len(failures) > 0

	switch {
	case o.check:
		return runLockCheck(stdout, stderr, o, lock, failed)
	case o.jsonOut:
		// --json wins the output format even when --dry-run is also set (both are
		// no-write): the documented `dotf lock --dry-run --json` must emit JSON.
		if err := writeJSON(stdout, buildLockReport(lock)); err != nil {
			return reportErr(stderr, err)
		}
		return lockExit(failed)
	case o.dryRun:
		if err := writeLockText(stdout, lock, true); err != nil {
			return reportErr(stderr, err)
		}
		return lockExit(failed)
	}

	outPath, err := lockOutputPath(o, resolveRepoRoot())
	if err != nil {
		return reportErr(stderr, err)
	}
	if err := writeLockFile(outPath, lock); err != nil {
		return reportErr(stderr, err)
	}
	if err := writeLockText(stdout, lock, false); err != nil {
		return reportErr(stderr, err)
	}
	_, _ = fmt.Fprintf(stderr, "%s: wrote %s\n", progName, outPath)
	return lockExit(failed)
}

// resolveLock resolves each pinnable package for every target into a Lock,
// collecting per-target failures. A package that resolves no artifact at all
// (every target failed) carries no lock entry. Artifacts are deduplicated by
// (os, arch) so a platform-agnostic script-url pin records a single entry even
// across multiple targets.
func resolveLock(engine *backend.Engine, pkgs []manifest.Package, targets []platform.Info) (*lockfile.Lock, []lockFailure) {
	lock := &lockfile.Lock{Version: lockfile.Version}
	var failures []lockFailure

	for _, pkg := range pkgs {
		if !lockfile.Pinnable(pkg) {
			continue
		}
		lp := lockfile.LockedPackage{
			Name:    pkg.Name,
			Backend: string(pkg.Backend),
			Tag:     pkg.Tag,
			URL:     pkg.URL,
		}
		for _, target := range targets {
			// Lock resolution is not a running desktop session. Honor explicit
			// OS/architecture restrictions without the graphical-session gate.
			candidate := pkg
			candidate.RequiresDesktop = false
			if ok, err := engine.Applicable(candidate, target); err == nil && !ok {
				continue
			}
			art, err := engine.Resolve(pkg, target)
			if err != nil {
				failures = append(failures, lockFailure{name: pkg.Name, target: platformToken(target), err: err})
				continue
			}
			lp.Artifacts = appendArtifact(lp.Artifacts, art)
		}
		if len(lp.Artifacts) == 0 {
			continue
		}
		lock.Packages = append(lock.Packages, lp)
	}

	return lock, failures
}

// appendArtifact appends a to arts, replacing any existing artifact with the
// same (os, arch) so re-resolving a platform-agnostic pin across targets keeps
// a single entry.
func appendArtifact(arts []lockfile.Artifact, a lockfile.Artifact) []lockfile.Artifact {
	for i, existing := range arts {
		if existing.OS == a.OS && existing.Arch == a.Arch {
			arts[i] = a
			return arts
		}
	}
	return append(arts, a)
}

// lockTargets returns the resolution target matrix: the detected host first,
// then each explicit --platform os/arch (host-only is the default because you
// can only checksum an asset you can download).
func lockTargets(r exec.Runner, platforms []string) ([]platform.Info, error) {
	targets := []platform.Info{platform.Detect(r)}
	for _, p := range platforms {
		info, err := parsePlatform(p)
		if err != nil {
			return nil, err
		}
		targets = append(targets, info)
	}
	return targets, nil
}

// parsePlatform parses an "os/arch" --platform value into a platform.Info.
func parsePlatform(s string) (platform.Info, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return platform.Info{}, fmt.Errorf("invalid --platform %q, want os/arch", s)
	}
	arch := platform.Arch(parts[1])
	switch parts[1] {
	case "amd64":
		arch = platform.ArchX86_64
	case "aarch64":
		arch = platform.ArchARM64
	}
	if arch != platform.ArchX86_64 && arch != platform.ArchARM64 {
		return platform.Info{}, fmt.Errorf("invalid --platform arch %q, want x86_64/amd64 or arm64/aarch64", parts[1])
	}
	return platform.Info{OS: platform.OS(parts[0]), Arch: arch}, nil
}

// platformToken renders a target as the "os/arch" token used in failure
// diagnostics, matching the release tokens recorded in the lock.
func platformToken(info platform.Info) string {
	return string(info.OS) + "/" + info.Arch.ReleaseArch()
}

// lockOutputPath resolves where the lock is written or checked: the --lock
// override wins, otherwise the manifest/dotf.lock under the repo root. Erroring
// when repoRoot is empty is correct — you cannot write into the embed.
func lockOutputPath(o *lockOpts, repoRoot string) (string, error) {
	if o.lock != "" {
		return o.lock, nil
	}
	if repoRoot == "" {
		return "", errors.New("could not locate the dotfiles repo root (set DOTFILES_ROOT) to write manifest/dotf.lock")
	}
	return filepath.Join(repoRoot, "manifest", "dotf.lock"), nil
}

// writeLockFile renders lock to deterministic TOML and writes it to path,
// creating the parent directory if needed.
func writeLockFile(path string, lock *lockfile.Lock) error {
	buf, err := canonicalLock(lock)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create lock directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(buf), 0o644); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

// runLockCheck re-resolves and diffs against the committed lock without
// writing, exiting 1 on drift. It is the CI freshness gate that keeps Renovate
// optional. Both sides are normalized through canonicalLock so cosmetic
// formatting never triggers false drift.
func runLockCheck(stdout, stderr io.Writer, o *lockOpts, resolved *lockfile.Lock, failed bool) int {
	// A resolution failure means we could not compute the fresh lock, so we
	// cannot honestly claim the committed lock is stale. Report a distinct error
	// verdict (the per-failure diagnostics were already printed to stderr) rather
	// than misattributing a dead URL as drift.
	if failed {
		_, _ = io.WriteString(stdout, "check=error\n")
		return lockExit(failed)
	}

	outPath, err := lockOutputPath(o, resolveRepoRoot())
	if err != nil {
		return reportErr(stderr, err)
	}
	committed, err := readCommittedLock(outPath)
	if err != nil {
		return reportErr(stderr, err)
	}

	want, err := canonicalLock(resolved)
	if err != nil {
		return reportErr(stderr, err)
	}
	got, err := canonicalLock(committed)
	if err != nil {
		return reportErr(stderr, err)
	}

	if want != got {
		_, _ = io.WriteString(stdout, "check=drift\n")
		_, _ = fmt.Fprintf(stderr, "%s: dotf.lock is stale; run `dotf lock` to refresh %s\n", progName, outPath)
		return 1
	}

	_, _ = io.WriteString(stdout, "check=current\n")
	return lockExit(failed)
}

// readCommittedLock parses the committed lock at path. A missing file yields an
// empty &Lock{Version} (drift against a non-empty resolution); a malformed file
// is a hard error.
func readCommittedLock(path string) (*lockfile.Lock, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &lockfile.Lock{Version: lockfile.Version}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return lockfile.Parse(f)
}

// canonicalLock renders lock to its deterministic TOML string, the normal form
// used for both writing and drift comparison.
func canonicalLock(lock *lockfile.Lock) (string, error) {
	var buf bytes.Buffer
	if err := lockfile.Write(&buf, lock); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// writeLockText writes the resolved lock as greppable key=value lines: a
// comma-joined locked= list and one artifact.<name>.<os/arch>=<sha256> line per
// artifact (the platform token is "any" for a platform-agnostic pin). dryRun
// prepends a dry-run=true marker.
func writeLockText(w io.Writer, lock *lockfile.Lock, dryRun bool) error {
	pkgs := sortedLockedPackages(lock)

	var lines []string
	if dryRun {
		lines = append(lines, "dry-run=true")
	}

	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}
	lines = append(lines, "locked="+strings.Join(names, ","))

	for _, p := range pkgs {
		for _, a := range sortedArtifacts(p) {
			lines = append(lines, fmt.Sprintf("artifact.%s.%s=%s", p.Name, artifactPlatform(a), a.SHA256))
		}
	}

	_, err := io.WriteString(w, strings.Join(lines, "\n")+"\n")
	return err
}

// buildLockReport builds the --json report from lock in deterministic order.
func buildLockReport(lock *lockfile.Lock) lockReport {
	pkgs := sortedLockedPackages(lock)
	rep := lockReport{Command: "lock", Packages: make([]lockedPackageReport, 0, len(pkgs))}
	for _, p := range pkgs {
		arts := sortedArtifacts(p)
		rows := make([]artifactReport, len(arts))
		for i, a := range arts {
			rows[i] = artifactReport{OS: a.OS, Arch: a.Arch, Asset: a.Asset, Binary: a.Binary, SHA256: a.SHA256}
		}
		rep.Packages = append(rep.Packages, lockedPackageReport{
			Name: p.Name, Backend: p.Backend, Tag: p.Tag, URL: p.URL, Artifacts: rows,
		})
	}
	return rep
}

// sortedLockedPackages returns lock's packages sorted by name (a copy).
func sortedLockedPackages(lock *lockfile.Lock) []lockfile.LockedPackage {
	pkgs := append([]lockfile.LockedPackage(nil), lock.Packages...)
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Name < pkgs[j].Name })
	return pkgs
}

// sortedArtifacts returns p's artifacts sorted by os then arch (a copy).
func sortedArtifacts(p lockfile.LockedPackage) []lockfile.Artifact {
	arts := append([]lockfile.Artifact(nil), p.Artifacts...)
	sort.Slice(arts, func(i, j int) bool {
		if arts[i].OS != arts[j].OS {
			return arts[i].OS < arts[j].OS
		}
		return arts[i].Arch < arts[j].Arch
	})
	return arts
}

// artifactPlatform renders an artifact's platform token for greppable output:
// "os/arch", or "any" for a platform-agnostic pin.
func artifactPlatform(a lockfile.Artifact) string {
	if a.OS == "" && a.Arch == "" {
		return "any"
	}
	return a.OS + "/" + a.Arch
}

// printLockSummary writes an aggregate count and per-failure diagnostics to
// stderr, mirroring install's summary discipline.
func printLockSummary(stderr io.Writer, lock *lockfile.Lock, failures []lockFailure) {
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].name != failures[j].name {
			return failures[i].name < failures[j].name
		}
		return failures[i].target < failures[j].target
	})

	artifacts := 0
	for _, p := range lock.Packages {
		artifacts += len(p.Artifacts)
	}

	_, _ = fmt.Fprintf(stderr, "%s: lock summary: packages=%d artifacts=%d failed=%d\n",
		progName, len(lock.Packages), artifacts, len(failures))
	for _, f := range failures {
		_, _ = fmt.Fprintf(stderr, "%s: %s (%s): %v\n", progName, f.name, f.target, f.err)
	}
}

// lockExit maps the soft-failure flag to an exit code (1 when any per-package
// resolution failed, else 0).
func lockExit(failed bool) int {
	if failed {
		return 1
	}
	return 0
}
