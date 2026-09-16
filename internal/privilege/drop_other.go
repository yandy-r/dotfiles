//go:build !unix

package privilege

import "errors"

// ErrUnsupportedPlatform is returned when privilege drop is unavailable.
var ErrUnsupportedPlatform = errors.New("privilege: drop not supported on this platform")

// Dropper runs commands as the real user when the current process is root.
type Dropper interface {
	Run(argv []string, home string) error
}

type stubDropper struct{}

// NewDropper returns a Dropper stub on non-unix platforms.
func NewDropper() Dropper {
	return stubDropper{}
}

func (stubDropper) Run([]string, string) error {
	return ErrUnsupportedPlatform
}

var _ Dropper = stubDropper{}
