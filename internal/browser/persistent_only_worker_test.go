package browser

import (
	"context"
	"errors"
)

type persistentOnlyWorker struct {
	fakeWorker
	openErr error
	pages   []PageInfo
}

func (worker *persistentOnlyWorker) OpenPersistentPage(
	_ context.Context,
	request OpenPageRequest,
) (PageInfo, error) {
	if worker.openErr != nil {
		return PageInfo{}, worker.openErr
	}
	page := PageInfo{
		ID:          request.SessionName + "-page",
		URL:         request.URL,
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		Active:      true,
	}
	for index := range worker.pages {
		worker.pages[index].Active = false
	}
	worker.pages = append(worker.pages, page)
	return page, nil
}

func (worker *persistentOnlyWorker) ListPersistentPages(
	_ context.Context,
	_ SessionRequest,
) ([]PageInfo, error) {
	return append([]PageInfo(nil), worker.pages...), nil
}

func (worker *persistentOnlyWorker) SwitchPersistentPage(
	_ context.Context,
	request PageSelection,
) (PageInfo, error) {
	for index := range worker.pages {
		if request.PageID != "" && worker.pages[index].ID != request.PageID {
			continue
		}
		for pageIndex := range worker.pages {
			worker.pages[pageIndex].Active = pageIndex == index
		}
		return worker.pages[index], nil
	}
	return PageInfo{}, errors.New("persistent page not found")
}

func (worker *persistentOnlyWorker) ClosePersistentPage(_ context.Context, request PageSelection) error {
	for index := range worker.pages {
		if request.PageID != "" && worker.pages[index].ID != request.PageID {
			continue
		}
		worker.pages = append(worker.pages[:index], worker.pages[index+1:]...)
		if len(worker.pages) > 0 {
			worker.pages[0].Active = true
		}
		return nil
	}
	return errors.New("persistent page not found")
}
