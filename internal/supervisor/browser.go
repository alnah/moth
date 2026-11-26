package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/content"
)

const (
	headerCWD            = "X-Moth-CWD"
	headerLocalStateDir  = "X-Moth-Local-State-Dir"
	headerGlobalStateDir = "X-Moth-Global-State-Dir"
)

type requestMetadata struct {
	CWD            string
	LocalStateDir  string
	GlobalStateDir string
}

type browserServiceKey struct {
	Local  string
	Global string
}

func requestMetadataFrom(request *http.Request) requestMetadata {
	return requestMetadata{
		CWD:            request.Header.Get(headerCWD),
		LocalStateDir:  request.Header.Get(headerLocalStateDir),
		GlobalStateDir: request.Header.Get(headerGlobalStateDir),
	}
}

func absoluteHeaderPath(name string, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: %s is required", errInvalidParams, name)
	}
	if strings.IndexByte(value, 0) >= 0 || strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("%w: %s is invalid", errInvalidParams, name)
	}
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("%w: %s must be absolute", errInvalidParams, name)
	}
	return filepath.Clean(value), nil
}

func resolveStateDirs(metadata requestMetadata) (browser.StateDirs, error) {
	if _, err := absoluteHeaderPath(headerCWD, metadata.CWD); err != nil {
		return browser.StateDirs{}, err
	}
	local, err := absoluteHeaderPath(headerLocalStateDir, metadata.LocalStateDir)
	if err != nil {
		return browser.StateDirs{}, err
	}
	global, err := absoluteHeaderPath(headerGlobalStateDir, metadata.GlobalStateDir)
	if err != nil {
		return browser.StateDirs{}, err
	}
	return browser.StateDirs{Local: local, Global: global}, nil
}

func resolveClientPath(cwd string, path string) (string, error) {
	cwd, err := absoluteHeaderPath(headerCWD, cwd)
	if err != nil {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: path is required", errInvalidParams)
	}
	if strings.IndexByte(path, 0) >= 0 || strings.ContainsAny(path, "\r\n") {
		return "", fmt.Errorf("%w: path is invalid", errInvalidParams)
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Join(cwd, path), nil
}

func (server *Server) useBrowser(
	metadata requestMetadata,
	run func(*browser.PersistentService) error,
) error {
	dirs, err := resolveStateDirs(metadata)
	if err != nil {
		return err
	}
	key := browserServiceKey{Local: dirs.Local, Global: dirs.Global}

	server.browserMu.Lock()
	defer server.browserMu.Unlock()
	if server.closed.Load() {
		return errors.New("daemon closing")
	}
	service := server.browsers[key]
	if service == nil {
		service = server.newBrowserService(browser.PersistentServiceOptions{
			StateDirs:  dirs,
			BrowserBin: server.browserBin,
			Lifecycle:  server.lifecycle,
			Clock:      server.clock,
		})
		if service == nil {
			return errors.New("browser service unavailable")
		}
		server.browsers[key] = service
	}
	return run(service)
}

func sortedBrowserServiceKeys(services map[browserServiceKey]*browser.PersistentService) []browserServiceKey {
	keys := make([]browserServiceKey, 0, len(services))
	for key := range services {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i int, j int) bool {
		if keys[i].Local != keys[j].Local {
			return keys[i].Local < keys[j].Local
		}
		return keys[i].Global < keys[j].Global
	})
	return keys
}

func (server *Server) registerBrowserHandlers() {
	server.handlers[MethodBrowserStart] = server.handleBrowserStart
	server.handlers[MethodBrowserStop] = server.handleBrowserStop
	server.handlers[MethodBrowserStatus] = server.handleBrowserStatus
	server.handlers[MethodBrowserConnect] = server.handleBrowserConnect
	server.handlers[MethodBrowserOpen] = server.handleBrowserOpen
	server.handlers[MethodBrowserPages] = server.handleBrowserPages
	server.handlers[MethodBrowserSwitch] = server.handleBrowserSwitch
	server.handlers[MethodBrowserClosePage] = server.handleBrowserClosePage
	server.handlers[MethodBrowserClick] = server.handleBrowserClick
	server.handlers[MethodBrowserInput] = server.handleBrowserInput
	server.handlers[MethodBrowserWait] = server.handleBrowserWait
	server.handlers[MethodBrowserAccessibility] = server.handleBrowserAccessibility
	server.handlers[MethodBrowserChallenge] = server.handleBrowserChallenge
	server.handlers[MethodBrowserScreenshot] = server.handleBrowserScreenshot
	server.handlers[MethodBrowserPDF] = server.handleBrowserPDF
	server.handlers[MethodBrowserDownload] = server.handleBrowserDownload
	server.handlers[MethodBrowserMetadata] = server.handleBrowserMetadata
}

