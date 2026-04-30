package content

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
