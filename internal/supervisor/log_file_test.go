//go:build !windows

package supervisor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLogMaxBytes(t *testing.T) {
	if DefaultLogMaxBytes != 10<<20 {
		t.Fatalf("DefaultLogMaxBytes = %d, want 10 MiB", DefaultLogMaxBytes)
	}
}

func TestOpenLogFileCreatesPrivateJSONLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log", "daemon.log")
	logFile, err := OpenLogFile(path, 0)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v, want nil", err)
	}

	logFile.Logger().Info("daemon started", "answer", 42)
	if closeErr := logFile.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v, want nil", closeErr)
	}

	assertLogFilePerm(t, filepath.Dir(path), 0o700)
	assertLogFilePerm(t, path, 0o600)

	data, err := os.ReadFile(path) //nolint:gosec // Test path is temporary.
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("log line count = %d, want 1\n%s", len(lines), data)
	}

	var record map[string]any
	if err := json.Unmarshal(lines[0], &record); err != nil {
		t.Fatalf("decode log line: %v\n%s", err, lines[0])
	}
	if record["msg"] != "daemon started" {
		t.Fatalf("log msg = %v, want daemon started", record["msg"])
	}
	if record["level"] != "INFO" {
		t.Fatalf("log level = %v, want INFO", record["level"])
	}
	if _, ok := record["time"]; !ok {
		t.Fatal("log line has no time field")
	}
	if record["answer"] != float64(42) {
		t.Fatalf("log answer = %v, want 42", record["answer"])
	}
}

func TestOpenLogFileRejectsEmptyPath(t *testing.T) {
	for _, path := range []string{"", "  "} {
		if _, err := OpenLogFile(path, 0); err == nil {
			t.Fatalf("OpenLogFile(%q) error = nil, want error", path)
		}
	}
}

func TestOpenLogFileRejectsRelativePath(t *testing.T) {
	if _, err := OpenLogFile("daemon.log", 0); err == nil {
		t.Fatal("OpenLogFile(relative path) error = nil, want error")
	}
}

func TestOpenLogFileSecuresExistingFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "log")
	//nolint:gosec // Test starts with a permissive directory to verify tightening.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create log directory: %v", err)
	}
	path := filepath.Join(dir, "daemon.log")
	//nolint:gosec // Test starts with a permissive file to verify tightening.
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write existing log: %v", err)
	}

	logFile, err := OpenLogFile(path, 0)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v, want nil", err)
	}
	logFile.Logger().Info("new")
	if closeErr := logFile.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v, want nil", closeErr)
	}

	assertLogFilePerm(t, dir, 0o700)
	assertLogFilePerm(t, path, 0o600)
	data, err := os.ReadFile(path) //nolint:gosec // Test path is temporary.
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !bytes.Contains(data, []byte("old\n")) || !bytes.Contains(data, []byte(`"msg":"new"`)) {
		t.Fatalf("log file did not preserve and append: %q", data)
	}
}

func TestOpenLogFileRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.log")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	path := filepath.Join(dir, "daemon.log")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := OpenLogFile(path, 0); err == nil {
		t.Fatal("OpenLogFile(symlink) error = nil, want error")
	}
}

func TestOpenLogFileRejectsNonRegularPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log", "daemon.log")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("create directory at log path: %v", err)
	}

	if _, err := OpenLogFile(path, 0); err == nil {
		t.Fatal("OpenLogFile(directory path) error = nil, want error")
	}
}

func TestOpenLogFileRotatesSingleBackup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "log")
	path := filepath.Join(dir, "daemon.log")
	maxBytes := int64(256)
	logFile, err := OpenLogFile(path, maxBytes)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v, want nil", err)
	}

	for index := range 32 {
		logFile.Logger().Info("rotation",
			"index", index,
			"padding", strings.Repeat("x", 48),
		)
	}
	if closeErr := logFile.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v, want nil", closeErr)
	}

	currentInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat current log: %v", err)
	}
	if currentInfo.Size() >= maxBytes {
		t.Fatalf("current log size = %d, want < %d", currentInfo.Size(), maxBytes)
	}

	backupInfo, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatalf("stat backup log: %v", err)
	}
	if backupInfo.Size() == 0 {
		t.Fatal("backup log is empty")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read log directory: %v", err)
	}
	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		names[entry.Name()] = true
	}
	if !names["daemon.log"] || !names["daemon.log.1"] || len(names) != 2 {
		t.Fatalf("log directory names = %v, want daemon.log and daemon.log.1", names)
	}
}

