package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	dotfiles "github.com/yandy-r/dotfiles"
	"github.com/yandy-r/dotfiles/internal/backend"
	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
	"github.com/yandy-r/dotfiles/internal/symlink"
	"github.com/yandy-r/dotfiles/internal/sysconfig"
)

// runDoctor composes the existing read-only paths — platform.Detect, LookPath
// prerequisite probes, the backend engine's Applicable/Installed probe, the
// symlink engine's Plan/Validate, sysconfig support classification, and the
// plasma engine's Support/Diff — into a single health report. It mutates nothing: no sudo, no writes, only reads and
// pure classification. stdout carries sorted greppable "check.CATEGORY.NAME=..."
// lines plus a summary line; stderr carries an actionable remedy per non-ok
// check. It exits 1 when any check fails (warnings stay green), 2 on a flag
// error — never 3, which is reserved for apply's hard aborts.
func runDoctor(args []string, stdout, stderr io.Writer, r exec.Runner) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)

	manifestPath, jsonOut, verbose := setDoctorFlags(fs)
	noUserConfig := setNoUserConfigFlag(fs)

	if err := fs.Parse(args); err != nil {
		return 2
	}

	info := platform.Detect(r)
	var checks []doctorCheck

	// The manifest is resolved but non-fatal: a broken manifest is surfaced as a
	// failing check so doctor still diagnoses the rest of the host, rather than
	// aborting early.
	m, mErr := manifest.Resolve(*manifestPath, os.Getenv("DOTF_MANIFEST"), dotfiles.ManifestFS,
		manifestOverlayOptions(noUserConfigEnabled(*noUserConfig), stderr))
	if mErr != nil {
		checks = append(checks, manifestFailCheck(mErr))
	}

	checks = append(checks, checkPlatform(info))
	checks = append(checks, checkPrerequisites(r, info)...)
	if m != nil {
		engine := backend.Default(r, nil, os.Getenv("DOTFILES_ROOT"), nil)
		checks = append(checks, checkPackages(m, engine, info))
	}

	home, _ := os.UserHomeDir()
	repoRoot := resolveRepoRoot()
	checks = append(checks, checkSymlinks(repoRoot, home, info)...)
	checks = append(checks, checkBackups(home))
	checks = append(checks, checkSysconfig(info))
	checks = append(checks, checkSELinux(r, info)...)
	checks = append(checks, checkPlasma(repoRoot, xdgConfigHome(home), xdgDataHome(home), info, r)...)

	sort.Slice(checks, func(i, j int) bool {
		if checks[i].Category != checks[j].Category {
			return checks[i].Category < checks[j].Category
		}
		return checks[i].Name < checks[j].Name
	})
	sum := summarizeChecks(checks)

	if *jsonOut {
		if err := writeJSON(stdout, doctorReport{Command: "doctor", Checks: checks, Summary: sum}); err != nil {
			return reportErr(stderr, err)
		}
		return doctorExitCode(sum)
	}

	if err := printDoctorText(stdout, checks, sum); err != nil {
		return reportErr(stderr, err)
	}
	printDoctorRemedies(stderr, checks, sum, *verbose)
	return doctorExitCode(sum)
}

// checkPlatform composes platform.Detect's result into the platform.detect
// check: an unknown OS is a hard fail (dotf cannot target it), a Linux host with
// no recognized package manager is a warning (the pkg backend cannot run), and
// anything else is ok.
func checkPlatform(info platform.Info) doctorCheck {
	detail := fmt.Sprintf("os=%s arch=%s pkg=%s", info.OS, info.Arch, info.PkgMgr)
	switch {
	case info.OS == platform.OSUnknown:
		return doctorCheck{
			Category: "platform", Name: "detect", Status: checkFail, Detail: detail,
			Remedy: "unsupported OS; dotf targets macOS and the Linux families (debian/redhat/arch/suse)",
		}
	case platform.IsLinuxFamily(info.OS) && info.PkgMgr == platform.PkgMgrUnknown:
		return doctorCheck{
			Category: "platform", Name: "detect", Status: checkWarn, Detail: detail,
			Remedy: "no supported package manager detected on PATH",
		}
	default:
		return doctorCheck{Category: "platform", Name: "detect", Status: checkOK, Detail: detail}
	}
}

