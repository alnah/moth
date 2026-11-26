//go:build !windows

package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/supervisor"
)

func TestServerBrowserConnectRoundTrip(t *testing.T) {
	cwd, dirs := browserTestStateDirs(t)
	ownedBrowser := newBrowserTestConnection()
	externalBrowser := newBrowserTestConnection()
	connector := newBrowserTestConnector(map[string]*browserTestConnection{
		"ws://owned":    ownedBrowser,
		"ws://external": externalBrowser,
	})
	launcher := &browserTestLauncher{result: browser.LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}}
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{
		PersistentServiceFactory: newBrowserTestFactory(launcher, connector, nil),
	})
	headers := browserRequestMetadata(t, cwd, dirs.Local, dirs.Global)

	decodeBrowserResult[supervisor.BrowserStatusResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserStart, supervisor.BrowserStartParams{Scope: "local"}, headers,
	))
	versionServer := newSupervisorVersionServer(t, "ws://external")
	status := decodeBrowserResult[supervisor.BrowserStatusResult](t, callBrowserServer(
		t, httpServer, supervisor.MethodBrowserConnect,
		supervisor.BrowserConnectParams{
			Scope:    "local",
			HostPort: supervisorHostPort(t, versionServer.URL),
		},
		headers,
	))
	if status.Owned {
		t.Fatalf("browser.connect = %#v, want external browser", status)
	}
	if !ownedBrowser.closed {
		t.Fatal("browser.connect did not close previous owned browser")
	}
	if externalBrowser.closed {
		t.Fatal("browser.connect closed external browser")
	}
}

func TestServerBrowserHandlersRequireMetadata(t *testing.T) {
	cwd := t.TempDir()
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{})
	tests := []struct {
		name    string
		method  string
		params  any
		headers map[string]string
	}{
		{"stop", supervisor.MethodBrowserStop, supervisor.BrowserStopParams{}, nil},
		{"status", supervisor.MethodBrowserStatus, supervisor.BrowserStatusParams{}, nil},
		{"connect", supervisor.MethodBrowserConnect, supervisor.BrowserConnectParams{HostPort: "127.0.0.1:9222"}, nil},
		{"switch", supervisor.MethodBrowserSwitch, supervisor.BrowserSelectionParams{PageID: "p1"}, nil},
		{"close page", supervisor.MethodBrowserClosePage, supervisor.BrowserSelectionParams{PageID: "p1"}, nil},
		{"click", supervisor.MethodBrowserClick, supervisor.BrowserInteractionParams{Selector: "button"}, nil},
		{"input", supervisor.MethodBrowserInput, supervisor.BrowserInputParams{Selector: "input", Text: "x"}, nil},
		{"wait", supervisor.MethodBrowserWait, supervisor.BrowserWaitParams{Selector: "main", State: "visible"}, nil},
		{"accessibility", supervisor.MethodBrowserAccessibility, supervisor.BrowserAccessibilityParams{}, nil},
		{"challenge", supervisor.MethodBrowserChallenge, supervisor.BrowserChallengeParams{}, nil},
		{"download", supervisor.MethodBrowserDownload, supervisor.BrowserDownloadParams{Selector: "#a", Path: "a.bin"},
			map[string]string{"X-Moth-CWD": cwd}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := callBrowserServer(t, httpServer, tt.method, tt.params, tt.headers)
			if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
				t.Fatalf("error = %#v, want protocol error", reply.Error)
			}
		})
	}
}

func TestServerBrowserParamErrors(t *testing.T) {
	cwd := t.TempDir()
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{})
	headers := map[string]string{"X-Moth-CWD": cwd}
	tests := []struct {
		name   string
		method string
		params any
	}{
		{"screenshot url", supervisor.MethodBrowserScreenshot, supervisor.BrowserScreenshotParams{Path: "x.png"}},
		{"pdf url", supervisor.MethodBrowserPDF, supervisor.BrowserPDFParams{Path: "x.pdf"}},
		{"metadata url", supervisor.MethodBrowserMetadata, supervisor.BrowserMetadataParams{}},
		{"connect host", supervisor.MethodBrowserConnect, supervisor.BrowserConnectParams{}},
		{"input selector", supervisor.MethodBrowserInput, supervisor.BrowserInputParams{Text: "x"}},
		{"download selector", supervisor.MethodBrowserDownload, supervisor.BrowserDownloadParams{Path: "a.bin"}},
		{"wait selector", supervisor.MethodBrowserWait, supervisor.BrowserWaitParams{State: "visible"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := callBrowserServer(t, httpServer, tt.method, tt.params, headers)
			if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
				t.Fatalf("error = %#v, want protocol error", reply.Error)
			}
		})
	}
}

func TestServerBrowserWaitRejectsInvalidState(t *testing.T) {
	cwd, dirs := browserTestStateDirs(t)
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{})
	reply := callBrowserServer(t, httpServer, supervisor.MethodBrowserWait, supervisor.BrowserWaitParams{
		Selector: "main",
		State:    "detached",
	}, browserRequestMetadata(t, cwd, dirs.Local, dirs.Global))
	if reply.Error == nil || reply.Error.Code != supervisor.CodeProtocolError {
		t.Fatalf("wait invalid state error = %#v, want protocol error", reply.Error)
	}
}

