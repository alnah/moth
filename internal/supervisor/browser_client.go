package supervisor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alnah/moth/internal/browser"
)

// Start launches or reuses a persistent browser.
func (client *Client) Start(
	ctx context.Context,
	request browser.StartRequest,
) (browser.BrowserStatus, error) {
	var result BrowserStatusResult
	if err := client.callBrowser(ctx, MethodBrowserStart, BrowserStartParams{
		Scope: request.Scope,
		Show:  request.Show,
	}, &result); err != nil {
		return browser.BrowserStatus{}, err
	}
	return result.browserStatus(), nil
}

// Stop stops a persistent browser.
func (client *Client) Stop(
	ctx context.Context,
	request browser.StopRequest,
) (browser.BrowserStatus, error) {
	var result BrowserStatusResult
	if err := client.callBrowser(ctx, MethodBrowserStop, BrowserStopParams{
		Scope: request.Scope,
	}, &result); err != nil {
		return browser.BrowserStatus{}, err
	}
	return result.browserStatus(), nil
}

// Status reports persistent browser state.
func (client *Client) Status(
	ctx context.Context,
	request browser.StatusRequest,
) (browser.BrowserStatus, error) {
	var result BrowserStatusResult
	if err := client.callBrowser(ctx, MethodBrowserStatus, BrowserStatusParams{
		Scope: request.Scope,
	}, &result); err != nil {
		return browser.BrowserStatus{}, err
	}
	return result.browserStatus(), nil
}

// Connect connects an external browser.
func (client *Client) Connect(
	ctx context.Context,
	request browser.ConnectRequest,
) (browser.BrowserStatus, error) {
	var result BrowserStatusResult
	if err := client.callBrowser(ctx, MethodBrowserConnect, BrowserConnectParams{
		Scope:    request.Scope,
		HostPort: request.HostPort,
	}, &result); err != nil {
		return browser.BrowserStatus{}, err
	}
	return result.browserStatus(), nil
}

// OpenPage opens a persistent page.
func (client *Client) OpenPage(
	ctx context.Context,
	request browser.OpenPageRequest,
) (browser.PageInfo, error) {
	var result BrowserPageResult
	if err := client.callBrowser(ctx, MethodBrowserOpen, BrowserOpenParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		URL:         request.URL,
		Headers:     request.Headers,
		UserAgent:   request.UserAgent,
	}, &result); err != nil {
		return browser.PageInfo{}, err
	}
	return result.browserPage(), nil
}

// ListPages lists persistent pages.
func (client *Client) ListPages(
	ctx context.Context,
	request browser.SessionRequest,
) ([]browser.PageInfo, error) {
	var result []BrowserPageResult
	if err := client.callBrowser(ctx, MethodBrowserPages, BrowserSessionParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
	}, &result); err != nil {
		return nil, err
	}
	pages := make([]browser.PageInfo, 0, len(result))
	for _, page := range result {
		pages = append(pages, page.browserPage())
	}
	return pages, nil
}

// SwitchPage selects a persistent page.
func (client *Client) SwitchPage(
	ctx context.Context,
	request browser.PageSelection,
) (browser.PageInfo, error) {
	var result BrowserPageResult
	if err := client.callBrowser(ctx, MethodBrowserSwitch, BrowserSelectionParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
	}, &result); err != nil {
		return browser.PageInfo{}, err
	}
	return result.browserPage(), nil
}

// ClosePage closes a persistent page.
func (client *Client) ClosePage(ctx context.Context, request browser.PageSelection) error {
	return client.callBrowser(ctx, MethodBrowserClosePage, BrowserSelectionParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
	}, nil)
}

// Click clicks a persistent page selector.
func (client *Client) Click(ctx context.Context, request browser.InteractionRequest) error {
	return client.callBrowser(ctx, MethodBrowserClick, BrowserInteractionParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
		Selector:    request.Selector,
	}, nil)
}

// Input types text into a persistent page selector.
func (client *Client) Input(ctx context.Context, request browser.InputRequest) error {
	return client.callBrowser(ctx, MethodBrowserInput, BrowserInputParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
		Selector:    request.Selector,
		Text:        request.Text,
	}, nil)
}

// Wait waits for a persistent page selector.
func (client *Client) Wait(ctx context.Context, request browser.WaitRequest) error {
	return client.callBrowser(ctx, MethodBrowserWait, BrowserWaitParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
		Selector:    request.Selector,
		State:       string(request.State),
	}, nil)
}

// AccessibilityTree extracts a persistent accessibility tree.
func (client *Client) AccessibilityTree(
	ctx context.Context,
	request browser.AccessibilityRequest,
) (browser.AccessibilityTree, error) {
	var result BrowserAccessibilityResult
	if err := client.callBrowser(ctx, MethodBrowserAccessibility, BrowserAccessibilityParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
		MaxDepth:    request.MaxDepth,
	}, &result); err != nil {
		return browser.AccessibilityTree{}, err
	}
	nodes := make([]browser.AccessibilityNode, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		nodes = append(nodes, browser.AccessibilityNode{Role: node.Role, Name: node.Name})
	}
	return browser.AccessibilityTree{Nodes: nodes}, nil
}

// DetectManualChallenge reports manual challenge state.
func (client *Client) DetectManualChallenge(
	ctx context.Context,
	request browser.ManualChallengeRequest,
) (browser.ManualChallengeResult, error) {
	var result BrowserChallengeResult
	if err := client.callBrowser(ctx, MethodBrowserChallenge, BrowserChallengeParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
	}, &result); err != nil {
		return browser.ManualChallengeResult{}, err
	}
	return newBrowserChallengeResultFromWarnings(result), nil
}

