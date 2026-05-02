package browser

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type rodPersistentLauncher struct{}

func (rodPersistentLauncher) LaunchBrowser(ctx context.Context, request LaunchRequest) (LaunchResult, error) {
	browserBin := resolvedBrowserBin(request.BrowserBin)
	if err := validateBrowserBin(browserBin); err != nil {
		return LaunchResult{}, err
	}
	if err := os.MkdirAll(request.DataDir, 0o750); err != nil {
		return LaunchResult{}, fmt.Errorf("create browser data directory: %w", err)
	}
	browserLauncher := launcher.New().
		Context(ctx).
		Headless(!request.Show).
		Leakless(false).
		UserDataDir(request.DataDir).
		KeepUserDataDir()
	if browserBin != "" {
		browserLauncher = browserLauncher.Bin(browserBin)
	}
	if request.NoSandbox || os.Getenv("ROD_NO_SANDBOX") == "1" {
		browserLauncher = browserLauncher.NoSandbox(true)
	}
	controlURL, err := browserLauncher.Launch()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return LaunchResult{}, ctxErr
		}
		return LaunchResult{}, fmt.Errorf("launch browser: %w: %w", ErrBrowserMissing, err)
	}
	return LaunchResult{DebugURL: controlURL, ChromePID: browserLauncher.PID()}, nil
}

type rodPersistentConnector struct{}

func (rodPersistentConnector) ConnectBrowser(ctx context.Context, debugURL string) (BrowserConnection, error) {
	rodBrowser := rod.New().ControlURL(debugURL).Context(ctx)
	if err := rodBrowser.Connect(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("connect browser: %w", err)
	}
	worker := &rodWorker{browser: rodBrowser.Context(context.Background()), sessions: make(map[string]*rodSession)}
	return &rodBrowserConnection{worker: worker}, nil
}

type rodBrowserConnection struct {
	worker *rodWorker
}

func (connection *rodBrowserConnection) OpenPage(ctx context.Context, request OpenPageRequest) (PageInfo, error) {
	return connection.worker.OpenPersistentPage(ctx, request)
}

func (connection *rodBrowserConnection) ListPages(ctx context.Context, request SessionRequest) ([]PageInfo, error) {
	if err := connection.hydrateSession(ctx, request, ""); err != nil {
		return nil, err
	}
	return connection.worker.ListPersistentPages(ctx, request)
}

func (connection *rodBrowserConnection) SwitchPage(ctx context.Context, request PageSelection) (PageInfo, error) {
	if err := connection.hydrateSession(ctx, selectionSession(request), request.PageID); err != nil {
		return PageInfo{}, err
	}
	return connection.worker.SwitchPersistentPage(ctx, request)
}

func (connection *rodBrowserConnection) ClosePage(ctx context.Context, request PageSelection) error {
	if err := connection.hydrateSession(ctx, selectionSession(request), request.PageID); err != nil {
		return err
	}
	return connection.worker.ClosePersistentPage(ctx, request)
}

func (connection *rodBrowserConnection) Click(ctx context.Context, request InteractionRequest) error {
	if err := connection.hydrateSession(ctx, interactionSession(request), request.PageID); err != nil {
		return err
	}
	return connection.worker.Click(ctx, request)
}

func (connection *rodBrowserConnection) Input(ctx context.Context, request InputRequest) error {
	if err := connection.hydrateSession(ctx, inputSession(request), request.PageID); err != nil {
		return err
	}
	return connection.worker.Input(ctx, request)
}

func (connection *rodBrowserConnection) Wait(ctx context.Context, request WaitRequest) error {
	if err := connection.hydrateSession(ctx, waitSession(request), request.PageID); err != nil {
		return err
	}
	return connection.worker.Wait(ctx, request)
}

func (connection *rodBrowserConnection) AccessibilityTree(
	ctx context.Context,
	request AccessibilityRequest,
) (AccessibilityTree, error) {
	if err := connection.hydrateSession(ctx, accessibilitySession(request), request.PageID); err != nil {
		return AccessibilityTree{}, err
	}
	return connection.worker.AccessibilityTree(ctx, request)
}

func (connection *rodBrowserConnection) DetectManualChallenge(
	ctx context.Context,
	request ManualChallengeRequest,
) (ManualChallengeResult, error) {
	if err := connection.hydrateSession(ctx, challengeSession(request), request.PageID); err != nil {
		return ManualChallengeResult{}, err
	}
	return connection.worker.DetectManualChallenge(ctx, request)
}

func (connection *rodBrowserConnection) Download(
	ctx context.Context,
	request DownloadRequest,
) (CapturedDownload, error) {
	if err := connection.hydrateSession(ctx, downloadSession(request), request.PageID); err != nil {
		return CapturedDownload{}, err
	}
	return connection.worker.CaptureDownload(ctx, request)
}

func (connection *rodBrowserConnection) Close(ctx context.Context) error {
	if connection.worker == nil || connection.worker.browser == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- connection.worker.browser.Context(ctx).Close() }()
	timer := time.NewTimer(browserCloseTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (connection *rodBrowserConnection) hydrateSession(
	ctx context.Context,
	request SessionRequest,
	activePageID string,
) error {
	pages, err := connection.worker.browser.Context(ctx).Pages()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("list browser pages: %w", err)
	}
	persistentPages := make([]*rodPersistentPage, 0, len(pages))
	activeIndex := -1
	for index, page := range pages {
		info, err := pageInfo(ctx, page, request.ProfileName, request.SessionName, page.String())
		if err != nil {
			return err
		}
		if activePageID != "" && info.ID == activePageID {
			activeIndex = index
		}
		persistentPages = append(persistentPages, &rodPersistentPage{
			id:   info.ID,
			page: page.Context(context.Background()),
			info: info,
		})
	}
	if activeIndex == -1 && len(persistentPages) > 0 {
		activeIndex = 0
	}
	for index := range persistentPages {
		persistentPages[index].info.Active = index == activeIndex
	}

	connection.worker.mu.Lock()
	defer connection.worker.mu.Unlock()
	session := connection.worker.sessionLocked(request.ProfileName, request.SessionName)
	session.pages = persistentPages
	session.active = activeIndex
	return nil
}
