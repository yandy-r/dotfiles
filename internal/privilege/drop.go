//go:build unix

package privilege

import (
	"errors"
	"os"
	osexec "os/exec"
	"syscall"
)

// Dropper runs commands as the real user when the current process is root.
type Dropper interface {
	Run(argv []string, home string) error
}

type unixDropper struct{}

// NewDropper returns a Dropper that drops root credentials on unix when euid
// is zero. When not root, commands run directly without credential changes.
func NewDropper() Dropper {
	return unixDropper{}
}

func (unixDropper) Run(argv []string, home string) error {
	if len(argv) == 0 {
		return errors.New("privilege: empty argv")
	}

	if os.Geteuid() != 0 {
		return runDirect(argv)
	}

	u, err := RealUser()
	if err != nil {
		return err
	}
	if u.UID == 0 {
		return ErrNoSudoUser
	}

	cmd := osexec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:         u.UID,
			Gid:         u.GID,
			NoSetGroups: true,
		},
	}
	cmd.Env = dropEnv(u.Name, home)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runDirect(argv []string) error {
	cmd := osexec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func dropEnv(user, home string) []string {
	return []string{
		"HOME=" + home,
		"USER=" + user,
		"LOGNAME=" + user,
		"PATH=" + os.Getenv("PATH"),
	}
}

var _ Dropper = unixDropper{}

// DropEnv builds the explicit environment for a dropped child process.
func DropEnv(user, home string) []string {
	return dropEnv(user, home)
}
