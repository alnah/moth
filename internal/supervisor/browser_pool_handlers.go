package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alnah/moth/internal/browser"
)

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
