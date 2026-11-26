//go:build !windows

package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStatusErrorMapping(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{status: http.StatusOK},
		{status: http.StatusUnauthorized, want: ErrDaemonAuth},
		{status: http.StatusBadRequest, want: ErrDaemonProtocol},
		{status: http.StatusRequestEntityTooLarge, want: ErrDaemonProtocol},
		{status: http.StatusInternalServerError, want: ErrDaemonUnavailable},
	}
	for _, tt := range tests {
		err := statusError(tt.status)
		if tt.want == nil {
			if err != nil {
				t.Fatalf("statusError(%d) = %v, want nil", tt.status, err)
			}
			continue
		}
		if !errors.Is(err, tt.want) {
			t.Fatalf("statusError(%d) = %v, want %v", tt.status, err, tt.want)
		}
	}
}

func TestCallReplyErrorMapping(t *testing.T) {
	tests := []struct {
		code string
		want error
	}{
		{code: CodeUnauthorized, want: ErrDaemonAuth},
		{code: CodeProtocolError, want: ErrDaemonProtocol},
		{code: CodeMethodNotFound, want: ErrDaemonProtocol},
		{code: "unknown", want: ErrDaemonUnavailable},
	}
	for _, tt := range tests {
		err := callReplyError(&CallError{Code: tt.code, Message: "boom"})
		if !errors.Is(err, tt.want) {
			t.Fatalf("callReplyError(%q) = %v, want %v", tt.code, err, tt.want)
		}
	}
}

func TestDecodeCallReply(t *testing.T) {
	t.Run("result", func(t *testing.T) {
		var health Health
		err := decodeCallReply(strings.NewReader(`{"result":{"status":"ok"}}`), &health)
		if err != nil {
			t.Fatalf("decodeCallReply(result) error = %v", err)
		}
		if health.Status != "ok" {
			t.Fatalf("health.Status = %q, want ok", health.Status)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		if err := decodeCallReply(strings.NewReader(`{"result":{}}`), nil); err != nil {
			t.Fatalf("decodeCallReply(nil result) error = %v", err)
		}
	})

	t.Run("error reply", func(t *testing.T) {
		err := decodeCallReply(
			strings.NewReader(`{"error":{"code":"daemon_unauthorized","message":"no"}}`),
			nil,
		)
		if !errors.Is(err, ErrDaemonAuth) {
			t.Fatalf("decodeCallReply(error) = %v, want ErrDaemonAuth", err)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		err := decodeCallReply(strings.NewReader(`{}`), &Health{})
		if !errors.Is(err, ErrDaemonProtocol) {
			t.Fatalf("decodeCallReply(empty) = %v, want ErrDaemonProtocol", err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		err := decodeCallReply(strings.NewReader(`{`), nil)
		if !errors.Is(err, ErrDaemonProtocol) {
			t.Fatalf("decodeCallReply(malformed) = %v, want ErrDaemonProtocol", err)
		}
	})
}

func TestDefaultRuntimeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got := defaultRuntimeDir(); got != filepath.Join(home, ".moth", "run") {
		t.Fatalf("defaultRuntimeDir() = %q, want home runtime dir", got)
	}
}

func TestServerShutdownRequestsStop(t *testing.T) {
	server := NewServer(ServerOptions{Version: "v1", Token: "token"})
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case <-server.shutdownCh:
	default:
		t.Fatal("Shutdown() did not close shutdown channel")
	}
}

func TestEndpointRuntimeDeadLocked(t *testing.T) {
	runtimeDir := t.TempDir()
	client := &Client{
		options:  ClientOptions{RuntimeDir: runtimeDir},
		endpoint: &Endpoint{Network: "unix", Address: SocketPath(runtimeDir), Token: "token"},
	}
	if err := writeRuntime(runtimeDir, Runtime{
		Network: "unix",
		Address: SocketPath(runtimeDir),
		Token:   "token",
		PID:     999999,
	}); err != nil {
		t.Fatalf("write dead runtime: %v", err)
	}
	if !client.endpointRuntimeDeadLocked() {
		t.Fatal("endpointRuntimeDeadLocked() = false, want true for dead pid")
	}

	if err := writeRuntime(runtimeDir, Runtime{
		Network: "unix",
		Address: SocketPath(runtimeDir),
		Token:   "token",
		PID:     os.Getpid(),
	}); err != nil {
		t.Fatalf("write live runtime: %v", err)
	}
	if client.endpointRuntimeDeadLocked() {
		t.Fatal("endpointRuntimeDeadLocked() = true, want false for live pid")
	}
}

func TestReplaceChangedEndpointLocked(t *testing.T) {
	runtimeDir := t.TempDir()
	client := &Client{
		options:  ClientOptions{RuntimeDir: runtimeDir},
		endpoint: &Endpoint{Network: "unix", Address: "old", Token: "old"},
	}
	if err := writeRuntime(runtimeDir, Runtime{
		Network: "unix",
		Address: "old",
		Token:   "old",
		PID:     os.Getpid(),
	}); err != nil {
		t.Fatalf("write old runtime: %v", err)
	}
	if client.replaceChangedEndpointLocked() {
		t.Fatal("replaceChangedEndpointLocked() = true, want false for same endpoint")
	}

	if err := writeRuntime(runtimeDir, Runtime{
		Network: "unix",
		Address: "new",
		Token:   "new",
		PID:     os.Getpid(),
	}); err != nil {
		t.Fatalf("write new runtime: %v", err)
	}
	if !client.replaceChangedEndpointLocked() {
		t.Fatal("replaceChangedEndpointLocked() = false, want true for new endpoint")
	}
	if client.endpoint == nil || client.endpoint.Address != "new" || client.endpoint.Token != "new" {
		t.Fatalf("client endpoint = %#v, want new endpoint", client.endpoint)
	}
}

func TestNewHTTPClientUsesConfiguredClient(t *testing.T) {
	configured := &http.Client{Timeout: time.Second}
	client := &Client{options: ClientOptions{HTTPClient: configured}}
	if got := client.newHTTPClientLocked(); got != configured {
		t.Fatalf("newHTTPClientLocked() = %p, want configured client %p", got, configured)
	}
}

func TestWriteRuntimeRejectsInvalidTargets(t *testing.T) {
	t.Run("directory is file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if err := writeRuntime(path, Runtime{}); err == nil {
			t.Fatal("writeRuntime(file) error = nil, want error")
		}
	})

	t.Run("temp path is directory", func(t *testing.T) {
		runtimeDir := t.TempDir()
		if err := os.Mkdir(RuntimePath(runtimeDir)+".tmp", 0o700); err != nil {
			t.Fatalf("create temp directory: %v", err)
		}
		if err := writeRuntime(runtimeDir, Runtime{}); err == nil {
			t.Fatal("writeRuntime(temp directory) error = nil, want error")
		}
	})
}

