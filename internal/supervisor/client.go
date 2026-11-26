package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// Client talks to the local Moth daemon.
type Client struct {
	options ClientOptions

	mu         sync.Mutex
	endpoint   *Endpoint
	httpClient *http.Client
}

// NewClient creates a daemon client.
func NewClient(options ClientOptions) *Client {
	return &Client{options: options}
}

// Health returns the daemon health, starting the daemon when necessary.
func (client *Client) Health(ctx context.Context) (Health, error) {
	if err := ctx.Err(); err != nil {
		return Health{}, err
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	return client.ensureLocked(ctx)
}

// Shutdown asks the daemon to stop and waits for its runtime file to disappear.
func (client *Client) Shutdown(ctx context.Context) error {
	client.mu.Lock()
	defer client.mu.Unlock()

	runtime, err := ReadRuntime(client.options.RuntimeDir)
	if errors.Is(err, os.ErrNotExist) {
		client.clearEndpointLocked()
		return nil
	}
	if err != nil {
		client.forgetRuntimeLocked()
		return nil
	}

	client.setEndpointLocked(runtime)
	callErr := client.callLocked(ctx, MethodShutdown, nil)
	if errors.Is(callErr, ErrDaemonAuth) {
		return callErr
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if callErr != nil {
		client.forgetRuntimeLocked()
		return nil
	}

	deadline := time.Now().Add(defaultShutdownWait)
	for {
		_, statErr := os.Stat(RuntimePath(client.options.RuntimeDir))
		if errors.Is(statErr, os.ErrNotExist) {
			client.clearEndpointLocked()
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: shutdown timed out", ErrDaemonUnavailable)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Close releases idle HTTP connections.
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.clearEndpointLocked()
	return nil
}

func (client *Client) ensureLocked(ctx context.Context) (Health, error) {
	for range daemonProbeAttempts {
		health, found, retry, err := client.probeLocked(ctx)
		if err == nil && found {
			return health, nil
		}
		if retry {
			continue
		}
		if err != nil && isDaemonIdentityError(err) {
			return Health{}, err
		}
		break
	}
	return client.startDaemonLocked(ctx)
}

func (client *Client) probeLocked(ctx context.Context) (Health, bool, bool, error) {
	if client.endpoint != nil {
		health, err := client.healthLocked(ctx)
		if err == nil {
			return health, true, false, nil
		}
		if isDaemonIdentityError(err) {
			if client.replaceChangedEndpointLocked() {
				return Health{}, false, true, nil
			}
			if client.endpointRuntimeDeadLocked() {
				client.clearEndpointLocked()
				return Health{}, false, false, nil
			}
			return Health{}, false, false, err
		}
		client.clearEndpointLocked()
	}

	runtime, err := ReadRuntime(client.options.RuntimeDir)
	if err != nil {
		//nolint:nilerr // Missing or corrupt runtime means there is no daemon to probe.
		return Health{}, false, false, nil
	}
	client.setEndpointLocked(runtime)
	health, healthErr := client.healthLocked(ctx)
	if healthErr == nil {
		return health, true, false, nil
	}
	if isDaemonIdentityError(healthErr) {
		if client.replaceChangedEndpointLocked() {
			return Health{}, false, true, nil
		}
		if !processAlive(runtime.PID) {
			client.clearEndpointLocked()
			return Health{}, false, false, nil
		}
		return Health{}, false, false, healthErr
	}
	client.clearEndpointLocked()
	return Health{}, false, false, healthErr
}

func (client *Client) startDaemonLocked(ctx context.Context) (Health, error) {
	if client.options.CommandFunc == nil {
		return Health{}, fmt.Errorf("%w: no daemon command configured", ErrDaemonUnavailable)
	}
	spec := DaemonSpec{
		RuntimeDir: client.options.RuntimeDir,
		Version:    client.options.Version,
		BrowserBin: client.options.BrowserBin,
	}
	if startErr := client.options.CommandFunc(ctx, spec); startErr != nil {
		return Health{}, fmt.Errorf("%w: %w", ErrDaemonUnavailable, startErr)
	}
	return client.waitForDaemonLocked(ctx)
}

func (client *Client) replaceChangedEndpointLocked() bool {
	current, err := ReadRuntime(client.options.RuntimeDir)
	if err != nil {
		return false
	}
	if client.endpoint != nil &&
		current.Token == client.endpoint.Token &&
		current.Address == client.endpoint.Address {
		return false
	}
	client.setEndpointLocked(current)
	return true
}

func (client *Client) endpointRuntimeDeadLocked() bool {
	current, err := ReadRuntime(client.options.RuntimeDir)
	if err != nil {
		return true
	}
	return !processAlive(current.PID)
}

func (client *Client) waitForDaemonLocked(ctx context.Context) (Health, error) {
	deadline := time.Now().Add(defaultHealthTimeout)
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Health{}, ctxErr
		}

		runtime, err := ReadRuntime(client.options.RuntimeDir)
		if err == nil {
			client.setEndpointLocked(runtime)
			health, healthErr := client.healthLocked(ctx)
			if healthErr == nil {
				return health, nil
			}
			if isDaemonIdentityError(healthErr) && processAlive(runtime.PID) {
				return Health{}, healthErr
			}
			client.clearEndpointLocked()
		}

		if time.Now().After(deadline) {
			return Health{}, fmt.Errorf("%w: daemon did not become ready", ErrDaemonUnavailable)
		}
		select {
		case <-ctx.Done():
			return Health{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (client *Client) healthLocked(ctx context.Context) (Health, error) {
	var health Health
	if err := client.callLocked(ctx, MethodHealth, &health); err != nil {
		return Health{}, err
	}
	if client.options.Version != "" && health.Version != client.options.Version {
		return Health{}, fmt.Errorf(
			"%w: daemon %q, client %q",
			ErrDaemonVersion,
			health.Version,
			client.options.Version,
		)
	}
	return health, nil
}

func (client *Client) callLocked(
	ctx context.Context,
	method string,
	result any,
) error {
	if client.endpoint == nil {
		return fmt.Errorf("%w: missing endpoint", ErrDaemonUnavailable)
	}
	if client.httpClient == nil {
		client.httpClient = client.newHTTPClientLocked()
	}

	payload, err := json.Marshal(Call{Method: method})
	if err != nil {
		return fmt.Errorf("encode daemon call: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://moth"+CallPath,
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("create daemon request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+client.endpoint.Token)

	response, err := client.httpClient.Do(request) //nolint:gosec // Endpoint is a local daemon, not user-controlled SSRF.
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w: %w", ErrDaemonUnavailable, err)
	}
	defer func() { _ = response.Body.Close() }()

	if err := statusError(response.StatusCode); err != nil {
		return err
	}
	return decodeCallReply(response.Body, result)
}

func statusError(status int) error {
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return ErrDaemonAuth
	case http.StatusBadRequest, http.StatusRequestEntityTooLarge:
		return ErrDaemonProtocol
	default:
		return fmt.Errorf("%w: status %d", ErrDaemonUnavailable, status)
	}
}

func decodeCallReply(body io.Reader, result any) error {
	var reply Reply
	if err := json.NewDecoder(io.LimitReader(body, maxRequestBody)).Decode(&reply); err != nil {
		return fmt.Errorf("%w: decode reply: %w", ErrDaemonProtocol, err)
	}
	if reply.Error != nil {
		return callReplyError(reply.Error)
	}
	if result == nil {
		return nil
	}
	if len(reply.Result) == 0 {
		return fmt.Errorf("%w: empty result", ErrDaemonProtocol)
	}
	if err := json.Unmarshal(reply.Result, result); err != nil {
		return fmt.Errorf("%w: decode result: %w", ErrDaemonProtocol, err)
	}
	return nil
}

func callReplyError(callErr *CallError) error {
	switch callErr.Code {
	case CodeUnauthorized:
		return ErrDaemonAuth
	case CodeProtocolError, CodeMethodNotFound:
		return fmt.Errorf("%w: %s", ErrDaemonProtocol, callErr.Message)
	default:
		return fmt.Errorf("%w: %s", ErrDaemonUnavailable, callErr.Message)
	}
}

func isDaemonIdentityError(err error) bool {
	return errors.Is(err, ErrDaemonVersion) || errors.Is(err, ErrDaemonAuth)
}

func (client *Client) setEndpointLocked(runtime Runtime) {
	endpoint := &Endpoint{
		Network: runtime.Network,
		Address: runtime.Address,
		Token:   runtime.Token,
	}
	if client.endpoint != nil && *client.endpoint == *endpoint {
		return
	}
	client.endpoint = endpoint
	if client.httpClient != nil {
		client.httpClient.CloseIdleConnections()
	}
	client.httpClient = client.newHTTPClientLocked()
}

func (client *Client) forgetRuntimeLocked() {
	client.clearEndpointLocked()
	removeRuntime(client.options.RuntimeDir)
}

func (client *Client) clearEndpointLocked() {
	if client.httpClient != nil {
		client.httpClient.CloseIdleConnections()
	}
	client.endpoint = nil
	client.httpClient = nil
}

func (client *Client) newHTTPClientLocked() *http.Client {
	if client.options.HTTPClient != nil {
		return client.options.HTTPClient
	}
	endpoint := client.endpoint
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			if endpoint == nil {
				return nil, fmt.Errorf("%w: missing endpoint", ErrDaemonUnavailable)
			}
			return (&net.Dialer{}).DialContext(ctx, endpoint.Network, endpoint.Address)
		},
		DisableKeepAlives: true,
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}
