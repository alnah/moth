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

	home := t.TempDir()
	t.Setenv("HOME", home)
	runtimeDir := newShortRuntimeDir(t)
	child := startDaemonSignalChild(t, runtimeDir)

	waitForDaemonRuntime(t, runtimeDir)
	if err := child.command.Process.Signal(signal); err != nil {
		t.Fatalf("send %s: %v", signal, err)
	}
	child.waitForExit(t, signal)

	assertDaemonPathRemoved(t, signal, "runtime", supervisor.RuntimePath(runtimeDir))
	assertDaemonPathRemoved(t, signal, "socket", supervisor.SocketPath(runtimeDir))
}

type daemonSignalChild struct {
	command *exec.Cmd
	done    chan struct{}
	waitErr error
	output  *bytes.Buffer
}

func startDaemonSignalChild(t *testing.T, runtimeDir string) *daemonSignalChild {
	t.Helper()

	output := new(bytes.Buffer)
	//nolint:gosec,noctx // The test binary path comes from os.Args[0].
	command := exec.Command(os.Args[0], "-test.run=^TestDaemonRunStopsOnSignals$")
	command.Env = append(
		os.Environ(),
		daemonSignalHelperEnv+"=1",
		daemonSignalRuntimeEnv+"="+runtimeDir,
	)
	command.Stdout = output
	command.Stderr = output

	child := &daemonSignalChild{
		command: command,
		done:    make(chan struct{}),
		output:  output,
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start daemon child: %v", err)
	}
	go func() {
		child.waitErr = command.Wait()
		close(child.done)
	}()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-child.done:
		case <-time.After(time.Second):
		}
	})
	return child
}

func (child *daemonSignalChild) waitForExit(t *testing.T, signal os.Signal) {
	t.Helper()

	select {
	case <-child.done:
		if child.waitErr != nil {
			t.Fatalf("daemon exit after %s: %v\n%s", signal, child.waitErr, child.output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("daemon did not exit after %s", signal)
	}
}

func assertDaemonPathRemoved(t *testing.T, signal os.Signal, name string, path string) {
	t.Helper()

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s after %s error = %v, want removed", name, signal, err)
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
