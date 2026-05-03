package content

// Kind identifies the source media or document category for an item.
type Kind string

// Supported content item kinds.
const (
	KindPage          Kind = "page"
	KindPDF           Kind = "pdf"
	KindImage         Kind = "image"
	KindVideo         Kind = "video"
	KindAudio         Kind = "audio"
	KindPodcast       Kind = "podcast"
	KindSocialPost    Kind = "social_post"
	KindSocialThread  Kind = "social_thread"
	KindSocialProfile Kind = "social_profile"
	KindFeed          Kind = "feed"
	KindFile          Kind = "file"
)

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
