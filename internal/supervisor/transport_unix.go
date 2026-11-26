//go:build !windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

type runtimeLock struct {
	file *os.File
}

func acquireRuntimeLock(runtimeDir string) (*runtimeLock, error) {
	path := runtimeDir + string(os.PathSeparator) + lockFileName
	//nolint:gosec // Lock path derives from the configured runtime directory.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open daemon lock: %w", err)
	}
	//nolint:gosec // File descriptors fit int on supported platforms.
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrDaemonAlreadyRunning
		}
		return nil, fmt.Errorf("lock daemon runtime: %w", err)
	}
	return &runtimeLock{file: file}, nil
}

func (lock *runtimeLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	//nolint:gosec // File descriptors fit int on supported platforms.
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	err := lock.file.Close()
	lock.file = nil
	return err
}

func listen(ctx context.Context, runtimeDir string) (net.Listener, string, string, error) {
	socketPath := SocketPath(runtimeDir)
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", "", fmt.Errorf("remove stale socket: %w", err)
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "unix", socketPath)
	if err != nil {
		return nil, "", "", fmt.Errorf("listen daemon socket: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		return nil, "", "", fmt.Errorf("secure daemon socket: %w", err)
	}
	return listener, "unix", socketPath, nil
}