// checkPrerequisites probes the host tools dotf relies on via r.LookPath (the
// same seam the backend probe uses). git and the detected package manager are
// hard prerequisites (fail when missing); a download tool, sudo, and Homebrew
// are advisory (warn). The package-manager row is emitted only when the platform
// has a known manager, and the sudo/homebrew rows are gated on euid/macOS so a
// root or non-macOS host is not warned about tools it does not need.
func checkPrerequisites(r exec.Runner, info platform.Info) []doctorCheck {
	onPath := func(cands ...string) bool {
		for _, c := range cands {
			if c == "" {
				continue
			}
			if _, err := r.LookPath(c); err == nil {
				return true
			}
		}
		return false
	}
	row := func(name, foundDetail, missingDetail string, sev checkStatus, remedy string, present bool) doctorCheck {
		if present {
			return doctorCheck{Category: "prerequisites", Name: name, Status: checkOK, Detail: foundDetail}
		}
		return doctorCheck{Category: "prerequisites", Name: name, Status: sev, Detail: missingDetail, Remedy: remedy}
	}

	checks := []doctorCheck{
		row("git", "git on PATH", "git not on PATH", checkFail,
			"install git (required by git-clone backends and repo ops)", onPath("git")),
		row("download", "curl or wget on PATH", "neither curl nor wget on PATH", checkWarn,
			"install curl or wget (release-download backends need one)", onPath("curl", "wget")),
	}

	if bin := pkgMgrBinary(info.PkgMgr); bin != "" {
		checks = append(checks, row("pkg-manager",
			fmt.Sprintf("package manager %q on PATH", bin),
			fmt.Sprintf("detected package manager %q is not on PATH", bin),
			checkFail, fmt.Sprintf("install or repair %q", bin), onPath(bin)))
	}
	if os.Geteuid() != 0 {
		checks = append(checks, row("sudo", "sudo on PATH", "sudo not on PATH", checkWarn,
			"install sudo or run as root for sysconfig / privileged installs", onPath("sudo")))
	}
	if info.OS == platform.OSMacOS {
		checks = append(checks, row("homebrew", "brew on PATH", "brew not on PATH", checkWarn,
			"install Homebrew (brew.sh); many macOS packages require it", onPath("brew")))
	}
	return checks
}

// pkgMgrBinary maps a canonical PkgMgr to the binary name expected on PATH:
// Homebrew resolves as "brew", the apt/dnf/yum/pacman/zypper managers resolve as
// their own names, and an unknown manager yields "" so callers can skip the
// package-manager prerequisite (the platform check already flags an unknown one).
func pkgMgrBinary(p platform.PkgMgr) string {
	switch p {
	case platform.PkgMgrUnknown:
		return ""
	case platform.PkgMgrHomebrew:
		return "brew"
	default:
		return string(p)
	}
}

// checkPackages aggregates the declared-package probe into a single drift check.
// It reuses the engine's public Applicable/Installed primitives (the same ones
// driftReport loops) rather than refactoring that byte-identical formatter: a
// probe error fails the check, any not-installed applicable package is an
// advisory warn, and a fully provisioned set is ok.
func checkPackages(m *manifest.Manifest, engine *backend.Engine, info platform.Info) doctorCheck {
	missing, applicable, err := missingPackages(m, engine, info)
	if err != nil {
		return doctorCheck{
			Category: "packages", Name: "drift", Status: checkFail,
			Detail: "package probe failed: " + err.Error(),
			Remedy: "fix the failing package's backend or manifest entry",
		}
	}
	if len(missing) > 0 {
		return doctorCheck{
			Category: "packages", Name: "drift", Status: checkWarn,
			Detail: fmt.Sprintf("%d of %d applicable packages not installed", len(missing), applicable),
			Remedy: "run `dotf install` or `dotf apply`",
			Items:  missing,
		}
	}
	return doctorCheck{
		Category: "packages", Name: "drift", Status: checkOK,
		Detail: fmt.Sprintf("all %d applicable packages installed", applicable),
	}
}

// missingPackages probes every declared package read-only, skipping the ones
// whose backend cannot run on this host (Applicable=false), and returns one
// item per applicable-but-not-installed package (name + backend + effective
// class) alongside the applicable count. The items are sorted by name so the
// itemized output is deterministic regardless of manifest declaration order. A
// probe error from Applicable/Installed is returned immediately, tagged with the
// offending package name.
func missingPackages(m *manifest.Manifest, engine *backend.Engine, info platform.Info) (missing []doctorItem, applicable int, err error) {
	for _, pkg := range m.Packages {
		ok, probeErr := engine.Applicable(pkg, info)
		if probeErr != nil {
			return nil, 0, fmt.Errorf("package %q: %w", pkg.Name, probeErr)
		}
		if !ok {
			continue
		}
		applicable++

		installed, probeErr := engine.Installed(pkg, info)
		if probeErr != nil {
			return nil, 0, fmt.Errorf("package %q: %w", pkg.Name, probeErr)
		}
		if !installed {
			missing = append(missing, doctorItem{
				Kind:    "package",
				Name:    pkg.Name,
				Backend: string(pkg.Backend),
				Class:   string(pkg.EffectiveClass()),
			})
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Name < missing[j].Name })
	return missing, applicable, nil
}

