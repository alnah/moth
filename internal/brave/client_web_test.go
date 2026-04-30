package brave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

func TestSearchWebSendsDocumentedRequestAndMapsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/web/search", searchRequest{
			query:      "gopher security",
			count:      "7",
			country:    "FR",
			language:   "fr",
			safeSearch: "strict",
			offset:     "14",
		})
		w.Header().Set("X-RateLimit-Limit", "2000")
		w.Header().Set("X-RateLimit-Remaining", "1998")
		w.Header().Set("X-RateLimit-Reset", "1710000000")
		writeJSONResponse(t, w, `{
			"type": "search",
			"web": {
				"type": "search",
				"results": [
					{
						"type": "search_result",
						"title": "Go project",
						"url": "https://go.dev/",
						"description": "Go is expressive and secure."
					},
					{
						"type": "search_result",
						"title": "Go security policy",
						"url": "https://go.dev/security/",
						"description": "How the Go team handles security reports."
					}
				]
			}
		}`)
	}))
	defer server.Close()

	client := newBraveTestClient(t, server)

	result, err := client.SearchWeb(context.Background(), SearchOptions{
		Query:      "gopher security",
		Count:      7,
		Country:    "FR",
		Language:   "fr",
		SafeSearch: "strict",
		Offset:     14,
	})
	if err != nil {
		t.Fatalf("SearchWeb error = %v, want nil", err)
	}

	assertContentPack(t, result, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindPage,
				URL:   "https://go.dev/",
				Title: "Go project",
				Text:  "Go is expressive and secure.",
			},
			{
				Kind:  content.KindPage,
				URL:   "https://go.dev/security/",
				Title: "Go security policy",
				Text:  "How the Go team handles security reports.",
			},
		},
		Metadata: map[string]any{
			"rate_limit_limit":     "2000",
			"rate_limit_remaining": "1998",
			"rate_limit_reset":     "1710000000",
		},
	})
}

func TestSearchWebDefaultsOffsetToZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/web/search", searchRequest{
			query:  "offset default",
			offset: "0",
		})
		writeJSONResponse(t, w, `{"type":"search","web":{"type":"search","results":[]}}`)
	}))
	defer server.Close()

	client := newBraveTestClient(t, server)

	result, err := client.SearchWeb(context.Background(), SearchOptions{Query: "offset default"})
	if err != nil {
		t.Fatalf("SearchWeb error = %v, want nil", err)
	}

	assertContentPack(t, result, content.Pack{
		Type:  content.TypeContentPack,
		Items: []content.Item{},
	})
}

func TestSearchUsesRetryingHTTPClient(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/web/search", searchRequest{
			query:      "retryable search",
			count:      "1",
			country:    "US",
			language:   "en",
			safeSearch: "strict",
			offset:     "0",
		})

		if attempts.Add(1) == 1 {
			http.Error(w, "temporary brave outage", http.StatusServiceUnavailable)
			return
		}

		writeJSONResponse(t, w, `{
			"type": "search",
			"web": {
				"type": "search",
				"results": [
					{
						"type": "search_result",
						"title": "Recovered result",
						"url": "https://example.com/recovered",
						"description": "Returned after one retry."
					}
				]
			}
		}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  server.URL,
		HTTPClient: httpclient.New(httpclient.Options{
			HTTPClient: &http.Client{Transport: server.Client().Transport},
			Attempts:   2,
			RetryBase:  time.Nanosecond,
			Jitter:     httpclient.NoJitter,
			Sleeper:    noWaitSleeper{},
		}),
	})

	result, err := client.SearchWeb(context.Background(), SearchOptions{
		Query:      "retryable search",
		Count:      1,
		Country:    "US",
		Language:   "en",
		SafeSearch: "strict",
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("SearchWeb retry error = %v, want nil", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("server attempts = %d, want retrying client to make 2 attempts", attempts.Load())
	}
	assertContentPack(t, result, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindPage,
				URL:   "https://example.com/recovered",
				Title: "Recovered result",
				Text:  "Returned after one retry.",
			},
		},
	})
}
