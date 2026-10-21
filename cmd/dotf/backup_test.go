package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yandy-r/dotfiles/internal/symlink"
)

// seedBackup creates ~/.config/dotfiles_backup_<id> under home holding files
// (relative path -> content) and, unless legacy, a manifest listing them as
// file entries created at created.
func seedBackup(t *testing.T, home, id string, created time.Time, legacy bool, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(symlink.BackupRoot(home), symlink.BackupDirPrefix+id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := symlink.BackupManifest{Version: 1, CreatedAt: created.UTC()}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		m.Entries = append(m.Entries, symlink.BackupEntry{Dest: rel, Kind: symlink.BackupKindFile, BackedUpAt: created.UTC()})
	}
	if !legacy {
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, symlink.BackupManifestName), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// backupHome points HOME at a fresh temp dir for one test.
func backupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func runBackupT(t *testing.T, stdin io.Reader, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	code = runBackup(args, &out, &errb, stdin)
	return out.String(), errb.String(), code
}

func TestRunBackup_UsageErrors(t *testing.T) {
	backupHome(t)
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no verb", nil, "requires a verb"},
		{"unknown verb", []string{"bogus"}, `unknown verb "bogus"`},
		{"bad flag", []string{"list", "--bogus"}, "flag provided but not defined"},
		{"list extra arg", []string{"list", "x"}, "takes no arguments"},
		{"restore without id", []string{"restore"}, "requires a backup id"},
		{"prune extra arg", []string{"prune", "x"}, "takes no arguments"},
		{"force on list", []string{"list", "--force"}, "--force does not apply"},
		{"yes on restore", []string{"restore", "1", "--yes"}, "--yes does not apply"},
		{"keep on restore", []string{"restore", "1", "--keep", "3"}, "--keep does not apply"},
		{"dry-run on list", []string{"list", "--dry-run"}, "--dry-run does not apply"},
		{"negative keep", []string{"prune", "--keep", "-1"}, "--keep must not be negative"},
		{"bad older-than", []string{"prune", "--older-than", "soon"}, "invalid --older-than"},
		{"older-than days overflow", []string{"prune", "--older-than", "106752d"}, "out of range"},
		{"older-than days huge", []string{"prune", "--older-than", "99999999999999999999d"}, "invalid --older-than"},
		{"older-than negative days", []string{"prune", "--older-than", "-1d"}, "invalid --older-than"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runBackupT(t, nil, tc.args...)
			if code != 2 {
				t.Errorf("code = %d, want 2 (stderr=%q)", code, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.want)
			}
		})
	}
}

