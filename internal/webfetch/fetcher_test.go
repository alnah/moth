package webfetch_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpdownload"
	"github.com/alnah/moth/internal/webfetch"
)

func TestFetchHTTPNormalizesHTMLPageWithSourceMetadata(t *testing.T) {
	downloader := &fakeDownloader{
		response: httpdownload.Response{
			URL:         "https://example.test/final",
			ContentType: "text/html",
			Bytes: []byte(`<!doctype html>
<html>
	<head><title>Example title</title><script>alert("ignored")</script></head>
	<body><main>Hello <strong>Piccolo</strong>.</main><a href="/next">next</a><img src="/cover.png"></body>
</html>`),
		},
	}

	pack, err := webfetch.New(webfetch.Options{Downloader: downloader}).Fetch(context.Background(), webfetch.Request{
		URL:         "https://example.test/start",
		IncludeText: true,
		MaxBytes:    4096,
		Timeout:     2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Fetch(http html) error = %v, want nil", err)
	}

	if len(downloader.requests) != 1 {
		t.Fatalf("downloader calls = %d, want exactly one bounded HTTP fetch", len(downloader.requests))
	}
	request := downloader.requests[0]
	if request.URL != "https://example.test/start" {
		t.Fatalf("download URL = %q, want request URL", request.URL)
	}
	if request.MaxBytes != 4096 {
		t.Fatalf("download max bytes = %d, want propagated request limit", request.MaxBytes)
	}
	if request.Timeout != 2*time.Second {
		t.Fatalf("download timeout = %s, want propagated request timeout", request.Timeout)
	}

	if pack.Type != content.TypeContentPack {
		t.Fatalf("pack type = %q, want %q", pack.Type, content.TypeContentPack)
	}
	if len(pack.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(pack.Items))
	}
	item := pack.Items[0]
	if item.Kind != content.KindPage {
		t.Fatalf("item kind = %q, want page", item.Kind)
	}
	if item.URL != "https://example.test/final" {
		t.Fatalf("item URL = %q, want final URL", item.URL)
	}
	if item.Title != "Example title" {
		t.Fatalf("item title = %q, want extracted title", item.Title)
	}
	assertTextContainsInOrder(t, item.Text, "Hello", "Piccolo", "next")
	assertTextExcludes(t, item.Text, "alert", "ignored")
	assertMetadataString(t, item.Metadata, "source", "http")
	assertMetadataString(t, item.Metadata, "final_url", "https://example.test/final")
	assertMetadataString(t, item.Metadata, "content_type", "text/html")
}

func TestFetchHTTPNormalizesNonHTMLAsFile(t *testing.T) {
	downloader := &fakeDownloader{
		response: httpdownload.Response{
			URL:         "https://example.test/archive.bin",
			ContentType: "application/octet-stream",
			Bytes:       []byte{0x01, 0x02, 0x03},
		},
	}

	pack, err := webfetch.New(webfetch.Options{Downloader: downloader}).Fetch(context.Background(), webfetch.Request{
		URL:      "https://example.test/archive.bin",
		MaxBytes: 3,
	})
	if err != nil {
		t.Fatalf("Fetch(http file) error = %v, want nil", err)
	}

	if len(pack.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(pack.Items))
	}
	item := pack.Items[0]
	if item.Kind != content.KindFile {
		t.Fatalf("item kind = %q, want file", item.Kind)
	}
	if item.Text != "" {
		t.Fatalf("file text = %q, want empty text for non-text file", item.Text)
	}
	assertMetadataString(t, item.Metadata, "source", "http")
	assertMetadataString(t, item.Metadata, "content_type", "application/octet-stream")
	assertMetadataInt(t, item.Metadata, "bytes", 3)
}

func TestFetchBrowserUsesInjectedRenderedPageFetcher(t *testing.T) {
	downloader := &fakeDownloader{}
	browserFetcher := &fakeBrowserFetcher{
		response: webfetch.BrowserResponse{
			URL:         "https://example.test/rendered",
			ContentType: "text/html",
			HTML:        "<html><head><title>Rendered</title></head><body>Rendered text</body></html>",
		},
	}

	pack, err := webfetch.New(webfetch.Options{
		Downloader:     downloader,
		BrowserFetcher: browserFetcher,
	}).Fetch(context.Background(), webfetch.Request{
		URL:         "https://example.test/app",
		UseBrowser:  true,
		IncludeText: true,
		MaxBytes:    8192,
		Timeout:     time.Second,
	})
	if err != nil {
		t.Fatalf("Fetch(browser) error = %v, want nil", err)
	}

	if len(downloader.requests) != 0 {
		t.Fatalf("HTTP downloader calls = %d, want browser path only", len(downloader.requests))
	}
	if len(browserFetcher.requests) != 1 {
		t.Fatalf("browser fetch calls = %d, want one injected rendered fetch", len(browserFetcher.requests))
	}
	request := browserFetcher.requests[0]
	if request.URL != "https://example.test/app" {
		t.Fatalf("browser URL = %q, want request URL", request.URL)
	}
	if request.MaxBytes != 8192 {
		t.Fatalf("browser max bytes = %d, want propagated request limit", request.MaxBytes)
	}
	if request.Timeout != time.Second {
		t.Fatalf("browser timeout = %s, want propagated request timeout", request.Timeout)
	}

	item := pack.Items[0]
	if item.Kind != content.KindPage {
		t.Fatalf("browser item kind = %q, want page", item.Kind)
	}
	if item.URL != "https://example.test/rendered" {
		t.Fatalf("browser item URL = %q, want rendered final URL", item.URL)
	}
	if item.Text != "Rendered text" {
		t.Fatalf("browser item text = %q, want rendered visible text", item.Text)
	}
	assertMetadataString(t, item.Metadata, "source", "browser")
	assertMetadataString(t, item.Metadata, "content_type", "text/html")
}

