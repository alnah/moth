package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/alnah/moth/internal/content"
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

func (worker *rodWorker) Click(ctx context.Context, request InteractionRequest) error {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return err
	}
	element, err := page.Element(request.Selector)
	if err != nil {
		return fmt.Errorf("find click target: %w", err)
	}
	if err := element.Context(ctx).Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("click target: %w", err)
	}
	return nil
}

func (worker *rodWorker) Input(ctx context.Context, request InputRequest) error {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return err
	}
	element, err := page.Element(request.Selector)
	if err != nil {
		return fmt.Errorf("find input target: %w", err)
	}
	if err := element.Context(ctx).Input(request.Text); err != nil {
		return fmt.Errorf("input target: %w", err)
	}
	return nil
}

func (worker *rodWorker) Wait(ctx context.Context, request WaitRequest) error {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return err
	}
	if request.State == WaitVisible {
		element, err := page.Element(request.Selector)
		if err != nil {
			return fmt.Errorf("find wait target: %w", err)
		}
		if err := element.Context(ctx).WaitVisible(); err != nil {
			return fmt.Errorf("wait target visible: %w", err)
		}
		return nil
	}
	if err := page.WaitElementsMoreThan(request.Selector, 0); err != nil {
		return fmt.Errorf("wait target attached: %w", err)
	}
	return nil
}

func (worker *rodWorker) AccessibilityTree(
	ctx context.Context,
	request AccessibilityRequest,
) (AccessibilityTree, error) {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return AccessibilityTree{}, err
	}
	var depth *int
	if request.MaxDepth > 0 {
		depth = &request.MaxDepth
	}
	result, err := proto.AccessibilityGetFullAXTree{Depth: depth}.Call(page.Context(ctx))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return AccessibilityTree{}, ctxErr
		}
		return AccessibilityTree{}, fmt.Errorf("get accessibility tree: %w", err)
	}
	nodes := make([]AccessibilityNode, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		if node.Ignored {
			continue
		}
		nodes = append(nodes, AccessibilityNode{Role: axValueString(node.Role), Name: axValueString(node.Name)})
	}
	return AccessibilityTree{Nodes: nodes}, nil
}

func (worker *rodWorker) CaptureDownload(ctx context.Context, request DownloadRequest) (CapturedDownload, error) {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return CapturedDownload{}, err
	}
	tmpDir, err := os.MkdirTemp("", "moth-browser-download-*")
	if err != nil {
		return CapturedDownload{}, fmt.Errorf("create download directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	waitDownload := worker.browser.Context(ctx).WaitDownload(tmpDir)
	element, err := page.Element(request.Selector)
	if err != nil {
		return CapturedDownload{}, fmt.Errorf("find download target: %w", err)
	}
	clickErr := element.Context(ctx).Click(proto.InputMouseButtonLeft, 1)
	if clickErr != nil {
		return CapturedDownload{}, fmt.Errorf("click download target: %w", clickErr)
	}
	info := waitDownload()
	if info == nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return CapturedDownload{}, ctxErr
		}
		return CapturedDownload{}, errors.New("download did not start")
	}
	path := filepath.Join(tmpDir, info.GUID)
	data, err := os.ReadFile(path) //nolint:gosec // Browser download path is Rod's generated GUID under tmpDir.
	if err != nil {
		return CapturedDownload{}, fmt.Errorf("read download: %w", err)
	}
	return CapturedDownload{Bytes: data, ContentType: http.DetectContentType(data)}, nil
}

func (worker *rodWorker) ResponseMetadata(
	ctx context.Context,
	request ResponseMetadataRequest,
) (ResponseMetadata, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return ResponseMetadata{}, fmt.Errorf("create metadata request: %w", err)
	}
	//nolint:gosec // Browser metadata intentionally fetches the caller-provided URL.
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return ResponseMetadata{}, fmt.Errorf("fetch response metadata: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
	return ResponseMetadata{
		URL:         response.Request.URL.String(),
		Status:      response.StatusCode,
		ContentType: response.Header.Get("Content-Type"),
		Headers:     response.Header.Clone(),
	}, nil
}

func (worker *rodWorker) CapturePDF(ctx context.Context, request PDFRequest) ([]byte, error) {
	page, err := worker.newOperationPage(ctx, request.URL, nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = page.Close() }()
	reader, err := page.PDF(&proto.PagePrintToPDF{PrintBackground: true})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("capture pdf: %w", err)
	}
	defer func() { _ = reader.Close() }()
	pdf, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	return pdf, nil
}

func (worker *rodWorker) DetectManualChallenge(
	ctx context.Context,
	request ManualChallengeRequest,
) (ManualChallengeResult, error) {
	page, err := worker.selectedPage(ctx, request.ProfileName, request.SessionName, request.PageID)
	if err != nil {
		return ManualChallengeResult{}, err
	}
	htmlText, err := page.Context(ctx).HTML()
	if err != nil {
		return ManualChallengeResult{}, fmt.Errorf("read challenge page: %w", err)
	}
	item, err := extractPageItem(LoadedPage{URL: page.String(), HTML: htmlText})
	if err != nil {
		return ManualChallengeResult{}, err
	}
	lowerText := strings.ToLower(item.Text)
	if strings.Contains(lowerText, "captcha") || strings.Contains(lowerText, "verify you are human") {
		return ManualChallengeResult{
			ManualRequired: true,
			Kind:           "captcha",
			Warnings:       []content.Warning{content.WarningCaptchaPossible},
		}, nil
	}
	return ManualChallengeResult{Warnings: []content.Warning{}}, nil
}

func (worker *rodWorker) newPersistentPage(
	ctx context.Context,
	pageURL string,
	headers map[string]string,
	userAgent string,
) (*rod.Page, error) {
	page, err := worker.browser.Context(ctx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("create persistent page: %w", err)
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
		return nil, fmt.Errorf("navigate persistent page: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		_ = page.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("wait persistent page load: %w", err)
	}
	return page.Context(context.Background()), nil
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

func axValueString(value *proto.AccessibilityAXValue) string {
	if value == nil {
		return ""
	}
	return value.Value.Str()
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
