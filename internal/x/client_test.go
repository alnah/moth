package x

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const xTestBearerToken = "x-test-bearer-token"

func TestSearchRecentSendsBearerTokenAndMapsSocialPosts(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		assertXBearerRequest(t, r, "/2/tweets/search/recent")
		query := r.URL.Query()
		assertQueryValue(t, query.Get("query"), "from:golang", "query")
		assertQueryValue(t, query.Get("max_results"), "10", "max_results")
		assertQueryValue(t, query.Get("expansions"), "author_id", "expansions")
		assertQueryContains(t, query.Get("tweet.fields"), "created_at", "tweet.fields")
		assertQueryContains(t, query.Get("user.fields"), "username", "user.fields")
		w.Header().Set("X-Rate-Limit-Limit", "300")
		w.Header().Set("X-Rate-Limit-Remaining", "299")
		w.Header().Set("X-Rate-Limit-Reset", "1710000000")
		writeXJSON(t, w, `{
			"data": [
				{"id":"20", "text":"Go 1.26 is out", "author_id":"100", "created_at":"2026-05-01T12:00:00.000Z"},
				{"id":"21", "text":"Toolchain notes", "author_id":"101", "created_at":"2026-05-01T13:00:00.000Z"}
			],
			"includes": {"users": [
				{"id":"100", "username":"golang", "name":"Go"},
				{"id":"101", "username":"gopher", "name":"Gopher"}
			]},
			"meta": {"result_count":2, "next_token":"next-page"}
		}`)
	}))
	defer server.Close()

	pack, err := newXTestClient(server).SearchRecent(context.Background(), SearchOptions{
		Query:      "from:golang",
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("SearchRecent() error = %v, want nil", err)
	}

	want := content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindSocialPost,
				URL:   "https://x.com/golang/status/20",
				Title: "@golang",
				Text:  "Go 1.26 is out",
				Metadata: map[string]any{
					"post_id":         "20",
					"author_id":       "100",
					"author_username": "golang",
					"author_name":     "Go",
					"created_at":      "2026-05-01T12:00:00.000Z",
				},
			},
			{
				Kind:  content.KindSocialPost,
				URL:   "https://x.com/gopher/status/21",
				Title: "@gopher",
				Text:  "Toolchain notes",
				Metadata: map[string]any{
					"post_id":         "21",
					"author_id":       "101",
					"author_username": "gopher",
					"author_name":     "Gopher",
					"created_at":      "2026-05-01T13:00:00.000Z",
				},
			},
		},
		Metadata: map[string]any{
			"result_count":         2,
			"next_token":           "next-page",
			"rate_limit_limit":     "300",
			"rate_limit_remaining": "299",
			"rate_limit_reset":     "1710000000",
		},
	}
	if diff := cmp.Diff(want, pack, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("SearchRecent() mismatch (-want +got):\n%s", diff)
	}
	assertXContentPackJSONWarningsAreArrays(t, pack)
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("SearchRecent() requests = %d, want 1; default must not follow pagination or enrich posts", got)
	}
}

func TestSearchRecentDefaultsToTenResultsAndOneRequest(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := atomic.AddInt32(&requests, 1); got > 1 {
			t.Fatalf("SearchRecent() made hidden request %d to %s", got, r.URL.String())
		}
		assertXBearerRequest(t, r, "/2/tweets/search/recent")
		assertQueryValue(t, r.URL.Query().Get("max_results"), "10", "max_results")
		writeXJSON(t, w, `{"data":[], "meta":{"result_count":0, "next_token":"must-not-fetch"}}`)
	}))
	defer server.Close()

	pack, err := newXTestClient(server).SearchRecent(context.Background(), SearchOptions{Query: "go"})
	if err != nil {
		t.Fatalf("SearchRecent(defaults) error = %v, want nil", err)
	}
	if got := len(pack.Items); got != 0 {
		t.Fatalf("SearchRecent(defaults) items = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("SearchRecent(defaults) requests = %d, want 1", got)
	}
}

func TestSearchRecentUsesExplicitNextToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/tweets/search/recent")
		assertQueryValue(t, r.URL.Query().Get("next_token"), "older-page", "next_token")
		writeXJSON(t, w, `{"data":[], "meta":{"result_count":0}}`)
	}))
	defer server.Close()

	_, err := newXTestClient(server).SearchRecent(context.Background(), SearchOptions{
		Query:     "go",
		NextToken: "older-page",
	})
	if err != nil {
		t.Fatalf("SearchRecent(NextToken) error = %v, want nil", err)
	}
}

