package content

// TypeContentPack identifies normalized Moth content JSON documents.
const TypeContentPack = "content_pack"

// Kind identifies the source media or document category for an item.
type Kind string

// Supported content item kinds.
const (
	KindPage         Kind = "page"
	KindPDF          Kind = "pdf"
	KindImage        Kind = "image"
	KindVideo        Kind = "video"
	KindAudio        Kind = "audio"
	KindPodcast      Kind = "podcast"
	KindSocialPost   Kind = "social_post"
	KindSocialThread Kind = "social_thread"
	KindFeed         Kind = "feed"
	KindFile         Kind = "file"
)

// Warning is a machine-readable Moth technical fact code.
type Warning string

// Supported warning codes emitted by Moth.
const (
	WarningTimeout             Warning = "timeout"
	WarningLoginRequired       Warning = "login_required"
	WarningCaptchaPossible     Warning = "captcha_possible"
	WarningNoTranscriptFound   Warning = "no_transcript_found"
	WarningFileTooLarge        Warning = "file_too_large"
	WarningPartialContent      Warning = "partial_content"
	WarningToolMissing         Warning = "tool_missing"
	WarningProviderRateLimited Warning = "provider_rate_limited"
	WarningOCRUsed             Warning = "ocr_used"
	WarningOCRFailed           Warning = "ocr_failed"
)

// Pack is the normalized JSON payload returned by Moth commands.
type Pack struct {
	Type     string    `json:"type"`
	Items    []Item    `json:"items"`
	Warnings []Warning `json:"warnings"`
}

// Item is one normalized content object in a Pack.
type Item struct {
	Kind       Kind           `json:"kind"`
	URL        string         `json:"url,omitempty"`
	Title      string         `json:"title,omitempty"`
	Text       string         `json:"text,omitempty"`
	Transcript string         `json:"transcript,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Warnings   []Warning      `json:"warnings"`
}
