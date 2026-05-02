package browser

import (
	"context"
	"errors"

	"github.com/alnah/moth/internal/content"
)

// ErrBrowserMissing reports that no usable Chromium-compatible browser could be launched.
var ErrBrowserMissing = errors.New("browser missing")

// ErrPoolClosed reports that a browser worker was requested after pool shutdown.
var ErrPoolClosed = errors.New("browser pool closed")

// WorkerFactory creates a browser worker for a pool slot.
type WorkerFactory func(context.Context) (Worker, error)

// ConfiguredWorkerFactory creates a browser worker from resolved worker options.
type ConfiguredWorkerFactory func(context.Context, WorkerOptions) (Worker, error)

// Worker owns one browser instance and runs page-scoped browser operations.
type Worker interface {
	OpenPage(context.Context, PageRequest) (LoadedPage, error)
	CaptureScreenshot(context.Context, ScreenshotRequest) ([]byte, error)
	Close() error
}

// WorkerOptions is the exported worker creation configuration seam.
type WorkerOptions struct {
	BrowserBin       string
	NoSandbox        bool
	UserDataDir      string
	Headless         bool
	ProxyURL         string
	BlockedResources ResourceSet
}

// Resource identifies a browser resource class that can be blocked.
type Resource uint8

const (
	// ResourceImages blocks common image resources.
	ResourceImages Resource = 1 << iota
	// ResourceFonts blocks common font resources.
	ResourceFonts
	// ResourceMedia blocks common audio and video resources.
	ResourceMedia
)

// ResourceSet stores browser resource block flags.
type ResourceSet uint8

// Has reports whether resource is enabled in the set.
func (set ResourceSet) Has(resource Resource) bool {
	return set&ResourceSet(resource) != 0
}

// PageRequest describes a rendered page fetch operation.
type PageRequest struct {
	URL       string
	MaxBytes  int64
	Headers   map[string]string
	UserAgent string
}

// LoadedPage is the rendered HTML returned by a browser worker.
type LoadedPage struct {
	URL  string
	HTML string
}

// ScreenshotRequest describes a rendered page screenshot operation.
type ScreenshotRequest struct {
	URL       string
	Path      string
	FullPage  bool
	MaxBytes  int64
	Headers   map[string]string
	UserAgent string
}

// OpenPageRequest describes a persistent session page open operation.
type OpenPageRequest struct {
	ProfileName string
	SessionName string
	URL         string
	Headers     map[string]string
	UserAgent   string
}

// SessionRequest selects a persistent browser session.
type SessionRequest struct {
	ProfileName string
	SessionName string
}

// PageSelection selects a persistent browser page.
type PageSelection struct {
	ProfileName string
	SessionName string
	PageID      string
}

// PageInfo describes a persistent browser page for JSON output.
type PageInfo struct {
	ID          string `json:"id"`
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Active      bool   `json:"active"`
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
}

// InteractionRequest describes a selector-only manual browser action.
type InteractionRequest struct {
	ProfileName string
	SessionName string
	PageID      string
	Selector    string
}

// InputRequest describes a selector text input action.
type InputRequest struct {
	ProfileName string
	SessionName string
	PageID      string
	Selector    string
	Text        string
}

// WaitState identifies a manual wait condition.
type WaitState string

const (
	// WaitAttached waits until a selector exists in the DOM.
	WaitAttached WaitState = "attached"
	// WaitVisible waits until a selector is visible.
	WaitVisible WaitState = "visible"
)

// WaitRequest describes a selector wait action.
type WaitRequest struct {
	ProfileName string
	SessionName string
	PageID      string
	Selector    string
	State       WaitState
}

// AccessibilityRequest describes accessibility tree extraction.
type AccessibilityRequest struct {
	ProfileName string
	SessionName string
	PageID      string
	MaxDepth    int
}

// AccessibilityTree is a stable accessibility snapshot.
type AccessibilityTree struct {
	Nodes []AccessibilityNode `json:"nodes"`
}

// AccessibilityNode is a stable role/name accessibility node.
type AccessibilityNode struct {
	Role string `json:"role,omitempty"`
	Name string `json:"name,omitempty"`
}

// DownloadRequest describes a browser-session download capture.
type DownloadRequest struct {
	ProfileName string
	SessionName string
	PageID      string
	Selector    string
	Path        string
}

// CapturedDownload describes a file written from browser download bytes.
type CapturedDownload struct {
	Path        string `json:"path"`
	Bytes       any    `json:"bytes"`
	ContentType string `json:"content_type,omitempty"`
}

// ResponseMetadataRequest describes network response metadata capture.
type ResponseMetadataRequest struct {
	URL            string
	MaxHeaderBytes int
}

// ResponseMetadata is bounded network response metadata for JSON output.
type ResponseMetadata struct {
	URL         string              `json:"url"`
	Status      int                 `json:"status,omitempty"`
	ContentType string              `json:"content_type,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
}

// PDFRequest describes a browser PDF capture operation.
type PDFRequest struct {
	URL      string
	Path     string
	MaxBytes int64
}

// ManualChallengeRequest selects a page for manual challenge detection.
type ManualChallengeRequest struct {
	ProfileName string
	SessionName string
	PageID      string
}

// ManualChallengeResult reports challenge state without solving it.
type ManualChallengeResult struct {
	ManualRequired bool              `json:"manual_required"`
	Kind           string            `json:"kind,omitempty"`
	Solved         bool              `json:"solved"`
	Warnings       []content.Warning `json:"warnings"`
}
