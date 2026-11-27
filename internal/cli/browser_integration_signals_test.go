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

	stopped := env.run(t, "daemon", "stop", "--pretty")
	if stopped.Err != nil {
		t.Fatalf("daemon stop: %v\nstdout: %s\nstderr: %s", stopped.Err, stopped.Stdout, stopped.Stderr)
	}
	env.waitRuntimeGone(t)
	waitProcessGone(t, tree.Chrome.PID)
	waitProcessGone(t, tree.Guard.PID)
	env.assertNoTestChrome(t)
}

func TestDaemonSIGTERMKillsOwnedChrome(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)

	started := env.run(t, "browser", "start", "--local", "--pretty")
	if started.Err != nil {
		t.Fatalf("browser start: %v\nstdout: %s\nstderr: %s", started.Err, started.Stdout, started.Stderr)
	}
	runtimeState := env.waitRuntime(t)
	state := env.waitState(t)
	tree := env.assertOwnedTree(t, runtimeState, state)
	if err := syscall.Kill(runtimeState.PID, syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to daemon: %v", err)
	}

	env.waitRuntimeGone(t)
	env.waitStateGone(t)
	waitProcessGone(t, tree.Chrome.PID)
	waitProcessGone(t, tree.Guard.PID)
	env.assertNoTestChrome(t)
}

func TestDaemonSIGKILLGuardKillsOwnedChrome(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)

	started := env.run(t, "browser", "start", "--local", "--pretty")
	if started.Err != nil {
		t.Fatalf("browser start: %v\nstdout: %s\nstderr: %s", started.Err, started.Stdout, started.Stderr)
	}
	oldRuntime := env.waitRuntime(t)
	oldState := env.waitState(t)
	oldTree := env.assertOwnedTree(t, oldRuntime, oldState)
	if err := syscall.Kill(oldRuntime.PID, syscall.SIGKILL); err != nil {
		t.Fatalf("send SIGKILL to daemon: %v", err)
	}

	waitProcessGone(t, oldTree.Chrome.PID)
	waitProcessGone(t, oldTree.Guard.PID)
	if _, err := os.Stat(env.localStatePath()); err != nil {
		t.Fatalf("state after daemon SIGKILL error = %v, want preserved", err)
	}

	status := env.run(t, "browser", "status", "--local", "--pretty")
	if status.Err != nil {
		t.Fatalf("browser status after SIGKILL: %v\nstdout: %s\nstderr: %s", status.Err, status.Stdout, status.Stderr)
	}
	statusDocument := decodeJSON[browserStatusEnvelope](t, status.Stdout)
	if statusDocument.Status.Status != "stale" {
		t.Fatalf("browser status after SIGKILL = %q, want stale", statusDocument.Status.Status)
	}

	restarted := env.run(t, "browser", "start", "--local", "--pretty")
	if restarted.Err != nil {
		t.Fatalf("browser start after SIGKILL: %v\nstdout: %s\nstderr: %s", restarted.Err, restarted.Stdout, restarted.Stderr)
	}
	restartedDocument := decodeJSON[browserStatusEnvelope](t, restarted.Stdout)
	if restartedDocument.Status.Status != "running" || restartedDocument.Status.ChromePID == oldState.ChromePID {
		t.Fatalf("restarted browser = %#v, want new running Chrome", restartedDocument.Status)
	}
	newState := env.waitState(t)
	if newState.ChromePID != restartedDocument.Status.ChromePID || !newState.Owned {
		t.Fatalf("new state = %#v, want Chrome %d", newState, restartedDocument.Status.ChromePID)
	}
	if !strings.HasSuffix(newState.DataDir, "/.moth/browser/chrome-data") {
		t.Fatalf("new data dir = %q, want test profile", newState.DataDir)
	}

	if stopped := env.run(t, "daemon", "stop", "--pretty"); stopped.Err != nil {
		t.Fatalf("daemon stop: %v\nstdout: %s\nstderr: %s", stopped.Err, stopped.Stdout, stopped.Stderr)
	}
	env.waitRuntimeGone(t)
	env.waitStateGone(t)
	waitProcessGone(t, newState.ChromePID)
	env.assertNoTestChrome(t)
}
