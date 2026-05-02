package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/browser"
)

func TestPersistentBrowserServiceCommandsRouteRequestsAndRenderStatusJSON(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStatus string
		assert     func(*testing.T, *fakeBrowser)
	}{
		{
			name:       "start local visible browser",
			args:       []string{"browser", "start", "--show", "--local"},
			wantStatus: "running",
			assert: func(t *testing.T, fake *fakeBrowser) {
				t.Helper()
				if fake.start.Scope != "local" || !fake.start.Show {
					t.Fatalf("start request = %#v, want local show request", fake.start)
				}
			},
		},
		{
			name:       "status global browser",
			args:       []string{"browser", "status", "--global"},
			wantStatus: "running",
			assert: func(t *testing.T, fake *fakeBrowser) {
				t.Helper()
				if fake.status.Scope != "global" {
					t.Fatalf("status request = %#v, want global scope", fake.status)
				}
			},
		},
		{
			name:       "stop auto browser",
			args:       []string{"browser", "stop"},
			wantStatus: "stopped",
			assert: func(t *testing.T, fake *fakeBrowser) {
				t.Helper()
				if fake.stop.Scope != "auto" {
					t.Fatalf("stop request = %#v, want auto scope", fake.stop)
				}
			},
		},
		{
			name:       "connect external browser",
			args:       []string{"browser", "connect", "127.0.0.1:9222", "--local"},
			wantStatus: "running",
			assert: func(t *testing.T, fake *fakeBrowser) {
				t.Helper()
				if fake.connect.Scope != "local" || fake.connect.HostPort != "127.0.0.1:9222" {
					t.Fatalf("connect request = %#v, want local host:port", fake.connect)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()
			stdout, stderr, err := harness.execute(tt.args...)
			if err != nil {
				t.Fatalf("execute %q: %v\nstderr: %s", strings.Join(tt.args, " "), err, stderr)
			}
			assertBrowserStatusDocument(t, stdout, tt.wantStatus)
			tt.assert(t, harness.browser)
		})
	}
}

func TestBrowserStatusRendersMissingAndStaleStateAsSuccessfulJSON(t *testing.T) {
	for _, status := range []string{"missing", "stale"} {
		t.Run(status, func(t *testing.T) {
			harness := newCommandHarness()
			harness.browser.statusResponse = browser.BrowserStatus{Status: status, Scope: "local"}

			stdout, stderr, err := harness.execute("browser", "status", "--local")
			if err != nil {
				t.Fatalf("execute browser status with %s state: %v\nstderr: %s", status, err, stderr)
			}
			assertBrowserStatusDocument(t, stdout, status)
		})
	}
}

func TestPersistentBrowserPageCommandsRenderStateErrorAsOneStableJSONDocument(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "open", args: []string{"browser", "open", "https://example.test"}},
		{name: "pages", args: []string{"browser", "pages"}},
		{name: "page", args: []string{"browser", "page", "page-1"}},
		{name: "close-page", args: []string{"browser", "close-page", "page-1"}},
		{name: "click", args: []string{"browser", "click", "button"}},
		{name: "input", args: []string{"browser", "input", "textarea", "hello"}},
		{name: "wait", args: []string{"browser", "wait", "main.ready"}},
		{name: "ax-tree", args: []string{"browser", "ax-tree"}},
		{name: "challenge", args: []string{"browser", "challenge"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()
			harness.browser.err = browser.ErrBrowserStateUnavailable

			stdout, stderr, err := harness.execute(tt.args...)
			if err == nil {
				t.Fatalf("execute %q error = nil, want browser state error", strings.Join(tt.args, " "))
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty stdout for browser state error", stdout)
			}

			document := decodeSingleJSONErrorDocument(t, stderr)
			if document.Error.Code != "browser_state_unavailable" {
				t.Fatalf("error.code = %q, want browser_state_unavailable", document.Error.Code)
			}
			if !strings.Contains(document.Error.Message, "browser state unavailable") {
				t.Fatalf("error.message = %q, want browser state unavailable context", document.Error.Message)
			}
		})
	}
}

func assertBrowserStatusDocument(t *testing.T, payload string, wantStatus string) {
	t.Helper()
	assertJSONType(t, payload, "browser_status")

	var document struct {
		Status browser.BrowserStatus `json:"status"`
	}
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatalf("decode browser status JSON %q: %v", payload, err)
	}
	if document.Status.Status != wantStatus {
		t.Fatalf("status.status = %q, want %q; payload: %s", document.Status.Status, wantStatus, payload)
	}
}
