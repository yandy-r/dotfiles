package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/manifest"
)

// buildIDPrefix is the payload subtree rpm_repack_strip_buildid removes.
const buildIDPrefix = "/usr/lib/.build-id"

// repackReleaseTag is appended to the vendor release so a rebuilt package
// sorts newer than the vendor build and is recognisable in `rpm -q`
// (Installed treats ".dotf" in the installed release as the probe).
const repackReleaseTag = ".dotf1"

const (
	rpmMetaQF    = "%{NAME}\n%{EPOCHNUM}\n%{VERSION}\n%{RELEASE}\n%{ARCH}\n%{LICENSE}\n%{SUMMARY}\n%{URL}\n%{DESCRIPTION}\n"
	rpmScriptsQF = "%{PREIN}\x1f%{PREINPROG}\x1f%{POSTIN}\x1f%{POSTINPROG}\x1f%{PREUN}\x1f%{PREUNPROG}\x1f%{POSTUN}\x1f%{POSTUNPROG}\x1f%{TRIGGERSCRIPTS}\x1f%{FILECAPS}"
)

// scriptKeys indexes the odd (body) and even (interpreter) halves of the
// scriptlet reply in rpmScriptsQF order.
var scriptKeys = []string{"pre", "post", "preun", "postun"}

// rpmMeta is the vendor (or rebuilt) package metadata the repack carries
// over and the post-build gate compares.
type rpmMeta struct {
	name, epoch, version, release, arch string
	license, summary, url, description  string
	requires, provides                  []string
	dump                                []rpmDumpEntry
	scripts                             map[string]string // "pre", "post", "preun", "postun" -> body
	scriptProgs                         map[string]string // same keys -> interpreter ("(none)" -> "")
	digestAlgo                          string            // rpm file digest algorithm (e.g. "sha256")
	triggers                            string            // non-empty when the package carries trigger scriptlets
	filecaps                            string            // non-empty when a file carries capabilities
}

// rpmDumpEntry is one `rpm --dump` line: the path plus the mode/owner/group
// and %config/%doc flags the generated spec must reproduce byte-for-byte.
type rpmDumpEntry struct {
	raw             string
	path, mode      string
	owner, group    string
	isConfig, isDoc bool
}

