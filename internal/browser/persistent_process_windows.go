//go:build windows

package browser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"golang.org/x/sys/windows"
)

// DefaultTerminateProcess force-kills a persistent browser process tree.
func DefaultTerminateProcess(pid int) error {
	if pid <= 0 || pid == os.Getpid() {
		return errors.New("invalid browser pid")
	}
	err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	if err == nil || errors.Is(err, exec.ErrNotFound) || !windowsProcessAlive(pid) {
		return nil
	}
	return fmt.Errorf("terminate browser pid %d: %w", pid, err)
}

func windowsProcessAlive(pid int) bool {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == 259
}