func (server *Server) handleBrowserStart(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserStartParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var status browser.BrowserStatus
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		status, runErr = service.Start(ctx, browser.StartRequest{Scope: params.Scope, Show: params.Show})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserStatusResult(status))
}

func (server *Server) handleBrowserStop(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserStopParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var status browser.BrowserStatus
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		status, runErr = service.Stop(ctx, browser.StopRequest{Scope: params.Scope})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserStatusResult(status))
}

func (server *Server) handleBrowserStatus(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserStatusParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var status browser.BrowserStatus
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		status, runErr = service.Status(ctx, browser.StatusRequest{Scope: params.Scope})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserStatusResult(status))
}

func (server *Server) handleBrowserConnect(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserConnectParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.HostPort) == "" {
		return nil, fmt.Errorf("%w: host_port is required", errInvalidParams)
	}
	var status browser.BrowserStatus
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		status, runErr = service.Connect(ctx, browser.ConnectRequest{
			Scope:    params.Scope,
			HostPort: params.HostPort,
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserStatusResult(status))
}

func (server *Server) handleBrowserOpen(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserOpenParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.URL) == "" {
		return nil, fmt.Errorf("%w: url is required", errInvalidParams)
	}
	var page browser.PageInfo
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		page, runErr = service.OpenPage(ctx, browser.OpenPageRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			URL:         params.URL,
			Headers:     params.Headers,
			UserAgent:   params.UserAgent,
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserPageResult(page))
}

func (server *Server) handleBrowserPages(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserSessionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var pages []browser.PageInfo
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		pages, runErr = service.ListPages(ctx, browser.SessionRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	result := make([]BrowserPageResult, 0, len(pages))
	for _, page := range pages {
		result = append(result, newBrowserPageResult(page))
	}
	return marshalResult(result)
}

func (server *Server) handleBrowserSwitch(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserSelectionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var page browser.PageInfo
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		page, runErr = service.SwitchPage(ctx, browser.PageSelection{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserPageResult(page))
}

func (server *Server) handleBrowserClosePage(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserSelectionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		return service.ClosePage(ctx, browser.PageSelection{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
		})
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(struct{}{})
}

func (server *Server) handleBrowserClick(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserInteractionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.Selector) == "" {
		return nil, fmt.Errorf("%w: selector is required", errInvalidParams)
	}
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		return service.Click(ctx, browser.InteractionRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
			Selector:    params.Selector,
		})
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(struct{}{})
}

func (server *Server) handleBrowserInput(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserInputParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.Selector) == "" {
		return nil, fmt.Errorf("%w: selector is required", errInvalidParams)
	}
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		return service.Input(ctx, browser.InputRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
			Selector:    params.Selector,
			Text:        params.Text,
		})
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(struct{}{})
}

func (server *Server) handleBrowserWait(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserWaitParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.Selector) == "" {
		return nil, fmt.Errorf("%w: selector is required", errInvalidParams)
	}
	state := browser.WaitState(params.State)
	if state != browser.WaitAttached && state != browser.WaitVisible {
		return nil, fmt.Errorf("%w: invalid wait state", errInvalidParams)
	}
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		return service.Wait(ctx, browser.WaitRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
			Selector:    params.Selector,
			State:       state,
		})
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(struct{}{})
}

func (server *Server) handleBrowserAccessibility(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserAccessibilityParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var tree browser.AccessibilityTree
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		tree, runErr = service.AccessibilityTree(ctx, browser.AccessibilityRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
			MaxDepth:    params.MaxDepth,
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserAccessibilityResult(tree))
}

func (server *Server) handleBrowserChallenge(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserChallengeParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	var challenge browser.ManualChallengeResult
	err := server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		challenge, runErr = service.DetectManualChallenge(ctx, browser.ManualChallengeRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserChallengeResult(challenge))
}

func (server *Server) handleBrowserDownload(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserDownloadParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.Selector) == "" {
		return nil, fmt.Errorf("%w: selector is required", errInvalidParams)
	}
	displayPath := params.Path
	absolutePath, err := resolveClientPath(metadata.CWD, params.Path)
	if err != nil {
		return nil, err
	}
	var captured browser.CapturedDownload
	err = server.useBrowser(metadata, func(service *browser.PersistentService) error {
		var runErr error
		captured, runErr = service.Download(ctx, browser.DownloadRequest{
			ProfileName: params.ProfileName,
			SessionName: params.SessionName,
			PageID:      params.PageID,
			Selector:    params.Selector,
			Path:        absolutePath,
		})
		return runErr
	})
	if err != nil {
		return nil, err
	}
	bytes, err := capturedByteCount(captured.Bytes)
	if err != nil {
		return nil, err
	}
	return marshalResult(BrowserDownloadResult{
		Path:        displayPath,
		Bytes:       bytes,
		ContentType: captured.ContentType,
	})
}

