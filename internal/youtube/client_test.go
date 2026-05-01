package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const youtubeTestAPIKey = "youtube-test-key"

type youtubeSearchRequest struct {
	query             string
	maxResults        string
	regionCode        string
	relevanceLanguage string
	safeSearch        string
	pageToken         string
}

type youtubeVideoDurationFixture struct {
	id       string
	duration string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestSearchVideosSendsDocumentedRequestAndMapsVideoItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertYouTubeSearchRequest(t, r, youtubeSearchRequest{
			query:             "go conference",
			maxResults:        "5",
			regionCode:        "US",
			relevanceLanguage: "en",
			safeSearch:        "moderate",
			pageToken:         "next-page",
		})
		writeYouTubeJSON(t, w, `{
			"kind": "youtube#searchListResponse",
			"nextPageToken": "next-search-page",
			"pageInfo": {"totalResults": 2, "resultsPerPage": 5},
			"items": [
				{
					"kind": "youtube#searchResult",
					"id": {"kind": "youtube#video", "videoId": "dQw4w9WgXcQ"},
					"snippet": {
						"publishedAt": "2024-04-30T12:30:00Z",
						"channelId": "UC-123",
						"title": "GopherCon talk",
						"description": "A practical Go conference talk.",
						"thumbnails": {"medium": {"url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg"}},
						"channelTitle": "GopherCon"
					}
				},
				{
					"kind": "youtube#searchResult",
					"id": {"kind": "youtube#channel", "channelId": "UC-channel"},
					"snippet": {"title": "Channel result must not become a video"}
				}
			]
		}`)
	}))
	defer server.Close()

	client := newYouTubeTestClient(server.URL)

	result, err := client.SearchVideos(context.Background(), SearchOptions{
		Query:             "go conference",
		MaxResults:        5,
		RegionCode:        "US",
		RelevanceLanguage: "en",
		SafeSearch:        "moderate",
		PageToken:         "next-page",
	})
	if err != nil {
		t.Fatalf("SearchVideos error = %v, want nil", err)
	}

	assertYouTubeContentPack(t, result, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindVideo,
				URL:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
				Title: "GopherCon talk",
				Text:  "A practical Go conference talk.",
				Metadata: map[string]any{
					"video_id":      "dQw4w9WgXcQ",
					"channel_id":    "UC-123",
					"channel_title": "GopherCon",
					"published_at":  "2024-04-30T12:30:00Z",
					"thumbnail_url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/mqdefault.jpg",
				},
			},
		},
		Metadata: map[string]any{
			"quota_cost":      100,
			"next_page_token": "next-search-page",
			"total_results":   2,
		},
	})
}

func TestVideoDetailsSendsDocumentedRequestAndMapsDurationChannelDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertYouTubeVideoDetailsRequest(t, r, []string{"dQw4w9WgXcQ", "abcDEF12345"})
		writeYouTubeJSON(t, w, `{
			"kind": "youtube#videoListResponse",
			"items": [
				{
					"kind": "youtube#video",
					"id": "dQw4w9WgXcQ",
					"snippet": {
						"publishedAt": "2009-10-25T06:57:33Z",
						"channelId": "UCuAXFkgsw1L7xaCfnd5JJOw",
						"title": "Never Gonna Give You Up",
						"description": "Official music video.",
						"channelTitle": "Rick Astley",
						"thumbnails": {"default": {"url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/default.jpg"}}
					},
					"contentDetails": {"duration": "PT3M33S"},
					"statistics": {"viewCount": "1000000"}
				}
			]
		}`)
	}))
	defer server.Close()

	client := newYouTubeTestClient(server.URL)

	result, err := client.VideoDetails(context.Background(), VideoDetailsOptions{
		IDs: []string{"dQw4w9WgXcQ", "abcDEF12345"},
	})
	if err != nil {
		t.Fatalf("VideoDetails error = %v, want nil", err)
	}

	assertYouTubeContentPack(t, result, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindVideo,
				URL:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
				Title: "Never Gonna Give You Up",
				Text:  "Official music video.",
				Metadata: map[string]any{
					"video_id":      "dQw4w9WgXcQ",
					"channel_id":    "UCuAXFkgsw1L7xaCfnd5JJOw",
					"channel_title": "Rick Astley",
					"published_at":  "2009-10-25T06:57:33Z",
					"duration":      "PT3M33S",
					"view_count":    "1000000",
					"thumbnail_url": "https://i.ytimg.com/vi/dQw4w9WgXcQ/default.jpg",
				},
			},
		},
		Metadata: map[string]any{"quota_cost": 1},
	})
}

