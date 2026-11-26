package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
	"github.com/alnah/moth/internal/webfetch"
)

type fakeBrowserClient struct {
	request browser.PageRequest
	page    browser.LoadedPage
	err     error
	block   bool
}

func (client *fakeBrowserClient) FetchRenderedPage(
	ctx context.Context,
	request browser.PageRequest,
) (browser.LoadedPage, error) {
	client.request = request
	if client.block {
		<-ctx.Done()
		return browser.LoadedPage{}, ctx.Err()
	}
	return client.page, client.err
}

func TestSupervisorBrowserFetcherMapsRequestAndResponse(t *testing.T) {
	client := &fakeBrowserClient{
		page: browser.LoadedPage{
			URL:  "https://example.test/final",
			HTML: `<html><title>rendered</title></html>`,
		},
	}
	fetcher := supervisorBrowserFetcher{client: client}

	response, err := fetcher.FetchRenderedPage(context.Background(), webfetch.BrowserRequest{
		URL:      "https://example.test/start",
		MaxBytes: 4096,
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("FetchRenderedPage() error = %v", err)
	}
	if client.request.URL != "https://example.test/start" || client.request.MaxBytes != 4096 {
		t.Fatalf("browser request = %#v, want URL and max bytes", client.request)
	}
	if response.URL != "https://example.test/final" {
		t.Fatalf("response.URL = %q, want final URL", response.URL)
	}
	if response.ContentType != "text/html" {
		t.Fatalf("response.ContentType = %q, want text/html", response.ContentType)
	}
	if response.HTML != `<html><title>rendered</title></html>` {
		t.Fatalf("response.HTML = %q, want rendered HTML", response.HTML)
	}
}

func TestSupervisorBrowserFetcherHonorsTimeout(t *testing.T) {
	client := &fakeBrowserClient{block: true}
	fetcher := supervisorBrowserFetcher{client: client}

	_, err := fetcher.FetchRenderedPage(context.Background(), webfetch.BrowserRequest{
		URL:     "https://example.test/slow",
		Timeout: 10 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FetchRenderedPage(timeout) error = %v, want context.DeadlineExceeded", err)
	}
}

func TestSupervisorBrowserFetcherPreservesClientError(t *testing.T) {
	client := &fakeBrowserClient{err: supervisor.ErrDaemonMethod}
	fetcher := supervisorBrowserFetcher{client: client}

	_, err := fetcher.FetchRenderedPage(context.Background(), webfetch.BrowserRequest{
		URL: "https://example.test/page",
	})
	if !errors.Is(err, supervisor.ErrDaemonMethod) {
		t.Fatalf("FetchRenderedPage() error = %v, want ErrDaemonMethod", err)
	}
}

func TestFetchBrowserUsesSupervisorAdapter(t *testing.T) {
	harness := newCommandHarness()
	client := &fakeBrowserClient{
		page: browser.LoadedPage{
			URL:  "https://example.test/page",
			HTML: `<html><title>adapter</title></html>`,
		},
	}
	harness.deps.WebFetch = webfetch.New(webfetch.Options{
		BrowserFetcher: supervisorBrowserFetcher{client: client},
	})

	stdout, stderr, err := harness.execute("fetch", "--browser", "https://example.test/page")
	if err != nil {
		t.Fatalf("execute fetch --browser: %v\nstderr: %s", err, stderr)
	}
	assertContentPackJSON(t, stdout)
	if client.request.URL != "https://example.test/page" {
		t.Fatalf("browser request URL = %q, want fetched URL", client.request.URL)
	}
}
