//go:build browser

package browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const browserRequiredEnv = "MOTH_BROWSER_REQUIRED"

func newBrowserTestContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func newBrowserTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func newBrowserPool(t *testing.T, options ...Option) *Pool {
	t.Helper()
	pool := NewPool(1, options...)
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

func requireNonEmptyFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("%s size = 0, want bytes", path)
	}
}

func requireFilePrefix(t *testing.T, path string, prefix string) {
	t.Helper()
	contents, err := os.ReadFile(path) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.HasPrefix(string(contents), prefix) {
		preview := string(contents)
		if len(preview) > 8 {
			preview = preview[:8]
		}
		t.Fatalf("%s prefix = %q, want %q", path, preview, prefix)
	}
}

func handleBrowserUnavailable(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrBrowserMissing) {
		return
	}
	if os.Getenv(browserRequiredEnv) == "1" {
		t.Fatalf("browser unavailable while %s=1: %v", browserRequiredEnv, err)
	}
	t.Skipf("browser unavailable for browser-tag integration test: %v", err)
}
