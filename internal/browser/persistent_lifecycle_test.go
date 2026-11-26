package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycleOptionsDefaults(t *testing.T) {
	options := (LifecycleOptions{}).withDefaults()

	if options.BrowserIdle != 15*time.Minute {
		t.Fatalf("BrowserIdle = %s, want %s", options.BrowserIdle, 15*time.Minute)
	}
	if options.SessionIdle != 15*time.Minute {
		t.Fatalf("SessionIdle = %s, want %s", options.SessionIdle, 15*time.Minute)
	}
	if options.SessionMax != 2*time.Hour {
		t.Fatalf("SessionMax = %s, want %s", options.SessionMax, 2*time.Hour)
	}
	if options.PageIdle != 30*time.Minute {
		t.Fatalf("PageIdle = %s, want %s", options.PageIdle, 30*time.Minute)
	}
	if options.MaxPages != 8 {
		t.Fatalf("MaxPages = %d, want 8", options.MaxPages)
	}
}

func TestOpenPageKeepsAtMostEightPages(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	browser.pages = pageRange(8)
	service, _, _ := newLifecycleService(t, clock, lifecycleTestOptions(), browser, true)

	page, err := service.OpenPage(ctx, OpenPageRequest{URL: "https://example.test/nine"})
	if err != nil {
		t.Fatalf("OpenPage(ninth) error = %v, want nil", err)
	}
	if page.ID != "page-9" {
		t.Fatalf("OpenPage(ninth).ID = %q, want page-9", page.ID)
	}
	assertLifecyclePageIDs(t, browser.pages, pageIDRange(2, 9))
}

func TestOpenPageKeepsExistingPagesWhenOpenFails(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	browser.pages = pageRange(8)
	browser.openErr = errors.New("navigation failed")
	service, _, _ := newLifecycleService(t, clock, lifecycleTestOptions(), browser, true)

	if _, err := service.OpenPage(ctx, OpenPageRequest{URL: "https://example.test/bad"}); err == nil {
		t.Fatal("OpenPage(failed navigation) error = nil, want error")
	}
	assertLifecyclePageIDs(t, browser.pages, pageIDRange(1, 8))
}

func TestOpenPageRollsBackNewPageWhenEvictionFails(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	browser.pages = pageRange(8)
	browser.closeErrors = map[string]error{"page-1": errors.New("close failed")}
	service, _, _ := newLifecycleService(t, clock, lifecycleTestOptions(), browser, true)

	if _, err := service.OpenPage(ctx, OpenPageRequest{URL: "https://example.test/nine"}); err == nil {
		t.Fatal("OpenPage(eviction failure) error = nil, want error")
	}
	assertLifecyclePageIDs(t, browser.pages, pageIDRange(1, 8))
}

func TestReapClosesIdlePageWithoutClosingBrowser(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	browser.pages = pageRange(2)
	options := lifecycleTestOptions()
	options.BrowserIdle = time.Hour
	options.SessionIdle = time.Hour
	options.PageIdle = 30 * time.Minute
	service, _, _ := newLifecycleService(t, clock, options, browser, true)

	if _, err := service.ListPages(ctx, SessionRequest{}); err != nil {
		t.Fatalf("ListPages(initial) error = %v, want nil", err)
	}
	clock.Advance(31 * time.Minute)
	if _, err := service.SwitchPage(ctx, PageSelection{PageID: "page-2"}); err != nil {
		t.Fatalf("SwitchPage(page-2) error = %v, want nil", err)
	}

	if _, err := service.Reap(ctx, clock.Now()); err != nil {
		t.Fatalf("Reap(idle page) error = %v, want nil", err)
	}
	assertLifecyclePageIDs(t, browser.pages, []string{"page-2"})
	if browser.closed {
		t.Fatal("Reap(idle page) closed browser, want page only")
	}
}

