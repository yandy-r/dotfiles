package sysconfig

import (
	"strings"
	"testing"

	"github.com/yandy-r/dotfiles/internal/exec"
	"github.com/yandy-r/dotfiles/internal/platform"
)

func TestHostsNeedsUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		host    string
		want    bool
	}{
		{name: "no 127.0.1.1 line", content: "127.0.0.1 localhost\n", host: "web01", want: true},
		{name: "wrong hostname", content: "127.0.0.1 localhost\n127.0.1.1 old\n", host: "web01", want: true},
		{name: "already correct", content: "127.0.0.1 localhost\n127.0.1.1 web01\n", host: "web01", want: false},
		{name: "tab separated correct", content: "127.0.1.1\tweb01\n", host: "web01", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hostsNeedsUpdate([]byte(tt.content), tt.host); got != tt.want {
				t.Errorf("hostsNeedsUpdate(%q, %q) = %v, want %v", tt.content, tt.host, got, tt.want)
			}
		})
	}
}

func TestRenderHosts_ReplacesNotDuplicates(t *testing.T) {
	t.Parallel()

	content := []byte("127.0.0.1\tlocalhost\n127.0.1.1\toldname\n::1\tlocalhost\n")
	got := string(renderHosts(content, "web01"))

	if strings.Count(got, "127.0.1.1") != 1 {
		t.Errorf("renderHosts duplicated 127.0.1.1:\n%s", got)
	}
	if !strings.Contains(got, "127.0.1.1\tweb01") {
		t.Errorf("renderHosts missing 127.0.1.1\\tweb01:\n%s", got)
	}
	if strings.Contains(got, "oldname") {
		t.Errorf("renderHosts left stale oldname:\n%s", got)
	}
	if !strings.Contains(got, "127.0.0.1\tlocalhost") {
		t.Errorf("renderHosts dropped 127.0.0.1 localhost:\n%s", got)
	}
	if !strings.Contains(got, "::1\tlocalhost") {
		t.Errorf("renderHosts dropped ::1 localhost:\n%s", got)
	}
}

func TestRenderHosts_AppendsWhenAbsent(t *testing.T) {
	t.Parallel()

	content := []byte("127.0.0.1\tlocalhost\n")
	got := string(renderHosts(content, "web01"))

	if !strings.Contains(got, "127.0.0.1\tlocalhost") {
		t.Errorf("renderHosts dropped existing line:\n%s", got)
	}
	if !strings.HasSuffix(got, "127.0.1.1\tweb01\n") {
		t.Errorf("renderHosts did not append trailing 127.0.1.1 line:\n%q", got)
	}
}

func TestEtcShellsHasPath(t *testing.T) {
	t.Parallel()

	content := []byte("/bin/sh\n/bin/bash\n/usr/bin/zsh\n")
	if !etcShellsHasPath(content, "/usr/bin/zsh") {
		t.Error("etcShellsHasPath(/usr/bin/zsh) = false, want true")
	}
	if etcShellsHasPath(content, "/opt/homebrew/bin/zsh") {
		t.Error("etcShellsHasPath(/opt/homebrew/bin/zsh) = true, want false")
	}
}

func TestRenderEtcShells(t *testing.T) {
	t.Parallel()

	t.Run("appends when absent", func(t *testing.T) {
		t.Parallel()
		got := string(renderEtcShells([]byte("/bin/sh\n/bin/bash\n"), "/usr/bin/zsh"))
		if !strings.HasSuffix(got, "/usr/bin/zsh\n") {
			t.Errorf("renderEtcShells did not append path:\n%q", got)
		}
	})

	t.Run("unchanged when present", func(t *testing.T) {
		t.Parallel()
		content := []byte("/bin/sh\n/usr/bin/zsh\n")
		got := renderEtcShells(content, "/usr/bin/zsh")
		if string(got) != string(content) {
			t.Errorf("renderEtcShells modified content when path present: %q", got)
		}
	})
}

