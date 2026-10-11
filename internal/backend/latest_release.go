package backend

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/yandy-r/dotfiles/internal/platform"
)

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type ghRelease struct {
	TagName    string    `json:"tag_name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

// latestReleaseArtifact resolves repo's latest stable release (or, when
// preview, the first non-draft prerelease of the newest 20) into an artifact
// whose asset name matches the glob pattern. Assets carrying a foreign-arch
// token (e.g. "-arm64" on an x86_64 host) are skipped so a multi-arch release
// resolves with one glob. Exactly one asset must match, with a GitHub-native
// sha256:<64-hex> digest and an https download URL; anything else fails closed.
func latestReleaseArtifact(repo, pattern string, preview bool, info platform.Info) (artifact, error) {
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	if preview {
		url = "https://api.github.com/repos/" + repo + "/releases?per_page=20"
	}
	resp, err := httpGet(url)
	if err != nil {
		return artifact{}, fmt.Errorf("github-latest %s: fetch: %w", repo, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return artifact{}, fmt.Errorf("github-latest %s: unexpected status %s", repo, resp.Status)
	}

	var rel ghRelease
	if preview {
		var list []ghRelease
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return artifact{}, fmt.Errorf("github-latest %s: decode: %w", repo, err)
		}
		found := false
		for _, r := range list {
			if !r.Draft && r.Prerelease {
				rel, found = r, true
				break
			}
		}
		if !found {
			return artifact{}, fmt.Errorf("github-latest %s: no prerelease found", repo)
		}
	} else if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return artifact{}, fmt.Errorf("github-latest %s: decode: %w", repo, err)
	}

	foreignArch := foreignArchTokens(info.Arch)
	var match *ghAsset
	for i := range rel.Assets {
		ok, err := filepath.Match(pattern, rel.Assets[i].Name)
		if err != nil {
			return artifact{}, fmt.Errorf("github-latest %s: bad pattern %q: %w", repo, pattern, err)
		}
		if !ok || hasAnyToken(rel.Assets[i].Name, foreignArch) {
			continue
		}
		if match != nil {
			return artifact{}, fmt.Errorf("github-latest %s@%s: pattern %q matches multiple assets (%q, %q)",
				repo, rel.TagName, pattern, match.Name, rel.Assets[i].Name)
		}
		match = &rel.Assets[i]
	}
	if match == nil {
		return artifact{}, fmt.Errorf("github-latest %s@%s: no asset matches %q", repo, rel.TagName, pattern)
	}

	sum, ok := strings.CutPrefix(match.Digest, "sha256:")
	if raw, err := hex.DecodeString(sum); !ok || err != nil || len(raw) != 32 {
		return artifact{}, fmt.Errorf("github-latest %s@%s: asset %q: digest %q is not sha256:<64-hex>",
			repo, rel.TagName, match.Name, match.Digest)
	}
	if !strings.HasPrefix(match.BrowserDownloadURL, "https://") {
		return artifact{}, fmt.Errorf("github-latest %s@%s: asset %q: non-https download URL %q",
			repo, rel.TagName, match.Name, match.BrowserDownloadURL)
	}

	return artifact{URL: match.BrowserDownloadURL, Tag: rel.TagName, Asset: match.Name, SHA256: sum, GitHub: true}, nil
}
