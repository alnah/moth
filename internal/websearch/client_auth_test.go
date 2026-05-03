package websearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
)

func TestSearchFailsBeforeRequestWhenAPIKeyMissing(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "request should not happen", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{
		Credentials: config.Credentials{},
		BaseURL:     server.URL,
	})

	_, err := client.SearchWeb(context.Background(), Options{Query: "blocked"})
	if err == nil {
		t.Fatal("SearchWeb missing API key error = nil, want error")
	}
	assertErrorContains(t, err, "brave")
	assertErrorContains(t, err, "api key")
	if requests.Load() != 0 {
		t.Fatalf("server requests = %d, want missing key to fail before request", requests.Load())
	}
}

func TestSearchTrimsAPIKeyBeforeSendingHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/web/search", searchRequest{
			query:  "trimmed auth",
			offset: "0",
		})
		writeJSONResponse(t, w, `{"type":"search","web":{"type":"search","results":[]}}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		Credentials: config.Credentials{BraveAPIKey: "  \t" + braveTestAPIKey + "\n  "},
		BaseURL:     server.URL,
	})

	result, err := client.SearchWeb(context.Background(), Options{Query: "trimmed auth"})
	if err != nil {
		t.Fatalf("SearchWeb error = %v, want nil", err)
	}
	assertContentPack(t, result, content.Pack{
		Type:  content.TypeContentPack,
		Items: []content.Item{},
	})
}
