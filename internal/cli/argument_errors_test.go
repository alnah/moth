package cli

import (
	"strings"
	"testing"
)

func TestCommandArgumentErrorsRenderStableJSON(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantMessagePart string
	}{
		{
			name:            "search web missing query",
			args:            []string{"search", "web"},
			wantMessagePart: "web search accepts exactly one query",
		},
		{
			name:            "search images extra query",
			args:            []string{"search", "images", "one", "two"},
			wantMessagePart: "images search accepts exactly one query",
		},
		{
			name:            "fetch missing URL",
			args:            []string{"fetch"},
			wantMessagePart: "fetch accepts exactly one URL",
		},
		{
			name:            "youtube search missing query",
			args:            []string{"youtube", "search"},
			wantMessagePart: "youtube search accepts exactly one query",
		},
		{
			name:            "youtube metadata extra ID",
			args:            []string{"youtube", "metadata", "one", "two"},
			wantMessagePart: "youtube metadata accepts exactly one URL or ID",
		},
		{
			name:            "youtube subtitles missing ID",
			args:            []string{"youtube", "subtitles"},
			wantMessagePart: "youtube subtitles accepts exactly one URL or ID",
		},
		{
			name:            "youtube audio extra ID",
			args:            []string{"youtube", "audio", "one", "two"},
			wantMessagePart: "youtube audio accepts exactly one URL or ID",
		},
		{
			name:            "podcast search missing query",
			args:            []string{"podcast", "search"},
			wantMessagePart: "podcast search accepts exactly one query",
		},
		{
			name:            "podcast episodes invalid feed ID",
			args:            []string{"podcast", "episodes", "not-an-id"},
			wantMessagePart: "invalid feed ID",
		},
		{
			name:            "podcast audio missing GUID",
			args:            []string{"podcast", "audio", "https://feed.test/rss.xml"},
			wantMessagePart: "podcast audio accepts feed URL and episode GUID",
		},
		{
			name:            "x search missing query",
			args:            []string{"x", "search"},
			wantMessagePart: "x search accepts exactly one query",
		},
		{
			name:            "x post missing ID",
			args:            []string{"x", "post"},
			wantMessagePart: "x post accepts exactly one post ID",
		},
		{
			name:            "x user extra ID",
			args:            []string{"x", "user", "one", "two"},
			wantMessagePart: "x user accepts exactly one user ID",
		},
		{
			name:            "pdf2txt missing input",
			args:            []string{"pdf2txt"},
			wantMessagePart: "pdf2txt accepts exactly one file or URL",
		},
		{
			name:            "transcribe extra file",
			args:            []string{"transcribe", "one.mp3", "two.mp3"},
			wantMessagePart: "transcribe accepts exactly one file",
		},
		{
			name:            "tools doctor extra argument",
			args:            []string{"tools", "doctor", "extra"},
			wantMessagePart: "tools doctor accepts no positional arguments",
		},
		{
			name:            "browser start extra argument",
			args:            []string{"browser", "start", "extra"},
			wantMessagePart: "browser start accepts no positional arguments",
		},
		{
			name:            "browser connect missing host",
			args:            []string{"browser", "connect"},
			wantMessagePart: "browser connect accepts exactly one host:port",
		},
		{
			name:            "browser close-page too many IDs",
			args:            []string{"browser", "close-page", "one", "two"},
			wantMessagePart: "browser close-page accepts at most one page ID",
		},
		{
			name:            "browser wait invalid state",
			args:            []string{"browser", "wait", ".ready", "--state", "hidden"},
			wantMessagePart: "invalid wait state \"hidden\"",
		},
		{
			name:            "browser screenshot missing path",
			args:            []string{"browser", "screenshot", "https://example.test"},
			wantMessagePart: "browser screenshot accepts URL and path",
		},
		{
			name:            "browser pdf missing path",
			args:            []string{"browser", "pdf", "https://example.test"},
			wantMessagePart: "browser pdf accepts URL and path",
		},
		{
			name:            "browser download missing path",
			args:            []string{"browser", "download", "a.save"},
			wantMessagePart: "browser download accepts selector and path",
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
			if !strings.Contains(document.Error.Message, tt.wantMessagePart) {
				t.Fatalf("error.message = %q, want it to contain %q", document.Error.Message, tt.wantMessagePart)
			}
			if len(document.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want empty array", document.Warnings)
			}
		})
	}
}
