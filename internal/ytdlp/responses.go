package ytdlp

type ytdlpMetadata struct {
	ID                string                     `json:"id"`
	Title             string                     `json:"title"`
	Description       string                     `json:"description"`
	Duration          int                        `json:"duration"`
	WebpageURL        string                     `json:"webpage_url"`
	Uploader          string                     `json:"uploader"`
	UploadDate        string                     `json:"upload_date"`
	Subtitles         map[string][]ytdlpSubtitle `json:"subtitles"`
	AutomaticCaptions map[string][]ytdlpSubtitle `json:"automatic_captions"`
}

type ytdlpSubtitle struct {
	Ext string `json:"ext"`
	URL string `json:"url"`
}
