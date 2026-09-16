package backend

import (
	"path/filepath"
	"testing"
)

func TestContainedJoin_OK(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	got, err := containedJoin(root, "a/b/c")
	if err != nil {
		t.Fatalf("containedJoin() error = %v, want nil", err)
	}
	if want := filepath.Join(root, "a", "b", "c"); got != want {
		t.Errorf("containedJoin() = %q, want %q", got, want)
	}
}

func TestContainedJoin_Rejects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tests := []struct {
		name string
		rel  string
	}{
		{name: "empty", rel: ""},
		{name: "absolute", rel: "/etc/passwd"},
		{name: "parent", rel: ".."},
		{name: "leading parent", rel: "../escape"},
		{name: "embedded traversal", rel: "a/../../b"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := containedJoin(root, tc.rel); err == nil {
				t.Errorf("containedJoin(%q) error = nil, want rejection", tc.rel)
			}
		})
	}
}
