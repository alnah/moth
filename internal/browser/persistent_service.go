package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	persistentStateFileName = "state.json"
	persistentChromeDirName = "chrome-data"
	defaultVersionTimeout   = 5 * time.Second
	maxDebuggerVersionBytes = 64 * 1024
)

// PersistentLauncher launches an owned persistent browser process.
type PersistentLauncher interface {
	LaunchBrowser(context.Context, LaunchRequest) (LaunchResult, error)
}

// PersistentConnector reconnects to a browser debug WebSocket URL.
type PersistentConnector interface {
	ConnectBrowser(context.Context, string) (BrowserConnection, error)
}

// BrowserConnection exposes persistent browser page operations without Rod details.
//
//nolint:revive // BrowserConnection is the explicit test and seam name for this API.
type BrowserConnection interface {
	OpenPage(context.Context, OpenPageRequest) (PageInfo, error)
	ListPages(context.Context, SessionRequest) ([]PageInfo, error)
	SwitchPage(context.Context, PageSelection) (PageInfo, error)
	ClosePage(context.Context, PageSelection) error
	Click(context.Context, InteractionRequest) error
	Input(context.Context, InputRequest) error
	Wait(context.Context, WaitRequest) error
	AccessibilityTree(context.Context, AccessibilityRequest) (AccessibilityTree, error)
	DetectManualChallenge(context.Context, ManualChallengeRequest) (ManualChallengeResult, error)
	Close(context.Context) error
}

type persistentDownloadConnection interface {
	Download(context.Context, DownloadRequest) (CapturedDownload, error)
}

// StatelessBrowserService runs URL-scoped browser operations.
type StatelessBrowserService interface {
	ResponseMetadata(context.Context, ResponseMetadataRequest) (ResponseMetadata, error)
	Screenshot(context.Context, ScreenshotRequest) error
	PDF(context.Context, PDFRequest) error
}

// PersistentServiceOptions configures the persistent browser service.
type PersistentServiceOptions struct {
	StateDirs  StateDirs
	Launcher   PersistentLauncher
	Connector  PersistentConnector
	HTTPClient *http.Client
	Clock      func() time.Time
	Stateless  StatelessBrowserService
	BrowserBin string
	NoSandbox  bool
}

// PersistentService stores short-lived CLI browser state and reconnects per command.
type PersistentService struct {
	stateDirs  StateDirs
	launcher   PersistentLauncher
	connector  PersistentConnector
	httpClient *http.Client
	clock      func() time.Time
	stateless  StatelessBrowserService
	browserBin string
	noSandbox  bool
}

// NewPersistentService creates a service for cross-process browser commands.
func NewPersistentService(options PersistentServiceOptions) *PersistentService {
	stateDirs := options.StateDirs
	if stateDirs.Local == "" || stateDirs.Global == "" {
		stateDirs = defaultStateDirs()
	}
	service := &PersistentService{
		stateDirs:  stateDirs,
		launcher:   options.Launcher,
		connector:  options.Connector,
		httpClient: options.HTTPClient,
		clock:      options.Clock,
		stateless:  options.Stateless,
		browserBin: options.BrowserBin,
		noSandbox:  options.NoSandbox,
	}
	if service.launcher == nil {
		service.launcher = rodPersistentLauncher{}
	}
	if service.connector == nil {
		service.connector = rodPersistentConnector{}
	}
	if service.httpClient == nil {
		service.httpClient = &http.Client{Timeout: defaultVersionTimeout}
	}
	if service.clock == nil {
		service.clock = time.Now
	}
	return service
}

// Start launches an owned browser unless current state is still running.
func (service *PersistentService) Start(ctx context.Context, request StartRequest) (BrowserStatus, error) {
	scope := service.writeScope(request.Scope)
	state, readErr := service.loadState(scope)
	if readErr == nil {
		if _, err := service.connector.ConnectBrowser(ctx, state.DebugURL); err == nil {
			return state.status("running"), nil
		}
	} else if !errors.Is(readErr, os.ErrNotExist) && !errors.Is(readErr, ErrBrowserStateCorrupt) {
		return BrowserStatus{}, readErr
	}

	stateDir := service.stateDir(scope)
	dataDir := filepath.Join(stateDir, persistentChromeDirName)
	result, err := service.launcher.LaunchBrowser(ctx, LaunchRequest{
		DataDir:    dataDir,
		Show:       request.Show,
		BrowserBin: service.browserBin,
		NoSandbox:  service.noSandbox,
	})
	if err != nil {
		return BrowserStatus{}, fmt.Errorf("launch persistent browser: %w", err)
	}
	state = persistentBrowserState{
		Scope:     scope,
		DebugURL:  result.DebugURL,
		ChromePID: result.ChromePID,
		Owned:     true,
		DataDir:   dataDir,
		UpdatedAt: service.clock().UTC(),
	}
	if err := service.saveState(state); err != nil {
		return BrowserStatus{}, err
	}
	return state.status("running"), nil
}