func (server *Server) handleBrowserScreenshot(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserScreenshotParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.URL) == "" {
		return nil, fmt.Errorf("%w: url is required", errInvalidParams)
	}
	path, err := resolveClientPath(metadata.CWD, params.Path)
	if err != nil {
		return nil, err
	}
	if server.pool == nil {
		return nil, errors.New("browser pool unavailable")
	}
	if err := server.pool.Screenshot(ctx, browser.ScreenshotRequest{
		URL:       params.URL,
		Path:      path,
		FullPage:  params.FullPage,
		MaxBytes:  params.MaxBytes,
		Headers:   params.Headers,
		UserAgent: params.UserAgent,
	}); err != nil {
		return nil, err
	}
	return marshalResult(struct{}{})
}

func (server *Server) handleBrowserPDF(
	ctx context.Context,
	metadata requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserPDFParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.URL) == "" {
		return nil, fmt.Errorf("%w: url is required", errInvalidParams)
	}
	path, err := resolveClientPath(metadata.CWD, params.Path)
	if err != nil {
		return nil, err
	}
	if server.pool == nil {
		return nil, errors.New("browser pool unavailable")
	}
	if err := server.pool.PDF(ctx, browser.PDFRequest{
		URL:      params.URL,
		Path:     path,
		MaxBytes: params.MaxBytes,
	}); err != nil {
		return nil, err
	}
	return marshalResult(struct{}{})
}

func (server *Server) handleBrowserMetadata(
	ctx context.Context,
	_ requestMetadata,
	raw json.RawMessage,
) (json.RawMessage, error) {
	var params BrowserMetadataParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, err
	}
	if strings.TrimSpace(params.URL) == "" {
		return nil, fmt.Errorf("%w: url is required", errInvalidParams)
	}
	if server.pool == nil {
		return nil, errors.New("browser pool unavailable")
	}
	metadata, err := server.pool.ResponseMetadata(ctx, browser.ResponseMetadataRequest{
		URL:            params.URL,
		MaxHeaderBytes: params.MaxHeaderBytes,
	})
	if err != nil {
		return nil, err
	}
	return marshalResult(newBrowserMetadataResult(metadata))
}

func newBrowserStatusResult(value browser.BrowserStatus) BrowserStatusResult {
	return BrowserStatusResult{
		Status:       value.Status,
		Scope:        value.Scope,
		DebugURL:     value.DebugURL,
		ChromePID:    value.ChromePID,
		Owned:        value.Owned,
		DataDir:      value.DataDir,
		ActivePageID: value.ActivePageID,
	}
}

func newBrowserPageResult(value browser.PageInfo) BrowserPageResult {
	return BrowserPageResult{
		ID:          value.ID,
		URL:         value.URL,
		Title:       value.Title,
		Active:      value.Active,
		ProfileName: value.ProfileName,
		SessionName: value.SessionName,
	}
}

func newBrowserAccessibilityResult(value browser.AccessibilityTree) BrowserAccessibilityResult {
	nodes := make([]BrowserAccessibilityNode, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		nodes = append(nodes, BrowserAccessibilityNode{Role: node.Role, Name: node.Name})
	}
	return BrowserAccessibilityResult{Nodes: nodes}
}

func newBrowserChallengeResult(value browser.ManualChallengeResult) BrowserChallengeResult {
	warnings := make([]string, 0, len(value.Warnings))
	for _, warning := range value.Warnings {
		warnings = append(warnings, string(warning))
	}
	return BrowserChallengeResult{
		ManualRequired: value.ManualRequired,
		Kind:           value.Kind,
		Solved:         value.Solved,
		Warnings:       warnings,
	}
}

func newBrowserMetadataResult(value browser.ResponseMetadata) BrowserMetadataResult {
	return BrowserMetadataResult{
		URL:         value.URL,
		Status:      value.Status,
		ContentType: value.ContentType,
		Headers:     value.Headers,
	}
}

func capturedByteCount(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case []byte:
		return int64(len(typed)), nil
	default:
		return 0, fmt.Errorf("unsupported download byte count %T", value)
	}
}

func newBrowserChallengeResultFromWarnings(
	value BrowserChallengeResult,
) browser.ManualChallengeResult {
	warnings := make([]content.Warning, 0, len(value.Warnings))
	for _, warning := range value.Warnings {
		warnings = append(warnings, content.Warning(warning))
	}
	return browser.ManualChallengeResult{
		ManualRequired: value.ManualRequired,
		Kind:           value.Kind,
		Solved:         value.Solved,
		Warnings:       warnings,
	}
}