func TestParseOlderThan(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"0d", 0, false},
		{"720h", 720 * time.Hour, false},
		{"90m", 90 * time.Minute, false},
		{"-1h", 0, true},
		{"d", 0, true},
		{"3w", 0, true},
		{"106751d", 106751 * 24 * time.Hour, false},
		{"106752d", 0, true},
		{"9223372036854775808d", 0, true},
		{"-1d", 0, true},
	}
	for _, tc := range cases {
		got, err := parseOlderThan(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("parseOlderThan(%q) = %v, %v; want %v, err=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:                      "0 B",
		1023:                   "1023 B",
		1024:                   "1.0 KiB",
		1536:                   "1.5 KiB",
		3565158:                "3.4 MiB",
		5 * 1024 * 1024 * 1024: "5.0 GiB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestRunBackup_ListEmpty(t *testing.T) {
	backupHome(t)
	stdout, _, code := runBackupT(t, nil, "list")
	if code != 0 || stdout != "backup.count=0 backup.size=0\n" {
		t.Errorf("list (empty) = %d %q", code, stdout)
	}
}

func TestRunBackup_ListNewestFirstWithLegacy(t *testing.T) {
	home := backupHome(t)
	now := time.Unix(1_800_000_000, 0)
	seedBackup(t, home, "1700000000", time.Unix(1_700_000_000, 0), true, map[string]string{".zshrc": "z"})
	seedBackup(t, home, strconv.FormatInt(now.Unix(), 10), now, false, map[string]string{".bashrc": "bb", ".gitconfig": "g"})
	// A non-matching sibling is never listed.
	if err := os.MkdirAll(filepath.Join(home, ".config", "dotfiles_backup_x"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runBackupT(t, nil, "list")
	if code != 0 {
		t.Fatalf("code = %d, stderr=%q", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q, want 2 sets + summary", lines)
	}
	if !strings.HasPrefix(lines[0], "backup.id=1800000000 ") || !strings.Contains(lines[0], "entries=2 ") ||
		!strings.Contains(lines[0], "legacy=false") {
		t.Errorf("first line = %q, want the newest managed set", lines[0])
	}
	if !strings.HasPrefix(lines[1], "backup.id=1700000000 ") || !strings.Contains(lines[1], "legacy=true") ||
		!strings.Contains(lines[1], "entries=0 ") {
		t.Errorf("second line = %q, want the legacy set", lines[1])
	}
	if !strings.HasPrefix(lines[2], "backup.count=2 ") {
		t.Errorf("summary = %q", lines[2])
	}
	if !strings.Contains(stderr, "2 backups") {
		t.Errorf("stderr = %q, want a human summary", stderr)
	}

	stdout, _, code = runBackupT(t, nil, "list", "--json")
	if code != 0 {
		t.Fatalf("--json code = %d", code)
	}
	var doc backupListJSON
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout)
	}
	if len(doc.Backups) != 2 || doc.Backups[0].ID != "1800000000" || doc.Backups[0].EntryCount != 2 ||
		!doc.Backups[1].Legacy {
		t.Errorf("json backups = %+v", doc.Backups)
	}
}

func TestRunBackup_Restore(t *testing.T) {
	t.Run("missing dest is restored, backup left intact", func(t *testing.T) {
		home := backupHome(t)
		dir := seedBackup(t, home, "100", time.Unix(100, 0), false, map[string]string{".bashrc": "orig\n"})
		stdout, stderr, code := runBackupT(t, nil, "restore", "100")
		if code != 0 {
			t.Fatalf("code = %d stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "restore.create=.bashrc kind=file") {
			t.Errorf("stdout = %q", stdout)
		}
		if got, _ := os.ReadFile(filepath.Join(home, ".bashrc")); string(got) != "orig\n" {
			t.Errorf(".bashrc = %q", got)
		}
		if _, err := os.Stat(filepath.Join(dir, ".bashrc")); err != nil {
			t.Errorf("backup copy gone: %v", err)
		}
	})

	t.Run("real file refused without force, plan printed", func(t *testing.T) {
		home := backupHome(t)
		seedBackup(t, home, "100", time.Unix(100, 0), false, map[string]string{".bashrc": "orig\n"})
		seedRepoFile(t, home, ".bashrc", "current\n")
		stdout, stderr, code := runBackupT(t, nil, "restore", "100")
		if code != 1 {
			t.Fatalf("code = %d, want 1", code)
		}
		if !strings.Contains(stdout, "restore.conflict=.bashrc") || !strings.Contains(stderr, "--force") {
			t.Errorf("stdout=%q stderr=%q", stdout, stderr)
		}
		if got, _ := os.ReadFile(filepath.Join(home, ".bashrc")); string(got) != "current\n" {
			t.Errorf(".bashrc changed to %q", got)
		}
	})

	t.Run("dry-run writes nothing", func(t *testing.T) {
		home := backupHome(t)
		seedBackup(t, home, "100", time.Unix(100, 0), false, map[string]string{".bashrc": "orig\n"})
		stdout, _, code := runBackupT(t, nil, "restore", "--dry-run", "100")
		if code != 0 || !strings.HasPrefix(stdout, "dry-run=true\n") {
			t.Fatalf("code=%d stdout=%q", code, stdout)
		}
		if _, err := os.Lstat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
			t.Errorf("dry-run created .bashrc (err=%v)", err)
		}
	})

	t.Run("force backs current content up first", func(t *testing.T) {
		home := backupHome(t)
		seedBackup(t, home, "100", time.Unix(100, 0), false, map[string]string{".bashrc": "orig\n"})
		seedRepoFile(t, home, ".bashrc", "current\n")
		stdout, stderr, code := runBackupT(t, nil, "restore", "100", "--force", "--json")
		if code != 0 {
			t.Fatalf("code = %d stderr=%q", code, stderr)
		}
		var doc backupRestoreJSON
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatal(err)
		}
		if doc.SafetyBackupDir == "" || len(doc.Items) != 1 || doc.Items[0].Action != symlink.RestoreActionReplaceReal {
			t.Fatalf("doc = %+v", doc)
		}
		if got, _ := os.ReadFile(filepath.Join(doc.SafetyBackupDir, ".bashrc")); string(got) != "current\n" {
			t.Errorf("safety copy = %q", got)
		}
		if got, _ := os.ReadFile(filepath.Join(home, ".bashrc")); string(got) != "orig\n" {
			t.Errorf(".bashrc = %q", got)
		}
	})

	t.Run("subset and unknown path", func(t *testing.T) {
		home := backupHome(t)
		seedBackup(t, home, "100", time.Unix(100, 0), false, map[string]string{".bashrc": "b\n", ".zshrc": "z\n"})
		if _, stderr, code := runBackupT(t, nil, "restore", "100", "~/.zshrc"); code != 0 {
			t.Fatalf("code = %d stderr=%q", code, stderr)
		}
		if _, err := os.Lstat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
			t.Error("subset restore also restored .bashrc")
		}
		if _, _, code := runBackupT(t, nil, "restore", "100", ".nope"); code != 1 {
			t.Errorf("unknown path code = %d, want 1", code)
		}
	})

	t.Run("legacy and missing ids refused", func(t *testing.T) {
		home := backupHome(t)
		seedBackup(t, home, "100", time.Unix(100, 0), true, map[string]string{".bashrc": "b\n"})
		if _, stderr, code := runBackupT(t, nil, "restore", "100"); code != 1 || !strings.Contains(stderr, "legacy") {
			t.Errorf("legacy: code=%d stderr=%q", code, stderr)
		}
		if _, stderr, code := runBackupT(t, nil, "restore", "999"); code != 1 || !strings.Contains(stderr, "not found") {
			t.Errorf("missing: code=%d stderr=%q", code, stderr)
		}
		if _, _, code := runBackupT(t, nil, "restore", "../x"); code != 1 {
			t.Errorf("path-separator id code = %d, want 1", code)
		}
	})
}

// seedPruneSets creates n managed sets with ids 1..n (n newest) and returns
// their dirs, oldest first.
func seedPruneSets(t *testing.T, home string, n int) []string {
	t.Helper()
	dirs := make([]string, n)
	for i := 1; i <= n; i++ {
		dirs[i-1] = seedBackup(t, home, strconv.Itoa(i), time.Unix(int64(i), 0), false, map[string]string{".x": "x"})
	}
	return dirs
}

func countExisting(dirs []string) int {
	n := 0
	for _, d := range dirs {
		if _, err := os.Stat(d); err == nil {
			n++
		}
	}
	return n
}

func TestRunBackup_Prune(t *testing.T) {
	t.Run("non-interactive without yes refuses", func(t *testing.T) {
		home := backupHome(t)
		dirs := seedPruneSets(t, home, 4)
		_, stderr, code := runBackupT(t, nil, "prune", "--keep", "1")
		if code != 1 || !strings.Contains(stderr, "without --yes") {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if countExisting(dirs) != 4 {
			t.Error("refused prune deleted something")
		}
	})

	t.Run("dry-run deletes nothing", func(t *testing.T) {
		home := backupHome(t)
		dirs := seedPruneSets(t, home, 4)
		stdout, _, code := runBackupT(t, nil, "prune", "--keep", "1", "--dry-run")
		if code != 0 || !strings.Contains(stdout, "prune.removed=3 prune.kept=1") {
			t.Fatalf("code=%d stdout=%q", code, stdout)
		}
		if countExisting(dirs) != 4 {
			t.Error("dry-run deleted something")
		}
	})

	t.Run("yes removes all but the newest keep", func(t *testing.T) {
		home := backupHome(t)
		dirs := seedPruneSets(t, home, 4)
		stdout, _, code := runBackupT(t, nil, "prune", "--keep", "2", "--yes")
		if code != 0 || !strings.Contains(stdout, "prune.removed=2 prune.kept=2") {
			t.Fatalf("code=%d stdout=%q", code, stdout)
		}
		if countExisting(dirs[:2]) != 0 || countExisting(dirs[2:]) != 2 {
			t.Error("wrong sets removed")
		}
	})

	t.Run("default keep is 10", func(t *testing.T) {
		home := backupHome(t)
		dirs := seedPruneSets(t, home, 12)
		if _, _, code := runBackupT(t, nil, "prune", "--yes"); code != 0 {
			t.Fatalf("code=%d", code)
		}
		if countExisting(dirs) != symlink.DefaultBackupKeep {
			t.Errorf("remaining = %d, want %d", countExisting(dirs), symlink.DefaultBackupKeep)
		}
	})

	t.Run("older-than limits candidates", func(t *testing.T) {
		home := backupHome(t)
		seedBackup(t, home, "1", time.Now().Add(-48*time.Hour), false, map[string]string{".x": "x"})
		recent := seedBackup(t, home, "2", time.Now(), false, map[string]string{".x": "x"})
		stdout, _, code := runBackupT(t, nil, "prune", "--keep", "0", "--older-than", "1d", "--yes", "--json")
		if code != 0 {
			t.Fatalf("code=%d", code)
		}
		var doc backupPruneJSON
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Removed) != 1 || doc.Removed[0].ID != "1" || doc.Kept != 1 {
			t.Errorf("doc = %+v", doc)
		}
		if _, err := os.Stat(recent); err != nil {
			t.Error("recent set removed")
		}
	})

	t.Run("interactive confirmation", func(t *testing.T) {
		orig := backupInteractive
		t.Cleanup(func() { backupInteractive = orig })
		backupInteractive = func(io.Reader, io.Writer) bool { return true }

		home := backupHome(t)
		dirs := seedPruneSets(t, home, 3)
		stdout, stderr, code := runBackupT(t, strings.NewReader("n\n"), "prune", "--keep", "1")
		if code != 1 || !strings.Contains(stdout, backupPrunePrompt) || !strings.Contains(stderr, "not confirmed") {
			t.Fatalf("decline: code=%d stdout=%q stderr=%q", code, stdout, stderr)
		}
		if countExisting(dirs) != 3 {
			t.Fatal("declined prune deleted something")
		}
		if _, _, code := runBackupT(t, strings.NewReader("y\n"), "prune", "--keep", "1"); code != 0 {
			t.Fatalf("confirm: code=%d", code)
		}
		if countExisting(dirs) != 1 {
			t.Errorf("remaining = %d, want 1", countExisting(dirs))
		}
	})

	t.Run("interactive json keeps stdout pure", func(t *testing.T) {
		orig := backupInteractive
		t.Cleanup(func() { backupInteractive = orig })
		backupInteractive = func(io.Reader, io.Writer) bool { return true }

		home := backupHome(t)
		dirs := seedPruneSets(t, home, 3)
		stdout, stderr, code := runBackupT(t, strings.NewReader("y\n"), "prune", "--keep", "1", "--json")
		if code != 0 {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if !strings.Contains(stderr, backupPrunePrompt) || !strings.Contains(stderr, filepath.Base(dirs[0])[len(symlink.BackupDirPrefix):]) {
			t.Errorf("stderr = %q, want victim list and prompt", stderr)
		}
		var doc backupPruneJSON
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout is not pure JSON: %v\n%s", err, stdout)
		}
		if len(doc.Removed) != 2 || countExisting(dirs) != 1 {
			t.Errorf("doc = %+v remaining=%d", doc, countExisting(dirs))
		}
	})
}

func TestCheckBackups(t *testing.T) {
	t.Run("zero is ok", func(t *testing.T) {
		c := checkBackups(t.TempDir())
		if c.Category != "backups" || c.Name != "count" || c.Status != checkOK || c.Detail != "0 backups, 0 B" {
			t.Errorf("check = %+v", c)
		}
	})
	t.Run("ten is ok", func(t *testing.T) {
		home := t.TempDir()
		seedPruneSets(t, home, symlink.DefaultBackupKeep)
		if c := checkBackups(home); c.Status != checkOK {
			t.Errorf("check = %+v, want ok", c)
		}
	})
	t.Run("eleven warns with prune remedy", func(t *testing.T) {
		home := t.TempDir()
		seedPruneSets(t, home, symlink.DefaultBackupKeep+1)
		c := checkBackups(home)
		if c.Status != checkWarn || !strings.Contains(c.Remedy, "dotf backup prune") ||
			!strings.HasPrefix(c.Detail, "11 backups, ") {
			t.Errorf("check = %+v", c)
		}
	})
}
