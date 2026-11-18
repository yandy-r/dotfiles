package backend

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
)

var (
	linuxX86 = platform.Info{OS: platform.OSRedHat, Arch: platform.ArchX86_64}
	linuxARM = platform.Info{OS: platform.OSRedHat, Arch: platform.ArchARM}
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
		{"skips foreign arch", `{"tag_name":"v1","assets":[` + asset("tool-linux-arm64.tar.gz", "sha256:"+testSum, "https://x/arm") + `,` + good + `]}`, false, "*linux*",
			"", "v1", ""},
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
			art, err := latestReleaseArtifact("o/r", tc.pattern, tc.preview, linuxX86)
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

// releaseJSON renders a one-release API body whose assets all carry a valid
// digest and https URL, so only asset-name selection is under test.
func releaseJSON(tag string, prerelease bool, names ...string) string {
	assets := make([]string, len(names))
	for i, name := range names {
		assets[i] = asset(name, "sha256:"+testSum, "https://github.com/o/r/releases/download/"+tag+"/"+name)
	}
	pre := ""
	if prerelease {
		pre = `"prerelease":true,`
	}
	return `{"tag_name":"` + tag + `",` + pre + `"assets":[` + strings.Join(assets, ",") + `]}`
}

func TestLatestReleaseArtifact_ArchSelection(t *testing.T) {
	obsidian := []string{"Obsidian-1.14.4-arm64.AppImage", "Obsidian-1.14.4.AppImage"}
	localSend := []string{
		"LocalSend-1.18.2-linux-arm-64.tar.gz", "LocalSend-1.18.2-linux-x86-64.tar.gz",
		"LocalSend-1.18.2-linux-x86-64.AppImage",
	}

	t3 := []string{
		"T3-Code-0.1.7-x86_64.AppImage", "T3-Code-0.1.7-arm64.AppImage",
		"T3-Code-0.1.7-amd64.deb", "T3-Code-0.1.7-arm64.deb",
	}
	// v0.1.10+ dropped the version from the AppImage names.
	t3Unversioned := []string{
		"T3-Code-x86_64.AppImage", "T3-Code-arm64.AppImage",
		"T3-Code-0.1.10-amd64.deb", "T3-Code-0.1.10-arm64.deb",
	}

	tests := []struct {
		name    string
		host    platform.Info
		assets  []string
		pattern string
		want    string
		wantErr string
	}{
		{"x86_64 takes the untagged asset beside arm64", linuxX86, obsidian, "Obsidian-*.AppImage",
			"Obsidian-1.14.4.AppImage", ""},
		{"asset order does not matter", linuxX86, []string{obsidian[1], obsidian[0]}, "Obsidian-*.AppImage",
			"Obsidian-1.14.4.AppImage", ""},
		{"arm64 sibling glob takes the tagged asset", linuxARM64, obsidian, "Obsidian-*-arm64.AppImage",
			"Obsidian-1.14.4-arm64.AppImage", ""},
		{"arm64 host keeps the shared glob ambiguous", linuxARM64, obsidian, "Obsidian-*.AppImage",
			"", "multiple assets"},
		{"aarch64 spelling, any case", linuxX86, []string{"Tool-1-AARCH64.AppImage", "Tool-1.AppImage"}, "Tool-*.AppImage",
			"Tool-1.AppImage", ""},
		{"x86_64 spellings are foreign on arm64", linuxARM64,
			[]string{"tool-1-x64.rpm", "tool-1.amd64.rpm", "tool-1.x86_64.rpm", "tool-1.aarch64.rpm"}, "tool-*.rpm",
			"tool-1.aarch64.rpm", ""},
		{"32-bit arm variants are foreign", linuxX86,
			[]string{"tool-1-armv7l.AppImage", "tool-1-armv6l.AppImage", "tool-1-armel.AppImage", "tool-1.AppImage"}, "tool-*.AppImage",
			"tool-1.AppImage", ""},
		{"untargeted arches are foreign", linuxX86,
			[]string{"tool-1-i386.AppImage", "tool-1-i686.AppImage", "tool-1-riscv64.AppImage", "tool-1-ppc64le.AppImage", "tool-1-s390x.AppImage", "tool-1.AppImage"},
			"tool-*.AppImage", "tool-1.AppImage", ""},
		{"foreign-only release fails closed", linuxX86,
			[]string{"tool-1-arm64.AppImage", "tool-1-armv7l.AppImage", "tool-1-riscv64.AppImage"}, "tool-*.AppImage",
			"", "no asset matches"},
		{"two host assets stay ambiguous after the skip", linuxX86,
			[]string{"tool-1-arm64.AppImage", "tool-1.AppImage", "tool-1-x86_64.AppImage"}, "tool-*.AppImage",
			"", "multiple assets"},
		{"arm-64 spelling is host on arm64", linuxARM64, localSend, "LocalSend-*-linux-arm-64.tar.gz",
			"LocalSend-1.18.2-linux-arm-64.tar.gz", ""},
		{"x86-64 spelling is foreign on arm64", linuxARM64, localSend, "LocalSend-*.AppImage",
			"", "no asset matches"},
		{"arm-64 spelling is foreign on x86_64", linuxX86, localSend, "LocalSend-*-linux-*.tar.gz",
			"LocalSend-1.18.2-linux-x86-64.tar.gz", ""},
		{"arm-64 spelling is foreign on 32-bit arm", linuxARM, localSend, "LocalSend-*-linux-*.tar.gz",
			"", "no asset matches"},
		{"T3 deb glob selects amd64 on x86_64", linuxX86, t3, "T3-Code-*.deb",
			"T3-Code-0.1.7-amd64.deb", ""},
		{"T3 deb glob selects arm64 on arm64", linuxARM64, t3, "T3-Code-*.deb",
			"T3-Code-0.1.7-arm64.deb", ""},
		{"T3 x86_64 AppImage glob is unambiguous", linuxX86, t3, "T3-Code-*x86_64.AppImage",
			"T3-Code-0.1.7-x86_64.AppImage", ""},
		{"T3 arm64 AppImage glob is unambiguous", linuxARM64, t3, "T3-Code-*arm64.AppImage",
			"T3-Code-0.1.7-arm64.AppImage", ""},
		{"T3 x86_64 AppImage glob matches unversioned name", linuxX86, t3Unversioned, "T3-Code-*x86_64.AppImage",
			"T3-Code-x86_64.AppImage", ""},
		{"T3 arm64 AppImage glob matches unversioned name", linuxARM64, t3Unversioned, "T3-Code-*arm64.AppImage",
			"T3-Code-arm64.AppImage", ""},
		{"host token outweighs a foreign token in the same name", linuxX86,
			[]string{"arm-gnu-toolchain-14-x86_64-arm-none-eabi.tar.xz", "arm-gnu-toolchain-14-aarch64-arm-none-eabi.tar.xz"},
			"arm-gnu-toolchain-*-arm-none-eabi.tar.xz", "arm-gnu-toolchain-14-x86_64-arm-none-eabi.tar.xz", ""},
		{"host token outweighs a foreign token on arm64", linuxARM64,
			[]string{"arm-gnu-toolchain-14-x86_64-arm-none-eabi.tar.xz", "arm-gnu-toolchain-14-aarch64-arm-none-eabi.tar.xz"},
			"arm-gnu-toolchain-*-arm-none-eabi.tar.xz", "arm-gnu-toolchain-14-aarch64-arm-none-eabi.tar.xz", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stubLatest(t, releaseJSON("v1", false, tc.assets...))
			art, err := latestReleaseArtifact("o/r", tc.pattern, false, tc.host)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v (asset %q), want containing %q", err, art.Asset, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if art.Asset != tc.want {
				t.Errorf("asset = %q, want %q", art.Asset, tc.want)
			}
		})
	}
}

func TestLatestReleaseArtifact_PreviewSkipsForeignArch(t *testing.T) {
	stubLatest(t, "["+releaseJSON("v2-pre", true, "tool-2-arm64.AppImage", "tool-2.AppImage")+"]")
	art, err := latestReleaseArtifact("o/r", "tool-*.AppImage", true, linuxX86)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if art.Asset != "tool-2.AppImage" || art.Tag != "v2-pre" {
		t.Errorf("artifact = %s@%s, want tool-2.AppImage@v2-pre", art.Asset, art.Tag)
	}
}
