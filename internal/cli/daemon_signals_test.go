//go:build !windows

package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

const (
	daemonSignalHelperEnv  = "MOTH_TEST_DAEMON_SIGNAL_HELPER"
	daemonSignalRuntimeEnv = "MOTH_TEST_DAEMON_SIGNAL_RUNTIME_DIR"
)

func TestDaemonRunStopsOnSignals(t *testing.T) {
	if os.Getenv(daemonSignalHelperEnv) == "1" {
		runDaemonSignalHelper(t)
		return
	}

	tests := []struct {
		name   string
		signal os.Signal
	}{
		{name: "sigterm", signal: syscall.SIGTERM},
		{name: "sigint", signal: os.Interrupt},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runDaemonSignalChild(t, test.signal)
		})
	}
}

func runDaemonSignalHelper(t *testing.T) {
	t.Helper()

	runtimeDir := os.Getenv(daemonSignalRuntimeEnv)
	if runtimeDir == "" {
		t.Fatalf("%s is empty", daemonSignalRuntimeEnv)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Execute(
		context.Background(),
		[]string{"daemon", "run", "--runtime-dir", runtimeDir},
		&stdout,
		&stderr,
	); err != nil {
		t.Fatalf("daemon run: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
}

func runDaemonSignalChild(t *testing.T, signal os.Signal) {
	t.Helper()

	runtimeDir := newShortRuntimeDir(t)
	//nolint:gosec,noctx // The test binary path comes from os.Args[0].
	command := exec.Command(os.Args[0], "-test.run=^TestDaemonRunStopsOnSignals$")
	command.Env = append(
		os.Environ(),
		daemonSignalHelperEnv+"=1",
		daemonSignalRuntimeEnv+"="+runtimeDir,
	)

	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatalf("start daemon child: %v", err)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- command.Wait()
	}()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-waitDone:
		default:
		}
	})

	waitForDaemonRuntime(t, runtimeDir)
	if err := command.Process.Signal(signal); err != nil {
		t.Fatalf("send %s: %v", signal, err)
	}

	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("daemon exit after %s: %v\n%s", signal, err, output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("daemon did not exit after %s", signal)
	}

	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime after %s error = %v, want removed", signal, err)
	}
	if _, err := os.Stat(supervisor.SocketPath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after %s error = %v, want removed", signal, err)
	}
}

func waitForDaemonRuntime(t *testing.T, runtimeDir string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := supervisor.ReadRuntime(runtimeDir); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("daemon runtime in %q did not become ready", runtimeDir)
}
