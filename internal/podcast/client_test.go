package podcast

import (
	"context"
	"crypto/sha1" //nolint:gosec // Podcast Index contract requires SHA1 signatures.
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
	"github.com/alnah/moth/internal/httpdownload"
)

const (
	podcastTestAPIKey    = "podcast-test-key"    //nolint:gosec // Fake credential for httptest only.
	podcastTestAPISecret = "podcast-test-secret" //nolint:gosec // Fake credential for httptest only.
	podcastTestUserAgent = "moth-test/1.0"
)

var podcastTestNow = time.Unix(1_715_000_000, 0).UTC()

func TestSearchSendsPodcastIndexAuthHeadersAndMapsPodcasts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertPodcastIndexAuthRequest(t, r, "/search/byterm")
		assertPodcastIndexQuery(t, r, map[string]string{
			"q":        "distributed systems",
			"max":      "2",
			"clean":    "1",
			"fulltext": "1",
		})
		writePodcastIndexJSON(t, w, `{
			"status":"true",
			"count":2,
			"feeds":[
				{
					"id":101,
					"title":"Systems Show",
					"description":"Distributed systems interviews.",
					"url":"https://podcasts.example/systems/feed.xml",
					"link":"https://podcasts.example/systems",
					"author":"Alex Producer",
					"image":"https://podcasts.example/systems.jpg",
					"episodeCount":42,
					"categories":{"102":"Technology"}
				},
				{"id":0,"title":"missing feed id must be ignored"}
			]
		}`)
	}))
	defer server.Close()

	client := newPodcastIndexTestClient(server.URL)

	pack, err := client.Search(context.Background(), SearchOptions{
		Query:      "distributed systems",
		MaxResults: 2,
		Clean:      true,
		FullText:   true,
	})
	if err != nil {
		t.Fatalf("Search error = %v, want nil", err)
	}

	assertPodcastContentPack(t, pack, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindPodcast,
				URL:   "https://podcasts.example/systems/feed.xml",
				Title: "Systems Show",
				Text:  "Distributed systems interviews.",
				Metadata: map[string]any{
					"feed_id":       101,
					"site_url":      "https://podcasts.example/systems",
					"author":        "Alex Producer",
					"image_url":     "https://podcasts.example/systems.jpg",
					"episode_count": 42,
					"categories":    []string{"Technology"},
				},
			},
		},
		Metadata: map[string]any{"total_results": 2},
	})
}

func TestEpisodesByFeedIDSendsDocumentedRequestAndMapsEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertPodcastIndexAuthRequest(t, r, "/episodes/byfeedid")
		assertPodcastIndexQuery(t, r, map[string]string{
			"id":       "101",
			"max":      "3",
			"since":    "1714521600",
			"fulltext": "1",
		})
		writePodcastIndexJSON(t, w, `{
			"status":"true",
			"count":1,
			"items":[{
				"id":9001,
				"feedId":101,
				"title":"Episode 17",
				"description":"Chunking audio safely.",
				"link":"https://podcasts.example/systems/17",
				"guid":"episode-17",
				"datePublished":1714564800,
				"duration":3661,
				"enclosureUrl":"https://cdn.example/episode-17.mp3",
				"enclosureType":"audio/mpeg",
				"enclosureLength":123456
			}]
		}`)
	}))
	defer server.Close()

	client := newPodcastIndexTestClient(server.URL)

	pack, err := client.EpisodesByFeedID(context.Background(), EpisodesByFeedIDOptions{
		FeedID:     101,
		MaxResults: 3,
		Since:      time.Unix(1_714_521_600, 0).UTC(),
		FullText:   true,
	})
	if err != nil {
		t.Fatalf("EpisodesByFeedID error = %v, want nil", err)
	}

	assertPodcastContentPack(t, pack, content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{
			{
				Kind:  content.KindAudio,
				URL:   "https://cdn.example/episode-17.mp3",
				Title: "Episode 17",
				Text:  "Chunking audio safely.",
				Metadata: map[string]any{
					"episode_id":        9001,
					"feed_id":           101,
					"episode_url":       "https://podcasts.example/systems/17",
					"guid":              "episode-17",
					"published_at_unix": 1714564800,
					"duration_seconds":  3661,
					"enclosure_type":    "audio/mpeg",
					"enclosure_length":  123456,
				},
			},
		},
		Metadata: map[string]any{"total_results": 1},
	})
}

