package brave

import "github.com/alnah/moth/internal/content"

func mapWebResponseItems(response braveWebResponse) []content.Item {
	return mapResults(response.Web.Results, mapWebResult)
}

func mapImagesResponseItems(response braveImagesResponse) []content.Item {
	return mapResults(response.Results, mapImageResult)
}

func mapVideosResponseItems(response braveVideosResponse) []content.Item {
	return mapResults(response.Results, mapVideoResult)
}

func mapResults[T any](results []T, mapResult func(T) content.Item) []content.Item {
	items := make([]content.Item, 0, len(results))
	for _, result := range results {
		items = append(items, mapResult(result))
	}

	return items
}

func mapWebResult(result braveWebResult) content.Item {
	return content.Item{
		Kind:  content.KindPage,
		URL:   result.URL,
		Title: result.Title,
		Text:  result.Description,
	}
}

func mapImageResult(result braveImageResult) content.Item {
	return content.Item{
		Kind:     content.KindImage,
		URL:      result.Properties.URL,
		Title:    result.Title,
		Text:     result.Description,
		Metadata: imageMetadata(result),
	}
}

func imageMetadata(result braveImageResult) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "page_url", result.URL)
	addStringMetadata(metadata, "thumbnail_url", result.Thumbnail.Src)
	addIntMetadata(metadata, "width", result.Properties.Width)
	addIntMetadata(metadata, "height", result.Properties.Height)

	return metadata
}

func mapVideoResult(result braveVideoResult) content.Item {
	return content.Item{
		Kind:     content.KindVideo,
		URL:      result.URL,
		Title:    result.Title,
		Text:     result.Description,
		Metadata: videoMetadata(result),
	}
}

func videoMetadata(result braveVideoResult) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "thumbnail_url", result.Thumbnail.Src)
	addStringMetadata(metadata, "duration", result.Duration)
	addStringMetadata(metadata, "publisher", result.Publisher)

	return metadata
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
