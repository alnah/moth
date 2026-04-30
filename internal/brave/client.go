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

type searchEndpoint struct {
	name string
	path string
}

var (
	webSearchEndpoint = searchEndpoint{
		name: "web",
		path: "/res/v1/web/search",
	}
	imagesSearchEndpoint = searchEndpoint{
		name: "images",
		path: "/res/v1/images/search",
	}
	videosSearchEndpoint = searchEndpoint{
		name: "videos",
		path: "/res/v1/videos/search",
	}
)

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
	return searchBrave(ctx, client, webSearchEndpoint, options, mapWebResponseItems)
}

// SearchImages returns normalized Brave image results.
func (client *Client) SearchImages(ctx context.Context, options SearchOptions) (content.Pack, error) {
	return searchBrave(ctx, client, imagesSearchEndpoint, options, mapImagesResponseItems)
}

// SearchVideos returns normalized Brave video results.
func (client *Client) SearchVideos(ctx context.Context, options SearchOptions) (content.Pack, error) {
	return searchBrave(ctx, client, videosSearchEndpoint, options, mapVideosResponseItems)
}

func searchBrave[T any](
	ctx context.Context,
	client *Client,
	endpoint searchEndpoint,
	options SearchOptions,
	mapResponseItems func(T) []content.Item,
) (content.Pack, error) {
	var response T
	metadata, err := client.get(ctx, endpoint, options, &response)
	if err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapResponseItems(response),
		Metadata: metadata,
	}, nil
}

func (client *Client) get(
	ctx context.Context,
	endpoint searchEndpoint,
	options SearchOptions,
	target any,
) (map[string]any, error) {
	apiKey := strings.TrimSpace(client.settings.BraveAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("brave %s search: api key is required", endpoint.name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.requestURL(endpoint, options), nil)
	if err != nil {
		return nil, fmt.Errorf("brave %s search: build request: %w", endpoint.name, err)
	}
	req.Header.Set(braveTokenHeader, apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave %s search request: %w", endpoint.name, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, providerStatusError(endpoint, resp)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return nil, fmt.Errorf("decode brave %s response: %w", endpoint.name, err)
	}

	return rateLimitMetadata(resp.Header), nil
}

func (client *Client) requestURL(endpoint searchEndpoint, options SearchOptions) string {
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

	return client.baseURL + endpoint.path + "?" + query.Encode()
}

func providerStatusError(endpoint searchEndpoint, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, responseBodyMax))
	responseText := strings.TrimSpace(string(body))

	return fmt.Errorf("brave %s search failed: status %d: %s", endpoint.name, resp.StatusCode, responseText)
}

func rateLimitMetadata(header http.Header) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "rate_limit_limit", header.Get("X-RateLimit-Limit"))
	addStringMetadata(metadata, "rate_limit_remaining", header.Get("X-RateLimit-Remaining"))
	addStringMetadata(metadata, "rate_limit_reset", header.Get("X-RateLimit-Reset"))
	if len(metadata) == 0 {
		return nil
	}

	return metadata
}
