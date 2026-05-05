//nolint:gocyclo,gosec,govet,lll // Red tests use verbose fakes and broad behavior assertions.
package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/content"
)

func TestPersistentServiceStartIsIdempotentAndReplacesStaleState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	firstBrowser := newFakePersistentBrowser()
	secondBrowser := newFakePersistentBrowser()
	launcher := &fakePersistentLauncher{result: LaunchResult{DebugURL: "ws://owned-one", ChromePID: 4242}}
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://owned-one": firstBrowser,
		"ws://owned-two": secondBrowser,
	})
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  launcher,
		Connector: connector,
		Clock:     fixedPersistentClock,
	})

	started, err := service.Start(ctx, StartRequest{Scope: "local", Show: true})
	if err != nil {
		t.Fatalf("PersistentService.Start(first) error = %v, want nil", err)
	}
	assertBrowserStatus(t, started, BrowserStatus{
		Status:    "running",
		Scope:     "local",
		DebugURL:  "ws://owned-one",
		ChromePID: 4242,
		Owned:     true,
		DataDir:   filepath.Join(stateDirs.Local, "chrome-data"),
	})
	if launcher.calls != 1 {
		t.Fatalf("launcher calls after first Start = %d, want 1", launcher.calls)
	}
	if !launcher.requests[0].Show {
		t.Fatalf("launcher request Show = false, want true")
	}

	secondStart, err := service.Start(ctx, StartRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("PersistentService.Start(idempotent) error = %v, want nil", err)
	}
	if secondStart.DebugURL != "ws://owned-one" || launcher.calls != 1 {
		t.Fatalf("Start(idempotent) = %#v and launcher calls %d, want existing browser", secondStart, launcher.calls)
	}

	connector.failures["ws://owned-one"] = errors.New("browser went away")
	launcher.result = LaunchResult{DebugURL: "ws://owned-two", ChromePID: 5252}
	replaced, err := service.Start(ctx, StartRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("PersistentService.Start(replace stale) error = %v, want nil", err)
	}
	if replaced.DebugURL != "ws://owned-two" || replaced.ChromePID != 5252 {
		t.Fatalf("Start(replace stale) = %#v, want second launched browser", replaced)
	}
	if launcher.calls != 2 {
		t.Fatalf("launcher calls after stale replacement = %d, want 2", launcher.calls)
	}
	stored := readTestState(t, stateDirs.Local)
	if stored["debug_url"] != "ws://owned-two" || stored["owned"] != true {
		t.Fatalf("stored state = %#v, want replacement owned debug URL", stored)
	}
}

