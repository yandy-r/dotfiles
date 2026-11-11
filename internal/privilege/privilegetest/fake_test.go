package privilegetest

import (
	"os"
	"testing"
)

func TestFakeDropper_recordsArgvAndUID(t *testing.T) {
	f := NewFakeDropper()
	argv := []string{"makepkg", "-si"}

	if err := f.Run(argv, "/home/test"); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(f.Runs) != 1 {
		t.Fatalf("len(Runs) = %d, want 1", len(f.Runs))
	}
	if got, want := f.Runs[0].Argv, argv; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Runs[0].Argv = %v, want %v", got, want)
	}
	if f.Runs[0].Home != "/home/test" {
		t.Errorf("Runs[0].Home = %q, want %q", f.Runs[0].Home, "/home/test")
	}
	if f.Runs[0].UID == 0 && os.Geteuid() != 0 {
		t.Errorf("Runs[0].UID = 0, want current user uid")
	}
}
