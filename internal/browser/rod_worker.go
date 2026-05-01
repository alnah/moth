package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

const browserCloseTimeout = 5 * time.Second

type rodWorker struct {
	browser  *rod.Browser
	launcher *launcher.Launcher
	options  WorkerOptions

	mu       sync.Mutex
	sessions map[string]*rodSession

	closeOnce sync.Once
}

func newRodWorker(ctx context.Context, config poolConfig) (*rodWorker, error) {
	browserBin := resolvedBrowserBin(config.browserBin)
	if err := validateBrowserBin(browserBin); err != nil {
		return nil, err
	}
	userDataDir := config.userDataDir()
	if userDataDir != "" {
		if err := os.MkdirAll(userDataDir, 0o750); err != nil {
			return nil, fmt.Errorf("create browser profile: %w", err)
		}
	}

	workerOptions := config.workerOptions()
	browserLauncher := launcher.New().
		Context(ctx).
		Headless(config.headless).
		Leakless(false).
		Set(flags.Flag("disable-gpu"))
	if browserBin != "" {
		browserLauncher = browserLauncher.Bin(browserBin)
	}
	if config.noSandbox || os.Getenv("ROD_NO_SANDBOX") == "1" {
		browserLauncher = browserLauncher.NoSandbox(true)
	}
	if config.proxyURL != "" {
		browserLauncher = browserLauncher.Proxy(config.proxyURL)
	}
	if userDataDir != "" {
		browserLauncher = browserLauncher.UserDataDir(userDataDir).KeepUserDataDir()
	}

	controlURL, err := browserLauncher.Launch()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("launch browser: %w: %w", ErrBrowserMissing, err)
	}

	rodBrowser := rod.New().ControlURL(controlURL).Context(ctx)
	if err := rodBrowser.Connect(); err != nil {
		browserLauncher.Kill()
		browserLauncher.Cleanup()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("connect browser: %w: %w", ErrBrowserMissing, err)
	}

	return &rodWorker{
		browser:  rodBrowser.Context(context.Background()),
		launcher: browserLauncher,
		options:  workerOptions,
		sessions: make(map[string]*rodSession),
	}, nil
}

func resolvedBrowserBin(path string) string {
	if path != "" {
		return path
	}
	return os.Getenv("ROD_BROWSER_BIN")
}

func validateBrowserBin(path string) error {
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("browser binary %q: %w: %w", path, ErrBrowserMissing, err)
	}
	if info.IsDir() {
		return fmt.Errorf("browser binary %q is a directory: %w", path, ErrBrowserMissing)
	}
	return nil
}

func (worker *rodWorker) OpenPage(ctx context.Context, request PageRequest) (LoadedPage, error) {
	page, err := worker.newOperationPage(ctx, request.URL, request.Headers, request.UserAgent)
	if err != nil {
		return LoadedPage{}, err
	}
	defer func() { _ = page.Close() }()

	html, err := page.HTML()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return LoadedPage{}, ctxErr
		}
		return LoadedPage{}, fmt.Errorf("read rendered html: %w", err)
	}
	return LoadedPage{URL: request.URL, HTML: html}, nil
}

func (worker *rodWorker) CaptureScreenshot(ctx context.Context, request ScreenshotRequest) ([]byte, error) {
	page, err := worker.newOperationPage(ctx, request.URL, request.Headers, request.UserAgent)
	if err != nil {
		return nil, err
	}
	defer func() { _ = page.Close() }()

	image, err := page.Screenshot(request.FullPage, &proto.PageCaptureScreenshot{
		Format:                proto.PageCaptureScreenshotFormatPng,
		FromSurface:           true,
		CaptureBeyondViewport: request.FullPage,
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("capture screenshot: %w", err)
	}
	return image, nil
}

func (worker *rodWorker) Close() error {
	var closeErr error
	worker.closeOnce.Do(func() {
		pagesErr := worker.closePersistentPages()
		if worker.browser != nil {
			closeErr = closeBrowserWithTimeout(worker.browser)
			worker.browser = nil
		}
		if worker.launcher != nil {
			worker.launcher.Kill()
			worker.launcher.Cleanup()
			worker.launcher = nil
		}
		closeErr = errors.Join(pagesErr, closeErr)
	})
	return closeErr
}

func (worker *rodWorker) newOperationPage(
	ctx context.Context,
	pageURL string,
	headers map[string]string,
	userAgent string,
) (*rod.Page, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	page, err := worker.browser.Context(ctx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("create page: %w", err)
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
		return nil, fmt.Errorf("navigate page: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		_ = page.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("wait page load: %w", err)
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

func closeBrowserWithTimeout(browser *rod.Browser) error {
	done := make(chan error, 1)
	go func() {
		done <- browser.Close()
	}()

	timer := time.NewTimer(browserCloseTimeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err
	case <-timer.C:
		return nil
	}
}
