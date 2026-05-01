package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadWritesStringPayloads(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	worker.downloadValue = "download text"
	worker.downloadContentType = "text/plain"
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	page := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "downloads",
		URL:         "https://example.test/report",
	})
	target := filepath.Join(t.TempDir(), "report.txt")
	result, err := pool.Download(ctx, DownloadRequest{
		ProfileName: "research",
		SessionName: "downloads",
		PageID:      page.ID,
		Selector:    "a.report",
		Path:        target,
	})
	if err != nil {
		t.Fatalf("Download(string payload) error = %v, want nil", err)
	}
	bytes, err := os.ReadFile(target) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(bytes) != "download text" {
		t.Fatalf("download bytes = %q, want string payload", bytes)
	}
	if result.Bytes != int64(len("download text")) || result.ContentType != "text/plain" {
		t.Fatalf("download result = %#v, want byte count and content type", result)
	}
}

func TestDownloadBytesAcceptsNilPayload(t *testing.T) {
	bytes, err := downloadBytes(nil)
	if err != nil {
		t.Fatalf("downloadBytes(nil) error = %v, want nil", err)
	}
	if len(bytes) != 0 {
		t.Fatalf("downloadBytes(nil) len = %d, want 0", len(bytes))
	}
}
