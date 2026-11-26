//go:build !windows

package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
)

type browserClientCall struct {
	method  string
	params  json.RawMessage
	headers http.Header
}

type browserClientDaemon struct {
	mu    sync.Mutex
	calls []browserClientCall
}

func newBrowserClient(t *testing.T, handle func(browserClientCall) supervisor.Reply) (*supervisor.Client, *browserClientDaemon) {
	t.Helper()
	daemon := &browserClientDaemon{}
	httpServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-token" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		var call supervisor.Call
		if err := json.NewDecoder(request.Body).Decode(&call); err != nil {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		record := browserClientCall{
			method:  call.Method,
			params:  call.Params,
			headers: request.Header.Clone(),
		}
		daemon.mu.Lock()
		daemon.calls = append(daemon.calls, record)
		daemon.mu.Unlock()

		var reply supervisor.Reply
		if call.Method == supervisor.MethodHealth {
			reply = browserClientReply(t, supervisor.Health{
				Status:  "ok",
				Version: testVersion,
				PID:     os.Getpid(),
			})
		} else {
			reply = handle(record)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(reply)
	}))
	t.Cleanup(httpServer.Close)

	runtimeDir := newRuntimeDir(t)
	host := strings.TrimPrefix(httpServer.URL, "http://")
	runtime := supervisor.Runtime{
		Version:   testVersion,
		PID:       os.Getpid(),
		Network:   "tcp",
		Address:   host,
		Token:     "test-token",
		StartedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(runtime)
	if err != nil {
		t.Fatalf("marshal runtime: %v", err)
	}
	if err := os.WriteFile(supervisor.RuntimePath(runtimeDir), data, 0o600); err != nil {
		t.Fatalf("write runtime: %v", err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", host)
		},
		DisableKeepAlives: true,
	}
	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		WorkingDir: t.TempDir(),
		StateDirs: browser.StateDirs{
			Local:  "local-browser",
			Global: "global-browser",
		},
		HTTPClient: &http.Client{Transport: transport, Timeout: time.Second},
	})
	t.Cleanup(func() {
		_ = client.Close()
		transport.CloseIdleConnections()
	})
	return client, daemon
}

func browserClientReply(t *testing.T, result any) supervisor.Reply {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return supervisor.Reply{Result: data}
}

func browserClientError(code string, message string) supervisor.Reply {
	return supervisor.Reply{Error: &supervisor.CallError{Code: code, Message: message}}
}

