package plasma

import (
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Synthetic stand-ins. Every identifier this feature writes down — in code, in
// tests, in fixtures and in the exported `plasma/` tree — uses one of these
// forms. They are shaped exactly like the real thing, which is all the strip
// rules and the codec ever look at, so nothing is weakened by using them.
const (
	// syntheticUUIDPrefix is the first 24 bytes of every UUID this feature is
	// allowed to contain: 00000000-0000-4000-8000-0000000000NN, numbered in
	// sorted order so group ordering in the fixtures is preserved.
	syntheticUUIDPrefix = "00000000-0000-4000-8000-"

	// syntheticToken stands in for an application-minted, per-installation
	// shortcut token (kglobalshortcutsrc mints one 32-hex-digit token per
	// install and per app).
	syntheticToken = "DEADBEEFDEADBEEFDEADBEEFDEADBEEF"
)

// scrubRoots are the repository-relative directories this feature contributes,
// and the exact scope of TestPlasmaTree_NoMachineIdentifiers. See that test's
// doc comment for why the boundary sits here and not at the repository root.
var scrubRoots = []string{"internal/plasma", "plasma"}

// scrubExceptions grants a named file the right to contain one specific
// matched literal. It is deliberately tiny and deliberately per-file: an
// exception is a claim that THIS occurrence is load-bearing, not a blanket
// amnesty for the pattern.
var scrubExceptions = map[string][]string{
	// The Family C guard's ban list has to spell the needle it hunts for.
	"internal/plasma/codec_fixture_test.go": {"/home/yandy"},
	// No exception: export_test.go's scanPrivacy negative inputs use a
	// synthetic account, so the guard covers that file unaided.
	// This file's own exception table, above, names those literals.
	"internal/plasma/scrub_guard_test.go": {"/home/yandy", "/Users/yandy"},
}

// identifierRule is one class of machine identifier, its detector, and the
// synthetic forms that are allowed to match it.
type identifierRule struct {
	name string
	// what a hit means, phrased for the failure message.
	what string
	re   *regexp.Regexp
	// allow reports whether a match is a recognised synthetic placeholder.
	// nil means no match is ever acceptable.
	allow func(match string) bool
}

var identifierRules = []identifierRule{
	{
		name: "uuid",
		what: "a UUID (KDE mints them per desktop, per activity and per monitor EDID)",
		re:   regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`),
		allow: func(m string) bool {
			return strings.HasPrefix(m, syntheticUUIDPrefix)
		},
	},
	{
		name: "home-path",
		what: "an absolute home directory, which names a real account",
		re:   regexp.MustCompile(`/(?:home|Users)/[A-Za-z0-9._][A-Za-z0-9._-]*`),
		allow: func(m string) bool {
			return isSyntheticUser(m[strings.LastIndexByte(m, '/')+1:])
		},
	},
	{
		name: "ipv4",
		// A four-part version string has the same shape and will trip this.
		// That is the intended trade: a false positive costs one
		// scrubExceptions line, a miss publishes a network topology.
		what:  "an IPv4 address, which maps out a private network",
		re:    regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
		allow: isNonIdentifyingIPv4,
	},
	{
		name: "mac",
		what: "a MAC address, which identifies one physical adapter forever",
		re:   regexp.MustCompile(`\b(?:[0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}\b`),
		allow: func(m string) bool {
			return strings.EqualFold(m, "00:00:00:00:00:00")
		},
	},
	{
		name: "email",
		what: "an email address",
		re:   regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}\b`),
		allow: func(m string) bool {
			return isReservedDomain(m[strings.LastIndexByte(m, '@')+1:])
		},
	},
	{
		name: "hex-token",
		what: "a long uppercase hex token, the shape of a per-install secret or hash",
		re:   regexp.MustCompile(`\b[0-9A-F]{16,}\b`),
		allow: func(m string) bool {
			return m == syntheticToken
		},
	},
}

// isSyntheticUser reports whether name is one of the placeholder account names
// the fixtures and tests are allowed to use in an absolute path.
func isSyntheticUser(name string) bool {
	switch name {
	case "testuser", "someone", "user", "u":
		return true
	}
	return false
}

// isNonIdentifyingIPv4 reports whether addr is a literal that identifies no
// real host: the unspecified and broadcast addresses, loopback, and the three
// RFC 5737 documentation blocks reserved precisely for examples.
func isNonIdentifyingIPv4(addr string) bool {
	ip, err := netip.ParseAddr(addr)
	if err != nil || !ip.Is4() {
		return true // not actually an address; a version string or similar
	}
	if ip.IsUnspecified() || ip.IsLoopback() || addr == "255.255.255.255" {
		return true
	}
	for _, block := range []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"} {
		if netip.MustParsePrefix(block).Contains(ip) {
			return true
		}
	}
	return false
}

