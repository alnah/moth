//go:build browser && !windows

package cli

import (
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestFetchBrowserSurvivesCLISIGKILL(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	server := newSlowPageServer(t)

	process := env.start(t, "fetch", "--browser", "--timeout", "30s", server.Server.URL+"/slow")
	t.Cleanup(server.close)

	runtimeState := env.waitRuntime(t)
	if runtimeState.PID == os.Getpid() {
		t.Fatal("daemon pid equals test pid, want detached child")
	}
	tree := env.waitPoolTree(t, runtimeState.PID)
	if !processAlive(tree.Chrome.PID) || !processAlive(tree.Guard.PID) {
		t.Fatalf("pool tree is not alive: chrome=%d guard=%d", tree.Chrome.PID, tree.Guard.PID)
	}

	process.kill(t)
	if err := process.wait(t, browserIntegrationStop); err == nil {
		t.Fatal("CLI exited cleanly after SIGKILL, want killed")
	}
	if !processAlive(runtimeState.PID) {
		t.Fatal("daemon died after CLI SIGKILL")
	}
	if !processAlive(tree.Chrome.PID) {
		t.Fatal("pool Chrome died after CLI SIGKILL")
	}
	if !processAlive(tree.Guard.PID) {
		t.Fatal("pool guard died after CLI SIGKILL")
	}

	stopDaemonAndWait(t, env, tree)
}

func TestDaemonSIGTERMKillsOwnedChrome(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	browser := startRealBrowser(t, env)

	if err := syscall.Kill(browser.Runtime.PID, syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to daemon: %v", err)
	}

	waitDaemonGone(t, env, browser.Tree)
}

func TestDaemonSIGKILLGuardKillsOwnedChrome(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	oldBrowser := startRealBrowser(t, env)

	if err := syscall.Kill(oldBrowser.Runtime.PID, syscall.SIGKILL); err != nil {
		t.Fatalf("send SIGKILL to daemon: %v", err)
	}
	waitProcessGone(t, oldBrowser.Tree.Chrome.PID)
	waitProcessGone(t, oldBrowser.Tree.Guard.PID)
	if _, err := os.Stat(env.localStatePath()); err != nil {
		t.Fatalf("state after daemon SIGKILL error = %v, want preserved", err)
	}

	status := env.run(t, "browser", "status", "--local", "--pretty")
	requireCommandSuccess(t, "browser status after SIGKILL", status)
	statusDocument := decodeJSON[browserStatusEnvelope](t, status.Stdout)
	if statusDocument.Status.Status != "stale" {
		t.Fatalf("browser status after SIGKILL = %q, want stale", statusDocument.Status.Status)
	}

	restarted := startRealBrowser(t, env)
	if restarted.Status.Status.ChromePID == oldBrowser.State.ChromePID {
		t.Fatalf("restarted browser = %#v, want new running Chrome", restarted.Status.Status)
	}
	if !strings.HasSuffix(restarted.State.DataDir, "/.moth/browser/chrome-data") {
		t.Fatalf("new data dir = %q, want test profile", restarted.State.DataDir)
	}

	stopDaemonAndWait(t, env, restarted.Tree)
}
