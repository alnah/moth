package x

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const responseBodyMax = 4096

type redactedXError struct {
	message string
	err     error
}

func (err redactedXError) Error() string {
	return err.message
}

func (err redactedXError) Unwrap() error {
	return err.err
}

func xTransportError(operation string, err error, bearerToken string) error {
	return redactedXError{
		message: fmt.Sprintf("x %s request: %s", operation, redactXSecret(err.Error(), bearerToken)),
		err:     err,
	}
}

func xStatusError(operation string, resp *http.Response, bearerToken string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, responseBodyMax))
	message := redactXSecret(strings.TrimSpace(string(body)), bearerToken)
	prefix := fmt.Sprintf("x %s failed", operation)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		prefix = fmt.Sprintf("x %s access denied", operation)
	}

	return fmt.Errorf("%s: status %d: %s", prefix, resp.StatusCode, message)
}

func redactXSecret(message string, bearerToken string) string {
	if bearerToken == "" {
		return message
	}
	message = strings.ReplaceAll(message, bearerToken, "[redacted]")
	message = strings.ReplaceAll(message, url.QueryEscape(bearerToken), "[redacted]")
	message = strings.ReplaceAll(message, xBearerTokenPattern+bearerToken, xBearerTokenPattern+"[redacted]")

	return message
}
