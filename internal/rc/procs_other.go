//go:build !unix

package rc

import (
	"errors"
	"syscall"
)

func signalProcess(int, syscall.Signal) error {
	return errors.New("signalling processes is not supported on this platform")
}

func detachedAttr() *syscall.SysProcAttr { return nil }
