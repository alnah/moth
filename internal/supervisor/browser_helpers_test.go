//go:build !windows

package supervisor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/supervisor"
)

type browserTestLauncher struct {
	result browser.LaunchResult
	err    error
	calls  int
}

func (launcher *browserTestLauncher) LaunchBrowser(context.Context, browser.LaunchRequest) (browser.LaunchResult, error) {
	launcher.calls++
	return launcher.result, launcher.err
}

type browserTestConnector struct {
	connections map[string]*browserTestConnection
	failures    map[string]error
	attempts    []string
}

func newBrowserTestConnector(connections map[string]*browserTestConnection) *browserTestConnector {
	if connections == nil {
		connections = map[string]*browserTestConnection{}
	}
	return &browserTestConnector{
		connections: connections,
		failures:    map[string]error{},
	}
}

func (connector *browserTestConnector) ConnectBrowser(
	ctx context.Context,
	debugURL string,
) (browser.BrowserConnection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	connector.attempts = append(connector.attempts, debugURL)
	if err := connector.failures[debugURL]; err != nil {
		return nil, err
	}
	connection, ok := connector.connections[debugURL]
	if !ok {
		return nil, os.ErrNotExist
	}
	return connection, nil
}

type browserTestConnection struct {
	mu sync.Mutex

	pages       []browser.PageInfo
	tree        browser.AccessibilityTree
	challenge   browser.ManualChallengeResult
	openRequest browser.OpenPageRequest
	downloads   []browser.DownloadRequest
	openStarted chan struct{}
	openRelease chan struct{}

	pings         int
	pingErr       error
	disconnects   int
	disconnectErr error
	closed        bool
	closeErr      error
}

func newBrowserTestConnection() *browserTestConnection {
	return &browserTestConnection{
		tree: browser.AccessibilityTree{Nodes: []browser.AccessibilityNode{{Role: "button", Name: "Submit"}}},
		challenge: browser.ManualChallengeResult{
			ManualRequired: true,
			Kind:           "captcha",
			Warnings:       []content.Warning{content.WarningCaptchaPossible},
		},
	}
}

func (connection *browserTestConnection) OpenPage(ctx context.Context, request browser.OpenPageRequest) (browser.PageInfo, error) {
	connection.mu.Lock()
	started := connection.openStarted
	release := connection.openRelease
	connection.openRequest = request
	connection.mu.Unlock()
	if started != nil {
		close(started)
	}
	if release != nil {
		select {
		case <-ctx.Done():
			return browser.PageInfo{}, ctx.Err()
		case <-release:
		}
	}

	connection.mu.Lock()
	defer connection.mu.Unlock()
	page := browser.PageInfo{
		ID:          "page-1",
		URL:         request.URL,
		Title:       "Test page",
		Active:      true,
		ProfileName: request.ProfileName,
		SessionName: request.SessionName,
	}
	connection.pages = append(connection.pages, page)
	return page, nil
}

func (connection *browserTestConnection) ListPages(context.Context, browser.SessionRequest) ([]browser.PageInfo, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return append([]browser.PageInfo(nil), connection.pages...), nil
}

func (connection *browserTestConnection) SwitchPage(_ context.Context, request browser.PageSelection) (browser.PageInfo, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	for index := range connection.pages {
		if connection.pages[index].ID == request.PageID {
			connection.pages[index].Active = true
			return connection.pages[index], nil
		}
	}
	return browser.PageInfo{}, os.ErrNotExist
}

func (connection *browserTestConnection) ClosePage(_ context.Context, request browser.PageSelection) error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	for index := range connection.pages {
		if connection.pages[index].ID == request.PageID {
			connection.pages = append(connection.pages[:index], connection.pages[index+1:]...)
			return nil
		}
	}
	return os.ErrNotExist
}

func (connection *browserTestConnection) Click(context.Context, browser.InteractionRequest) error {
	return nil
}

func (connection *browserTestConnection) Input(context.Context, browser.InputRequest) error {
	return nil
}

func (connection *browserTestConnection) Wait(context.Context, browser.WaitRequest) error {
	return nil
}

func (connection *browserTestConnection) AccessibilityTree(
	context.Context,
	browser.AccessibilityRequest,
) (browser.AccessibilityTree, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.tree, nil
}

func (connection *browserTestConnection) DetectManualChallenge(
	context.Context,
	browser.ManualChallengeRequest,
) (browser.ManualChallengeResult, error) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.challenge, nil
}

func (connection *browserTestConnection) Download(_ context.Context, request browser.DownloadRequest) (browser.CapturedDownload, error) {
	connection.mu.Lock()
	connection.downloads = append(connection.downloads, request)
	connection.mu.Unlock()
	return browser.CapturedDownload{
		Bytes:       []byte("download-data"),
		ContentType: "application/octet-stream",
	}, nil
}

func (connection *browserTestConnection) Ping(context.Context) error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	connection.pings++
	return connection.pingErr
}

func (connection *browserTestConnection) Close(context.Context) error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	connection.closed = true
	return connection.closeErr
}

func (connection *browserTestConnection) Disconnect(context.Context) error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	connection.disconnects++
	return connection.disconnectErr
}

func newBrowserTestFactory(
	launcher *browserTestLauncher,
	connector *browserTestConnector,
	terminate func(int) error,
) supervisor.BrowserServiceFactory {
	return func(options browser.PersistentServiceOptions) *browser.PersistentService {
		options.Launcher = launcher
		options.Connector = connector
		options.TerminateProcess = terminate
		return browser.NewPersistentService(options)
	}
}

func newBrowserTestServer(t *testing.T, options supervisor.ServerOptions) (*supervisor.Server, *httptest.Server) {
	t.Helper()
	if options.Version == "" {
		options.Version = testVersion
	}
	if options.Token == "" {
		options.Token = testToken
	}
	server := supervisor.NewServer(options)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	t.Cleanup(func() { _ = server.Close() })
	return server, httpServer
}

func browserRequestMetadata(t *testing.T, cwd string, local string, global string) map[string]string {
	t.Helper()
	return map[string]string{
		"X-Moth-CWD":              cwd,
		"X-Moth-Local-State-Dir":  local,
		"X-Moth-Global-State-Dir": global,
	}
}

func callBrowserServer(
	t *testing.T,
	httpServer *httptest.Server,
	method string,
	params any,
	headers map[string]string,
) supervisor.Reply {
	t.Helper()
	var body string
	if params == nil {
		body = marshalCall(t, supervisor.Call{Method: method})
	} else {
		body = marshalCall(t, supervisor.Call{Method: method, Params: rawParams(t, params)})
	}
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		httpServer.URL+supervisor.CallPath,
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create call request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+testToken)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := httpServer.Client().Do(request)
	if err != nil {
		t.Fatalf("post call: %v", err)
	}
	return decodeReply(t, response)
}

func decodeBrowserResult[T any](t *testing.T, reply supervisor.Reply) T {
	t.Helper()
	return decodeResult[T](t, reply)
}

func browserTestStateDirs(t *testing.T) (string, browser.StateDirs) {
	t.Helper()
	cwd := t.TempDir()
	return cwd, browser.StateDirs{
		Local:  filepath.Join(cwd, ".moth", "browser"),
		Global: filepath.Join(t.TempDir(), ".moth", "browser"),
	}
}
