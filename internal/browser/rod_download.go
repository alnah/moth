package browser

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-rod/rod/lib/proto"
)

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
