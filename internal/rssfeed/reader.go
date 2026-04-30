package rssfeed

import (
	"fmt"
	"io"
)

func readFeed(reader io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxFeedBytes
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read feed: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("read feed over %d bytes", maxBytes)
	}

	return data, nil
}
