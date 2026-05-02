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
	stdout, stderr, err := executeRootCommand("missing-command")
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
	stdout, stderr, err := executeRootCommand("--pretty", "missing-command")
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

func TestRootCommandRejectsJSONFlag(t *testing.T) {
	stdout, stderr, err := executeRootCommand("--json")
	if err == nil {
		t.Fatal("execute --json error = nil, want unsupported flag error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout", stdout)
	}
	if !strings.Contains(err.Error(), "unknown flag: --json") {
		t.Fatalf("error = %v, want unsupported --json flag", err)
	}

	payload := strings.TrimSpace(stderr)
	if payload == "" {
		t.Fatal("unsupported flag JSON error payload is empty")
	}

	var document map[string]any
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatalf("decode unsupported flag JSON error payload %q: %v", payload, err)
	}
	errorDocument, ok := document["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %#v, want object", document["error"])
	}
	if got := errorDocument["code"]; got != "invalid_arguments" {
		t.Fatalf("error.code = %v, want invalid_arguments", got)
	}
	if got := errorDocument["message"]; got != "unknown flag: --json" {
		t.Fatalf("error.message = %v, want unsupported --json flag", got)
	}
}

func TestCommandErrorsEmitSingleStableJSONDocumentByDefault(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantCode        string
		wantMessagePart string
	}{
		{
			name:            "root unknown command",
			args:            []string{"missing-command"},
			wantCode:        "unknown_command",
			wantMessagePart: "unknown command: missing-command",
		},
		{
			name:            "root invalid flag",
			args:            []string{"--json"},
			wantCode:        "invalid_arguments",
			wantMessagePart: "unknown flag: --json",
		},
		{
			name:            "subcommand positional argument",
			args:            []string{"tools", "doctor", "extra"},
			wantCode:        "invalid_arguments",
			wantMessagePart: "tools doctor accepts no positional arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, err := executeRootCommand(tt.args...)
			if err == nil {
				t.Fatalf("execute %q error = nil, want command error", strings.Join(tt.args, " "))
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty stdout for command error", stdout)
			}

			document := decodeSingleJSONErrorDocument(t, stderr)
			if document.Type != "error" {
				t.Fatalf("type = %q, want error", document.Type)
			}
			if document.Error.Code != tt.wantCode {
				t.Fatalf("error.code = %q, want %q", document.Error.Code, tt.wantCode)
			}
			if !strings.Contains(document.Error.Message, tt.wantMessagePart) {
				t.Fatalf("error.message = %q, want it to contain %q", document.Error.Message, tt.wantMessagePart)
			}
			if document.Warnings == nil {
				t.Fatal("warnings = nil, want empty array")
			}
			if len(document.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want empty array", document.Warnings)
			}
		})
	}
}

func TestRootCommandJSONErrorReportsWriteFailure(t *testing.T) {
	var stdout bytes.Buffer
	stderr := failingWriter{err: errors.New("disk full")}

	err := executeRootCommandWithWriters([]string{"missing-command"}, &stdout, stderr)
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

type cliErrorJSON struct {
	Type     string           `json:"type"`
	Error    cliErrorJSONBody `json:"error"`
	Warnings []string         `json:"warnings"`
}

type cliErrorJSONBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeSingleJSONErrorDocument(t *testing.T, payload string) cliErrorJSON {
	t.Helper()

	if strings.TrimSpace(payload) == "" {
		t.Fatal("JSON error payload is empty")
	}

	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.DisallowUnknownFields()

	var document cliErrorJSON
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode JSON error payload %q: %v", payload, err)
	}

	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("JSON error payload %q contains extra document", payload)
	}

	return document
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
