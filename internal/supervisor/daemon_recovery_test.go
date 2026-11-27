//go:build !windows

package supervisor_test

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

const (
	daemonRecoveryHelperEnv  = "MOTH_TEST_DAEMON_RECOVERY_HELPER"
	daemonRecoveryRuntimeEnv = "MOTH_TEST_DAEMON_RECOVERY_RUNTIME_DIR"
)

func TestDaemonRecoversAfterSIGKILL(t *testing.T) {
	if os.Getenv(daemonRecoveryHelperEnv) == "1" {
		runDaemonRecoveryHelper(t)
		return
	}

	runtimeDir := newRuntimeDir(t)
	child := startDaemonRecoveryChild(t, runtimeDir)
	oldRuntime := waitForRuntimePID(t, runtimeDir, 0)

	if err := child.command.Process.Kill(); err != nil {
		t.Fatalf("kill daemon child: %v", err)
	}
	if waitErr := child.waitForExit(t); waitErr == nil {
		t.Fatal("daemon child exited cleanly after SIGKILL, want killed")
	}

	preserved, err := supervisor.ReadRuntime(runtimeDir)
	if err != nil {
		t.Fatalf("read runtime after SIGKILL: %v", err)
	}
	if preserved.PID != oldRuntime.PID || preserved.Token != oldRuntime.Token {
		t.Fatalf("runtime after SIGKILL = %#v, want old daemon", preserved)
	}
	if _, socketErr := os.Stat(supervisor.SocketPath(runtimeDir)); socketErr != nil {
		t.Fatalf("socket after SIGKILL error = %v, want present", socketErr)
	}

	_ = startDaemonAt(t, runtimeDir, testVersion)
	fresh := waitForRuntimePID(t, runtimeDir, oldRuntime.PID)
	if fresh.Token == oldRuntime.Token {
		t.Fatal("runtime token did not rotate after recovery")
	}

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	t.Cleanup(func() { _ = client.Close() })
	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() after recovery error = %v, want nil", err)
	}
	if health.PID != fresh.PID {
		t.Fatalf("Health().PID = %d, want recovered PID %d", health.PID, fresh.PID)
	}

	httpClient := unixHTTPClient(t, fresh.Address)
	response := postCall(
		t,
		httpClient,
		oldRuntime.Token,
		marshalCall(t, supervisor.Call{Method: supervisor.MethodHealth}),
	)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old token status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func runDaemonRecoveryHelper(t *testing.T) {
	t.Helper()

	runtimeDir := os.Getenv(daemonRecoveryRuntimeEnv)
	if runtimeDir == "" {
		t.Fatalf("%s is empty", daemonRecoveryRuntimeEnv)
	}
	if err := supervisor.Run(context.Background(), supervisor.RunOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	}); err != nil {
		t.Fatalf("daemon run: %v", err)
	}
}

type recoveryChild struct {
	command *exec.Cmd
	done    chan struct{}
	waitErr error
	output  *bytes.Buffer
}

func startDaemonRecoveryChild(t *testing.T, runtimeDir string) *recoveryChild {
	t.Helper()

	output := new(bytes.Buffer)
	//nolint:gosec,noctx // The test binary path comes from os.Args[0].
	command := exec.Command(os.Args[0], "-test.run=^TestDaemonRecoversAfterSIGKILL$")
	command.Env = append(
		os.Environ(),
		daemonRecoveryHelperEnv+"=1",
		daemonRecoveryRuntimeEnv+"="+runtimeDir,
	)
	command.Stdout = output
	command.Stderr = output

	child := &recoveryChild{
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

func (child *recoveryChild) waitForExit(t *testing.T) error {
	t.Helper()

	select {
	case <-child.done:
		return child.waitErr
	case <-time.After(5 * time.Second):
		t.Fatalf("daemon child did not exit\n%s", child.output.String())
		return nil
	}
}

func waitForRuntimePID(t *testing.T, runtimeDir string, notPID int) supervisor.Runtime {
	t.Helper()

	deadline := time.Now().Add(testStartTimeout)
	for time.Now().Before(deadline) {
		runtime, err := supervisor.ReadRuntime(runtimeDir)
		if err == nil && runtime.PID != 0 && runtime.PID != notPID {
			return runtime
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, err := supervisor.ReadRuntime(runtimeDir)
	t.Fatalf("runtime in %q did not change from PID %d: %v", runtimeDir, notPID, err)
	return supervisor.Runtime{}
}