// queryRPMMeta reads the metadata, scriptlets, requires, provides and file
// dump of the RPM at path through the host-native rpm tool.
func queryRPMMeta(r exec.Runner, path string) (*rpmMeta, error) {
	metaOut, err := r.Output("rpm", "-qp", "--nosignature", "--qf", rpmMetaQF, path)
	if err != nil {
		return nil, fmt.Errorf("query metadata: %w", err)
	}
	f := strings.SplitN(string(metaOut), "\n", 9)
	if len(f) < 9 {
		return nil, fmt.Errorf("short metadata reply (%d fields)", len(f))
	}
	m := &rpmMeta{
		name: f[0], epoch: rpmNone(f[1]), version: f[2], release: f[3],
		arch: f[4], license: rpmNone(f[5]), summary: f[6], url: rpmNone(f[7]),
		description: strings.TrimRight(f[8], "\n"),
		scripts:     map[string]string{},
		scriptProgs: map[string]string{},
	}

	scriptsOut, err := r.Output("rpm", "-qp", "--nosignature", "--qf", rpmScriptsQF, path)
	if err != nil {
		return nil, fmt.Errorf("query scriptlets: %w", err)
	}
	parts := strings.Split(string(scriptsOut), "\x1f")
	if len(parts) != 10 {
		return nil, fmt.Errorf("short scriptlet reply (%d fields)", len(parts))
	}
	for i, k := range scriptKeys {
		m.scripts[k] = rpmNone(strings.TrimRight(parts[2*i], "\n"))
		m.scriptProgs[k] = rpmNone(parts[2*i+1])
	}
	m.triggers = rpmNone(strings.TrimSpace(parts[8]))
	m.filecaps = rpmNone(strings.TrimSpace(parts[9]))

	for _, q := range []struct {
		flag string
		dst  *[]string
	}{
		{"-R", &m.requires},
		{"--provides", &m.provides},
	} {
		out, err := r.Output("rpm", "-qp", q.flag, "--nosignature", path)
		if err != nil {
			return nil, fmt.Errorf("query %s: %w", q.flag, err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			// rpmlib(*) markers are re-added by rpmbuild itself.
			if line == "" || strings.HasPrefix(line, "rpmlib(") {
				continue
			}
			*q.dst = append(*q.dst, line)
		}
	}

	// rpmbuild must preserve the vendor's payload digest algorithm, or the
	// gate's dump comparison would compare hashes from different digests.
	algoOut, err := r.Output("rpm", "-qp", "--nosignature", "--qf", "%{FILEDIGESTALGO}", path)
	if err != nil {
		return nil, fmt.Errorf("query digest algorithm: %w", err)
	}
	m.digestAlgo = rpmNone(strings.TrimSpace(string(algoOut)))
	if m.digestAlgo == "" {
		m.digestAlgo = "1"
	} // RPM absence means legacy MD5

	dumpOut, err := r.Output("rpm", "-qp", "--nosignature", "--dump", path)
	if err != nil {
		return nil, fmt.Errorf("query file dump: %w", err)
	}
	if m.digestAlgo == "" {
		return nil, fmt.Errorf("rpm reports no file digest algorithm")
	}
	if m.dump, err = parseRPMDump(dumpOut); err != nil {
		return nil, err
	}
	return m, nil
}

// repackUnsupported rejects vendor metadata the generated spec cannot yet
// reproduce faithfully: trigger scriptlets, file capabilities, non-empty
// %trans/%verify scriptlets and non-/bin/sh scriptlet interpreters would all
// be silently DROPPED by the rebuild, so they abort the repack instead.
func repackUnsupported(m *rpmMeta) error {
	if m.triggers != "" {
		return fmt.Errorf("package carries trigger scriptlets, which a repack would drop")
	}
	if m.filecaps != "" {
		return fmt.Errorf("package carries file capabilities, which a repack would drop")
	}
	for _, k := range []string{"pretrans", "posttrans", "verifyscript"} {
		if m.scripts[k] != "" {
			return fmt.Errorf("package carries a %s scriptlet, which a repack would drop", k)
		}
	}
	for _, k := range scriptKeys {
		if p := m.scriptProgs[k]; m.scripts[k] != "" && p != "" && p != "/bin/sh" {
			return fmt.Errorf("%s scriptlet uses interpreter %q; only /bin/sh is preserved", k, p)
		}
	}
	return nil
}

func rpmNone(s string) string {
	if s == "(none)" {
		return ""
	}
	return s
}

// parseRPMDump parses `rpm --dump` lines. The format is space-separated and
// unquoted, so paths containing whitespace are rejected outright.
// ponytail: 11-field lines only; quoted/escaped rpm dump variants can be
// added if a vendor ever ships a spaced path.
func parseRPMDump(out []byte) ([]rpmDumpEntry, error) {
	var entries []rpmDumpEntry
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 11 {
			return nil, fmt.Errorf("unparseable rpm dump line (paths with spaces are unsupported): %q", line)
		}
		entries = append(entries, rpmDumpEntry{
			raw: line, path: f[0], mode: f[4], owner: f[5], group: f[6],
			isConfig: f[7] == "1", isDoc: f[8] == "1",
		})
	}
	return entries, nil
}

// filterBuildID splits entries into the payload to keep and the
// /usr/lib/.build-id subtree to drop.
func filterBuildID(entries []rpmDumpEntry) (kept []rpmDumpEntry, dropped int) {
	for _, e := range entries {
		if e.path == buildIDPrefix || strings.HasPrefix(e.path, buildIDPrefix+"/") {
			dropped++
			continue
		}
		kept = append(kept, e)
	}
	return kept, dropped
}

// safePayloadPath validates one absolute payload path from the rpm dump
// against the cpio extraction fence: it must stay a clean absolute path
// (no ".", "..", empty or trailing-slash components) so that joining it
// under the extraction dir cannot escape. `cpio --no-absolute-filenames`
// does NOT sanitize "../" components, so this check — plus
// checkExtractedContainment after extraction — is the actual fence.
func safePayloadPath(p string) error {
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("payload path %q is not absolute", p)
	}
	for _, seg := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("reject unsafe payload path %q", p)
		}
	}
	return nil
}

