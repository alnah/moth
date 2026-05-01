package browser

import (
	"net/url"
	"sort"
	"strings"
)

const defaultMaxHeaderBytes = 4096

func normalizeResponseMetadata(metadata ResponseMetadata, request ResponseMetadataRequest) ResponseMetadata {
	if metadata.URL == "" {
		metadata.URL = request.URL
	}
	metadata.URL = urlWithoutFragment(metadata.URL)
	metadata.ContentType = strings.TrimSpace(metadata.ContentType)
	metadata.Headers = boundedHeaders(metadata.Headers, headerByteLimit(request.MaxHeaderBytes))
	return metadata
}

func urlWithoutFragment(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parsedURL.Fragment = ""
	return parsedURL.String()
}

func headerByteLimit(limit int) int {
	if limit > 0 {
		return limit
	}
	return defaultMaxHeaderBytes
}

func boundedHeaders(headers map[string][]string, maxBytes int) map[string][]string {
	if len(headers) == 0 || maxBytes <= 0 {
		return nil
	}

	keys := make([]string, 0, len(headers))
	for key := range headers {
		if sensitiveHeader(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(left, right int) bool {
		leftPriority := headerPriority(keys[left])
		rightPriority := headerPriority(keys[right])
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return strings.ToLower(keys[left]) < strings.ToLower(keys[right])
	})

	bounded := map[string][]string{}
	used := 0
	for _, key := range keys {
		name := strings.ToLower(key)
		values := headers[key]
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			entryBytes := len(name) + len(value)
			if used+entryBytes > maxBytes {
				remaining := maxBytes - used - len(name)
				if remaining <= 0 {
					break
				}
				value = value[:min(len(value), remaining)]
				entryBytes = len(name) + len(value)
			}
			bounded[name] = append(bounded[name], value)
			used += entryBytes
			if used >= maxBytes {
				break
			}
		}
		if used >= maxBytes {
			break
		}
	}
	if len(bounded) == 0 {
		return nil
	}
	return bounded
}

func sensitiveHeader(key string) bool {
	switch strings.ToLower(key) {
	case "authorization", "cookie", "proxy-authorization", "set-cookie":
		return true
	default:
		return false
	}
}

func headerPriority(key string) int {
	switch strings.ToLower(key) {
	case "content-type":
		return 0
	case "cache-control":
		return 1
	default:
		return 2
	}
}
