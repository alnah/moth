// Package brave searches Brave web, image, and video endpoints.
package brave

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const (
	defaultBaseURL = "https://api.search.brave.com"

	webEndpoint    = "web"
	imagesEndpoint = "images"
	videosEndpoint = "videos"

	webPath    = "/res/v1/web/search"
	imagesPath = "/res/v1/images/search"
	videosPath = "/res/v1/videos/search"

	braveTokenHeader = "X-Subscription-Token"
	responseBodyMax  = 4096
)

// Config contains Brave client dependencies and credentials.
type Config struct {
	Settings   config.Settings
	BaseURL    string
	HTTPClient *httpclient.Client
}

// SearchOptions contains Brave search query parameters.
type SearchOptions struct {
	Query      string
	Count      int
	Country    string
	Language   string
	SafeSearch string
	Offset     int
}

// Client sends raw HTTP requests to the Brave Search API.
type Client struct {
	settings   config.Settings
	baseURL    string
	httpClient *httpclient.Client
}

// NewClient creates a Brave client with defaults for unset dependencies.
func NewClient(cfg Config) *Client {
	baseURL := cmp.Or(strings.TrimRight(cfg.BaseURL, "/"), defaultBaseURL)

	client := cfg.HTTPClient
	if client == nil {
		client = httpclient.New(httpclient.Options{})
	}

	return &Client{
		settings:   cfg.Settings,
		baseURL:    baseURL,
		httpClient: client,
	}
}

// SearchWeb returns normalized Brave web results.
func (client *Client) SearchWeb(ctx context.Context, options SearchOptions) (content.Pack, error) {
	var response braveWebResponse
	metadata, err := client.get(ctx, webEndpoint, webPath, options, &response)
	if err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapWebItems(response.Web.Results),
		Metadata: metadata,
	}, nil
}

// SearchImages returns normalized Brave image results.
func (client *Client) SearchImages(ctx context.Context, options SearchOptions) (content.Pack, error) {
	var response braveImagesResponse
	metadata, err := client.get(ctx, imagesEndpoint, imagesPath, options, &response)
	if err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapImageItems(response.Results),
		Metadata: metadata,
	}, nil
}

// SearchVideos returns normalized Brave video results.
func (client *Client) SearchVideos(ctx context.Context, options SearchOptions) (content.Pack, error) {
	var response braveVideosResponse
	metadata, err := client.get(ctx, videosEndpoint, videosPath, options, &response)
	if err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapVideoItems(response.Results),
		Metadata: metadata,
	}, nil
}

func (client *Client) get(
	ctx context.Context,
	endpoint string,
	path string,
	options SearchOptions,
	target any,
) (map[string]any, error) {
	apiKey := strings.TrimSpace(client.settings.BraveAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("brave %s search: api key is required", endpoint)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.requestURL(path, options), nil)
	if err != nil {
		return nil, fmt.Errorf("brave %s search: build request: %w", endpoint, err)
	}
	req.Header.Set(braveTokenHeader, apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave %s search request: %w", endpoint, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, providerStatusError(endpoint, resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return nil, fmt.Errorf("decode brave %s response: %w", endpoint, err)
	}

	return rateLimitMetadata(resp.Header), nil
}

func (client *Client) requestURL(path string, options SearchOptions) string {
	query := url.Values{}
	query.Set("q", options.Query)
	if options.Count > 0 {
		query.Set("count", strconv.Itoa(options.Count))
	}
	if options.Country != "" {
		query.Set("country", options.Country)
	}
	if options.Language != "" {
		query.Set("search_lang", options.Language)
	}
	if options.SafeSearch != "" {
		query.Set("safesearch", options.SafeSearch)
	}
	if options.Offset > 0 {
		query.Set("offset", strconv.Itoa(options.Offset))
	} else {
		query.Set("offset", "0")
	}

	return client.baseURL + path + "?" + query.Encode()
}

func providerStatusError(endpoint string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, responseBodyMax))
	responseText := strings.TrimSpace(string(body))

	return fmt.Errorf("brave %s search failed: status %d: %s", endpoint, resp.StatusCode, responseText)
}

func rateLimitMetadata(header http.Header) map[string]any {
	metadata := make(map[string]any)
	addHeaderMetadata(metadata, "rate_limit_limit", header.Get("X-RateLimit-Limit"))
	addHeaderMetadata(metadata, "rate_limit_remaining", header.Get("X-RateLimit-Remaining"))
	addHeaderMetadata(metadata, "rate_limit_reset", header.Get("X-RateLimit-Reset"))
	if len(metadata) == 0 {
		return nil
	}

	return metadata
}

func addHeaderMetadata(metadata map[string]any, key string, value string) {
	if value != "" {
		metadata[key] = value
	}
}

func mapWebItems(results []braveWebResult) []content.Item {
	items := make([]content.Item, 0, len(results))
	for _, result := range results {
		items = append(items, content.Item{
			Kind:  content.KindPage,
			URL:   result.URL,
			Title: result.Title,
			Text:  result.Description,
		})
	}

	return items
}

func mapImageItems(results []braveImageResult) []content.Item {
	items := make([]content.Item, 0, len(results))
	for _, result := range results {
		items = append(items, content.Item{
			Kind:     content.KindImage,
			URL:      result.Properties.URL,
			Title:    result.Title,
			Text:     result.Description,
			Metadata: imageMetadata(result),
		})
	}

	return items
}

func imageMetadata(result braveImageResult) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "page_url", result.URL)
	addStringMetadata(metadata, "thumbnail_url", result.Thumbnail.Src)
	addIntMetadata(metadata, "width", result.Properties.Width)
	addIntMetadata(metadata, "height", result.Properties.Height)

	return metadata
}

func mapVideoItems(results []braveVideoResult) []content.Item {
	items := make([]content.Item, 0, len(results))
	for _, result := range results {
		items = append(items, content.Item{
			Kind:     content.KindVideo,
			URL:      result.URL,
			Title:    result.Title,
			Text:     result.Description,
			Metadata: videoMetadata(result),
		})
	}

	return items
}

func videoMetadata(result braveVideoResult) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "thumbnail_url", result.Thumbnail.Src)
	addStringMetadata(metadata, "duration", result.Duration)
	addStringMetadata(metadata, "publisher", result.Publisher)

	return metadata
}

func addStringMetadata(metadata map[string]any, key string, value string) {
	if value != "" {
		metadata[key] = value
	}
}

func addIntMetadata(metadata map[string]any, key string, value int) {
	if value != 0 {
		metadata[key] = value
	}
}
