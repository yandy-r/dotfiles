package backend

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

const testSum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func stubLatest(t *testing.T, body string) *string {
	t.Helper()
	var got string
	orig := httpGet
	httpGet = func(url string) (*http.Response, error) {
		got = url
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	t.Cleanup(func() { httpGet = orig })
	return &got
}

func asset(name, digest, url string) string {
	return `{"name":"` + name + `","digest":"` + digest + `","browser_download_url":"` + url + `"}`
}

func TestLatestReleaseArtifact(t *testing.T) {
	good := asset("tool-linux.tar.gz", "sha256:"+testSum, "https://github.com/o/r/releases/download/v1/tool-linux.tar.gz")
	other := asset("tool-darwin.tar.gz", "", "https://x/y")

	tests := []struct {
		name    string
		body    string
		preview bool
		pattern string
		wantURL string
		wantTag string
		wantErr string
	}{
		{"stable ok", `{"tag_name":"v1","assets":[` + good + `,` + other + `]}`, false, "*linux*",
			"/releases/latest", "v1", ""},
		{"missing digest", `{"tag_name":"v1","assets":[` + asset("tool-linux.tar.gz", "", "https://x/y") + `]}`, false, "*linux*",
			"", "", "not sha256"},
		{"short digest", `{"tag_name":"v1","assets":[` + asset("tool-linux.tar.gz", "sha256:abcd", "https://x/y") + `]}`, false, "*linux*",
			"", "", "not sha256"},
		{"non-https url", `{"tag_name":"v1","assets":[` + asset("tool-linux.tar.gz", "sha256:"+testSum, "http://x/y") + `]}`, false, "*linux*",
			"", "", "non-https"},
		{"ambiguous", `{"tag_name":"v1","assets":[` + good + `,` + asset("tool-linux.zip", "sha256:"+testSum, "https://x/z") + `]}`, false, "*linux*",
			"", "", "multiple assets"},
		{"no match", `{"tag_name":"v1","assets":[` + good + `]}`, false, "*windows*",
			"", "", "no asset matches"},
		{"preview skips draft and stable", `[{"tag_name":"d","draft":true,"prerelease":true,"assets":[]},{"tag_name":"s","assets":[]},` +
			`{"tag_name":"v2-rc1","prerelease":true,"assets":[` + good + `]}]`, true, "*linux*",
			"per_page=20", "v2-rc1", ""},
		{"preview none", `[{"tag_name":"s","assets":[]}]`, true, "*linux*",
			"", "", "no prerelease"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := stubLatest(t, tc.body)
			art, err := latestReleaseArtifact("o/r", tc.pattern, tc.preview)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasSuffix(*url, tc.wantURL) {
				t.Errorf("requested %q, want suffix %q", *url, tc.wantURL)
			}
			want := artifact{
				URL: "https://github.com/o/r/releases/download/v1/tool-linux.tar.gz", Tag: tc.wantTag,
				Asset: "tool-linux.tar.gz", SHA256: testSum, GitHub: true,
			}
			if art != want {
				t.Errorf("artifact = %+v, want %+v", art, want)
			}
		})
	}
}
