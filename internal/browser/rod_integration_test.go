//go:build browser

package browser

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRodPoolFetchesRenderedPageAndWritesScreenshot(t *testing.T) {
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 15*time.Second)

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
	requireNonEmptyFile(t, screenshotPath)
}
