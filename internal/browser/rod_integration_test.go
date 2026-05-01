//go:build browser

package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRodPoolFetchesRenderedPageAndWritesScreenshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<html>
<head><title>Rendered page</title></head>
<body>
  <h1 id="title">Rendered page</h1>
  <script>document.body.append(" JavaScript text")</script>
</body>
</html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	pool := NewPool(1)
	defer func() { _ = pool.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	item, err := pool.FetchPage(ctx, PageRequest{URL: server.URL + "/"})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("FetchPage(real Rod) error = %v, want nil", err)
	}
	if item.Title != "Rendered page" {
		t.Fatalf("FetchPage(real Rod) title = %q, want Rendered page", item.Title)
	}
	if !strings.Contains(item.Text, "JavaScript text") {
		t.Fatalf("FetchPage(real Rod) text = %q, want rendered JavaScript text", item.Text)
	}

	screenshotPath := filepath.Join(t.TempDir(), "page.png")
	if err := pool.Screenshot(ctx, ScreenshotRequest{URL: server.URL + "/", Path: screenshotPath, FullPage: true}); err != nil {
		t.Fatalf("Screenshot(real Rod) error = %v, want nil", err)
	}
	info, err := os.Stat(screenshotPath)
	if err != nil {
		t.Fatalf("stat screenshot: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("screenshot size = 0, want bytes")
	}
}