// checkExtractedContainment walks root and rejects any entry (symlink,
// hardlink or otherwise) whose resolved target points outside root. A
// hostile payload could otherwise plant a symlink that the later `cp -a`
// into the rpmbuild buildroot follows somewhere privileged.
func checkExtractedContainment(root string) error {
	return filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type() != os.ModeSymlink {
			return nil
		}
		// Lstat via d: resolve the link against its parent, not CWD.
		target, err := os.Readlink(p)
		if err != nil {
			return err
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(p), target))
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("payload symlink %q resolves outside the extraction root", p)
		}
		return nil
	})
}

// specEscapeLine escapes a single-line spec value; newlines are rejected so
// vendor metadata cannot inject extra spec directives.
func specEscapeLine(what, s string) (string, error) {
	if strings.ContainsAny(s, "\n\r") {
		return "", fmt.Errorf("%s contains a newline", what)
	}
	return strings.ReplaceAll(s, "%", "%%"), nil
}

// specEscapeBlock escapes a multi-line spec body (description, scriptlets).
func specEscapeBlock(s string) string {
	return strings.ReplaceAll(s, "%", "%%")
}

// renderRepackSpec renders the spec that rebuilds the vendor payload (minus
// build-id entries) with build-id generation disabled and the release
// suffixed. payloadDir is copied into the buildroot verbatim by %install.
func renderRepackSpec(m *rpmMeta, kept []rpmDumpEntry, payloadDir string) (string, error) {
	if len(kept) == 0 {
		return "", fmt.Errorf("nothing left to package after stripping %s", buildIDPrefix)
	}
	if strings.ContainsAny(payloadDir, " \t\n\r") {
		return "", fmt.Errorf("payload dir %q contains whitespace", payloadDir)
	}
	var b strings.Builder
	b.WriteString("%global _build_id_links none\n%global __os_install_post %{nil}\n\n")
	fmt.Fprintf(&b, "%%global _binary_filedigest_algorithm %s\n", m.digestAlgo)

	write := func(label, value string) error {
		v, err := specEscapeLine(label, value)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s: %s\n", label, v)
		return nil
	}
	if err := write("Name", m.name); err != nil {
		return "", err
	}
	if m.epoch != "" && m.epoch != "0" {
		if err := write("Epoch", m.epoch); err != nil {
			return "", err
		}
	}
	if err := write("Version", m.version); err != nil {
		return "", err
	}
	rel, err := specEscapeLine("release", m.release)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(&b, "Release: %s%s%%{?dist}\n", rel, repackReleaseTag)
	if err := write("Summary", m.summary); err != nil {
		return "", err
	}
	if err := write("License", m.license); err != nil {
		return "", err
	}
	if m.url != "" {
		if err := write("URL", m.url); err != nil {
			return "", err
		}
	}
	for _, req := range m.requires {
		if err := write("Requires", req); err != nil {
			return "", err
		}
	}
	// Requires are carried verbatim; autoreq would only re-derive them.
	b.WriteString("AutoReq: no\n")
	b.WriteString("AutoProv: no\n")
	if m.arch == "noarch" {
		b.WriteString("BuildArch: noarch\n")
	}
	desc := m.description
	if desc == "" {
		desc = m.summary
	}
	fmt.Fprintf(&b, "\n%%description\n%s\n", specEscapeBlock(desc))
	fmt.Fprintf(&b, "\n%%install\nmkdir -p %%{buildroot}\ncp -a %s/. %%{buildroot}/\n", payloadDir)
	for _, k := range scriptKeys {
		if body := m.scripts[k]; body != "" {
			fmt.Fprintf(&b, "\n%%%s\n%s\n", k, specEscapeBlock(body))
		}
	}
	b.WriteString("\n%files\n")
	for _, e := range kept {
		mode, err := strconv.ParseUint(e.mode, 8, 32)
		if err != nil {
			return "", fmt.Errorf("file %s: parse mode %q: %w", e.path, e.mode, err)
		}
		var flags string
		if mode&0o170000 == 0o040000 {
			flags += "%dir " // never recurse into a directory's payload
		}
		if e.isConfig {
			flags += "%config "
		}
		if e.isDoc {
			flags += "%doc "
		}
		owner, err := specEscapeLine("owner", e.owner)
		if err != nil {
			return "", err
		}
		group, err := specEscapeLine("group", e.group)
		if err != nil {
			return "", err
		}
		path, err := specEscapeLine("path", e.path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "%s%%attr(%04o,%s,%s) %s\n", flags, mode&0o7777, owner, group, path)
	}
	return b.String(), nil
}

