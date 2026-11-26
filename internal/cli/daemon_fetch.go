package cli

import (
	"context"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/webfetch"
)

type browserClient interface {
	FetchRenderedPage(context.Context, browser.PageRequest) (browser.LoadedPage, error)
}

type supervisorBrowserFetcher struct {
	client browserClient
}

func (fetcher supervisorBrowserFetcher) FetchRenderedPage(
	ctx context.Context,
	request webfetch.BrowserRequest,
) (webfetch.BrowserResponse, error) {
	if request.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
	}

	page, err := fetcher.client.FetchRenderedPage(ctx, browser.PageRequest{
		URL:      request.URL,
		MaxBytes: request.MaxBytes,
	})
	if err != nil {
		return webfetch.BrowserResponse{}, err
	}
	return webfetch.BrowserResponse{
		URL:         page.URL,
		ContentType: "text/html",
		HTML:        page.HTML,
	}, nil
}
