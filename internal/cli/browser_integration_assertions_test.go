//go:build browser

package cli

import (
	"os"
	"testing"

	"github.com/alnah/moth/internal/supervisor"
)

type realBrowser struct {
	Status  browserStatusEnvelope
	Runtime supervisor.Runtime
	State   browserStateFile
	Tree    processTree
}

func requireCommandSuccess(t *testing.T, action string, result commandResult) {
	t.Helper()

	if result.Err != nil {
		t.Fatalf("%s: %v\nstdout: %s\nstderr: %s", action, result.Err, result.Stdout, result.Stderr)
	}
}

func startRealBrowser(t *testing.T, env *browserIntegrationEnv) realBrowser {
	t.Helper()

	started := env.run(t, "browser", "start", "--local", "--pretty")
	requireCommandSuccess(t, "browser start", started)
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
	return realBrowser{
		Status:  status,
		Runtime: runtimeState,
		State:   state,
		Tree:    tree,
	}
}

func stopDaemonAndWait(t *testing.T, env *browserIntegrationEnv, tree processTree) {
	t.Helper()

	stopped := env.run(t, "daemon", "stop", "--pretty")
	requireCommandSuccess(t, "daemon stop", stopped)
	document := decodeJSON[daemonStatusEnvelope](t, stopped.Stdout)
	if document.Status != "stopped" {
		t.Fatalf("daemon stop status = %q, want stopped", document.Status)
	}
	waitDaemonGone(t, env, tree)
}

func waitDaemonGone(t *testing.T, env *browserIntegrationEnv, tree processTree) {
	t.Helper()

	env.waitRuntimeGone(t)
	env.waitStateGone(t)
	waitProcessGone(t, tree.Chrome.PID)
	waitProcessGone(t, tree.Guard.PID)
	env.assertNoTestChrome(t)
}
