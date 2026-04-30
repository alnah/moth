package httpclient_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/alnah/moth/internal/httpclient"
)

func TestDefaultOptionsUseAPIRetryPolicy(t *testing.T) {
	options := httpclient.DefaultOptions()

	if options.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", options.Attempts)
	}
	if options.RetryBase != 500*time.Millisecond {
		t.Fatalf("retry base = %s, want 500ms", options.RetryBase)
	}
	if options.RetryFactor != 2 {
		t.Fatalf("retry factor = %g, want 2", options.RetryFactor)
	}
	if options.RetryMax != 8*time.Second {
		t.Fatalf("retry max = %s, want 8s", options.RetryMax)
	}
	if options.Jitter == nil {
		t.Fatal("jitter = nil, want default jitter for concurrent API clients")
	}
	if options.MaxRetryBodyBytes <= 0 {
		t.Fatalf("max retry body bytes = %d, want positive bounded drain", options.MaxRetryBodyBytes)
	}
}

func TestNoJitterKeepsDelayUnchanged(t *testing.T) {
	for _, delay := range []time.Duration{-time.Second, 0, time.Nanosecond, 500 * time.Millisecond} {
		if got := httpclient.NoJitter(delay); got != delay {
			t.Fatalf("NoJitter(%s) = %s, want unchanged delay", delay, got)
		}
	}
}

func TestDefaultJitterKeepsDelayWithinBoundedRange(t *testing.T) {
	jitter := httpclient.DefaultOptions().Jitter
	if jitter == nil {
		t.Fatal("default jitter = nil, want bounded jitter function")
	}

	for _, delay := range []time.Duration{-time.Second, 0, time.Nanosecond} {
		if got := jitter(delay); got != delay {
			t.Fatalf("default jitter(%s) = %s, want unchanged edge delay", delay, got)
		}
	}

	const delay = 100 * time.Millisecond
	for range 100 {
		got := jitter(delay)
		if got < delay/2 || got > delay {
			t.Fatalf("default jitter(%s) = %s, want within [%s, %s]", delay, got, delay/2, delay)
		}
	}
}

func TestDoReturnsErrorForNilRequest(t *testing.T) {
	transport := newSequenceTransport(response(http.StatusOK, "must not be called"))
	client := httpclient.New(httpclient.Options{
		HTTPClient: &http.Client{Transport: transport},
	})

	resp, err := client.Do(nil)
	if err == nil {
		t.Fatal("do nil request error = nil, want error")
	}
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
		t.Fatalf("do nil request response = %v, want nil", resp)
	}
	if !strings.Contains(err.Error(), "nil request") {
		t.Fatalf("do nil request error = %v, want nil request context", err)
	}
	if transport.calls() != 0 {
		t.Fatalf("round trips = %d, want none", transport.calls())
	}
}

func TestDoRetriesOnlyRetryableStatuses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		retry  bool
	}{
		{name: "408 request timeout", status: http.StatusRequestTimeout, retry: true},
		{name: "409 conflict", status: http.StatusConflict, retry: true},
		{name: "425 too early", status: http.StatusTooEarly, retry: true},
		{name: "429 too many requests", status: http.StatusTooManyRequests, retry: true},
		{name: "500 internal server error", status: http.StatusInternalServerError, retry: true},
		{name: "502 bad gateway", status: http.StatusBadGateway, retry: true},
		{name: "503 service unavailable", status: http.StatusServiceUnavailable, retry: true},
		{name: "504 gateway timeout", status: http.StatusGatewayTimeout, retry: true},
		{name: "200 ok", status: http.StatusOK},
		{name: "202 accepted", status: http.StatusAccepted},
		{name: "300 multiple choices", status: http.StatusMultipleChoices},
		{name: "400 bad request", status: http.StatusBadRequest},
		{name: "401 unauthorized", status: http.StatusUnauthorized},
		{name: "403 forbidden", status: http.StatusForbidden},
		{name: "404 not found", status: http.StatusNotFound},
		{name: "422 unprocessable entity", status: http.StatusUnprocessableEntity},
		{name: "501 not implemented", status: http.StatusNotImplemented},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sleeper := newFakeSleeper()
			firstBody := "original response"
			responses := []any{response(tc.status, firstBody)}
			wantStatus := tc.status
			wantBody := firstBody
			wantRoundTrips := 1
			var wantDelays []time.Duration

			if tc.retry {
				firstBody = "retry later"
				responses[0] = response(tc.status, firstBody)
				responses = append(responses, response(http.StatusOK, "ok"))
				wantStatus = http.StatusOK
				wantBody = "ok"
				wantRoundTrips = 2
				wantDelays = []time.Duration{500 * time.Millisecond}
			}

			transport := newSequenceTransport(responses...)
			client := newDeterministicClient(transport, sleeper)

			resp, err := client.Do(newGETRequest(t, "https://api.example.test/resource"))
			if err != nil {
				t.Fatalf("Client.Do(status %d) error = %v, want nil", tc.status, err)
			}
			defer func() {
				if closeErr := resp.Body.Close(); closeErr != nil {
					t.Fatalf("close response body: %v", closeErr)
				}
			}()

			if resp.StatusCode != wantStatus {
				t.Fatalf("Client.Do(status %d) status = %d, want %d", tc.status, resp.StatusCode, wantStatus)
			}
			if got := readBody(t, resp.Body); got != wantBody {
				t.Fatalf("Client.Do(status %d) body = %q, want %q", tc.status, got, wantBody)
			}
			if transport.calls() != wantRoundTrips {
				t.Fatalf("Client.Do(status %d) round trips = %d, want %d", tc.status, transport.calls(), wantRoundTrips)
			}
			assertRetryDelays(t, sleeper, wantDelays)
		})
	}
}