func TestPersistentServiceReopensStoredBrowserAndPersistsActivePages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	persistentBrowser := newFakePersistentBrowser()
	launcher := &fakePersistentLauncher{result: LaunchResult{DebugURL: "ws://owned-browser", ChromePID: 4242}}
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{"ws://owned-browser": persistentBrowser})

	firstService := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  launcher,
		Connector: connector,
		Clock:     fixedPersistentClock,
	})
	if _, err := firstService.Start(ctx, StartRequest{Scope: "local"}); err != nil {
		t.Fatalf("PersistentService.Start() error = %v, want nil", err)
	}
	firstPage, err := firstService.OpenPage(ctx, OpenPageRequest{URL: "https://example.test/one"})
	if err != nil {
		t.Fatalf("PersistentService.OpenPage(first) error = %v, want nil", err)
	}
	secondPage, err := firstService.OpenPage(ctx, OpenPageRequest{URL: "https://example.test/two"})
	if err != nil {
		t.Fatalf("PersistentService.OpenPage(second) error = %v, want nil", err)
	}
	if firstPage.ID == secondPage.ID || !secondPage.Active {
		t.Fatalf("OpenPage pages = %#v %#v, want distinct second active page", firstPage, secondPage)
	}

	secondService := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Connector: connector,
	})
	pages, err := secondService.ListPages(ctx, SessionRequest{})
	if err != nil {
		t.Fatalf("PersistentService.ListPages(reconstructed) error = %v, want nil", err)
	}
	assertPageIDs(t, pages, []string{firstPage.ID, secondPage.ID})
	assertActivePage(t, pages, secondPage.ID)

	switched, err := secondService.SwitchPage(ctx, PageSelection{PageID: firstPage.ID})
	if err != nil {
		t.Fatalf("PersistentService.SwitchPage() error = %v, want nil", err)
	}
	if switched.ID != firstPage.ID || !switched.Active {
		t.Fatalf("SwitchPage() = %#v, want first page active", switched)
	}

	if err := secondService.Input(ctx, InputRequest{Selector: "input[name=q]", Text: "moth"}); err != nil {
		t.Fatalf("PersistentService.Input(active) error = %v, want nil", err)
	}
	if err := secondService.Click(ctx, InteractionRequest{Selector: "button[type=submit]"}); err != nil {
		t.Fatalf("PersistentService.Click(active) error = %v, want nil", err)
	}
	if err := secondService.Wait(ctx, WaitRequest{Selector: "main.ready", State: WaitVisible}); err != nil {
		t.Fatalf("PersistentService.Wait(active) error = %v, want nil", err)
	}
	tree, err := secondService.AccessibilityTree(ctx, AccessibilityRequest{MaxDepth: 2})
	if err != nil {
		t.Fatalf("PersistentService.AccessibilityTree(active) error = %v, want nil", err)
	}
	if !reflect.DeepEqual(tree, persistentBrowser.tree) {
		t.Fatalf("AccessibilityTree() = %#v, want %#v", tree, persistentBrowser.tree)
	}
	challenge, err := secondService.DetectManualChallenge(ctx, ManualChallengeRequest{})
	if err != nil {
		t.Fatalf("PersistentService.DetectManualChallenge(active) error = %v, want nil", err)
	}
	if !challenge.ManualRequired || challenge.Kind != "captcha" {
		t.Fatalf("DetectManualChallenge() = %#v, want captcha challenge", challenge)
	}

	wantActions := []persistentBrowserAction{
		{Kind: "input", PageID: firstPage.ID, Selector: "input[name=q]", Text: "moth"},
		{Kind: "click", PageID: firstPage.ID, Selector: "button[type=submit]"},
		{Kind: "wait", PageID: firstPage.ID, Selector: "main.ready"},
		{Kind: "ax-tree", PageID: firstPage.ID},
		{Kind: "challenge", PageID: firstPage.ID},
	}
	if !reflect.DeepEqual(persistentBrowser.actions, wantActions) {
		t.Fatalf("persistent browser actions = %#v, want %#v", persistentBrowser.actions, wantActions)
	}

	stopped, err := secondService.Stop(ctx, StopRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("PersistentService.Stop(owned) error = %v, want nil", err)
	}
	if stopped.Status != "stopped" || !persistentBrowser.closed {
		t.Fatalf("Stop(owned) = %#v closed=%v, want stopped and browser closed", stopped, persistentBrowser.closed)
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state after Stop exists err = %v, want removed", err)
	}
}

func TestPersistentServiceClosePageUpdatesStoredActivePageAndDownloadsFromActivePage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	persistentBrowser := newFakePersistentBrowser()
	persistentBrowser.pages = []PageInfo{
		{ID: "page-1", URL: "https://example.test/one"},
		{ID: "page-2", URL: "https://example.test/two", Active: true},
	}
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{"ws://owned-browser": persistentBrowser})
	service := NewPersistentService(PersistentServiceOptions{StateDirs: stateDirs, Connector: connector})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":      "ws://owned-browser",
		"owned":          true,
		"active_page_id": "page-2",
	})

	if err := service.ClosePage(ctx, PageSelection{}); err != nil {
		t.Fatalf("PersistentService.ClosePage(active) error = %v, want nil", err)
	}
	if len(persistentBrowser.pages) != 1 || persistentBrowser.pages[0].ID != "page-1" || !persistentBrowser.pages[0].Active {
		t.Fatalf("pages after ClosePage(active) = %#v, want page-1 as active", persistentBrowser.pages)
	}
	stored := readTestState(t, stateDirs.Local)
	if stored["active_page_id"] != "page-1" {
		t.Fatalf("stored active page after ClosePage(active) = %#v, want page-1", stored["active_page_id"])
	}

	download, err := service.Download(ctx, DownloadRequest{Selector: "#archive", Path: "archive.zip"})
	if err != nil {
		t.Fatalf("PersistentService.Download(active) error = %v, want nil", err)
	}
	if download.Path != "archive.zip" || download.Bytes != int64(11) || download.ContentType != "application/zip" {
		t.Fatalf("Download(active) = %#v, want captured archive", download)
	}
	wantDownloads := []DownloadRequest{{PageID: "page-1", Selector: "#archive", Path: "archive.zip"}}
	if !reflect.DeepEqual(persistentBrowser.downloads, wantDownloads) {
		t.Fatalf("download requests = %#v, want %#v", persistentBrowser.downloads, wantDownloads)
	}
}

