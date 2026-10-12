package backend

import (
	"errors"
	"slices"
	"sort"
	"testing"

	"github.com/yandy-r/dotfiles/internal/platform"
)

// Host shorthands for the corpus. matchAsset keys off ReleaseOS/ReleaseArch
// aliases, so only OS and Arch matter here.
var (
	linuxAMD64 = platform.Info{OS: platform.OSArch, Arch: platform.ArchX86_64}
	linuxARM64 = platform.Info{OS: platform.OSDebian, Arch: platform.ArchARM64}
	darwinAMD  = platform.Info{OS: platform.OSMacOS, Arch: platform.ArchX86_64}
	darwinARM  = platform.Info{OS: platform.OSMacOS, Arch: platform.ArchARM64}
)

// Real (representative) release asset lists for the spike corpus. These mirror
// how each tool publishes its release page: case-mixed OS tokens, bare
// binaries, .gz/.zip/.tar.gz archives, and the sidecar/signature/package noise
// that must be rejected.
var (
	tursoAssets = []string{
		"turso_Linux_x86_64.tar.gz",
		"turso_Linux_arm64.tar.gz",
		"turso_Darwin_x86_64.tar.gz",
		"turso_Darwin_arm64.tar.gz",
	}

	teaAssets = []string{
		"tea-0.9.2-linux-amd64",
		"tea-0.9.2-linux-arm64",
		"tea-0.9.2-darwin-amd64",
		"tea-0.9.2-darwin-arm64",
		"tea-0.9.2-linux-amd64.sha256",
		"tea-0.9.2-linux-amd64.asc",
	}

	lefthookAssets = []string{
		"lefthook_1.6.10_Linux_x86_64.gz",
		"lefthook_1.6.10_Linux_arm64.gz",
		"lefthook_1.6.10_MacOS_x86_64.gz",
		"lefthook_1.6.10_MacOS_arm64.gz",
		"lefthook_1.6.10_Windows_x86_64.gz",
		"lefthook_1.6.10_checksums.txt",
		"lefthook-1.6.10.x86_64.rpm",
		"lefthook_1.6.10_amd64.deb",
	}

	hadolintAssets = []string{
		"hadolint-Linux-x86_64",
		"hadolint-Linux-arm64",
		"hadolint-Darwin-x86_64",
		"hadolint-Darwin-arm64",
		"hadolint-Windows-x86_64.exe",
	}

	tflintAssets = []string{
		"tflint_linux_amd64.zip",
		"tflint_linux_arm64.zip",
		"tflint_darwin_amd64.zip",
		"tflint_darwin_arm64.zip",
		"tflint_windows_amd64.zip",
		"checksums.txt",
		"checksums.txt.sig",
	}
)

