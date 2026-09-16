package debtap

import (
	"errors"
	"fmt"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// This file owns the VERIFICATION side of the pipeline: whether a native
// package already exists, what the built artifact actually contains, whether
// its translated dependencies hold, and what pacman ended up owning.
//
// Every pacman query here is chosen for being locale independent. `pacman -Si`
// and `pacman -Qi` render translated field labels ("Nombre", "Version") and are
// unparseable on a non-English host; `-Ssq`, `-T`, `-Qq` and `-Qql` emit bare
// names and paths on every locale, and .PKGINFO is a plain key=value file.

// Precedence outcomes reported by Converter.Precedence.
const (
	PrecedenceNone = "none"
	PrecedenceRepo = "repo"
	PrecedenceAUR  = "aur"
)

// aurHelpers are the AUR search front ends tried, in order of preference.
var aurHelpers = []string{"paru", "yay"}

// safeOperand reports whether s may be passed to pacman as a positional
// operand.
//
// This is an argument-injection guard, not cosmetics. Package and dependency
// names reaching this file come from the .deb's own control data and the built
// package's .PKGINFO — both attacker-controlled — and pacman parses any operand
// beginning with "-" as a FLAG. A crafted `depend = --dbpath=/tmp/evil` would
// otherwise point the dependency check at a database the attacker supplied and
// make it report that everything is satisfied.
//
// pacman does honor a "--" end-of-options terminator (it uses getopt_long), so
// that is an available alternative. Filtering is preferred anyway: no legitimate
// package name begins with "-", so rejecting one is both safe and more honest
// than silently reinterpreting it as a package that cannot exist — CheckDeps
// reports it as broken instead of hiding it.
func safeOperand(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-")
}

// isNoMatch reports whether err is a search's ordinary "nothing matched" exit
// status (1) rather than a genuine failure.
func isNoMatch(err error) bool {
	var ee *osexec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 1
	}
	return false
}

// isDepTestUnsatisfied reports whether err is `pacman -T`'s "some dependencies
// are unsatisfied" exit status (127) rather than a genuine failure.
func isDepTestUnsatisfied(err error) bool {
	var ee *osexec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 127
	}
	return false
}

// PkgInfo is the subset of a built package's .PKGINFO this package reports.
type PkgInfo struct {
	Name    string
	Version string
	Size    string
	Depends []string
}

// Precedence reports whether an Arch package named name already exists, and
// where. Converting a .deb when a native package is available is debtap's worst
// outcome: the converted package shadows the real one and never receives
// updates. The command layer treats a repo hit as a hard gate and an AUR hit as
// advisory, because AUR naming is loose enough that blocking on it would be
// noise.
// A query failure is returned as an error rather than being reported as "no
// native package exists". Collapsing the two would silently disable the gate on
// exactly the hosts most likely to need it — a fresh container that never ran
// `pacman -Sy` — and the user would never learn the check had not run.
func (c *Converter) Precedence(name string) (string, error) {
	// searchExact anchors the name inside a regex, so it can never itself
	// become a flag; an unusable name simply has no native counterpart.
	if !safeOperand(name) {
		return PrecedenceNone, nil
	}
	found, err := c.searchExact("pacman", name)
	if err != nil {
		return "", err
	}
	if found {
		return PrecedenceRepo, nil
	}
	for _, helper := range aurHelpers {
		if _, err := c.r.LookPath(helper); err != nil {
			continue
		}
		// An AUR helper failure is deliberately not propagated: the repo check
		// above is the actual gate, and this only decides whether to print an
		// advisory warning. A missing AUR index must not block a conversion.
		if found, _ := c.searchExact(helper, name); found {
			return PrecedenceAUR, nil
		}
		// One helper is authoritative for the AUR; trying a second would only
		// repeat the same query against the same index.
		break
	}
	return PrecedenceNone, nil
}

