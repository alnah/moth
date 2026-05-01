package browser

import (
	"context"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenPageSerializesSameRegistrableDomain(t *testing.T) {
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32

	workers := []Worker{
		&domainSurfaceWorker{
			surfaceWorker: newSurfaceWorker(),
			onOpen:        domainCallRecorder(&calls, firstStarted, secondStarted),
			blockOpenURL:  "https://docs.example.test/one",
			releaseOpen:   releaseFirst,
		},
		&domainSurfaceWorker{
			surfaceWorker: newSurfaceWorker(),
			onOpen:        domainCallRecorder(&calls, firstStarted, secondStarted),
		},
	}
	pool := NewPool(2, WithWorkerFactory(newQueuedGenericWorkerFactory(t, workers...)))
	defer func() { _ = pool.Close() }()

	firstDone := make(chan error, 1)
	go func() {
		_, err := pool.OpenPage(context.Background(), OpenPageRequest{
			ProfileName: "one",
			SessionName: "work",
			URL:         "https://docs.example.test/one",
		})
		firstDone <- err
	}()
	waitForClosed(t, firstStarted, "first persistent open to start")

	secondDone := make(chan error, 1)
	go func() {
		_, err := pool.OpenPage(context.Background(), OpenPageRequest{
			ProfileName: "two",
			SessionName: "work",
			URL:         "https://cdn.example.test/two",
		})
		secondDone <- err
	}()
	assertNotStartedBeforeRelease(t, secondStarted, releaseFirst)
	assertDoneWithoutError(t, firstDone, "first OpenPage")
	assertDoneWithoutError(t, secondDone, "second OpenPage")
	waitForClosed(t, secondStarted, "second persistent open after first finished")
}

func TestPDFAndResponseMetadataSerializeSameRegistrableDomain(t *testing.T) {
	t.Run("pdf", func(t *testing.T) {
		firstStarted := make(chan struct{})
		secondStarted := make(chan struct{})
		releaseFirst := make(chan struct{})
		var calls atomic.Int32
		workers := []Worker{
			&domainSurfaceWorker{
				surfaceWorker: newSurfaceWorker(),
				onPDF:         domainCallRecorder(&calls, firstStarted, secondStarted),
				blockPDFURL:   "https://docs.example.test/one",
				releasePDF:    releaseFirst,
			},
			&domainSurfaceWorker{
				surfaceWorker: newSurfaceWorker(),
				onPDF:         domainCallRecorder(&calls, firstStarted, secondStarted),
			},
		}
		pool := NewPool(2, WithWorkerFactory(newQueuedGenericWorkerFactory(t, workers...)))
		defer func() { _ = pool.Close() }()

		firstDone := make(chan error, 1)
		go func() {
			firstDone <- pool.PDF(context.Background(), PDFRequest{
				URL:  "https://docs.example.test/one",
				Path: filepath.Join(t.TempDir(), "one.pdf"),
			})
		}()
		waitForClosed(t, firstStarted, "first PDF to start")

		secondDone := make(chan error, 1)
		go func() {
			secondDone <- pool.PDF(context.Background(), PDFRequest{
				URL:  "https://cdn.example.test/two",
				Path: filepath.Join(t.TempDir(), "two.pdf"),
			})
		}()
		assertNotStartedBeforeRelease(t, secondStarted, releaseFirst)
		assertDoneWithoutError(t, firstDone, "first PDF")
		assertDoneWithoutError(t, secondDone, "second PDF")
	})

	t.Run("response metadata", func(t *testing.T) {
		firstStarted := make(chan struct{})
		secondStarted := make(chan struct{})
		releaseFirst := make(chan struct{})
		var calls atomic.Int32
		workers := []Worker{
			&domainSurfaceWorker{
				surfaceWorker:      newSurfaceWorker(),
				onMetadata:         domainCallRecorder(&calls, firstStarted, secondStarted),
				blockMetadataURL:   "https://docs.example.test/one",
				releaseMetadata:    releaseFirst,
				metadataHTTPStatus: http.StatusOK,
			},
			&domainSurfaceWorker{
				surfaceWorker:      newSurfaceWorker(),
				onMetadata:         domainCallRecorder(&calls, firstStarted, secondStarted),
				metadataHTTPStatus: http.StatusOK,
			},
		}
		pool := NewPool(2, WithWorkerFactory(newQueuedGenericWorkerFactory(t, workers...)))
		defer func() { _ = pool.Close() }()

		firstDone := make(chan error, 1)
		go func() {
			_, err := pool.ResponseMetadata(context.Background(), ResponseMetadataRequest{
				URL: "https://docs.example.test/one",
			})
			firstDone <- err
		}()
		waitForClosed(t, firstStarted, "first metadata request to start")

		secondDone := make(chan error, 1)
		go func() {
			_, err := pool.ResponseMetadata(context.Background(), ResponseMetadataRequest{
				URL: "https://cdn.example.test/two",
			})
			secondDone <- err
		}()
		assertNotStartedBeforeRelease(t, secondStarted, releaseFirst)
		assertDoneWithoutError(t, firstDone, "first ResponseMetadata")
		assertDoneWithoutError(t, secondDone, "second ResponseMetadata")
	})
}

type domainSurfaceWorker struct {
	*surfaceWorker

	onOpen       func(string)
	blockOpenURL string
	releaseOpen  <-chan struct{}

	onPDF       func(string)
	blockPDFURL string
	releasePDF  <-chan struct{}

	onMetadata         func(string)
	blockMetadataURL   string
	releaseMetadata    <-chan struct{}
	metadataHTTPStatus int
}

func (worker *domainSurfaceWorker) OpenPersistentPage(ctx context.Context, request OpenPageRequest) (PageInfo, error) {
	if worker.onOpen != nil {
		worker.onOpen(request.URL)
	}
	if request.URL == worker.blockOpenURL {
		select {
		case <-worker.releaseOpen:
		case <-ctx.Done():
			return PageInfo{}, ctx.Err()
		}
	}
	return worker.surfaceWorker.OpenPersistentPage(ctx, request)
}

func (worker *domainSurfaceWorker) CapturePDF(ctx context.Context, request PDFRequest) ([]byte, error) {
	if worker.onPDF != nil {
		worker.onPDF(request.URL)
	}
	if request.URL == worker.blockPDFURL {
		select {
		case <-worker.releasePDF:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return worker.surfaceWorker.CapturePDF(ctx, request)
}

func (worker *domainSurfaceWorker) ResponseMetadata(
	ctx context.Context,
	request ResponseMetadataRequest,
) (ResponseMetadata, error) {
	if worker.onMetadata != nil {
		worker.onMetadata(request.URL)
	}
	if request.URL == worker.blockMetadataURL {
		select {
		case <-worker.releaseMetadata:
		case <-ctx.Done():
			return ResponseMetadata{}, ctx.Err()
		}
	}
	status := worker.metadataHTTPStatus
	if status == 0 {
		status = http.StatusOK
	}
	return ResponseMetadata{URL: request.URL, Status: status}, nil
}

func domainCallRecorder(calls *atomic.Int32, firstStarted chan struct{}, secondStarted chan struct{}) func(string) {
	return func(string) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			return
		}
		close(secondStarted)
	}
}

func assertNotStartedBeforeRelease(t *testing.T, started chan struct{}, release chan struct{}) {
	t.Helper()
	select {
	case <-started:
		close(release)
		t.Fatal("same-domain operation started while first operation was active")
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
}

func assertDoneWithoutError(t *testing.T, done <-chan error, operation string) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s error = %v, want nil", operation, err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for %s", operation)
	}
}
