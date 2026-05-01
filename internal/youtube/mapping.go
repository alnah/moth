package youtube

import "github.com/alnah/moth/internal/content"

const youtubeWatchBaseURL = "https://www.youtube.com/watch?v="

func mapSearchItems(results []youtubeSearchResult) []content.Item {
	items := make([]content.Item, 0, len(results))
	for _, result := range results {
		if result.ID.Kind != "youtube#video" || result.ID.VideoID == "" {
			continue
		}
		items = append(items, content.Item{
			Kind:     content.KindVideo,
			URL:      youtubeWatchBaseURL + result.ID.VideoID,
			Title:    result.Snippet.Title,
			Text:     result.Snippet.Description,
			Metadata: searchItemMetadata(result),
		})
	}

	return items
}

func mapVideoItems(videos []youtubeVideo) []content.Item {
	items := make([]content.Item, 0, len(videos))
	for _, video := range videos {
		if video.ID == "" {
			continue
		}
		items = append(items, content.Item{
			Kind:     content.KindVideo,
			URL:      youtubeWatchBaseURL + video.ID,
			Title:    video.Snippet.Title,
			Text:     video.Snippet.Description,
			Metadata: videoItemMetadata(video),
		})
	}

	return items
}

func searchItemMetadata(result youtubeSearchResult) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "video_id", result.ID.VideoID)
	addSnippetMetadata(metadata, result.Snippet)

	return metadata
}

func videoItemMetadata(video youtubeVideo) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "video_id", video.ID)
	addSnippetMetadata(metadata, video.Snippet)
	addStringMetadata(metadata, "duration", video.ContentDetails.Duration)
	addStringMetadata(metadata, "view_count", video.Statistics.ViewCount)

	return metadata
}

func addSnippetMetadata(metadata map[string]any, snippet youtubeSnippet) {
	addStringMetadata(metadata, "channel_id", snippet.ChannelID)
	addStringMetadata(metadata, "channel_title", snippet.ChannelTitle)
	addStringMetadata(metadata, "published_at", snippet.PublishedAt)
	addStringMetadata(metadata, "thumbnail_url", thumbnailURL(snippet.Thumbnails))
}

func thumbnailURL(thumbnails map[string]youtubeThumbnail) string {
	for _, name := range []string{"maxres", "standard", "high", "medium", "default"} {
		if thumbnail := thumbnails[name]; thumbnail.URL != "" {
			return thumbnail.URL
		}
	}

	return ""
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