// checkSymlinks composes the symlink engine's read-only Plan/Validate/FindStale
// over the built-in DefaultTable (so, like `dotf link`, there is no manifest
// coupling). It emits three checks: symlinks.broken fails when a real file sits
// where a managed symlink belongs, symlinks.pending warns when links are
// declared but not yet created, and symlinks.stale warns when stale links into
// the repo linger. A missing repo root or source tree collapses to a single
// broken fail, since nothing else can be planned.
func checkSymlinks(repoRoot, home string, info platform.Info) []doctorCheck {
	if repoRoot == "" {
		return []doctorCheck{{
			Category: "symlinks", Name: "broken", Status: checkFail,
			Detail: "cannot locate the dotfiles repo root",
			Remedy: "set DOTFILES_ROOT to the dotfiles repo root",
		}}
	}
	if home == "" {
		return []doctorCheck{{
			Category: "symlinks", Name: "broken", Status: checkFail,
			Detail: "cannot resolve the home directory ($HOME)",
			Remedy: "set HOME to your home directory",
		}}
	}

	eng := symlink.NewEngine(repoRoot, home)
	specs := symlink.DefaultTable()
	opts := symlink.Options{Validate: true}

	plan, err := eng.Plan(specs, info, opts)
	if err != nil {
		remedy := "resolve the symlink planning error"
		var mste *symlink.MissingSourceTreeError
		if errors.As(err, &mste) {
			remedy = "set DOTFILES_ROOT to the dotfiles repo root"
		}
		return []doctorCheck{{
			Category: "symlinks", Name: "broken", Status: checkFail,
			Detail: err.Error(), Remedy: remedy,
		}}
	}

	return []doctorCheck{
		symlinkBrokenCheck(eng, specs, info, opts),
		symlinkPendingCheck(plan),
		symlinkStaleCheck(eng),
	}
}

// symlinkStaleCheck lists stale links into the repo (orphaned: a non-table
// link whose target is missing or is a managed source linked from an old
// location; dangling: a table link whose source is gone; see FindStale) over
// the full table, so a disabled facet's links are never reported. Each stale
// link becomes an item carrying its reason and $HOME-relative dest. Stale links
// are an advisory warn, as is a scan error.
func symlinkStaleCheck(eng *symlink.Engine) doctorCheck {
	stale, err := eng.FindStale(symlink.DefaultTable())
	if err != nil {
		return doctorCheck{
			Category: "symlinks", Name: "stale", Status: checkWarn,
			Detail: err.Error(), Remedy: "resolve the stale-link scan error",
		}
	}
	if len(stale) == 0 {
		return doctorCheck{Category: "symlinks", Name: "stale", Status: checkOK, Detail: "no stale links"}
	}
	items := make([]doctorItem, 0, len(stale))
	for _, s := range stale {
		items = append(items, doctorItem{Kind: "symlink", Action: s.Reason, Dest: s.Dest})
	}
	return doctorCheck{
		Category: "symlinks", Name: "stale", Status: checkWarn,
		Detail: fmt.Sprintf("%d stale link(s) into the repo", len(stale)),
		Remedy: "run `dotf link --prune`",
		Items:  items,
	}
}

// symlinkBrokenCheck runs post-plan validation and maps a *BrokenLinkError (real
// files where symlinks are expected, or dangling links whose source exists) to a
// fail listing the offending dests. Any other validation error is also a fail; a
// clean validation is ok.
func symlinkBrokenCheck(eng *symlink.Engine, specs []symlink.LinkSpec, info platform.Info, opts symlink.Options) doctorCheck {
	_, err := eng.Validate(specs, info, opts)
	if err == nil {
		return doctorCheck{Category: "symlinks", Name: "broken", Status: checkOK, Detail: "no broken links"}
	}
	var ble *symlink.BrokenLinkError
	if errors.As(err, &ble) {
		return doctorCheck{
			Category: "symlinks", Name: "broken", Status: checkFail,
			Detail: fmt.Sprintf("%d dest(s) not a valid symlink to their source: %s",
				len(ble.Dests), strings.Join(ble.Dests, ", ")),
			Remedy: "run `dotf link` (real files are backed up first)",
		}
	}
	return doctorCheck{
		Category: "symlinks", Name: "broken", Status: checkFail,
		Detail: err.Error(), Remedy: "resolve the symlink validation error",
	}
}