func TestPersistentServiceDownloadWritesRawBytesToRequestedPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	rawBytes := []byte("archive bytes")
	persistentBrowser := newFakePersistentBrowser()
	persistentBrowser.downloadResult = CapturedDownload{Bytes: rawBytes, ContentType: "application/zip"}
	persistentBrowser.hasDownloadResult = true
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{"ws://owned-browser": persistentBrowser})
	service := NewPersistentService(PersistentServiceOptions{StateDirs: stateDirs, Connector: connector})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":      "ws://owned-browser",
		"owned":          true,
		"active_page_id": "page-2",
	})
	downloadPath := filepath.Join(t.TempDir(), "archive.zip")

	download, err := service.Download(ctx, DownloadRequest{Selector: "#archive", Path: downloadPath})
	if err != nil {
		t.Fatalf("PersistentService.Download(raw bytes) error = %v, want nil", err)
	}
	written, err := os.ReadFile(downloadPath) //nolint:gosec // Test path is controlled under t.TempDir.
	if err != nil {
		t.Fatalf("read requested download path: %v", err)
	}
	if !bytes.Equal(written, rawBytes) {
		t.Fatalf("downloaded bytes = %q, want %q", written, rawBytes)
	}
	if download.Path != downloadPath {
		t.Fatalf("Download(raw bytes) path = %q, want %q", download.Path, downloadPath)
	}
	if download.Bytes != int64(len(rawBytes)) {
		t.Fatalf("Download(raw bytes) bytes = %#v, want %d", download.Bytes, len(rawBytes))
	}
	if download.ContentType != "application/zip" {
		t.Fatalf("Download(raw bytes) content type = %q, want application/zip", download.ContentType)
	}

	wantDownloads := []DownloadRequest{{PageID: "page-2", Selector: "#archive", Path: downloadPath}}
	if !reflect.DeepEqual(persistentBrowser.downloads, wantDownloads) {
		t.Fatalf("download requests = %#v, want %#v", persistentBrowser.downloads, wantDownloads)
	}
}

func TestPersistentServiceDelegatesStatelessOperationsAndReportsUnavailableCapabilities(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateless := &fakeStatelessBrowser{
		metadata: ResponseMetadata{
			URL:         "https://example.test/file.pdf",
			Status:      http.StatusOK,
			ContentType: "application/pdf",
		},
	}
	service := NewPersistentService(PersistentServiceOptions{Stateless: stateless})

	metadata, err := service.ResponseMetadata(ctx, ResponseMetadataRequest{URL: "https://example.test/file.pdf", MaxHeaderBytes: 64})
	if err != nil {
		t.Fatalf("PersistentService.ResponseMetadata() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(metadata, stateless.metadata) {
		t.Fatalf("ResponseMetadata() = %#v, want %#v", metadata, stateless.metadata)
	}
	if err := service.Screenshot(ctx, ScreenshotRequest{URL: "https://example.test", Path: "page.png"}); err != nil {
		t.Fatalf("PersistentService.Screenshot() error = %v, want nil", err)
	}
	if err := service.PDF(ctx, PDFRequest{URL: "https://example.test", Path: "page.pdf", MaxBytes: 4096}); err != nil {
		t.Fatalf("PersistentService.PDF() error = %v, want nil", err)
	}
	if stateless.metadataRequest.URL != "https://example.test/file.pdf" || stateless.screenshotRequest.Path != "page.png" ||
		stateless.pdfRequest.Path != "page.pdf" {
		t.Fatalf("stateless requests = %#v %#v %#v, want delegated operations", stateless.metadataRequest, stateless.screenshotRequest, stateless.pdfRequest)
	}

	unavailable := NewPersistentService(PersistentServiceOptions{})
	if _, err := unavailable.ResponseMetadata(ctx, ResponseMetadataRequest{}); err == nil || !strings.Contains(err.Error(), "metadata unavailable") {
		t.Fatalf("ResponseMetadata(unavailable) error = %v, want unavailable metadata error", err)
	}
	if err := unavailable.Screenshot(ctx, ScreenshotRequest{}); err == nil || !strings.Contains(err.Error(), "screenshot unavailable") {
		t.Fatalf("Screenshot(unavailable) error = %v, want unavailable screenshot error", err)
	}
	if err := unavailable.PDF(ctx, PDFRequest{}); err == nil || !strings.Contains(err.Error(), "pdf unavailable") {
		t.Fatalf("PDF(unavailable) error = %v, want unavailable pdf error", err)
	}
}

func TestDefaultStateDirsUseHomeMothBrowserForGlobalState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	stateDirs := defaultStateDirs()

	if stateDirs.Local != filepath.Join(".moth", "browser") {
		t.Fatalf("default local state dir = %q, want .moth/browser", stateDirs.Local)
	}
	wantGlobal := filepath.Join(home, ".moth", "browser")
	if stateDirs.Global != wantGlobal {
		t.Fatalf("default global state dir = %q, want %q", stateDirs.Global, wantGlobal)
	}
}

