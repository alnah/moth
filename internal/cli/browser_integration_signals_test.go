//go:build browser && !windows

package cli

import (
	"syscall"
	"testing"
)

func TestDaemonSIGTERMKillsOwnedChrome(t *testing.T) {
	env := newBrowserIntegrationEnv(t)
	env.requireBrowser(t)
	browser := startRealBrowser(t, env)

	if err := syscall.Kill(browser.Runtime.PID, syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to daemon: %v", err)
	}

	waitDaemonGone(t, env, browser.Tree)
}
