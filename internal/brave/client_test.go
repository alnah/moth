package brave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const braveTestAPIKey = "brave-test-token"

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
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "2000")
		w.Header().Set("X-RateLimit-Remaining", "1998")
		w.Header().Set("X-RateLimit-Reset", "1710000000")
		writeResponse(t, w, `{
			"web": {
				"results": [
					{
						"title": "Go project",
						"url": "https://go.dev/",
						"description": "Go is expressive and secure."
					},
					{
						"title": "Go security policy",
						"url": "https://go.dev/security/",
						"description": "How the Go team handles security reports."
					}
				]
			}
		}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  server.URL,
	})

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

	wantItems := []content.Item{
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
	}
	assertItems(t, result.Items, wantItems)
	assertMetadataString(t, result.Metadata, "rate_limit_limit", "2000")
	assertMetadataString(t, result.Metadata, "rate_limit_remaining", "1998")
	assertMetadataString(t, result.Metadata, "rate_limit_reset", "1710000000")
}

func TestSearchImagesSendsDocumentedRequestAndMapsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/images/search", searchRequest{
			query:      "blue gopher",
			count:      "3",
			country:    "CA",
			language:   "en",
			safeSearch: "moderate",
			offset:     "6",
		})
		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, `{
			"results": [
				{
					"title": "Blue gopher",
					"url": "https://example.com/gopher-page",
					"description": "A blue gopher mascot.",
					"thumbnail": {"src": "https://cdn.example.com/gopher-thumb.jpg"},
					"properties": {
						"url": "https://cdn.example.com/gopher-full.jpg",
						"width": 640,
						"height": 480
					}
				}
			]
		}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  server.URL,
	})

	result, err := client.SearchImages(context.Background(), SearchOptions{
		Query:      "blue gopher",
		Count:      3,
		Country:    "CA",
		Language:   "en",
		SafeSearch: "moderate",
		Offset:     6,
	})
	if err != nil {
		t.Fatalf("SearchImages error = %v, want nil", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("image items len = %d, want 1", len(result.Items))
	}

	item := result.Items[0]
	if item.Kind != content.KindImage {
		t.Fatalf("image kind = %q, want %q", item.Kind, content.KindImage)
	}
	if item.URL != "https://cdn.example.com/gopher-full.jpg" {
		t.Fatalf("image URL = %q, want original image URL", item.URL)
	}
	if item.Title != "Blue gopher" {
		t.Fatalf("image title = %q, want mapped title", item.Title)
	}
	if item.Text != "A blue gopher mascot." {
		t.Fatalf("image text = %q, want mapped description", item.Text)
	}
	assertMetadataString(t, item.Metadata, "page_url", "https://example.com/gopher-page")
	assertMetadataString(t, item.Metadata, "thumbnail_url", "https://cdn.example.com/gopher-thumb.jpg")
	assertMetadataInt(t, item.Metadata, "width", 640)
	assertMetadataInt(t, item.Metadata, "height", 480)
}

