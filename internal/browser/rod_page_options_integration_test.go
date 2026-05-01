//go:build browser

package browser

import (
	"net/http"
	"testing"
	"time"
)

func TestRodPoolPropagatesCustomHeadersAndUserAgent(t *testing.T) {
	type headerObservation struct {
		testHeader string
		userAgent  string
	}
	observations := make(chan headerObservation, 1)
	server := newBrowserTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/headers":
			select {
			case observations <- headerObservation{
				testHeader: r.Header.Get("X-Moth-Test"),
				userAgent:  r.UserAgent(),
			}:
			default:
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><title>Header page</title><main>headers observed</main>`))
		default:
			http.NotFound(w, r)
		}
	})

	pool := newBrowserPool(t)
	ctx := newBrowserTestContext(t, 15*time.Second)
	const userAgent = "MothBrowserTest/1.0"

	item, err := pool.FetchPage(ctx, PageRequest{
		URL:       server.URL + "/headers",
		Headers:   map[string]string{"X-Moth-Test": "header-value"},
		UserAgent: userAgent,
	})
	handleBrowserUnavailable(t, err)
	if err != nil {
		t.Fatalf("FetchPage(headers real Rod page) error = %v, want nil", err)
	}
	if item.Title != "Header page" {
		t.Fatalf("FetchPage(headers real Rod page) title = %q, want Header page", item.Title)
	}

	select {
	case got := <-observations:
		if got.testHeader != "header-value" {
			t.Fatalf("server X-Moth-Test = %q, want header-value", got.testHeader)
		}
		if got.userAgent != userAgent {
			t.Fatalf("server User-Agent = %q, want %q", got.userAgent, userAgent)
		}
	case <-ctx.Done():
		t.Fatalf("server did not observe headers before context done: %v", ctx.Err())
	}
}
