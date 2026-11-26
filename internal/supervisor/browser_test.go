//go:build !windows

//nolint:gosec,lll // Server tests read temporary files and keep request tables explicit.
package supervisor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
)

func TestServerBrowserStartRequiresMetadata(t *testing.T) {
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connection := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	server, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	_ = server

	reply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, nil)
	if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
		t.Fatalf("start without metadata error = %#v, want %q", reply.Error, supervisor.CodeProtocolError)
	}
}

func TestServerBrowserStartRejectsRelativeStateDirs(t *testing.T) {
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connection := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	cwd, dirs := browserTestStateDirs(t)

	reply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, browserRequestMetadata(t, cwd, ".moth/browser", dirs.Global))
	if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
		t.Fatalf("start with relative state dir error = %#v, want %q", reply.Error, supervisor.CodeProtocolError)
	}
}

func TestServerBrowserStartCreatesServiceAndReportsRunning(t *testing.T) {
	cwd, dirs := browserTestStateDirs(t)
	var gotOptions browser.PersistentServiceOptions
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connection := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	factory := func(options browser.PersistentServiceOptions) *browser.PersistentService {
		gotOptions = options
		options.Launcher = launcher
		options.Connector = connector
		return browser.NewPersistentService(options)
	}
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		BrowserBin:               "/opt/chrome",
		PersistentServiceFactory: factory,
	})

	reply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, browserRequestMetadata(t, cwd, dirs.Local, dirs.Global))
	result := decodeBrowserResult[supervisor.BrowserStatusResult](t, reply)
	if result.Status != "running" || !result.Owned || result.ChromePID != 4242 {
		t.Fatalf("browser.start = %#v, want running owned browser", result)
	}
	if result.DataDir != filepath.Join(dirs.Local, "chrome-data") {
		t.Fatalf("browser.start data dir = %q, want local chrome-data", result.DataDir)
	}
	if gotOptions.BrowserBin != "/opt/chrome" {
		t.Fatalf("factory BrowserBin = %q, want /opt/chrome", gotOptions.BrowserBin)
	}
	if gotOptions.StateDirs != dirs {
		t.Fatalf("factory StateDirs = %#v, want %#v", gotOptions.StateDirs, dirs)
	}
	if _, err := os.Stat(filepath.Join(dirs.Local, "state.json")); err != nil {
		t.Fatalf("state after browser.start error = %v, want file", err)
	}

	healthReply := callBrowserServer(t, httpServer, supervisor.MethodHealth, nil, nil)
	health := decodeBrowserResult[supervisor.Health](t, healthReply)
	if health.ActiveBrowsers != 1 {
		t.Fatalf("health.ActiveBrowsers = %d, want 1", health.ActiveBrowsers)
	}
}

func TestServerBrowserOpenPagesAndDownloadRoundTrip(t *testing.T) {
	cwd, dirs := browserTestStateDirs(t)
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connection := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	headers := browserRequestMetadata(t, cwd, dirs.Local, dirs.Global)

	startReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, headers)
	decodeBrowserResult[supervisor.BrowserStatusResult](t, startReply)

	openReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserOpen, supervisor.BrowserOpenParams{
		URL: "https://example.test/page",
	}, headers)
	page := decodeBrowserResult[supervisor.BrowserPageResult](t, openReply)
	if page.ID != "page-1" || page.URL != "https://example.test/page" || !page.Active {
		t.Fatalf("browser.open = %#v, want active page", page)
	}

	pagesReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserPages, supervisor.BrowserSessionParams{}, headers)
	pages := decodeBrowserResult[[]supervisor.BrowserPageResult](t, pagesReply)
	if len(pages) != 1 || pages[0].ID != "page-1" {
		t.Fatalf("browser.pages = %#v, want one page", pages)
	}

	downloadReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserDownload, supervisor.BrowserDownloadParams{
		Selector: "#archive",
		Path:     "archive.bin",
	}, headers)
	download := decodeBrowserResult[supervisor.BrowserDownloadResult](t, downloadReply)
	if download.Path != "archive.bin" || download.Bytes != int64(len("download-data")) {
		t.Fatalf("browser.download = %#v, want display path and bytes", download)
	}
	data, err := os.ReadFile(filepath.Join(cwd, "archive.bin"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "download-data" {
		t.Fatalf("downloaded data = %q, want download-data", data)
	}
}

func TestServerBrowserMapsStateErrors(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
		code  string
	}{
		{
			name: "unavailable",
			code: supervisor.CodeBrowserStateUnavailable,
		},
		{
			name: "corrupt",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.MkdirAll(dir, 0o750); err != nil {
					t.Fatalf("create state dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{"), 0o600); err != nil {
					t.Fatalf("write corrupt state: %v", err)
				}
			},
			code: supervisor.CodeBrowserStateCorrupt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cwd, dirs := browserTestStateDirs(t)
			if tt.setup != nil {
				tt.setup(t, dirs.Local)
			}
			_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
				PersistentServiceFactory: newBrowserTestFactory(
					&browserTestLauncher{},
					newBrowserTestConnector(nil),
					nil,
				),
			})
			reply := callBrowserServer(t, httpServer, supervisor.MethodBrowserOpen, supervisor.BrowserOpenParams{
				URL: "https://example.test/page",
			}, browserRequestMetadata(t, cwd, dirs.Local, dirs.Global))
			if reply.Error == nil || reply.Error.Code != tt.code {
				t.Fatalf("browser.open error = %#v, want %q", reply.Error, tt.code)
			}
		})
	}
}