// Screenshot captures a stateless screenshot.
func (client *Client) Screenshot(ctx context.Context, request browser.ScreenshotRequest) error {
	return client.callBrowserPath(ctx, MethodBrowserScreenshot, BrowserScreenshotParams{
		URL:       request.URL,
		Path:      request.Path,
		FullPage:  request.FullPage,
		MaxBytes:  request.MaxBytes,
		Headers:   request.Headers,
		UserAgent: request.UserAgent,
	}, nil)
}

// PDF captures a stateless PDF.
func (client *Client) PDF(ctx context.Context, request browser.PDFRequest) error {
	return client.callBrowserPath(ctx, MethodBrowserPDF, BrowserPDFParams{
		URL:      request.URL,
		Path:     request.Path,
		MaxBytes: request.MaxBytes,
	}, nil)
}

// Download captures a persistent download.
func (client *Client) Download(
	ctx context.Context,
	request browser.DownloadRequest,
) (browser.CapturedDownload, error) {
	var result BrowserDownloadResult
	if err := client.callBrowser(ctx, MethodBrowserDownload, BrowserDownloadParams{
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
		PageID:      request.PageID,
		Selector:    request.Selector,
		Path:        request.Path,
	}, &result); err != nil {
		return browser.CapturedDownload{}, err
	}
	return browser.CapturedDownload{
		Path:        result.Path,
		Bytes:       result.Bytes,
		ContentType: result.ContentType,
	}, nil
}

// ResponseMetadata captures stateless response metadata.
func (client *Client) ResponseMetadata(
	ctx context.Context,
	request browser.ResponseMetadataRequest,
) (browser.ResponseMetadata, error) {
	var result BrowserMetadataResult
	if err := client.callBrowserWithoutHeaders(ctx, MethodBrowserMetadata, BrowserMetadataParams{
		URL:            request.URL,
		MaxHeaderBytes: request.MaxHeaderBytes,
	}, &result); err != nil {
		return browser.ResponseMetadata{}, err
	}
	return browser.ResponseMetadata{
		URL:         result.URL,
		Status:      result.Status,
		ContentType: result.ContentType,
		Headers:     result.Headers,
	}, nil
}

func (client *Client) callBrowser(
	ctx context.Context,
	method string,
	params any,
	result any,
) error {
	return client.callBrowserWithHeaders(ctx, method, params, result, true)
}

func (client *Client) callBrowserPath(
	ctx context.Context,
	method string,
	params any,
	result any,
) error {
	return client.callBrowserWithHeaders(ctx, method, params, result, false)
}

func (client *Client) callBrowserWithoutHeaders(
	ctx context.Context,
	method string,
	params any,
	result any,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if _, err := client.ensureLocked(ctx); err != nil {
		return err
	}
	return client.callWithParamsLocked(ctx, method, params, result)
}

func (client *Client) callBrowserWithHeaders(
	ctx context.Context,
	method string,
	params any,
	result any,
	includeStateDirs bool,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if _, err := client.ensureLocked(ctx); err != nil {
		return err
	}
	headers, err := client.browserHeaders(includeStateDirs)
	if err != nil {
		return err
	}
	return client.callWithParamsAndHeadersLocked(ctx, method, params, result, headers)
}

func (client *Client) browserHeaders(includeStateDirs bool) (map[string]string, error) {
	workingDir, err := client.workingDir()
	if err != nil {
		return nil, err
	}
	headers := map[string]string{headerCWD: workingDir}
	if !includeStateDirs {
		return headers, nil
	}

	dirs := client.options.StateDirs
	defaults := browser.DefaultStateDirs()
	if dirs.Local == "" {
		dirs.Local = defaults.Local
	}
	if dirs.Global == "" {
		dirs.Global = defaults.Global
	}
	local, err := absoluteBrowserPath(workingDir, dirs.Local)
	if err != nil {
		return nil, err
	}
	global, err := absoluteBrowserPath(workingDir, dirs.Global)
	if err != nil {
		return nil, err
	}
	headers[headerLocalStateDir] = local
	headers[headerGlobalStateDir] = global
	return headers, nil
}

func (client *Client) workingDir() (string, error) {
	workingDir := client.options.WorkingDir
	if workingDir == "" {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
	}
	absolute, err := filepath.Abs(workingDir)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	if strings.IndexByte(absolute, 0) >= 0 || strings.ContainsAny(absolute, "\r\n") {
		return "", fmt.Errorf("working directory is invalid")
	}
	return filepath.Clean(absolute), nil
}

func absoluteBrowserPath(workingDir string, path string) (string, error) {
	if strings.IndexByte(path, 0) >= 0 || strings.ContainsAny(path, "\r\n") {
		return "", fmt.Errorf("browser state path is invalid")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workingDir, path)
	}
	return filepath.Clean(path), nil
}

func (value BrowserStatusResult) browserStatus() browser.BrowserStatus {
	return browser.BrowserStatus{
		Status:       value.Status,
		Scope:        value.Scope,
		DebugURL:     value.DebugURL,
		ChromePID:    value.ChromePID,
		Owned:        value.Owned,
		DataDir:      value.DataDir,
		ActivePageID: value.ActivePageID,
	}
}

func (value BrowserPageResult) browserPage() browser.PageInfo {
	return browser.PageInfo{
		ID:          value.ID,
		URL:         value.URL,
		Title:       value.Title,
		Active:      value.Active,
		ProfileName: value.ProfileName,
		SessionName: value.SessionName,
	}
}
