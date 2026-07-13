package x

import (
	"maps"
	"net/http"
)

func rateLimitMetadata(header http.Header) map[string]any {
	metadata := make(map[string]any)
	addStringMetadata(metadata, "rate_limit_limit", header.Get("X-Rate-Limit-Limit"))
	addStringMetadata(metadata, "rate_limit_remaining", header.Get("X-Rate-Limit-Remaining"))
	addStringMetadata(metadata, "rate_limit_reset", header.Get("X-Rate-Limit-Reset"))
	if len(metadata) == 0 {
		return nil
	}

	return metadata
}

func mergeResponseMetadata(metadata map[string]any, responseMetadata xResponseMeta) map[string]any {
	if responseMetadata.ResultCount == 0 && responseMetadata.NextToken == "" {
		return metadata
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	addIntMetadata(metadata, "result_count", responseMetadata.ResultCount)
	addStringMetadata(metadata, "next_token", responseMetadata.NextToken)

	return metadata
}

func mergeMetadata(left map[string]any, right map[string]any) map[string]any {
	if len(right) == 0 {
		return left
	}
	if left == nil {
		left = make(map[string]any, len(right))
	}
	maps.Copy(left, right)

	return left
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