func TestPersistentServiceStartAutoWritesGlobalWhenNoLocalStateExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	launcher := &fakePersistentLauncher{result: LaunchResult{DebugURL: "ws://owned-auto", ChromePID: 4242}}
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  launcher,
		Connector: newFakePersistentConnector(nil),
		Clock:     fixedPersistentClock,
	})

	started, err := service.Start(ctx, StartRequest{Scope: "auto"})
	if err != nil {
		t.Fatalf("PersistentService.Start(auto) error = %v, want nil", err)
	}
	if started.Scope != "global" {
		t.Fatalf("Start(auto).Scope = %q, want global", started.Scope)
	}
	stored := readTestState(t, stateDirs.Global)
	if stored["debug_url"] != "ws://owned-auto" || stored["owned"] != true {
		t.Fatalf("global state = %#v, want owned auto browser", stored)
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local state exists err = %v, want absent", err)
	}
}

func TestPersistentServiceConnectAutoWritesGlobalWhenNoLocalStateExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	external := newFakePersistentBrowser()
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{"ws://external-auto": external})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": "ws://external-auto"})
	}))
	defer server.Close()
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs:  stateDirs,
		Connector:  connector,
		HTTPClient: server.Client(),
	})

	connected, err := service.Connect(ctx, ConnectRequest{Scope: "auto", HostPort: server.Listener.Addr().String()})
	if err != nil {
		t.Fatalf("PersistentService.Connect(auto) error = %v, want nil", err)
	}
	if connected.Scope != "global" || connected.DebugURL != "ws://external-auto" || connected.Owned {
		t.Fatalf("Connect(auto) = %#v, want global external browser", connected)
	}
	stored := readTestState(t, stateDirs.Global)
	if stored["debug_url"] != "ws://external-auto" || stored["owned"] != false {
		t.Fatalf("global state = %#v, want external auto browser", stored)
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local state exists err = %v, want absent", err)
	}
}

func TestPersistentServiceConnectRejectsOversizedVersionResponseWithoutStoringState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat(" ", maxDebuggerVersionBytes+1)))
	}))
	defer server.Close()
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs:  stateDirs,
		Connector:  newFakePersistentConnector(nil),
		HTTPClient: server.Client(),
	})

	_, err := service.Connect(ctx, ConnectRequest{Scope: "auto", HostPort: server.Listener.Addr().String()})
	if err == nil {
		t.Fatal("PersistentService.Connect(oversized version) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "response over") {
		t.Fatalf("Connect(oversized version) error = %v, want response over context", err)
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Global, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("global state exists err = %v, want absent", err)
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local state exists err = %v, want absent", err)
	}
}

func TestPersistentServiceConnectsExternalBrowserAndNeverClosesIt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	external := newFakePersistentBrowser()
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{"ws://external-browser": external})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			t.Fatalf("version endpoint path = %q, want /json/version", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": "ws://external-browser"})
	}))
	defer server.Close()

	service := NewPersistentService(PersistentServiceOptions{
		StateDirs:  stateDirs,
		Connector:  connector,
		HTTPClient: server.Client(),
	})
	connected, err := service.Connect(ctx, ConnectRequest{Scope: "local", HostPort: server.Listener.Addr().String()})
	if err != nil {
		t.Fatalf("PersistentService.Connect() error = %v, want nil", err)
	}
	if connected.DebugURL != "ws://external-browser" || connected.Owned {
		t.Fatalf("Connect() = %#v, want external unowned browser", connected)
	}

	stopped, err := service.Stop(ctx, StopRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("PersistentService.Stop(external) error = %v, want nil", err)
	}
	if stopped.Status != "stopped" || external.closed {
		t.Fatalf("Stop(external) = %#v closed=%v, want state cleared without closing browser", stopped, external.closed)
	}
}

