package selfupdate

import (
	"fmt"
	"strconv"
	"strings"
)

// normalize strips a single leading "v" from a version string so tags like
// "v0.1.0" and bare GoReleaser versions like "0.1.0" compare identically. It
// leaves everything else untouched (including "dev", which stays "dev").
func normalize(v string) string {
	return strings.TrimPrefix(v, "v")
}

// parseVersion splits a normalized "X.Y.Z[-prerelease][+build]" string into its
// numeric core and prerelease identifier. Build metadata is discarded (it does
// not affect precedence). A non-numeric or malformed core (e.g. "dev") is a
// typed error, which the caller treats as an unparseable/dev build.
func parseVersion(v string) (major, minor, patch int, prerelease string, err error) {
	v = normalize(v)

	core := v
	if i := strings.IndexByte(core, '+'); i >= 0 { // drop build metadata
		core = core[:i]
	}
	if i := strings.IndexByte(core, '-'); i >= 0 { // split off prerelease
		prerelease = core[i+1:]
		core = core[:i]
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, "", fmt.Errorf("invalid version %q: want major.minor.patch", v)
	}

	nums := make([]int, 3)
	for i, p := range parts {
		n, convErr := strconv.Atoi(p)
		if convErr != nil || n < 0 {
			return 0, 0, 0, "", fmt.Errorf("invalid version %q: non-numeric component %q", v, p)
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], prerelease, nil
}

// compareVersions returns -1, 0, or +1 as a is less than, equal to, or greater
// than b, using semantic-version precedence: numeric core first, then the
// prerelease rule that a version WITH a prerelease ranks below the same core
// WITHOUT one. An unparseable operand is a typed error.
func compareVersions(a, b string) (int, error) {
	amaj, amin, apatch, apre, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	bmaj, bmin, bpatch, bpre, err := parseVersion(b)
	if err != nil {
		return 0, err
	}

	for _, pair := range [][2]int{{amaj, bmaj}, {amin, bmin}, {apatch, bpatch}} {
		if c := cmpInt(pair[0], pair[1]); c != 0 {
			return c, nil
		}
	}
	return comparePrerelease(apre, bpre), nil
}

// comparePrerelease compares two SemVer prerelease strings. An empty prerelease
// (a stable release) outranks any non-empty one. Otherwise dot-separated
// identifiers are compared left to right: numeric identifiers compare
// numerically and always rank below alphanumeric ones; a longer identifier set
// outranks a shorter prefix-equal one.
func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}

	ai, bi := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		if c := compareIdent(ai[i], bi[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(ai), len(bi))
}

// compareIdent compares one prerelease identifier per SemVer rules: two numeric
// identifiers compare numerically, a numeric identifier ranks below an
// alphanumeric one, and two alphanumeric identifiers compare lexically.
func compareIdent(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return cmpInt(an, bn)
	case aErr == nil:
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
