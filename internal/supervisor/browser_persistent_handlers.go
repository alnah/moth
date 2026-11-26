package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alnah/moth/internal/browser"
)

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
