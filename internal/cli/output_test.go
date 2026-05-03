package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOutputPathWriteFailureRendersStableJSONError(t *testing.T) {
	harness := newCommandHarness()
	outputPath := t.TempDir()

	stdout, stderr, err := harness.execute("--output", outputPath, "search", "web", "moth")
	if err == nil {
		t.Fatal("execute command with directory output path error = nil, want write error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout when output path write fails", stdout)
	}

	document := decodeSingleJSONErrorDocument(t, stderr)
	if document.Error.Code != "command_failed" {
		t.Fatalf("error.code = %q, want command_failed", document.Error.Code)
	}
	if !strings.Contains(document.Error.Message, "write output JSON") {
		t.Fatalf("error.message = %q, want output write context", document.Error.Message)
	}
}

func TestOutputPathCreatesParentDirectoriesAndSuppressesStdout(t *testing.T) {
	harness := newCommandHarness()
	outputPath := filepath.Join(t.TempDir(), "nested", "result.json")

	stdout, stderr, err := harness.execute("--output", outputPath, "search", "web", "moth")
	if err != nil {
		t.Fatalf("execute command with nested output path: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout when --output writes JSON", stdout)
	}

	assertContentPackFile(t, outputPath)
}

func assertContentPackFile(t *testing.T, path string) {
	t.Helper()

	payload, err := os.ReadFile(path) //nolint:gosec // Test reads the command output path under t.TempDir().
	if err != nil {
		t.Fatalf("read output JSON: %v", err)
	}
	assertContentPackJSON(t, string(payload))
}
