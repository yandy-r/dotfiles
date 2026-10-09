package backend

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/yandy-r/dotfiles/internal/lockfile"
	"github.com/yandy-r/dotfiles/internal/manifest"
	"github.com/yandy-r/dotfiles/internal/platform"
)

// githubRepoPattern validates a GitHub "owner/repo" slug.
var githubRepoPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*/[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// artifact is the resolved download coordinate for a package on one host: the
// concrete URL, the release tag it came from ("" for a URL route), the asset
// filename, and the expected sha256 ("" when no source resolves one; the
// caller applies any fail-closed rule). GitHub reports whether the coordinate
// came from the repo route (release asset) as opposed to a verbatim pkg.URL.
type artifact struct {
	URL    string
	Tag    string
	Asset  string
	SHA256 string
	GitHub bool
}

// resolveArtifact routes pkg to its download coordinates: a set Repo takes the
// github-release route (asset resolution + checksum precedence), else a set URL
// is used verbatim with pkg.SHA256. allowExts exempts listed extensions (e.g.
// ".rpm") from the matcher's non-installable reject list; nil keeps the default
// behavior.
func resolveArtifact(pkg manifest.Package, info platform.Info, allowExts []string) (artifact, error) {
	if pkg.Repo != "" {
		if pkg.Class == manifest.ClassRolling && pkg.Tag == "" {
			if err := validateGitHubRepo(pkg.Repo); err != nil {
				return artifact{}, err
			}
			return latestReleaseArtifact(pkg.Repo, pkg.Asset, pkg.Preview)
		}
		return resolveGitHubArtifact(pkg, info, allowExts)
	}
	if pkg.URL != "" {
		if pkg.URLJSONField != "" {
			resp, err := httpGet(pkg.URL)
			if err != nil {
				return artifact{}, err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != 200 {
				return artifact{}, fmt.Errorf("release API status %s", resp.Status)
			}
			var result map[string]json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return artifact{}, err
			}
			var u string
			if err := json.Unmarshal(result[pkg.URLJSONField], &u); err != nil || !strings.HasPrefix(u, "https://") {
				return artifact{}, fmt.Errorf("release API missing HTTPS field %q", pkg.URLJSONField)
			}
			return artifact{URL: u, Asset: urlAssetName(u)}, nil
		}
		return artifact{
			URL:    pkg.URL,
			Asset:  urlAssetName(pkg.URL),
			SHA256: pkg.SHA256,
		}, nil
	}
	return artifact{}, fmt.Errorf("resolve %q: package has neither repo nor url", pkg.Name)
}

// resolveArtifactLocked is resolveArtifact with a lock override: a lock artifact
// for (pkg.Name, host tokens) replaces the checksum, and for a repo package the
// locked tag + asset, for a direct URL the locked URL. A nil lock or a miss
// falls through to resolveArtifact. Used by Install; lock recording uses
// lockArtifact.
func resolveArtifactLocked(pkg manifest.Package, info platform.Info, allowExts []string, lock *lockfile.Lock) (artifact, error) {
	osTok, archTok := releaseTokens(info)
	art, ok := lock.Artifact(pkg.Name, osTok, archTok)
	if !ok {
		return resolveArtifact(pkg, info, allowExts)
	}
	lp, _ := lock.Package(pkg.Name)
	switch {
	case pkg.Repo != "" && art.Asset != "":
		if err := validateGitHubRepo(pkg.Repo); err != nil {
			return artifact{}, err
		}
		tag := cmp.Or(lp.Tag, pkg.Tag)
		return artifact{
			URL:    githubReleaseURL(pkg.Repo, tag, art.Asset),
			Tag:    tag,
			Asset:  art.Asset,
			SHA256: art.SHA256,
			GitHub: true,
		}, nil
	case pkg.Repo == "" && pkg.URL != "":
		u := cmp.Or(lp.URL, pkg.URL)
		return artifact{URL: u, Asset: urlAssetName(u), SHA256: art.SHA256}, nil
	}
	return resolveArtifact(pkg, info, allowExts)
}

// lockArtifact freezes pkg for info into a lockfile.Artifact. A repo package
// records OS/Arch/Asset/SHA (Binary is pkg.Binary, "" for a bare artifact). A
// direct URL is platform-agnostic: just the SHA256 — the declared one with no
// download, else the hash of a fetch (trust on first lock).
func lockArtifact(pkg manifest.Package, info platform.Info, allowExts []string) (lockfile.Artifact, error) {
	a, err := resolveArtifact(pkg, info, allowExts)
	if err != nil {
		return lockfile.Artifact{}, err
	}
	if !a.GitHub {
		if a.SHA256 == "" {
			if a.SHA256, err = fetchSHA256(a.URL); err != nil {
				return lockfile.Artifact{}, fmt.Errorf("resolve %q: %w", pkg.Name, err)
			}
		}
		return lockfile.Artifact{SHA256: a.SHA256}, nil
	}
	osTok, archTok := releaseTokens(info)
	return lockfile.Artifact{OS: osTok, Arch: archTok, Asset: a.Asset, Binary: pkg.Binary, SHA256: a.SHA256}, nil
}

// resolveGitHubArtifact resolves a repo-backed package into concrete download
// coordinates. Asset precedence: an explicit pkg.Asset is templated
// ({{.OS}}/{{.ReleaseOS}}/{{.Arch}}); an empty pkg.Asset triggers release-page
// auto-detection via listReleaseAssets + matchAsset. SHA precedence:
// Checksums[targetKey(info)] → inline SHA256 → ChecksumsAsset sidecar. An
// unresolvable checksum yields SHA256 == "" (not an error); the caller enforces
// fail-closed for Pinnable packages.
func resolveGitHubArtifact(pkg manifest.Package, info platform.Info, allowExts []string) (artifact, error) {
	if err := validateGitHubRepo(pkg.Repo); err != nil {
		return artifact{}, err
	}

	asset, err := resolveAssetName(pkg, info, allowExts)
	if err != nil {
		return artifact{}, err
	}

	sha, err := resolveChecksum(pkg, info, asset)
	if err != nil {
		return artifact{}, err
	}

	return artifact{
		URL:    githubReleaseURL(pkg.Repo, pkg.Tag, asset),
		Tag:    pkg.Tag,
		Asset:  asset,
		SHA256: sha,
		GitHub: true,
	}, nil
}

// resolveAssetName returns the concrete asset filename: the templated explicit
// asset when set (network-free), else the auto-detected asset from the release
// page. An AssetMatchError from auto-detection is enriched with repo/tag.
func resolveAssetName(pkg manifest.Package, info platform.Info, allowExts []string) (string, error) {
	if pkg.Asset != "" {
		return templateGitHubAsset(pkg.Asset, info), nil
	}

	assets, err := listReleaseAssets(pkg.Repo, pkg.Tag)
	if err != nil {
		return "", err
	}

	prefs := matchPrefs{name: binaryName(pkg), allowExts: allowExts}
	if pkg.Match != nil {
		prefs.libc = pkg.Match.Libc
		prefs.exclude = pkg.Match.Exclude
	}

	asset, err := matchAsset(assets, info, prefs)
	if err != nil {
		var ame *AssetMatchError
		if errors.As(err, &ame) {
			ame.Repo = pkg.Repo
			ame.Tag = pkg.Tag
		}
		return "", err
	}
	return asset, nil
}

// resolveChecksum applies the SHA precedence for asset: the per-target Checksums
// entry, then the inline SHA256, then the ChecksumsAsset sidecar. It returns ""
// (no error) when no source is configured for this host, leaving the fail-closed
// decision to the caller.
func resolveChecksum(pkg manifest.Package, info platform.Info, asset string) (string, error) {
	if sum, ok := pkg.Checksums[targetKey(info)]; ok {
		return sum, nil
	}
	if pkg.SHA256 != "" {
		return pkg.SHA256, nil
	}
	if pkg.ChecksumsAsset != "" {
		return checksumFor(pkg.Repo, pkg.Tag, pkg.ChecksumsAsset, asset)
	}
	return "", nil
}

// targetKey is the Checksums map key for info: "<ReleaseOS>/<canonical-arch>"
// (e.g. "linux/x86_64", "darwin/arm64"). It uses the canonical detection arch
// (info.Arch), not ReleaseArch, so the key is stable, human-writable, and free
// of the amd64/x86_64 ambiguity.
func targetKey(info platform.Info) string {
	return info.OS.ReleaseOS() + "/" + string(info.Arch)
}

func validateGitHubRepo(repo string) error {
	if !githubRepoPattern.MatchString(repo) {
		return fmt.Errorf("github-release: invalid repo %q, want owner/repo", repo)
	}
	return nil
}

// templateGitHubAsset expands an explicit asset template. {{.OS}} renders the
// distro-family OS (preserved for back-compat), {{.ReleaseOS}} renders the
// release OS token (linux/darwin/windows), and {{.Arch}} renders the release
// arch (amd64/arm64).
func templateGitHubAsset(asset string, info platform.Info) string {
	repl := strings.NewReplacer(
		"{{.OS}}", string(info.OS),
		"{{.ReleaseOS}}", info.OS.ReleaseOS(),
		"{{.Arch}}", info.Arch.ReleaseArch(),
	)
	return repl.Replace(asset)
}

// githubReleaseURL builds the download URL for a release asset. A non-empty
// tag pins the exact release (releases/download/<tag>/); an empty tag falls
// back to the floating latest release. Manifest validation requires a tag
// whenever a sha256 is pinned, because a checksum against releases/latest
// breaks on the next upstream release.
func githubReleaseURL(repo, tag, asset string) string {
	if tag != "" {
		return fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, url.PathEscape(tag), encodeAssetPath(asset))
	}
	return fmt.Sprintf("https://github.com/%s/releases/latest/download/%s", repo, encodeAssetPath(asset))
}

func encodeAssetPath(asset string) string {
	parts := strings.Split(asset, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// urlAssetName returns the last path segment of rawurl (the filename a verbatim
// URL downloads to), or the whole URL when it has no usable path.
func urlAssetName(rawurl string) string {
	if u, err := url.Parse(rawurl); err == nil && u.Path != "" && u.Path != "/" {
		return path.Base(u.Path)
	}
	return rawurl
}
