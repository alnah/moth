package transcription

import "strings"

// mergeChunkResults applies chunk offsets and preserves overlap text without deduplication.
func mergeChunkResults(request normalizedRequest, chunks []Chunk, results []Result) Result {
	texts := make([]string, 0, len(results))
	segments := make([]Segment, 0)
	for index, result := range results {
		if result.Text != "" {
			texts = append(texts, result.Text)
		}
		for _, segment := range result.Segments {
			segments = append(segments, Segment{
				Start: segment.Start + chunks[index].Offset,
				End:   segment.End + chunks[index].Offset,
				Text:  segment.Text,
			})
		}
	}

	return Result{
		Text:     strings.Join(texts, " "),
		Segments: segments,
		Metadata: transcriptionMetadata(request, len(chunks)),
	}
}

func transcriptionMetadata(request normalizedRequest, chunks int) map[string]any {
	return map[string]any{
		"model":           request.Model,
		"response_format": request.ResponseFormat,
		"chunks":          chunks,
	}
}
