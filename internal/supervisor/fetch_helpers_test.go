//go:build !windows

package supervisor_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
)

const testToken = "test-token"

type fakePoolWorker struct {
	mu        sync.Mutex
	html      string
	err       error
	closeErr  error
	request   browser.PageRequest
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakePoolWorker(html string) *fakePoolWorker {
	return &fakePoolWorker{
		html:   html,
		closed: make(chan struct{}),
	}
}

func (worker *fakePoolWorker) OpenPage(_ context.Context, request browser.PageRequest) (browser.LoadedPage, error) {
	worker.mu.Lock()
	worker.request = request
	err := worker.err
	html := worker.html
	worker.mu.Unlock()

	if err != nil {
		return browser.LoadedPage{}, err
	}
	return browser.LoadedPage{URL: request.URL, HTML: html}, nil
}

func (worker *fakePoolWorker) CaptureScreenshot(context.Context, browser.ScreenshotRequest) ([]byte, error) {
	return nil, errors.New("screenshot not implemented in fake worker")
}

func (worker *fakePoolWorker) Close() error {
	worker.closeOnce.Do(func() { close(worker.closed) })
	return worker.closeErr
}

func (worker *fakePoolWorker) pageRequest() browser.PageRequest {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	return worker.request
}

func (worker *fakePoolWorker) waitClosed(t *testing.T) {
	t.Helper()

	select {
	case <-worker.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("fake worker was not closed")
	}
}

func newFakePool(worker browser.Worker) *browser.Pool {
	return browser.NewPool(1, browser.WithWorkerFactory(func(context.Context) (browser.Worker, error) {
		return worker, nil
	}))
}

func launchDaemonWithPool(runtimeDir string, version string, pool *browser.Pool) (*inProcessDaemon, error) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	daemon := &inProcessDaemon{
		cancel:  cancel,
		done:    done,
		stopped: make(chan struct{}),
	}
	go func() {
		done <- supervisor.Run(ctx, supervisor.RunOptions{
			RuntimeDir: runtimeDir,
			Version:    version,
			Pool:       pool,
		})
	}()

	if err := waitForRuntimeFile(runtimeDir, testStartTimeout); err != nil {
		_ = daemon.stop()
		return nil, err
	}
	return daemon, nil
}

func startDaemonWithPool(t *testing.T, runtimeDir string, pool *browser.Pool) *inProcessDaemon {
	t.Helper()

	daemon, err := launchDaemonWithPool(runtimeDir, testVersion, pool)
	if err != nil {
		t.Fatalf("start daemon with pool at %q: %v", runtimeDir, err)
	}
	t.Cleanup(func() {
		if err := daemon.stop(); err != nil {
			t.Errorf("stop daemon with pool at %q: %v", runtimeDir, err)
		}
	})
	return daemon
}
