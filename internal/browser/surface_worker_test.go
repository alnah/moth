package browser

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/content"
)

func newSurfacePool(worker *surfaceWorker) *Pool {
	return NewPool(1, WithWorkerFactory(func(context.Context) (Worker, error) { return worker, nil }))
}

func openPersistentPage(ctx context.Context, t *testing.T, pool *Pool, request OpenPageRequest) PageInfo {
	t.Helper()
	page, err := pool.OpenPage(ctx, request)
	if err != nil {
		t.Fatalf("OpenPage(%s/%s %s) error = %v, want nil", request.ProfileName, request.SessionName, request.URL, err)
	}
	if page.ID == "" {
		t.Fatalf("OpenPage(%s) returned empty page ID", request.URL)
	}
	return page
}

func assertActivePage(t *testing.T, pages []PageInfo, wantID string) {
	t.Helper()
	active := []string{}
	for _, page := range pages {
		if page.Active {
			active = append(active, page.ID)
		}
	}
	if len(active) != 1 || active[0] != wantID {
		t.Fatalf("active pages = %#v, want [%s] in %#v", active, wantID, pages)
	}
}

func assertPageIDs(t *testing.T, pages []PageInfo, want []string) {
	t.Helper()
	got := make([]string, 0, len(pages))
	for _, page := range pages {
		got = append(got, page.ID)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("page IDs mismatch (-want +got):\n%s", diff)
	}
}

func assertSurfaceActions(t *testing.T, got []surfaceAction, want []surfaceAction) {
	t.Helper()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("surface actions mismatch (-want +got):\n%s", diff)
	}
}

func hasWarning(warnings []content.Warning, want content.Warning) bool {
	for _, warning := range warnings {
		if warning == want {
			return true
		}
	}
	return false
}

type surfaceAction struct {
	Kind     string
	PageID   string
	Selector string
	Text     string
}

type surfaceWorker struct {
	fakeWorker

	mu        sync.Mutex
	nextID    int
	sessions  map[string]*surfaceSession
	actionLog []surfaceAction

	accessibility      AccessibilityTree
	blockAccessibility chan struct{}

	downloadPayload     []byte
	downloadValue       any
	downloadContentType string
	blockDownload       chan struct{}

	response   ResponseMetadata
	pdfPayload []byte
	challenge  ManualChallengeResult
}

type surfaceSession struct {
	pages  []PageInfo
	active int
}

func newSurfaceWorker() *surfaceWorker {
	return &surfaceWorker{
		sessions:            make(map[string]*surfaceSession),
		downloadPayload:     []byte("download"),
		downloadContentType: "application/octet-stream",
		pdfPayload:          []byte("%PDF-1.7"),
	}
}

func (worker *surfaceWorker) OpenPersistentPage(_ context.Context, request OpenPageRequest) (PageInfo, error) {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	worker.nextID++
	page := PageInfo{
		ID:          request.ProfileName + ":" + request.SessionName + ":" + strconv.Itoa(worker.nextID),
		URL:         request.URL,
		Title:       "Page " + request.URL,
		Active:      true,
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
	}
	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	for index := range session.pages {
		session.pages[index].Active = false
	}
	session.pages = append(session.pages, page)
	session.active = len(session.pages) - 1
	return page, nil
}

func (worker *surfaceWorker) ListPersistentPages(_ context.Context, request SessionRequest) ([]PageInfo, error) {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	return append([]PageInfo(nil), session.pages...), nil
}

func (worker *surfaceWorker) SwitchPersistentPage(_ context.Context, request PageSelection) (PageInfo, error) {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	index := worker.selectedIndexLocked(session, request.PageID)
	for pageIndex := range session.pages {
		session.pages[pageIndex].Active = pageIndex == index
	}
	session.active = index
	return session.pages[index], nil
}

func (worker *surfaceWorker) ClosePersistentPage(_ context.Context, request PageSelection) error {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	session := worker.sessionLocked(request.ProfileName, request.SessionName)
	index := worker.selectedIndexLocked(session, request.PageID)
	session.pages = append(session.pages[:index], session.pages[index+1:]...)
	if len(session.pages) == 0 {
		session.active = -1
		return nil
	}
	if index >= len(session.pages) {
		index = len(session.pages) - 1
	}
	for pageIndex := range session.pages {
		session.pages[pageIndex].Active = pageIndex == index
	}
	session.active = index
	return nil
}

func (worker *surfaceWorker) Click(_ context.Context, request InteractionRequest) error {
	worker.recordAction("click", request.ProfileName, request.SessionName, request.PageID, request.Selector, "")
	return nil
}

func (worker *surfaceWorker) Input(_ context.Context, request InputRequest) error {
	worker.recordAction("input", request.ProfileName, request.SessionName, request.PageID, request.Selector, request.Text)
	return nil
}

func (worker *surfaceWorker) Wait(_ context.Context, request WaitRequest) error {
	worker.recordAction("wait", request.ProfileName, request.SessionName, request.PageID, request.Selector, "")
	return nil
}

func (worker *surfaceWorker) AccessibilityTree(ctx context.Context, _ AccessibilityRequest) (AccessibilityTree, error) {
	if worker.blockAccessibility != nil {
		select {
		case <-worker.blockAccessibility:
		case <-ctx.Done():
			return AccessibilityTree{}, ctx.Err()
		}
	}
	return worker.accessibility, nil
}

func (worker *surfaceWorker) CaptureDownload(ctx context.Context, _ DownloadRequest) (CapturedDownload, error) {
	if worker.blockDownload != nil {
		select {
		case <-worker.blockDownload:
		case <-ctx.Done():
			return CapturedDownload{}, ctx.Err()
		}
	}
	value := any(worker.downloadPayload)
	if worker.downloadValue != nil {
		value = worker.downloadValue
	}
	return CapturedDownload{Bytes: value, ContentType: worker.downloadContentType}, nil
}

func (worker *surfaceWorker) ResponseMetadata(context.Context, ResponseMetadataRequest) (ResponseMetadata, error) {
	return worker.response, nil
}

func (worker *surfaceWorker) CapturePDF(context.Context, PDFRequest) ([]byte, error) {
	return worker.pdfPayload, nil
}

func (worker *surfaceWorker) DetectManualChallenge(
	context.Context,
	ManualChallengeRequest,
) (ManualChallengeResult, error) {
	return worker.challenge, nil
}

func (worker *surfaceWorker) actions() []surfaceAction {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	return append([]surfaceAction(nil), worker.actionLog...)
}

func (worker *surfaceWorker) recordAction(kind, profileName, sessionName, pageID, selector, text string) {
	worker.mu.Lock()
	defer worker.mu.Unlock()
	session := worker.sessionLocked(profileName, sessionName)
	if pageID == "" && session.active >= 0 && session.active < len(session.pages) {
		pageID = session.pages[session.active].ID
	}
	worker.actionLog = append(worker.actionLog, surfaceAction{Kind: kind, PageID: pageID, Selector: selector, Text: text})
}

func (worker *surfaceWorker) sessionLocked(profileName, sessionName string) *surfaceSession {
	key := profileName + "\x00" + sessionName
	session := worker.sessions[key]
	if session == nil {
		session = &surfaceSession{active: -1}
		worker.sessions[key] = session
	}
	return session
}

func (worker *surfaceWorker) selectedIndexLocked(session *surfaceSession, pageID string) int {
	if pageID == "" {
		return session.active
	}
	for index, page := range session.pages {
		if page.ID == pageID {
			return index
		}
	}
	return session.active
}