func TestPersistentServiceAutoScopeUsesLocalStateBeforeGlobalState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	localBrowser := newFakePersistentBrowser()
	localBrowser.pages = []PageInfo{{ID: "local-page", URL: "https://local.test", Active: true}}
	globalBrowser := newFakePersistentBrowser()
	globalBrowser.pages = []PageInfo{{ID: "global-page", URL: "https://global.test", Active: true}}
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://local-browser":  localBrowser,
		"ws://global-browser": globalBrowser,
	})
	launcher := &fakePersistentLauncher{}
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  launcher,
		Connector: connector,
	})

	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":      "ws://local-browser",
		"chrome_pid":     1111,
		"owned":          true,
		"data_dir":       filepath.Join(stateDirs.Local, "chrome-data"),
		"active_page_id": "local-page",
	})
	writeTestState(t, stateDirs.Global, map[string]any{
		"debug_url":      "ws://global-browser",
		"chrome_pid":     2222,
		"owned":          true,
		"data_dir":       filepath.Join(stateDirs.Global, "chrome-data"),
		"active_page_id": "global-page",
	})

	localPages, err := service.ListPages(ctx, SessionRequest{})
	if err != nil {
		t.Fatalf("PersistentService.ListPages(auto with local state) error = %v, want nil", err)
	}
	assertPageIDs(t, localPages, []string{"local-page"})
	localStatus, err := service.Status(ctx, StatusRequest{Scope: "auto"})
	if err != nil {
		t.Fatalf("PersistentService.Status(auto with local state) error = %v, want nil", err)
	}
	if localStatus.Scope != "local" || localStatus.DebugURL != "ws://local-browser" {
		t.Fatalf("Status(auto with local state) = %#v, want local state", localStatus)
	}

	if err := os.Remove(filepath.Join(stateDirs.Local, "state.json")); err != nil {
		t.Fatalf("remove local state: %v", err)
	}
	globalPages, err := service.ListPages(ctx, SessionRequest{})
	if err != nil {
		t.Fatalf("PersistentService.ListPages(auto with global state) error = %v, want nil", err)
	}
	assertPageIDs(t, globalPages, []string{"global-page"})
	globalStatus, err := service.Status(ctx, StatusRequest{Scope: "auto"})
	if err != nil {
		t.Fatalf("PersistentService.Status(auto with global state) error = %v, want nil", err)
	}
	if globalStatus.Scope != "global" || globalStatus.DebugURL != "ws://global-browser" {
		t.Fatalf("Status(auto with global state) = %#v, want global state", globalStatus)
	}
}

func TestPersistentServiceStatusAndPageCommandsHandleMissingStaleAndCorruptState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stateDirs := newTestStateDirs(t)
	connector := newFakePersistentConnector(nil)
	service := NewPersistentService(PersistentServiceOptions{StateDirs: stateDirs, Connector: connector})

	missing, err := service.Status(ctx, StatusRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("PersistentService.Status(missing) error = %v, want nil", err)
	}
	if missing.Status != "missing" {
		t.Fatalf("Status(missing) = %#v, want missing status", missing)
	}
	_, err = service.OpenPage(ctx, OpenPageRequest{URL: "https://example.test"})
	if !errors.Is(err, ErrBrowserStateUnavailable) {
		t.Fatalf("OpenPage(missing state) error = %v, want ErrBrowserStateUnavailable", err)
	}

	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":      "ws://stale-browser",
		"chrome_pid":     5151,
		"owned":          true,
		"data_dir":       filepath.Join(stateDirs.Local, "chrome-data"),
		"active_page_id": "page-1",
	})
	connector.failures["ws://stale-browser"] = errors.New("connection refused")
	stale, err := service.Status(ctx, StatusRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("PersistentService.Status(stale) error = %v, want nil", err)
	}
	if stale.Status != "stale" || stale.ChromePID != 5151 {
		t.Fatalf("Status(stale) = %#v, want stale status with PID", stale)
	}
	if _, err := service.ListPages(ctx, SessionRequest{}); !errors.Is(err, ErrBrowserStateUnavailable) {
		t.Fatalf("ListPages(stale state) error = %v, want ErrBrowserStateUnavailable", err)
	}

	if err := os.WriteFile(filepath.Join(stateDirs.Local, "state.json"), []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}
	if _, err := service.Status(ctx, StatusRequest{Scope: "local"}); !errors.Is(err, ErrBrowserStateCorrupt) {
		t.Fatalf("Status(corrupt state) error = %v, want ErrBrowserStateCorrupt", err)
	}
	if _, err := service.AccessibilityTree(ctx, AccessibilityRequest{}); !errors.Is(err, ErrBrowserStateUnavailable) {
		t.Fatalf("AccessibilityTree(corrupt state) error = %v, want ErrBrowserStateUnavailable", err)
	}
}