func TestMatchAsset_Corpus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		assets []string
		host   platform.Info
		prefs  matchPrefs
		want   string
	}{
		{
			name:   "turso linux amd64 (case-insensitive OS+arch)",
			assets: tursoAssets,
			host:   linuxAMD64,
			prefs:  matchPrefs{name: "turso"},
			want:   "turso_Linux_x86_64.tar.gz",
		},
		{
			name:   "turso darwin arm64",
			assets: tursoAssets,
			host:   darwinARM,
			prefs:  matchPrefs{name: "turso"},
			want:   "turso_Darwin_arm64.tar.gz",
		},
		{
			name:   "tea bare binary, .sha256/.asc rejected",
			assets: teaAssets,
			host:   linuxAMD64,
			prefs:  matchPrefs{name: "tea"},
			want:   "tea-0.9.2-linux-amd64",
		},
		{
			name:   "tea darwin arm64 bare",
			assets: teaAssets,
			host:   darwinARM,
			prefs:  matchPrefs{name: "tea"},
			want:   "tea-0.9.2-darwin-arm64",
		},
		{
			name:   "lefthook .gz, MacOS alias + checksums.txt + deb/rpm rejected",
			assets: lefthookAssets,
			host:   linuxAMD64,
			prefs:  matchPrefs{name: "lefthook"},
			want:   "lefthook_1.6.10_Linux_x86_64.gz",
		},
		{
			name:   "lefthook MacOS alias resolves darwin host",
			assets: lefthookAssets,
			host:   darwinAMD,
			prefs:  matchPrefs{name: "lefthook"},
			want:   "lefthook_1.6.10_MacOS_x86_64.gz",
		},
		{
			name:   "hadolint bare, uppercase OS, x86_64",
			assets: hadolintAssets,
			host:   linuxAMD64,
			prefs:  matchPrefs{name: "hadolint"},
			want:   "hadolint-Linux-x86_64",
		},
		{
			name:   "tflint .zip, checksums.txt + .sig rejected",
			assets: tflintAssets,
			host:   linuxAMD64,
			prefs:  matchPrefs{name: "tflint"},
			want:   "tflint_linux_amd64.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := matchAsset(tt.assets, tt.host, tt.prefs)
			if err != nil {
				t.Fatalf("matchAsset() error = %v, want %q", err, tt.want)
			}
			if got != tt.want {
				t.Errorf("matchAsset() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The synthetic difficult page: gnu+musl variants, a completions look-alike
// that carries genuine OS/arch tokens, package/sbom/checksum/signature noise,
// and the arm/arm64 whole-token trap.
var difficultAssets = []string{
	"app-linux-x86_64-gnu.tar.gz",
	"app-linux-x86_64-musl.tar.gz",
	"app-linux-aarch64-gnu.tar.gz",
	"app-darwin-x86_64.tar.gz",
	"app-darwin-arm64.tar.gz",
	"app-completions-linux-x86_64.tar.gz",
	"app-linux-x86_64.deb",
	"app-linux-x86_64.rpm",
	"app-linux-x86_64.sbom",
	"SHA256SUMS",
	"SHA256SUMS.sig",
}

func TestMatchAsset_Difficult(t *testing.T) {
	t.Parallel()

	t.Run("default prefers gnu over musl", func(t *testing.T) {
		t.Parallel()
		got, err := matchAsset(difficultAssets, linuxAMD64, matchPrefs{name: "app", exclude: []string{"completions"}})
		if err != nil {
			t.Fatalf("matchAsset() error = %v", err)
		}
		if got != "app-linux-x86_64-gnu.tar.gz" {
			t.Errorf("matchAsset() = %q, want gnu build", got)
		}
	})

	t.Run("match.libc=musl flips the pick", func(t *testing.T) {
		t.Parallel()
		got, err := matchAsset(difficultAssets, linuxAMD64, matchPrefs{name: "app", libc: "musl", exclude: []string{"completions"}})
		if err != nil {
			t.Fatalf("matchAsset() error = %v", err)
		}
		if got != "app-linux-x86_64-musl.tar.gz" {
			t.Errorf("matchAsset() = %q, want musl build", got)
		}
	})

	t.Run("exclude drops the completions look-alike, resolving ambiguity", func(t *testing.T) {
		t.Parallel()
		// Two assets tie on libc(none)/format/name-closeness; without the
		// exclude the completions look-alike makes it ambiguous.
		assets := []string{
			"app-linux-x86_64.tar.gz",
			"app-completions-linux-x86_64.tar.gz",
		}
		if _, err := matchAsset(assets, linuxAMD64, matchPrefs{name: "app"}); err == nil {
			t.Fatal("matchAsset() error = nil, want ambiguous before exclude")
		}
		got, err := matchAsset(assets, linuxAMD64, matchPrefs{name: "app", exclude: []string{"completions"}})
		if err != nil {
			t.Fatalf("matchAsset() error = %v after exclude", err)
		}
		if got != "app-linux-x86_64.tar.gz" {
			t.Errorf("matchAsset() = %q, want the real binary", got)
		}
	})

	t.Run("genuinely ambiguous set errors with candidates", func(t *testing.T) {
		t.Parallel()
		// amd64 and x86_64 are aliases of the same host arch; same format, same
		// name-closeness, no libc token -> indistinguishable -> ambiguous.
		assets := []string{
			"app-linux-amd64.tar.gz",
			"app-linux-x86_64.tar.gz",
		}
		_, err := matchAsset(assets, linuxAMD64, matchPrefs{name: "app"})
		var ame *AssetMatchError
		if !errors.As(err, &ame) {
			t.Fatalf("matchAsset() error = %v, want *AssetMatchError", err)
		}
		if ame.Kind != "ambiguous" {
			t.Errorf("AssetMatchError.Kind = %q, want ambiguous", ame.Kind)
		}
		wantCandidates := []string{"app-linux-amd64.tar.gz", "app-linux-x86_64.tar.gz"}
		got := append([]string(nil), ame.Candidates...)
		sort.Strings(got)
		if len(got) != 2 || got[0] != wantCandidates[0] || got[1] != wantCandidates[1] {
			t.Errorf("AssetMatchError.Candidates = %v, want %v", ame.Candidates, wantCandidates)
		}
	})

	t.Run("no host match returns none", func(t *testing.T) {
		t.Parallel()
		assets := []string{"app-darwin-arm64.tar.gz", "app-windows-x86_64.zip"}
		_, err := matchAsset(assets, linuxAMD64, matchPrefs{name: "app"})
		var ame *AssetMatchError
		if !errors.As(err, &ame) {
			t.Fatalf("matchAsset() error = %v, want *AssetMatchError", err)
		}
		if ame.Kind != "none" {
			t.Errorf("AssetMatchError.Kind = %q, want none", ame.Kind)
		}
	})
}

func TestMatchAsset_ArchTokenTrap(t *testing.T) {
	t.Parallel()

	t.Run("arm64 host keeps arm64, rejects 32-bit arm and x86_64", func(t *testing.T) {
		t.Parallel()
		assets := []string{
			"tool-linux-arm64.tar.gz",
			"tool-linux-arm.tar.gz",
			"tool-linux-armv7.tar.gz",
			"tool-linux-x86_64.tar.gz",
		}
		got, err := matchAsset(assets, linuxARM64, matchPrefs{name: "tool"})
		if err != nil {
			t.Fatalf("matchAsset() error = %v, want the arm64 asset", err)
		}
		if got != "tool-linux-arm64.tar.gz" {
			t.Errorf("matchAsset() = %q, want tool-linux-arm64.tar.gz (arm must not disqualify arm64)", got)
		}
	})

	t.Run("aarch64 alias resolves on arm64 host", func(t *testing.T) {
		t.Parallel()
		assets := []string{"tool-linux-aarch64.tar.gz", "tool-linux-amd64.tar.gz"}
		got, err := matchAsset(assets, linuxARM64, matchPrefs{name: "tool"})
		if err != nil {
			t.Fatalf("matchAsset() error = %v", err)
		}
		if got != "tool-linux-aarch64.tar.gz" {
			t.Errorf("matchAsset() = %q, want the aarch64 asset", got)
		}
	})

	t.Run("x86_64 host rejects the aarch64 asset", func(t *testing.T) {
		t.Parallel()
		assets := []string{"tool-linux-x86_64.tar.gz", "tool-linux-aarch64.tar.gz"}
		got, err := matchAsset(assets, linuxAMD64, matchPrefs{name: "tool"})
		if err != nil {
			t.Fatalf("matchAsset() error = %v", err)
		}
		if got != "tool-linux-x86_64.tar.gz" {
			t.Errorf("matchAsset() = %q, want the x86_64 asset", got)
		}
	})
}

func TestMatchAsset_HyphenatedArch(t *testing.T) {
	t.Parallel()

	localSend := []string{"LocalSend-1.18.2-linux-arm-64.tar.gz", "LocalSend-1.18.2-linux-x86-64.tar.gz"}
	tests := []struct {
		name    string
		host    platform.Info
		assets  []string
		want    string
		wantErr bool
	}{
		{"arm-64 resolves on arm64", linuxARM64, localSend, "LocalSend-1.18.2-linux-arm-64.tar.gz", false},
		{"x86-64 resolves on x86_64", linuxAMD64, localSend, "LocalSend-1.18.2-linux-x86-64.tar.gz", false},
		{"arm-64 is not a 32-bit arm asset", platform.Info{OS: platform.OSDebian, Arch: platform.ArchARM},
			localSend, "", true},
		// Auto-detect has no author-written pattern to narrow the release, so a
		// name carrying an independent foreign-arch token still fails closed.
		{"independent foreign token still rejects", linuxAMD64,
			[]string{"arm-gnu-toolchain-14-linux-x86_64-arm-none-eabi.tar.gz"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := matchAsset(tc.assets, tc.host, matchPrefs{})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("matchAsset() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("matchAsset() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("matchAsset() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMatchAsset_AppleTriple(t *testing.T) {
	t.Parallel()

	// Rust-triple naming: x86_64-apple-darwin. The apple/darwin aliases both
	// match a darwin host.
	assets := []string{
		"app-x86_64-apple-darwin.tar.gz",
		"app-x86_64-unknown-linux-gnu.tar.gz",
	}
	got, err := matchAsset(assets, darwinAMD, matchPrefs{name: "app"})
	if err != nil {
		t.Fatalf("matchAsset() error = %v", err)
	}
	if got != "app-x86_64-apple-darwin.tar.gz" {
		t.Errorf("matchAsset() = %q, want the apple-darwin asset", got)
	}
}

func TestForeignArchTokens(t *testing.T) {
	tests := []struct {
		host        platform.Arch
		wantForeign []string
		wantHost    []string
	}{
		{platform.ArchX86_64, []string{"arm64", "aarch64", "arm", "armv7l", "armel", "i386", "i686", "riscv64", "s390x"}, []string{"x86_64", "x86-64", "amd64", "x64"}},
		{platform.ArchARM64, []string{"x86_64", "x86-64", "amd64", "x64", "arm", "armv7l", "i386", "riscv64"}, []string{"arm64", "aarch64", "arm-64"}},
		{platform.ArchARM, []string{"x86_64", "arm64", "aarch64", "i386", "riscv64"}, []string{"arm", "armv7", "armv7l", "armv6l", "armhf", "armel"}},
		// An arch dotf has no canonical name for must not reject its own assets.
		{platform.Arch("riscv64"), []string{"x86_64", "arm64", "arm", "i386", "s390x"}, []string{"riscv64"}},
	}
	for _, tc := range tests {
		t.Run(string(tc.host), func(t *testing.T) {
			got := foreignArchTokens(tc.host)
			for _, token := range tc.wantForeign {
				if !slices.Contains(got, token) {
					t.Errorf("foreignArchTokens(%s) lacks %q: %v", tc.host, token, got)
				}
			}
			for _, token := range tc.wantHost {
				if slices.Contains(got, token) {
					t.Errorf("foreignArchTokens(%s) contains host token %q: %v", tc.host, token, got)
				}
			}
		})
	}
}