// Stop clears state and closes only owned browsers.
func (service *PersistentService) Stop(ctx context.Context, request StopRequest) (BrowserStatus, error) {
	scope := service.readScope(request.Scope)
	state, err := service.loadState(scope)
	if errors.Is(err, os.ErrNotExist) {
		return BrowserStatus{Status: "stopped", Scope: scope}, nil
	}
	if err != nil {
		return BrowserStatus{}, err
	}

	if state.Owned {
		if connection, connectErr := service.connector.ConnectBrowser(ctx, state.DebugURL); connectErr == nil {
			if closeErr := connection.Close(ctx); closeErr != nil {
				return BrowserStatus{}, fmt.Errorf("close owned browser: %w", closeErr)
			}
		}
	}
	if err := service.removeState(scope); err != nil {
		return BrowserStatus{}, err
	}
	status := state.status("stopped")
	status.Scope = scope
	return status, nil
}

// Status reports missing, running, or stale state without failing for missing or stale state.
func (service *PersistentService) Status(ctx context.Context, request StatusRequest) (BrowserStatus, error) {
	scope := service.readScope(request.Scope)
	state, err := service.loadState(scope)
	if errors.Is(err, os.ErrNotExist) {
		return BrowserStatus{Status: "missing", Scope: scope}, nil
	}
	if err != nil {
		return BrowserStatus{}, err
	}
	if _, connectErr := service.connector.ConnectBrowser(ctx, state.DebugURL); connectErr == nil {
		return state.status("running"), nil
	}
	return state.status("stale"), nil
}

// Connect validates and stores an external browser debugger endpoint.
func (service *PersistentService) Connect(ctx context.Context, request ConnectRequest) (BrowserStatus, error) {
	scope := service.writeScope(request.Scope)
	debugURL, err := service.lookupDebuggerURL(ctx, request.HostPort)
	if err != nil {
		return BrowserStatus{}, err
	}
	if _, err := service.connector.ConnectBrowser(ctx, debugURL); err != nil {
		return BrowserStatus{}, fmt.Errorf("connect external browser: %w", err)
	}
	state := persistentBrowserState{
		Scope:     scope,
		DebugURL:  debugURL,
		Owned:     false,
		UpdatedAt: service.clock().UTC(),
	}
	if err := service.saveState(state); err != nil {
		return BrowserStatus{}, err
	}
	return state.status("running"), nil
}

// OpenPage opens a page in the stored persistent browser and records it active.
func (service *PersistentService) OpenPage(ctx context.Context, request OpenPageRequest) (PageInfo, error) {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return PageInfo{}, err
	}
	page, err := connection.OpenPage(ctx, request)
	if err != nil {
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
	pages, err := connection.ListPages(ctx, request)
	if err != nil {
		return nil, err
	}
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
	page, err := connection.SwitchPage(ctx, request)
	if err != nil {
		return PageInfo{}, err
	}
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
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	if closeErr := connection.ClosePage(ctx, request); closeErr != nil {
		return closeErr
	}
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
	return connection.Click(ctx, request)
}

// Input types text into a selector on the active or selected persistent page.
func (service *PersistentService) Input(ctx context.Context, request InputRequest) error {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return err
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	return connection.Input(ctx, request)
}

