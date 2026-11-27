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

//nolint:gocyclo // The integration scenario keeps each protocol step explicit.
func TestBrowserCLIRealLifecycle(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	server := newIntegrationPageServer(t)

	started := env.run(t, "browser", "start", "--local", "--pretty")
	if started.Err != nil {
		t.Fatalf("browser start: %v\nstdout: %s\nstderr: %s", started.Err, started.Stdout, started.Stderr)
	}
	status := decodeJSON[browserStatusEnvelope](t, started.Stdout)
	if status.Type != "browser_status" || status.Status.Status != "running" {
		t.Fatalf("browser start = %#v, want running status", status)
	}
	if !status.Status.Owned || status.Status.ChromePID <= 0 || status.Status.Scope != "local" {
		t.Fatalf("browser start status = %#v, want owned local Chrome", status.Status)
	}

	runtimeState := env.waitRuntime(t)
	if runtimeState.PID == os.Getpid() {
		t.Fatal("daemon pid equals test pid, want detached child")
	}
	state := env.waitState(t)
	if state.ChromePID != status.Status.ChromePID || !state.Owned {
		t.Fatalf("state = %#v, want owned Chrome %d", state, status.Status.ChromePID)
	}
	tree := env.assertOwnedTree(t, runtimeState, state)

	current := env.run(t, "browser", "status", "--local", "--pretty")
	if current.Err != nil {
		t.Fatalf("browser status: %v\nstdout: %s\nstderr: %s", current.Err, current.Stdout, current.Stderr)
	}
	currentStatus := decodeJSON[browserStatusEnvelope](t, current.Stdout)
	if currentStatus.Status.Status != "running" || currentStatus.Status.ChromePID != state.ChromePID {
		t.Fatalf("browser status = %#v, want running Chrome %d", currentStatus.Status, state.ChromePID)
	}

	health := env.run(t, "daemon", "status", "--pretty")
	if health.Err != nil {
		t.Fatalf("daemon status: %v\nstdout: %s\nstderr: %s", health.Err, health.Stdout, health.Stderr)
	}
	healthDocument := decodeJSON[daemonStatusEnvelope](t, health.Stdout)
	if healthDocument.Status != "running" || healthDocument.Health == nil || healthDocument.Health.ActiveBrowsers != 1 {
		t.Fatalf("daemon status = %#v, want one active browser", healthDocument)
	}

	pageURL := server.URL + "/page"
	opened := env.run(t, "browser", "open", pageURL, "--pretty")
	if opened.Err != nil {
		t.Fatalf("browser open: %v\nstdout: %s\nstderr: %s", opened.Err, opened.Stdout, opened.Stderr)
	}
	page := decodeJSON[browserPageEnvelope](t, opened.Stdout)
	if page.Page.Title != "Moth integration page" || !page.Page.Active {
		t.Fatalf("browser open page = %#v, want active integration page", page.Page)
	}

	pages := env.run(t, "browser", "pages", "--pretty")
	if pages.Err != nil {
		t.Fatalf("browser pages: %v\nstdout: %s\nstderr: %s", pages.Err, pages.Stdout, pages.Stderr)
	}
	pagesDocument := decodeJSON[browserPagesEnvelope](t, pages.Stdout)
	if len(pagesDocument.Pages) != 1 || !pagesDocument.Pages[0].Active {
		t.Fatalf("browser pages = %#v, want one active page", pagesDocument.Pages)
	}

	screenshotPath := filepath.Join(env.Work, "page.png")
	screenshot := env.run(t, "browser", "screenshot", pageURL, screenshotPath, "--pretty")
	if screenshot.Err != nil {
		t.Fatalf("browser screenshot: %v\nstdout: %s\nstderr: %s", screenshot.Err, screenshot.Stdout, screenshot.Stderr)
	}
	assertNonEmptyFile(t, screenshotPath)

	pdfPath := filepath.Join(env.Work, "page.pdf")
	pdf := env.run(t, "browser", "pdf", pageURL, pdfPath, "--pretty")
	if pdf.Err != nil {
		t.Fatalf("browser pdf: %v\nstdout: %s\nstderr: %s", pdf.Err, pdf.Stdout, pdf.Stderr)
	}
	assertFilePrefix(t, pdfPath, "%PDF")

	stopped := env.run(t, "daemon", "stop", "--pretty")
	if stopped.Err != nil {
		t.Fatalf("daemon stop: %v\nstdout: %s\nstderr: %s", stopped.Err, stopped.Stdout, stopped.Stderr)
	}
	stopDocument := decodeJSON[daemonStatusEnvelope](t, stopped.Stdout)
	if stopDocument.Status != "stopped" {
		t.Fatalf("daemon stop status = %q, want stopped", stopDocument.Status)
	}

	env.waitRuntimeGone(t)
	env.waitStateGone(t)
	waitProcessGone(t, tree.Chrome.PID)
	waitProcessGone(t, tree.Guard.PID)
	env.assertNoTestChrome(t)

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
	assertNoToken(t, logText, runtimeState.Token)
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
