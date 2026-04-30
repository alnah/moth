package httpdownload

import (
	"mime"
	"strings"
)

func normalizeContentType(header string) string {
	mediaType, _, err := mime.ParseMediaType(header)
	if err == nil {
		return strings.ToLower(mediaType)
	}

	return strings.ToLower(strings.TrimSpace(header))
}

func contentTypeAllowed(contentType string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if contentType == strings.ToLower(strings.TrimSpace(candidate)) {
			return true
		}
	}

	return false
}