func TestResolveShellPath(t *testing.T) {
	t.Parallel()

	linux := platform.Info{OS: platform.OSDebian}
	mac := platform.Info{OS: platform.OSMacOS}

	t.Run("existing absolute path returned as-is", func(t *testing.T) {
		t.Parallel()
		fake := exec.NewFake()
		fake.AddPath("/usr/bin/zsh", "/usr/bin/zsh")
		got, err := resolveShellPath(fake, "/usr/bin/zsh", linux)
		if err != nil {
			t.Fatalf("resolveShellPath() error = %v", err)
		}
		if got != "/usr/bin/zsh" {
			t.Errorf("resolveShellPath(abs) = %q, want /usr/bin/zsh", got)
		}
	})

	t.Run("nonexistent absolute path not blindly returned", func(t *testing.T) {
		t.Parallel()
		// The absolute path is NOT registered → LookPath fails → resolves to ""
		// (shell domain skipped) rather than being returned as a login shell
		// that would lock the user out.
		got, err := resolveShellPath(exec.NewFake(), "/usr/bin/does-not-exist", linux)
		if err != nil {
			t.Fatalf("resolveShellPath() error = %v", err)
		}
		if got != "" {
			t.Errorf("resolveShellPath(missing abs) = %q, want empty (skip)", got)
		}
	})

	t.Run("bare name resolved via LookPath", func(t *testing.T) {
		t.Parallel()
		fake := exec.NewFake()
		fake.AddPath("zsh", "/usr/bin/zsh")
		got, err := resolveShellPath(fake, "zsh", linux)
		if err != nil {
			t.Fatalf("resolveShellPath() error = %v", err)
		}
		if got != "/usr/bin/zsh" {
			t.Errorf("resolveShellPath(zsh) = %q, want /usr/bin/zsh", got)
		}
	})

	t.Run("linux candidate fallback", func(t *testing.T) {
		t.Parallel()
		fake := exec.NewFake()
		fake.AddPath("/home/linuxbrew/.linuxbrew/bin/zsh", "/home/linuxbrew/.linuxbrew/bin/zsh")
		got, err := resolveShellPath(fake, "zsh", linux)
		if err != nil {
			t.Fatalf("resolveShellPath() error = %v", err)
		}
		if got != "/home/linuxbrew/.linuxbrew/bin/zsh" {
			t.Errorf("resolveShellPath(zsh) = %q, want linuxbrew candidate", got)
		}
	})

	t.Run("macos candidate fallback", func(t *testing.T) {
		t.Parallel()
		fake := exec.NewFake()
		fake.AddPath("/opt/homebrew/bin/zsh", "/opt/homebrew/bin/zsh")
		got, err := resolveShellPath(fake, "zsh", mac)
		if err != nil {
			t.Fatalf("resolveShellPath() error = %v", err)
		}
		if got != "/opt/homebrew/bin/zsh" {
			t.Errorf("resolveShellPath(zsh) = %q, want homebrew candidate", got)
		}
	})

	t.Run("not found returns empty", func(t *testing.T) {
		t.Parallel()
		got, err := resolveShellPath(exec.NewFake(), "fish", linux)
		if err != nil {
			t.Fatalf("resolveShellPath() error = %v", err)
		}
		if got != "" {
			t.Errorf("resolveShellPath(fish) = %q, want empty", got)
		}
	})
}

func TestNormalizeLocale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "en_US.UTF-8", want: "en_us.utf8"},
		{in: "en_us.utf8", want: "en_us.utf8"},
		{in: "  C.UTF-8  ", want: "c.utf8"},
		{in: "POSIX", want: "posix"},
	}
	for _, tt := range tests {
		if got := normalizeLocale(tt.in); got != tt.want {
			t.Errorf("normalizeLocale(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLocaleAvailable_NormalizedMatch(t *testing.T) {
	t.Parallel()

	avail := []byte("C\nC.UTF-8\nen_us.utf8\nPOSIX\n")
	if !localeAvailable(avail, "en_US.UTF-8") {
		t.Error("localeAvailable(en_US.UTF-8) = false, want true (utf-8 == utf8, case-insensitive)")
	}
	if localeAvailable(avail, "de_DE.UTF-8") {
		t.Error("localeAvailable(de_DE.UTF-8) = true, want false")
	}
}

func TestResolveLocale_FallbackPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		avail     string
		want      string
	}{
		{name: "requested available", requested: "en_GB.UTF-8", avail: "en_GB.UTF-8\nC\n", want: "en_GB.UTF-8"},
		{name: "fall to en_US", requested: "de_DE.UTF-8", avail: "en_US.UTF-8\nC.UTF-8\nC\n", want: "en_US.UTF-8"},
		{name: "fall to C.UTF-8", requested: "de_DE.UTF-8", avail: "C\nC.UTF-8\nPOSIX\n", want: "C.UTF-8"},
		{name: "fall to C when nothing else", requested: "de_DE.UTF-8", avail: "POSIX\n", want: "C"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveLocale(tt.requested, []byte(tt.avail)); got != tt.want {
				t.Errorf("resolveLocale(%q) = %q, want %q", tt.requested, got, tt.want)
			}
		})
	}
}

func TestIsBuiltinLocale(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"C", "C.UTF-8", "POSIX", "c.utf8"} {
		if !isBuiltinLocale(s) {
			t.Errorf("isBuiltinLocale(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"en_US.UTF-8", "de_DE.UTF-8"} {
		if isBuiltinLocale(s) {
			t.Errorf("isBuiltinLocale(%q) = true, want false", s)
		}
	}
}

func TestUncommentLocaleGen(t *testing.T) {
	t.Parallel()

	t.Run("uncomments the target line", func(t *testing.T) {
		t.Parallel()
		content := []byte("#en_US.UTF-8 UTF-8\n#de_DE.UTF-8 UTF-8\n")
		got, changed := uncommentLocaleGen(content, "en_US.UTF-8")
		if !changed {
			t.Fatal("uncommentLocaleGen changed = false, want true")
		}
		s := string(got)
		if !strings.Contains(s, "\nen_US.UTF-8 UTF-8") && !strings.HasPrefix(s, "en_US.UTF-8 UTF-8") {
			t.Errorf("uncommentLocaleGen did not uncomment target:\n%s", s)
		}
		if !strings.Contains(s, "#de_DE.UTF-8 UTF-8") {
			t.Errorf("uncommentLocaleGen touched an unrelated line:\n%s", s)
		}
	})

	t.Run("no change when already uncommented", func(t *testing.T) {
		t.Parallel()
		content := []byte("en_US.UTF-8 UTF-8\n")
		_, changed := uncommentLocaleGen(content, "en_US.UTF-8")
		if changed {
			t.Error("uncommentLocaleGen changed = true, want false (already uncommented)")
		}
	})
}
