//go:build !windows

package supervisor_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

func TestRuntimePathsAreStable(t *testing.T) {
	runtimeDir := t.TempDir()

	if got := supervisor.RuntimePath(runtimeDir); got != filepath.Join(runtimeDir, "daemon.json") {
		t.Fatalf("RuntimePath() = %q, want daemon.json under runtime dir", got)
	}
	if got := supervisor.SocketPath(runtimeDir); got != filepath.Join(runtimeDir, "moth.sock") {
		t.Fatalf("SocketPath() = %q, want moth.sock under runtime dir", got)
	}
}

func TestRunWritesPrivateRuntimeState(t *testing.T) {
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonAt(t, runtimeDir, testVersion)

	runtime := readRuntime(t, runtimeDir)
	if runtime.PID != os.Getpid() {
		t.Fatalf("runtime PID = %d, want test process %d", runtime.PID, os.Getpid())
	}
	if runtime.Network != "unix" {
		t.Fatalf("runtime network = %q, want unix", runtime.Network)
	}
	if runtime.Address != supervisor.SocketPath(runtimeDir) {
		t.Fatalf("runtime address = %q, want %q", runtime.Address, supervisor.SocketPath(runtimeDir))
	}
	if runtime.Version != testVersion {
		t.Fatalf("runtime version = %q, want %q", runtime.Version, testVersion)
	}
	if len(runtime.Token) < 32 {
		t.Fatalf("runtime token length = %d, want at least 32", len(runtime.Token))
	}
	if runtime.StartedAt.IsZero() {
		t.Fatal("runtime StartedAt is zero")
	}
	if since := time.Since(runtime.StartedAt); since < 0 || since > time.Minute {
		t.Fatalf("runtime StartedAt = %s, want recent time", runtime.StartedAt)
	}

	assertPerm(t, runtimeDir, 0o700)
	assertPerm(t, supervisor.RuntimePath(runtimeDir), 0o600)
	assertPerm(t, supervisor.SocketPath(runtimeDir), 0o600)
}

func TestReadRuntimeRejectsMissingAndCorruptFiles(t *testing.T) {
	runtimeDir := t.TempDir()

	if _, err := supervisor.ReadRuntime(runtimeDir); err == nil {
		t.Fatal("ReadRuntime(missing) error = nil, want error")
	}

	if err := os.WriteFile(supervisor.RuntimePath(runtimeDir), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt runtime: %v", err)
	}
	if _, err := supervisor.ReadRuntime(runtimeDir); err == nil {
		t.Fatal("ReadRuntime(corrupt) error = nil, want error")
	}
}