// symlinkPendingCheck collects the plan's create/repoint actions — links that
// are declared but not yet present or point elsewhere — which Validate (a check
// for real-file blockers and dangling/looping links whose source exists) does
// not flag for missing dests or wrong-target links. Each pending action becomes
// an item carrying the operation and the $HOME-relative dest (plus repo-relative
// src), the same Spec.Dest/Spec.Src vocabulary `dotf link` prints. Any pending link is an
// advisory warn; the plan's phase-stable action order is preserved.
func symlinkPendingCheck(plan *symlink.Plan) doctorCheck {
	var items []doctorItem
	for _, a := range plan.Actions {
		switch bucket := actionBucket(a); bucket {
		case "create", "repoint":
			items = append(items, doctorItem{
				Kind:   "symlink",
				Action: bucket,
				Dest:   a.Spec.Dest,
				Src:    a.Spec.Src,
			})
		}
	}
	if len(items) > 0 {
		return doctorCheck{
			Category: "symlinks", Name: "pending", Status: checkWarn,
			Detail: fmt.Sprintf("%d link(s) not yet created", len(items)),
			Remedy: "run `dotf link`",
			Items:  items,
		}
	}
	return doctorCheck{Category: "symlinks", Name: "pending", Status: checkOK, Detail: "all managed links present"}
}

// checkSysconfig maps the OS's sysconfig support level to a check: an
// unsupported OS fails, Windows (deferred) warns, and a supported OS is ok.
func checkSysconfig(info platform.Info) doctorCheck {
	switch sysconfig.Support(info) {
	case sysconfig.SupportUnsupported:
		return doctorCheck{
			Category: "sysconfig", Name: "support", Status: checkFail,
			Detail: "sysconfig is unsupported on this OS",
			Remedy: "dotf sysconfig targets macOS and the Linux families",
		}
	case sysconfig.SupportDeferred:
		return doctorCheck{
			Category: "sysconfig", Name: "support", Status: checkWarn,
			Detail: "sysconfig is deferred on Windows",
			Remedy: "sysconfig support for Windows is not yet implemented",
		}
	default:
		return doctorCheck{Category: "sysconfig", Name: "support", Status: checkOK, Detail: "sysconfig supported on this OS"}
	}
}

// checkBackups reports how many symlink backup dirs (~/.config/
// dotfiles_backup_*) exist and their total size. More than
// symlink.DefaultBackupKeep is an advisory warn pointing at `dotf backup
// prune`; any count up to that (zero included) is ok. An unreadable backup
// root is a warn, never a fail: backups are a convenience, not a fault.
func checkBackups(home string) doctorCheck {
	c := doctorCheck{Category: "backups", Name: "count", Status: checkOK}
	if home == "" {
		c.Status = checkWarn
		c.Detail = "could not resolve $HOME to list backups"
		c.Remedy = "set HOME"
		return c
	}
	sets, err := symlink.ListBackups(home)
	if err != nil {
		c.Status = checkWarn
		c.Detail = fmt.Sprintf("could not list backups: %v", err)
		c.Remedy = "check permissions on " + symlink.BackupRoot(home)
		return c
	}
	var size int64
	for _, s := range sets {
		size += s.Size
	}
	c.Detail = backupSummary(len(sets), size)
	if len(sets) > symlink.DefaultBackupKeep {
		c.Status = checkWarn
		c.Remedy = fmt.Sprintf("dotf backup prune (keeps the newest %d)", symlink.DefaultBackupKeep)
	}
	return c
}

// checkSELinux reports the host's SELinux mode as an INFORMATIONAL row. It is
// never a fail and never a warn: dotf's current write surface (symlinks under
// $HOME, sysconfig writes into existing inodes, plasma's same-directory atomic
// rename) all inherit or preserve context, so no mode is a fault. The row
// exists so a provisioning session can record the mode before and after a
// mutation and notice a flip. Hosts without getenforce (macOS, most
// non-RedHat Linux) emit no row at all.
func checkSELinux(r exec.Runner, info platform.Info) []doctorCheck {
	if !platform.IsLinuxFamily(info.OS) {
		return nil
	}
	if _, err := r.LookPath("getenforce"); err != nil {
		return nil
	}
	mode := "unknown"
	if out, err := r.Output("getenforce"); err == nil {
		if v := strings.TrimSpace(string(out)); v != "" {
			mode = strings.ToLower(v)
		}
	}
	return []doctorCheck{{
		Category: "selinux", Name: "mode", Status: checkOK,
		Detail: "selinux=" + mode,
	}}
}

