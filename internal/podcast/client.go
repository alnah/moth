// Package podcast discovers podcasts through Podcast Index and RSS feeds.
package podcast

import (
	"context"
	"crypto/sha1" //nolint:gosec // Podcast Index documents SHA1 for request authentication.
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
	"github.com/alnah/moth/internal/httpdownload"
	"github.com/alnah/moth/internal/rssfeed"
)

const (
	defaultPodcastIndexBaseURL   = "https://api.podcastindex.org/api/1.0"
	defaultPodcastIndexUserAgent = "moth/1.0"
	podcastResponseBodyMax       = 4096
	defaultFeedDownloadMaxBytes  = 10 * 1024 * 1024
)

// Config contains Podcast Index client dependencies and credentials.
type Config struct {
	Settings   config.Settings
	BaseURL    string
	HTTPClient *httpclient.Client
	UserAgent  string
	Now        func() time.Time
}

// SearchOptions contains Podcast Index /search/byterm query parameters.
type SearchOptions struct {
	Query      string
	MaxResults int
	Clean      bool
	FullText   bool
}

// EpisodesByFeedIDOptions contains Podcast Index /episodes/byfeedid query parameters.
type EpisodesByFeedIDOptions struct {
	FeedID     int
	MaxResults int
	Since      time.Time
	FullText   bool
}

// Client sends raw HTTP requests to Podcast Index.
type Client struct {
	settings   config.Settings
	baseURL    string
	httpClient *httpclient.Client
	userAgent  string
	now        func() time.Time
}

// NewClient creates a Podcast Index client with defaults for unset dependencies.
func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultPodcastIndexBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = httpclient.New(httpclient.Options{})
	}
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = defaultPodcastIndexUserAgent
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	return &Client{
		settings:   cfg.Settings,
		baseURL:    baseURL,
		httpClient: httpClient,
		userAgent:  userAgent,
		now:        now,
	}
}

// Search returns normalized podcast results from Podcast Index.
func (client *Client) Search(ctx context.Context, options SearchOptions) (content.Pack, error) {
	query := url.Values{}
	query.Set("q", options.Query)
	if options.MaxResults > 0 {
		query.Set("max", strconv.Itoa(options.MaxResults))
	}
	if options.Clean {
		query.Set("clean", "1")
	}
	if options.FullText {
		query.Set("fulltext", "1")
	}

	var response podcastSearchResponse
	if err := client.get(ctx, "/search/byterm", query, &response); err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapPodcastFeeds(response.Feeds),
		Metadata: map[string]any{"total_results": response.Count},
	}, nil
}

// EpisodesByFeedID returns normalized episode results for one Podcast Index feed.
func (client *Client) EpisodesByFeedID(ctx context.Context, options EpisodesByFeedIDOptions) (content.Pack, error) {
	query := url.Values{}
	query.Set("id", strconv.Itoa(options.FeedID))
	if options.MaxResults > 0 {
		query.Set("max", strconv.Itoa(options.MaxResults))
	}
	if !options.Since.IsZero() {
		query.Set("since", strconv.FormatInt(options.Since.Unix(), 10))
	}
	if options.FullText {
		query.Set("fulltext", "1")
	}

	var response podcastEpisodesResponse
	if err := client.get(ctx, "/episodes/byfeedid", query, &response); err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapPodcastEpisodes(response.Items),
		Metadata: map[string]any{"total_results": response.Count},
	}, nil
}

func (client *Client) get(ctx context.Context, path string, query url.Values, target any) error {
	apiKey := strings.TrimSpace(client.settings.PodcastIndexAPIKey)
	apiSecret := strings.TrimSpace(client.settings.PodcastIndexAPISecret)
	if apiKey == "" {
		return fmt.Errorf("podcast index: api key is required")
	}
	if apiSecret == "" {
		return fmt.Errorf("podcast index: api secret is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("podcast index: build request: %w", err)
	}
	client.sign(req, apiKey, apiSecret)

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("podcast index request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return client.statusError(resp, apiKey, apiSecret)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("podcast index decode response: %w", err)
	}

	return nil
}

func (client *Client) sign(req *http.Request, apiKey string, apiSecret string) {
	now := strconv.FormatInt(client.now().Unix(), 10)
	sum := sha1.Sum([]byte(apiKey + apiSecret + now)) //nolint:gosec // Podcast Index requires SHA1 auth.

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", client.userAgent)
	req.Header.Set("X-Auth-Key", apiKey)
	req.Header.Set("X-Auth-Date", now)
	req.Header.Set("Authorization", hex.EncodeToString(sum[:]))
}

func (client *Client) statusError(resp *http.Response, secrets ...string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, podcastResponseBodyMax))
	responseText := scrubSecrets(strings.TrimSpace(string(body)), secrets...)

	return fmt.Errorf("podcast index failed: status %d: %s", resp.StatusCode, responseText)
}

type podcastSearchResponse struct {
	Count int           `json:"count"`
	Feeds []podcastFeed `json:"feeds"`
}

type podcastFeed struct {
	ID           int               `json:"id"`
	Title        string            `json:"title"`
	Description  string            `json:"description"`
	URL          string            `json:"url"`
	Link         string            `json:"link"`
	Author       string            `json:"author"`
	Image        string            `json:"image"`
	EpisodeCount int               `json:"episodeCount"`
	Categories   map[string]string `json:"categories"`
}

type podcastEpisodesResponse struct {
	Count int              `json:"count"`
	Items []podcastEpisode `json:"items"`
}

type podcastEpisode struct {
	ID              int    `json:"id"`
	FeedID          int    `json:"feedId"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	Link            string `json:"link"`
	GUID            string `json:"guid"`
	DatePublished   int    `json:"datePublished"`
	Duration        int    `json:"duration"`
	EnclosureURL    string `json:"enclosureUrl"`
	EnclosureType   string `json:"enclosureType"`
	EnclosureLength int    `json:"enclosureLength"`
}

func mapPodcastFeeds(feeds []podcastFeed) []content.Item {
	items := make([]content.Item, 0, len(feeds))
	for _, feed := range feeds {
		if feed.ID == 0 {
			continue
		}
		items = append(items, content.Item{
			Kind:  content.KindPodcast,
			URL:   feed.URL,
			Title: feed.Title,
			Text:  feed.Description,
			Metadata: map[string]any{
				"feed_id":       feed.ID,
				"site_url":      feed.Link,
				"author":        feed.Author,
				"image_url":     feed.Image,
				"episode_count": feed.EpisodeCount,
				"categories":    sortedCategoryNames(feed.Categories),
			},
		})
	}

	return items
}

func mapPodcastEpisodes(episodes []podcastEpisode) []content.Item {
	items := make([]content.Item, 0, len(episodes))
	for _, episode := range episodes {
		items = append(items, content.Item{
			Kind:  content.KindAudio,
			URL:   episode.EnclosureURL,
			Title: episode.Title,
			Text:  episode.Description,
			Metadata: map[string]any{
				"episode_id":        episode.ID,
				"feed_id":           episode.FeedID,
				"episode_url":       episode.Link,
				"guid":              episode.GUID,
				"published_at_unix": episode.DatePublished,
				"duration_seconds":  episode.Duration,
				"enclosure_type":    episode.EnclosureType,
				"enclosure_length":  episode.EnclosureLength,
			},
		})
	}

	return items
}

func sortedCategoryNames(categories map[string]string) []string {
	keys := make([]string, 0, len(categories))
	for key := range categories {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, categories[key])
	}

	return names
}

func scrubSecrets(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}

	return text
}

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
