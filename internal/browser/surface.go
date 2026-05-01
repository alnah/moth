package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type persistentPageWorker interface {
	OpenPersistentPage(context.Context, OpenPageRequest) (PageInfo, error)
	ListPersistentPages(context.Context, SessionRequest) ([]PageInfo, error)
	SwitchPersistentPage(context.Context, PageSelection) (PageInfo, error)
	ClosePersistentPage(context.Context, PageSelection) error
}

type interactiveWorker interface {
	Click(context.Context, InteractionRequest) error
	Input(context.Context, InputRequest) error
	Wait(context.Context, WaitRequest) error
}

type accessibleWorker interface {
	AccessibilityTree(context.Context, AccessibilityRequest) (AccessibilityTree, error)
}

type downloadWorker interface {
	CaptureDownload(context.Context, DownloadRequest) (CapturedDownload, error)
}

type metadataWorker interface {
	ResponseMetadata(context.Context, ResponseMetadataRequest) (ResponseMetadata, error)
}

type pdfWorker interface {
	CapturePDF(context.Context, PDFRequest) ([]byte, error)
}

type manualChallengeWorker interface {
	DetectManualChallenge(context.Context, ManualChallengeRequest) (ManualChallengeResult, error)
}

func requirePersistentPageWorker(worker Worker) (persistentPageWorker, error) {
	persistentWorker, ok := worker.(persistentPageWorker)
	if !ok {
		return nil, errors.New("browser worker does not support persistent pages")
	}
	return persistentWorker, nil
}

func requireInteractiveWorker(worker Worker) (interactiveWorker, error) {
	interactive, ok := worker.(interactiveWorker)
	if !ok {
		return nil, errors.New("browser worker does not support interactions")
	}
	return interactive, nil
}

func requireAccessibleWorker(worker Worker) (accessibleWorker, error) {
	accessible, ok := worker.(accessibleWorker)
	if !ok {
		return nil, errors.New("browser worker does not support accessibility")
	}
	return accessible, nil
}

func requireDownloadWorker(worker Worker) (downloadWorker, error) {
	downloads, ok := worker.(downloadWorker)
	if !ok {
		return nil, errors.New("browser worker does not support downloads")
	}
	return downloads, nil
}

func requireMetadataWorker(worker Worker) (metadataWorker, error) {
	metadata, ok := worker.(metadataWorker)
	if !ok {
		return nil, errors.New("browser worker does not support response metadata")
	}
	return metadata, nil
}

func requirePDFWorker(worker Worker) (pdfWorker, error) {
	pdf, ok := worker.(pdfWorker)
	if !ok {
		return nil, errors.New("browser worker does not support pdf capture")
	}
	return pdf, nil
}

func requireManualChallengeWorker(worker Worker) (manualChallengeWorker, error) {
	challenge, ok := worker.(manualChallengeWorker)
	if !ok {
		return nil, errors.New("browser worker does not support manual challenge detection")
	}
	return challenge, nil
}

func writeBrowserFile(path string, data []byte, label string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create %s directory: %w", label, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	return nil
}

func downloadBytes(value any) ([]byte, error) {
	switch typed := value.(type) {
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("download bytes have unsupported type %T", value)
	}
}
