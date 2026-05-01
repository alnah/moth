package x

import (
	"cmp"
	"context"
	"net/url"
	"strings"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

const (
	defaultBaseURL     = "https://api.x.com"
	defaultMaxResults  = 10
	defaultMaxRequests = 1
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

// Client sends read-only raw HTTP requests to the X API.
type Client struct {
	settings   config.Settings
	baseURL    string
	httpClient *httpclient.Client
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