func TestClientBrowserMethodsSendMetadataAndRoundTrip(t *testing.T) {
	client, daemon := newBrowserClient(t, func(call browserClientCall) supervisor.Reply {
		switch call.method {
		case supervisor.MethodBrowserStart:
			return browserClientReply(t, supervisor.BrowserStatusResult{
				Status:    "running",
				Scope:     "local",
				ChromePID: 4242,
				Owned:     true,
			})
		case supervisor.MethodBrowserOpen:
			return browserClientReply(t, supervisor.BrowserPageResult{
				ID:     "page-1",
				URL:    "https://example.test/page",
				Active: true,
			})
		case supervisor.MethodBrowserPages:
			return browserClientReply(t, []supervisor.BrowserPageResult{{
				ID:     "page-1",
				URL:    "https://example.test/page",
				Active: true,
			}})
		case supervisor.MethodBrowserDownload:
			return browserClientReply(t, supervisor.BrowserDownloadResult{
				Path:  "archive.bin",
				Bytes: 5,
			})
		default:
			t.Fatalf("unexpected method %q", call.method)
			return supervisor.Reply{}
		}
	})

	status, err := client.Start(context.Background(), browser.StartRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if status.Status != "running" || status.ChromePID != 4242 {
		t.Fatalf("Start() = %#v, want running", status)
	}

	page, err := client.OpenPage(context.Background(), browser.OpenPageRequest{
		URL: "https://example.test/page",
	})
	if err != nil {
		t.Fatalf("OpenPage() error = %v", err)
	}
	if page.ID != "page-1" || page.URL != "https://example.test/page" {
		t.Fatalf("OpenPage() = %#v, want page", page)
	}

	pages, err := client.ListPages(context.Background(), browser.SessionRequest{})
	if err != nil {
		t.Fatalf("ListPages() error = %v", err)
	}
	if len(pages) != 1 || pages[0].ID != "page-1" {
		t.Fatalf("ListPages() = %#v, want one page", pages)
	}

	download, err := client.Download(context.Background(), browser.DownloadRequest{
		Selector: "#archive",
		Path:     "archive.bin",
	})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if download.Path != "archive.bin" || download.Bytes != int64(5) {
		t.Fatalf("Download() = %#v, want display path and int64 bytes", download)
	}

	daemon.mu.Lock()
	calls := append([]browserClientCall(nil), daemon.calls...)
	daemon.mu.Unlock()
	for _, call := range calls {
		if call.method == supervisor.MethodHealth {
			continue
		}
		assertBrowserMetadataHeaders(t, call)
	}
}

func TestClientBrowserPathMethodsSendOnlyCWD(t *testing.T) {
	client, daemon := newBrowserClient(t, func(call browserClientCall) supervisor.Reply {
		switch call.method {
		case supervisor.MethodBrowserScreenshot, supervisor.MethodBrowserPDF:
			return browserClientReply(t, struct{}{})
		case supervisor.MethodBrowserMetadata:
			return browserClientReply(t, supervisor.BrowserMetadataResult{
				URL:    "https://example.test",
				Status: 200,
			})
		default:
			t.Fatalf("unexpected method %q", call.method)
			return supervisor.Reply{}
		}
	})

	if err := client.Screenshot(context.Background(), browser.ScreenshotRequest{
		URL:  "https://example.test",
		Path: "shot.png",
	}); err != nil {
		t.Fatalf("Screenshot() error = %v", err)
	}
	if err := client.PDF(context.Background(), browser.PDFRequest{
		URL:  "https://example.test",
		Path: "page.pdf",
	}); err != nil {
		t.Fatalf("PDF() error = %v", err)
	}
	metadata, err := client.ResponseMetadata(context.Background(), browser.ResponseMetadataRequest{
		URL: "https://example.test",
	})
	if err != nil {
		t.Fatalf("ResponseMetadata() error = %v", err)
	}
	if metadata.Status != 200 {
		t.Fatalf("ResponseMetadata() = %#v, want 200", metadata)
	}

	daemon.mu.Lock()
	calls := append([]browserClientCall(nil), daemon.calls...)
	daemon.mu.Unlock()
	for _, call := range calls {
		switch call.method {
		case supervisor.MethodBrowserScreenshot, supervisor.MethodBrowserPDF:
			if call.headers.Get("X-Moth-CWD") == "" {
				t.Fatalf("%s missing X-Moth-CWD", call.method)
			}
			if call.headers.Get("X-Moth-Local-State-Dir") != "" || call.headers.Get("X-Moth-Global-State-Dir") != "" {
				t.Fatalf("%s sent state dirs, want CWD only", call.method)
			}
		case supervisor.MethodBrowserMetadata:
			if call.headers.Get("X-Moth-CWD") != "" ||
				call.headers.Get("X-Moth-Local-State-Dir") != "" ||
				call.headers.Get("X-Moth-Global-State-Dir") != "" {
				t.Fatal("browser.metadata sent path headers, want none")
			}
		}
	}
}

func TestClientBrowserMapsStateErrors(t *testing.T) {
	tests := []struct {
		code string
		want error
	}{
		{code: supervisor.CodeBrowserStateUnavailable, want: browser.ErrBrowserStateUnavailable},
		{code: supervisor.CodeBrowserStateCorrupt, want: browser.ErrBrowserStateCorrupt},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			client, _ := newBrowserClient(t, func(browserClientCall) supervisor.Reply {
				return browserClientError(tt.code, "state failed")
			})
			_, err := client.OpenPage(context.Background(), browser.OpenPageRequest{URL: "https://example.test"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("OpenPage() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestClientBrowserMapsProtocolAndMethodErrors(t *testing.T) {
	tests := []struct {
		code string
		want error
	}{
		{code: supervisor.CodeProtocolError, want: supervisor.ErrDaemonProtocol},
		{code: supervisor.CodeMethodFailed, want: supervisor.ErrDaemonMethod},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			client, _ := newBrowserClient(t, func(browserClientCall) supervisor.Reply {
				return browserClientError(tt.code, "call failed")
			})
			_, err := client.OpenPage(context.Background(), browser.OpenPageRequest{URL: "https://example.test"})
			if !errors.Is(err, tt.want) {
				t.Fatalf("OpenPage() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestClientBrowserRequestCanceled(t *testing.T) {
	client, _ := newBrowserClient(t, func(browserClientCall) supervisor.Reply {
		time.Sleep(100 * time.Millisecond)
		return browserClientError(supervisor.CodeMethodFailed, "late")
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.OpenPage(ctx, browser.OpenPageRequest{URL: "https://example.test"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenPage(canceled) error = %v, want context.Canceled", err)
	}
}

func assertBrowserMetadataHeaders(t *testing.T, call browserClientCall) {
	t.Helper()
	if call.headers.Get("X-Moth-CWD") == "" {
		t.Fatalf("%s missing X-Moth-CWD", call.method)
	}
	for _, name := range []string{"X-Moth-CWD", "X-Moth-Local-State-Dir", "X-Moth-Global-State-Dir"} {
		value := call.headers.Get(name)
		if value == "" {
			t.Fatalf("%s missing %s", call.method, name)
		}
		if !filepath.IsAbs(value) {
			t.Fatalf("%s %s = %q, want absolute path", call.method, name, value)
		}
	}
}
