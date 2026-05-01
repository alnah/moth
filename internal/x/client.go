// Package x calls the X API and maps posts to content items.
package x

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
	defaultBaseURL      = "https://api.x.com"
	defaultMaxResults   = 10
	defaultMaxRequests  = 1
	responseBodyMax     = 4096
	xBearerTokenPattern = "Bearer "
)

// Config contains X API dependencies and credentials.
type Config struct {
	Settings   config.Settings
	BaseURL    string
	HTTPClient *httpclient.Client
}

// SearchOptions contains X recent search query parameters and request guards.
type SearchOptions struct {
	Query       string
	MaxResults  int
	MaxRequests int
	NextToken   string
}

// LookupPostOptions contains X post lookup parameters.
type LookupPostOptions struct {
	ID string
}

// UserPostsOptions contains X user posts query parameters and request guards.
type UserPostsOptions struct {
	UserID      string
	MaxResults  int
	MaxRequests int
	NextToken   string
}

// Client sends raw HTTP requests to the X API.
type Client struct {
	settings   config.Settings
	baseURL    string
	httpClient *httpclient.Client
}

type redactedXError struct {
	message string
	err     error
}

func (err redactedXError) Error() string {
	return err.message
}

func (err redactedXError) Unwrap() error {
	return err.err
}

// NewClient creates an X client with defaults for unset dependencies.
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

// SearchRecent returns normalized recent X posts.
func (client *Client) SearchRecent(ctx context.Context, options SearchOptions) (content.Pack, error) {
	budget := requestBudget(options.MaxRequests)
	limit := resultLimit(options.MaxResults)
	query := searchQuery(options, limit)

	posts, users, metadata, err := client.collectPages(
		ctx,
		"recent search",
		"/2/tweets/search/recent",
		query,
		budget,
		searchNextToken,
	)
	if err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapPosts(posts, users, limit),
		Metadata: metadata,
	}, nil
}

// LookupPost returns one normalized X post.
func (client *Client) LookupPost(ctx context.Context, options LookupPostOptions) (content.Pack, error) {
	var response xSinglePostResponse
	metadata, err := client.get(ctx, "post lookup", "/2/tweets/"+url.PathEscape(options.ID), lookupQuery(), &response)
	if err != nil {
		return content.Pack{}, err
	}
	metadata = mergeResponseMetadata(metadata, response.Meta)

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapPosts([]xPost{response.Data}, usersByID(response.Includes.Users), 1),
		Metadata: metadata,
	}, nil
}

// UserPosts returns normalized posts authored by one X user.
func (client *Client) UserPosts(ctx context.Context, options UserPostsOptions) (content.Pack, error) {
	budget := requestBudget(options.MaxRequests)
	limit := resultLimit(options.MaxResults)
	query := userPostsQuery(options, limit)
	path := "/2/users/" + url.PathEscape(options.UserID) + "/tweets"

	posts, users, metadata, err := client.collectPages(ctx, "user posts", path, query, budget, userPostsNextToken)
	if err != nil {
		return content.Pack{}, err
	}

	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    mapPosts(posts, users, limit),
		Metadata: metadata,
	}, nil
}

func (client *Client) collectPages(
	ctx context.Context,
	operation string,
	path string,
	query url.Values,
	maxRequests int,
	nextTokenParam string,
) ([]xPost, map[string]xUser, map[string]any, error) {
	posts := make([]xPost, 0)
	users := make(map[string]xUser)
	metadata := map[string]any(nil)

	for requestNumber := 0; requestNumber < maxRequests; requestNumber++ {
		var response xPostListResponse
		pageMetadata, err := client.get(ctx, operation, path, query, &response)
		if err != nil {
			return nil, nil, nil, err
		}
		metadata = mergeMetadata(metadata, pageMetadata)
		metadata = mergeResponseMetadata(metadata, response.Meta)
		posts = append(posts, response.Data...)
		for _, user := range response.Includes.Users {
			if user.ID != "" {
				users[user.ID] = user
			}
		}
		if response.Meta.NextToken == "" || requestNumber+1 >= maxRequests {
			break
		}
		query.Set(nextTokenParam, response.Meta.NextToken)
	}

	return posts, users, metadata, nil
}

