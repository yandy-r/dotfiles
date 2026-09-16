package debtap

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// This file mirrors debtap's OWN start-up gate so `dotf deb` can diagnose a
// refusal before debtap prints one, and offer the exact fix.
//
// debtap 3.6.3 aborts unless all five of these hold (its line 154):
//
//	ls /var/cache/pkgfile | grep -E '*.files?(.[[:digit:]]{3})'
//	ls /var/cache/debtap/*-packages-files
//	-e /var/cache/debtap/extended-base-packages-list
//	-e /var/cache/debtap/aur-packages
//	-e /var/cache/debtap/virtual-packages
//
// and emits ONE message for all five: "You must run at least once debtap -u
// with root privileges". That is actively misleading for the first condition,
// which no amount of syncing can satisfy (see pkgfileGateRe). Evaluating the
// conditions separately is the only way to say which actually failed.

// DefaultDBDir is debtap's translation-database directory. debtap cannot map
// Debian dependencies onto Arch package names without it.
const DefaultDBDir = "/var/cache/debtap"

// DefaultPkgfileDir is pkgfile's repository file cache, which debtap gates on.
const DefaultPkgfileDir = "/var/cache/pkgfile"

// pkgfileGateRe is debtap's own pkgfile-cache pattern, translated to Go.
//
// debtap writes it as the ERE `*.files?(.[[:digit:]]{3})`. The leading `*` is
// malformed — GNU grep warns "* at start of expression" and ignores it — so
// what actually runs is the rest: any character, "file", an OPTIONAL "s", any
// character, then exactly three digits. The author near-certainly meant
// `\.files(\.[0-9]{3})?`, with the suffix optional; writing `files?(…)` instead
// made the "s" optional and the digit group mandatory. That inverted `?` is the
// whole bug, and it is why a fully synced pkgfile cache still fails the gate.
var pkgfileGateRe = regexp.MustCompile(`.files?(.[0-9]{3})`)

// PkgfileMarkerName is the marker file that satisfies debtap's pkgfile gate.
//
// It matches pkgfileGateRe, and deliberately matches no repository name, so
// pkgfile itself never reads it: pkgfile only looks for "<repo>.files" for the
// repositories listed in pacman.conf.
const PkgfileMarkerName = "debtap-gate.files.000"

// Remedy is the command that fixes a failed precondition, with enough context
// for a user to decide whether to run it. Argv is passed to exec.Runner as a
// slice, never through a shell.
type Remedy struct {
	// Detail explains why the precondition failed and what the command will
	// and will not touch. Rendered as an indented block above the prompt.
	Detail string
	// Argv is the fix, argv-style. Empty means the problem is not
	// auto-fixable and Detail must tell the user what to do instead.
	Argv []string
	// Undo, when non-empty, is the command that reverses Argv. Only set for
	// remedies that leave something behind worth removing later.
	Undo []string
}

// PreflightError is a failed precondition. When it carries a Remedy the
// command layer can offer to run it.
type PreflightError struct {
	// Problem is the one-line diagnosis.
	Problem string
	Remedy  Remedy
}

func (e *PreflightError) Error() string { return e.Problem }

// Fixable reports whether this failure has a command that resolves it.
func (e *PreflightError) Fixable() bool { return len(e.Remedy.Argv) > 0 }

// Preflight verifies the host can run a conversion at all: a pacman-based
// distro, the tools on PATH, and every condition debtap itself gates on.
//
// It is safe to call repeatedly: the command layer re-runs it after applying a
// remedy to confirm the fix actually took.
func (c *Converter) Preflight(info platform.Info) error {
	if info.PkgMgr != platform.PkgMgrPacman {
		return &UnsupportedOSError{OS: info.OS, PkgMgr: info.PkgMgr}
	}
	if err := c.checkTools(); err != nil {
		return err
	}
	return c.checkDebtapGate()
}

// checkTools verifies the three external commands the pipeline drives.
func (c *Converter) checkTools() error {
	if _, err := c.r.LookPath("pacman"); err != nil {
		// Nothing here can install pacman onto a host that lacks it.
		return &PreflightError{Problem: "pacman is not on PATH; this command requires a pacman-based host"}
	}
	// bsdtar reads both the .deb's control archive and the built package's
	// .PKGINFO. It ships with libarchive, a hard dependency of pacman itself,
	// so this should never fire on a healthy host.
	if _, err := c.r.LookPath("bsdtar"); err != nil {
		return &PreflightError{
			Problem: "bsdtar is not installed (it reads the .deb's control archive)",
			Remedy: Remedy{
				Detail: "bsdtar ships with libarchive. Installing it adds one package and\n" +
					"changes no configuration.",
				Argv: []string{"sudo", "pacman", "-S", "--needed", "libarchive"},
			},
		}
	}
	if _, err := c.r.LookPath("debtap"); err != nil {
		return c.debtapMissingErr()
	}
	if _, err := c.r.LookPath("pkgfile"); err != nil {
		return &PreflightError{
			Problem: "pkgfile is not installed (debtap uses it to resolve library dependencies)",
			Remedy: Remedy{
				Detail: "Installing it adds one package and changes no configuration.\n" +
					"Its database still needs a separate sync afterwards.",
				Argv: []string{"sudo", "pacman", "-S", "--needed", "pkgfile"},
			},
		}
	}
	return nil
}

