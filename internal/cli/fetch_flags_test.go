package cli

import (
	"strings"
	"testing"
)

func TestFetchHelpOmitsUnsupportedFlags(t *testing.T) {
	stdout, stderr, err := executeRootCommand("fetch", "--help")
	if err != nil {
		t.Fatalf("execute fetch help: %v", err)
	}

	help := stdout + stderr
	for _, flag := range []string{"--html", "--media", "--screenshot", "--download"} {
		if strings.Contains(help, flag) {
			t.Fatalf("fetch help = %q, want it to omit unsupported flag %s", help, flag)
		}
	}
}

func TestFetchRejectsUnsupportedFlagsAsInvalidArguments(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "html",
			args: []string{"fetch", "https://example.test", "--html"},
		},
		{
			name: "media",
			args: []string{"fetch", "https://example.test", "--media"},
		},
		{
			name: "screenshot",
			args: []string{"fetch", "https://example.test", "--screenshot", "shot.png"},
		},
		{
			name: "download",
			args: []string{"fetch", "https://example.test", "--download"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()

			stdout, stderr, err := harness.execute(tt.args...)
			if err == nil {
				t.Fatalf("execute %q error = nil, want invalid arguments error", strings.Join(tt.args, " "))
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty stdout for invalid arguments", stdout)
			}

			document := decodeSingleJSONErrorDocument(t, stderr)
			if document.Type != "error" {
				t.Fatalf("type = %q, want error", document.Type)
			}
			if document.Error.Code != "invalid_arguments" {
				t.Fatalf("error.code = %q, want invalid_arguments", document.Error.Code)
			}
			if len(document.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want empty array", document.Warnings)
			}
		})
	}
}

func TestFetchSupportedFlagsStillRoute(t *testing.T) {
	harness := newCommandHarness()

	stdout, stderr, err := harness.execute("fetch", "https://example.test", "--text", "--browser")
	if err != nil {
		t.Fatalf("execute fetch with supported flags: %v\nstderr: %s", err, stderr)
	}
	assertContentPackJSON(t, stdout)

	request := harness.webFetch.request
	if request.URL != "https://example.test" {
		t.Fatalf("fetch URL = %q, want https://example.test", request.URL)
	}
	if !request.IncludeText {
		t.Fatal("fetch IncludeText = false, want true")
	}
	if !request.UseBrowser {
		t.Fatal("fetch UseBrowser = false, want true")
	}
}