func TestLookupPostMapsTextAuthorDateAndPermalink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/tweets/20")
		query := r.URL.Query()
		assertQueryValue(t, query.Get("expansions"), "author_id", "expansions")
		assertQueryContains(t, query.Get("tweet.fields"), "created_at", "tweet.fields")
		assertQueryContains(t, query.Get("user.fields"), "username", "user.fields")
		writeXJSON(t, w, `{
			"data": {"id":"20", "text":"Single lookup", "author_id":"100", "created_at":"2026-05-01T12:00:00.000Z"},
			"includes": {"users": [{"id":"100", "username":"golang", "name":"Go"}]}
		}`)
	}))
	defer server.Close()

	pack, err := newXTestClient(server).LookupPost(context.Background(), LookupPostOptions{ID: "20"})
	if err != nil {
		t.Fatalf("LookupPost() error = %v, want nil", err)
	}

	want := content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{{
			Kind:  content.KindSocialPost,
			URL:   "https://x.com/golang/status/20",
			Title: "@golang",
			Text:  "Single lookup",
			Metadata: map[string]any{
				"post_id":         "20",
				"author_id":       "100",
				"author_username": "golang",
				"author_name":     "Go",
				"created_at":      "2026-05-01T12:00:00.000Z",
			},
		}},
	}
	if diff := cmp.Diff(want, pack, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("LookupPost() mismatch (-want +got):\n%s", diff)
	}
	assertXContentPackJSONWarningsAreArrays(t, pack)
}

func TestLookupPostUsesPermalinkFallbackWhenAuthorUsernameMissing(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		assertXBearerRequest(t, r, "/2/tweets/20")
		writeXJSON(t, w, `{
			"data": {"id":"20", "text":"Single lookup", "author_id":"100"},
			"includes": {"users": [{"id":"100", "name":"Go"}]}
		}`)
	}))
	defer server.Close()

	pack, err := newXTestClient(server).LookupPost(context.Background(), LookupPostOptions{ID: "20"})
	if err != nil {
		t.Fatalf("LookupPost(username missing) error = %v, want nil", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("LookupPost(username missing) requests = %d, want 1", got)
	}

	want := content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{{
			Kind:  content.KindSocialPost,
			URL:   "https://x.com/i/web/status/20",
			Title: "Go",
			Text:  "Single lookup",
			Metadata: map[string]any{
				"post_id":     "20",
				"author_id":   "100",
				"author_name": "Go",
			},
		}},
	}
	if diff := cmp.Diff(want, pack, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("LookupPost(username missing) mismatch (-want +got):\n%s", diff)
	}
}

func TestUserPostsBoundsResultsByLimitAndDoesNotPageByDefault(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := atomic.AddInt32(&requests, 1); got > 1 {
			t.Fatalf("UserPosts() made hidden request %d to %s", got, r.URL.String())
		}
		assertXBearerRequest(t, r, "/2/users/100/tweets")
		assertQueryValue(t, r.URL.Query().Get("max_results"), "2", "max_results")
		writeXJSON(t, w, `{
			"data": [
				{"id":"20", "text":"first", "author_id":"100", "created_at":"2026-05-01T12:00:00.000Z"},
				{"id":"21", "text":"second", "author_id":"100", "created_at":"2026-05-01T13:00:00.000Z"},
				{"id":"22", "text":"provider over-returned", "author_id":"100", "created_at":"2026-05-01T14:00:00.000Z"}
			],
			"includes": {"users": [{"id":"100", "username":"golang", "name":"Go"}]},
			"meta": {"result_count":3, "next_token":"must-not-fetch"}
		}`)
	}))
	defer server.Close()

	pack, err := newXTestClient(server).UserPosts(context.Background(), UserPostsOptions{
		UserID:     "100",
		MaxResults: 2,
	})
	if err != nil {
		t.Fatalf("UserPosts(limit 2) error = %v, want nil", err)
	}
	if got := len(pack.Items); got != 2 {
		t.Fatalf("UserPosts(limit 2) items = %d, want 2", got)
	}
	assertXContentPackJSONWarningsAreArrays(t, pack)
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("UserPosts(limit 2) requests = %d, want 1", got)
	}
}

