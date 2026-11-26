//go:build !windows

package browser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentServiceStatusUsesCachedConnectionAndPings(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	persistentBrowser := newFakePersistentBrowser()
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://owned": persistentBrowser,
	})
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  &fakePersistentLauncher{result: LaunchResult{DebugURL: "ws://owned", ChromePID: 4242}},
		Connector: connector,
	})

	if _, err := service.Start(ctx, StartRequest{Scope: "local"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	connected := len(connector.connected)

	status, err := service.Status(ctx, StatusRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Status != "running" {
		t.Fatalf("Status() = %#v, want running", status)
	}
	if persistentBrowser.pings != 1 {
		t.Fatalf("Ping calls = %d, want 1", persistentBrowser.pings)
	}
	if len(connector.connected) != connected {
		t.Fatalf("connector calls = %d, want %d cached calls", len(connector.connected), connected)
	}

	persistentBrowser.pingErr = errors.New("cdp closed")
	status, err = service.Status(ctx, StatusRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("Status(stale ping) error = %v", err)
	}
	if status.Status != "stale" {
		t.Fatalf("Status(stale ping) = %#v, want stale", status)
	}
}

func TestPersistentServiceStopFallsBackWhenCloseFails(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	persistentBrowser := newFakePersistentBrowser()
	persistentBrowser.closeErr = errors.New("cdp close failed")
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Connector: newFakePersistentConnector(map[string]*fakePersistentBrowser{
			"ws://owned": persistentBrowser,
		}),
		TerminateProcess: func(pid int) error {
			if pid != 4242 {
				t.Fatalf("TerminateProcess(%d), want 4242", pid)
			}
			return nil
		},
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":  "ws://owned",
		"chrome_pid": 4242,
		"owned":      true,
	})
	service.cacheConnection("local", "ws://owned", persistentBrowser)

	status, err := service.Stop(ctx, StopRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("Stop(close fallback) error = %v", err)
	}
	if status.Status != "stopped" {
		t.Fatalf("Stop() = %#v, want stopped", status)
	}
	if !persistentBrowser.closed {
		t.Fatal("Stop() did not attempt Close")
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state after fallback exists err = %v, want removed", err)
	}
}

func TestPersistentServiceStopDoesNotFallbackWhenContextCanceled(t *testing.T) {
	stateDirs := newTestStateDirs(t)
	persistentBrowser := newFakePersistentBrowser()
	persistentBrowser.closeErr = errors.New("cdp close failed")
	terminated := false
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Connector: newFakePersistentConnector(map[string]*fakePersistentBrowser{
			"ws://owned": persistentBrowser,
		}),
		TerminateProcess: func(int) error {
			terminated = true
			return nil
		},
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":  "ws://owned",
		"chrome_pid": 4242,
		"owned":      true,
	})
	service.cacheConnection("local", "ws://owned", persistentBrowser)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Stop(ctx, StopRequest{Scope: "local"}); err == nil {
		t.Fatal("Stop(canceled) error = nil, want close error")
	}
	if terminated {
		t.Fatal("Stop(canceled) called TerminateProcess, want no fallback")
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); err != nil {
		t.Fatalf("state after canceled stop error = %v, want preserved", err)
	}
}

func TestPersistentServiceStopExternalDisconnectsWithoutClosing(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	persistentBrowser := newFakePersistentBrowser()
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Connector: newFakePersistentConnector(map[string]*fakePersistentBrowser{
			"ws://external": persistentBrowser,
		}),
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url": "ws://external",
		"owned":     false,
	})
	service.cacheConnection("local", "ws://external", persistentBrowser)

	status, err := service.Stop(ctx, StopRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("Stop(external) error = %v", err)
	}
	if status.Status != "stopped" {
		t.Fatalf("Stop(external) = %#v, want stopped", status)
	}
	if persistentBrowser.disconnects != 1 {
		t.Fatalf("Disconnect calls = %d, want 1", persistentBrowser.disconnects)
	}
	if persistentBrowser.closed {
		t.Fatal("Stop(external) called Close, want Disconnect only")
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state after external stop exists err = %v, want removed", err)
	}
}

func TestPersistentServiceStartTerminatesStaleOwnedBeforeLaunch(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	oldBrowser := newFakePersistentBrowser()
	newBrowser := newFakePersistentBrowser()
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://old": oldBrowser,
		"ws://new": newBrowser,
	})
	connector.failures["ws://old"] = errors.New("connection refused")
	terminatedPID := 0
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  &fakePersistentLauncher{result: LaunchResult{DebugURL: "ws://new", ChromePID: 5252}},
		Connector: connector,
		TerminateProcess: func(pid int) error {
			terminatedPID = pid
			return nil
		},
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":  "ws://old",
		"chrome_pid": 4242,
		"owned":      true,
	})

	status, err := service.Start(ctx, StartRequest{Scope: "local"})
	if err != nil {
		t.Fatalf("Start(stale owned) error = %v", err)
	}
	if status.DebugURL != "ws://new" || status.Status != "running" {
		t.Fatalf("Start(stale owned) = %#v, want new browser", status)
	}
	if terminatedPID != 4242 {
		t.Fatalf("terminated PID = %d, want 4242", terminatedPID)
	}
}

