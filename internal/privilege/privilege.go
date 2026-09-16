// Package privilege provides sudo priming/keep-alive and unix credential drop
// helpers for running commands as the real user after elevation.
package privilege

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/yandy-r/dotfiles/internal/exec"
)

const defaultKeepAliveInterval = 4*time.Minute + 30*time.Second

// keepAliveInterval controls how often sudo credentials are refreshed.
// Tests may temporarily override this value.
var keepAliveInterval = defaultKeepAliveInterval

// ErrNoSudoUser is returned when the process is root but SUDO_USER is missing
// or set to root.
var ErrNoSudoUser = errors.New("privilege: running as root without a non-root SUDO_USER")

// Sudo manages interactive sudo priming and non-interactive keep-alive refresh.
type Sudo struct {
	keepAliveErr atomic.Pointer[error]
}

// NewSudo returns a ready-to-use Sudo helper.
func NewSudo() *Sudo {
	return &Sudo{}
}

// Prime validates sudo credentials interactively (typically prompting once).
func (s *Sudo) Prime(r exec.Runner) error {
	return r.Run("sudo", "-v")
}

// KeepAlive starts a goroutine that periodically runs sudo -n -v until ctx is
// cancelled or the returned cancel function is invoked. The first refresh runs
// immediately; subsequent refreshes use keepAliveInterval (<5 minutes).
func (s *Sudo) KeepAlive(ctx context.Context, r exec.Runner) func() {
	ctx, cancel := context.WithCancel(ctx)
	ticker := time.NewTicker(keepAliveInterval)

	go func() {
		defer ticker.Stop()

		refresh := func() error {
			return r.Run("sudo", "-n", "-v")
		}

		if err := refresh(); err != nil {
			s.storeKeepAliveErr(err)
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := refresh(); err != nil {
					s.storeKeepAliveErr(err)
					return
				}
			}
		}
	}()

	return cancel
}

// KeepAliveErr returns the first keep-alive refresh failure, if any.
func (s *Sudo) KeepAliveErr() error {
	if errPtr := s.keepAliveErr.Load(); errPtr != nil {
		return *errPtr
	}
	return nil
}

func (s *Sudo) storeKeepAliveErr(err error) {
	errCopy := err
	s.keepAliveErr.Store(&errCopy)
}

// Prefix returns argv prefix for escalating a command through sudo.
func (s *Sudo) Prefix() []string {
	return []string{"sudo"}
}

// User holds the real (non-root) account identity used for privilege drops.
type User struct {
	Name string
	UID  uint32
	GID  uint32
}

// RealUser resolves the invoking user from SUDO_USER/SUDO_UID/SUDO_GID when
// running as root, or from the current account otherwise.
func RealUser() (User, error) {
	if os.Geteuid() != 0 {
		return currentUser()
	}

	name := os.Getenv("SUDO_USER")
	if name == "" || name == "root" {
		return User{}, ErrNoSudoUser
	}

	uid, err := parseIDEnv("SUDO_UID")
	if err != nil {
		return User{}, fmt.Errorf("privilege: parse SUDO_UID: %w", err)
	}
	if uid == 0 {
		return User{}, ErrNoSudoUser
	}

	gid, err := parseIDEnv("SUDO_GID")
	if err != nil {
		return User{}, fmt.Errorf("privilege: parse SUDO_GID: %w", err)
	}

	return User{Name: name, UID: uid, GID: gid}, nil
}

func currentUser() (User, error) {
	uid := os.Getuid()
	gid := os.Getgid()
	name := os.Getenv("USER")
	if name == "" {
		name = os.Getenv("LOGNAME")
	}
	if name == "" {
		return User{}, errors.New("privilege: USER and LOGNAME are unset")
	}
	return User{Name: name, UID: uint32(uid), GID: uint32(gid)}, nil
}

func parseIDEnv(key string) (uint32, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, fmt.Errorf("%s is unset", key)
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint32(n), nil
}
