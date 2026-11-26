package cli

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

type testDaemon struct {
	cancel context.CancelFunc
	done   chan error
}

func newShortRuntimeDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "moth-cli-run-")
	if err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func startTestDaemon(t *testing.T, runtimeDir string) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	daemon := &testDaemon{cancel: cancel, done: make(chan error, 1)}
	go func() {
		daemon.done <- supervisor.Run(ctx, supervisor.RunOptions{
			RuntimeDir: runtimeDir,
			Version:    version,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := supervisor.ReadRuntime(runtimeDir); err == nil {
			t.Cleanup(func() { daemon.stop(t) })
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	t.Fatal("daemon runtime did not become ready")
}

func (daemon *testDaemon) stop(t *testing.T) {
	t.Helper()

	daemon.cancel()
	select {
	case err := <-daemon.done:
		if err != nil {
			t.Errorf("stop daemon: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("daemon did not stop")
	}
}