func newTestStateDirs(t *testing.T) StateDirs {
	t.Helper()
	root := t.TempDir()
	return StateDirs{
		Local:  filepath.Join(root, "local", "browser"),
		Global: filepath.Join(root, "global", "browser"),
	}
}

func fixedPersistentClock() time.Time {
	return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
}

func assertBrowserStatus(t *testing.T, got BrowserStatus, want BrowserStatus) {
	t.Helper()
	if got.Status != want.Status || got.Scope != want.Scope || got.DebugURL != want.DebugURL ||
		got.ChromePID != want.ChromePID || got.Owned != want.Owned || got.DataDir != want.DataDir {
		t.Fatalf("browser status = %#v, want fields %#v", got, want)
	}
}

func writeTestState(t *testing.T, dir string, state map[string]any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("create test state dir: %v", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal test state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o600); err != nil {
		t.Fatalf("write test state: %v", err)
	}
}

func readTestState(t *testing.T, dir string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("read test state: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("decode test state %s: %v", data, err)
	}
	return state
}

type fakePersistentLauncher struct {
	result   LaunchResult
	calls    int
	requests []LaunchRequest
}

func (launcher *fakePersistentLauncher) LaunchBrowser(ctx context.Context, request LaunchRequest) (LaunchResult, error) {
	if err := ctx.Err(); err != nil {
		return LaunchResult{}, err
	}
	launcher.calls++
	launcher.requests = append(launcher.requests, request)
	return launcher.result, nil
}

type fakePersistentConnector struct {
	browsers  map[string]*fakePersistentBrowser
	failures  map[string]error
	connected []string
}

func newFakePersistentConnector(browsers map[string]*fakePersistentBrowser) *fakePersistentConnector {
	if browsers == nil {
		browsers = map[string]*fakePersistentBrowser{}
	}
	return &fakePersistentConnector{browsers: browsers, failures: map[string]error{}}
}

