package ytdlp

import (
	"sort"

	"github.com/alnah/moth/internal/content"
)

func mapMetadata(metadata ytdlpMetadata) content.Item {
	return content.Item{
		Kind:     content.KindVideo,
		URL:      metadata.WebpageURL,
		Title:    metadata.Title,
		Text:     metadata.Description,
		Metadata: metadataMap(metadata),
	}
}

func metadataMap(metadata ytdlpMetadata) map[string]any {
	values := make(map[string]any)
	addStringMetadata(values, "video_id", metadata.ID)
	addIntMetadata(values, "duration_seconds", metadata.Duration)
	addStringMetadata(values, "uploader", metadata.Uploader)
	addStringMetadata(values, "upload_date", metadata.UploadDate)
	addStringSliceMetadata(values, "subtitles", sortedMapKeys(metadata.Subtitles))
	addStringSliceMetadata(values, "automatic_captions", sortedMapKeys(metadata.AutomaticCaptions))

	return values
}

func addStringMetadata(metadata map[string]any, key string, value string) {
	if value != "" {
		metadata[key] = value
	}
}

func addIntMetadata(metadata map[string]any, key string, value int) {
	if value != 0 {
		metadata[key] = value
	}
}

func addStringSliceMetadata(metadata map[string]any, key string, values []string) {
	if len(values) != 0 {
		metadata[key] = values
	}
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