func TestPersistentServiceStartClosesNewBrowserOnStateSaveFailure(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	if err := os.MkdirAll(stateDirs.Local, 0o750); err != nil {
		t.Fatalf("create local state dir: %v", err)
	}
	if err := os.Chmod(stateDirs.Local, 0o500); err != nil {
		t.Fatalf("make local state dir read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(stateDirs.Local, 0o750) })

	newBrowser := newFakePersistentBrowser()
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Launcher:  &fakePersistentLauncher{result: LaunchResult{DebugURL: "ws://new", ChromePID: 5252}},
		Connector: newFakePersistentConnector(map[string]*fakePersistentBrowser{
			"ws://new": newBrowser,
		}),
	})

	if _, err := service.Start(ctx, StartRequest{Scope: "local"}); err == nil {
		t.Fatal("Start(save failure) error = nil, want write error")
	}
	if !newBrowser.closed {
		t.Fatal("Start(save failure) did not close new browser")
	}
}

func TestPersistentServiceConnectClosesPreviousOwned(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	oldBrowser := newFakePersistentBrowser()
	newBrowser := newFakePersistentBrowser()
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://old":      oldBrowser,
		"ws://external": newBrowser,
	})
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs:  stateDirs,
		Connector:  connector,
		HTTPClient: &http.Client{Timeout: time.Second},
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":  "ws://old",
		"chrome_pid": 4242,
		"owned":      true,
	})
	service.cacheConnection("local", "ws://old", oldBrowser)

	versionServer := newBrowserVersionServer(t, "ws://external")
	status, err := service.Connect(ctx, ConnectRequest{
		Scope:    "local",
		HostPort: hostPort(t, versionServer.URL),
	})
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if status.Status != "running" || status.Owned {
		t.Fatalf("Connect() = %#v, want running external", status)
	}
	if !oldBrowser.closed {
		t.Fatal("Connect() did not close previous owned browser")
	}
	if newBrowser.closed {
		t.Fatal("Connect() closed new external browser")
	}
	state := readTestState(t, stateDirs.Local)
	if state["debug_url"] != "ws://external" || state["owned"] != false {
		t.Fatalf("state after Connect = %#v, want external state", state)
	}
}

func TestPersistentServiceConnectKeepsOldBrowserWhenNewConnectFails(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	oldBrowser := newFakePersistentBrowser()
	connector := newFakePersistentConnector(map[string]*fakePersistentBrowser{
		"ws://old": oldBrowser,
	})
	connector.failures["ws://external"] = errors.New("connection refused")
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Connector: connector,
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":  "ws://old",
		"chrome_pid": 4242,
		"owned":      true,
	})
	service.cacheConnection("local", "ws://old", oldBrowser)

	versionServer := newBrowserVersionServer(t, "ws://external")
	if _, err := service.Connect(ctx, ConnectRequest{
		Scope:    "local",
		HostPort: hostPort(t, versionServer.URL),
	}); err == nil {
		t.Fatal("Connect(failed new browser) error = nil, want error")
	}
	if oldBrowser.closed {
		t.Fatal("Connect(failed new browser) closed old browser")
	}
	state := readTestState(t, stateDirs.Local)
	if state["debug_url"] != "ws://old" {
		t.Fatalf("state after failed Connect = %#v, want old state", state)
	}
}

func TestPersistentServiceShutdownClosesOwnedAndKeepsExternal(t *testing.T) {
	ctx := context.Background()
	stateDirs := newTestStateDirs(t)
	ownedBrowser := newFakePersistentBrowser()
	externalBrowser := newFakePersistentBrowser()
	service := NewPersistentService(PersistentServiceOptions{
		StateDirs: stateDirs,
		Connector: newFakePersistentConnector(map[string]*fakePersistentBrowser{
			"ws://owned":    ownedBrowser,
			"ws://external": externalBrowser,
		}),
	})
	writeTestState(t, stateDirs.Local, map[string]any{
		"debug_url":  "ws://owned",
		"chrome_pid": 4242,
		"owned":      true,
	})
	writeTestState(t, stateDirs.Global, map[string]any{
		"debug_url": "ws://external",
		"owned":     false,
	})
	service.cacheConnection("local", "ws://owned", ownedBrowser)
	service.cacheConnection("global", "ws://external", externalBrowser)

	if err := service.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if !ownedBrowser.closed {
		t.Fatal("Shutdown() did not close owned browser")
	}
	if externalBrowser.disconnects != 1 || externalBrowser.closed {
		t.Fatalf("external browser disconnects=%d closed=%v, want disconnect only",
			externalBrowser.disconnects, externalBrowser.closed)
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Local, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned state after Shutdown error = %v, want removed", err)
	}
	if _, err := os.Stat(filepath.Join(stateDirs.Global, "state.json")); err != nil {
		t.Fatalf("external state after Shutdown error = %v, want preserved", err)
	}
}

func TestDefaultStateDirsExposesUsableRoots(t *testing.T) {
	dirs := DefaultStateDirs()
	if dirs.Local == "" || dirs.Global == "" {
		t.Fatalf("DefaultStateDirs() = %#v, want local and global roots", dirs)
	}
}

func newBrowserVersionServer(t *testing.T, debugURL string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/json/version" {
			t.Errorf("version path = %q, want /json/version", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]string{"webSocketDebuggerUrl": debugURL})
	}))
	t.Cleanup(server.Close)
	return server
}

func hostPort(t *testing.T, rawURL string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse test URL %q: %v", rawURL, err)
	}
	return parsed.Host
}