func TestReadRuntimeRejectsMissingFields(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.WriteFile(RuntimePath(runtimeDir), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write runtime: %v", err)
	}
	if _, err := ReadRuntime(runtimeDir); err == nil {
		t.Fatal("ReadRuntime(empty fields) error = nil, want error")
	}
}

func TestServerHealthResultEncoding(t *testing.T) {
	server := NewServer(ServerOptions{
		Version:   "v1",
		Token:     "token",
		StartedAt: time.Unix(1, 0),
	})
	result, err := marshalResult(server.health())
	if err != nil {
		t.Fatalf("marshalResult() error = %v", err)
	}
	var health Health
	if err := json.Unmarshal(result, &health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Version != "v1" || health.Status != "ok" {
		t.Fatalf("health = %#v, want v1 ok", health)
	}
}

func TestProbeLockedReplacesChangedEndpoint(t *testing.T) {
	runtimeDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	address := server.Listener.Addr().String()
	client := &Client{
		options:  ClientOptions{RuntimeDir: runtimeDir},
		endpoint: &Endpoint{Network: "tcp", Address: address, Token: "old-token"},
	}
	if err := writeRuntime(runtimeDir, Runtime{
		Network: "tcp",
		Address: address,
		Token:   "new-token",
		PID:     os.Getpid(),
	}); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	_, found, retry, err := client.probeLocked(context.Background())
	if err != nil || found || !retry {
		t.Fatalf("probeLocked() = found=%v retry=%v err=%v, want retry", found, retry, err)
	}
	if client.endpoint == nil || client.endpoint.Token != "new-token" {
		t.Fatalf("endpoint = %#v, want new token", client.endpoint)
	}
}

func TestProbeLockedClearsDeadEndpoint(t *testing.T) {
	runtimeDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	address := server.Listener.Addr().String()
	client := &Client{
		options:  ClientOptions{RuntimeDir: runtimeDir},
		endpoint: &Endpoint{Network: "tcp", Address: address, Token: "token"},
	}
	if err := writeRuntime(runtimeDir, Runtime{
		Network: "tcp",
		Address: address,
		Token:   "token",
		PID:     999999,
	}); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	_, found, retry, err := client.probeLocked(context.Background())
	if err != nil || found || retry {
		t.Fatalf("probeLocked() = found=%v retry=%v err=%v, want start", found, retry, err)
	}
	if client.endpoint != nil {
		t.Fatalf("endpoint = %#v, want cleared", client.endpoint)
	}
}

func TestWaitForDaemonHonorsCanceledContext(t *testing.T) {
	client := &Client{options: ClientOptions{RuntimeDir: t.TempDir()}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.waitForDaemonLocked(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForDaemonLocked(canceled) = %v, want context.Canceled", err)
	}
}

func TestCallLockedErrors(t *testing.T) {
	t.Run("missing endpoint", func(t *testing.T) {
		client := &Client{}
		err := client.callLocked(context.Background(), MethodHealth, nil)
		if !errors.Is(err, ErrDaemonUnavailable) {
			t.Fatalf("callLocked() = %v, want ErrDaemonUnavailable", err)
		}
	})

	t.Run("nil context", func(t *testing.T) {
		client := &Client{endpoint: &Endpoint{Network: "tcp", Address: "127.0.0.1:1"}}
		//nolint:staticcheck // Exercises the defensive request-construction error path.
		err := client.callLocked(nil, MethodHealth, nil)
		if err == nil {
			t.Fatal("callLocked(nil context) error = nil, want error")
		}
	})

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{name: "server error", status: http.StatusInternalServerError, body: `{}`, wantErr: ErrDaemonUnavailable},
		{name: "empty result", status: http.StatusOK, body: `{}`, wantErr: ErrDaemonProtocol},
		{name: "bad result", status: http.StatusOK, body: `{"result":"bad"}`, wantErr: ErrDaemonProtocol},
		{
			name:    "unknown reply error",
			status:  http.StatusOK,
			body:    `{"error":{"code":"unknown","message":"boom"}}`,
			wantErr: ErrDaemonUnavailable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(tt.status)
				_, _ = writer.Write([]byte(tt.body))
			}))
			t.Cleanup(server.Close)

			client := &Client{
				endpoint: &Endpoint{
					Network: "tcp",
					Address: server.Listener.Addr().String(),
					Token:   "token",
				},
			}
			var health Health
			err := client.callLocked(context.Background(), MethodHealth, &health)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("callLocked() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestShutdownErrorPaths(t *testing.T) {
	t.Run("corrupt runtime", func(t *testing.T) {
		runtimeDir := t.TempDir()
		if err := os.WriteFile(RuntimePath(runtimeDir), []byte("{"), 0o600); err != nil {
			t.Fatalf("write corrupt runtime: %v", err)
		}
		client := NewClient(ClientOptions{RuntimeDir: runtimeDir})
		if err := client.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown(corrupt) error = %v", err)
		}
		if _, err := os.Stat(RuntimePath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runtime after Shutdown(corrupt) err = %v, want removed", err)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		runtimeDir := t.TempDir()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(server.Close)
		if err := writeRuntime(runtimeDir, Runtime{
			Network: "tcp",
			Address: server.Listener.Addr().String(),
			Token:   "token",
		}); err != nil {
			t.Fatalf("write runtime: %v", err)
		}
		client := NewClient(ClientOptions{RuntimeDir: runtimeDir})
		if err := client.Shutdown(context.Background()); !errors.Is(err, ErrDaemonAuth) {
			t.Fatalf("Shutdown(unauthorized) = %v, want ErrDaemonAuth", err)
		}
	})

	t.Run("call error removes runtime", func(t *testing.T) {
		runtimeDir := t.TempDir()
		if err := writeRuntime(runtimeDir, Runtime{
			Network: "tcp",
			Address: "127.0.0.1:1",
			Token:   "token",
		}); err != nil {
			t.Fatalf("write runtime: %v", err)
		}
		client := NewClient(ClientOptions{RuntimeDir: runtimeDir})
		if err := client.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown(call error) error = %v", err)
		}
		if _, err := os.Stat(RuntimePath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runtime after Shutdown(call error) err = %v, want removed", err)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		runtimeDir := t.TempDir()
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)
		if err := writeRuntime(runtimeDir, Runtime{
			Network: "tcp",
			Address: server.Listener.Addr().String(),
			Token:   "token",
		}); err != nil {
			t.Fatalf("write runtime: %v", err)
		}
		client := NewClient(ClientOptions{RuntimeDir: runtimeDir})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := client.Shutdown(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Shutdown(canceled) = %v, want context.Canceled", err)
		}
	})
}

