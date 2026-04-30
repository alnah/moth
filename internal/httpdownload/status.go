package httpdownload

import (
	"fmt"
	"net/http"
)

func rejectStatus(statusCode int) error {
	if statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices {
		return nil
	}

	return fmt.Errorf("download HTTP status %d: %w", statusCode, ErrHTTPStatus)
}
