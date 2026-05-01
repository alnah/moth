package browser

import (
	"context"
	"strings"
	"testing"
)

func TestSessionOperationsReportMissingSession(t *testing.T) {
	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{})))
	defer func() { _ = pool.Close() }()

	pages, err := pool.ListPages(context.Background(), SessionRequest{ProfileName: "missing", SessionName: "work"})
	if err != nil {
		t.Fatalf("ListPages(missing session) error = %v, want nil", err)
	}
	if len(pages) != 0 {
		t.Fatalf("ListPages(missing session) = %#v, want empty", pages)
	}

	_, err = pool.SwitchPage(context.Background(), PageSelection{ProfileName: "missing", SessionName: "work"})
	if err == nil || !strings.Contains(err.Error(), "browser session") {
		t.Fatalf("SwitchPage(missing session) error = %v, want missing session context", err)
	}
}

func TestUnsupportedWorkerSurfaceReportsClearErrors(t *testing.T) {
	pool := NewPool(1, WithWorkerFactory(newQueuedWorkerFactory(t, &fakeWorker{})))
	defer func() { _ = pool.Close() }()

	_, err := pool.OpenPage(context.Background(), OpenPageRequest{
		ProfileName: "research",
		SessionName: "work",
		URL:         "https://example.test",
	})
	if err == nil || !strings.Contains(err.Error(), "persistent pages") {
		t.Fatalf("OpenPage(unsupported worker) error = %v, want persistent pages context", err)
	}

	_, err = pool.ResponseMetadata(context.Background(), ResponseMetadataRequest{URL: "https://example.test"})
	if err == nil || !strings.Contains(err.Error(), "response metadata") {
		t.Fatalf("ResponseMetadata(unsupported worker) error = %v, want metadata context", err)
	}
}

func TestDownloadRejectsUnsupportedWorkerBytes(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	worker.downloadPayload = nil
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	page := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "downloads",
		URL:         "https://example.test/report",
	})
	worker.downloadPayload = nil
	worker.downloadValue = 42

	_, err := pool.Download(ctx, DownloadRequest{
		ProfileName: "research",
		SessionName: "downloads",
		PageID:      page.ID,
		Selector:    "a.report",
		Path:        t.TempDir() + "/report.txt",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("Download(unsupported bytes) error = %v, want unsupported type", err)
	}
}

func TestClosingLastPageRemovesPersistentSession(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	page := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "single",
		URL:         "https://example.test/one",
	})
	if err := pool.ClosePage(ctx, PageSelection{
		ProfileName: "research",
		SessionName: "single",
		PageID:      page.ID,
	}); err != nil {
		t.Fatalf("ClosePage(last page) error = %v, want nil", err)
	}

	pages, err := pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "single"})
	if err != nil {
		t.Fatalf("ListPages(after last close) error = %v, want nil", err)
	}
	if len(pages) != 0 {
		t.Fatalf("ListPages(after last close) = %#v, want empty", pages)
	}
}
