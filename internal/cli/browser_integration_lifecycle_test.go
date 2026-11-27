//go:build browser && !windows

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserCLIRealLifecycle(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	server := newIntegrationPageServer(t)
	browser := startRealBrowser(t, env)

	current := env.run(t, "browser", "status", "--local", "--pretty")
	requireCommandSuccess(t, "browser status", current)
	currentStatus := decodeJSON[browserStatusEnvelope](t, current.Stdout)
	if currentStatus.Status.Status != "running" || currentStatus.Status.ChromePID != browser.State.ChromePID {
		t.Fatalf("browser status = %#v, want running Chrome %d", currentStatus.Status, browser.State.ChromePID)
	}

	health := env.run(t, "daemon", "status", "--pretty")
	requireCommandSuccess(t, "daemon status", health)
	healthDocument := decodeJSON[daemonStatusEnvelope](t, health.Stdout)
	if healthDocument.Status != "running" || healthDocument.Health == nil || healthDocument.Health.ActiveBrowsers != 1 {
		t.Fatalf("daemon status = %#v, want one active browser", healthDocument)
	}

	pageURL := server.URL + "/page"
	opened := env.run(t, "browser", "open", pageURL, "--pretty")
	requireCommandSuccess(t, "browser open", opened)
	page := decodeJSON[browserPageEnvelope](t, opened.Stdout)
	if page.Page.Title != "Moth integration page" || !page.Page.Active {
		t.Fatalf("browser open page = %#v, want active integration page", page.Page)
	}

	pages := env.run(t, "browser", "pages", "--pretty")
	requireCommandSuccess(t, "browser pages", pages)
	pagesDocument := decodeJSON[browserPagesEnvelope](t, pages.Stdout)
	if len(pagesDocument.Pages) != 1 || !pagesDocument.Pages[0].Active {
		t.Fatalf("browser pages = %#v, want one active page", pagesDocument.Pages)
	}

	screenshotPath := filepath.Join(env.Work, "page.png")
	screenshot := env.run(t, "browser", "screenshot", pageURL, screenshotPath, "--pretty")
	requireCommandSuccess(t, "browser screenshot", screenshot)
	assertNonEmptyFile(t, screenshotPath)

	pdfPath := filepath.Join(env.Work, "page.pdf")
	pdf := env.run(t, "browser", "pdf", pageURL, pdfPath, "--pretty")
	requireCommandSuccess(t, "browser pdf", pdf)
	assertFilePrefix(t, pdfPath, "%PDF")

	stopDaemonAndWait(t, env, browser.Tree)

	logPath := filepath.Join(env.Home, ".moth", "log", "daemon.log")
	logData, err := os.ReadFile(logPath) //nolint:gosec // Test path is isolated under Home.
	if err != nil {
		t.Fatalf("read daemon log: %v", err)
	}
	logText := string(logData)
	for _, message := range []string{"daemon started", "daemon stopping", "daemon stopped"} {
		if !strings.Contains(logText, message) {
			t.Fatalf("daemon log missing %q", message)
		}
	}
	assertNoToken(t, logText, browser.Runtime.Token)
}

func TestBrowserStopClosesOwnedChrome(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	browser := startRealBrowser(t, env)

	stopped := env.run(t, "browser", "stop", "--local", "--pretty")
	requireCommandSuccess(t, "browser stop", stopped)
	stopDocument := decodeJSON[browserStatusEnvelope](t, stopped.Stdout)
	if stopDocument.Status.Status != "stopped" || !stopDocument.Status.Owned {
		t.Fatalf("browser stop = %#v, want stopped owned browser", stopDocument.Status)
	}

	env.waitStateGone(t)
	waitProcessGone(t, browser.Tree.Chrome.PID)
	waitProcessGone(t, browser.Tree.Guard.PID)
	env.assertNoTestChrome(t)

	current := env.waitRuntime(t)
	if current.PID != browser.Runtime.PID {
		t.Fatalf("daemon pid after browser stop = %d, want %d", current.PID, browser.Runtime.PID)
	}
	status := env.run(t, "browser", "status", "--local", "--pretty")
	requireCommandSuccess(t, "browser status after stop", status)
	statusDocument := decodeJSON[browserStatusEnvelope](t, status.Stdout)
	if statusDocument.Status.Status != "missing" {
		t.Fatalf("browser status after stop = %q, want missing", statusDocument.Status.Status)
	}

	stopDaemonAndWait(t, env, browser.Tree)
}

func newIntegrationPageServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/page" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html>
<head><title>Moth integration page</title></head>
<body><h1>Moth integration page</h1></body>
</html>`))
	}))
	t.Cleanup(server.Close)
	return server
}

func assertNonEmptyFile(t *testing.T, path string) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
}

func assertFilePrefix(t *testing.T, path string, prefix string) {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // Test path is isolated under Work.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.HasPrefix(string(data), prefix) {
		t.Fatalf("%s prefix = %q, want %q", path, string(data[:min(len(data), len(prefix))]), prefix)
	}
}
