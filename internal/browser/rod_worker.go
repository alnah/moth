package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
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
		browserLauncher = browserLauncher.UserDataDir(userDataDir)
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
		cleanupRodLauncher(browserLauncher, userDataDir != "")
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
	var loadedPage LoadedPage
	err := worker.withStatelessPage(ctx, request.URL, request.Headers, request.UserAgent, func(page *rod.Page) error {
		html, err := page.HTML()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("read rendered html: %w", err)
		}
		loadedPage = LoadedPage{URL: request.URL, HTML: html}
		return nil
	})
	return loadedPage, err
}

func (worker *rodWorker) CaptureScreenshot(ctx context.Context, request ScreenshotRequest) ([]byte, error) {
	var image []byte
	err := worker.withStatelessPage(ctx, request.URL, request.Headers, request.UserAgent, func(page *rod.Page) error {
		capturedImage, err := page.Screenshot(request.FullPage, &proto.PageCaptureScreenshot{
			Format:                proto.PageCaptureScreenshotFormatPng,
			FromSurface:           true,
			CaptureBeyondViewport: request.FullPage,
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("capture screenshot: %w", err)
		}
		if err := rejectOversizedCapture(capturedImage, "screenshot", request.MaxBytes); err != nil {
			return err
		}
		image = capturedImage
		return nil
	})
	return image, err
}

func (worker *rodWorker) Close() error {
	var closeErr error
	worker.closeOnce.Do(func() {
		pagesErr := worker.closePersistentPages()
		browserErr := worker.closeBrowserProcess()
		worker.cleanupLauncherProcess()
		closeErr = errors.Join(pagesErr, browserErr)
	})
	return closeErr
}

func (worker *rodWorker) closeBrowserProcess() error {
	if worker.browser == nil {
		return nil
	}
	browserErr := closeBrowserWithTimeout(worker.browser)
	worker.browser = nil
	return browserErr
}

func (worker *rodWorker) cleanupLauncherProcess() {
	if worker.launcher == nil {
		return
	}
	cleanupRodLauncher(worker.launcher, worker.options.UserDataDir != "")
	worker.launcher = nil
}

func cleanupRodLauncher(browserLauncher *launcher.Launcher, preserveUserDataDir bool) {
	browserLauncher.Kill()
	if preserveUserDataDir {
		return
	}
	browserLauncher.Cleanup()
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
