package backend

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
)

// httpDo is the request-level HTTP entry point for the GitHub REST API (the
// asset-list fetch, which needs Accept/Authorization headers). Tests replace it
// to aim at an httptest.Server, mirroring download.go's httpGet var.
var httpDo = http.DefaultClient.Do

// listReleaseAssets returns the asset filenames published on repo's release
// tagged tag, via the GitHub REST API. It is a package var so tests (and #46's
// dotf lock) can inject a stub. GITHUB_TOKEN or GH_TOKEN, when set, is sent as a
// bearer token to lift the unauthenticated rate limit.
var listReleaseAssets = func(repo, tag string) ([]string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("github-release: build asset-list request for %s@%s: %w", repo, tag, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("github-release: fetch asset list for %s@%s: %w", repo, tag, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github-release: asset list for %s@%s: unexpected status %s", repo, tag, resp.Status)
	}

	var payload struct {
		Assets []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("github-release: decode asset list for %s@%s: %w", repo, tag, err)
	}

	names := make([]string, 0, len(payload.Assets))
	for _, a := range payload.Assets {
		names = append(names, a.Name)
	}
	return names, nil
}

// checksumFor downloads the sidecar checksum file (checksums.txt / SHA256SUMS)
// published alongside repo@tag and returns the hex sha256 recorded for asset. It
// is a package var for the same injection reason as listReleaseAssets. Sidecar
// lines follow the coreutils sha256sum format "<sha256>␠␠<filename>" (a single
// space and an optional "*" binary-mode marker are tolerated); a leading path on
// the filename is stripped to its base. An absent asset line is a typed error.
var checksumFor = func(repo, tag, sidecar, asset string) (string, error) {
	url := githubReleaseURL(repo, tag, sidecar)
	resp, err := httpGet(url)
	if err != nil {
		return "", fmt.Errorf("github-release: fetch checksum sidecar %q for %s@%s: %w", sidecar, repo, tag, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github-release: checksum sidecar %q for %s@%s: unexpected status %s", sidecar, repo, tag, resp.Status)
	}

	want := path.Base(asset)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		name := path.Base(strings.TrimPrefix(fields[1], "*"))
		if name == want {
			return fields[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("github-release: read checksum sidecar %q for %s@%s: %w", sidecar, repo, tag, err)
	}

	return "", fmt.Errorf("github-release: checksum sidecar %q has no line for asset %q", sidecar, want)
}

// githubToken returns the GitHub API token from the environment, preferring
// GITHUB_TOKEN over GH_TOKEN. Empty when neither is set (unauthenticated).
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}
