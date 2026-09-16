package privilege

// FakeDropRun records a single Dropper invocation for test assertions.
type FakeDropRun struct {
	Argv []string
	Home string
	UID  uint32
}

// FakeDropper is a test double for Dropper that records dropped argv and uid.
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
	if u, err := RealUser(); err == nil {
		uid = u.UID
	}

	f.Runs = append(f.Runs, FakeDropRun{
		Argv: append([]string(nil), argv...),
		Home: home,
		UID:  uid,
	})
	return f.Err
}

var _ Dropper = (*FakeDropper)(nil)
