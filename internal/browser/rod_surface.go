package browser

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-rod/rod"
)

type rodSession struct {
	pages  []*rodPersistentPage
	active int
}

type rodPersistentPage struct {
	id   string
	page *rod.Page
	info PageInfo
}

func (worker *rodWorker) OpenPersistentPage(ctx context.Context, request OpenPageRequest) (PageInfo, error) {
	page, err := worker.newPersistentPage(ctx, request.URL, request.Headers, request.UserAgent)
	if err != nil {
		return PageInfo{}, err
	}

	info, err := pageInfo(ctx, page, request.ProfileName, request.SessionName, request.URL)
	if err != nil {
		_ = page.Close()
		return PageInfo{}, err
	}
	info.Active = true

	worker.mu.Lock()
	defer worker.mu.Unlock()
	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	for index := range session.pages {
		session.pages[index].info.Active = false
	}
	session.pages = append(session.pages, &rodPersistentPage{id: info.ID, page: page, info: info})
	session.active = len(session.pages) - 1
	return info, nil
}

func (worker *rodWorker) ListPersistentPages(_ context.Context, request SessionRequest) ([]PageInfo, error) {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	pages := make([]PageInfo, 0, len(session.pages))
	for _, page := range session.pages {
		pages = append(pages, page.info)
	}
	return pages, nil
}

func (worker *rodWorker) SwitchPersistentPage(ctx context.Context, request PageSelection) (PageInfo, error) {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	index, err := selectedRodPageIndex(session, request.PageID)
	if err != nil {
		return PageInfo{}, err
	}
	for pageIndex := range session.pages {
		session.pages[pageIndex].info.Active = pageIndex == index
	}
	session.active = index
	if _, err := session.pages[index].page.Context(ctx).Activate(); err != nil {
		return PageInfo{}, fmt.Errorf("activate page: %w", err)
	}
	return session.pages[index].info, nil
}

func (worker *rodWorker) ClosePersistentPage(_ context.Context, request PageSelection) error {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	index, err := selectedRodPageIndex(session, request.PageID)
	if err != nil {
		return err
	}
	closedPage := session.pages[index]
	session.pages = append(session.pages[:index], session.pages[index+1:]...)
	if err := closedPage.page.Close(); err != nil {
		return fmt.Errorf("close page: %w", err)
	}
	if len(session.pages) == 0 {
		session.active = -1
		return nil
	}
	if index >= len(session.pages) {
		index = len(session.pages) - 1
	}
	for pageIndex := range session.pages {
		session.pages[pageIndex].info.Active = pageIndex == index
	}
	session.active = index
	return nil
}

func (worker *rodWorker) selectedPage(ctx context.Context, profileName, sessionName, pageID string) (*rod.Page, error) {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	session := worker.sessionLocked(profileName, sessionName)
	index, err := selectedRodPageIndex(session, pageID)
	if err != nil {
		return nil, err
	}
	return session.pages[index].page.Context(ctx), nil
}

func (worker *rodWorker) sessionLocked(profileName string, sessionName string) *rodSession {
	key := profileName + "\x00" + sessionName
	session := worker.sessions[key]
	if session == nil {
		session = &rodSession{active: -1}
		worker.sessions[key] = session
	}
	return session
}

func selectedRodPageIndex(session *rodSession, pageID string) (int, error) {
	if pageID == "" {
		if session.active < 0 || session.active >= len(session.pages) {
			return -1, errors.New("no active page")
		}
		return session.active, nil
	}
	for index, page := range session.pages {
		if page.id == pageID {
			return index, nil
		}
	}
	return -1, fmt.Errorf("page %q not found", pageID)
}

func pageInfo(
	ctx context.Context,
	page *rod.Page,
	profileName string,
	sessionName string,
	fallbackURL string,
) (PageInfo, error) {
	target, err := page.Context(ctx).Info()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return PageInfo{}, ctxErr
		}
		return PageInfo{}, fmt.Errorf("read page info: %w", err)
	}
	pageURL := target.URL
	if pageURL == "" {
		pageURL = fallbackURL
	}
	return PageInfo{
		ID:          string(page.TargetID),
		URL:         pageURL,
		Title:       target.Title,
		ProfileName: profileName,
		SessionName: sessionName,
	}, nil
}

func (worker *rodWorker) closePersistentPages() error {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	errs := []error{}
	for _, session := range worker.sessions {
		for _, persistentPage := range session.pages {
			if err := persistentPage.page.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		session.pages = nil
		session.active = -1
	}
	return errors.Join(errs...)
}
