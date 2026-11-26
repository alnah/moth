//go:build !windows

package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
)

func TestServerFetchRenderedReturnsHTMLAndForwardsRequest(t *testing.T) {
	worker := newFakePoolWorker(`<html><title>ok</title></html>`)
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonWithPool(t, runtimeDir, testVersion, pool)
	runtime := readRuntime(t, runtimeDir)

	reply := callDaemon(t, runtime, supervisor.MethodFetchRendered, supervisor.FetchRenderedParams{
		URL:       "https://example.test/page",
		MaxBytes:  4096,
		Headers:   map[string]string{"Accept-Language": "fr"},
		UserAgent: "Moth/1.0",
	})
	if reply.Error != nil {
		t.Fatalf("fetch rendered error = %#v, want nil", reply.Error)
	}

	result := decodeResult[supervisor.FetchRenderedResult](t, reply)
	if result.URL != "https://example.test/page" {
		t.Fatalf("result.URL = %q, want request URL", result.URL)
	}
	if result.HTML != `<html><title>ok</title></html>` {
		t.Fatalf("result.HTML = %q, want worker HTML", result.HTML)
	}

	request := worker.pageRequest()
	if request.URL != "https://example.test/page" || request.MaxBytes != 4096 {
		t.Fatalf("page request = %#v, want URL and max bytes", request)
	}
	if request.Headers["Accept-Language"] != "fr" || request.UserAgent != "Moth/1.0" {
		t.Fatalf("page request headers = %#v user agent = %q", request.Headers, request.UserAgent)
	}
}

func TestServerFetchRenderedWithoutPoolReportsMethodFailure(t *testing.T) {
	server := supervisor.NewServer(supervisor.ServerOptions{
		Version: testVersion,
		Token:   testToken,
	})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	response := postCallAt(
		t,
		httpServer.Client(),
		httpServer.URL,
		testToken,
		marshalCall(t, supervisor.Call{
			Method: supervisor.MethodFetchRendered,
			Params: rawParams(t, supervisor.FetchRenderedParams{URL: "https://example.test"}),
		}),
	)
	reply := decodeReply(t, response)
	if reply.Error == nil || reply.Error.Code != supervisor.CodeMethodFailed {
		t.Fatalf("reply error = %#v, want code %q", reply.Error, supervisor.CodeMethodFailed)
	}
}

func TestServerFetchRenderedWorkerFailureReportsMethodFailure(t *testing.T) {
	worker := newFakePoolWorker("")
	worker.err = errors.New("worker exploded")
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonWithPool(t, runtimeDir, testVersion, pool)
	runtime := readRuntime(t, runtimeDir)

	reply := callDaemon(t, runtime, supervisor.MethodFetchRendered, supervisor.FetchRenderedParams{
		URL: "https://example.test/page",
	})
	if reply.Error == nil {
		t.Fatal("fetch rendered error = nil, want method failed")
	}
	if reply.Error.Code != supervisor.CodeMethodFailed {
		t.Fatalf("error code = %q, want %q", reply.Error.Code, supervisor.CodeMethodFailed)
	}
	if !strings.Contains(reply.Error.Message, "worker exploded") {
		t.Fatalf("error message = %q, want worker context", reply.Error.Message)
	}
}