func TestServerBrowserRejectsInvalidParamsAndMissingCWD(t *testing.T) {
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{})
	cwd, dirs := browserTestStateDirs(t)

	tests := []struct {
		name    string
		method  string
		params  any
		headers map[string]string
	}{
		{
			name:    "missing cwd",
			method:  supervisor.MethodBrowserStart,
			params:  supervisor.BrowserStartParams{Scope: "local"},
			headers: map[string]string{},
		},
		{
			name:    "open missing url",
			method:  supervisor.MethodBrowserOpen,
			params:  supervisor.BrowserOpenParams{},
			headers: browserRequestMetadata(t, cwd, dirs.Local, dirs.Global),
		},
		{
			name:    "click missing selector",
			method:  supervisor.MethodBrowserClick,
			params:  supervisor.BrowserInteractionParams{},
			headers: browserRequestMetadata(t, cwd, dirs.Local, dirs.Global),
		},
		{
			name:    "screenshot missing cwd",
			method:  supervisor.MethodBrowserScreenshot,
			params:  supervisor.BrowserScreenshotParams{URL: "https://example.test", Path: "shot.png"},
			headers: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := callBrowserServer(t, httpServer, tt.method, tt.params, tt.headers)
			if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
				t.Fatalf("error = %#v, want %q", reply.Error, supervisor.CodeProtocolError)
			}
		})
	}
}

func TestServerBrowserMetadataDoesNotRequireHeaders(t *testing.T) {
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{})
	reply := callBrowserServer(t, httpServer, supervisor.MethodBrowserMetadata, supervisor.BrowserMetadataParams{
		URL: "https://example.test",
	}, nil)
	if reply.Error == nil || reply.Error.Code != supervisor.CodeMethodFailed {
		t.Fatalf("metadata without pool error = %#v, want method failed", reply.Error)
	}
}

func TestServerBrowserReapClosesIdleService(t *testing.T) {
	cwd, dirs := browserTestStateDirs(t)
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connection := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	server, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		Clock: func() time.Time { return now },
		Lifecycle: browser.LifecycleOptions{
			BrowserIdle: time.Minute,
			SessionIdle: time.Minute,
			PageIdle:    time.Minute,
		},
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	headers := browserRequestMetadata(t, cwd, dirs.Local, dirs.Global)

	startReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, headers)
	decodeBrowserResult[supervisor.BrowserStatusResult](t, startReply)

	now = now.Add(2 * time.Minute)
	report, err := server.Reap(context.Background())
	if err != nil {
		t.Fatalf("Reap() error = %v", err)
	}
	if report.ClosedBrowsers != 1 {
		t.Fatalf("Reap report = %#v, want one closed browser", report)
	}
	if !connection.closed {
		t.Fatal("Reap() did not close idle browser")
	}
	if _, err := os.Stat(filepath.Join(dirs.Local, "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state after Reap error = %v, want removed", err)
	}

	healthReply := callBrowserServer(t, httpServer, supervisor.MethodHealth, nil, nil)
	health := decodeBrowserResult[supervisor.Health](t, healthReply)
	if health.ActiveBrowsers != 0 {
		t.Fatalf("health.ActiveBrowsers after Reap = %d, want 0", health.ActiveBrowsers)
	}
}