func (connector *fakePersistentConnector) ConnectBrowser(ctx context.Context, debugURL string) (BrowserConnection, error) {
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

type fakePersistentBrowser struct {
	pages             []PageInfo
	tree              AccessibilityTree
	challenge         ManualChallengeResult
	actions           []persistentBrowserAction
	downloads         []DownloadRequest
	downloadResult    CapturedDownload
	hasDownloadResult bool
	closed            bool
}

type persistentBrowserAction struct {
	Kind     string
	PageID   string
	Selector string
	Text     string
}

func newFakePersistentBrowser() *fakePersistentBrowser {
	return &fakePersistentBrowser{
		tree: AccessibilityTree{Nodes: []AccessibilityNode{{Role: "button", Name: "Submit"}}},
		challenge: ManualChallengeResult{
			ManualRequired: true,
			Kind:           "captcha",
			Warnings:       []content.Warning{content.WarningCaptchaPossible},
		},
	}
}

func (browser *fakePersistentBrowser) OpenPage(_ context.Context, request OpenPageRequest) (PageInfo, error) {
	for index := range browser.pages {
		browser.pages[index].Active = false
	}
	page := PageInfo{
		ID:     fmt.Sprintf("page-%d", len(browser.pages)+1),
		URL:    request.URL,
		Title:  fmt.Sprintf("Title %d", len(browser.pages)+1),
		Active: true,
	}
	browser.pages = append(browser.pages, page)
	return page, nil
}

func (browser *fakePersistentBrowser) ListPages(context.Context, SessionRequest) ([]PageInfo, error) {
	return append([]PageInfo(nil), browser.pages...), nil
}

func (browser *fakePersistentBrowser) SwitchPage(_ context.Context, request PageSelection) (PageInfo, error) {
	selected := browser.selectedPageID(request.PageID)
	for index := range browser.pages {
		active := browser.pages[index].ID == selected
		browser.pages[index].Active = active
		if active {
			return browser.pages[index], nil
		}
	}
	return PageInfo{}, fmt.Errorf("page %q not found", selected)
}

func (browser *fakePersistentBrowser) ClosePage(_ context.Context, request PageSelection) error {
	selected := browser.selectedPageID(request.PageID)
	for index := range browser.pages {
		if browser.pages[index].ID != selected {
			continue
		}
		browser.pages = append(browser.pages[:index], browser.pages[index+1:]...)
		if len(browser.pages) > 0 {
			browser.pages[0].Active = true
		}
		return nil
	}
	return fmt.Errorf("page %q not found", selected)
}

func (browser *fakePersistentBrowser) Click(_ context.Context, request InteractionRequest) error {
	browser.actions = append(browser.actions, persistentBrowserAction{
		Kind:     "click",
		PageID:   browser.selectedPageID(request.PageID),
		Selector: request.Selector,
	})
	return nil
}

func (browser *fakePersistentBrowser) Input(_ context.Context, request InputRequest) error {
	browser.actions = append(browser.actions, persistentBrowserAction{
		Kind:     "input",
		PageID:   browser.selectedPageID(request.PageID),
		Selector: request.Selector,
		Text:     request.Text,
	})
	return nil
}

func (browser *fakePersistentBrowser) Wait(_ context.Context, request WaitRequest) error {
	browser.actions = append(browser.actions, persistentBrowserAction{
		Kind:     "wait",
		PageID:   browser.selectedPageID(request.PageID),
		Selector: request.Selector,
	})
	return nil
}

func (browser *fakePersistentBrowser) AccessibilityTree(_ context.Context, request AccessibilityRequest) (AccessibilityTree, error) {
	browser.actions = append(browser.actions, persistentBrowserAction{Kind: "ax-tree", PageID: browser.selectedPageID(request.PageID)})
	return browser.tree, nil
}

func (browser *fakePersistentBrowser) DetectManualChallenge(
	_ context.Context,
	request ManualChallengeRequest,
) (ManualChallengeResult, error) {
	browser.actions = append(browser.actions, persistentBrowserAction{Kind: "challenge", PageID: browser.selectedPageID(request.PageID)})
	return browser.challenge, nil
}

func (browser *fakePersistentBrowser) Download(_ context.Context, request DownloadRequest) (CapturedDownload, error) {
	request.PageID = browser.selectedPageID(request.PageID)
	browser.downloads = append(browser.downloads, request)
	if browser.hasDownloadResult {
		return browser.downloadResult, nil
	}
	return CapturedDownload{Path: request.Path, Bytes: int64(11), ContentType: "application/zip"}, nil
}

func (browser *fakePersistentBrowser) Close(context.Context) error {
	browser.closed = true
	return nil
}

func (browser *fakePersistentBrowser) selectedPageID(pageID string) string {
	if pageID != "" {
		return pageID
	}
	for _, page := range browser.pages {
		if page.Active {
			return page.ID
		}
	}
	if len(browser.pages) == 0 {
		return ""
	}
	return browser.pages[0].ID
}

type fakeStatelessBrowser struct {
	metadata          ResponseMetadata
	metadataRequest   ResponseMetadataRequest
	screenshotRequest ScreenshotRequest
	pdfRequest        PDFRequest
}

func (browser *fakeStatelessBrowser) ResponseMetadata(
	_ context.Context,
	request ResponseMetadataRequest,
) (ResponseMetadata, error) {
	browser.metadataRequest = request
	return browser.metadata, nil
}

func (browser *fakeStatelessBrowser) Screenshot(_ context.Context, request ScreenshotRequest) error {
	browser.screenshotRequest = request
	return nil
}

func (browser *fakeStatelessBrowser) PDF(_ context.Context, request PDFRequest) error {
	browser.pdfRequest = request
	return nil
}
