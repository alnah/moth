package httpclient

import (
	"crypto/rand"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultAttempts          = 3
	defaultRetryBase         = 500 * time.Millisecond
	defaultRetryFactor       = 2
	defaultRetryMax          = 8 * time.Second
	defaultMaxRetryBodyBytes = 1 << 20
)

var (
	errBodyNotReplayable = errors.New("request body cannot be replayed")
	errNilRequest        = errors.New("nil request")
)

// NoJitter keeps a retry delay unchanged.
func NoJitter(delay time.Duration) time.Duration {
	return delay
}

type retryPolicy struct {
	attempts          int
	base              time.Duration
	factor            float64
	max               time.Duration
	jitter            Jitter
	maxRetryBodyBytes int64
}

func (policy retryPolicy) hasRetryLeft(attempt int) bool {
	return attempt < policy.attempts-1
}

func (policy retryPolicy) delay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter >= 0 {
		return retryAfter
	}

	delay := policy.backoff(attempt)
	if policy.jitter == nil {
		return delay
	}

	return policy.jitter(delay)
}

func (policy retryPolicy) backoff(attempt int) time.Duration {
	delay := policy.base
	for range attempt {
		delay = time.Duration(float64(delay) * policy.factor)
		if delay >= policy.max {
			return policy.max
		}
	}
	if delay > policy.max {
		return policy.max
	}

	return delay
}

func (policy retryPolicy) closeRetryBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, policy.maxRetryBodyBytes))
	_ = resp.Body.Close()
}

func retryAfterDelay(resp *http.Response) time.Duration {
	if resp == nil {
		return -1
	}

	header := resp.Header.Get("Retry-After")
	if header == "" {
		return -1
	}
	seconds, err := strconv.ParseInt(header, 10, 64)
	if err == nil {
		if seconds < 0 {
			return 0
		}

		return time.Duration(seconds) * time.Second
	}

	when, err := http.ParseTime(header)
	if err != nil {
		return -1
	}
	until := time.Until(when)
	if until < 0 {
		return 0
	}

	return until
}

func defaultJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return delay
	}

	halfDelay := delay / 2
	if halfDelay <= 0 {
		return delay
	}
	extra, err := randomDuration(halfDelay)
	if err != nil {
		return delay
	}

	return halfDelay + extra
}

func randomDuration(max time.Duration) (time.Duration, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)+1))
	if err != nil {
		return 0, err
	}

	return time.Duration(n.Int64()), nil
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusTooEarly,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func isRetryableError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return errors.Is(err, syscall.ECONNRESET)
}