// Wait waits for a selector on the active or selected persistent page.
func (service *PersistentService) Wait(ctx context.Context, request WaitRequest) error {
	state, connection, err := service.connectFromState(ctx)
	if err != nil {
		return err
	}
	request.PageID = selectedPageID(request.PageID, state.ActivePageID)
	return connection.Wait(ctx, request)
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
	return connection.AccessibilityTree(ctx, request)
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
	return connection.DetectManualChallenge(ctx, request)
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

// ResponseMetadata delegates URL-scoped metadata capture to the stateless browser service.
func (service *PersistentService) ResponseMetadata(
	ctx context.Context,
	request ResponseMetadataRequest,
) (ResponseMetadata, error) {
	if service.stateless == nil {
		return ResponseMetadata{}, errors.New("browser response metadata unavailable")
	}
	return service.stateless.ResponseMetadata(ctx, request)
}

// Screenshot delegates URL-scoped screenshot capture to the stateless browser service.
func (service *PersistentService) Screenshot(ctx context.Context, request ScreenshotRequest) error {
	if service.stateless == nil {
		return errors.New("browser screenshot unavailable")
	}
	return service.stateless.Screenshot(ctx, request)
}

// PDF delegates URL-scoped PDF capture to the stateless browser service.
func (service *PersistentService) PDF(ctx context.Context, request PDFRequest) error {
	if service.stateless == nil {
		return errors.New("browser pdf unavailable")
	}
	return service.stateless.PDF(ctx, request)
}

func (service *PersistentService) connectFromState(
	ctx context.Context,
) (persistentBrowserState, BrowserConnection, error) {
	scope := service.readScope("auto")
	state, err := service.loadState(scope)
	if errors.Is(err, os.ErrNotExist) {
		return persistentBrowserState{}, nil, ErrBrowserStateUnavailable
	}
	if err != nil {
		return persistentBrowserState{}, nil, fmt.Errorf("%w: %w", ErrBrowserStateUnavailable, err)
	}
	connection, err := service.connector.ConnectBrowser(ctx, state.DebugURL)
	if err != nil {
		return persistentBrowserState{}, nil, fmt.Errorf("%w: %w", ErrBrowserStateUnavailable, err)
	}
	return state, connection, nil
}

func (service *PersistentService) lookupDebuggerURL(ctx context.Context, hostPort string) (string, error) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return "", fmt.Errorf("browser connect requires host:port")
	}
	endpoint := url.URL{Scheme: "http", Host: hostPort, Path: "/json/version"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create browser version request: %w", err)
	}
	//nolint:gosec // User-requested debugger host:port is the command contract.
	response, err := service.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch browser version: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", fmt.Errorf("fetch browser version: status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxDebuggerVersionBytes+1))
	if err != nil {
		return "", fmt.Errorf("read browser version: %w", err)
	}
	if len(data) > maxDebuggerVersionBytes {
		return "", fmt.Errorf("read browser version: response over %d bytes", maxDebuggerVersionBytes)
	}
	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("decode browser version: %w", err)
	}
	if strings.TrimSpace(payload.WebSocketDebuggerURL) == "" {
		return "", errors.New("browser version missing webSocketDebuggerUrl")
	}
	return payload.WebSocketDebuggerURL, nil
}

func (service *PersistentService) markActivePage(pages []PageInfo, activeID string) {
	if activeID == "" {
		return
	}
	for index := range pages {
		pages[index].Active = pages[index].ID == activeID
	}
}

func (service *PersistentService) readScope(scope string) string {
	scope = normalizedScope(scope)
	if scope != "auto" {
		return scope
	}
	if stateFileExists(service.stateDir("local")) {
		return "local"
	}
	return "global"
}

func (service *PersistentService) writeScope(scope string) string {
	scope = normalizedScope(scope)
	if scope != "auto" {
		return scope
	}
	if stateFileExists(service.stateDir("local")) {
		return "local"
	}
	return "global"
}

func (service *PersistentService) stateDir(scope string) string {
	if normalizedScope(scope) == "global" {
		return service.stateDirs.Global
	}
	return service.stateDirs.Local
}

func (service *PersistentService) loadState(scope string) (persistentBrowserState, error) {
	scope = normalizedScope(scope)
	path := filepath.Join(service.stateDir(scope), persistentStateFileName)
	data, err := os.ReadFile(path) //nolint:gosec // State path is scoped by configured Moth state directories.
	if err != nil {
		return persistentBrowserState{}, err
	}
	var state persistentBrowserState
	if err := json.Unmarshal(data, &state); err != nil {
		return persistentBrowserState{}, fmt.Errorf("%w: %w", ErrBrowserStateCorrupt, err)
	}
	if strings.TrimSpace(state.DebugURL) == "" {
		return persistentBrowserState{}, fmt.Errorf("%w: missing debug URL", ErrBrowserStateCorrupt)
	}
	state.Scope = scope
	return state, nil
}

func (service *PersistentService) saveState(state persistentBrowserState) error {
	state.Scope = normalizedScope(state.Scope)
	state.UpdatedAt = service.clock().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode browser state: %w", err)
	}
	dir := service.stateDir(state.Scope)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create browser state directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, persistentStateFileName), data, 0o600); err != nil {
		return fmt.Errorf("write browser state: %w", err)
	}
	return nil
}

func (service *PersistentService) removeState(scope string) error {
	err := os.Remove(filepath.Join(service.stateDir(scope), persistentStateFileName))
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("remove browser state: %w", err)
}

type persistentBrowserState struct {
	Scope        string    `json:"-"`
	DebugURL     string    `json:"debug_url"`
	ChromePID    int       `json:"chrome_pid,omitempty"`
	Owned        bool      `json:"owned"`
	DataDir      string    `json:"data_dir,omitempty"`
	ActivePageID string    `json:"active_page_id,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (state persistentBrowserState) status(status string) BrowserStatus {
	return BrowserStatus{
		Status:       status,
		Scope:        state.Scope,
		DebugURL:     state.DebugURL,
		ChromePID:    state.ChromePID,
		Owned:        state.Owned,
		DataDir:      state.DataDir,
		ActivePageID: state.ActivePageID,
	}
}
