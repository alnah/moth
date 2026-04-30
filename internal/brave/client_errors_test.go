package brave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/httpclient"
)

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

	client := newBraveTestClient(t, server)

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
	assertErrorDoesNotContain(t, err, braveTestAPIKey)
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
		writeJSONResponse(t, w, `{"results":[`)
	}))
	defer server.Close()

	client := newBraveTestClient(t, server)

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

func TestSearchReturnsContextualRequestBuildError(t *testing.T) {
	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  "://bad-base-url",
	})

	_, err := client.SearchVideos(context.Background(), SearchOptions{Query: "bad base url"})
	if err == nil {
		t.Fatal("SearchVideos request build error = nil, want error")
	}
	assertErrorContains(t, err, "brave")
	assertErrorContains(t, err, "videos")
	assertErrorContains(t, err, "build request")
	assertErrorDoesNotContain(t, err, braveTestAPIKey)
}

func TestSearchReturnsContextualTransportError(t *testing.T) {
	client := NewClient(Config{
		Settings: config.Settings{BraveAPIKey: braveTestAPIKey},
		BaseURL:  "https://brave.test",
		HTTPClient: httpclient.New(httpclient.Options{
			HTTPClient: &http.Client{Transport: failingRoundTripper{}},
			Attempts:   1,
		}),
	})

	_, err := client.SearchWeb(context.Background(), SearchOptions{Query: "network failure"})
	if err == nil {
		t.Fatal("SearchWeb transport error = nil, want error")
	}
	assertErrorContains(t, err, "brave")
	assertErrorContains(t, err, "web")
	assertErrorContains(t, err, "request")
	assertErrorContains(t, err, "network closed")
	assertErrorDoesNotContain(t, err, braveTestAPIKey)
}

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network closed")
}

var _ http.RoundTripper = failingRoundTripper{}
