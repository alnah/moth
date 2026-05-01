package podcast

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/httpdownload"
)

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

func TestAudioDownloaderRejectsFeedFailures(t *testing.T) {
	t.Run("feed status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "feed unavailable", http.StatusBadGateway)
		}))
		defer server.Close()

		_, err := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()}).DownloadEpisodeAudio(
			context.Background(),
			AudioDownloadOptions{FeedURL: server.URL + "/feed.xml", EpisodeGUID: "episode-17"},
		)
		assertPodcastErrorContains(t, err, "502")
	})

	t.Run("malformed RSS", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writePodcastIndexXML(t, w, `<rss><channel><item>`)
		}))
		defer server.Close()

		_, err := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()}).DownloadEpisodeAudio(
			context.Background(),
			AudioDownloadOptions{FeedURL: server.URL + "/feed.xml", EpisodeGUID: "episode-17"},
		)
		assertPodcastErrorContains(t, err, "malformed_feed")
	})

	t.Run("episode not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writePodcastIndexXML(t, w, podcastFeedXML("other-episode", serverURLFromRequest(r)+"/audio/other.mp3", "1"))
		}))
		defer server.Close()

		_, err := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()}).DownloadEpisodeAudio(
			context.Background(),
			AudioDownloadOptions{FeedURL: server.URL + "/feed.xml", EpisodeGUID: "episode-17"},
		)
		assertPodcastErrorContains(t, err, "not found")
	})

	t.Run("episode has no allowed enclosure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writePodcastIndexXML(t, w, podcastFeedXMLWithType(
				"episode-17",
				serverURLFromRequest(r)+"/audio/episode-17.txt",
				"text/plain; charset=utf-8",
				"1",
			))
		}))
		defer server.Close()

		_, err := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()}).DownloadEpisodeAudio(
			context.Background(),
			AudioDownloadOptions{
				FeedURL:             server.URL + "/feed.xml",
				EpisodeGUID:         "episode-17",
				AllowedContentTypes: []string{"audio/mpeg"},
			},
		)
		assertPodcastErrorContains(t, err, "no allowed")
	})
}

func TestAudioDownloaderRejectsAudioResponseFailures(t *testing.T) {
	t.Run("status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/feed.xml":
				writePodcastIndexXML(t, w, podcastFeedXML("episode-17", serverURLFromRequest(r)+"/audio/episode-17.mp3", "1"))
			case "/audio/episode-17.mp3":
				http.Error(w, "missing", http.StatusNotFound)
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		_, err := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()}).DownloadEpisodeAudio(
			context.Background(),
			AudioDownloadOptions{
				FeedURL:             server.URL + "/feed.xml",
				EpisodeGUID:         "episode-17",
				AllowedContentTypes: []string{"audio/mpeg"},
			},
		)
		assertPodcastErrorContains(t, err, "404")
	})

	t.Run("content type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/feed.xml":
				writePodcastIndexXML(t, w, podcastFeedXMLWithType(
					"episode-17",
					serverURLFromRequest(r)+"/audio/episode-17.bin",
					"audio/mpeg",
					"1",
				))
			case "/audio/episode-17.bin":
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = io.WriteString(w, "x")
			default:
				http.NotFound(w, r)
			}
		}))
		defer server.Close()

		_, err := NewAudioDownloader(AudioDownloaderConfig{HTTPClient: server.Client()}).DownloadEpisodeAudio(
			context.Background(),
			AudioDownloadOptions{
				FeedURL:             server.URL + "/feed.xml",
				EpisodeGUID:         "episode-17",
				AllowedContentTypes: []string{"audio/mpeg"},
			},
		)
		assertPodcastErrorContains(t, err, "content type")
	})
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
	return podcastFeedXMLWithType(guid, enclosureURL, "audio/mpeg", length)
}

func podcastFeedXMLWithType(guid string, enclosureURL string, contentType string, length string) string {
	return strings.Join([]string{
		`<rss version="2.0"><channel><title>Systems Show</title><item>`,
		`<title>Episode 17</title><guid>`, guid, `</guid>`,
		`<enclosure url="`, enclosureURL, `" type="`, contentType, `" length="`, length, `" />`,
		`</item></channel></rss>`,
	}, "")
}

func serverURLFromRequest(r *http.Request) string {
	return "http://" + r.Host
}
