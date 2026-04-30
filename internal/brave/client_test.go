package brave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const braveTestAPIKey = "brave-test-token"

type searchRequest struct {
	query      string
	count      string
	country    string
	language   string
	safeSearch string
	offset     string
}

func newBraveTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	return NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  server.URL,
	})
}

func assertBraveRequest(t *testing.T, r *http.Request, wantPath string, want searchRequest) {
	t.Helper()

	if r.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != wantPath {
		t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
	}
	if got := r.Header.Get("X-Subscription-Token"); got != braveTestAPIKey {
		t.Fatalf("X-Subscription-Token = %q, want test token", got)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}

	query := r.URL.Query()
	assertQueryParam(t, query.Get("q"), want.query, "q")
	assertQueryParam(t, query.Get("count"), want.count, "count")
	assertQueryParam(t, query.Get("country"), want.country, "country")
	assertQueryParam(t, query.Get("search_lang"), want.language, "search_lang")
	assertQueryParam(t, query.Get("safesearch"), want.safeSearch, "safesearch")
	assertQueryParam(t, query.Get("offset"), want.offset, "offset")
}

func assertQueryParam(t *testing.T, got string, want string, name string) {
	t.Helper()

	if got != want {
		t.Fatalf("query %s = %q, want %q", name, got, want)
	}
}

func writeJSONResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func assertContentPack(t *testing.T, got content.Pack, want content.Pack) {
	t.Helper()

	if got.Type != want.Type {
		t.Fatalf("content pack type = %q, want %q", got.Type, want.Type)
	}
	assertItems(t, got.Items, want.Items)
	if !reflect.DeepEqual(got.Metadata, want.Metadata) {
		t.Fatalf("metadata = %#v, want %#v", got.Metadata, want.Metadata)
	}
}

func assertItems(t *testing.T, got []content.Item, want []content.Item) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %#v, want %#v", got, want)
	}
}

func assertMetadataString(t *testing.T, metadata map[string]any, key string, want string) {
	t.Helper()

	got, ok := metadata[key]
	if !ok {
		t.Fatalf("metadata[%q] missing", key)
	}
	if got != want {
		t.Fatalf("metadata[%q] = %v, want %q", key, got, want)
	}
}

func assertNoMetadata(t *testing.T, metadata map[string]any) {
	t.Helper()

	if len(metadata) != 0 {
		t.Fatalf("metadata = %#v, want empty", metadata)
	}
}

func assertMetadataKeyMissing(t *testing.T, metadata map[string]any, key string) {
	t.Helper()

	if _, ok := metadata[key]; ok {
		t.Fatalf("metadata[%q] = %v, want missing", key, metadata[key])
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want %q", want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func assertErrorDoesNotContain(t *testing.T, err error, unwanted string) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want non-nil error")
	}
	if strings.Contains(err.Error(), unwanted) {
		t.Fatalf("error = %v, want no substring %q", err, unwanted)
	}
}

type noWaitSleeper struct{}

func (noWaitSleeper) Sleep(ctx context.Context, _ time.Duration) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	default:
		return nil
	}
}

var _ httpclient.Sleeper = noWaitSleeper{}
