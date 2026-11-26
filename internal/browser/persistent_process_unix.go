//go:build !windows

package browser

import (
	"errors"
	"os"
	"syscall"
)

// DefaultTerminateProcess force-kills a persistent browser process group.
func DefaultTerminateProcess(pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("invalid browser pid")
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
