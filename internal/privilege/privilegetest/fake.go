// Package privilegetest provides test doubles for the privilege package. It is
// imported only by _test.go files, so the double never links into the dotf
// binary.
package privilegetest

import "github.com/yandy-r/dotfiles/internal/privilege"

// FakeDropRun records a single Dropper invocation for test assertions.
type FakeDropRun struct {
	Argv []string
	Home string
	UID  uint32
}

// FakeDropper is a test double for privilege.Dropper that records dropped argv
// and uid.
type FakeDropper struct {
	Runs []FakeDropRun
	Err  error
}

// NewFakeDropper returns an empty FakeDropper.
func NewFakeDropper() *FakeDropper {
	return &FakeDropper{}
}

// Run records argv and home, resolving RealUser for uid when euid is zero.
func (f *FakeDropper) Run(argv []string, home string) error {
	uid := uint32(0)
	if u, err := privilege.RealUser(); err == nil {
		uid = u.UID
	}

	f.Runs = append(f.Runs, FakeDropRun{
		Argv: append([]string(nil), argv...),
		Home: home,
		UID:  uid,
	})
	return f.Err
}

var _ privilege.Dropper = (*FakeDropper)(nil)
