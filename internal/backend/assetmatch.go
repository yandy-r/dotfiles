package backend

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// matchPrefs tunes the asset matcher from a package's manifest.AssetMatch: the
// preferred libc variant (empty or "gnu" prefers glibc, "musl" flips it), a set
// of case-insensitive exclude substrings, and the repo/binary name used for the
// name-closeness tie-break.
type matchPrefs struct {
	libc    string
	exclude []string
	name    string
	// allowExts exempts these suffixes (e.g. ".rpm") from nonInstallableExts,
	// for backends that install package files; nil keeps the default.
	allowExts []string
}

// AssetMatchError is returned when the auto-detector cannot pick exactly one
// installable asset for the host. Kind is "none" (no candidate carried the
// host's OS and arch tokens) or "ambiguous" (several survived scoring with an
// identical rank). Repo/Tag are filled in by the caller for a locating message;
// Candidates lists the ambiguous survivors so the user can pin `asset`/`match`.
type AssetMatchError struct {
	// Repo is the "owner/repo" slug, set by the caller.
	Repo string

	// Tag is the release tag, set by the caller.
	Tag string

	// Host is the "<release-os>/<arch>" the match was attempted for.
	Host string

	// Kind is "none" or "ambiguous".
	Kind string

	// Candidates lists the ambiguous survivors (empty for a "none" error).
	Candidates []string
}

// Error implements the error interface.
func (e *AssetMatchError) Error() string {
	loc := e.Repo
	if e.Tag != "" {
		loc = e.Repo + "@" + e.Tag
	}
	if e.Kind == "ambiguous" {
		return fmt.Sprintf("github-release: ambiguous asset for %s (host %s): %s; pin `asset` or narrow with `match`", loc, e.Host, strings.Join(e.Candidates, ", "))
	}
	return fmt.Sprintf("github-release: no installable asset for %s (host %s); pin `asset`", loc, e.Host)
}

// matchAsset picks the single release asset that best fits info from the list of
// asset filenames, honoring prefs. It is pure and network-free: tokenize each
// filename on [-_. ] (case-insensitively), keep assets that carry a host
// OS-alias token AND a host arch-alias token, reject assets carrying a foreign
// OS/arch token (whole delimited tokens only, so "arm" never disqualifies
// "arm64") or a non-installable extension/sidecar, then tie-break survivors by
// libc, archive-format rank, and name-closeness. Zero survivors yields
// *AssetMatchError{Kind:"none"}; a scoring tie yields *AssetMatchError{Kind:
// "ambiguous"} listing the candidates.
func matchAsset(assets []string, info platform.Info, prefs matchPrefs) (string, error) {
	osAliases := info.OS.ReleaseOSAliases()
	archAliases := info.Arch.ReleaseArchAliases()
	foreignOS := foreignOSTokens(info.OS)
	foreignArch := foreignArchTokens(info.Arch)
	host := fmt.Sprintf("%s/%s", info.OS.ReleaseOS(), info.Arch)

	var survivors []string
	for _, asset := range assets {
		if notInstallable(asset, prefs.allowExts) || excluded(asset, prefs.exclude) {
			continue
		}
		if !hasAnyToken(asset, osAliases) || !hasAnyToken(asset, archAliases) {
			continue
		}
		if hasAnyToken(asset, foreignOS) || hasAnyToken(asset, foreignArch) {
			continue
		}
		survivors = append(survivors, asset)
	}

	switch len(survivors) {
	case 0:
		return "", &AssetMatchError{Host: host, Kind: "none"}
	case 1:
		return survivors[0], nil
	default:
		return pickBest(survivors, prefs, host)
	}
}

// releaseArchExemplars is the closed set of canonical arches whose alias sets
// define the "foreign arch" vocabulary; releaseOSExemplars is the analogous set
// of one OS per distinct ReleaseOS token.
var (
	releaseArchExemplars = []platform.Arch{platform.ArchX86_64, platform.ArchARM64, platform.ArchARM}
	releaseOSExemplars   = []platform.OS{platform.OSMacOS, platform.OSLinux, platform.OSWindows}
)