func TestReapClosesIdleSessionPagesButKeepsOtherSession(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	options := lifecycleTestOptions()
	options.BrowserIdle = time.Hour
	options.SessionIdle = 15 * time.Minute
	options.PageIdle = time.Hour
	service, _, _ := newLifecycleService(t, clock, options, browser, true)

	if _, err := service.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "idle",
		URL:         "https://example.test/idle",
	}); err != nil {
		t.Fatalf("OpenPage(idle session) error = %v, want nil", err)
	}
	if _, err := service.OpenPage(ctx, OpenPageRequest{
		ProfileName: "research",
		SessionName: "active",
		URL:         "https://example.test/active",
	}); err != nil {
		t.Fatalf("OpenPage(active session) error = %v, want nil", err)
	}

	clock.Advance(16 * time.Minute)
	if err := service.Click(ctx, InteractionRequest{
		ProfileName: "research",
		SessionName: "active",
		PageID:      "page-2",
		Selector:    "button",
	}); err != nil {
		t.Fatalf("Click(active session) error = %v, want nil", err)
	}
	if _, err := service.Reap(ctx, clock.Now()); err != nil {
		t.Fatalf("Reap(idle session) error = %v, want nil", err)
	}

	assertLifecyclePageIDs(t, browser.pages, []string{"page-2"})
	if browser.closed {
		t.Fatal("Reap(idle session) closed browser, want other session kept")
	}
}

func TestReapClosesSessionAtMaxAge(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	options := lifecycleTestOptions()
	options.BrowserIdle = 24 * time.Hour
	options.SessionIdle = 24 * time.Hour
	options.SessionMax = 2 * time.Hour
	options.PageIdle = 24 * time.Hour
	service, _, _ := newLifecycleService(t, clock, options, browser, true)

	if _, err := service.OpenPage(ctx, OpenPageRequest{URL: "https://example.test/old"}); err != nil {
		t.Fatalf("OpenPage() error = %v, want nil", err)
	}
	clock.Advance(2*time.Hour + time.Minute)
	if _, err := service.ListPages(ctx, SessionRequest{}); err != nil {
		t.Fatalf("ListPages(max age) error = %v, want nil", err)
	}
	if _, err := service.Reap(ctx, clock.Now()); err != nil {
		t.Fatalf("Reap(max age session) error = %v, want nil", err)
	}

	if len(browser.pages) != 0 {
		t.Fatalf("pages after max age = %#v, want none", browser.pages)
	}
	if browser.closed {
		t.Fatal("Reap(max age session) closed browser, want browser kept")
	}
}

func TestReapClosesIdleOwnedBrowser(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	stateDirs := newTestStateDirs(t)
	launcher := &fakePersistentLauncher{result: LaunchResult{
		DebugURL:  "ws://lifecycle",
		ChromePID: 4242,
	}}
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://lifecycle": browser.fakePersistentBrowser,
	})
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  launcher,
		Connector: connector,
		Clock:     clock.Now,
		Lifecycle: lifecycleDefaultOptions(),
	})

	if _, err := service.Start(ctx, StartRequest{Scope: "local"}); err != nil {
		t.Fatalf("Start(local) error = %v, want nil", err)
	}
	if _, err := service.ListPages(ctx, SessionRequest{}); err != nil {
		t.Fatalf("ListPages(started browser) error = %v, want nil", err)
	}
	clock.Advance(16 * time.Minute)
	if _, err := service.Reap(ctx, clock.Now()); err != nil {
		t.Fatalf("Reap(idle owned browser) error = %v, want nil", err)
	}

	if !browser.closed {
		t.Fatal("Reap(idle owned browser) did not close browser")
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, persistentStateFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state after idle browser reap = %v, want missing", err)
	}
	if got := service.ActiveBrowsers(); got != 0 {
		t.Fatalf("ActiveBrowsers() after idle owned browser = %d, want 0", got)
	}
}

func TestReapDisconnectsIdleExternalBrowser(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	options := lifecycleTestOptions()
	options.BrowserIdle = 15 * time.Minute
	service, stateDirs, _ := newLifecycleService(t, clock, options, browser, false)

	if _, err := service.ListPages(ctx, SessionRequest{}); err != nil {
		t.Fatalf("ListPages(external) error = %v, want nil", err)
	}
	clock.Advance(16 * time.Minute)
	if _, err := service.Reap(ctx, clock.Now()); err != nil {
		t.Fatalf("Reap(idle external browser) error = %v, want nil", err)
	}

	if !browser.disconnected {
		t.Fatal("Reap(idle external browser) did not disconnect transport")
	}
	if browser.closed {
		t.Fatal("Reap(idle external browser) closed external browser")
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, persistentStateFileName)); err != nil {
		t.Fatalf("state after idle external browser = %v, want kept", err)
	}
}

func TestReapDoesNotConnectMissingBrowser(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	service, _, connector := newLifecycleService(t, clock, lifecycleTestOptions(), browser, true)

	if _, err := service.Reap(ctx, clock.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("Reap(no cached browser) error = %v, want nil", err)
	}
	if len(connector.connected) != 0 {
		t.Fatalf("connector calls during Reap = %v, want none", connector.connected)
	}
}

