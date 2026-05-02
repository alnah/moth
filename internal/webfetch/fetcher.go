// Package webfetch orchestrates URL fetching into normalized content packs.
package webfetch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpdownload"
)

// ErrBrowserFetcherRequired reports that browser mode was requested without a browser dependency.
var ErrBrowserFetcherRequired = errors.New("browser_fetcher_required")

// Downloader is the bounded HTTP acquisition dependency used by Fetcher.
type Downloader interface {
	Download(context.Context, httpdownload.Request) (httpdownload.Response, error)
}

// BrowserFetcher is the rendered-page acquisition dependency used by browser mode.
type BrowserFetcher interface {
	FetchRenderedPage(context.Context, BrowserRequest) (BrowserResponse, error)
}

// Options configures a Fetcher.
type Options struct {
	Downloader     Downloader
	BrowserFetcher BrowserFetcher
}

// Request describes one user-facing fetch operation.
type Request struct {
	URL         string
	UseBrowser  bool
	IncludeText bool
	MaxBytes    int64
	Timeout     time.Duration
}

// BrowserRequest describes one rendered-page fetch operation.
type BrowserRequest struct {
	URL      string
	MaxBytes int64
	Timeout  time.Duration
}

// BrowserResponse contains rendered page HTML and response metadata.
type BrowserResponse struct {
	URL         string
	ContentType string
	HTML        string
}

// Fetcher fetches URLs through HTTP or browser primitives and normalizes the result.
type Fetcher struct {
	downloader     Downloader
	browserFetcher BrowserFetcher
}

// New creates a Fetcher using default bounded HTTP download when no downloader is provided.
func New(options Options) *Fetcher {
	downloader := options.Downloader
	if downloader == nil {
		downloader = httpdownload.New(httpdownload.Options{})
	}

	return &Fetcher{
		downloader:     downloader,
		browserFetcher: options.BrowserFetcher,
	}
}

// Fetch returns a normalized content pack for request.URL.
func (fetcher *Fetcher) Fetch(ctx context.Context, request Request) (content.Pack, error) {
	if request.UseBrowser {
		return fetcher.fetchBrowser(ctx, request)
	}

	return fetcher.fetchHTTP(ctx, request)
}

func (fetcher *Fetcher) fetchHTTP(ctx context.Context, request Request) (content.Pack, error) {
	response, err := fetcher.downloader.Download(ctx, httpdownload.Request{
		URL:                 request.URL,
		AllowedContentTypes: nil,
		MaxBytes:            request.MaxBytes,
		Timeout:             request.Timeout,
	})
	if err != nil {
		return content.Pack{}, fmt.Errorf("fetch %s: %w", request.URL, err)
	}

	return packWithItem(normalizeHTTPResponse(request, response)), nil
}

func (fetcher *Fetcher) fetchBrowser(ctx context.Context, request Request) (content.Pack, error) {
	if fetcher.browserFetcher == nil {
		return content.Pack{}, fmt.Errorf("fetch %s: %w", request.URL, ErrBrowserFetcherRequired)
	}

	response, err := fetcher.browserFetcher.FetchRenderedPage(ctx, BrowserRequest{
		URL:      request.URL,
		MaxBytes: request.MaxBytes,
		Timeout:  request.Timeout,
	})
	if err != nil {
		return content.Pack{}, fmt.Errorf("fetch %s: %w", request.URL, err)
	}

	return packWithItem(normalizeBrowserResponse(request, response)), nil
}

func normalizeHTTPResponse(request Request, response httpdownload.Response) content.Item {
	resolvedURL := finalURL(request.URL, response.URL)
	metadata := map[string]any{
		"source":       "http",
		"final_url":    resolvedURL,
		"content_type": response.ContentType,
		"bytes":        int64(len(response.Bytes)),
	}

	if isHTML(response.ContentType) {
		document, _ := html.Parse(strings.NewReader(string(response.Bytes)))
		return pageItem(document, resolvedURL, request.IncludeText, metadata)
	}

	return content.Item{
		Kind:     content.KindFile,
		URL:      resolvedURL,
		Metadata: metadata,
		Warnings: []content.Warning{},
	}
}

func normalizeBrowserResponse(request Request, response BrowserResponse) content.Item {
	resolvedURL := finalURL(request.URL, response.URL)
	contentType := response.ContentType
	if contentType == "" {
		contentType = "text/html"
	}
	metadata := map[string]any{
		"source":       "browser",
		"final_url":    resolvedURL,
		"content_type": contentType,
		"bytes":        int64(len(response.HTML)),
	}

	document, _ := html.Parse(strings.NewReader(response.HTML))
	return pageItem(document, resolvedURL, request.IncludeText, metadata)
}

func pageItem(document *html.Node, url string, includeText bool, metadata map[string]any) content.Item {
	item := content.Item{
		Kind:     content.KindPage,
		URL:      url,
		Title:    firstTitle(document),
		Metadata: metadata,
		Warnings: []content.Warning{},
	}
	if includeText {
		item.Text = visibleText(document)
	}
	return item
}

func packWithItem(item content.Item) content.Pack {
	return content.Pack{
		Type:     content.TypeContentPack,
		Items:    []content.Item{item},
		Warnings: []content.Warning{},
	}
}

func finalURL(requestURL string, responseURL string) string {
	if responseURL != "" {
		return responseURL
	}
	return requestURL
}

func isHTML(contentType string) bool {
	return strings.EqualFold(strings.TrimSpace(contentType), "text/html")
}

func firstTitle(node *html.Node) string {
	if node.Type == html.ElementNode && node.Data == "title" {
		return normalizedText(node)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if title := firstTitle(child); title != "" {
			return title
		}
	}
	return ""
}

func visibleText(node *html.Node) string {
	return strings.Join(visibleTextFields(node, nil), " ")
}

func visibleTextFields(node *html.Node, fields []string) []string {
	if ignoredTextNode(node) {
		return fields
	}
	if node.Type == html.TextNode {
		return append(fields, strings.Fields(node.Data)...)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		fields = visibleTextFields(child, fields)
	}
	return fields
}

func ignoredTextNode(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}

	switch node.Data {
	case "head", "script", "style", "noscript", "template", "svg":
		return true
	default:
		return false
	}
}

func normalizedText(node *html.Node) string {
	return strings.Join(textFields(node, nil), " ")
}

func textFields(node *html.Node, fields []string) []string {
	if ignoredTextNode(node) {
		return fields
	}
	if node.Type == html.TextNode {
		return append(fields, strings.Fields(node.Data)...)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		fields = textFields(child, fields)
	}
	return fields
}
