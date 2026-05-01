package browser

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/content"
)

func TestReaderModeExtractsArticleAndReportsClearFallback(t *testing.T) {
	article := LoadedPage{URL: "https://example.test/story", HTML: `<!doctype html>
<html>
<head><title>Site chrome title</title></head>
<body>
  <nav>Home Products Pricing</nav>
  <article>
    <h1>Readable story title</h1>
    <p>First useful paragraph with enough content to look like an article.</p>
    <p>Second useful paragraph that should be preserved for the caller.</p>
  </article>
  <footer>Footer links</footer>
</body>
</html>`}

	got, err := ExtractReaderContent(article)
	if err != nil {
		t.Fatalf("ExtractReaderContent(article) error = %v, want nil", err)
	}
	if got.URL != article.URL {
		t.Fatalf("reader URL = %q, want %q", got.URL, article.URL)
	}
	if got.Title != "Readable story title" {
		t.Fatalf("reader title = %q, want article heading", got.Title)
	}
	if !strings.Contains(got.Text, "First useful paragraph") || !strings.Contains(got.Text, "Second useful paragraph") {
		t.Fatalf("reader text = %q, want article paragraphs", got.Text)
	}
	if strings.Contains(got.Text, "Products Pricing") || strings.Contains(got.Text, "Footer links") {
		t.Fatalf("reader text = %q, want navigation and footer removed", got.Text)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("reader warnings = %#v, want none", got.Warnings)
	}

	fallback, err := ExtractReaderContent(LoadedPage{URL: "https://example.test/app", HTML: `
<html><head><title>Dashboard</title></head><body><button>Refresh</button></body></html>`})
	if err != nil {
		t.Fatalf("ExtractReaderContent(fallback) error = %v, want nil", err)
	}
	if fallback.Title != "Dashboard" || !strings.Contains(fallback.Text, "Refresh") {
		t.Fatalf("fallback reader = %#v, want visible page fallback", fallback)
	}
	if !hasWarning(fallback.Warnings, content.Warning("reader_content_not_found")) {
		t.Fatalf("fallback warnings = %#v, want reader_content_not_found", fallback.Warnings)
	}
}

func TestAccessibilityTreeReturnsStableNodesAndHonorsCancellation(t *testing.T) {
	ctx := context.Background()
	worker := newSurfaceWorker()
	worker.accessibility = AccessibilityTree{
		Nodes: []AccessibilityNode{
			{Role: "RootWebArea", Name: "Checkout"},
			{Role: "heading", Name: "Checkout"},
			{Role: "button", Name: "Pay now"},
		},
	}
	pool := newSurfacePool(worker)
	defer func() { _ = pool.Close() }()

	page := openPersistentPage(ctx, t, pool, OpenPageRequest{
		ProfileName: "research",
		SessionName: "checkout",
		URL:         "https://shop.example.test/checkout",
	})
	got, err := pool.AccessibilityTree(ctx, AccessibilityRequest{
		ProfileName: "research",
		SessionName: "checkout",
		PageID:      page.ID,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("AccessibilityTree() error = %v, want nil", err)
	}
	if len(got.Nodes) != 3 {
		t.Fatalf("accessibility nodes len = %d, want 3", len(got.Nodes))
	}
	if got.Nodes[2].Role != "button" || got.Nodes[2].Name != "Pay now" {
		t.Fatalf("button node = %#v, want stable role/name", got.Nodes[2])
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal accessibility tree: %v", err)
	}
	if strings.Contains(string(encoded), "backend") || strings.Contains(string(encoded), "nodeId") {
		t.Fatalf("accessibility JSON = %s, want normalized stable fields only", encoded)
	}

	worker.blockAccessibility = make(chan struct{})
	cancelCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = pool.AccessibilityTree(cancelCtx, AccessibilityRequest{
		ProfileName: "research",
		SessionName: "checkout",
		PageID:      page.ID,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AccessibilityTree(cancelled) error = %v, want context deadline", err)
	}
}
