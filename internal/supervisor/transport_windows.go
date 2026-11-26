//go:build windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const allLockBytes = ^uint32(0)

type runtimeLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireRuntimeLock(runtimeDir string) (*runtimeLock, error) {
	path := filepath.Join(runtimeDir, lockFileName)
	//nolint:gosec // Lock path derives from the configured runtime directory.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon lock: %w", err)
	}

	lock := &runtimeLock{file: file}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		flags,
		0,
		allLockBytes,
		allLockBytes,
		&lock.overlapped,
	)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrDaemonAlreadyRunning
		}
		return nil, fmt.Errorf("lock daemon runtime: %w", err)
	}
	return lock, nil
}

func (lock *runtimeLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := windows.UnlockFileEx(
		windows.Handle(lock.file.Fd()),
		0,
		allLockBytes,
		allLockBytes,
		&lock.overlapped,
	)
	closeErr := lock.file.Close()
	lock.file = nil
	return errors.Join(unlockErr, closeErr)
}

func listen(ctx context.Context, runtimeDir string) (net.Listener, string, string, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", "", fmt.Errorf("listen daemon socket: %w", err)
	}
	return listener, "tcp", listener.Addr().String(), nil
}
