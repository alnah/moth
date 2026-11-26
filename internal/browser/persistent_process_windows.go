//go:build windows

package browser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// DefaultTerminateProcess force-kills a persistent browser process tree.
func DefaultTerminateProcess(pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("invalid browser pid")
	}
	err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	if err == nil || errors.Is(err, exec.ErrNotFound) {
		return err
	}
	return fmt.Errorf("terminate browser pid %d: %w", pid, err)
}