func TestListenRejectsLongSocketPath(t *testing.T) {
	runtimeDir := filepath.Join(t.TempDir(), strings.Repeat("x", 200))
	if _, _, _, err := listen(context.Background(), runtimeDir); err == nil {
		t.Fatal("listen(long path) error = nil, want error")
	}
}

func TestAcquireRuntimeLockRejectsInvalidDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := acquireRuntimeLock(path); err == nil {
		t.Fatal("acquireRuntimeLock(file) error = nil, want error")
	}
}

func TestProcessAliveRejectsInvalidPID(t *testing.T) {
	if processAlive(0) {
		t.Fatal("processAlive(0) = true, want false")
	}
}

func TestSetEndpointLockedKeepsSameEndpoint(t *testing.T) {
	runtimeDir := t.TempDir()
	client := &Client{options: ClientOptions{RuntimeDir: runtimeDir}}
	runtime := Runtime{Network: "unix", Address: "socket", Token: "token"}
	client.setEndpointLocked(runtime)
	first := client.httpClient
	client.setEndpointLocked(runtime)
	if client.httpClient != first {
		t.Fatal("setEndpointLocked(same) replaced HTTP client")
	}
}

func TestAuthorizedRejectsEmptyToken(t *testing.T) {
	server := NewServer(ServerOptions{})
	request := httptest.NewRequest(http.MethodPost, CallPath, strings.NewReader(`{}`))
	if server.authorized(request) {
		t.Fatal("authorized(empty token) = true, want false")
	}
}

