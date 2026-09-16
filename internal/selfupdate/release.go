package selfupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
)

// latestRelease resolves repo's latest published release tag. It is a package
// var so tests can inject a stub for the end-to-end replace path without a
// network. The default implementation is defaultLatestRelease.
var latestRelease = defaultLatestRelease

// defaultLatestRelease queries GET https://api.github.com/repos/<repo>/releases/latest
// (which EXCLUDES prereleases, matching GoReleaser's prerelease:auto and
// bootstrap.sh's HTML /releases/latest/download probe). On 404 — a
// prerelease-only repo with no full release yet — it falls back to GET
// /releases and takes the first entry's tag_name. bootstrap.sh mirrors the
// same latest→releases[0] tag resolution (via gh or token+python3) before
// downloading assets by tag / asset id; see github_resolve_latest_tag.
// GITHUB_TOKEN/GH_TOKEN, when set, is sent as a bearer token to lift the
// unauthenticated rate limit.
func defaultLatestRelease(repo string) (string, error) {
	latestURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	tag, status, err := fetchReleaseTag(latestURL, false)
	if err != nil {
		return "", fmt.Errorf("self-update: fetch latest release for %s: %w", repo, err)
	}

	switch status {
	case http.StatusOK:
		if tag == "" {
			return "", fmt.Errorf("self-update: latest release for %s has no tag_name", repo)
		}
		return tag, nil
	case http.StatusNotFound:
		return firstReleaseTag(repo)
	default:
		return "", fmt.Errorf("self-update: latest release for %s: unexpected status %d", repo, status)
	}
}

// firstReleaseTag resolves the newest release (prerelease-inclusive) via GET
// /releases[0], the same fallback bootstrap.sh uses when /releases/latest 404s.
func firstReleaseTag(repo string) (string, error) {
	listURL := fmt.Sprintf("https://api.github.com/repos/%s/releases", repo)
	tag, status, err := fetchReleaseTag(listURL, true)
	if err != nil {
		return "", fmt.Errorf("self-update: fetch releases for %s: %w", repo, err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("self-update: releases for %s: unexpected status %d", repo, status)
	}
	if tag == "" {
		return "", fmt.Errorf("self-update: no published releases for %s", repo)
	}
	return tag, nil
}

// fetchReleaseTag performs one GitHub REST call via the httpDo seam and decodes
// a tag_name. When listMode is true the body is a JSON array and the first
// element's tag is returned; otherwise the body is a single release object. The
// HTTP status is returned alongside the tag so the caller can drive the 404
// fallback without treating a non-200 as a transport error.
func fetchReleaseTag(apiURL string, listMode bool) (tag string, status int, err error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := httpDo(req)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, nil
	}

	if listMode {
		var releases []struct {
			TagName string `json:"tag_name"`
		}
		if decErr := json.NewDecoder(resp.Body).Decode(&releases); decErr != nil {
			return "", resp.StatusCode, fmt.Errorf("decode releases: %w", decErr)
		}
		if len(releases) == 0 {
			return "", resp.StatusCode, errors.New("empty release list")
		}
		return releases[0].TagName, resp.StatusCode, nil
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if decErr := json.NewDecoder(resp.Body).Decode(&release); decErr != nil {
		return "", resp.StatusCode, fmt.Errorf("decode release: %w", decErr)
	}
	return release.TagName, resp.StatusCode, nil
}

// githubToken returns the GitHub API token from the environment, preferring
// GITHUB_TOKEN over GH_TOKEN (matching bootstrap.sh and internal/backend). Empty
// when neither is set (unauthenticated).
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}
