package browser

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

func (worker *rodWorker) ResponseMetadata(
	ctx context.Context,
	request ResponseMetadataRequest,
) (ResponseMetadata, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return ResponseMetadata{}, fmt.Errorf("create metadata request: %w", err)
	}
	//nolint:gosec // Browser metadata intentionally fetches the caller-provided URL.
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return ResponseMetadata{}, fmt.Errorf("fetch response metadata: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	return ResponseMetadata{
		URL:         response.Request.URL.String(),
		Status:      response.StatusCode,
		ContentType: response.Header.Get("Content-Type"),
		Headers:     response.Header.Clone(),
	}, nil
}