// debtapMissingErr offers to install debtap through whichever AUR helper is
// present. debtap is AUR-only, so there is no plain pacman path.
func (c *Converter) debtapMissingErr() *PreflightError {
	for _, helper := range aurHelpers {
		if _, err := c.r.LookPath(helper); err != nil {
			continue
		}
		return &PreflightError{
			Problem: "debtap is not installed (it is the conversion engine, and is AUR-only)",
			Remedy: Remedy{
				Detail: fmt.Sprintf(
					"Installs the AUR package `debtap` via %s, which will show you the\n"+
						"PKGBUILD before building. This is the same thing `dotf install debtap`\n"+
						"does. Its database still needs a separate sync afterwards.", helper),
				Argv: []string{helper, "-S", "--needed", "debtap"},
			},
		}
	}
	return &PreflightError{
		Problem: "debtap is not installed and no AUR helper (paru, yay) was found to install it",
		Remedy: Remedy{
			Detail: "Install an AUR helper first, then run `dotf install debtap`.",
		},
	}
}

// checkDebtapGate evaluates debtap's five start-up conditions in its own order,
// returning a specific diagnosis for whichever fails first.
func (c *Converter) checkDebtapGate() error {
	if err := c.checkPkgfileGate(); err != nil {
		return err
	}

	// The four debtap-database conditions share one remedy, so they share one
	// message — but it names the missing file, unlike debtap's own.
	globbed, err := filepath.Glob(filepath.Join(c.dbDir, "*-packages-files"))
	if err != nil || len(globbed) == 0 {
		return c.debtapSyncErr("*-packages-files")
	}
	for _, name := range []string{
		"extended-base-packages-list",
		"aur-packages",
		"virtual-packages",
	} {
		if _, err := os.Stat(filepath.Join(c.dbDir, name)); err != nil {
			return c.debtapSyncErr(name)
		}
	}
	return nil
}

// debtapSyncErr builds the remedy for a genuinely missing debtap database file.
func (c *Converter) debtapSyncErr(missing string) *PreflightError {
	return &PreflightError{
		Problem: fmt.Sprintf("debtap's translation database in %s is incomplete (%s is missing)",
			c.dbDir, missing),
		Remedy: Remedy{
			Detail: "Downloads debtap's Debian/Ubuntu package indexes into its own cache\n" +
				"directory. It installs nothing and changes no system configuration,\n" +
				"but it downloads a few hundred MB and can take a couple of minutes.",
			Argv: []string{"sudo", "debtap", "-u"},
		},
	}
}

// checkPkgfileGate evaluates debtap's first condition and distinguishes its two
// very different failure modes: pkgfile was never synced (fixable by syncing),
// versus pkgfile IS synced but debtap's pattern cannot match it (a debtap bug
// that syncing will never resolve).
func (c *Converter) checkPkgfileGate() error {
	entries, err := os.ReadDir(c.pkgfileDir)
	if err != nil || len(entries) == 0 {
		return &PreflightError{
			Problem: fmt.Sprintf("pkgfile's cache in %s is missing or empty", c.pkgfileDir),
			Remedy: Remedy{
				Detail: "Downloads the repository file lists debtap queries to map libraries\n" +
					"onto packages. It installs nothing and changes no configuration.",
				Argv: []string{"sudo", "pkgfile", "-u"},
			},
		}
	}
	for _, e := range entries {
		if pkgfileGateRe.MatchString(e.Name()) {
			return nil
		}
	}

	marker := filepath.Join(c.pkgfileDir, PkgfileMarkerName)
	return &PreflightError{
		Problem: fmt.Sprintf(
			"debtap will refuse to run: its pkgfile check requires a cache entry matching "+
				"`*.files?(.[[:digit:]]{3})`, but pkgfile writes plain \"<repo>.files\" names "+
				"(%d entries present in %s)", len(entries), c.pkgfileDir),
		Remedy: Remedy{
			Detail: "This is a bug in debtap, not a missing sync — re-running `sudo debtap -u`\n" +
				"cannot fix it. debtap's pattern demands a three-digit suffix\n" +
				"(\"core.files.000\") that pkgfile has no reason to write, so the check can\n" +
				"never pass. debtap only ever uses pkgfile as a command, never by reading\n" +
				"this directory, so the capability itself is fine.\n" +
				"\n" +
				"The fix is one EMPTY marker file whose name satisfies that pattern. It\n" +
				"matches no repository name, so pkgfile ignores it. Nothing is installed,\n" +
				"no configuration is touched, and no existing file is modified. sudo is\n" +
				"needed only because the directory is root-owned.",
			Argv: []string{"sudo", "touch", marker},
			Undo: []string{"sudo", "rm", marker},
		},
	}
}

// ApplyRemedy runs a remedy's command. Every argument is passed as an argv
// element, never through a shell.
func (c *Converter) ApplyRemedy(rem Remedy) error {
	if len(rem.Argv) == 0 {
		return fmt.Errorf("no automatic fix available")
	}
	if err := c.r.Run(rem.Argv[0], rem.Argv[1:]...); err != nil {
		return fmt.Errorf("%s: %w", rem.Argv[0], err)
	}
	return nil
}
