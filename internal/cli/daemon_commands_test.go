package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/alnah/moth/internal/supervisor"
)

func TestDaemonCommandHelpListsStatusAndStop(t *testing.T) {
	stdout, stderr, err := executeRootCommand("daemon", "--help")
	if err != nil {
		t.Fatalf("execute daemon help: %v", err)
	}
	help := stdout + stderr
	for _, name := range []string{"status", "stop"} {
		if !strings.Contains(help, name) {
			t.Fatalf("daemon help = %q, want %s", help, name)
		}
	}
}

func TestDaemonStatusMissingRendersStatusJSON(t *testing.T) {
	runtimeDir := t.TempDir()

	stdout, stderr, err := executeRootCommand("daemon", "status", "--runtime-dir", runtimeDir)
	if err != nil {
		t.Fatalf("execute daemon status: %v\nstderr: %s", err, stderr)
	}
	document := decodeDaemonStatus(t, stdout)
	if document.Type != "daemon_status" {
		t.Fatalf("type = %q, want daemon_status", document.Type)
	}
	if document.Status != "missing" {
		t.Fatalf("status = %q, want missing", document.Status)
	}
}

func TestDaemonStatusRunningHidesToken(t *testing.T) {
	runtimeDir := newShortRuntimeDir(t)
	startTestDaemon(t, runtimeDir)
	runtime, err := supervisor.ReadRuntime(runtimeDir)
	if err != nil {
		t.Fatalf("read runtime: %v", err)
	}

	stdout, stderr, err := executeRootCommand("daemon", "status", "--runtime-dir", runtimeDir)
	if err != nil {
		t.Fatalf("execute daemon status: %v\nstderr: %s", err, stderr)
	}
	document := decodeDaemonStatus(t, stdout)
	if document.Type != "daemon_status" || document.Status != "running" {
		t.Fatalf("daemon status = %#v, want running", document)
	}
	if strings.Contains(stdout, runtime.Token) {
		t.Fatal("daemon status JSON contains the runtime token")
	}
}

func TestDaemonStopMissingAndRunning(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		runtimeDir := t.TempDir()
		stdout, stderr, err := executeRootCommand("daemon", "stop", "--runtime-dir", runtimeDir)
		if err != nil {
			t.Fatalf("execute daemon stop: %v\nstderr: %s", err, stderr)
		}
		document := decodeDaemonStatus(t, stdout)
		if document.Status != "stopped" {
			t.Fatalf("status = %q, want stopped", document.Status)
		}
	})

	t.Run("running", func(t *testing.T) {
		runtimeDir := newShortRuntimeDir(t)
		startTestDaemon(t, runtimeDir)

		stdout, stderr, err := executeRootCommand("daemon", "stop", "--runtime-dir", runtimeDir)
		if err != nil {
			t.Fatalf("execute daemon stop: %v\nstderr: %s", err, stderr)
		}
		document := decodeDaemonStatus(t, stdout)
		if document.Status != "stopped" {
			t.Fatalf("status = %q, want stopped", document.Status)
		}
		if _, err := supervisor.ReadRuntime(runtimeDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("runtime after stop error = %v, want removed", err)
		}
	})
}

func TestDaemonRunPassesOptionsAndUsesRootContext(t *testing.T) {
	previousRunDaemon := runDaemon
	t.Cleanup(func() { runDaemon = previousRunDaemon })

	var got supervisor.RunOptions
	var hadDeadline bool
	runDaemon = func(ctx context.Context, options supervisor.RunOptions) error {
		_, hadDeadline = ctx.Deadline()
		got = options
		return nil
	}

	stdout, stderr, err := executeRootCommand(
		"daemon", "run",
		"--runtime-dir", "/tmp/moth-test-run",
		"--browser-bin", "/opt/chrome",
	)
	if err != nil {
		t.Fatalf("execute daemon run: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if hadDeadline {
		t.Fatal("daemon run context has a deadline, want root context without CLI timeout")
	}
	if got.RuntimeDir != "/tmp/moth-test-run" || got.BrowserBin != "/opt/chrome" {
		t.Fatalf("run options = %#v, want runtime dir and browser bin", got)
	}
}

func TestCLIErrorMapsDaemonErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "unavailable", err: supervisor.ErrDaemonUnavailable, want: "daemon_unavailable"},
		{name: "version", err: supervisor.ErrDaemonVersion, want: "daemon_version_mismatch"},
		{name: "auth", err: supervisor.ErrDaemonAuth, want: "daemon_unauthorized"},
		{name: "protocol", err: supervisor.ErrDaemonProtocol, want: "daemon_protocol_error"},
		{name: "method", err: supervisor.ErrDaemonMethod, want: "daemon_method_failed"},
		{name: "already running", err: supervisor.ErrDaemonAlreadyRunning, want: "daemon_already_running"},
		{name: "wrapped method", err: wrapError{supervisor.ErrDaemonMethod}, want: "daemon_method_failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := commandErrorFields(tt.err)
			if got != tt.want {
				t.Fatalf("commandErrorFields() = %q, want %q", got, tt.want)
			}
		})
	}
}

type wrapError struct{ err error }

func (err wrapError) Error() string { return "wrapped: " + err.err.Error() }
func (err wrapError) Unwrap() error { return err.err }

type daemonStatusDocumentTest struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

func decodeDaemonStatus(t *testing.T, payload string) daemonStatusDocumentTest {
	t.Helper()

	var document daemonStatusDocumentTest
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatalf("decode daemon status %q: %v", payload, err)
	}
	return document
}

func TestDaemonRunPassesDefaultLogPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	previousRunDaemon := runDaemon
	t.Cleanup(func() { runDaemon = previousRunDaemon })

	var got supervisor.RunOptions
	runDaemon = func(_ context.Context, options supervisor.RunOptions) error {
		got = options
		return nil
	}

	stdout, stderr, err := executeRootCommand("daemon", "run")
	if err != nil {
		t.Fatalf("execute daemon run: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	want := supervisor.DefaultLogPath()
	if got.LogPath != want {
		t.Fatalf("RunOptions.LogPath = %q, want %q", got.LogPath, want)
	}
	if _, statErr := os.Stat(want); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("CLI created log file, stat error = %v", statErr)
	}
}