func (client *Client) get(
	ctx context.Context,
	operation string,
	path string,
	query url.Values,
	target any,
) (map[string]any, error) {
	bearerToken := strings.TrimSpace(client.settings.XBearerToken)
	if bearerToken == "" {
		return nil, fmt.Errorf("x %s: not configured: bearer token is required", operation)
	}

	requestURL := client.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		requestURL += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("x %s: build request: %w", operation, err)
	}
	req.Header.Set("Authorization", xBearerTokenPattern+bearerToken)
	req.Header.Set("Accept", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return nil, xTransportError(operation, err, bearerToken)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, xStatusError(operation, resp, bearerToken)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return nil, fmt.Errorf("x %s decode response: %w", operation, err)
	}

	return rateLimitMetadata(resp.Header), nil
}

func searchQuery(options SearchOptions, limit int) url.Values {
	query := commonPostQuery()
	query.Set("query", options.Query)
	query.Set("max_results", strconv.Itoa(limit))
	if options.NextToken != "" {
		query.Set(searchNextToken, options.NextToken)
	}

	return query
}

func lookupQuery() url.Values {
	return commonPostQuery()
}

func userPostsQuery(options UserPostsOptions, limit int) url.Values {
	query := commonPostQuery()
	query.Set("max_results", strconv.Itoa(limit))
	if options.NextToken != "" {
		query.Set(userPostsNextToken, options.NextToken)
	}

	return query
}

func commonPostQuery() url.Values {
	query := url.Values{}
	query.Set("expansions", "author_id")
	query.Set("tweet.fields", "created_at,author_id")
	query.Set("user.fields", "username,name")

	return query
}

func requestBudget(maxRequests int) int {
	if maxRequests > 0 {
		return maxRequests
	}

	return defaultMaxRequests
}

func resultLimit(maxResults int) int {
	if maxResults > 0 {
		return maxResults
	}

	return defaultMaxResults
}

func rateLimitMetadata(header http.Header) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "rate_limit_limit", header.Get("X-Rate-Limit-Limit"))
	addStringMetadata(metadata, "rate_limit_remaining", header.Get("X-Rate-Limit-Remaining"))
	addStringMetadata(metadata, "rate_limit_reset", header.Get("X-Rate-Limit-Reset"))
	if len(metadata) == 0 {
		return nil
	}

	return metadata
}

func mergeResponseMetadata(metadata map[string]any, responseMetadata xResponseMeta) map[string]any {
	if responseMetadata.ResultCount == 0 && responseMetadata.NextToken == "" {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	addIntMetadata(metadata, "result_count", responseMetadata.ResultCount)
	addStringMetadata(metadata, "next_token", responseMetadata.NextToken)

	return metadata
}

func mergeMetadata(left map[string]any, right map[string]any) map[string]any {
	if len(right) == 0 {
		return left
	}
	if left == nil {
		left = make(map[string]any, len(right))
	}
	for key, value := range right {
		left[key] = value
	}

	return left
}

func xTransportError(operation string, err error, bearerToken string) error {
	return redactedXError{
		message: fmt.Sprintf("x %s request: %s", operation, redactXSecret(err.Error(), bearerToken)),
		err:     err,
	}
}

func xStatusError(operation string, resp *http.Response, bearerToken string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, responseBodyMax))
	message := redactXSecret(strings.TrimSpace(string(body)), bearerToken)
	prefix := fmt.Sprintf("x %s failed", operation)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		prefix = fmt.Sprintf("x %s access denied", operation)
	}

	return fmt.Errorf("%s: status %d: %s", prefix, resp.StatusCode, message)
}

func redactXSecret(message string, bearerToken string) string {
	if bearerToken == "" {
		return message
	}
	message = strings.ReplaceAll(message, bearerToken, "[redacted]")
	message = strings.ReplaceAll(message, url.QueryEscape(bearerToken), "[redacted]")
	message = strings.ReplaceAll(message, xBearerTokenPattern+bearerToken, xBearerTokenPattern+"[redacted]")

	return message
}