// manifestFailCheck records an unresolvable manifest as a failing check so the
// rest of the host is still diagnosed instead of aborting the run.
func manifestFailCheck(err error) doctorCheck {
	return doctorCheck{
		Category: "manifest", Name: "load", Status: checkFail,
		Detail: err.Error(), Remedy: "fix the manifest path or TOML syntax",
	}
}

// summarizeChecks tallies the verdicts into a doctorSummary.
func summarizeChecks(checks []doctorCheck) doctorSummary {
	sum := doctorSummary{Total: len(checks)}
	for _, c := range checks {
		switch c.Status {
		case checkOK:
			sum.OK++
		case checkWarn:
			sum.Warn++
		case checkFail:
			sum.Fail++
		}
	}
	return sum
}

// doctorExitCode returns 1 when any check failed and 0 otherwise. Warnings are
// advisory (an un-provisioned host is not a fault) and never fail the run,
// mirroring how diff exits non-zero only on real drift.
func doctorExitCode(sum doctorSummary) int {
	if sum.Fail > 0 {
		return 1
	}
	return 0
}

// printDoctorText writes the sorted greppable "check.CATEGORY.NAME=STATUS" lines
// plus a final "doctor.total=.. doctor.ok=.. .." summary line to stdout in a
// single checked write, matching status.go/link.go's output discipline.
func printDoctorText(stdout io.Writer, checks []doctorCheck, sum doctorSummary) error {
	lines := make([]string, 0, len(checks)+1)
	for _, c := range checks {
		lines = append(lines, fmt.Sprintf("check.%s.%s=%s", c.Category, c.Name, c.Status))
	}
	lines = append(lines, fmt.Sprintf("doctor.total=%d doctor.ok=%d doctor.warn=%d doctor.fail=%d",
		sum.Total, sum.OK, sum.Warn, sum.Fail))

	_, err := io.WriteString(stdout, strings.Join(lines, "\n")+"\n")
	return err
}

// printDoctorRemedies writes one human "[status] category.name: detail; remedy"
// diagnostic per check to stderr, followed by one indented "  - <item>" line for
// each itemized finding the check carries (missing packages, pending links), plus
// a final summary line — mirroring printLinkSummary's stderr diagnostics style.
// By default only non-ok checks are printed; with verbose, ok checks print their
// evidence too (the platform tuple, prerequisite paths, the applicable-package
// tally, symlink health, sysconfig support level). Write failures on stderr are
// unactionable and discarded.
func printDoctorRemedies(stderr io.Writer, checks []doctorCheck, sum doctorSummary, verbose bool) {
	for _, c := range checks {
		if c.Status == checkOK && !verbose {
			continue
		}
		line := fmt.Sprintf("%s: [%s] %s.%s: %s", progName, c.Status, c.Category, c.Name, c.Detail)
		if c.Remedy != "" {
			line += "; " + c.Remedy
		}
		_, _ = fmt.Fprintln(stderr, line)
		for _, it := range c.Items {
			_, _ = fmt.Fprintf(stderr, "  - %s\n", doctorItemLine(it))
		}
	}
	_, _ = fmt.Fprintf(stderr, "%s: doctor summary: total=%d ok=%d warn=%d fail=%d\n",
		progName, sum.Total, sum.OK, sum.Warn, sum.Fail)
}

// doctorItemLine renders one itemized finding as a compact human string for the
// indented stderr breakdown: a package as "name (backend=..., class=...)" (the
// class clause dropped when the effective class is empty), a pending symlink as
// "action ~/dest" (dest is $HOME-relative, so the ~/ prefix is exact), and a
// drifted plasma file as "drift path" — its dest is config-dir-relative, which
// no fixed prefix can spell, so it is printed bare. An unknown kind falls back
// to its Name so no finding is ever printed blank.
func doctorItemLine(it doctorItem) string {
	switch it.Kind {
	case "package":
		if it.Class != "" {
			return fmt.Sprintf("%s (backend=%s, class=%s)", it.Name, it.Backend, it.Class)
		}
		return fmt.Sprintf("%s (backend=%s)", it.Name, it.Backend)
	case "symlink":
		return fmt.Sprintf("%s ~/%s", it.Action, it.Dest)
	case "plasma":
		return fmt.Sprintf("%s %s", it.Action, it.Dest)
	default:
		return it.Name
	}
}
