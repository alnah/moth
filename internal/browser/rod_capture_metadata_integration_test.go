//go:build browser

package browser

import (
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRodPoolCapturesPDFAndResponseMetadata(t *testing.T) {
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/print":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><title>Printable</title><h1>Printable report</h1>`))
		case "/metadata":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-Moth-Test", "metadata")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte("metadata body"))
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 20*time.Second)

	pdfPath := filepath.Join(t.TempDir(), "page.pdf")
	err := pool.PDF(ctx, PDFRequest{URL: server.URL + "/print", Path: pdfPath})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("PDF(real Rod page) error = %v, want nil", err)
	}
	requireFilePrefix(t, pdfPath, "%PDF-")

	metadata, err := pool.ResponseMetadata(ctx, ResponseMetadataRequest{
		URL:            server.URL + "/metadata",
		MaxHeaderBytes: 4096,
	})
	if err != nil {
		t.Fatalf("ResponseMetadata(real pool) error = %v, want nil", err)
	}
	if metadata.Status != http.StatusAccepted {
		t.Fatalf("ResponseMetadata(real pool) status = %d, want %d", metadata.Status, http.StatusAccepted)
	}
	if !strings.HasPrefix(metadata.ContentType, "text/plain") {
		t.Fatalf("ResponseMetadata(real pool) content type = %q, want text/plain", metadata.ContentType)
	}
	if got := metadata.Headers["x-moth-test"]; len(got) != 1 || got[0] != "metadata" {
		t.Fatalf("ResponseMetadata(real pool) x-moth-test = %#v, want [metadata]", got)
	}
}

func TestRodPoolBlocksConfiguredImageResources(t *testing.T) {
	var imageRequests atomic.Int32
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html>
<title>Blocking</title>
<img id="probe" src="/blocked.png" onload="document.body.append(' image loaded')" onerror="document.body.append(' image blocked')">
<main>Resource page</main>`))
		case "/blocked.png":
			imageRequests.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t, WithBlockedResources(ResourceImages))
	ctx := newBrowserTestContext(t, 20*time.Second)

	item, err := pool.FetchPage(ctx, PageRequest{URL: server.URL + "/page"})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("FetchPage(blocked image real Rod page) error = %v, want nil", err)
	}
	if imageRequests.Load() != 0 {
		t.Fatalf("image requests = %d, want 0 when ResourceImages is blocked", imageRequests.Load())
	}
	if !strings.Contains(item.Text, "image blocked") {
		t.Fatalf("FetchPage(blocked image real Rod page) text = %q, want blocked-image marker", item.Text)
	}
}
