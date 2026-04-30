package httpdownload

import (
	"fmt"
	"io"
)

func readBounded(reader io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("download body over %d bytes: %w", maxBytes, ErrFileTooLarge)
	}

	return body, nil
}
