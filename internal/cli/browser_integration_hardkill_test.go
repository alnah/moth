//go:build browser

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchBrowserSurvivesCLIHardKill(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	server := newSlowPageServer(t)

	process := env.start(t, "fetch", "--browser", "--timeout", "30s", server.Server.URL+"/slow")
	t.Cleanup(server.close)

	runtimeState := env.waitRuntime(t)
	if runtimeState.PID == os.Getpid() {
		t.Fatal("daemon pid equals test pid, want detached child")
	}
	select {
	case <-process.done:
		t.Fatalf("fetch exited before pool Chrome started: %v\nstdout: %s\nstderr: %s", process.waitErr, process.Stdout, process.Stderr)
	default:
	}
	tree := env.waitPoolTree(t, runtimeState.PID)
	if !processAlive(tree.Chrome.PID) || !processAlive(tree.Guard.PID) {
		t.Fatalf("pool tree is not alive: chrome=%d guard=%d", tree.Chrome.PID, tree.Guard.PID)
	}

	process.kill(t)
	if err := process.wait(t, browserIntegrationStop); err == nil {
		t.Fatal("CLI exited cleanly after hard kill, want killed")
	}
	if !processAlive(runtimeState.PID) {
		t.Fatal("daemon died after CLI hard kill")
	}
	if !processAlive(tree.Chrome.PID) {
		t.Fatal("pool Chrome died after CLI hard kill")
	}
	if !processAlive(tree.Guard.PID) {
		t.Fatal("pool guard died after CLI hard kill")
	}

	stopDaemonAndWait(t, env, tree)
}

func TestDaemonHardKillGuardKillsOwnedChrome(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	oldBrowser := startRealBrowser(t, env)

	hardKillDaemon(t, oldBrowser.Runtime.PID)
	waitProcessGone(t, oldBrowser.Tree.Chrome.PID)
	waitProcessGone(t, oldBrowser.Tree.Guard.PID)
	if _, err := os.Stat(env.localStatePath()); err != nil {
		t.Fatalf("state after daemon hard kill error = %v, want preserved", err)
	}

	status := env.run(t, "browser", "status", "--local", "--pretty")
	requireCommandSuccess(t, "browser status after hard kill", status)
	statusDocument := decodeJSON[browserStatusEnvelope](t, status.Stdout)
	if statusDocument.Status.Status != "stale" {
		t.Fatalf("browser status after hard kill = %q, want stale", statusDocument.Status.Status)
	}

	restarted := startRealBrowser(t, env)
	if restarted.Status.Status.ChromePID == oldBrowser.State.ChromePID {
		t.Fatalf("restarted browser = %#v, want new running Chrome", restarted.Status.Status)
	}
	if !strings.HasSuffix(restarted.State.DataDir, filepath.Join(".moth", "browser", "chrome-data")) {
		t.Fatalf("new data dir = %q, want test profile", restarted.State.DataDir)
	}

	stopDaemonAndWait(t, env, restarted.Tree)
}
