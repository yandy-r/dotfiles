package manifest

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// coprSlugPattern matches a COPR "owner/project" slug. The owner half accepts
// the "@" of a COPR group account (e.g. "@caddy/caddy"), which the project half
// never carries.
var coprSlugPattern = regexp.MustCompile(`^[A-Za-z0-9._@+-]+/[A-Za-z0-9._+-]+$`)

// keyFingerprintPattern matches a 40-hex OpenPGP v4 fingerprint, the expected
// shape of AptRepo.KeyFingerprint (case-insensitive; the backend compares with
// strings.EqualFold).
var keyFingerprintPattern = regexp.MustCompile(`^[0-9A-Fa-f]{40}$`)

// validateRepo checks a "repo" package (and its deprecated "apt-repo" alias):
// at least one family sub-table must be present, and each present sub-table is
// checked on its own terms. The alias keeps the legacy apt-only vocabulary in
// its absent-sub-table message so an existing manifest's failure text does not
// shift under the rename.
func validateRepo(p Package) []error {
	if p.AptRepo == nil && p.DNFRepo == nil {
		if p.Backend == BackendAPTRepo {
			return []error{&ValidationError{Package: p.Name, Msg: "apt-repo backend requires apt_repo"}}
		}
		return []error{&ValidationError{Package: p.Name, Msg: "repo backend requires at least one of apt_repo or dnf_repo"}}
	}

	var errs []error
	if p.AptRepo != nil {
		errs = append(errs, validateAptRepo(p)...)
	}
	if p.DNFRepo != nil {
		errs = append(errs, validateDNFRepo(p)...)
	}
	return errs
}

// validateAptRepo checks the apt_repo sub-table: all five fields are required,
// because the Debian-family install path pins the key by fingerprint, writes
// the sources entry, and needs the signed-by keyring path in both.
func validateAptRepo(p Package) []error {
	var errs []error

	if p.AptRepo.KeyURL == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "apt-repo backend requires apt_repo.key_url"})
	}
	if p.AptRepo.Keyring == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "apt-repo backend requires apt_repo.keyring"})
	}
	if p.AptRepo.RepoLine == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "apt-repo backend requires apt_repo.repo_line"})
	}
	if p.AptRepo.SourcesPath == "" {
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "apt-repo backend requires apt_repo.sources_path"})
	}
	switch fp := p.AptRepo.KeyFingerprint; {
	case fp == "":
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "apt-repo backend requires apt_repo.key_fingerprint"})
	case !keyFingerprintPattern.MatchString(fp):
		errs = append(errs, &ValidationError{Package: p.Name, Msg: fmt.Sprintf("apt_repo.key_fingerprint %q is not a 40-char hex OpenPGP fingerprint", fp)})
	}

	return errs
}

// validateDNFRepo checks the dnf_repo sub-table's three-mode discriminator
// (copr / repofile / baseurl), the fields each mode owns, and the https scheme
// of every URL it carries. Templated URLs are deliberately accepted: a
// {{.ReleaseVer}} or {{.Arch}} placeholder still parses with an https scheme.
func validateDNFRepo(p Package) []error {
	var errs []error
	d := p.DNFRepo

	modes := 0
	for _, mode := range []string{d.COPR, d.RepoFile, d.BaseURL} {
		if mode != "" {
			modes++
		}
	}
	switch {
	case modes == 0:
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "repo backend dnf_repo requires one of copr, repofile, or baseurl"})
	case modes > 1:
		errs = append(errs, &ValidationError{Package: p.Name, Msg: "repo backend dnf_repo modes copr, repofile and baseurl are mutually exclusive"})
	}

	if d.COPR != "" {
		if !coprSlugPattern.MatchString(d.COPR) {
			errs = append(errs, &ValidationError{
				Package: p.Name,
				Msg:     fmt.Sprintf("repo backend dnf_repo copr %q must be \"owner/project\"", d.COPR),
			})
		}
		if d.ID != "" || d.BaseURL != "" || d.GPGKey != "" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "repo backend dnf_repo copr mode takes no id, baseurl or gpgkey"})
		}
	} else if d.RepoFile != "" || d.BaseURL != "" {
		errs = append(errs, validateDNFRepoID(p)...)
	}

	if d.RepoGPGCheck {
		switch {
		case d.BaseURL == "" || d.COPR != "" || d.RepoFile != "":
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "repo backend dnf_repo repo_gpgcheck applies only to baseurl mode"})
		case d.GPGKey == "":
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "repo backend dnf_repo repo_gpgcheck requires gpgkey"})
		}
	}

	for _, u := range []struct{ value, field string }{
		{d.RepoFile, "repofile"},
		{d.BaseURL, "baseurl"},
		{d.GPGKey, "gpgkey"},
	} {
		if u.value == "" {
			continue
		}
		if parsed, parseErr := url.Parse(u.value); parseErr != nil || parsed.Scheme != "https" {
			errs = append(errs, &ValidationError{Package: p.Name, Msg: "repo backend dnf_repo requires https " + u.field})
		}
	}

	return errs
}

// validateDNFRepoID checks the id the file modes require: it is joined into
// /etc/yum.repos.d/<id>.repo, so it must be a single safe path element.
func validateDNFRepoID(p Package) []error {
	id := p.DNFRepo.ID

	if id == "" {
		return []error{&ValidationError{Package: p.Name, Msg: "repo backend dnf_repo requires id"}}
	}
	if strings.Contains(id, "/") || id == "." || id == ".." {
		return []error{&ValidationError{
			Package: p.Name,
			Msg:     fmt.Sprintf("repo backend dnf_repo id %q must be a bare filename", id),
		}}
	}

	return nil
}
