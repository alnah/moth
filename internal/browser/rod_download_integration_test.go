//go:build browser

package browser

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRodPoolDownloadsLocalFileThroughPersistentPage(t *testing.T) {
	fixture := []byte("moth download fixture\n")
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<title>Download page</title>
<a id="download" href="/fixture.txt" download="fixture.txt">Download fixture</a>`))
		case "/fixture.txt":
			w.Header().Set("Content-Disposition", `attachment; filename="fixture.txt"`)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write(fixture)
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 20*time.Second)

	page, err := pool.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "downloads",
		URL:         server.URL + "/page",
	})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("OpenPage(download page) error = %v, want nil", err)
	}

	downloadDir := t.TempDir()
	downloadPath := filepath.Join(downloadDir, "nested", "fixture.txt")
	result, err := pool.Download(ctx, DownloadRequest{
		ProfileName: "research",
		SessionName: "downloads",
		PageID:      page.ID,
		Selector:    "#download",
		Path:        downloadPath,
	})
	if err != nil {
		t.Fatalf("Download(real Rod page) error = %v, want nil", err)
	}
	if result.Path != downloadPath {
		t.Fatalf("Download(real Rod page) path = %q, want caller path %q", result.Path, downloadPath)
	}
	if rel, err := filepath.Rel(downloadDir, result.Path); err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		t.Fatalf("Download(real Rod page) path = %q, want path under %q", result.Path, downloadDir)
	}
	got, err := os.ReadFile(downloadPath) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if !bytes.Equal(got, fixture) {
		t.Fatalf("downloaded bytes = %q, want fixture %q", got, fixture)
	}
	if result.Bytes != int64(len(fixture)) {
		t.Fatalf("Download(real Rod page) bytes = %#v, want %d", result.Bytes, len(fixture))
	}
	if !strings.HasPrefix(result.ContentType, "text/plain") {
		t.Fatalf("Download(real Rod page) content type = %q, want text/plain", result.ContentType)
	}
}
