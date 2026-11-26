//go:build windows

package supervisor

import (
	"math"

	"golang.org/x/sys/windows"
)

const windowsStillActive uint32 = 259

func processAlive(pid int) bool {
	exitCode, ok := queryProcessExitCode(pid)
	return ok && exitCode == windowsStillActive
}

func queryProcessExitCode(pid int) (uint32, bool) {
	if pid <= 0 || uint64(pid) > math.MaxUint32 {
		return 0, false
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(pid),
	)
	if err != nil {
		return 0, false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return 0, false
	}
	return exitCode, true
}