func TestUserPostsExplicitMaxRequestsFollowsPaginationWithinBudget(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/users/100/tweets")
		paths = append(paths, r.URL.RawQuery)
		switch len(paths) {
		case 1:
			if got := r.URL.Query().Get("pagination_token"); got != "" {
				t.Fatalf("first page pagination_token = %q, want empty", got)
			}
			writeXJSON(t, w, `{
				"data": [{"id":"20", "text":"first", "author_id":"100"}],
				"includes": {"users": [{"id":"100", "username":"golang", "name":"Go"}]},
				"meta": {"result_count":1, "next_token":"page-2"}
			}`)
		case 2:
			assertQueryValue(t, r.URL.Query().Get("pagination_token"), "page-2", "pagination_token")
			writeXJSON(t, w, `{
				"data": [
					{"id":"21", "text":"second", "author_id":"100"},
					{"id":"22", "text":"third", "author_id":"100"}
				],
				"includes": {"users": [{"id":"100", "username":"golang", "name":"Go"}]},
				"meta": {"result_count":2, "next_token":"must-not-fetch"}
			}`)
		default:
			t.Fatalf("UserPosts(MaxRequests 2) made request %d, want at most 2", len(paths))
		}
	}))
	defer server.Close()

	pack, err := newXTestClient(server).UserPosts(context.Background(), UserPostsOptions{
		UserID:      "100",
		MaxResults:  3,
		MaxRequests: 2,
	})
	if err != nil {
		t.Fatalf("UserPosts(MaxRequests 2) error = %v, want nil", err)
	}
	if got := len(pack.Items); got != 3 {
		t.Fatalf("UserPosts(MaxRequests 2) items = %d, want 3", got)
	}
	if got := len(paths); got != 2 {
		t.Fatalf("UserPosts(MaxRequests 2) requests = %d, want 2", got)
	}
}

func TestUserPostsUsesExplicitNextToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/users/100/tweets")
		assertQueryValue(t, r.URL.Query().Get("pagination_token"), "older-page", "pagination_token")
		writeXJSON(t, w, `{"data":[], "meta":{"result_count":0}}`)
	}))
	defer server.Close()

	_, err := newXTestClient(server).UserPosts(context.Background(), UserPostsOptions{
		UserID:    "100",
		NextToken: "older-page",
	})
	if err != nil {
		t.Fatalf("UserPosts(NextToken) error = %v, want nil", err)
	}
}

func TestMissingBearerTokenReturnsNotConfiguredBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("SearchRecent() made request without bearer token to %s", r.URL.String())
	}))
	defer server.Close()

	_, err := NewClient(Config{BaseURL: server.URL}).SearchRecent(context.Background(), SearchOptions{Query: "go"})
	assertErrorContains(t, err, "not configured")
}

func TestAccessDeniedTierErrorIsClear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/tweets/search/recent")
		w.WriteHeader(http.StatusForbidden)
		writeXJSON(t, w, `{"title":"Forbidden", "detail":"client-not-enrolled for recent search"}`)
	}))
	defer server.Close()

	_, err := newXTestClient(server).SearchRecent(context.Background(), SearchOptions{Query: "tier gated"})
	assertErrorContains(t, err, "access denied")
	assertErrorContains(t, err, "client-not-enrolled")
}

func TestTooManyRequestsUsesRetryingHTTPClient(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/tweets/search/recent")
		if atomic.AddInt32(&requests, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"title":"Too Many Requests"}`)
			return
		}
		writeXJSON(t, w, `{"data":[], "meta":{"result_count":0}}`)
	}))
	defer server.Close()

	client := NewClient(Config{
		Credentials: config.Credentials{XBearerToken: xTestBearerToken},
		BaseURL:     server.URL,
		HTTPClient: httpclient.New(httpclient.Options{
			HTTPClient: &http.Client{Timeout: time.Second},
			Attempts:   2,
			RetryBase:  time.Nanosecond,
			RetryMax:   time.Nanosecond,
			Jitter:     httpclient.NoJitter,
			Sleeper:    noWaitSleeper{},
		}),
	})

	_, err := client.SearchRecent(context.Background(), SearchOptions{Query: "retry"})
	if err != nil {
		t.Fatalf("SearchRecent(429 then ok) error = %v, want nil", err)
	}
	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("SearchRecent(429 then ok) requests = %d, want 2", got)
	}
}

func TestMalformedResponseReturnsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/tweets/search/recent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":`)
	}))
	defer server.Close()

	_, err := newXTestClient(server).SearchRecent(context.Background(), SearchOptions{Query: "bad json"})
	assertErrorContains(t, err, "decode")
}

