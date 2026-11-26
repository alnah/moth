//go:build !windows

package supervisor_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
)

// TestDaemonShutdownCancelsActiveRequest specifies that an explicit daemon
// shutdown cancels in-flight request contexts instead of waiting for the CLI
// timeout or the HTTP shutdown grace period.
func TestDaemonShutdownCancelsActiveRequest(t *testing.T) {
	worker := newBlockingOpenWorker()
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonWithPool(t, runtimeDir, pool)

	fetchClient := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() {
		worker.release()
		_ = fetchClient.Close()
	})

	fetchDone := make(chan error, 1)
	go func() {
		_, err := fetchClient.FetchRenderedPage(context.Background(), browser.PageRequest{
			URL: "https://example.test/slow",
		})
		fetchDone <- err
	}()

	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("fetch request did not start")
	}

	shutdownClient := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() { _ = shutdownClient.Close() })

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := shutdownClient.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown(during active request) error = %v, want nil", err)
	}

	select {
	case <-fetchDone:
	case <-time.After(2 * time.Second):
		t.Fatal("active fetch did not return after shutdown")
	}
}

// TestDaemonRuntimeRemovedAfterResourcesClose specifies that daemon.json is
// removed only after the server has closed its browser resources. A client
// must not observe a stopped daemon while cleanup is still in progress.
func TestDaemonRuntimeRemovedAfterResourcesClose(t *testing.T) {
	worker := newBlockingCloseWorker()
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonWithPool(t, runtimeDir, pool)

	fetchClient := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() {
		worker.release()
		_ = fetchClient.Close()
	})
	if _, err := fetchClient.FetchRenderedPage(context.Background(), browser.PageRequest{
		URL: "https://example.test/page",
	}); err != nil {
		t.Fatalf("FetchRenderedPage() error = %v, want nil", err)
	}

	shutdownClient := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() { _ = shutdownClient.Close() })

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		shutdownDone <- shutdownClient.Shutdown(ctx)
	}()

	select {
	case <-worker.closeStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not start closing the browser pool")
	}

	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); err != nil {
		t.Fatalf("runtime removed before resources closed: %v", err)
	}

	worker.release()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown() did not finish after resources closed")
	}
}

type blockingOpenWorker struct {
	started     chan struct{}
	releaseCh   chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingOpenWorker() *blockingOpenWorker {
	return &blockingOpenWorker{
		started:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
}

func (worker *blockingOpenWorker) OpenPage(
	ctx context.Context,
	request browser.PageRequest,
) (browser.LoadedPage, error) {
	worker.startedOnce.Do(func() { close(worker.started) })
	select {
	case <-ctx.Done():
		return browser.LoadedPage{}, ctx.Err()
	case <-worker.releaseCh:
		return browser.LoadedPage{URL: request.URL, HTML: "<html>released</html>"}, nil
	}
}

func (worker *blockingOpenWorker) CaptureScreenshot(
	context.Context,
	browser.ScreenshotRequest,
) ([]byte, error) {
	return nil, errors.New("screenshot not implemented")
}

func (worker *blockingOpenWorker) Close() error { return nil }

func (worker *blockingOpenWorker) release() {
	worker.releaseOnce.Do(func() { close(worker.releaseCh) })
}

type blockingCloseWorker struct {
	closeStarted chan struct{}
	releaseCh    chan struct{}
	closeOnce    sync.Once
	releaseOnce  sync.Once
}

func newBlockingCloseWorker() *blockingCloseWorker {
	return &blockingCloseWorker{
		closeStarted: make(chan struct{}),
		releaseCh:    make(chan struct{}),
	}
}

func (worker *blockingCloseWorker) OpenPage(
	_ context.Context,
	request browser.PageRequest,
) (browser.LoadedPage, error) {
	return browser.LoadedPage{URL: request.URL, HTML: "<html>ok</html>"}, nil
}

func (worker *blockingCloseWorker) CaptureScreenshot(
	context.Context,
	browser.ScreenshotRequest,
) ([]byte, error) {
	return nil, errors.New("screenshot not implemented")
}

func (worker *blockingCloseWorker) Close() error {
	worker.closeOnce.Do(func() { close(worker.closeStarted) })
	<-worker.releaseCh
	return nil
}

func (worker *blockingCloseWorker) release() {
	worker.releaseOnce.Do(func() { close(worker.releaseCh) })
}