func TestHandleCallRejectsMissingMethod(t *testing.T) {
	server := NewServer(ServerOptions{Version: "v1", Token: "token"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, CallPath, strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer token")
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing method status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestDefaultRuntimeDirFallback(t *testing.T) {
	t.Setenv("HOME", "")
	want := filepath.Join(os.TempDir(), ".moth", "run")
	if got := defaultRuntimeDir(); got != want {
		t.Fatalf("defaultRuntimeDir() = %q, want %q", got, want)
	}
}

func TestRunUsesDefaultRuntimeDir(t *testing.T) {
	home, err := os.MkdirTemp("", "moth-home-")
	if err != nil {
		t.Fatalf("create home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	t.Setenv("HOME", home)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, RunOptions{Version: "v1"})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("daemon did not stop")
		}
	})

	runtimeDir := filepath.Join(home, ".moth", "run")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, readErr := ReadRuntime(runtimeDir); readErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("runtime file in %q not ready", runtimeDir)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWriteRuntimeRejectsNonEmptyTargetDirectory(t *testing.T) {
	runtimeDir := t.TempDir()
	target := RuntimePath(runtimeDir)
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "child"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write child: %v", err)
	}
	if err := writeRuntime(runtimeDir, Runtime{}); err == nil {
		t.Fatal("writeRuntime(non-empty target) error = nil, want error")
	}
}

func TestListenRejectsUnremovableStalePath(t *testing.T) {
	runtimeDir := t.TempDir()
	socketPath := SocketPath(runtimeDir)
	if err := os.MkdirAll(socketPath, 0o700); err != nil {
		t.Fatalf("create socket directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(socketPath, "child"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write child: %v", err)
	}
	if _, _, _, err := listen(context.Background(), runtimeDir); err == nil {
		t.Fatal("listen(stale directory) error = nil, want error")
	}
}

func TestEndpointRuntimeDeadLockedMissingRuntime(t *testing.T) {
	client := &Client{
		options:  ClientOptions{RuntimeDir: t.TempDir()},
		endpoint: &Endpoint{Network: "unix", Address: "socket", Token: "token"},
	}
	if !client.endpointRuntimeDeadLocked() {
		t.Fatal("endpointRuntimeDeadLocked(missing runtime) = false, want true")
	}
}

func TestEnsureLockedWithoutCommand(t *testing.T) {
	client := NewClient(ClientOptions{RuntimeDir: t.TempDir()})
	_, err := client.Health(context.Background())
	if !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("Health(no command) = %v, want ErrDaemonUnavailable", err)
	}
}

func TestNewHTTPClientRejectsMissingEndpoint(t *testing.T) {
	client := &Client{}
	httpClient := client.newHTTPClientLocked()
	transport, ok := httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("HTTP transport type = %T, want *http.Transport", httpClient.Transport)
	}
	conn, err := transport.DialContext(context.Background(), "", "")
	if conn != nil {
		_ = conn.Close()
	}
	if !errors.Is(err, ErrDaemonUnavailable) {
		t.Fatalf("DialContext() error = %v, want ErrDaemonUnavailable", err)
	}
}