// searchExact reports whether tool's package search yields a package named
// exactly name.
//
// The name is regex-quoted because Debian package names legitimately contain
// regex metacharacters — "libstdc++6" and "python3.11" would otherwise match
// far more than intended. Because `-Ss` searches descriptions as well as names,
// results are filtered back down to exact name matches, so a package whose
// description happens to match cannot masquerade as a hit.
// It distinguishes "queried, nothing matched" from "the query failed". pacman
// exits 1 with empty output for a legitimate no-match, but it ALSO fails when
// the sync database was never populated or is locked — and treating those as
// "no native package exists" would turn the precedence gate into a no-op
// without telling anyone.
func (c *Converter) searchExact(tool, name string) (bool, error) {
	out, err := c.r.Output(tool, "-Ssq", "^"+regexp.QuoteMeta(name)+"$")
	if err != nil && !isNoMatch(err) {
		return false, fmt.Errorf("%s search for %q failed: %w", tool, name, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == name {
			return true, nil
		}
	}
	return false, nil
}

// Inspect reads .PKGINFO straight out of the built package, rather than parsing
// `pacman -Qip`, whose field labels are translated.
func (c *Converter) Inspect(artifact string) (PkgInfo, error) {
	out, err := c.r.Output("bsdtar", "-xOf", artifact, ".PKGINFO")
	if err != nil {
		return PkgInfo{}, fmt.Errorf("read .PKGINFO from %s: %w", filepath.Base(artifact), err)
	}
	return parsePkgInfo(string(out)), nil
}

// parsePkgInfo reads pkgname, pkgver, size and every depend entry from a
// .PKGINFO body. Unknown keys are ignored so a future pacman field cannot break
// parsing.
func parsePkgInfo(s string) PkgInfo {
	var pi PkgInfo
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "pkgname":
			pi.Name = val
		case "pkgver":
			pi.Version = val
		case "size":
			pi.Size = val
		case "depend":
			pi.Depends = append(pi.Depends, val)
		}
	}
	return pi
}

// CheckDeps classifies depends into those not satisfied on this host, and the
// subset of those that no repository can supply either — the genuinely broken
// dependencies debtap's approximate Debian-to-Arch translation produced.
//
// `pacman -T` is used rather than a name search because it is the only check
// that is both provider-aware and version-aware: it knows that "sh" is provided
// by bash and that "gtk3>=3.24" is a constraint, neither of which a name lookup
// can express.
//
// The result is advisory and never blocks. `pacman -U` is the authority on
// installability and produces a better diagnostic than a pre-flight guess, and
// the unresolvable set can overreport: a dependency satisfied by a repo package
// that *provides* it under another name is not found by an exact name search.
// A failure of the check itself is returned as an error, never reported as
// "zero problems found" — a dependency check that did not run must not look
// like one that passed.
func (c *Converter) CheckDeps(depends []string) (unsatisfied, unresolvable []string, err error) {
	// A dependency that cannot be a pacman operand is reported as both
	// unsatisfied and unresolvable WITHOUT being queried: no real package can
	// bear such a name, so the honest answer is "broken", and never putting it
	// in an argv closes the injection path.
	var query []string
	for _, dep := range depends {
		if safeOperand(dep) {
			query = append(query, dep)
			continue
		}
		unsatisfied = append(unsatisfied, dep)
		unresolvable = append(unresolvable, dep)
	}
	if len(query) == 0 {
		return unsatisfied, unresolvable, nil
	}

	// pacman -T exits 127 precisely when something is unsatisfied, which is the
	// case this function exists to detect, so that status is expected and its
	// stdout is the result. Any OTHER failure means the check did not run.
	out, terr := c.r.Output("pacman", append([]string{"-T"}, query...)...)
	if terr != nil && !isDepTestUnsatisfied(terr) {
		return nil, nil, fmt.Errorf("dependency check failed: %w", terr)
	}

	for _, line := range strings.Split(string(out), "\n") {
		dep := strings.TrimSpace(line)
		if dep == "" {
			continue
		}
		unsatisfied = append(unsatisfied, dep)
		found, serr := c.searchExact("pacman", depName(dep))
		if serr != nil {
			return nil, nil, serr
		}
		if !found {
			unresolvable = append(unresolvable, dep)
		}
	}
	return unsatisfied, unresolvable, nil
}

// depName strips a version constraint from a dependency spec, so "gtk3>=3.24"
// is searched for as "gtk3".
func depName(dep string) string {
	if i := strings.IndexAny(dep, "<>="); i >= 0 {
		return dep[:i]
	}
	return dep
}

// VerifyInstalled confirms pacman registered name, and counts the files it
// actually owns. The count comes from pacman's own database rather than the
// artifact's manifest, so it reflects what really landed on the system.
func (c *Converter) VerifyInstalled(name string) (bool, int) {
	// name is passed to pacman as a bare operand, so it must not be able to
	// masquerade as a flag (see safeOperand).
	if !safeOperand(name) {
		return false, 0
	}
	if _, err := c.r.Output("pacman", "-Qq", name); err != nil {
		return false, 0
	}

	out, err := c.r.Output("pacman", "-Qql", name)
	if err != nil {
		// Registered but unlistable is still installed; report it without a
		// file count rather than downgrading a real success to a failure.
		return true, 0
	}
	files := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) != "" {
			files++
		}
	}
	return true, files
}