func TestServerBrowserPoolHandlerErrors(t *testing.T) {
	worker := newFakePoolWorker("")
	worker.screenshotErr = errors.New("screenshot failed")
	worker.pdfErr = errors.New("pdf failed")
	worker.metadataErr = errors.New("metadata failed")
	pool := newFakePool(worker)
	cwd := t.TempDir()
	_, httpServer := newBrowserTestServer(t, supervisor.ServerOptions{Pool: pool})
	headers := map[string]string{"X-Moth-CWD": cwd}

	tests := []struct {
		name   string
		method string
		params any
	}{
		{"screenshot", supervisor.MethodBrowserScreenshot,
			supervisor.BrowserScreenshotParams{URL: "https://example.test", Path: "x.png"}},
		{"pdf", supervisor.MethodBrowserPDF,
			supervisor.BrowserPDFParams{URL: "https://example.test", Path: "x.pdf"}},
		{"metadata", supervisor.MethodBrowserMetadata,
			supervisor.BrowserMetadataParams{URL: "https://example.test"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestHeaders map[string]string
			if tt.method != supervisor.MethodBrowserMetadata {
				requestHeaders = headers
			}
			reply := callBrowserServer(t, httpServer, tt.method, tt.params, requestHeaders)
			if reply.Error == nil || reply.Error.Code != supervisor.CodeMethodFailed {
				t.Fatalf("error = %#v, want method failed", reply.Error)
			}
		})
	}
}

func TestClientBrowserMethodsPropagateErrors(t *testing.T) {
	tests := []struct {
		name string
		call func(*supervisor.Client) error
	}{
		{"start", func(client *supervisor.Client) error {
			_, err := client.Start(context.Background(), browser.StartRequest{})
			return err
		}},
		{"stop", func(client *supervisor.Client) error {
			_, err := client.Stop(context.Background(), browser.StopRequest{})
			return err
		}},
		{"status", func(client *supervisor.Client) error {
			_, err := client.Status(context.Background(), browser.StatusRequest{})
			return err
		}},
		{"connect", func(client *supervisor.Client) error {
			_, err := client.Connect(context.Background(), browser.ConnectRequest{HostPort: "127.0.0.1:9222"})
			return err
		}},
		{"open", func(client *supervisor.Client) error {
			_, err := client.OpenPage(context.Background(), browser.OpenPageRequest{URL: "https://example.test"})
			return err
		}},
		{"pages", func(client *supervisor.Client) error {
			_, err := client.ListPages(context.Background(), browser.SessionRequest{})
			return err
		}},
		{"switch", func(client *supervisor.Client) error {
			_, err := client.SwitchPage(context.Background(), browser.PageSelection{})
			return err
		}},
		{"close page", func(client *supervisor.Client) error {
			return client.ClosePage(context.Background(), browser.PageSelection{})
		}},
		{"click", func(client *supervisor.Client) error {
			return client.Click(context.Background(), browser.InteractionRequest{Selector: "button"})
		}},
		{"input", func(client *supervisor.Client) error {
			return client.Input(context.Background(), browser.InputRequest{Selector: "input", Text: "x"})
		}},
		{"wait", func(client *supervisor.Client) error {
			return client.Wait(context.Background(), browser.WaitRequest{Selector: "main", State: browser.WaitVisible})
		}},
		{"accessibility", func(client *supervisor.Client) error {
			_, err := client.AccessibilityTree(context.Background(), browser.AccessibilityRequest{})
			return err
		}},
		{"challenge", func(client *supervisor.Client) error {
			_, err := client.DetectManualChallenge(context.Background(), browser.ManualChallengeRequest{})
			return err
		}},
		{"screenshot", func(client *supervisor.Client) error {
			return client.Screenshot(context.Background(), browser.ScreenshotRequest{URL: "https://example.test", Path: "x.png"})
		}},
		{"pdf", func(client *supervisor.Client) error {
			return client.PDF(context.Background(), browser.PDFRequest{URL: "https://example.test", Path: "x.pdf"})
		}},
		{"download", func(client *supervisor.Client) error {
			_, err := client.Download(context.Background(), browser.DownloadRequest{Selector: "#a", Path: "a.bin"})
			return err
		}},
		{"metadata", func(client *supervisor.Client) error {
			_, err := client.ResponseMetadata(context.Background(), browser.ResponseMetadataRequest{URL: "https://example.test"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := newBrowserClient(t, func(browserClientCall) supervisor.Reply {
				return browserClientError(supervisor.CodeMethodFailed, "method failed")
			})
			if err := tt.call(client); !errors.Is(err, supervisor.ErrDaemonMethod) {
				t.Fatalf("call error = %v, want ErrDaemonMethod", err)
			}
		})
	}
}

func newSupervisorVersionServer(t *testing.T, debugURL string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]string{"webSocketDebuggerUrl": debugURL})
	}))
	t.Cleanup(server.Close)
	return server
}

func supervisorHostPort(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse version URL: %v", err)
	}
	return parsed.Host
}