func TestServerCloseShutsDownPersistentServicesAndIsIdempotent(t *testing.T) {
	cwd, dirs := browserTestStateDirs(t)
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connection := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	server, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	headers := browserRequestMetadata(t, cwd, dirs.Local, dirs.Global)
	startReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, headers)
	decodeBrowserResult[supervisor.BrowserStatusResult](t, startReply)

	if err := server.Close(); err != nil {
		t.Fatalf("Server.Close() error = %v", err)
	}
	if !connection.closed {
		t.Fatal("Server.Close() did not close persistent browser")
	}
	if _, err := os.Stat(filepath.Join(dirs.Local, "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state after Server.Close error = %v, want removed", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Server.Close(second) error = %v, want nil", err)
	}

	reply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, headers)
	if reply.Error == nil || reply.Error.Code != supervisor.CodeMethodFailed {
		t.Fatalf("browser.start after close error = %#v, want method failed", reply.Error)
	}
}

func TestServerBrowserPagesResultIsArray(t *testing.T) {
	connection := newBrowserTestConnection()
	connection.pages = []browser.PageInfo{{ID: "page-1", URL: "https://example.test", Active: true}}
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	connector := newBrowserTestConnector(map[string]*browserTestConnection{"ws://owned": connection})
	cwd, dirs := browserTestStateDirs(t)
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	headers := browserRequestMetadata(t, cwd, dirs.Local, dirs.Global)

	startReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{
		Scope: "local",
	}, headers)
	decodeBrowserResult[supervisor.BrowserStatusResult](t, startReply)
	pagesReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserPages, supervisor.BrowserSessionParams{}, headers)
	raw := pagesReply.Result
	if len(raw) == 0 || raw[0] != '[' {
		t.Fatalf("browser.pages result = %s, want JSON array", raw)
	}
}

func TestServerBrowserPoolHandlersResolveClientPaths(t *testing.T) {
	worker := newFakePoolWorker("")
	worker.screenshot = []byte("image")
	worker.pdf = []byte("pdf")
	worker.metadata = browser.ResponseMetadata{
		URL:         "https://example.test",
		Status:      200,
		ContentType: "text/html",
	}
	pool := newFakePool(worker)
	cwd := t.TempDir()
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{Pool: pool})
	cwdHeaders := map[string]string{"X-Moth-CWD": cwd}

	screenshotReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserScreenshot,
		supervisor.BrowserScreenshotParams{
			URL:      "https://example.test",
			Path:     "shot.png",
			MaxBytes: 1 << 20,
		}, cwdHeaders)
	decodeBrowserResult[struct{}](t, screenshotReply)
	if _, err := os.Stat(filepath.Join(cwd, "shot.png")); err != nil {
		t.Fatalf("screenshot file error = %v, want file under client cwd", err)
	}

	pdfReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserPDF,
		supervisor.BrowserPDFParams{
			URL:      "https://example.test",
			Path:     "page.pdf",
			MaxBytes: 1 << 20,
		}, cwdHeaders)
	decodeBrowserResult[struct{}](t, pdfReply)
	if _, err := os.Stat(filepath.Join(cwd, "page.pdf")); err != nil {
		t.Fatalf("pdf file error = %v, want file under client cwd", err)
	}

	metadataReply := callBrowserServer(t, httpServer, supervisor.MethodBrowserMetadata,
		supervisor.BrowserMetadataParams{URL: "https://example.test"}, nil)
	metadata := decodeBrowserResult[supervisor.BrowserMetadataResult](t, metadataReply)
	if metadata.Status != 200 || metadata.ContentType != "text/html" {
		t.Fatalf("browser.metadata = %#v, want status and content type", metadata)
	}
}