func TestVideoDetailsPreservesYouTubeISO8601DurationEdges(t *testing.T) {
	fixtures := []youtubeVideoDurationFixture{
		{id: "zeroDuration", duration: "PT0S"},
		{id: "secondsOnly", duration: "PT15S"},
		{id: "minutesOnly", duration: "PT2M"},
		{id: "fullClock", duration: "PT1H2M3S"},
		{id: "dayClock", duration: "P1DT2H3M4S"},
	}
	ids := youtubeVideoDurationFixtureIDs(fixtures)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertYouTubeVideoDetailsRequest(t, r, ids)
		writeYouTubeVideoDetailsJSON(t, w, fixtures)
	}))
	defer server.Close()

	client := newYouTubeTestClient(server.URL)

	result, err := client.VideoDetails(context.Background(), VideoDetailsOptions{IDs: ids})
	if err != nil {
		t.Fatalf("VideoDetails error = %v, want nil", err)
	}

	wantItems := make([]content.Item, 0, len(fixtures))
	for _, fixture := range fixtures {
		wantItems = append(wantItems, content.Item{
			Kind: content.KindVideo,
			URL:  "https://www.youtube.com/watch?v=" + fixture.id,
			Metadata: map[string]any{
				"video_id": fixture.id,
				"duration": fixture.duration,
			},
		})
	}
	assertYouTubeContentPack(t, result, content.Pack{
		Type:     content.TypeContentPack,
		Items:    wantItems,
		Metadata: map[string]any{"quota_cost": 1},
	})
}

func TestSearchVideosReturnsProviderErrorWithoutLeakingAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertYouTubeSearchRequest(t, r, youtubeSearchRequest{query: "quota", maxResults: "1"})
		errorBody := `{"error":{"code":403,"message":"quota exceeded","errors":[{"reason":"quotaExceeded"}]}}`
		http.Error(w, errorBody, http.StatusForbidden)
	}))
	defer server.Close()

	client := newYouTubeTestClient(server.URL)

	_, err := client.SearchVideos(context.Background(), SearchOptions{Query: "quota", MaxResults: 1})
	if err == nil {
		t.Fatal("SearchVideos provider error = nil, want error")
	}
	assertYouTubeErrorContains(t, err, "youtube")
	assertYouTubeErrorContains(t, err, "search")
	assertYouTubeErrorContains(t, err, "403")
	assertYouTubeErrorContains(t, err, "quotaExceeded")
	assertYouTubeErrorDoesNotContain(t, err, youtubeTestAPIKey)
}

func TestSearchVideosReturnsTransportErrorWithoutLeakingAPIKey(t *testing.T) {
	sentinel := errors.New("transport sentinel")
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("dial failed for %s: %w", r.URL.String(), sentinel)
	})
	client := NewClient(Config{
		Settings: config.Settings{YouTubeAPIKey: youtubeTestAPIKey},
		BaseURL:  "https://youtube.example.test",
		HTTPClient: httpclient.New(httpclient.Options{
			HTTPClient: &http.Client{Transport: transport},
			Attempts:   1,
		}),
	})

	_, err := client.SearchVideos(context.Background(), SearchOptions{Query: "transport", MaxResults: 1})
	if err == nil {
		t.Fatal("SearchVideos transport error = nil, want error")
	}
	assertYouTubeErrorContains(t, err, "youtube")
	assertYouTubeErrorContains(t, err, "search")
	assertYouTubeErrorContains(t, err, "request")
	assertYouTubeErrorContains(t, err, "[redacted]")
	assertYouTubeErrorDoesNotContain(t, err, youtubeTestAPIKey)
	assertYouTubeErrorDoesNotContain(t, err, url.QueryEscape(youtubeTestAPIKey))
	if !errors.Is(err, sentinel) {
		t.Fatalf("SearchVideos transport error = %v, want errors.Is transport sentinel", err)
	}
}

func TestSearchVideosReturnsDecodeErrorForMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertYouTubeSearchRequest(t, r, youtubeSearchRequest{query: "bad-json"})
		writeYouTubeJSON(t, w, `{"items":[`)
	}))
	defer server.Close()

	client := newYouTubeTestClient(server.URL)

	_, err := client.SearchVideos(context.Background(), SearchOptions{Query: "bad-json"})
	if err == nil {
		t.Fatal("SearchVideos malformed JSON error = nil, want decode error")
	}
	assertYouTubeErrorContains(t, err, "youtube")
	assertYouTubeErrorContains(t, err, "search")
	assertYouTubeErrorContains(t, err, "decode")
}