// untargetedArchTokens are release-asset tokens for CPU families dotf has no
// canonical arch for. They are foreign on every supported host; without them a
// release missing its host asset could resolve to, say, an i386 or riscv64
// build instead of failing closed.
var untargetedArchTokens = []string{"i386", "i686", "ia32", "riscv64", "ppc64", "ppc64le", "s390x", "loong64"}

// foreignOSTokens is the list of OS-alias tokens belonging to every release OS
// other than the host's, used to reject cross-OS assets that happen to also
// carry the host arch token.
func foreignOSTokens(host platform.OS) []string {
	hostRelease := host.ReleaseOS()
	var out []string
	for _, o := range releaseOSExemplars {
		if o.ReleaseOS() == hostRelease {
			continue
		}
		out = append(out, o.ReleaseOSAliases()...)
	}
	return out
}

// foreignArchTokens is the list of arch tokens that mark an asset as built for a
// CPU other than the host's: the aliases of every other canonical arch plus
// untargetedArchTokens, minus the host's own aliases (so an unrecognized host
// arch such as riscv64 never rejects its own assets). Because tokens match on
// whole delimited boundaries, the 32-bit "arm" family never leaks into (and so
// never rejects) an "arm64" host.
func foreignArchTokens(host platform.Arch) []string {
	hostAliases := host.ReleaseArchAliases()
	var out []string
	for _, a := range releaseArchExemplars {
		if a == host {
			continue
		}
		out = append(out, a.ReleaseArchAliases()...)
	}
	for _, token := range untargetedArchTokens {
		if !slices.Contains(hostAliases, token) {
			out = append(out, token)
		}
	}
	return out
}

// assetScore ranks a survivor for the tie-break: higher is better, compared
// field by field (libc, then format, then name-closeness).
type assetScore struct {
	name   string
	libc   int
	format int
	name0  int
}

// pickBest scores every survivor and returns the top-ranked one, or an ambiguous
// *AssetMatchError when the best rank is shared by more than one asset.
func pickBest(survivors []string, prefs matchPrefs, host string) (string, error) {
	scores := make([]assetScore, len(survivors))
	for i, a := range survivors {
		scores[i] = scoreAsset(a, prefs)
	}

	best := scores[0]
	for _, s := range scores[1:] {
		if scoreLess(best, s) {
			best = s
		}
	}

	var top []string
	for _, s := range scores {
		if s.libc == best.libc && s.format == best.format && s.name0 == best.name0 {
			top = append(top, s.name)
		}
	}
	sort.Strings(top)

	if len(top) == 1 {
		return top[0], nil
	}
	return "", &AssetMatchError{Host: host, Kind: "ambiguous", Candidates: top}
}

// scoreLess reports whether a ranks below b in the (libc, format, name) order.
func scoreLess(a, b assetScore) bool {
	if a.libc != b.libc {
		return a.libc < b.libc
	}
	if a.format != b.format {
		return a.format < b.format
	}
	return a.name0 < b.name0
}

func scoreAsset(name string, prefs matchPrefs) assetScore {
	return assetScore{
		name:   name,
		libc:   libcScore(name, prefs.libc),
		format: formatRank(name),
		name0:  nameCloseness(tokenize(name), prefs.name),
	}
}

// libcScore rewards the preferred libc variant (2), penalizes the non-preferred
// one (0), and treats an asset with no libc token as neutral (1). The default
// preference is gnu; prefLibc="musl" flips it.
func libcScore(name, prefLibc string) int {
	hasGnu := containsToken(name, "gnu")
	hasMusl := containsToken(name, "musl")
	if prefLibc == "musl" {
		switch {
		case hasMusl:
			return 2
		case hasGnu:
			return 0
		}
		return 1
	}
	switch {
	case hasGnu:
		return 2
	case hasMusl:
		return 0
	}
	return 1
}

