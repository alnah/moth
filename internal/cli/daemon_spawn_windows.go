//go:build windows

package cli

import "syscall"

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}