func TestDoUsesExponentialBackoffAndStopsAtAttemptLimit(t *testing.T) {
	sleeper := newFakeSleeper()
	transport := newSequenceTransport(
		response(http.StatusInternalServerError, "first failure"),
		response(http.StatusBadGateway, "second failure"),
		response(http.StatusServiceUnavailable, "final failure"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          3,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          750 * time.Millisecond,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/exhausted"))
	if err != nil {
		t.Fatalf("do exhausted retryable status request: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want final retryable response 503", resp.StatusCode)
	}
	if got := readBody(t, resp.Body); got != "final failure" {
		t.Fatalf("body = %q, want final retryable response body", got)
	}
	if transport.calls() != 3 {
		t.Fatalf("round trips = %d, want attempt limit", transport.calls())
	}
	assertRetryDelays(t, sleeper, []time.Duration{500 * time.Millisecond, 750 * time.Millisecond})
}

func TestDoUsesRetryAfterHeaderBeforeBackoff(t *testing.T) {
	sleeper := newFakeSleeper()
	transport := newSequenceTransport(
		retryAfterResponse("2"),
		response(http.StatusOK, "ok"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          3,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/rate-limited"))
	if err != nil {
		t.Fatalf("do rate-limited request: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	assertRetryDelays(t, sleeper, []time.Duration{2 * time.Second})
}

func TestDoFallsBackToBackoffForInvalidRetryAfterHeader(t *testing.T) {
	sleeper := newFakeSleeper()
	transport := newSequenceTransport(
		retryAfterResponse("not a date"),
		response(http.StatusOK, "ok"),
	)
	client := newDeterministicClient(transport, sleeper)

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/rate-limited"))
	if err != nil {
		t.Fatalf("do request with invalid retry-after: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	assertRetryDelays(t, sleeper, []time.Duration{500 * time.Millisecond})
}

func TestDoClampsPastRetryAfterHTTPDateToZero(t *testing.T) {
	sleeper := newFakeSleeper()
	pastDate := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	transport := newSequenceTransport(
		retryAfterResponse(pastDate),
		response(http.StatusOK, "ok"),
	)
	client := newDeterministicClient(transport, sleeper)

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/rate-limited"))
	if err != nil {
		t.Fatalf("do request with past retry-after date: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	assertRetryDelays(t, sleeper, []time.Duration{0})
}

func TestDoFallsBackToBackoffWhenRetryAfterHeaderIsMissing(t *testing.T) {
	sleeper := newFakeSleeper()
	transport := newSequenceTransport(
		response(http.StatusTooManyRequests, "rate limited"),
		response(http.StatusOK, "ok"),
	)
	client := newDeterministicClient(transport, sleeper)

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/rate-limited"))
	if err != nil {
		t.Fatalf("do request without retry-after: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	assertRetryDelays(t, sleeper, []time.Duration{500 * time.Millisecond})
}

func TestDoUsesDefaultSleeperWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := newSequenceTransport(
		retryAfterResponse("0"),
		response(http.StatusOK, "must not retry"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         time.Millisecond,
		RetryFactor:       2,
		RetryMax:          time.Millisecond,
		Jitter:            httpclient.NoJitter,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequestWithContext(ctx, t, "https://api.example.test/cancel"))
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do canceled request error = nil, want context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("do canceled request error = %v, want context.Canceled", err)
	}
	if transport.calls() != 1 {
		t.Fatalf("round trips = %d, want no retry after default sleeper sees cancellation", transport.calls())
	}
}

func TestDoUsesDefaultSleeperWithCanceledContextBeforeBackoffRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := newSequenceTransport(
		response(http.StatusInternalServerError, "temporary failure"),
		response(http.StatusOK, "must not retry"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         time.Hour,
		RetryFactor:       2,
		RetryMax:          time.Hour,
		Jitter:            httpclient.NoJitter,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequestWithContext(ctx, t, "https://api.example.test/cancel-backoff"))
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do canceled request error = nil, want context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("do canceled request error = %v, want context.Canceled", err)
	}
	if transport.calls() != 1 {
		t.Fatalf("round trips = %d, want no retry after default sleeper sees cancellation", transport.calls())
	}
}

func TestDoUsesDefaultSleeperWhenNoSleeperOptionIsProvided(t *testing.T) {
	transport := newSequenceTransport(
		response(http.StatusInternalServerError, "temporary failure"),
		response(http.StatusOK, "ok"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         time.Nanosecond,
		RetryFactor:       2,
		RetryMax:          time.Nanosecond,
		Jitter:            httpclient.NoJitter,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/default-sleeper"))
	if err != nil {
		t.Fatalf("do request with default sleeper: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	if transport.calls() != 2 {
		t.Fatalf("round trips = %d, want retry through default sleeper", transport.calls())
	}
}

func TestDoStopsRetriesWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sleeper := newFakeSleeper()
	sleeper.beforeSleep = cancel
	transport := newSequenceTransport(
		response(http.StatusInternalServerError, "temporary failure"),
		response(http.StatusOK, "must not be called"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          3,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequestWithContext(ctx, t, "https://api.example.test/cancel"))
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do canceled request error = nil, want context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("do canceled request error = %v, want context.Canceled", err)
	}
	if transport.calls() != 1 {
		t.Fatalf("round trips = %d, want no retry after cancellation", transport.calls())
	}
	assertRetryDelays(t, sleeper, []time.Duration{500 * time.Millisecond})
}

func TestDoRetriesNetworkResetThenReturnsSuccessfulResponse(t *testing.T) {
	sleeper := newFakeSleeper()
	resetErr := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	transport := newSequenceTransport(
		transportError(resetErr),
		response(http.StatusOK, "ok"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/reset"))
	if err != nil {
		t.Fatalf("do request after connection reset: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if transport.calls() != 2 {
		t.Fatalf("round trips = %d, want retry after connection reset", transport.calls())
	}
	assertRetryDelays(t, sleeper, []time.Duration{500 * time.Millisecond})
}

func TestDoRetriesNetworkTimeoutAndWrapsExhaustedError(t *testing.T) {
	injectedErr := timeoutError{message: "dial timeout"}
	sleeper := newFakeSleeper()
	transport := newSequenceTransport(
		transportError(injectedErr),
		transportError(injectedErr),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/timeout"))
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do exhausted timeout request error = nil, want wrapped timeout")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("do exhausted timeout request error = %v, want it to wrap timeout error", err)
	}
	if transport.calls() != 2 {
		t.Fatalf("round trips = %d, want retry through attempt limit", transport.calls())
	}
	assertRetryDelays(t, sleeper, []time.Duration{500 * time.Millisecond})
}

func TestDoDoesNotRetryCanceledContextBeforeRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	injectedErr := timeoutError{message: "dial timeout"}
	sleeper := newFakeSleeper()
	transport := newSequenceTransport(
		transportError(injectedErr),
		response(http.StatusOK, "must not retry"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequestWithContext(ctx, t, "https://api.example.test/cancel-before"))
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do canceled request error = nil, want wrapped transport error")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("do canceled request error = %v, want wrapped transport error", err)
	}
	if transport.calls() != 1 {
		t.Fatalf("round trips = %d, want no retry for canceled context", transport.calls())
	}
	assertRetryDelays(t, sleeper, nil)
}

func TestDoDoesNotRetryWhenContextIsCanceledAfterTransportError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	injectedErr := timeoutError{message: "dial timeout"}
	sleeper := newFakeSleeper()
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		cancel()
		return nil, injectedErr
	})
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequestWithContext(ctx, t, "https://api.example.test/cancel-after"))
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do canceled request error = nil, want wrapped transport error")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("do canceled request error = %v, want wrapped transport error", err)
	}
	assertRetryDelays(t, sleeper, nil)
}

func TestDoReplaysRequestBodyWithGetBody(t *testing.T) {
	sleeper := newFakeSleeper()
	transport := newBodyCapturingSequenceTransport(
		response(http.StatusInternalServerError, "temporary failure"),
		response(http.StatusOK, "ok"),
	)
	client := newDeterministicClient(transport, sleeper)
	req := newPOSTRequest(t, "https://api.example.test/upload", "payload")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do replayable request: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	if got, want := transport.requestBodies(), []string{"payload", "payload"}; !slices.Equal(got, want) {
		t.Fatalf("request bodies = %q, want replayed bodies %q", got, want)
	}
	if transport.calls() != 2 {
		t.Fatalf("round trips = %d, want retry with replayed body", transport.calls())
	}
}

func TestDoDoesNotRetryNonReplayableRequestBodyAfterTransportError(t *testing.T) {
	resetErr := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	sleeper := newFakeSleeper()
	transport := newBodyCapturingSequenceTransport(
		transportError(resetErr),
		response(http.StatusOK, "must not retry"),
	)
	client := newDeterministicClient(transport, sleeper)
	req := newNonReplayablePOSTRequest(t, "https://api.example.test/upload", "payload")

	resp, err := client.Do(req)
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do non-replayable request error = nil, want wrapped transport error")
	}
	if !errors.Is(err, resetErr) {
		t.Fatalf("do non-replayable request error = %v, want wrapped reset error", err)
	}
	if !strings.Contains(err.Error(), "http request failed") {
		t.Fatalf("do non-replayable request error = %v, want HTTP failure context", err)
	}
	if transport.calls() != 1 {
		t.Fatalf("round trips = %d, want no retry for non-replayable body", transport.calls())
	}
	assertRetryDelays(t, sleeper, nil)
}

func TestDoReturnsWrappedErrorWhenGetBodyFails(t *testing.T) {
	injectedErr := errors.New("reopen body")
	sleeper := newFakeSleeper()
	transport := newSequenceTransport(
		response(http.StatusInternalServerError, "temporary failure"),
		response(http.StatusOK, "must not retry"),
	)
	client := newDeterministicClient(transport, sleeper)
	req := newPOSTRequest(t, "https://api.example.test/upload", "payload")
	req.GetBody = func() (io.ReadCloser, error) {
		return nil, injectedErr
	}

	resp, err := client.Do(req)
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Fatalf("close unexpected response body: %v", closeErr)
			}
		}()
	}
	if err == nil {
		t.Fatal("do request with failing GetBody error = nil, want wrapped error")
	}
	if !errors.Is(err, injectedErr) {
		t.Fatalf("do request with failing GetBody error = %v, want wrapped GetBody error", err)
	}
	if transport.calls() != 1 {
		t.Fatalf("round trips = %d, want stop before second attempt", transport.calls())
	}
}

func TestDoBoundsRetryResponseBodyReads(t *testing.T) {
	largeRetryBody := &countingReadCloser{remaining: 1024 * 1024}
	transport := newSequenceTransport(
		responseWithBody(http.StatusInternalServerError, largeRetryBody),
		response(http.StatusOK, "ok"),
	)
	client := httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          2,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           newFakeSleeper(),
		MaxRetryBodyBytes: 64,
	})

	resp, err := client.Do(newGETRequest(t, "https://api.example.test/large-retry-body"))
	if err != nil {
		t.Fatalf("do request after large retry body: %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if largeRetryBody.bytesRead > 64 {
		t.Fatalf("retry body bytes read = %d, want at most 64", largeRetryBody.bytesRead)
	}
	if !largeRetryBody.closed {
		t.Fatal("retry response body was not closed before retry")
	}
}

func newDeterministicClient(transport http.RoundTripper, sleeper *fakeSleeper) *httpclient.Client {
	return httpclient.New(httpclient.Options{
		HTTPClient:        &http.Client{Transport: transport},
		Attempts:          3,
		RetryBase:         500 * time.Millisecond,
		RetryFactor:       2,
		RetryMax:          8 * time.Second,
		Jitter:            httpclient.NoJitter,
		Sleeper:           sleeper,
		MaxRetryBodyBytes: 64,
	})
}

func newGETRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	return newGETRequestWithContext(context.Background(), t, url)
}

func newGETRequestWithContext(ctx context.Context, t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func newPOSTRequest(t *testing.T, url string, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new POST request: %v", err)
	}
	return req
}

func newNonReplayablePOSTRequest(t *testing.T, url string, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		url,
		io.NopCloser(strings.NewReader(body)),
	)
	if err != nil {
		t.Fatalf("new non-replayable POST request: %v", err)
	}
	return req
}

func response(status int, body string) responseFixture {
	return responseFixture{
		status: status,
		body:   body,
		header: make(http.Header),
	}
}

func retryAfterResponse(value string) responseFixture {
	fixture := response(http.StatusTooManyRequests, "rate limited")
	fixture.header.Set("Retry-After", value)
	return fixture
}

func responseWithBody(status int, body io.ReadCloser) responseFixture {
	return responseFixture{
		status: status,
		header: make(http.Header),
		bodyRC: body,
	}
}

func transportError(err error) errorResponse {
	return errorResponse{err: err}
}

func readBody(t *testing.T, body io.Reader) string {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func assertRetryDelays(t *testing.T, sleeper *fakeSleeper, want []time.Duration) {
	t.Helper()
	if got := sleeper.delays(); !slices.Equal(got, want) {
		t.Fatalf("retry delays = %v, want %v", got, want)
	}
}

type fakeSleeper struct {
	mu          sync.Mutex
	delaysSeen  []time.Duration
	beforeSleep func()
}

func newFakeSleeper() *fakeSleeper {
	return &fakeSleeper{}
}

func (sleeper *fakeSleeper) Sleep(ctx context.Context, delay time.Duration) error {
	sleeper.mu.Lock()
	sleeper.delaysSeen = append(sleeper.delaysSeen, delay)
	sleeper.mu.Unlock()

	if sleeper.beforeSleep != nil {
		sleeper.beforeSleep()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (sleeper *fakeSleeper) delays() []time.Duration {
	sleeper.mu.Lock()
	defer sleeper.mu.Unlock()
	return append([]time.Duration(nil), sleeper.delaysSeen...)
}

type responseFixture struct {
	status int
	body   string
	header http.Header
	bodyRC io.ReadCloser
}

type sequenceTransport struct {
	responses     []any
	callCount     int
	captureBodies bool
	bodies        []string
}

func newSequenceTransport(responses ...any) *sequenceTransport {
	return &sequenceTransport{responses: responses}
}

func newBodyCapturingSequenceTransport(responses ...any) *sequenceTransport {
	return &sequenceTransport{responses: responses, captureBodies: true}
}

func (transport *sequenceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := transport.consumeRequestBody(req); err != nil {
		return nil, err
	}
	if len(transport.responses) == 0 {
		return nil, fmt.Errorf("unexpected request to %s", req.URL)
	}
	transport.callCount++
	next := transport.responses[0]
	transport.responses = transport.responses[1:]

	switch value := next.(type) {
	case responseFixture:
		return value.httpResponse(req), nil
	case errorResponse:
		return nil, value.err
	default:
		return nil, fmt.Errorf("unsupported transport fixture %T", value)
	}
}

func (transport *sequenceTransport) consumeRequestBody(req *http.Request) error {
	if req.Body == nil || req.Body == http.NoBody {
		if transport.captureBodies {
			transport.bodies = append(transport.bodies, "")
		}
		return nil
	}

	var data []byte
	if transport.captureBodies {
		var err error
		data, err = io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("read request body: %w", err)
		}
	} else if _, err := io.Copy(io.Discard, req.Body); err != nil {
		return fmt.Errorf("discard request body: %w", err)
	}
	if err := req.Body.Close(); err != nil {
		return fmt.Errorf("close request body: %w", err)
	}

	if transport.captureBodies {
		transport.bodies = append(transport.bodies, string(data))
	}
	return nil
}

func (transport *sequenceTransport) calls() int {
	return transport.callCount
}

func (transport *sequenceTransport) requestBodies() []string {
	return append([]string(nil), transport.bodies...)
}

func (fixture responseFixture) httpResponse(req *http.Request) *http.Response {
	body := fixture.bodyRC
	if body == nil {
		body = io.NopCloser(strings.NewReader(fixture.body))
	}

	return &http.Response{
		StatusCode: fixture.status,
		Status:     fmt.Sprintf("%d %s", fixture.status, http.StatusText(fixture.status)),
		Header:     fixture.header.Clone(),
		Body:       body,
		Request:    req,
	}
}

type errorResponse struct {
	err error
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type timeoutError struct {
	message string
}

func (err timeoutError) Error() string {
	return err.message
}

func (err timeoutError) Timeout() bool {
	return true
}

func (err timeoutError) Temporary() bool {
	return true
}

var _ net.Error = timeoutError{}

type countingReadCloser struct {
	remaining int64
	bytesRead int64
	closed    bool
}

func (body *countingReadCloser) Read(p []byte) (int, error) {
	if body.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > body.remaining {
		p = p[:body.remaining]
	}
	for index := range p {
		p[index] = 'x'
	}
	body.remaining -= int64(len(p))
	body.bytesRead += int64(len(p))
	return len(p), nil
}

func (body *countingReadCloser) Close() error {
	body.closed = true
	return nil
}