func TestReapKeepsPageWhenCloseFails(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	browser.pages = pageRange(1)
	options := lifecycleTestOptions()
	options.PageIdle = 30 * time.Minute
	service, _, _ := newLifecycleService(t, clock, options, browser, true)

	if _, err := service.ListPages(ctx, SessionRequest{}); err != nil {
		t.Fatalf("ListPages(initial) error = %v, want nil", err)
	}
	browser.closeErrors = map[string]error{"page-1": errors.New("close failed")}
	clock.Advance(31 * time.Minute)
	if _, err := service.Reap(ctx, clock.Now()); err == nil {
		t.Fatal("Reap(close failure) error = nil, want error")
	}
	assertLifecyclePageIDs(t, browser.pages, []string{"page-1"})
}

func TestReapIsConservativeWhenListPagesFails(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	browser.pages = pageRange(1)
	service, _, _ := newLifecycleService(t, clock, lifecycleTestOptions(), browser, true)

	if _, err := service.ListPages(ctx, SessionRequest{}); err != nil {
		t.Fatalf("ListPages(initial) error = %v, want nil", err)
	}
	browser.listErr = errors.New("list failed")
	if _, err := service.Reap(ctx, clock.Now().Add(24*time.Hour)); err == nil {
		t.Fatal("Reap(list failure) error = nil, want error")
	}
	assertLifecyclePageIDs(t, browser.pages, []string{"page-1"})
}

func TestActiveBrowsersTracksStartedAndStoppedScopes(t *testing.T) {
	ctx := context.Background()
	clock := newLifecycleClock()
	browser := newLifecycleBrowser()
	stateDirs := newTestStateDirs(t)
	launcher := &fakePersistentLauncher{result: LaunchResult{
		DebugURL:  "ws://lifecycle",
		ChromePID: 4242,
	}}
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://lifecycle": browser.fakePersistentBrowser,
	})
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  launcher,
		Connector: connector,
		Clock:     clock.Now,
		Lifecycle: lifecycleDefaultOptions(),
	})

	if _, err := service.Start(ctx, StartRequest{Scope: "local"}); err != nil {
		t.Fatalf("Start(local) error = %v, want nil", err)
	}
	if got := service.ActiveBrowsers(); got != 1 {
		t.Fatalf("ActiveBrowsers() after local start = %d, want 1", got)
	}
	if _, err := service.Start(ctx, StartRequest{Scope: "global"}); err != nil {
		t.Fatalf("Start(global) error = %v, want nil", err)
	}
	if got := service.ActiveBrowsers(); got != 2 {
		t.Fatalf("ActiveBrowsers() after global start = %d, want 2", got)
	}
	if _, err := service.Stop(ctx, StopRequest{Scope: "local"}); err != nil {
		t.Fatalf("Stop(local) error = %v, want nil", err)
	}
	if got := service.ActiveBrowsers(); got != 1 {
		t.Fatalf("ActiveBrowsers() after local stop = %d, want 1", got)
	}
}

func lifecycleDefaultOptions() LifecycleOptions {
	return LifecycleOptions{
		BrowserIdle: 15 * time.Minute,
		SessionIdle: 15 * time.Minute,
		SessionMax:  2 * time.Hour,
		PageIdle:    30 * time.Minute,
		MaxPages:    8,
	}
}

func lifecycleTestOptions() LifecycleOptions {
	options := lifecycleDefaultOptions()
	options.BrowserIdle = 24 * time.Hour
	options.SessionIdle = 24 * time.Hour
	options.SessionMax = 24 * time.Hour
	options.PageIdle = 24 * time.Hour
	return options
}

type lifecycleConnector struct {
	browsers  map[string]*lifecycleBrowser
	failures  map[string]error
	connected []string
}

func newLifecycleConnector(browsers map[string]*lifecycleBrowser) *lifecycleConnector {
	return &lifecycleConnector{
		browsers: browsers,
		failures: make(map[string]error),
	}
}

func (connector *lifecycleConnector) ConnectBrowser(
	ctx context.Context,
	debugURL string,
) (BrowserConnection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	connector.connected = append(connector.connected, debugURL)
	if err := connector.failures[debugURL]; err != nil {
		return nil, err
	}
	browser, ok := connector.browsers[debugURL]
	if !ok {
		return nil, fmt.Errorf("unexpected debug URL %q", debugURL)
	}
	return browser, nil
}

