package x

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const xBearerTokenPattern = "Bearer "

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