// isReservedDomain reports whether domain is one of the RFC 2606 names set
// aside for documentation.
func isReservedDomain(domain string) bool {
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	switch domain {
	case "example.com", "example.net", "example.org":
		return true
	}
	for _, tld := range []string{".example", ".invalid", ".test", ".localhost"} {
		if strings.HasSuffix(domain, tld) {
			return true
		}
	}
	return false
}

// TestPlasmaTree_NoMachineIdentifiers is a committed-data guard covering
// everything the plasma feature contributes: internal/plasma (source, tests and
// testdata alike) and the exported plasma/ tree.
//
// Why that scope, and not the repository:
//
// TestCodec_FamilyC_IsScrubbed already guards testdata/kconfig/real, but only
// that directory — which is exactly how three live UUIDs from the maintainer's
// kwinrc, kwinoutputconfig.json and kactivitymanagerdrc reached
// table_glob_test.go, a file the scrubber never looks at. A guard that stops at
// the fixture corpus does not protect a feature whose entire purpose is
// stripping machine identifiers out of KDE config.
//
// Widening it to the whole repository is the wrong correction. main already
// carries absolute /home paths in eighteen files, a NAS host name in four and a
// private address in scripts/createDnsRecords.ps1 — pre-existing, intentional,
// already-published content that this branch neither introduced nor is entitled
// to rewrite. A repository-wide scan would fail on all of it on day one and be
// silenced within the week, which is worse than no guard. The boundary is
// therefore "what this feature owns": the two directories in scrubRoots. Extend
// it when the feature grows a directory, not to police the rest of the tree.
func TestPlasmaTree_NoMachineIdentifiers(t *testing.T) {
	root := repoRoot(t)
	scanned := 0
	for _, dir := range scrubRoots {
		abs := filepath.Join(root, filepath.FromSlash(dir))
		err := filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scanned++
			scanForIdentifiers(t, rel, string(data))
			return nil
		})
		if err != nil {
			t.Fatalf("WalkDir(%q) error = %v", abs, err)
		}
	}
	if scanned == 0 {
		t.Fatalf("scanned no files under %v; the guard is not covering anything", scrubRoots)
	}
}

// scanForIdentifiers reports every rule hit in content that is neither a
// recognised synthetic form nor listed in scrubExceptions for rel.
func scanForIdentifiers(t *testing.T, rel, content string) {
	t.Helper()
	for _, rule := range identifierRules {
		for _, span := range rule.re.FindAllStringIndex(content, -1) {
			match := content[span[0]:span[1]]
			if rule.allow != nil && rule.allow(match) {
				continue
			}
			if isExcepted(rel, match) {
				continue
			}
			t.Errorf("%s:%d: %s leaks %q — %s. Replace it with the synthetic form "+
				"(see the constants at the top of scrub_guard_test.go), or, if this exact "+
				"occurrence is load-bearing, add it to scrubExceptions with the reason",
				rel, lineOf(content, span[0]), rule.name, match, rule.what)
		}
	}
}

// isExcepted reports whether match is one of the literals rel is allowed to
// contain.
func isExcepted(rel, match string) bool {
	for _, allowed := range scrubExceptions[rel] {
		if match == allowed {
			return true
		}
	}
	return false
}

// lineOf converts a byte offset into a 1-based line number.
func lineOf(content string, offset int) int {
	return 1 + strings.Count(content[:offset], "\n")
}

// repoRoot walks up from the test's working directory to the directory holding
// go.mod. The guard covers a tree outside this package, so it cannot rely on a
// relative path from the package directory.
func repoRoot(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %q; cannot locate the repository root", dir)
		}
		dir = parent
	}
}