type lifecycleClock struct {
	now time.Time
}

func newLifecycleClock() *lifecycleClock {
	return &lifecycleClock{now: fixedPersistentClock()}
}

func (clock *lifecycleClock) Now() time.Time {
	return clock.now
}

func (clock *lifecycleClock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
}

type lifecycleBrowser struct {
	*fakePersistentBrowser
	openErr      error
	listErr      error
	closeErrors  map[string]error
	disconnected bool
	sessions     map[persistentSessionKey][]string
}

func newLifecycleBrowser() *lifecycleBrowser {
	return &lifecycleBrowser{
		fakePersistentBrowser: newFakePersistentBrowser(),
		sessions:              make(map[persistentSessionKey][]string),
	}
}

func (browser *lifecycleBrowser) OpenPage(
	ctx context.Context,
	request OpenPageRequest,
) (PageInfo, error) {
	if browser.openErr != nil {
		return PageInfo{}, browser.openErr
	}
	page, err := browser.fakePersistentBrowser.OpenPage(ctx, request)
	if err != nil {
		return PageInfo{}, err
	}
	key := persistentSessionKey{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
	}
	browser.sessions[key] = append(browser.sessions[key], page.ID)
	return page, nil
}

func (browser *lifecycleBrowser) ListPages(
	ctx context.Context,
	request SessionRequest,
) ([]PageInfo, error) {
	if browser.listErr != nil {
		return nil, browser.listErr
	}
	key := persistentSessionKey{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
	}
	ids := browser.sessions[key]
	if len(ids) == 0 {
		return []PageInfo{}, nil
	}
	pages := make([]PageInfo, 0, len(ids))
	for _, page := range browser.pages {
		for _, id := range ids {
			if page.ID == id {
				pages = append(pages, page)
				break
			}
		}
	}
	return pages, nil
}

func (browser *lifecycleBrowser) ClosePage(
	ctx context.Context,
	request PageSelection,
) error {
	if err := browser.closeErrors[request.PageID]; err != nil {
		return err
	}
	key := persistentSessionKey{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
	}
	if err := browser.fakePersistentBrowser.ClosePage(ctx, request); err != nil {
		return err
	}
	ids := browser.sessions[key]
	for index, id := range ids {
		if id == request.PageID {
			browser.sessions[key] = append(ids[:index], ids[index+1:]...)
			break
		}
	}
	return nil
}

func (browser *lifecycleBrowser) Ping(context.Context) error {
	return nil
}

func (browser *lifecycleBrowser) Disconnect(context.Context) error {
	browser.disconnected = true
	return nil
}

func newLifecycleService(
	t *testing.T,
	clock *lifecycleClock,
	options LifecycleOptions,
	browser *lifecycleBrowser,
	owned bool,
) (*PersistentService, StateDirs, *lifecycleConnector) {
	t.Helper()

	stateDirs := newTestStateDirs(t)
	connector := newLifecycleConnector(map[string]*lifecycleBrowser{
		"ws://lifecycle": browser,
	})
	if len(browser.sessions) == 0 && len(browser.pages) > 0 {
		key := persistentSessionKey{}
		for _, page := range browser.pages {
			browser.sessions[key] = append(browser.sessions[key], page.ID)
		}
	}
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Connector: connector,
		Clock:     clock.Now,
		Lifecycle: options,
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url": "ws://lifecycle",
		"owned":     owned,
	})
	return service, stateDirs, connector
}

func pageRange(last int) []PageInfo {
	pages := make([]PageInfo, 0, last)
	for id := 1; id <= last; id++ {
		pages = append(pages, PageInfo{
			ID:     fmt.Sprintf("page-%d", id),
			URL:    fmt.Sprintf("https://example.test/%d", id),
			Active: id == last,
		})
	}
	return pages
}

func pageIDRange(first int, last int) []string {
	ids := make([]string, 0, last-first+1)
	for id := first; id <= last; id++ {
		ids = append(ids, fmt.Sprintf("page-%d", id))
	}
	return ids
}

func assertLifecyclePageIDs(t *testing.T, pages []PageInfo, want []string) {
	t.Helper()

	got := make([]string, 0, len(pages))
	for _, page := range pages {
		got = append(got, page.ID)
	}
	if len(got) != len(want) {
		t.Fatalf("page IDs = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("page IDs = %v, want %v", got, want)
		}
	}
}
