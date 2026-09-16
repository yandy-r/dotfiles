package exec

import (
	"fmt"
	osexec "os/exec"
	"strings"
	"sync"
)

// FakeResult is the canned (output, error) pair returned for a registered
// command invocation.
type FakeResult struct {
	Out []byte
	Err error
}

// FakeCall records a single invocation made through Fake.Output, in order,
// so tests can assert on what was actually run.
type FakeCall struct {
	// Dir is the working directory for a RunDir invocation, empty for
	// Output/Run (which do not set a working directory).
	Dir  string
	Name string
	Args []string
}

// Fake is a test double implementing Runner. Callers register expected
// command invocations (by name + args) and the canned bytes/error to return,
// plus an optional set of names that LookPath should resolve. Fake is safe
// to use from other packages' tests.
type Fake struct {
	mu sync.Mutex

	// Commands maps a command key (see CommandKey) to the result Output
	// should return for that invocation.
	Commands map[string]FakeResult

	// Found maps a command name to the path LookPath should report it
	// resolves to. Names absent from this map are treated as not found,
	// matching `command -v` semantics.
	Found map[string]string

	// RunErrors maps a command key (see CommandKey) to the error Run should
	// return for that invocation. Keys absent from this map succeed (nil error).
	RunErrors map[string]error

	// Calls records every invocation made through Output or Run, in call order.
	Calls []FakeCall
}

// NewFake returns an empty, ready-to-use Fake.
func NewFake() *Fake {
	return &Fake{
		Commands:  make(map[string]FakeResult),
		Found:     make(map[string]string),
		RunErrors: make(map[string]error),
	}
}

// CommandKey builds the lookup key used to match a command invocation by
// name and args. Exposed so callers can build keys identically when
// registering commands directly via the Commands map.
func CommandKey(name string, args ...string) string {
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		b.WriteByte(0)
		b.WriteString(a)
	}
	return b.String()
}

// AddCommand registers the canned result Output should return when invoked
// with name and args.
func (f *Fake) AddCommand(name string, args []string, out []byte, err error) {
	if f.Commands == nil {
		f.Commands = make(map[string]FakeResult)
	}
	f.Commands[CommandKey(name, args...)] = FakeResult{Out: out, Err: err}
}

// AddPath registers name as resolvable by LookPath, returning path.
func (f *Fake) AddPath(name, path string) {
	if f.Found == nil {
		f.Found = make(map[string]string)
	}
	f.Found[name] = path
}

// recordCall appends a working-directory-less invocation to Calls.
func (f *Fake) recordCall(name string, args ...string) {
	f.recordCallDir("", name, args...)
}

// recordCallDir appends an invocation (with working directory dir) to Calls
// under the Fake mutex.
func (f *Fake) recordCallDir(dir, name string, args ...string) {
	f.mu.Lock()
	f.Calls = append(f.Calls, FakeCall{Dir: dir, Name: name, Args: append([]string(nil), args...)})
	f.mu.Unlock()
}

// CallsSnapshot returns a copy of recorded invocations. Safe for concurrent
// readers while Output or Run are invoked from other goroutines.
func (f *Fake) CallsSnapshot() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakeCall, len(f.Calls))
	copy(out, f.Calls)
	return out
}

// Output records the call and returns the canned result registered via
// AddCommand. If no matching command was registered, it returns an error.
func (f *Fake) Output(name string, args ...string) ([]byte, error) {
	f.recordCall(name, args...)

	res, ok := f.Commands[CommandKey(name, args...)]
	if !ok {
		return nil, fmt.Errorf("exec.Fake: no canned output registered for %q", CommandKey(name, args...))
	}
	return res.Out, res.Err
}

// Run records the call and returns the canned error registered in RunErrors
// for that invocation. If no matching entry was registered, it succeeds.
func (f *Fake) Run(name string, args ...string) error {
	f.recordCall(name, args...)

	if f.RunErrors == nil {
		return nil
	}
	return f.RunErrors[CommandKey(name, args...)]
}

// RunDir records the call (with working directory dir) and returns the canned
// error registered in RunErrors, keyed by name+args like Run. A test can read
// the recorded FakeCall.Dir to assert a step ran in the expected directory.
func (f *Fake) RunDir(dir, name string, args ...string) error {
	f.recordCallDir(dir, name, args...)

	if f.RunErrors == nil {
		return nil
	}
	return f.RunErrors[CommandKey(name, args...)]
}

// LookPath returns the path registered via AddPath for name, or an error
// mirroring os/exec.LookPath's "not found" behavior.
func (f *Fake) LookPath(name string) (string, error) {
	if path, ok := f.Found[name]; ok {
		return path, nil
	}
	return "", &osexec.Error{Name: name, Err: osexec.ErrNotFound}
}

var _ Runner = (*Fake)(nil)
