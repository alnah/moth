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
	"strconv"
	"strings"
	"time"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const (
	defaultPodcastIndexBaseURL   = "https://api.podcastindex.org/api/1.0"
	defaultPodcastIndexUserAgent = "moth/1.0"
	podcastResponseBodyMax       = 4096
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

func scrubSecrets(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		text = strings.ReplaceAll(text, secret, "[redacted]")
	}

	return text
}
