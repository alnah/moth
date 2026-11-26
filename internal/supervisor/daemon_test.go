//go:build !windows

package supervisor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/supervisor"
)

func TestRunStopsOnContextCancelAndRemovesRuntime(t *testing.T) {
	runtimeDir := newRuntimeDir(t)
	daemon := startDaemonAt(t, runtimeDir, testVersion)

	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); err != nil {
		t.Fatalf("runtime file before stop: %v", err)
	}

	if err := daemon.stop(); err != nil {
		t.Fatalf("stop daemon: %v", err)
	}
	if _, err := os.Stat(supervisor.RuntimePath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runtime file after stop err = %v, want removed", err)
	}
	if _, err := os.Stat(supervisor.SocketPath(runtimeDir)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after stop err = %v, want removed", err)
	}
}

func TestRunRejectsSecondInstance(t *testing.T) {
	runtimeDir := newRuntimeDir(t)
	_ = startDaemonAt(t, runtimeDir, testVersion)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := supervisor.Run(ctx, supervisor.RunOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	if !errors.Is(err, supervisor.ErrDaemonAlreadyRunning) {
		t.Fatalf("Run(second instance) error = %v, want ErrDaemonAlreadyRunning", err)
	}
}

func TestRunReplacesStaleSocket(t *testing.T) {
	runtimeDir := newRuntimeDir(t)
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	writeStaleSocket(t, supervisor.SocketPath(runtimeDir))

	_ = startDaemonAt(t, runtimeDir, testVersion)

	client := supervisor.NewClient(supervisor.ClientOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	health, err := client.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() after stale socket = %v, want nil", err)
	}
	if health.Status != "ok" {
		t.Fatalf("Health().Status = %q, want ok", health.Status)
	}
}

func TestRunFailsWhenRuntimeDirIsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write runtime dir file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := supervisor.Run(ctx, supervisor.RunOptions{
		RuntimeDir: path,
		Version:    testVersion,
	})
	if err == nil {
		t.Fatal("Run(runtime dir is file) error = nil, want error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run(runtime dir is file) hung until context deadline: %v", err)
	}
}

func TestRunWithCanceledContextDoesNotLeaveRuntime(t *testing.T) {
	runtimeDir := newRuntimeDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := supervisor.Run(ctx, supervisor.RunOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run(canceled context) error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(supervisor.RuntimePath(runtimeDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("runtime file after canceled start err = %v, want absent", statErr)
	}
}

func TestDaemonWritesStartAndStopLogs(t *testing.T) {
	runtimeDir := newRuntimeDir(t)
	logPath := filepath.Join(t.TempDir(), "log", "daemon.log")
	daemon := startDaemonWithOptions(t, runtimeDir, supervisor.RunOptions{
		LogPath: logPath,
	})
	runtime := readRuntime(t, runtimeDir)

	if err := daemon.stop(); err != nil {
		t.Fatalf("stop daemon: %v", err)
	}

	messages := daemonLogMessages(t, logPath)
	for _, want := range []string{"daemon started", "daemon stopping", "daemon stopped"} {
		if !messages[want] {
			t.Fatalf("log messages = %v, want %q", messages, want)
		}
	}

	data, err := os.ReadFile(logPath) //nolint:gosec // Test path is temporary.
	if err != nil {
		t.Fatalf("read daemon log: %v", err)
	}
	if strings.Contains(string(data), runtime.Token) {
		t.Fatal("daemon log contains the runtime token")
	}
}

func TestDaemonFailsWhenLogPathUnwritable(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write log blocker: %v", err)
	}

	runtimeDir := newRuntimeDir(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := supervisor.Run(ctx, supervisor.RunOptions{
		RuntimeDir: runtimeDir,
		Version:    testVersion,
		LogPath:    filepath.Join(blocker, "daemon.log"),
	})
	if err == nil {
		t.Fatal("Run(unwritable log path) error = nil, want error")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run(unwritable log path) hung until context deadline: %v", err)
	}
	if _, statErr := os.Stat(supervisor.RuntimePath(runtimeDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("runtime file after log failure err = %v, want absent", statErr)
	}
}

func TestDaemonCustomLoggerWinsOverLogPath(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	runtimeDir := newRuntimeDir(t)
	logPath := filepath.Join(t.TempDir(), "log", "daemon.log")
	daemon := startDaemonWithOptions(t, runtimeDir, supervisor.RunOptions{
		Logger:  logger,
		LogPath: logPath,
	})

	if err := daemon.stop(); err != nil {
		t.Fatalf("stop daemon: %v", err)
	}

	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("custom logger created log file, stat error = %v", err)
	}
	if !strings.Contains(logs.String(), "daemon started") {
		t.Fatalf("custom logger missing start event: %s", logs.String())
	}
}

func daemonLogMessages(t *testing.T, path string) map[string]bool {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // Test path is temporary.
	if err != nil {
		t.Fatalf("read daemon log: %v", err)
	}
	messages := make(map[string]bool)
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var record struct {
			Message string `json:"msg"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode daemon log line: %v\n%s", err, line)
		}
		messages[record.Message] = true
	}
	return messages
}
