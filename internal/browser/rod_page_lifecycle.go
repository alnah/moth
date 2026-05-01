package browser

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (worker *rodWorker) withStatelessPage(
	ctx context.Context,
	pageURL string,
	headers map[string]string,
	userAgent string,
	use func(*rod.Page) error,
) error {
	page, err := worker.newOperationPage(ctx, pageURL, headers, userAgent)
	if err != nil {
		return err
	}
	defer func() { _ = page.Close() }()
	return use(page)
}

func (worker *rodWorker) newOperationPage(
	ctx context.Context,
	pageURL string,
	headers map[string]string,
	userAgent string,
) (*rod.Page, error) {
	return worker.newLoadedPage(ctx, pageURL, headers, userAgent, "page")
}

func (worker *rodWorker) newPersistentPage(
	ctx context.Context,
	pageURL string,
	headers map[string]string,
	userAgent string,
) (*rod.Page, error) {
	page, err := worker.newLoadedPage(ctx, pageURL, headers, userAgent, "persistent page")
	if err != nil {
		return nil, err
	}
	return page.Context(context.Background()), nil
}

func (worker *rodWorker) newLoadedPage(
	ctx context.Context,
	pageURL string,
	headers map[string]string,
	userAgent string,
	label string,
) (*rod.Page, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	page, err := worker.browser.Context(ctx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("create %s: %w", label, err)
	}
	page = page.Context(ctx)

	if err := worker.configurePage(page, headers, userAgent); err != nil {
		_ = page.Close()
		return nil, err
	}
	if err := page.Navigate(pageURL); err != nil {
		_ = page.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("navigate %s: %w", label, err)
	}
	if err := page.WaitLoad(); err != nil {
		_ = page.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("wait %s load: %w", label, err)
	}
	return page, nil
}

func (worker *rodWorker) configurePage(page *rod.Page, headers map[string]string, userAgent string) error {
	if userAgent != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: userAgent}); err != nil {
			return fmt.Errorf("set user agent: %w", err)
		}
	}
	if urls := blockedURLPatterns(worker.options.BlockedResources); len(urls) > 0 {
		page.EnableDomain(proto.NetworkEnable{})
		if err := page.SetBlockedURLs(urls); err != nil {
			return fmt.Errorf("set blocked urls: %w", err)
		}
	}
	if len(headers) == 0 {
		return nil
	}
	cleanup, err := page.SetExtraHeaders(headerPairs(headers))
	if err != nil {
		return fmt.Errorf("set headers: %w", err)
	}
	_ = cleanup
	return nil
}

func blockedURLPatterns(resources ResourceSet) []string {
	patterns := []string{}
	if resources.Has(ResourceImages) {
		patterns = append(patterns, "*.avif", "*.gif", "*.jpeg", "*.jpg", "*.png", "*.svg", "*.webp")
	}
	if resources.Has(ResourceFonts) {
		patterns = append(patterns, "*.otf", "*.ttf", "*.woff", "*.woff2")
	}
	if resources.Has(ResourceMedia) {
		patterns = append(patterns, "*.avi", "*.m4a", "*.mp3", "*.mp4", "*.mpeg", "*.ogg", "*.webm")
	}
	return patterns
}

func headerPairs(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(headers)*2)
	for _, key := range keys {
		pairs = append(pairs, key, headers[key])
	}
	return pairs
}
