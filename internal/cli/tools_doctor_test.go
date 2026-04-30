package cli

import (
	"encoding/json"
	"testing"
)

func TestToolsDoctorJSONReportsMissingTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("ROD_BROWSER_BIN", "")

	stdout, stderr, err := executeRootCommand("--json", "tools", "doctor", "--tools-dir", t.TempDir())
	if err != nil {
		t.Fatalf("execute tools doctor: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	payload := stdout
	if payload == "" {
		payload = stderr
	}
	if payload == "" {
		t.Fatal("tools doctor JSON payload is empty")
	}

	var document map[string]any
	if err := json.Unmarshal([]byte(payload), &document); err != nil {
		t.Fatalf("decode tools doctor JSON %q: %v", payload, err)
	}
	if document["type"] != "tool_doctor" {
		t.Fatalf("type = %v, want tool_doctor", document["type"])
	}

	tools, ok := document["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools = %#v, want non-empty array", document["tools"])
	}

	foundYTDLP := false
	for _, item := range tools {
		status, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("tool status = %#v, want object", item)
		}
		if status["tool"] != "yt-dlp" {
			continue
		}
		foundYTDLP = true
		if status["status"] != "missing" {
			t.Fatalf("yt-dlp status = %v, want missing", status["status"])
		}
		warnings, ok := status["warnings"].([]any)
		if !ok {
			t.Fatalf("yt-dlp warnings = %#v, want array", status["warnings"])
		}
		if !jsonArrayContains(warnings, "tool_missing") {
			t.Fatalf("yt-dlp warnings = %#v, want tool_missing", warnings)
		}
	}
	if !foundYTDLP {
		t.Fatalf("yt-dlp missing from tools doctor payload: %#v", tools)
	}
}

func jsonArrayContains(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