func TestTransportErrorRedactsBearerTokenAndUnwrapsCause(t *testing.T) {
	bearerToken := "secret/token value+"
	cause := errors.New("dial failed with " + bearerToken + " and " + url.QueryEscape(bearerToken))
	client := NewClient(Config{
		Credentials: config.Credentials{XBearerToken: bearerToken},
		BaseURL:     "https://api.example.test",
		HTTPClient: httpclient.New(httpclient.Options{
			HTTPClient: &http.Client{Transport: failingRoundTripper{err: cause}},
			Attempts:   1,
		}),
	})

	_, err := client.SearchRecent(context.Background(), SearchOptions{Query: "go"})
	assertErrorContains(t, err, "x recent search request")
	assertErrorContains(t, err, "[redacted]")
	assertErrorDoesNotContain(t, err, bearerToken)
	assertErrorDoesNotContain(t, err, url.QueryEscape(bearerToken))
	if !errors.Is(err, cause) {
		t.Fatalf("SearchRecent(transport error) error = %v, want to unwrap %v", err, cause)
	}
}

func TestStatusErrorRedactsBearerTokenFromProviderBody(t *testing.T) {
	bearerToken := "secret/token value+"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+bearerToken {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		w.WriteHeader(http.StatusForbidden)
		writeXJSON(t, w, `{"detail":"`+bearerToken+` `+url.QueryEscape(bearerToken)+`"}`)
	}))
	defer server.Close()

	_, err := NewClient(Config{
		Credentials: config.Credentials{XBearerToken: bearerToken},
		BaseURL:     server.URL,
	}).SearchRecent(context.Background(), SearchOptions{Query: "tier gated"})
	assertErrorContains(t, err, "access denied")
	assertErrorContains(t, err, "[redacted]")
	assertErrorDoesNotContain(t, err, bearerToken)
	assertErrorDoesNotContain(t, err, url.QueryEscape(bearerToken))
}

func newXTestClient(server *httptest.Server) *Client {
	return NewClient(Config{
		Credentials: config.Credentials{XBearerToken: xTestBearerToken},
		BaseURL:     server.URL,
	})
}

func assertXBearerRequest(t *testing.T, r *http.Request, wantPath string) {
	t.Helper()

	if r.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != wantPath {
		t.Fatalf("path = %q, want %q", r.URL.Path, wantPath)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer "+xTestBearerToken {
		t.Fatalf("Authorization = %q, want bearer token", got)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}
}

func assertQueryValue(t *testing.T, got string, want string, name string) {
	t.Helper()

	if got != want {
		t.Fatalf("query %s = %q, want %q", name, got, want)
	}
}

func assertQueryContains(t *testing.T, got string, want string, name string) {
	t.Helper()

	for part := range strings.SplitSeq(got, ",") {
		if strings.TrimSpace(part) == want {
			return
		}
	}
	t.Fatalf("query %s = %q, want to contain %q", name, got, want)
}

func writeXJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func assertXContentPackJSONWarningsAreArrays(t *testing.T, pack content.Pack) {
	t.Helper()

	encoded, err := json.Marshal(pack)
	if err != nil {
		t.Fatalf("marshal content pack: %v", err)
	}
	var document struct {
		Warnings json.RawMessage `json:"warnings"`
		Items    []struct {
			Warnings json.RawMessage `json:"warnings"`
		} `json:"items"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode content pack JSON: %v", err)
	}
	assertXRawJSON(t, document.Warnings, []byte(`[]`), "pack warnings")
	for _, item := range document.Items {
		assertXRawJSON(t, item.Warnings, []byte(`[]`), "item warnings")
	}
}

func assertXRawJSON(t *testing.T, got json.RawMessage, want []byte, label string) {
	t.Helper()

	if !bytes.Equal(got, want) {
		t.Fatalf("%s JSON = %s, want %s", label, got, want)
	}
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func assertErrorDoesNotContain(t *testing.T, err error, forbidden string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want no substring %q", forbidden)
	}
	if strings.Contains(err.Error(), forbidden) {
		t.Fatalf("error = %v, want no substring %q", err, forbidden)
	}
}

type failingRoundTripper struct {
	err error
}

func (transport failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, transport.err
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