// formatRank ranks archive formats by extraction preference:
// tar.gz > tgz > zip > gz > bare binary.
func formatRank(name string) int {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return 4
	case strings.HasSuffix(lower, ".tgz"):
		return 3
	case strings.HasSuffix(lower, ".zip"):
		return 2
	case strings.HasSuffix(lower, ".gz"):
		return 1
	default:
		return 0
	}
}

// nameCloseness rewards an asset whose leading token equals the repo/binary
// name (1) over one that does not (0) — a light nudge toward the primary
// artifact and away from companion downloads.
func nameCloseness(tokens []string, name string) int {
	if name == "" || len(tokens) == 0 {
		return 0
	}
	if tokens[0] == strings.ToLower(name) {
		return 1
	}
	return 0
}

// nonInstallableExts are asset suffixes that are never an installable binary:
// OS package formats, disk images, and signature/checksum/SBOM sidecars.
var nonInstallableExts = []string{
	".deb", ".rpm", ".apk", ".msi", ".dmg", ".pkg",
	".sig", ".asc", ".sha256", ".sha512", ".pem", ".pub", ".sbom",
}

// noiseTokens are whole-token markers of a checksum/SBOM sidecar (e.g.
// "checksums.txt", "SHA256SUMS"), which carry no host arch/OS token but must be
// rejected defensively.
var noiseTokens = map[string]bool{
	"checksums":  true,
	"checksum":   true,
	"sha256sums": true,
	"sha512sums": true,
	"sha256sum":  true,
	"sbom":       true,
}

// notInstallable reports whether name is a package format, disk image, or
// signature/checksum sidecar rather than an installable binary/archive.
func notInstallable(name string, allowExts []string) bool {
	lower := strings.ToLower(name)
	for _, ext := range nonInstallableExts {
		if strings.HasSuffix(lower, ext) && !slices.Contains(allowExts, ext) {
			return true
		}
	}
	for tok := range noiseTokens {
		if containsToken(name, tok) {
			return true
		}
	}
	return false
}

// excluded reports whether name contains any of the case-insensitive exclude
// substrings from a package's Match.Exclude.
func excluded(name string, patterns []string) bool {
	lower := strings.ToLower(name)
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// tokenize splits an asset filename into lowercased fields on the [-_. ]
// delimiter set. It is used only for the leading-token name-closeness nudge:
// OS/arch alias matching uses containsToken so a separator-bearing alias like
// "x86_64" is matched whole rather than shredded into "x86"+"64".
func tokenize(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), isSeparator)
}

// isSeparator reports whether r delimits asset-name tokens.
func isSeparator(r rune) bool {
	switch r {
	case '-', '_', '.', ' ':
		return true
	default:
		return false
	}
}

// hasAnyToken reports whether name carries any of tokens as a whole delimited
// token.
func hasAnyToken(name string, tokens []string) bool {
	for _, t := range tokens {
		if containsToken(name, t) {
			return true
		}
	}
	return false
}

// containsToken reports whether token appears in name bounded on both sides by a
// separator or a string edge — a whole-token match that, unlike splitting on the
// separator set, tolerates a separator INSIDE the token (e.g. the underscore in
// "x86_64") while still refusing to let "arm" match inside "arm64".
func containsToken(name, token string) bool {
	lower := strings.ToLower(name)
	tok := strings.ToLower(token)
	if tok == "" {
		return false
	}
	for from := 0; ; {
		idx := strings.Index(lower[from:], tok)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(tok)
		if isBoundary(lower, start-1) && isBoundary(lower, end) {
			return true
		}
		from = start + 1
	}
}

// isBoundary reports whether position pos in s is a token boundary: a string
// edge or a separator character.
func isBoundary(s string, pos int) bool {
	if pos < 0 || pos >= len(s) {
		return true
	}
	return isSeparator(rune(s[pos]))
}