func TestOpenLogFileKeepsWritingWhenRotationFails(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "log")
	path := filepath.Join(dir, "daemon.log")
	backupPath := path + ".1"
	if err := os.MkdirAll(backupPath, 0o700); err != nil {
		t.Fatalf("create backup directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupPath, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatalf("create backup blocker: %v", err)
	}

	logFile, err := OpenLogFile(path, 64)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v, want nil", err)
	}
	defer func() { _ = logFile.Close() }()

	first := []byte(strings.Repeat("a", 100))
	n, writeErr := logFile.writer.Write(first)
	if writeErr != nil || n != len(first) {
		t.Fatalf("first Write() = %d, %v, want %d, nil", n, writeErr, len(first))
	}

	second := []byte("second\n")
	n, err = logFile.writer.Write(second)
	if n != len(second) {
		t.Fatalf("second Write() count = %d, want %d", n, len(second))
	}
	if err == nil {
		t.Fatal("second Write() error = nil, want rotation error")
	}

	data, readErr := os.ReadFile(path) //nolint:gosec // Test path is temporary.
	if readErr != nil {
		t.Fatalf("read current log: %v", readErr)
	}
	if !bytes.Contains(data, first) || !bytes.Contains(data, second) {
		t.Fatalf("current log lost writes after rotation failure: %q", data)
	}
}

func TestOpenLogFileRecoversWhenRenameFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}

	dir := filepath.Join(t.TempDir(), "log")
	path := filepath.Join(dir, "daemon.log")
	logFile, err := OpenLogFile(path, 64)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v, want nil", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0o700) //nolint:gosec // Test restores the owner-only directory.
		_ = logFile.Close()
	})

	first := []byte(strings.Repeat("a", 100))
	if n, writeErr := logFile.writer.Write(first); writeErr != nil || n != len(first) {
		t.Fatalf("first Write() = %d, %v, want %d, nil", n, writeErr, len(first))
	}
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // Test removes owner write permission.
		t.Fatalf("make log directory read-only: %v", err)
	}

	second := []byte("second\n")
	n, writeErr := logFile.writer.Write(second)
	if n != len(second) {
		t.Fatalf("second Write() count = %d, want %d", n, len(second))
	}
	if writeErr == nil {
		t.Fatal("second Write() error = nil, want rotation error")
	}
	if err := os.Chmod(dir, 0o700); err != nil { //nolint:gosec // Test restores the owner-only directory.
		t.Fatalf("restore log directory: %v", err)
	}

	data, readErr := os.ReadFile(path) //nolint:gosec // Test path is temporary.
	if readErr != nil {
		t.Fatalf("read current log: %v", readErr)
	}
	if !bytes.Contains(data, first) || !bytes.Contains(data, second) {
		t.Fatalf("current log lost writes after rename failure: %q", data)
	}
}

func TestLogFileNilReceiver(t *testing.T) {
	var logFile *LogFile
	if logFile.Logger() != nil {
		t.Fatal("nil LogFile.Logger() != nil")
	}
	if err := logFile.Close(); err != nil {
		t.Fatalf("nil LogFile.Close() error = %v, want nil", err)
	}
}

func TestOpenLogFileCloseIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log", "daemon.log")
	logFile, err := OpenLogFile(path, 0)
	if err != nil {
		t.Fatalf("OpenLogFile() error = %v, want nil", err)
	}

	if err := logFile.Close(); err != nil {
		t.Fatalf("first Close() error = %v, want nil", err)
	}
	if err := logFile.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}

	logFile.Logger().Info("after close")
}

func assertLogFilePerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}
