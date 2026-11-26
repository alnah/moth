//go:build windows

package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeLockAcquiresStaleLockFile(t *testing.T) {
	runtimeDir := t.TempDir()
	path := filepath.Join(runtimeDir, lockFileName)
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}

	lock, err := acquireRuntimeLock(runtimeDir)
	if err != nil {
		t.Fatalf("acquireRuntimeLock(stale file) error = %v, want nil", err)
	}
	if closeErr := lock.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v, want nil", closeErr)
	}
}

func TestRuntimeLockRejectsSecondInstance(t *testing.T) {
	runtimeDir := t.TempDir()
	lock, err := acquireRuntimeLock(runtimeDir)
	if err != nil {
		t.Fatalf("first acquireRuntimeLock() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		if closeErr := lock.Close(); closeErr != nil {
			t.Errorf("Close() error = %v, want nil", closeErr)
		}
	})

	second, err := acquireRuntimeLock(runtimeDir)
	if err == nil {
		_ = second.Close()
		t.Fatal("second acquireRuntimeLock() error = nil, want ErrDaemonAlreadyRunning")
	}
	if !errors.Is(err, ErrDaemonAlreadyRunning) {
		t.Fatalf("second acquireRuntimeLock() error = %v, want ErrDaemonAlreadyRunning", err)
	}
}

func TestRuntimeLockCanBeReacquiredAfterClose(t *testing.T) {
	runtimeDir := t.TempDir()
	lock, err := acquireRuntimeLock(runtimeDir)
	if err != nil {
		t.Fatalf("first acquireRuntimeLock() error = %v, want nil", err)
	}
	if closeErr := lock.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v, want nil", closeErr)
	}

	reacquired, err := acquireRuntimeLock(runtimeDir)
	if err != nil {
		t.Fatalf("reacquireRuntimeLock() error = %v, want nil", err)
	}
	if closeErr := reacquired.Close(); closeErr != nil {
		t.Fatalf("reacquired Close() error = %v, want nil", closeErr)
	}
}

func TestRuntimeLockKeepsFileAfterClose(t *testing.T) {
	runtimeDir := t.TempDir()
	lock, err := acquireRuntimeLock(runtimeDir)
	if err != nil {
		t.Fatalf("acquireRuntimeLock() error = %v, want nil", err)
	}
	path := filepath.Join(runtimeDir, lockFileName)
	if closeErr := lock.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v, want nil", closeErr)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("daemon.lock after close error = %v, want present", err)
	}
}
