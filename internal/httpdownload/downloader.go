// Package httpdownload downloads bounded HTTP resources.
package httpdownload

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alnah/moth/internal/limits"
)

// Semantic downloader errors.
var (
	ErrUnsupportedContentType = errors.New("unsupported_content_type")
	ErrFileTooLarge           = errors.New("file_too_large")
	ErrTimeout                = errors.New("timeout")
	ErrHTTPStatus             = errors.New("http_status")
)

// Options configures a Downloader.
type Options struct {
	HTTPClient *http.Client
}

// Request describes one bounded HTTP download.
type Request struct {
	URL                 string
	AllowedContentTypes []string
	MaxBytes            int64
	Timeout             time.Duration
}

// Response contains a completed HTTP download.
type Response struct {
	URL         string
	ContentType string
	Bytes       []byte
}

// Downloader fetches one URL with type and byte limits.
type Downloader struct {
	httpClient *http.Client
}

// New creates a Downloader using defaults for unset options.
func New(options Options) *Downloader {
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Downloader{httpClient: httpClient}
}

// Download fetches a URL and rejects disallowed content types, statuses, timeouts, and oversized bodies.
func (downloader *Downloader) Download(ctx context.Context, request Request) (Response, error) {
	ctx, cancel := contextWithOptionalTimeout(ctx, request.Timeout)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return Response{}, fmt.Errorf("create download request: %w", err)
	}

	//nolint:gosec // Downloader intentionally fetches caller-provided URLs with byte and type limits.
	resp, err := downloader.httpClient.Do(httpRequest)
	if err != nil {
		return Response{}, wrapTimeout("download request", err)
	}
	defer closeResponseBody(resp)

	err = rejectStatus(resp.StatusCode)
	if err != nil {
		return Response{}, err
	}

	contentType := normalizeContentType(resp.Header.Get("Content-Type"))
	if !contentTypeAllowed(contentType, request.AllowedContentTypes) {
		return Response{}, fmt.Errorf("download content type %q: %w", contentType, ErrUnsupportedContentType)
	}

	maxBytes := request.MaxBytes
	if maxBytes <= 0 {
		maxBytes = limits.DefaultMaxBytes
	}
	if resp.ContentLength > maxBytes {
		return Response{}, fmt.Errorf("download content length %d over %d: %w", resp.ContentLength, maxBytes, ErrFileTooLarge)
	}

	body, err := readBounded(resp.Body, maxBytes)
	if err != nil {
		return Response{}, wrapTimeout("read download body", err)
	}

	return Response{URL: request.URL, ContentType: contentType, Bytes: body}, nil
}

func closeResponseBody(resp *http.Response) {
	_ = resp.Body.Close()
}
