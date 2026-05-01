package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadCaptureWritesCallerPathAndReportsCancellation(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	worker.downloadPayload = []byte("downloaded report")
	worker.downloadContentType = "text/plain; charset=utf-8"
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	page := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "downloads",
		URL:         "https://example.test/report",
	})
	target := filepath.Join(t.TempDir(), "nested", "report.txt")
	result, err := pool.Download(ctx, DownloadRequest{
		ProfileName: "research",
		SessionName: "downloads",
		PageID:      page.ID,
		Selector:    "a.report",
		Path:        target,
	})
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	got, err := os.ReadFile(target) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != "downloaded report" {
		t.Fatalf("downloaded bytes = %q, want fixture", got)
	}
	if result.Path != target || result.Bytes != int64(len(got)) || result.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("download result = %#v, want path, bytes, and content type", result)
	}

	worker.blockDownload = make(chan struct{})
	cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = pool.Download(cancelCtx, DownloadRequest{
		ProfileName: "research",
		SessionName: "downloads",
		PageID:      page.ID,
		Selector:    "a.report",
		Path:        filepath.Join(t.TempDir(), "cancelled.txt"),
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Download(cancelled) error = %v, want context deadline", err)
	}
}

func TestPDFCaptureWritesBytesAndWrapsWriteErrors(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	worker.pdfPayload = []byte("%PDF-1.7 fake pdf")
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	target := filepath.Join(t.TempDir(), "nested", "page.pdf")
	if err := pool.PDF(ctx, PDFRequest{URL: "https://example.test/print", Path: target}); err != nil {
		t.Fatalf("PDF() error = %v, want nil", err)
	}
	got, err := os.ReadFile(target) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read PDF file: %v", err)
	}
	if string(got) != string(worker.pdfPayload) {
		t.Fatalf("PDF bytes = %q, want fixture", got)
	}

	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	writeErr := os.WriteFile(parentFile, []byte("file"), 0o600)
	if writeErr != nil {
		t.Fatalf("write parent fixture: %v", writeErr)
	}
	err = pool.PDF(ctx, PDFRequest{URL: "https://example.test/print", Path: filepath.Join(parentFile, "page.pdf")})
	if err == nil {
		t.Fatal("PDF(write failure) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "write pdf") && !strings.Contains(err.Error(), "create pdf directory") {
		t.Fatalf("PDF(write failure) error = %v, want path context", err)
	}
}
