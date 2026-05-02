package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/httpdownload"
	"github.com/alnah/moth/internal/limits"
)

func TestScreenshotRejectsCaptureOverMaxBytesBeforeWriting(t *testing.T) {
	ctx := context.Background()
	payload := []byte("12345")
	worker := newSurfaceWorker()
	worker.captureScreenshot = func(context.Context, ScreenshotRequest) ([]byte, error) {
		return payload, nil
	}
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	target := filepath.Join(t.TempDir(), "nested", "page.png")
	request := ScreenshotRequest{URL: "https://example.test/capture", Path: target}
	setCaptureMaxBytes(t, &request, 4)

	err := pool.Screenshot(ctx, request)
	assertCaptureTooLargeError(t, err, 4)
	assertFullCaptureWasNotWritten(t, target, int64(len(payload)))
}

func TestPDFRejectsCaptureOverMaxBytesBeforeWriting(t *testing.T) {
	ctx := context.Background()
	payload := []byte("%PDF-12345")
	worker := newSurfaceWorker()
	worker.pdfPayload = payload
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	target := filepath.Join(t.TempDir(), "nested", "page.pdf")
	request := PDFRequest{URL: "https://example.test/print", Path: target}
	setCaptureMaxBytes(t, &request, 8)

	err := pool.PDF(ctx, request)
	assertCaptureTooLargeError(t, err, 8)
	assertFullCaptureWasNotWritten(t, target, int64(len(payload)))
}

func TestBrowserCapturesWritePayloadsWithinMaxBytes(t *testing.T) {
	ctx := context.Background()

	t.Run("screenshot", func(t *testing.T) {
		payload := []byte("small png")
		worker := newSurfaceWorker()
		worker.captureScreenshot = func(context.Context, ScreenshotRequest) ([]byte, error) {
			return payload, nil
		}
		pool := newSurfacePool(worker)
		defer func() { _ = pool.Close() }()

		target := filepath.Join(t.TempDir(), "page.png")
		request := ScreenshotRequest{URL: "https://example.test/capture", Path: target}
		setCaptureMaxBytes(t, &request, int64(len(payload)))

		if err := pool.Screenshot(ctx, request); err != nil {
			t.Fatalf("Screenshot(payload at max bytes) error = %v, want nil", err)
		}
		assertFileBytes(t, target, payload)
	})

	t.Run("pdf", func(t *testing.T) {
		payload := []byte("%PDF-small")
		worker := newSurfaceWorker()
		worker.pdfPayload = payload
		pool := newSurfacePool(worker)
		defer func() { _ = pool.Close() }()

		target := filepath.Join(t.TempDir(), "page.pdf")
		request := PDFRequest{URL: "https://example.test/print", Path: target}
		setCaptureMaxBytes(t, &request, int64(len(payload)))

		if err := pool.PDF(ctx, request); err != nil {
			t.Fatalf("PDF(payload at max bytes) error = %v, want nil", err)
		}
		assertFileBytes(t, target, payload)
	})
}

func TestScreenshotRejectsCaptureOverDefaultMaxBytesBeforeWriting(t *testing.T) {
	ctx := context.Background()
	payload := oversizedDefaultCapturePayload(t)
	worker := newSurfaceWorker()
	worker.captureScreenshot = func(context.Context, ScreenshotRequest) ([]byte, error) {
		return payload, nil
	}
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	target := filepath.Join(t.TempDir(), "default.png")
	err := pool.Screenshot(ctx, ScreenshotRequest{URL: "https://example.test/capture", Path: target})
	assertCaptureTooLargeError(t, err, limits.DefaultMaxBytes)
	assertFullCaptureWasNotWritten(t, target, int64(len(payload)))
}

func setCaptureMaxBytes(t *testing.T, request any, maxBytes int64) {
	t.Helper()
	value := reflect.ValueOf(request)
	if value.Kind() != reflect.Pointer || value.Elem().Kind() != reflect.Struct {
		t.Fatalf("capture request type = %T, want pointer to struct", request)
	}
	field := value.Elem().FieldByName("MaxBytes")
	if !field.IsValid() {
		t.Fatalf("%T has no MaxBytes field for caller byte limits", request)
	}
	if field.Kind() != reflect.Int64 {
		t.Fatalf("%T MaxBytes kind = %s, want int64", request, field.Kind())
	}
	field.SetInt(maxBytes)
}

func assertCaptureTooLargeError(t *testing.T, err error, maxBytes int64) {
	t.Helper()
	if err == nil {
		t.Fatal("capture error = nil, want bounded-size error")
	}
	if errors.Is(err, httpdownload.ErrFileTooLarge) {
		return
	}
	message := err.Error()
	limit := strconv.FormatInt(maxBytes, 10)
	if strings.Contains(message, httpdownload.ErrFileTooLarge.Error()) && strings.Contains(message, limit) {
		return
	}
	if strings.Contains(message, "over") && strings.Contains(message, limit) {
		return
	}
	t.Fatalf("capture error = %v, want file_too_large or bounded-size error mentioning %s bytes", err, limit)
}

func assertFullCaptureWasNotWritten(t *testing.T, path string, fullSize int64) {
	t.Helper()
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatalf("stat capture file: %v", err)
	}
	if info.Size() == fullSize {
		t.Fatalf("capture wrote full oversized file of %d bytes, want rejection before full write", fullSize)
	}
}

func assertFileBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read capture file: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("capture file bytes = %q, want %q", got, want)
	}
}

func oversizedDefaultCapturePayload(t *testing.T) []byte {
	t.Helper()
	if limits.DefaultMaxBytes > int64(int(limits.DefaultMaxBytes)) {
		t.Fatalf("default max bytes %d exceeds int size", limits.DefaultMaxBytes)
	}
	return make([]byte, int(limits.DefaultMaxBytes)+1)
}