func TestSearchVideosSendsDocumentedRequestAndMapsResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/videos/search", searchRequest{
			query:      "go conference",
			count:      "5",
			country:    "US",
			language:   "en",
			safeSearch: "off",
			offset:     "10",
		})
		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, `{
			"results": [
				{
					"title": "GopherCon talk",
					"url": "https://video.example.com/watch/go",
					"description": "A practical Go conference talk.",
					"thumbnail": {"src": "https://video.example.com/thumb.jpg"},
					"duration": "PT3M20S",
					"publisher": "GopherCon"
				}
			]
		}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  server.URL,
	})

	result, err := client.SearchVideos(context.Background(), SearchOptions{
		Query:      "go conference",
		Count:      5,
		Country:    "US",
		Language:   "en",
		SafeSearch: "off",
		Offset:     10,
	})
	if err != nil {
		t.Fatalf("SearchVideos error = %v, want nil", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("video items len = %d, want 1", len(result.Items))
	}

	item := result.Items[0]
	if item.Kind != content.KindVideo {
		t.Fatalf("video kind = %q, want %q", item.Kind, content.KindVideo)
	}
	if item.URL != "https://video.example.com/watch/go" {
		t.Fatalf("video URL = %q, want mapped URL", item.URL)
	}
	if item.Title != "GopherCon talk" {
		t.Fatalf("video title = %q, want mapped title", item.Title)
	}
	if item.Text != "A practical Go conference talk." {
		t.Fatalf("video text = %q, want mapped description", item.Text)
	}
	assertMetadataString(t, item.Metadata, "thumbnail_url", "https://video.example.com/thumb.jpg")
	assertMetadataString(t, item.Metadata, "duration", "PT3M20S")
	assertMetadataString(t, item.Metadata, "publisher", "GopherCon")
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

		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, `{
			"web": {
				"results": [
					{
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
	assertItems(t, result.Items, []content.Item{
		{
			Kind:  content.KindPage,
			URL:   "https://example.com/recovered",
			Title: "Recovered result",
			Text:  "Returned after one retry.",
		},
	})
}

func TestSearchFailsBeforeRequestWhenAPIKeyMissing(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "request should not happen", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{
		Settings: config.Settings{},
		BaseURL:  server.URL,
	})

	_, err := client.SearchWeb(context.Background(), SearchOptions{Query: "blocked"})
	if err == nil {
		t.Fatal("SearchWeb missing API key error = nil, want error")
	}
	assertErrorContains(t, err, "brave")
	assertErrorContains(t, err, "api key")
	if requests.Load() != 0 {
		t.Fatalf("server requests = %d, want missing key to fail before request", requests.Load())
	}
}

func TestSearchReturnsContextualProviderErrorForNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/web/search", searchRequest{
			query:      "denied",
			count:      "1",
			country:    "US",
			language:   "en",
			safeSearch: "strict",
			offset:     "0",
		})
		http.Error(w, `{"error":"invalid subscription"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  server.URL,
	})

	_, err := client.SearchWeb(context.Background(), SearchOptions{
		Query:      "denied",
		Count:      1,
		Country:    "US",
		Language:   "en",
		SafeSearch: "strict",
		Offset:     0,
	})
	if err == nil {
		t.Fatal("SearchWeb non-2xx error = nil, want provider error")
	}
	assertErrorContains(t, err, "brave")
	assertErrorContains(t, err, "web")
	assertErrorContains(t, err, "401")
	assertErrorContains(t, err, "invalid subscription")
	if strings.Contains(err.Error(), braveTestAPIKey) {
		t.Fatalf("provider error leaks API key: %v", err)
	}
}

func TestSearchReturnsContextualDecodeErrorForMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertBraveRequest(t, r, "/res/v1/images/search", searchRequest{
			query:      "bad json",
			count:      "1",
			country:    "US",
			language:   "en",
			safeSearch: "strict",
			offset:     "0",
		})
		w.Header().Set("Content-Type", "application/json")
		writeResponse(t, w, `{"results":[`)
	}))
	defer server.Close()

	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  server.URL,
	})

	_, err := client.SearchImages(context.Background(), SearchOptions{
		Query:      "bad json",
		Count:      1,
		Country:    "US",
		Language:   "en",
		SafeSearch: "strict",
		Offset:     0,
	})
	if err == nil {
		t.Fatal("SearchImages malformed JSON error = nil, want decode error")
	}
	assertErrorContains(t, err, "brave")
	assertErrorContains(t, err, "images")
	assertErrorContains(t, err, "decode")
}

type searchRequest struct {
	query      string
	count      string
	country    string
	language   string
	safeSearch string
	offset     string
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

func writeResponse(t *testing.T, w io.Writer, body string) {
	t.Helper()

	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func assertItems(t *testing.T, got []content.Item, want []content.Item) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("items len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Kind != want[i].Kind {
			t.Fatalf("items[%d].Kind = %q, want %q", i, got[i].Kind, want[i].Kind)
		}
		if got[i].URL != want[i].URL {
			t.Fatalf("items[%d].URL = %q, want %q", i, got[i].URL, want[i].URL)
		}
		if got[i].Title != want[i].Title {
			t.Fatalf("items[%d].Title = %q, want %q", i, got[i].Title, want[i].Title)
		}
		if got[i].Text != want[i].Text {
			t.Fatalf("items[%d].Text = %q, want %q", i, got[i].Text, want[i].Text)
		}
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

func assertMetadataInt(t *testing.T, metadata map[string]any, key string, want int) {
	t.Helper()

	got, ok := metadata[key]
	if !ok {
		t.Fatalf("metadata[%q] missing", key)
	}
	if got != want {
		t.Fatalf("metadata[%q] = %v, want %d", key, got, want)
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