// gateRepacked compares the rebuilt package against the vendor package
// BEFORE its path is handed to any privileged install step: identical
// name/version/arch, vendor release plus the .dotf1 suffix, identical
// requires and provides, a file dump identical (modes, symlinks, flags,
// mtimes, digests — so the digest algorithm cannot silently change) except
// for the removed build-id entries, and unchanged scriptlets.
func gateRepacked(vendor, rebuilt *rpmMeta, kept []rpmDumpEntry) error {
	if rebuilt.name != vendor.name {
		return fmt.Errorf("name changed: %q -> %q", vendor.name, rebuilt.name)
	}
	if rebuilt.epoch != vendor.epoch && (rebuilt.epoch != "0" || vendor.epoch != "") && (vendor.epoch != "0" || rebuilt.epoch != "") {
		return fmt.Errorf("epoch changed: %q -> %q", vendor.epoch, rebuilt.epoch)
	}
	if !slices.Equal(uniqSorted(vendor.requires), uniqSorted(rebuilt.requires)) {
		return fmt.Errorf("requires differ: vendor %v, rebuilt %v", vendor.requires, rebuilt.requires)
	}
	vendorEVR := vendor.version + "-" + vendor.release
	rebuiltEVR := rebuilt.version + "-" + rebuilt.release
	provides := make([]string, len(vendor.provides))
	for i, p := range vendor.provides {
		provides[i] = strings.ReplaceAll(p, " = "+vendorEVR, " = "+rebuiltEVR)
	}
	if !slices.Equal(uniqSorted(provides), uniqSorted(rebuilt.provides)) {
		return fmt.Errorf("provides differ: vendor %v, rebuilt %v", vendor.provides, rebuilt.provides)
	}
	if rebuilt.version != vendor.version {
		return fmt.Errorf("version changed: %q -> %q", vendor.version, rebuilt.version)
	}
	if rebuilt.arch != vendor.arch {
		return fmt.Errorf("arch changed: %q -> %q", vendor.arch, rebuilt.arch)
	}
	if rebuilt.license != vendor.license {
		return fmt.Errorf("license changed: %q -> %q", vendor.license, rebuilt.license)
	}
	if rebuilt.summary != vendor.summary {
		return fmt.Errorf("summary changed: %q -> %q", vendor.summary, rebuilt.summary)
	}
	if rebuilt.url != vendor.url {
		return fmt.Errorf("url changed: %q -> %q", vendor.url, rebuilt.url)
	}
	if rebuilt.description != vendor.description {
		return fmt.Errorf("description changed")
	}
	if !strings.HasPrefix(rebuilt.release, vendor.release+repackReleaseTag) {
		return fmt.Errorf("release %q is not vendor release %q plus %q suffix", rebuilt.release, vendor.release, repackReleaseTag)
	}
	if rebuilt.digestAlgo != vendor.digestAlgo {
		return fmt.Errorf("file digest algorithm changed: %q -> %q (repack must preserve the vendor digest algorithm)", vendor.digestAlgo, rebuilt.digestAlgo)
	}
	want := make([]string, 0, len(kept))
	for _, e := range kept {
		want = append(want, e.raw)
	}
	got := make([]string, 0, len(rebuilt.dump))
	for _, e := range rebuilt.dump {
		got = append(got, e.raw)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		return fmt.Errorf("payload differs: rebuilt dump must equal the vendor dump (modes/symlinks/flags/mtimes/digests) minus %s entries", buildIDPrefix)
	}
	for _, k := range scriptKeys {
		if vendor.scripts[k] != rebuilt.scripts[k] {
			return fmt.Errorf("%s scriptlet differs", k)
		}
	}
	return nil
}

