package httpdownload

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDownloadReturnsBodyWhenContentTypeAndSizeAreAllowed(t *testing.T) {
	const body = "%PDF-1.7\nsmall pdf\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/paper.pdf" {
			t.Fatalf("request path = %q, want /paper.pdf", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/pdf; charset=binary")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		writeHTTPBody(t, w, body)
	}))
	defer server.Close()

	downloader := New(Options{HTTPClient: server.Client()})
	got, err := downloader.Download(context.Background(), Request{
		URL:                 server.URL + "/paper.pdf",
		AllowedContentTypes: []string{"application/pdf"},
		MaxBytes:            64,
		Timeout:             time.Second,
	})
	if err != nil {
		t.Fatalf("Download(valid pdf) error = %v, want nil", err)
	}

	if got.URL != server.URL+"/paper.pdf" {
		t.Fatalf("Download(valid pdf) URL = %q, want request URL", got.URL)
	}
	if got.ContentType != "application/pdf" {
		t.Fatalf("Download(valid pdf) content type = %q, want application/pdf", got.ContentType)
	}
	if string(got.Bytes) != body {
		t.Fatalf("Download(valid pdf) body = %q, want %q", got.Bytes, body)
	}
}

func TestDownloadRejectsUnexpectedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		writeHTTPBody(t, w, "<html>not a pdf</html>")
	}))
	defer server.Close()

	downloader := New(Options{HTTPClient: server.Client()})
	_, err := downloader.Download(context.Background(), Request{
		URL:                 server.URL,
		AllowedContentTypes: []string{"application/pdf"},
		MaxBytes:            1024,
		Timeout:             time.Second,
	})

	if err == nil {
		t.Fatal("Download(text/html) error = nil, want unsupported content type")
	}
	if !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("Download(text/html) error = %v, want ErrUnsupportedContentType", err)
	}
}

func TestDownloadRejectsContentLengthOverLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "2048")
		writeHTTPBody(t, w, strings.Repeat("x", 32))
	}))
	defer server.Close()

	downloader := New(Options{HTTPClient: server.Client()})
	_, err := downloader.Download(context.Background(), Request{
		URL:                 server.URL,
		AllowedContentTypes: []string{"audio/mpeg"},
		MaxBytes:            1024,
		Timeout:             time.Second,
	})

	if err == nil {
		t.Fatal("Download(oversized content-length) error = nil, want file too large")
	}
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Download(oversized content-length) error = %v, want ErrFileTooLarge", err)
	}
}

func TestDownloadStopsWhenStreamingBodyExceedsLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		writeHTTPBody(t, w, "12345")
	}))
	defer server.Close()

	downloader := New(Options{HTTPClient: server.Client()})
	_, err := downloader.Download(context.Background(), Request{
		URL:                 server.URL,
		AllowedContentTypes: []string{"audio/mpeg"},
		MaxBytes:            4,
		Timeout:             time.Second,
	})

	if err == nil {
		t.Fatal("Download(stream over max bytes) error = nil, want file too large")
	}
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Download(stream over max bytes) error = %v, want ErrFileTooLarge", err)
	}
}

func TestDownloadRejectsHTTPFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusGone)
	}))
	defer server.Close()

	downloader := New(Options{HTTPClient: server.Client()})
	_, err := downloader.Download(context.Background(), Request{
		URL:                 server.URL,
		AllowedContentTypes: []string{"application/pdf"},
		MaxBytes:            1024,
		Timeout:             time.Second,
	})

	if err == nil {
		t.Fatal("Download(HTTP 410) error = nil, want HTTP status error")
	}
	if !errors.Is(err, ErrHTTPStatus) {
		t.Fatalf("Download(HTTP 410) error = %v, want ErrHTTPStatus", err)
	}
}

func TestDownloadEnforcesRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	defer server.Close()

	downloader := New(Options{HTTPClient: server.Client()})
	_, err := downloader.Download(context.Background(), Request{
		URL:                 server.URL,
		AllowedContentTypes: []string{"application/pdf"},
		MaxBytes:            1024,
		Timeout:             time.Nanosecond,
	})

	if err == nil {
		t.Fatal("Download(timeout) error = nil, want timeout")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Download(timeout) error = %v, want ErrTimeout", err)
	}
}

func TestDownloadUsesSafeDefaultsWhenOptionalLimitsAreUnset(t *testing.T) {
	const body = "plain body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeHTTPBody(t, w, body)
	}))
	defer server.Close()

	got, err := New(Options{}).Download(context.Background(), Request{URL: server.URL})
	if err != nil {
		t.Fatalf("Download(default options) error = %v, want nil", err)
	}
	if string(got.Bytes) != body {
		t.Fatalf("Download(default options) body = %q, want %q", got.Bytes, body)
	}
	if got.ContentType != "text/plain" {
		t.Fatalf("Download(default options) content type = %q, want text/plain", got.ContentType)
	}
}

func TestDownloadAcceptsBodyAtExactByteLimit(t *testing.T) {
	const body = "1234"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		writeHTTPBody(t, w, body)
	}))
	defer server.Close()

	got, err := New(Options{HTTPClient: server.Client()}).Download(context.Background(), Request{
		URL:      server.URL,
		MaxBytes: int64(len(body)),
	})
	if err != nil {
		t.Fatalf("Download(exact byte limit) error = %v, want nil", err)
	}
	if string(got.Bytes) != body {
		t.Fatalf("Download(exact byte limit) body = %q, want %q", got.Bytes, body)
	}
}

func TestDownloadNormalizesMalformedContentTypeHeader(t *testing.T) {
	const body = "plain body"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", " Text/Plain ; bad")
		writeHTTPBody(t, w, body)
	}))
	defer server.Close()

	got, err := New(Options{HTTPClient: server.Client()}).Download(context.Background(), Request{
		URL:                 server.URL,
		AllowedContentTypes: []string{"text/plain ; bad"},
	})
	if err != nil {
		t.Fatalf("Download(malformed content type) error = %v, want nil", err)
	}
	if got.ContentType != "text/plain ; bad" {
		t.Fatalf("Download(malformed content type) content type = %q, want normalized fallback", got.ContentType)
	}
}

func TestDownloadReturnsBodyReadError(t *testing.T) {
	downloader := New(Options{HTTPClient: &http.Client{Transport: staticDownloadTransport{}}})

	_, err := downloader.Download(context.Background(), Request{URL: "https://example.test/file"})
	if err == nil {
		t.Fatal("Download(body read failure) error = nil, want read error")
	}
	if !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("Download(body read failure) error = %v, want read failure context", err)
	}
}

func TestDownloadHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := New(Options{}).Download(ctx, Request{URL: "http://127.0.0.1/never-called"})
	if err == nil {
		t.Fatal("Download(canceled context) error = nil, want context canceled")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Download(canceled context) error = %v, want context.Canceled", err)
	}
}

type staticDownloadTransport struct{}

func (staticDownloadTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       errReadCloser{},
	}, nil
}

type errReadCloser struct{}

func (errReadCloser) Read(_ []byte) (int, error) {
	return 0, errors.New("read failed")
}

func (errReadCloser) Close() error {
	return nil
}

var _ io.ReadCloser = errReadCloser{}

func writeHTTPBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write HTTP body: %v", err)
	}
}
