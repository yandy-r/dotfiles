package backend

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// yumReposDir is where a .repo file must land for dnf to read it.
const yumReposDir = "/etc/yum.repos.d/"

// dnfRepoMarker is the first line of every repo file dotf writes. Its presence
// marks the file as dotf-owned, so a later run may refresh it; any other
// existing file is left untouched.
const dnfRepoMarker = "# Managed by dotf (repo backend); delete this line to keep dotf from rewriting this file."

// installDNF is the RedHat-family arm of the repo backend: configure the
// third-party repository named by dnf_repo, then install the package from it.
// The three dnf_repo modes are mutually exclusive (validateDNFRepo enforces
// that), and every one of them finishes with the same `dnf install -y`.
//
// Scope is platform.PkgMgrDNF only: yum hosts fall through to not-applicable,
// since every RedHat-family target this repo provisions ships dnf.
func (b *repoBackend) installDNF(pkg manifest.Package, info platform.Info) error {
	dnfName, ok := pkg.Names[string(platform.PkgMgrDNF)]
	if !ok || dnfName == "" {
		return &MissingPackageNameError{Package: pkg.Name, PkgMgr: platform.PkgMgrDNF}
	}

	repo := pkg.DNFRepo
	if repo == nil {
		return fmt.Errorf("backend: package %q missing dnf_repo configuration", pkg.Name)
	}

	switch {
	case repo.COPR != "":
		if err := b.enableCOPR(repo.COPR, info); err != nil {
			return err
		}
	case repo.RepoFile != "":
		if err := b.installRepoFile(repo, info); err != nil {
			return err
		}
	case repo.BaseURL != "":
		if err := b.installRepoBody(repo, info); err != nil {
			return err
		}
	default:
		return fmt.Errorf("backend: package %q dnf_repo declares no copr, repofile or baseurl", pkg.Name)
	}

	enable, err := b.dnfEnableRepoArgs(repo)
	if err != nil {
		return err
	}
	args := append(append([]string{"dnf", "install"}, enable...), "-y", dnfName)
	if err := b.r.Run("sudo", args...); err != nil {
		return fmt.Errorf("dnf install %q: %w", dnfName, err)
	}

	return nil
}

// dnfEnableRepoArgs returns the --enablerepo flag install and upgrade need when
// /etc/yum.repos.d/<id>.repo is a kept file dotf does not own (no marker): such
// a file may ship its section disabled (e.g. Fedora Workstation's
// fedora-workstation-repositories google-chrome.repo has enabled=0). Only a
// section named exactly [<id>] is enabled: dnf rejects an unknown --enablerepo
// id, and other sections in the file are not ours to turn on. A dotf-owned
// file is already enabled=1, and copr mode has no such file, so both get none.
func (b *repoBackend) dnfEnableRepoArgs(repo *manifest.DNFRepo) ([]string, error) {
	if repo.RepoFile == "" && repo.BaseURL == "" {
		return nil, nil
	}
	dest := yumReposDir + repo.ID + ".repo"
	body, err := b.readFile(dest)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read existing dnf repo %q: %w", dest, err)
	}
	if hasDNFRepoMarker(body) || !hasRepoSection(body, repo.ID) {
		return nil, nil
	}
	return []string{"--enablerepo=" + repo.ID}, nil
}

// hasDNFRepoMarker reports whether body's first line is dnfRepoMarker.
func hasDNFRepoMarker(body []byte) bool {
	firstLine, _, _ := bytes.Cut(body, []byte("\n"))
	return string(bytes.TrimSuffix(firstLine, []byte("\r"))) == dnfRepoMarker
}

// hasRepoSection reports whether a .repo file body declares a section header
// exactly "[id]" (surrounding whitespace and CR ignored).
func hasRepoSection(body []byte, id string) bool {
	header := "[" + id + "]"
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == header {
			return true
		}
	}
	return false
}

// installRepoFile fetches a vendor-published .repo file and installs it
// verbatim below dnfRepoMarker. An existing file dotf does not own is kept and
// nothing is fetched. `sudo install -m 644` (not a mv from /tmp) is deliberate:
// the file is created at the destination and so inherits the right SELinux type
// label, where a move would carry tmp_t across and leave dnf unable to read it.
func (b *repoBackend) installRepoFile(repo *manifest.DNFRepo, info platform.Info) error {
	repoFile, err := renderRepoTemplate(repo.RepoFile, info)
	if err != nil {
		return err
	}

	dest := yumReposDir + repo.ID + ".repo"
	owned, err := b.dnfRepoOwned(dest, nil)
	if err != nil || !owned {
		return err
	}

	body, err := b.r.Output("curl", "-fsSL", repoFile)
	if err != nil {
		return fmt.Errorf("fetch dnf repo file %q: %w", repoFile, err)
	}

	return b.installRepoTempFile(dest, append([]byte(dnfRepoMarker+"\n"), body...))
}

// installRepoBody renders the explicit baseurl mode's repo body in Go and
// installs it, so no network fetch is needed to configure the repository.
func (b *repoBackend) installRepoBody(repo *manifest.DNFRepo, info platform.Info) error {
	baseURL, err := renderRepoTemplate(repo.BaseURL, info)
	if err != nil {
		return err
	}

	gpgKey, err := renderRepoTemplate(repo.GPGKey, info)
	if err != nil {
		return err
	}

	// The unmarked legacy body counts as dotf-owned only when the new rendering
	// differs from it (repo_gpgcheck opted in), so there is a migration to do.
	// Otherwise a file whose marker the user deleted would equal the legacy body
	// and be reclaimed, defeating the delete-the-marker opt-out.
	var legacy []byte
	if repo.RepoGPGCheck {
		legacy = []byte(renderDNFRepoBody(repo.ID, baseURL, gpgKey, false))
	}
	dest := yumReposDir + repo.ID + ".repo"
	owned, err := b.dnfRepoOwned(dest, legacy)
	if err != nil || !owned {
		return err
	}

	body := dnfRepoMarker + "\n" + renderDNFRepoBody(repo.ID, baseURL, gpgKey, repo.RepoGPGCheck)
	return b.installRepoTempFile(dest, []byte(body))
}

