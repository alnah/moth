package supervisor

const (
	// MethodBrowserStart launches or reuses a persistent browser.
	MethodBrowserStart = "browser.start"
	// MethodBrowserStop stops a persistent browser.
	MethodBrowserStop = "browser.stop"
	// MethodBrowserStatus reports persistent browser state.
	MethodBrowserStatus = "browser.status"
	// MethodBrowserConnect connects an external browser.
	MethodBrowserConnect = "browser.connect"
	// MethodBrowserOpen opens a persistent page.
	MethodBrowserOpen = "browser.open"
	// MethodBrowserPages lists persistent pages.
	MethodBrowserPages = "browser.pages"
	// MethodBrowserSwitch selects a persistent page.
	MethodBrowserSwitch = "browser.switch"
	// MethodBrowserClosePage closes a persistent page.
	MethodBrowserClosePage = "browser.close-page"
	// MethodBrowserClick clicks a persistent page selector.
	MethodBrowserClick = "browser.click"
	// MethodBrowserInput types into a persistent page selector.
	MethodBrowserInput = "browser.input"
	// MethodBrowserWait waits for a persistent page selector.
	MethodBrowserWait = "browser.wait"
	// MethodBrowserAccessibility extracts a persistent accessibility tree.
	MethodBrowserAccessibility = "browser.accessibility"
	// MethodBrowserChallenge detects a manual challenge.
	MethodBrowserChallenge = "browser.challenge"
	// MethodBrowserScreenshot captures a stateless screenshot.
	MethodBrowserScreenshot = "browser.screenshot"
	// MethodBrowserPDF captures a stateless PDF.
	MethodBrowserPDF = "browser.pdf"
	// MethodBrowserDownload captures a persistent download.
	MethodBrowserDownload = "browser.download"
	// MethodBrowserMetadata captures stateless response metadata.
	MethodBrowserMetadata = "browser.metadata"

	// CodeBrowserStateUnavailable reports missing or unusable browser state.
	CodeBrowserStateUnavailable = "browser_state_unavailable"
	// CodeBrowserStateCorrupt reports corrupt browser state.
	CodeBrowserStateCorrupt = "browser_state_corrupt"
)

// BrowserStartParams describes a persistent browser start.
type BrowserStartParams struct {
	Scope string `json:"scope,omitempty"`
	Show  bool   `json:"show,omitempty"`
}

// BrowserStopParams describes a persistent browser stop.
type BrowserStopParams struct {
	Scope string `json:"scope,omitempty"`
}

// BrowserStatusParams describes a persistent browser status request.
type BrowserStatusParams struct {
	Scope string `json:"scope,omitempty"`
}

// BrowserConnectParams describes an external browser connection.
type BrowserConnectParams struct {
	Scope    string `json:"scope,omitempty"`
	HostPort string `json:"host_port"`
}

// BrowserOpenParams describes a persistent page open.
type BrowserOpenParams struct {
	ProfileName string            `json:"profile_name,omitempty"`
	SessionName string            `json:"session_name,omitempty"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	UserAgent   string            `json:"user_agent,omitempty"`
}

// BrowserSessionParams selects a persistent browser session.
type BrowserSessionParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
}

// BrowserSelectionParams selects a persistent browser page.
type BrowserSelectionParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	PageID      string `json:"page_id,omitempty"`
}

// BrowserInteractionParams describes a persistent selector action.
type BrowserInteractionParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	PageID      string `json:"page_id,omitempty"`
	Selector    string `json:"selector"`
}

// BrowserInputParams describes a persistent text input.
type BrowserInputParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	PageID      string `json:"page_id,omitempty"`
	Selector    string `json:"selector"`
	Text        string `json:"text"`
}

// BrowserWaitParams describes a persistent selector wait.
type BrowserWaitParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	PageID      string `json:"page_id,omitempty"`
	Selector    string `json:"selector"`
	State       string `json:"state"`
}

// BrowserAccessibilityParams describes accessibility extraction.
type BrowserAccessibilityParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	PageID      string `json:"page_id,omitempty"`
	MaxDepth    int    `json:"max_depth,omitempty"`
}

// BrowserChallengeParams describes manual challenge detection.
type BrowserChallengeParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	PageID      string `json:"page_id,omitempty"`
}

// BrowserDownloadParams describes a persistent download capture.
type BrowserDownloadParams struct {
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
	PageID      string `json:"page_id,omitempty"`
	Selector    string `json:"selector"`
	Path        string `json:"path"`
}

// BrowserScreenshotParams describes a stateless screenshot.
type BrowserScreenshotParams struct {
	URL       string            `json:"url"`
	Path      string            `json:"path"`
	FullPage  bool              `json:"full_page,omitempty"`
	MaxBytes  int64             `json:"max_bytes,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
}

// BrowserPDFParams describes a stateless PDF capture.
type BrowserPDFParams struct {
	URL      string `json:"url"`
	Path     string `json:"path"`
	MaxBytes int64  `json:"max_bytes,omitempty"`
}

// BrowserMetadataParams describes stateless response metadata capture.
type BrowserMetadataParams struct {
	URL            string `json:"url"`
	MaxHeaderBytes int    `json:"max_header_bytes,omitempty"`
}

// BrowserStatusResult describes persistent browser state.
type BrowserStatusResult struct {
	Status       string `json:"status"`
	Scope        string `json:"scope"`
	DebugURL     string `json:"debug_url,omitempty"`
	ChromePID    int    `json:"chrome_pid,omitempty"`
	Owned        bool   `json:"owned"`
	DataDir      string `json:"data_dir,omitempty"`
	ActivePageID string `json:"active_page_id,omitempty"`
}

// BrowserPageResult describes one persistent page.
type BrowserPageResult struct {
	ID          string `json:"id"`
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Active      bool   `json:"active"`
	ProfileName string `json:"profile_name,omitempty"`
	SessionName string `json:"session_name,omitempty"`
}

// BrowserAccessibilityResult contains accessibility nodes.
type BrowserAccessibilityResult struct {
	Nodes []BrowserAccessibilityNode `json:"nodes"`
}

// BrowserAccessibilityNode is one accessibility node.
type BrowserAccessibilityNode struct {
	Role string `json:"role,omitempty"`
	Name string `json:"name,omitempty"`
}

// BrowserChallengeResult reports manual challenge state.
type BrowserChallengeResult struct {
	ManualRequired bool     `json:"manual_required"`
	Kind           string   `json:"kind,omitempty"`
	Solved         bool     `json:"solved"`
	Warnings       []string `json:"warnings"`
}

// BrowserMetadataResult contains bounded response metadata.
type BrowserMetadataResult struct {
	URL         string              `json:"url"`
	Status      int                 `json:"status,omitempty"`
	ContentType string              `json:"content_type,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
}

// BrowserDownloadResult contains a captured download summary.
type BrowserDownloadResult struct {
	Path        string `json:"path"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type,omitempty"`
}
