//go:build !windows

package supervisor_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
)

func TestDaemonStopsWhenIdle(t *testing.T) {
	runtimeDir := newRuntimeDir(t)
	startDaemonWithOptions(t, runtimeDir, testVersion, supervisor.RunOptions{
		IdleTimeout:  100 * time.Millisecond,
		ReapInterval: 10 * time.Millisecond,
	})

	waitForRuntimeMissing(t, runtimeDir, 2*time.Second)
}

func TestDaemonDoesNotStopDuringActiveRequest(t *testing.T) {
	worker := newBlockingOpenWorker()
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	startDaemonWithOptions(t, runtimeDir, testVersion, supervisor.RunOptions{
		IdleTimeout:  250 * time.Millisecond,
		ReapInterval: 10 * time.Millisecond,
		Pool:         pool,
	})

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() {
		worker.release()
		_ = client.Close()
	})

	fetchDone := make(chan error, 1)
	go func() {
		_, err := client.FetchRenderedPage(context.Background(), browser.PageRequest{
			URL: "https://example.test/slow",
		})
		fetchDone <- err
	}()

	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch request did not start")
	}

	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); err != nil {
		t.Fatalf("runtime disappeared during active request: %v", err)
	}

	worker.release()
	select {
	case err := <-fetchDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("FetchRenderedPage() error = %v, want nil or canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("active fetch did not return")
	}

	waitForRuntimeMissing(t, runtimeDir, 2*time.Second)
}

func startDaemonWithOptions(
	t *testing.T,
	runtimeDir string,
	version string,
	options supervisor.RunOptions,
) {
	t.Helper()

	daemon, err := launchDaemonWithOptions(runtimeDir, version, options)
	if err != nil {
		t.Fatalf("start daemon with options at %q: %v", runtimeDir, err)
	}
	t.Cleanup(func() {
		if err := daemon.stop(); err != nil {
			t.Errorf("stop daemon with options at %q: %v", runtimeDir, err)
		}
	})
}

func launchDaemonWithOptions(
	runtimeDir string,
	version string,
	options supervisor.RunOptions,
) (*inProcessDaemon, error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	daemon := &inProcessDaemon{
		cancel:  cancel,
		done:    done,
		stopped: make(chan struct{}),
	}

	options.RuntimeDir = runtimeDir
	options.Version = version
	go func() {
		done <- supervisor.Run(ctx, options)
	}()

	if err := waitForRuntimeFile(runtimeDir, testStartTimeout); err != nil {
		_ = daemon.stop()
		return nil, err
	}
	return daemon, nil
}

func waitForRuntimeMissing(t *testing.T, runtimeDir string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := os.Stat(supervisor.RuntimePath(runtimeDir))
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, err := os.Stat(supervisor.RuntimePath(runtimeDir))
	t.Fatalf("runtime file in %q still exists after %s: %v", runtimeDir, timeout, err)
}
