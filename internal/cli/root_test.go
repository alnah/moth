package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestRootCommandHelpExecutesWithoutProcessExit(t *testing.T) {
	stdout, stderr, err := executeRootCommand("--help")
	if err != nil {
		t.Fatalf("execute root help: %v", err)
	}

	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "moth") {
		t.Fatalf("help output = %q, want it to mention moth", combined)
	}
}

func TestRootCommandWithoutArgsShowsHelp(t *testing.T) {
	stdout, stderr, err := executeRootCommand()
	if err != nil {
		t.Fatalf("execute root without args: %v", err)
	}

	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "moth") {
		t.Fatalf("help output = %q, want it to mention moth", combined)
	}
}

func TestRootCommandJSONErrorShape(t *testing.T) {
	stdout, stderr, err := executeRootCommand("--json", "missing-command")
	if err == nil {
		t.Fatal("execute unknown command error = nil, want stable JSON error")
	}

	payload := strings.TrimSpace(stdout)
	if payload == "" {
		payload = strings.TrimSpace(stderr)
	}
	if payload == "" {
		t.Fatal("JSON error payload is empty")
	}

	var document map[string]any
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatalf("decode JSON error payload %q: %v", payload, err)
	}
	if got := document["type"]; got != "error" {
		t.Fatalf("type = %v, want error", got)
	}

	errorDocument, ok := document["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %#v, want object", document["error"])
	}
	if got := errorDocument["code"]; got != "unknown_command" {
		t.Fatalf("error.code = %v, want unknown_command", got)
	}
	message, isString := errorDocument["message"].(string)
	if !isString || message == "" {
		t.Fatalf("error.message = %#v, want non-empty string", errorDocument["message"])
	}
	warnings, ok := document["warnings"].([]any)
	if !ok {
		t.Fatalf("warnings field = %#v, want array", document["warnings"])
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want empty array for CLI parse error", warnings)
	}
}

func TestRootCommandPrettyJSONError(t *testing.T) {
	stdout, stderr, err := executeRootCommand("--json", "--pretty", "missing-command")
	if err == nil {
		t.Fatal("execute unknown command error = nil, want stable JSON error")
	}

	payload := stdout
	if strings.TrimSpace(payload) == "" {
		payload = stderr
	}
	if !strings.Contains(payload, "\n  \"type\": \"error\"") {
		t.Fatalf("pretty JSON payload = %q, want indented error document", payload)
	}

	var document map[string]any
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatalf("decode pretty JSON error payload %q: %v", payload, err)
	}
}

func TestRootCommandPlainUnknownCommandWritesTextError(t *testing.T) {
	stdout, stderr, err := executeRootCommand("missing-command")
	if err == nil {
		t.Fatal("execute unknown command error = nil, want error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout", stdout)
	}
	if !strings.Contains(stderr, "unknown command: missing-command") {
		t.Fatalf("stderr = %q, want unknown command message", stderr)
	}
}

func TestRootCommandJSONErrorReportsWriteFailure(t *testing.T) {
	var stdout bytes.Buffer
	stderr := failingWriter{err: errors.New("disk full")}

	err := executeRootCommandWithWriters([]string{"--json", "missing-command"}, &stdout, stderr)
	if err == nil {
		t.Fatal("execute unknown command with failing writer error = nil, want error")
	}
	if !strings.Contains(err.Error(), "write unknown command error") {
		t.Fatalf("error = %v, want write context", err)
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("error = %v, want writer failure", err)
	}
}

func executeRootCommand(args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := executeRootCommandWithWriters(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func executeRootCommandWithWriters(args []string, stdout io.Writer, stderr io.Writer) error {
	cmd := NewRootCommand()
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	return cmd.Execute()
}

type failingWriter struct {
	err error
}

func (writer failingWriter) Write([]byte) (int, error) {
	return 0, writer.err
}
