package browser

import (
	"context"
	"testing"
)

func TestClosingPagesKeepsRemainingSessionThenRemovesLastPage(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	first := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "multi-close",
		URL:         "https://example.test/one",
	})
	second := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "multi-close",
		URL:         "https://example.test/two",
	})

	if err := pool.ClosePage(ctx, PageSelection{
		ProfileName: "research",
		SessionName: "multi-close",
		PageID:      second.ID,
	}); err != nil {
		t.Fatalf("ClosePage(active page) error = %v, want nil", err)
	}
	pages, err := pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "multi-close"})
	if err != nil {
		t.Fatalf("ListPages(after closing active page) error = %v, want nil", err)
	}
	if len(pages) != 1 || pages[0].ID != first.ID || !pages[0].Active {
		t.Fatalf("ListPages(after closing active page) = %#v, want first page active", pages)
	}

	err = pool.ClosePage(ctx, PageSelection{
		ProfileName: "research",
		SessionName: "multi-close",
		PageID:      first.ID,
	})
	if err != nil {
		t.Fatalf("ClosePage(last page) error = %v, want nil", err)
	}
	pages, err = pool.ListPages(ctx, SessionRequest{ProfileName: "research", SessionName: "multi-close"})
	if err != nil {
		t.Fatalf("ListPages(after closing last page) error = %v, want nil", err)
	}
	if len(pages) != 0 {
		t.Fatalf("ListPages(after closing last page) = %#v, want empty", pages)
	}
}
