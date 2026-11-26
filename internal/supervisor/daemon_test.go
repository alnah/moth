//go:build !windows

package supervisor_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

func TestRunStopsOnContextCancelAndRemovesRuntime(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	daemon := startDaemonAt(t, runtimeDir, testVersion)

	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); err != nil {
		t.Fatalf("runtime file before stop: %v", err)
	}

	if err := daemon.stop(); err != nil {
		t.Fatalf("stop daemon: %v", err)
	}
	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime file after stop err = %v, want removed", err)
	}
	if _, err := os.Stat(supervisor.SocketPath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after stop err = %v, want removed", err)
	}
}

func TestRunRejectsSecondInstance(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	_ = startDaemonAt(t, runtimeDir, testVersion)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := supervisor.Run(ctx, supervisor.RunOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	if !errors.Is(err, supervisor.ErrDaemonAlreadyRunning) {
		t.Fatalf("Run(second instance) error = %v, want ErrDaemonAlreadyRunning", err)
	}
}

func TestRunReplacesStaleSocket(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	writeStaleSocket(t, supervisor.SocketPath(runtimeDir))

	_ = startDaemonAt(t, runtimeDir, testVersion)

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() after stale socket = %v, want nil", err)
	}
	if health.Status != "ok" {
		t.Fatalf("Health().Status = %q, want ok", health.Status)
	}
}

func TestRunFailsWhenRuntimeDirIsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write runtime dir file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := supervisor.Run(ctx, supervisor.RunOptions{
		RuntimeDir: path,
		Version:    testVersion,
	})
	if err == nil {
		t.Fatal("Run(runtime dir is file) error = nil, want error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run(runtime dir is file) hung until context deadline: %v", err)
	}
}

func TestRunWithCanceledContextDoesNotLeaveRuntime(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), "run")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := supervisor.Run(ctx, supervisor.RunOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run(canceled context) error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(supervisor.RuntimePath(runtimeDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("runtime file after canceled start err = %v, want absent", statErr)
	}
}
