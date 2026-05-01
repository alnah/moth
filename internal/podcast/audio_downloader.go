package podcast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/alnah/moth/internal/httpdownload"
	"github.com/alnah/moth/internal/rssfeed"
)

const defaultFeedDownloadMaxBytes = 10 * 1024 * 1024

// AudioDownloaderConfig contains dependencies for podcast audio downloads.
type AudioDownloaderConfig struct {
	HTTPClient *http.Client
}

// AudioDownloadOptions identifies one RSS episode enclosure download.
type AudioDownloadOptions struct {
	FeedURL             string
	EpisodeGUID         string
	AllowedContentTypes []string
	MaxBytes            int64
}

// AudioFile is a downloaded podcast enclosure.
type AudioFile struct {
	URL         string
	ContentType string
	Bytes       []byte
}

// AudioDownloader resolves RSS enclosures and downloads bounded audio files.
type AudioDownloader struct {
	httpClient *http.Client
	parser     rssfeed.Parser
}

// NewAudioDownloader creates a podcast audio downloader.
func NewAudioDownloader(cfg AudioDownloaderConfig) *AudioDownloader {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &AudioDownloader{httpClient: httpClient, parser: rssfeed.NewParser()}
}

// DownloadEpisodeAudio downloads the matching RSS enclosure after checking declared size limits.
func (downloader *AudioDownloader) DownloadEpisodeAudio(
	ctx context.Context,
	options AudioDownloadOptions,
) (AudioFile, error) {
	feed, err := downloader.fetchFeed(ctx, options.FeedURL)
	if err != nil {
		return AudioFile{}, err
	}
	enclosure, err := findEpisodeEnclosure(feed, options)
	if err != nil {
		return AudioFile{}, err
	}
	if options.MaxBytes > 0 && enclosure.Length > options.MaxBytes {
		return AudioFile{}, fmt.Errorf(
			"podcast enclosure length %d over %d: %w",
			enclosure.Length,
			options.MaxBytes,
			httpdownload.ErrFileTooLarge,
		)
	}

	return downloader.downloadEnclosure(ctx, enclosure.URL, options)
}

func (downloader *AudioDownloader) fetchFeed(ctx context.Context, feedURL string) (rssfeed.Feed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return rssfeed.Feed{}, fmt.Errorf("podcast feed request: %w", err)
	}
	//nolint:gosec // Podcast feed downloads intentionally fetch caller-selected URLs with byte limits.
	resp, err := downloader.httpClient.Do(req)
	if err != nil {
		return rssfeed.Feed{}, fmt.Errorf("podcast feed download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return rssfeed.Feed{}, fmt.Errorf("podcast feed status %d", resp.StatusCode)
	}

	return downloader.parser.Parse(
		ctx,
		io.LimitReader(resp.Body, defaultFeedDownloadMaxBytes),
		rssfeed.ParseOptions{FeedURL: feedURL},
	)
}

func (downloader *AudioDownloader) downloadEnclosure(
	ctx context.Context,
	enclosureURL string,
	options AudioDownloadOptions,
) (AudioFile, error) {
	response, err := httpdownload.New(httpdownload.Options{HTTPClient: downloader.httpClient}).Download(
		ctx,
		httpdownload.Request{
			URL:                 enclosureURL,
			AllowedContentTypes: options.AllowedContentTypes,
			MaxBytes:            options.MaxBytes,
		},
	)
	if err != nil {
		return AudioFile{}, err
	}

	return AudioFile{
		URL:         response.URL,
		ContentType: response.ContentType,
		Bytes:       response.Bytes,
	}, nil
}

func findEpisodeEnclosure(feed rssfeed.Feed, options AudioDownloadOptions) (rssfeed.Enclosure, error) {
	for _, item := range feed.Items {
		if item.GUID != options.EpisodeGUID {
			continue
		}
		for _, enclosure := range item.Enclosures {
			if podcastContentTypeAllowed(enclosure.Type, options.AllowedContentTypes) {
				return enclosure, nil
			}
		}
		return rssfeed.Enclosure{}, fmt.Errorf("podcast episode %q has no allowed audio enclosure", options.EpisodeGUID)
	}

	return rssfeed.Enclosure{}, fmt.Errorf("podcast episode %q not found", options.EpisodeGUID)
}

func podcastContentTypeAllowed(contentType string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	for _, candidate := range allowed {
		if contentType == strings.ToLower(strings.TrimSpace(candidate)) {
			return true
		}
	}

	return false
}