// renderDNFRepoBody renders the explicit baseurl mode's repo section, without
// the ownership marker. With repoGPGCheck false it is byte-identical to the
// body dotf wrote before the marker existed, which dnfRepoOwned relies on to
// recognise those files as dotf-owned.
func renderDNFRepoBody(id, baseURL, gpgKey string, repoGPGCheck bool) string {
	var body strings.Builder
	fmt.Fprintf(&body, "[%s]\n", id)
	fmt.Fprintf(&body, "name=%s\n", id)
	fmt.Fprintf(&body, "baseurl=%s\n", baseURL)
	body.WriteString("enabled=1\n")
	if gpgKey == "" {
		body.WriteString("gpgcheck=0\n")
	} else {
		body.WriteString("gpgcheck=1\n")
		if repoGPGCheck {
			body.WriteString("repo_gpgcheck=1\n")
		}
		fmt.Fprintf(&body, "gpgkey=%s\n", gpgKey)
	}
	return body.String()
}

// dnfRepoOwned reports whether dotf may write dest: it does not exist yet, its
// first line is dnfRepoMarker, or it byte-equals legacy (the unmarked body an
// earlier dotf rendered; nil disables that check). Any other existing file
// belongs to someone else and is preserved. A read failure other than
// not-exist is returned as an error rather than risking an overwrite.
func (b *repoBackend) dnfRepoOwned(dest string, legacy []byte) (bool, error) {
	existing, err := b.readFile(dest)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read existing dnf repo %q: %w", dest, err)
	}
	if hasDNFRepoMarker(existing) || (legacy != nil && bytes.Equal(existing, legacy)) {
		return true, nil
	}

	fmt.Fprintf(os.Stderr, "dotf: preserving existing %s (not managed by dotf)\n", dest)
	return false, nil
}

// installRepoTempFile stages body in a private temp file and installs it as
// dest under /etc/yum.repos.d.
func (b *repoBackend) installRepoTempFile(dest string, body []byte) error {
	tmp, err := writeTempFile("dotf-dnf-repo-*", body)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()

	if err := b.r.Run("sudo", "install", "-m", "644", tmp, dest); err != nil {
		return fmt.Errorf("write dnf repo %q: %w", dest, err)
	}

	return nil
}

// enableCOPR installs the copr plugin the host's dnf generation ships under
// and enables the project. The plugin package was renamed between dnf4
// (dnf-plugins-core) and dnf5 (dnf5-plugins), so the generation is probed
// first (ADR-7).
func (b *repoBackend) enableCOPR(slug string, info platform.Info) error {
	plugin := "dnf-plugins-core"
	if b.dnfMajor(info) >= 5 {
		plugin = "dnf5-plugins"
	}

	if err := b.r.Run("sudo", "dnf", "install", "-y", plugin); err != nil {
		return fmt.Errorf("install copr plugin %q: %w", plugin, err)
	}

	if err := b.r.Run("sudo", "dnf", "-y", "copr", "enable", slug); err != nil {
		return fmt.Errorf("enable copr %q: %w", slug, err)
	}

	return nil
}

// dnfMajor reports the host's dnf generation (4 or 5). It reads `dnf --version`,
// whose dnf5 output leads with "dnf5" and whose dnf4 output leads with the bare
// version number, and falls back to the os-release VERSION_ID (Fedora 41 is the
// first release where /usr/bin/dnf is dnf5) when the probe is unavailable or
// unparseable. It deliberately does NOT probe for a dnf5 binary: on Fedora 40
// dnf5 can be installed alongside a dnf4 /usr/bin/dnf, so that signal lies.
func (b *repoBackend) dnfMajor(info platform.Info) int {
	out, err := b.r.Output("dnf", "--version")
	if err == nil {
		line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
		if strings.HasPrefix(line, "dnf5") {
			return 5
		}
		for _, field := range strings.Fields(line) {
			if major, ok := leadingInt(field); ok {
				return major
			}
		}
	}

	if version, ok := leadingInt(info.VersionID); ok && version >= 41 {
		return 5
	}
	return 4
}

// leadingInt parses the leading run of digits in s, reporting whether s starts
// with one at all.
func leadingInt(s string) (int, bool) {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// renderRepoTemplate substitutes the {{.ReleaseVer}} and {{.Arch}} placeholders
// in a dnf_repo URL with the host's os-release VERSION_ID and release arch, the
// dnf analogue of renderRepoLine. It errors when the URL needs a releasever the
// host does not report, since an empty $releasever would 404 on the first
// metadata fetch.
func renderRepoTemplate(s string, info platform.Info) (string, error) {
	if strings.Contains(s, "{{.ReleaseVer}}") && info.VersionID == "" {
		return "", fmt.Errorf("repo: host releasever unknown (os-release VERSION_ID absent); cannot render %q", s)
	}
	return strings.NewReplacer("{{.ReleaseVer}}", info.VersionID, "{{.Arch}}", info.Arch.ReleaseArch()).Replace(s), nil
}