func TestSearchVideosFailsBeforeRequestWhenAPIKeyMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server received request, want missing API key to fail before HTTP")
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	_, err := client.SearchVideos(context.Background(), SearchOptions{Query: "go"})
	if err == nil {
		t.Fatal("SearchVideos missing API key error = nil, want error")
	}
	assertYouTubeErrorContains(t, err, "api key")
}

func TestVideoDetailsRejectsInvalidIDsBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server received request, want invalid video IDs to fail before HTTP")
	}))
	defer server.Close()

	client := newYouTubeTestClient(server.URL)

	_, err := client.VideoDetails(context.Background(), VideoDetailsOptions{IDs: []string{"dQw4w9WgXcQ", "bad id"}})
	if err == nil {
		t.Fatal("VideoDetails invalid ID error = nil, want error")
	}
	assertYouTubeErrorContains(t, err, "invalid")
	assertYouTubeErrorContains(t, err, "video id")
}

func newYouTubeTestClient(baseURL string) *Client {
	return NewClient(Config{
		Settings: config.Settings{YouTubeAPIKey: youtubeTestAPIKey},
		BaseURL:  baseURL,
	})
}

func assertYouTubeSearchRequest(t *testing.T, r *http.Request, want youtubeSearchRequest) {
	t.Helper()

	if r.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != "/search" {
		t.Fatalf("path = %q, want /search", r.URL.Path)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}

	query := r.URL.Query()
	assertYouTubeQueryParam(t, query.Get("key"), youtubeTestAPIKey, "key")
	assertYouTubeQueryParam(t, query.Get("part"), "snippet", "part")
	assertYouTubeQueryParam(t, query.Get("type"), "video", "type")
	assertYouTubeQueryParam(t, query.Get("q"), want.query, "q")
	assertYouTubeQueryParam(t, query.Get("maxResults"), want.maxResults, "maxResults")
	assertYouTubeQueryParam(t, query.Get("regionCode"), want.regionCode, "regionCode")
	assertYouTubeQueryParam(t, query.Get("relevanceLanguage"), want.relevanceLanguage, "relevanceLanguage")
	assertYouTubeQueryParam(t, query.Get("safeSearch"), want.safeSearch, "safeSearch")
	assertYouTubeQueryParam(t, query.Get("pageToken"), want.pageToken, "pageToken")
}

func assertYouTubeVideoDetailsRequest(t *testing.T, r *http.Request, wantIDs []string) {
	t.Helper()

	if r.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != "/videos" {
		t.Fatalf("path = %q, want /videos", r.URL.Path)
	}

	query := r.URL.Query()
	assertYouTubeQueryParam(t, query.Get("key"), youtubeTestAPIKey, "key")
	assertYouTubeQueryParam(t, query.Get("part"), "snippet,contentDetails,statistics", "part")
	assertYouTubeQueryParam(t, query.Get("id"), strings.Join(wantIDs, ","), "id")
}

func assertYouTubeQueryParam(t *testing.T, got string, want string, name string) {
	t.Helper()

	if got != want {
		t.Fatalf("query %s = %q, want %q", name, got, want)
	}
}

func writeYouTubeJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func writeYouTubeVideoDetailsJSON(t *testing.T, w http.ResponseWriter, fixtures []youtubeVideoDurationFixture) {
	t.Helper()

	items := make([]map[string]any, 0, len(fixtures))
	for _, fixture := range fixtures {
		items = append(items, map[string]any{
			"kind": "youtube#video",
			"id":   fixture.id,
			"contentDetails": map[string]any{
				"duration": fixture.duration,
			},
		})
	}
	writeYouTubeJSONValue(t, w, map[string]any{
		"kind":  "youtube#videoListResponse",
		"items": items,
	})
}

func writeYouTubeJSONValue(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func youtubeVideoDurationFixtureIDs(fixtures []youtubeVideoDurationFixture) []string {
	ids := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		ids = append(ids, fixture.id)
	}

	return ids
}

func assertYouTubeContentPack(t *testing.T, got content.Pack, want content.Pack) {
	t.Helper()

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("content pack mismatch (-want +got):\n%s", diff)
	}
}

func assertYouTubeErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func assertYouTubeErrorDoesNotContain(t *testing.T, err error, unwanted string) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want non-nil error")
	}
	if strings.Contains(err.Error(), unwanted) {
		t.Fatalf("error = %v, want no substring %q", err, unwanted)
	}
}
