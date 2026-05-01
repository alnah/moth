package browser

import (
	"context"
	"fmt"
	"testing"
)

type fakeWorker struct {
	openPage          func(context.Context, PageRequest) (LoadedPage, error)
	captureScreenshot func(context.Context, ScreenshotRequest) ([]byte, error)
	close             func() error
}

func (worker *fakeWorker) OpenPage(ctx context.Context, request PageRequest) (LoadedPage, error) {
	if worker.openPage == nil {
		return LoadedPage{URL: request.URL, HTML: "<html><head><title>fake</title></head><body>fake</body></html>"}, nil
	}
	return worker.openPage(ctx, request)
}

func (worker *fakeWorker) CaptureScreenshot(
	ctx context.Context,
	request ScreenshotRequest,
) ([]byte, error) {
	if worker.captureScreenshot == nil {
		return []byte("fake png"), nil
	}
	return worker.captureScreenshot(ctx, request)
}

func (worker *fakeWorker) Close() error {
	if worker.close == nil {
		return nil
	}
	return worker.close()
}

func newQueuedWorkerFactory(t *testing.T, workers ...*fakeWorker) WorkerFactory {
	t.Helper()
	created := 0
	return func(context.Context) (Worker, error) {
		if created >= len(workers) {
			return nil, fmt.Errorf("unexpected worker creation %d", created+1)
		}
		worker := workers[created]
		created++
		return worker, nil
	}
}