func TestFetchPreservesFileTooLargeSemantics(t *testing.T) {
	downloader := &fakeDownloader{err: httpdownload.ErrFileTooLarge}

	_, err := webfetch.New(webfetch.Options{Downloader: downloader}).Fetch(context.Background(), webfetch.Request{
		URL:      "https://example.test/huge.pdf",
		MaxBytes: 1024,
	})
	if err == nil {
		t.Fatal("Fetch(oversized) error = nil, want file_too_large")
	}
	if !errors.Is(err, httpdownload.ErrFileTooLarge) {
		t.Fatalf("Fetch(oversized) error = %v, want errors.Is ErrFileTooLarge", err)
	}
	if !strings.Contains(err.Error(), "fetch https://example.test/huge.pdf") {
		t.Fatalf("Fetch(oversized) error = %v, want fetch URL context", err)
	}
}

func TestFetchErrorsIncludeContextAndPreserveSentinel(t *testing.T) {
	sentinel := errors.New("sentinel failure")
	downloader := &fakeDownloader{err: sentinel}

	_, err := webfetch.New(webfetch.Options{Downloader: downloader}).Fetch(context.Background(), webfetch.Request{
		URL: "https://example.test/broken",
	})
	if err == nil {
		t.Fatal("Fetch(broken) error = nil, want error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Fetch(broken) error = %v, want sentinel preserved", err)
	}
	if !strings.Contains(err.Error(), "fetch https://example.test/broken") {
		t.Fatalf("Fetch(broken) error = %v, want fetch URL context", err)
	}
}

type fakeDownloader struct {
	response httpdownload.Response
	err      error
	requests []httpdownload.Request
}

func (downloader *fakeDownloader) Download(
	_ context.Context,
	request httpdownload.Request,
) (httpdownload.Response, error) {
	downloader.requests = append(downloader.requests, request)
	if downloader.err != nil {
		return httpdownload.Response{}, downloader.err
	}
	return downloader.response, nil
}

type fakeBrowserFetcher struct {
	response webfetch.BrowserResponse
	err      error
	requests []webfetch.BrowserRequest
}

func (fetcher *fakeBrowserFetcher) FetchRenderedPage(
	_ context.Context,
	request webfetch.BrowserRequest,
) (webfetch.BrowserResponse, error) {
	fetcher.requests = append(fetcher.requests, request)
	if fetcher.err != nil {
		return webfetch.BrowserResponse{}, fetcher.err
	}
	return fetcher.response, nil
}

func assertTextContainsInOrder(t *testing.T, text string, fragments ...string) {
	t.Helper()

	if strings.TrimSpace(text) == "" {
		t.Fatal("item text is empty, want visible page text")
	}
	searchStart := 0
	for _, fragment := range fragments {
		index := strings.Index(text[searchStart:], fragment)
		if index < 0 {
			t.Fatalf("item text = %q, want visible fragment %q after byte %d", text, fragment, searchStart)
		}
		searchStart += index + len(fragment)
	}
}

func assertTextExcludes(t *testing.T, text string, fragments ...string) {
	t.Helper()

	for _, fragment := range fragments {
		if strings.Contains(text, fragment) {
			t.Fatalf("item text = %q, want fragment %q excluded", text, fragment)
		}
	}
}

func assertMetadataString(t *testing.T, metadata map[string]any, key string, want string) {
	t.Helper()

	got, ok := metadata[key].(string)
	if !ok {
		t.Fatalf("metadata[%q] = %#v, want string %q", key, metadata[key], want)
	}
	if got != want {
		t.Fatalf("metadata[%q] = %q, want %q", key, got, want)
	}
}

func assertMetadataInt(t *testing.T, metadata map[string]any, key string, want int64) {
	t.Helper()

	var got int64
	switch value := metadata[key].(type) {
	case int:
		got = int64(value)
	case int64:
		got = value
	case float64:
		got = int64(value)
	default:
		t.Fatalf("metadata[%q] = %#v, want numeric %d", key, metadata[key], want)
	}
	if got != want {
		t.Fatalf("metadata[%q] = %d, want %d", key, got, want)
	}
}