func TestServerFetchRenderedRejectsInvalidParams(t *testing.T) {
	worker := newFakePoolWorker("<html></html>")
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonWithPool(t, runtimeDir, testVersion, pool)
	runtime := readRuntime(t, runtimeDir)

	tests := []struct {
		name   string
		params json.RawMessage
	}{
		{name: "missing params"},
		{name: "empty object", params: json.RawMessage(`{}`)},
		{name: "empty url", params: json.RawMessage(`{"url":"  "}`)},
		{name: "unknown field", params: json.RawMessage(`{"url":"https://example.test","extra":1}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := callDaemonRaw(t, runtime, supervisor.MethodFetchRendered, tt.params)
			if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
				t.Fatalf("reply error = %#v, want code %q", reply.Error, supervisor.CodeProtocolError)
			}
		})
	}
}

func TestServerCloseClosesPool(t *testing.T) {
	worker := newFakePoolWorker("<html></html>")
	pool := newFakePool(worker)
	server := supervisor.NewServer(supervisor.ServerOptions{
		Version: testVersion,
		Token:   testToken,
		Pool:    pool,
	})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	response := postCallAt(
		t,
		httpServer.Client(),
		httpServer.URL,
		testToken,
		marshalCall(t, supervisor.Call{
			Method: supervisor.MethodFetchRendered,
			Params: rawParams(t, supervisor.FetchRenderedParams{URL: "https://example.test"}),
		}),
	)
	if reply := decodeReply(t, response); reply.Error != nil {
		t.Fatalf("fetch rendered error = %#v, want nil", reply.Error)
	}

	if err := server.Close(); err != nil {
		t.Fatalf("Server.Close() error = %v", err)
	}
	worker.waitClosed(t)
	if err := server.Close(); err != nil {
		t.Fatalf("Server.Close(second) error = %v", err)
	}
}

func TestClientFetchRendered(t *testing.T) {
	worker := newFakePoolWorker(`<html><title>rendered</title></html>`)
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonWithPool(t, runtimeDir, testVersion, pool)

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() { _ = client.Close() })

	page, err := client.FetchRenderedPage(context.Background(), browser.PageRequest{
		URL:       "https://example.test/page",
		MaxBytes:  2048,
		Headers:   map[string]string{"Accept": "text/html"},
		UserAgent: "Moth/1.0",
	})
	if err != nil {
		t.Fatalf("FetchRenderedPage() error = %v", err)
	}
	if page.URL != "https://example.test/page" || page.HTML != `<html><title>rendered</title></html>` {
		t.Fatalf("FetchRenderedPage() = %#v, want rendered page", page)
	}

	request := worker.pageRequest()
	if request.URL != page.URL || request.MaxBytes != 2048 {
		t.Fatalf("page request = %#v, want URL and max bytes", request)
	}
	if request.Headers["Accept"] != "text/html" || request.UserAgent != "Moth/1.0" {
		t.Fatalf("page request headers = %#v user agent = %q", request.Headers, request.UserAgent)
	}
}

func TestClientFetchRenderedAutoStartsDaemon(t *testing.T) {
	worker := newFakePoolWorker(`<html>auto</html>`)
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)

	var mu sync.Mutex
	var daemons []*inProcessDaemon
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		CommandFunc: func(_ context.Context, spec supervisor.DaemonSpec) error {
			daemon, err := launchDaemonWithPool(spec.RuntimeDir, spec.Version, pool)
			if err != nil {
				return err
			}
			mu.Lock()
			daemons = append(daemons, daemon)
			mu.Unlock()
			return nil
		},
	})
	t.Cleanup(func() {
		_ = client.Close()
		stopDaemons(t, &mu, &daemons)
	})

	page, err := client.FetchRenderedPage(context.Background(), browser.PageRequest{
		URL: "https://example.test/auto",
	})
	if err != nil {
		t.Fatalf("FetchRenderedPage(auto-start) error = %v", err)
	}
	if page.HTML != `<html>auto</html>` {
		t.Fatalf("page.HTML = %q, want auto HTML", page.HTML)
	}
	if len(daemons) != 1 {
		t.Fatalf("daemon starts = %d, want 1", len(daemons))
	}
}

func TestClientFetchRenderedReportsMethodFailure(t *testing.T) {
	worker := newFakePoolWorker("")
	worker.err = errors.New("worker exploded")
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonWithPool(t, runtimeDir, testVersion, pool)

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() { _ = client.Close() })

	_, err := client.FetchRenderedPage(context.Background(), browser.PageRequest{
		URL: "https://example.test/page",
	})
	if !errors.Is(err, supervisor.ErrDaemonMethod) {
		t.Fatalf("FetchRenderedPage() error = %v, want ErrDaemonMethod", err)
	}
}

func TestRunWithInjectedPoolClosesPool(t *testing.T) {
	worker := newFakePoolWorker("<html></html>")
	pool := newFakePool(worker)
	runtimeDir := newRuntimeDir(t)
	daemon := startDaemonWithPool(t, runtimeDir, testVersion, pool)

	if err := daemon.stop(); err != nil {
		t.Fatalf("stop daemon: %v", err)
	}
	worker.waitClosed(t)
}

func TestExportedDefaultRuntimeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".moth", "run")
	if got := supervisor.DefaultRuntimeDir(); got != want {
		t.Fatalf("DefaultRuntimeDir() = %q, want %q", got, want)
	}
}

func callDaemon(
	t *testing.T,
	runtime supervisor.Runtime,
	method string,
	params any,
) supervisor.Reply {
	t.Helper()
	return callDaemonRaw(t, runtime, method, rawParams(t, params))
}

func callDaemonRaw(
	t *testing.T,
	runtime supervisor.Runtime,
	method string,
	params json.RawMessage,
) supervisor.Reply {
	t.Helper()

	client := unixHTTPClient(t, runtime.Address)
	response := postCall(
		t,
		client,
		runtime.Token,
		marshalCall(t, supervisor.Call{Method: method, Params: params}),
	)
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		t.Fatalf("%s status = %d, want %d", method, response.StatusCode, http.StatusOK)
	}
	return decodeReply(t, response)
}

func rawParams(t *testing.T, value any) json.RawMessage {
	t.Helper()
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return data
}

func decodeResult[T any](t *testing.T, reply supervisor.Reply) T {
	t.Helper()
	if reply.Error != nil {
		t.Fatalf("reply error = %#v, want nil", reply.Error)
	}
	var result T
	if err := json.Unmarshal(reply.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	return result
}

func postCallAt(
	t *testing.T,
	client *http.Client,
	baseURL string,
	token string,
	body string,
) *http.Response {
	t.Helper()

	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		baseURL+supervisor.CallPath,
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create call request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	response, err := client.Do(request) //nolint:gosec // Test client dials a local test server.
	if err != nil {
		t.Fatalf("post call: %v", err)
	}
	return response
}
