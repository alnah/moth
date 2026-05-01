package browser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRodWorkerResponseMetadataUsesBoundedHTTPProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("metadata method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Probe", "ok")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "metadata body is intentionally ignored")
	}))
	defer server.Close()

	worker := &rodWorker{}
	got, err := worker.ResponseMetadata(context.Background(), ResponseMetadataRequest{URL: server.URL + "/probe"})
	if err != nil {
		t.Fatalf("ResponseMetadata() error = %v, want nil", err)
	}
	if got.URL != server.URL+"/probe" {
		t.Fatalf("metadata URL = %q, want probe URL", got.URL)
	}
	if got.Status != http.StatusAccepted {
		t.Fatalf("metadata status = %d, want %d", got.Status, http.StatusAccepted)
	}
	if got.ContentType != "text/plain; charset=utf-8" {
		t.Fatalf("metadata content type = %q, want text/plain", got.ContentType)
	}
	if values := got.Headers["X-Probe"]; len(values) != 1 || values[0] != "ok" {
		t.Fatalf("metadata headers = %#v, want X-Probe", got.Headers)
	}
}

func TestRodWorkerResponseMetadataReportsRequestErrors(t *testing.T) {
	worker := &rodWorker{}
	_, err := worker.ResponseMetadata(context.Background(), ResponseMetadataRequest{URL: "http://[::1"})
	if err == nil {
		t.Fatal("ResponseMetadata(invalid URL) error = nil, want error")
	}
}
