//go:build windows

package supervisor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
)

type runtimeLock struct {
	file *os.File
	path string
}

func acquireRuntimeLock(runtimeDir string) (*runtimeLock, error) {
	path := filepath.Join(runtimeDir, lockFileName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil, ErrDaemonAlreadyRunning
	}
	if err != nil {
		return nil, fmt.Errorf("open daemon lock: %w", err)
	}
	return &runtimeLock{file: file, path: path}, nil
}

func (lock *runtimeLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	err := lock.file.Close()
	_ = os.Remove(lock.path)
	lock.file = nil
	return err
}

func listen(ctx context.Context, runtimeDir string) (net.Listener, string, string, error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", "", fmt.Errorf("listen daemon socket: %w", err)
	}
	return listener, "tcp", listener.Addr().String(), nil
}
