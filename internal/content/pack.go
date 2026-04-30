package content

// TypeContentPack identifies normalized Moth content JSON documents.
const TypeContentPack = "content_pack"

// Pack is the normalized JSON payload returned by Moth commands.
type Pack struct {
	Type     string    `json:"type"`
	Items    []Item    `json:"items"`
	Warnings []Warning `json:"warnings"`
}
