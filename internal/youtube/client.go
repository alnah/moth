// Package youtube calls the YouTube Data API and maps videos to content items.
package youtube

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const (
	defaultBaseURL  = "https://www.googleapis.com/youtube/v3"
	responseBodyMax = 4096
)

var (
	validVideoID       = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	youtubeAPIKeyParam = regexp.MustCompile(`([?&]key=)[^&\s]+`)
)

// Config contains YouTube API dependencies and credentials.
type Config struct {
	Settings   config.Settings
	BaseURL    string
	HTTPClient *httpclient.Client
}

// SearchOptions contains YouTube search.list query parameters.
type SearchOptions struct {
	Query             string
	MaxResults        int
	RegionCode        string
	RelevanceLanguage string
	SafeSearch        string
	PageToken         string
}

// VideoDetailsOptions contains YouTube videos.list query parameters.
type VideoDetailsOptions struct {
	IDs []string
}

// Client sends raw HTTP requests to the YouTube Data API.
type Client struct {
	settings   config.Settings
	baseURL    string
	httpClient *httpclient.Client
}

type redactedYouTubeError struct {
	message string
	err     error
}

func (err redactedYouTubeError) Error() string {
	return err.message
}

func (err redactedYouTubeError) Unwrap() error {
	return err.err
}

// NewClient creates a YouTube client with defaults for unset dependencies.
func NewClient(cfg Config) *Client {
	client := cfg.HTTPClient
	if client == nil {
		client = httpclient.New(httpclient.Options{})
	}

	return &Client{
		settings:   cfg.Settings,
		baseURL:    cmp.Or(strings.TrimRight(cfg.BaseURL, "/"), defaultBaseURL),
		httpClient: client,
	}
}

// SearchVideos returns normalized video search results.
func (client *Client) SearchVideos(ctx context.Context, options SearchOptions) (content.Pack, error) {
	var response youtubeSearchResponse
	if err := client.get(ctx, "/search", searchQuery(options), "search", &response); err != nil {
		return content.Pack{}, err
	}

	metadata := map[string]any{"quota_cost": 100}
	addStringMetadata(metadata, "next_page_token", response.NextPageToken)
	addIntMetadata(metadata, "total_results", response.PageInfo.TotalResults)

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapSearchItems(response.Items),
		Metadata: metadata,
	}, nil
}

// VideoDetails returns normalized metadata for video IDs.
func (client *Client) VideoDetails(ctx context.Context, options VideoDetailsOptions) (content.Pack, error) {
	if err := validateVideoIDs(options.IDs); err != nil {
		return content.Pack{}, err
	}

	var response youtubeVideosResponse
	if err := client.get(ctx, "/videos", videoDetailsQuery(options), "videos", &response); err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapVideoItems(response.Items),
		Metadata: map[string]any{"quota_cost": 1},
	}, nil
}

func (client *Client) get(ctx context.Context, path string, query url.Values, operation string, target any) error {
	apiKey := strings.TrimSpace(client.settings.YouTubeAPIKey)
	if apiKey == "" {
		return fmt.Errorf("youtube %s: api key is required", operation)
	}
	query.Set("key", apiKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("youtube %s: build request: %w", operation, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return youtubeTransportError(operation, err, apiKey)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return youtubeStatusError(operation, resp, apiKey)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("youtube %s decode response: %w", operation, err)
	}

	return nil
}

func searchQuery(options SearchOptions) url.Values {
	query := url.Values{}
	query.Set("part", "snippet")
	query.Set("type", "video")
	query.Set("q", options.Query)
	if options.MaxResults > 0 {
		query.Set("maxResults", strconv.Itoa(options.MaxResults))
	}
	if options.RegionCode != "" {
		query.Set("regionCode", options.RegionCode)
	}
	if options.RelevanceLanguage != "" {
		query.Set("relevanceLanguage", options.RelevanceLanguage)
	}
	if options.SafeSearch != "" {
		query.Set("safeSearch", options.SafeSearch)
	}
	if options.PageToken != "" {
		query.Set("pageToken", options.PageToken)
	}

	return query
}

func videoDetailsQuery(options VideoDetailsOptions) url.Values {
	query := url.Values{}
	query.Set("part", "snippet,contentDetails,statistics")
	query.Set("id", strings.Join(options.IDs, ","))

	return query
}

func validateVideoIDs(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("youtube videos: invalid video id: at least one id is required")
	}
	for _, id := range ids {
		if id == "" || !validVideoID.MatchString(id) {
			return fmt.Errorf("youtube videos: invalid video id %q", id)
		}
	}

	return nil
}

func youtubeTransportError(operation string, err error, apiKey string) error {
	return redactedYouTubeError{
		message: fmt.Sprintf("youtube %s request: %s", operation, redactYouTubeSecret(err.Error(), apiKey)),
		err:     err,
	}
}

func youtubeStatusError(operation string, resp *http.Response, apiKey string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, responseBodyMax))
	message := redactYouTubeSecret(strings.TrimSpace(string(body)), apiKey)

	return fmt.Errorf("youtube %s failed: status %d: %s", operation, resp.StatusCode, message)
}

func redactYouTubeSecret(message string, apiKey string) string {
	message = youtubeAPIKeyParam.ReplaceAllString(message, `${1}[redacted]`)
	if apiKey == "" {
		return message
	}
	message = strings.ReplaceAll(message, apiKey, "[redacted]")
	message = strings.ReplaceAll(message, url.QueryEscape(apiKey), "[redacted]")

	return message
}