// repackStripBuildID rebuilds the verified vendor RPM at src with the
// /usr/lib/.build-id payload stripped, using host-native
// rpm2cpio/cpio/rpm/rpmbuild (never rpmrebuild). Every intermediate lives
// under root (the single private dotf-repack-* dir owned and removed by the
// caller); the returned rebuilt RPM path also lives under root. The result
// is gated against the vendor metadata before its path is returned; no step
// here escalates — the caller installs the gated RPM with sudo dnf install.
func repackStripBuildID(r exec.Runner, pkg manifest.Package, src, root string) (string, error) {
	vendor, err := queryRPMMeta(r, src)
	if err != nil {
		return "", err
	}
	if vendor.name != pkg.PkgName {
		return "", fmt.Errorf("vendor package name %q does not match pkg_name %q", vendor.name, pkg.PkgName)
	}
	if err := repackUnsupported(vendor); err != nil {
		return "", fmt.Errorf("rpm repack unsupported: %w", err)
	}
	kept, dropped := filterBuildID(vendor.dump)
	if dropped == 0 {
		return "", fmt.Errorf("vendor RPM carries no %s payload", buildIDPrefix)
	}
	// Fence the cpio archive BEFORE extraction: the dump (from the same
	// payload) and the archive entry list must agree on safe paths — cpio
	// --no-absolute-filenames does not sanitize "../".
	for _, e := range vendor.dump {
		if err := safePayloadPath(e.path); err != nil {
			return "", err
		}
	}

	cpioBytes, err := r.Output("rpm2cpio", src)
	if err != nil {
		return "", fmt.Errorf("rpm2cpio: %w", err)
	}
	cpioPath := filepath.Join(root, "payload.cpio")
	if err := os.WriteFile(cpioPath, cpioBytes, 0o600); err != nil {
		return "", err
	}
	payloadDir := filepath.Join(root, "payload")
	if err := os.MkdirAll(payloadDir, 0o755); err != nil {
		return "", err
	}
	if err := r.RunDir(payloadDir, "cpio", "-idm", "--quiet", "--no-absolute-filenames", "--file="+cpioPath); err != nil {
		return "", fmt.Errorf("extract payload: %w", err)
	}
	if err := checkExtractedContainment(payloadDir); err != nil {
		return "", err
	}
	if err := os.RemoveAll(filepath.Join(payloadDir, strings.TrimPrefix(buildIDPrefix, "/"))); err != nil {
		return "", fmt.Errorf("remove excluded build-id payload: %w", err)
	}

	spec, err := renderRepackSpec(vendor, kept, payloadDir)
	if err != nil {
		return "", err
	}
	topdir := filepath.Join(root, "rpmbuild")
	for _, d := range []string{"BUILD", "RPMS", "SOURCES", "SPECS", "SRPMS"} {
		if err := os.MkdirAll(filepath.Join(topdir, d), 0o755); err != nil {
			return "", err
		}
	}
	specPath := filepath.Join(topdir, "SPECS", "repack.spec")
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		return "", err
	}
	// Containment: no default HOME caches or ~/.rpmmacros bleed in, and every
	// rpmbuild temp path stays under root (env wrapper: Runner has no env API).
	tmp := filepath.Join(root, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "dotf: rebuilding %s RPM without %s (this can take a few minutes)...\n", vendor.name, buildIDPrefix)
	if err := r.Run("env", "HOME="+root, "TMPDIR="+tmp, "rpmbuild", "--quiet", "-bb",
		"--define", "_binary_payload w3T.xzdio",
		"--define", "_topdir "+topdir, "--define", "_tmppath "+tmp, specPath); err != nil {
		return "", fmt.Errorf("rpmbuild: %w", err)
	}

	hits, err := filepath.Glob(filepath.Join(topdir, "RPMS", "*", "*.rpm"))
	if err != nil || len(hits) != 1 {
		return "", fmt.Errorf("rpmbuild produced %d RPMs, want exactly 1", len(hits))
	}
	rebuilt, err := queryRPMMeta(r, hits[0])
	if err != nil {
		return "", fmt.Errorf("read rebuilt metadata: %w", err)
	}
	if err := gateRepacked(vendor, rebuilt, kept); err != nil {
		return "", fmt.Errorf("repack gate: %w", err)
	}
	return hits[0], nil
}
