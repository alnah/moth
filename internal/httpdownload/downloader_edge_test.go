package httpdownload

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
