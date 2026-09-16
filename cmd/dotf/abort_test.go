package main

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/yandy-r/dotfiles/internal/tui"
)

func TestReportAbort(t *testing.T) {
	var stderr bytes.Buffer
	code := reportAbort(&stderr)

	if code != 130 {
		t.Errorf("reportAbort code = %d, want 130", code)
	}
	if got, want := stderr.String(), "dotf: aborted\n"; got != want {
		t.Errorf("reportAbort stderr = %q, want %q", got, want)
	}
}

func TestWizardExit(t *testing.T) {
	t.Run("abort maps to 130 and clean line", func(t *testing.T) {
		var stderr bytes.Buffer
		code := wizardExit(&stderr, tui.ErrAborted)
		if code != 130 {
			t.Errorf("code = %d, want 130", code)
		}
		if got, want := stderr.String(), "dotf: aborted\n"; got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})

	t.Run("wrapped abort still classified", func(t *testing.T) {
		var stderr bytes.Buffer
		code := wizardExit(&stderr, fmt.Errorf("run wizard: %w", tui.ErrAborted))
		if code != 130 {
			t.Errorf("code = %d, want 130", code)
		}
	})

	t.Run("real error maps to 1 with diagnostic", func(t *testing.T) {
		var stderr bytes.Buffer
		code := wizardExit(&stderr, errors.New("boom"))
		if code != 1 {
			t.Errorf("code = %d, want 1", code)
		}
		if got, want := stderr.String(), "dotf: boom\n"; got != want {
			t.Errorf("stderr = %q, want %q", got, want)
		}
	})
}
