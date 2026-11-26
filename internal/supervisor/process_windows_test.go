//go:build windows

package supervisor

import (
	"os"
	"os/exec"
	"testing"
)

func TestProcessAliveAcceptsCurrentProcess(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("processAlive(current PID) = false, want true")
	}
}

func TestProcessAliveRejectsInvalidPID(t *testing.T) {
	if processAlive(0) {
		t.Fatal("processAlive(0) = true, want false")
	}
}

func TestProcessAliveRejectsExitedProcess(t *testing.T) {
	//nolint:gosec,noctx // Test uses a fixed Windows command.
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	if err := command.Run(); err != nil {
		t.Fatalf("run child: %v", err)
	}
	if command.Process == nil {
		t.Fatal("child process is nil")
	}
	if processAlive(command.Process.Pid) {
		t.Fatalf("processAlive(exited PID %d) = true, want false", command.Process.Pid)
	}
}
