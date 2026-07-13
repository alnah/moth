// Package httpclient provides a retrying wrapper around net/http clients.
package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Sleeper waits for retry delays and can be replaced by tests.
type Sleeper interface {
	Sleep(ctx context.Context, delay time.Duration) error
}

// Jitter adjusts a retry delay before sleeping.
type Jitter func(delay time.Duration) time.Duration

// Options configures a retrying HTTP client.
type Options struct {
	HTTPClient        *http.Client
	Attempts          int
	RetryBase         time.Duration
	RetryFactor       float64
	RetryMax          time.Duration
	Jitter            Jitter
	Sleeper           Sleeper
	MaxRetryBodyBytes int64
}

// Client wraps net/http.Client with retry behavior.
type Client struct {
	httpClient *http.Client
	policy     retryPolicy
	sleeper    Sleeper
}

// DefaultOptions returns the standard retry policy for outbound HTTP calls.
func DefaultOptions() Options {
	return Options{
		HTTPClient:        http.DefaultClient,
		Attempts:          defaultAttempts,
		RetryBase:         defaultRetryBase,
		RetryFactor:       defaultRetryFactor,
		RetryMax:          defaultRetryMax,
		Jitter:            defaultJitter,
		Sleeper:           timerSleeper{},
		MaxRetryBodyBytes: defaultMaxRetryBodyBytes,
	}
}

// New creates a client using defaults for unset options.
func New(options Options) *Client {
	defaults := DefaultOptions()
	merged := mergeOptions(defaults, options)

	return &Client{
		httpClient: merged.HTTPClient,
		policy: retryPolicy{
			attempts:          merged.Attempts,
			base:              merged.RetryBase,
			factor:            merged.RetryFactor,
			max:               merged.RetryMax,
			jitter:            merged.Jitter,
			maxRetryBodyBytes: merged.MaxRetryBodyBytes,
		},
		sleeper: merged.Sleeper,
	}
}

// Do sends an HTTP request and retries transient failures.
//
// When Do returns a nil error, the caller owns and must close the response body.
// Intermediate retry response bodies are bounded-drained and closed before retrying.
func (client *Client) Do(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errNilRequest
	}

	attempts := client.policy.attempts
	var lastErr error

	for attempt := range attempts {
		attemptNumber := attempt + 1
		attemptReq, err := requestForAttempt(req, attempt)
		if err != nil {
			return nil, fmt.Errorf("prepare attempt %d/%d: %w", attemptNumber, attempts, err)
		}

		//nolint:gosec // Requests are built by callers; this package only retries supplied requests.
		resp, err := client.httpClient.Do(attemptReq)
		if err != nil {
			lastErr = err
			if !client.shouldRetryError(req, err, attempt) {
				return nil, fmt.Errorf("http request failed on attempt %d/%d: %w", attemptNumber, attempts, err)
			}
		} else {
			if !client.shouldRetryResponse(req, resp, attempt) {
				return resp, nil
			}
			client.policy.closeRetryBody(resp)
		}

		delay := client.policy.delay(attempt, retryAfterDelay(resp))
		if err := client.sleeper.Sleep(req.Context(), delay); err != nil {
			return nil, fmt.Errorf("wait before retry attempt %d/%d: %w", attemptNumber+1, attempts, err)
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("http request failed after %d attempts: %w", attempts, lastErr)
	}

	return nil, fmt.Errorf("http request failed after %d attempts", attempts)
}

func mergeOptions(defaults Options, options Options) Options {
	if options.HTTPClient != nil {
		defaults.HTTPClient = options.HTTPClient
	}
	if options.Attempts > 0 {
		defaults.Attempts = options.Attempts
	}
	if options.RetryBase > 0 {
		defaults.RetryBase = options.RetryBase
	}
	if options.RetryFactor > 0 {
		defaults.RetryFactor = options.RetryFactor
	}
	if options.RetryMax > 0 {
		defaults.RetryMax = options.RetryMax
	}
	if options.Jitter != nil {
		defaults.Jitter = options.Jitter
	}
	if options.Sleeper != nil {
		defaults.Sleeper = options.Sleeper
	}
	if options.MaxRetryBodyBytes > 0 {
		defaults.MaxRetryBodyBytes = options.MaxRetryBodyBytes
	}

	return defaults
}

func requestForAttempt(req *http.Request, attempt int) (*http.Request, error) {
	if attempt == 0 || req.Body == nil || req.Body == http.NoBody {
		return req, nil
	}
	if req.GetBody == nil {
		return nil, fmt.Errorf("retry request body: %w", errBodyNotReplayable)
	}

	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("retry request body: %w", err)
	}
	clone := req.Clone(req.Context())
	clone.Body = body

	return clone, nil
}

func (client *Client) shouldRetryResponse(req *http.Request, resp *http.Response, attempt int) bool {
	return client.policy.hasRetryLeft(attempt) && canReplayRequest(req) && isRetryableStatus(resp.StatusCode)
}

func (client *Client) shouldRetryError(req *http.Request, err error, attempt int) bool {
	if req.Context().Err() != nil {
		return false
	}

	return client.policy.hasRetryLeft(attempt) && canReplayRequest(req) && isRetryableError(err)
}

func canReplayRequest(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

type timerSleeper struct{}

func (timerSleeper) Sleep(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