func TestSearchReturnsProviderErrorForBadSignatureWithoutLeakingSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == podcastIndexSignature("different-secret") {
			t.Fatal("Authorization unexpectedly matched server secret")
		}
		http.Error(w, `{"status":"false","description":"invalid signature"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newPodcastIndexTestClient(server.URL)

	_, err := client.Search(context.Background(), SearchOptions{Query: "auth failure", MaxResults: 1})
	if err == nil {
		t.Fatal("Search bad signature error = nil, want provider error")
	}
	assertPodcastErrorContains(t, err, "podcast")
	assertPodcastErrorContains(t, err, "401")
	assertPodcastErrorContains(t, err, "invalid signature")
	assertPodcastErrorDoesNotContain(t, err, podcastTestAPIKey)
	assertPodcastErrorDoesNotContain(t, err, podcastTestAPISecret)
}

func TestSearchReturnsDecodeErrorForMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertPodcastIndexAuthRequest(t, r, "/search/byterm")
		writePodcastIndexJSON(t, w, `{"feeds":[`)
	}))
	defer server.Close()

	client := newPodcastIndexTestClient(server.URL)

	_, err := client.Search(context.Background(), SearchOptions{Query: "bad-json"})
	if err == nil {
		t.Fatal("Search malformed JSON error = nil, want decode error")
	}
	assertPodcastErrorContains(t, err, "podcast")
	assertPodcastErrorContains(t, err, "decode")
}

func TestSearchFailsBeforeRequestWhenCredentialsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server received request, want missing credentials to fail before HTTP")
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL:   server.URL,
		UserAgent: podcastTestUserAgent,
		Now:       func() time.Time { return podcastTestNow },
	})

	_, err := client.Search(context.Background(), SearchOptions{Query: "go"})
	if err == nil {
		t.Fatal("Search missing credentials error = nil, want error")
	}
	assertPodcastErrorContains(t, err, "api key")
}

func TestAudioDownloaderDownloadsRSSEnclosureWithLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			writePodcastIndexXML(t, w, podcastFeedXML(
				"episode-17",
				serverURLFromRequest(r)+"/audio/episode-17.mp3",
				"13",
			))
		case "/audio/episode-17.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Content-Length", "13")
			_, _ = io.WriteString(w, "fake mp3 data")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	downloader := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()})

	file, err := downloader.DownloadEpisodeAudio(context.Background(), AudioDownloadOptions{
		FeedURL:             server.URL + "/feed.xml",
		EpisodeGUID:         "episode-17",
		AllowedContentTypes: []string{"audio/mpeg", "audio/mp4", "audio/ogg"},
		MaxBytes:            13,
	})
	if err != nil {
		t.Fatalf("DownloadEpisodeAudio error = %v, want nil", err)
	}
	if file.URL != server.URL+"/audio/episode-17.mp3" {
		t.Fatalf("audio URL = %q, want enclosure URL", file.URL)
	}
	if file.ContentType != "audio/mpeg" {
		t.Fatalf("content type = %q, want audio/mpeg", file.ContentType)
	}
	if string(file.Bytes) != "fake mp3 data" {
		t.Fatalf("audio bytes = %q, want downloaded enclosure", file.Bytes)
	}
}

func TestAudioDownloaderRejectsOversizedEnclosureBeforeReadingBody(t *testing.T) {
	bodyRequested := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			writePodcastIndexXML(t, w, podcastFeedXML(
				"episode-oversize",
				serverURLFromRequest(r)+"/audio/oversize.mp3",
				"13",
			))
		case "/audio/oversize.mp3":
			bodyRequested = true
			w.Header().Set("Content-Type", "audio/mpeg")
			w.Header().Set("Content-Length", "13")
			_, _ = io.WriteString(w, "too many bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	downloader := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()})

	_, err := downloader.DownloadEpisodeAudio(context.Background(), AudioDownloadOptions{
		FeedURL:             server.URL + "/feed.xml",
		EpisodeGUID:         "episode-oversize",
		AllowedContentTypes: []string{"audio/mpeg"},
		MaxBytes:            12,
	})
	if err == nil {
		t.Fatal("DownloadEpisodeAudio oversized enclosure error = nil, want error")
	}
	if !errors.Is(err, httpdownload.ErrFileTooLarge) {
		t.Fatalf("DownloadEpisodeAudio error = %v, want file_too_large", err)
	}
	if bodyRequested {
		t.Fatal("audio body requested, want enclosure length rejected before body download")
	}
}

func TestAudioDownloaderRejectsUnknownLengthBodyOverMaxBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			writePodcastIndexXML(t, w, podcastFeedXML(
				"episode-stream",
				serverURLFromRequest(r)+"/audio/stream.mp3",
				"0",
			))
		case "/audio/stream.mp3":
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = io.WriteString(w, "too many bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	downloader := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()})

	_, err := downloader.DownloadEpisodeAudio(context.Background(), AudioDownloadOptions{
		FeedURL:             server.URL + "/feed.xml",
		EpisodeGUID:         "episode-stream",
		AllowedContentTypes: []string{"audio/mpeg"},
		MaxBytes:            3,
	})
	if !errors.Is(err, httpdownload.ErrFileTooLarge) {
		t.Fatalf("DownloadEpisodeAudio error = %v, want file_too_large", err)
	}
}

func newPodcastIndexTestClient(baseURL string) *Client {
	return NewClient(Config{
		Settings: config.Settings{
			PodcastIndexAPIKey:    podcastTestAPIKey,
			PodcastIndexAPISecret: podcastTestAPISecret,
		},
		BaseURL:    baseURL,
		HTTPClient: httpclient.New(httpclient.Options{Attempts: 1}),
		UserAgent:  podcastTestUserAgent,
		Now:        func() time.Time { return podcastTestNow },
	})
}

func assertPodcastIndexAuthRequest(t *testing.T, r *http.Request, wantPath string) {
	t.Helper()

	if r.Method != http.MethodGet {
		t.Fatalf("method = %s, want GET", r.Method)
	}
	if r.URL.Path != wantPath {
		t.Fatalf("path = %q, want %s", r.URL.Path, wantPath)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}
	if got := r.Header.Get("User-Agent"); got != podcastTestUserAgent {
		t.Fatalf("User-Agent = %q, want %q", got, podcastTestUserAgent)
	}
	if got := r.Header.Get("X-Auth-Key"); got != podcastTestAPIKey {
		t.Fatalf("X-Auth-Key = %q, want configured key", got)
	}
	if got := r.Header.Get("X-Auth-Date"); got != strconv.FormatInt(podcastTestNow.Unix(), 10) {
		t.Fatalf("X-Auth-Date = %q, want fake clock unix seconds", got)
	}
	if got := r.Header.Get("Authorization"); got != podcastIndexSignature(podcastTestAPISecret) {
		t.Fatalf("Authorization = %q, want deterministic sha1 signature", got)
	}
}

func podcastIndexSignature(secret string) string {
	signed := podcastTestAPIKey + secret + strconv.FormatInt(podcastTestNow.Unix(), 10)
	sum := sha1.Sum([]byte(signed)) //nolint:gosec // Podcast Index contract requires SHA1 signatures.

	return hex.EncodeToString(sum[:])
}

func assertPodcastIndexQuery(t *testing.T, r *http.Request, wants map[string]string) {
	t.Helper()

	query := r.URL.Query()
	for name, want := range wants {
		if got := query.Get(name); got != want {
			t.Fatalf("query %s = %q, want %q", name, got, want)
		}
	}
}

func writePodcastIndexJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write JSON response: %v", err)
	}
}

func writePodcastIndexXML(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/rss+xml")
	//nolint:gosec // Test writes controlled XML fixtures to a local httptest response.
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write XML response: %v", err)
	}
}

func podcastFeedXML(guid string, enclosureURL string, length string) string {
	return strings.Join([]string{
		`<rss version="2.0"><channel><title>Systems Show</title><item>`,
		`<title>Episode 17</title><guid>`, guid, `</guid>`,
		`<enclosure url="`, enclosureURL, `" type="audio/mpeg" length="`, length, `" />`,
		`</item></channel></rss>`,
	}, "")
}

func serverURLFromRequest(r *http.Request) string {
	return "http://" + r.Host
}

func assertPodcastContentPack(t *testing.T, got content.Pack, want content.Pack) {
	t.Helper()

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("content pack mismatch (-want +got):\n%s", diff)
	}
}

func assertPodcastErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func assertPodcastErrorDoesNotContain(t *testing.T, err error, unwanted string) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want non-nil error")
	}
	if strings.Contains(err.Error(), unwanted) {
		t.Fatalf("error = %v, want no substring %q", err, unwanted)
	}
}
