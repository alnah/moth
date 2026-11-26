package browser

import (
	"context"
	"errors"
)

// OpenPage opens a page in the stored persistent browser and records it active.
func (service *PersistentService) OpenPage(ctx context.Context, request OpenPageRequest) (PageInfo, error) {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return PageInfo{}, err
	}
	now := service.clock()
	record := service.ensureBrowser(state.Scope, now, state.Owned)
	key := newPersistentSessionKey(state.Scope, request.ProfileName, request.SessionName)
	if _, _, syncErr := service.syncSessionPages(ctx, state.Scope, connection, key, now); syncErr != nil {
		return PageInfo{}, syncErr
	}
	service.ensureSession(record, key, now)

	page, err := connection.OpenPage(ctx, request)
	if err != nil {
		return PageInfo{}, err
	}
	service.recordPage(state.Scope, key, page, now)
	if err := service.enforcePageLimit(ctx, connection, key, page.ID, now); err != nil {
		return PageInfo{}, err
	}
	state.ActivePageID = page.ID
	if err := service.saveState(state); err != nil {
		return PageInfo{}, err
	}
	return page, nil
}

// ListPages lists pages from the stored persistent browser.
func (service *PersistentService) ListPages(ctx context.Context, request SessionRequest) ([]PageInfo, error) {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return nil, err
	}
	now := service.clock()
	service.ensureBrowser(state.Scope, now, state.Owned)
	key := newPersistentSessionKey(state.Scope, request.ProfileName, request.SessionName)
	pages, err := connection.ListPages(ctx, request)
	if err != nil {
		return nil, err
	}
	if _, _, syncErr := service.syncSessionPages(ctx, state.Scope, connection, key, now); syncErr != nil {
		return nil, syncErr
	}
	if len(pages) > 0 {
		service.touchSession(state.Scope, key, now)
	}
	service.touchBrowser(state.Scope, now, state.Owned)
	service.markActivePage(pages, state.ActivePageID)
	if active := activePageID(pages); active != "" && active != state.ActivePageID {
		state.ActivePageID = active
		if err := service.saveState(state); err != nil {
			return nil, err
		}
	}
	return pages, nil
}

// SwitchPage records a stored persistent browser page as active.
func (service *PersistentService) SwitchPage(ctx context.Context, request PageSelection) (PageInfo, error) {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return PageInfo{}, err
	}
	now := service.clock()
	service.ensureBrowser(state.Scope, now, state.Owned)
	key := newPersistentSessionKey(state.Scope, request.ProfileName, request.SessionName)
	if _, _, syncErr := service.syncSessionPages(ctx, state.Scope, connection, key, now); syncErr != nil {
		return PageInfo{}, syncErr
	}
	page, err := connection.SwitchPage(ctx, request)
	if err != nil {
		return PageInfo{}, err
	}
	service.recordPage(state.Scope, key, page, now)
	state.ActivePageID = page.ID
	if err := service.saveState(state); err != nil {
		return PageInfo{}, err
	}
	return page, nil
}

// ClosePage closes a stored persistent browser page and updates active state.
func (service *PersistentService) ClosePage(ctx context.Context, request PageSelection) error {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return err
	}
	now := service.clock()
	service.ensureBrowser(state.Scope, now, state.Owned)
	key := newPersistentSessionKey(state.Scope, request.ProfileName, request.SessionName)
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	if closeErr := connection.ClosePage(ctx, request); closeErr != nil {
		return closeErr
	}
	service.removePage(state.Scope, key, request.PageID)
	service.touchBrowser(state.Scope, now, state.Owned)
	pages, err := connection.ListPages(ctx, selectionSession(request))
	if err != nil {
		return err
	}
	state.ActivePageID = activePageID(pages)
	return service.saveState(state)
}

// Click clicks a selector on the active or selected persistent page.
func (service *PersistentService) Click(ctx context.Context, request InteractionRequest) error {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return err
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	if err := connection.Click(ctx, request); err != nil {
		return err
	}
	service.touchRequestPage(state, request.ProfileName, request.SessionName, request.PageID)
	return nil
}

// Input types text into a selector on the active or selected persistent page.
func (service *PersistentService) Input(ctx context.Context, request InputRequest) error {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return err
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	if err := connection.Input(ctx, request); err != nil {
		return err
	}
	service.touchRequestPage(state, request.ProfileName, request.SessionName, request.PageID)
	return nil
}

// Wait waits for a selector on the active or selected persistent page.
func (service *PersistentService) Wait(ctx context.Context, request WaitRequest) error {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return err
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	if err := connection.Wait(ctx, request); err != nil {
		return err
	}
	service.touchRequestPage(state, request.ProfileName, request.SessionName, request.PageID)
	return nil
}

// AccessibilityTree extracts accessibility data from the active or selected persistent page.
func (service *PersistentService) AccessibilityTree(
	ctx context.Context,
	request AccessibilityRequest,
) (AccessibilityTree, error) {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return AccessibilityTree{}, err
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	tree, err := connection.AccessibilityTree(ctx, request)
	if err != nil {
		return AccessibilityTree{}, err
	}
	service.touchRequestPage(state, request.ProfileName, request.SessionName, request.PageID)
	return tree, nil
}

// DetectManualChallenge reports manual challenge state for the active or selected persistent page.
func (service *PersistentService) DetectManualChallenge(
	ctx context.Context,
	request ManualChallengeRequest,
) (ManualChallengeResult, error) {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return ManualChallengeResult{}, err
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	result, err := connection.DetectManualChallenge(ctx, request)
	if err != nil {
		return ManualChallengeResult{}, err
	}
	service.touchRequestPage(state, request.ProfileName, request.SessionName, request.PageID)
	return result, nil
}

// Download captures a download from the active or selected persistent page.
func (service *PersistentService) Download(ctx context.Context, request DownloadRequest) (CapturedDownload, error) {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return CapturedDownload{}, err
	}
	downloader, ok := connection.(persistentDownloadConnection)
	if !ok {
		return CapturedDownload{}, errors.New("persistent browser does not support downloads")
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	captured, err := downloader.Download(ctx, request)
	if err != nil {
		return CapturedDownload{}, err
	}
	service.touchRequestPage(state, request.ProfileName, request.SessionName, request.PageID)
	if captured.Path == request.Path {
		return captured, nil
	}
	data, err := downloadBytes(captured.Bytes)
	if err != nil {
		return CapturedDownload{}, err
	}
	if err := writeBrowserFile(request.Path, data, "download"); err != nil {
		return CapturedDownload{}, err
	}
	return CapturedDownload{Path: request.Path, Bytes: int64(len(data)), ContentType: captured.ContentType}, nil
}
