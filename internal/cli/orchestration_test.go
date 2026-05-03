package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/browser"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/pdf2txt"
	"github.com/alnah/moth/internal/podcast"
	"github.com/alnah/moth/internal/tools"
	"github.com/alnah/moth/internal/transcription"
	"github.com/alnah/moth/internal/webfetch"
	"github.com/alnah/moth/internal/websearch"
	xclient "github.com/alnah/moth/internal/x"
	"github.com/alnah/moth/internal/youtube"
	"github.com/alnah/moth/internal/ytdlp"
)

var _ interface {
	LookupUserByUsername(context.Context, xclient.UsernameLookupOptions) (content.Pack, error)
} = (XService)(nil)

func TestExecuteRendersTopLevelJSONErrorsWithoutProcessExit(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := Execute(context.Background(), []string{"missing-command"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("Execute error = nil, want unknown command error")
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty stdout for error", stdout.String())
	}

	document := decodeSingleJSONErrorDocument(t, stderr.String())
	if document.Error.Code != "unknown_command" {
		t.Fatalf("error.code = %q, want unknown_command", document.Error.Code)
	}
}

func TestSearchCommandsRouteTypedOptionsAndRenderContentPack(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		want       websearch.Options
	}{
		{
			name: "web",
			args: []string{
				"search", "web", "moth testing",
				"--count", "7",
				"--offset", "3",
				"--country", "FR",
				"--lang", "fr",
				"--safe", "strict",
			},
			wantMethod: "web",
			want: websearch.Options{
				Query:      "moth testing",
				Count:      7,
				Country:    "FR",
				Language:   "fr",
				SafeSearch: "strict",
				Offset:     3,
			},
		},
		{
			name:       "images use global max results as count",
			args:       []string{"--max-results", "5", "search", "images", "wing scales"},
			wantMethod: "images",
			want: websearch.Options{
				Query: "wing scales",
				Count: 5,
			},
		},
		{
			name:       "videos",
			args:       []string{"search", "videos", "nocturnal insects"},
			wantMethod: "videos",
			want: websearch.Options{
				Query: "nocturnal insects",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()

			stdout, stderr, err := harness.execute(tt.args...)
			if err != nil {
				t.Fatalf("execute search command: %v\nstderr: %s", err, stderr)
			}
			assertContentPackJSON(t, stdout)
			if harness.webSearch.method != tt.wantMethod {
				t.Fatalf("search method = %q, want %q", harness.webSearch.method, tt.wantMethod)
			}
			if !reflect.DeepEqual(harness.webSearch.options, tt.want) {
				t.Fatalf("websearch options = %#v, want %#v", harness.webSearch.options, tt.want)
			}
		})
	}
}

func TestFetchRoutesUseCaseLimitsBrowserModeAndOutputFile(t *testing.T) {
	harness := newCommandHarness()
	outputPath := filepath.Join(t.TempDir(), "fetch.json")

	stdout, stderr, err := harness.execute(
		"--timeout", "2s",
		"--max-bytes", "4096",
		"--output", outputPath,
		"fetch", "--browser", "https://example.test/page",
	)
	if err != nil {
		t.Fatalf("execute fetch command: %v\nstderr: %s", err, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout when --output writes result document", stdout)
	}

	payload, err := os.ReadFile(outputPath) //nolint:gosec // Test reads the command output path under t.TempDir().
	if err != nil {
		t.Fatalf("read output JSON: %v", err)
	}
	assertContentPackJSON(t, string(payload))

	want := webfetch.Request{
		URL:        "https://example.test/page",
		UseBrowser: true,
		MaxBytes:   4096,
		Timeout:    2 * time.Second,
	}
	if !reflect.DeepEqual(harness.webFetch.request, want) {
		t.Fatalf("webfetch request = %#v, want %#v", harness.webFetch.request, want)
	}
}

func TestAcquisitionCommandsRouteFakesAndRenderContentPack(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		assert func(*testing.T, *commandHarness)
	}{
		{
			name: "youtube search",
			args: []string{
				"--max-results", "12",
				"youtube", "search", "lofi moths",
				"--region", "CA",
				"--lang", "fr",
				"--safe", "strict",
			},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := youtube.SearchOptions{
					Query:             "lofi moths",
					MaxResults:        12,
					RegionCode:        "CA",
					RelevanceLanguage: "fr",
					SafeSearch:        "strict",
				}
				if !reflect.DeepEqual(harness.youtube.search, want) {
					t.Fatalf("youtube search options = %#v, want %#v", harness.youtube.search, want)
				}
			},
		},
		{
			name: "youtube metadata",
			args: []string{"youtube", "metadata", "video123"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := youtube.VideoDetailsOptions{IDs: []string{"video123"}}
				if !reflect.DeepEqual(harness.youtube.details, want) {
					t.Fatalf("youtube metadata options = %#v, want %#v", harness.youtube.details, want)
				}
			},
		},
		{
			name: "youtube subtitles",
			args: []string{
				"youtube", "subtitles", "https://youtu.be/video123",
				"--output-dir", "/tmp/subs",
				"--language", "fr",
				"--format", "vtt",
				"--automatic",
			},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := ytdlp.SubtitleRequest{
					URL:              "https://youtu.be/video123",
					OutputDir:        "/tmp/subs",
					Languages:        []string{"fr"},
					Format:           "vtt",
					IncludeAutomatic: true,
				}
				if !reflect.DeepEqual(harness.ytdlp.subtitles, want) {
					t.Fatalf("youtube subtitles request = %#v, want %#v", harness.ytdlp.subtitles, want)
				}
			},
		},
		{
			name: "youtube audio",
			args: []string{"youtube", "audio", "https://youtu.be/video123", "--output-dir", "/tmp/audio", "--format", "mp3"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := ytdlp.AudioRequest{URL: "https://youtu.be/video123", OutputDir: "/tmp/audio", Format: "mp3"}
				if !reflect.DeepEqual(harness.ytdlp.audio, want) {
					t.Fatalf("youtube audio request = %#v, want %#v", harness.ytdlp.audio, want)
				}
			},
		},
		{
			name: "podcast search",
			args: []string{"--max-results", "6", "podcast", "search", "biology", "--clean", "--fulltext"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := podcast.SearchOptions{Query: "biology", MaxResults: 6, Clean: true, FullText: true}
				if !reflect.DeepEqual(harness.podcast.search, want) {
					t.Fatalf("podcast search options = %#v, want %#v", harness.podcast.search, want)
				}
			},
		},
		{
			name: "podcast episodes",
			args: []string{"podcast", "episodes", "12345", "--max-results", "4", "--fulltext"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := podcast.EpisodesByFeedIDOptions{FeedID: 12345, MaxResults: 4, FullText: true}
				if !reflect.DeepEqual(harness.podcast.episodes, want) {
					t.Fatalf("podcast episodes options = %#v, want %#v", harness.podcast.episodes, want)
				}
			},
		},
		{
			name: "podcast audio",
			args: []string{
				"--max-bytes", "2048",
				"podcast", "audio", "https://feed.test/rss.xml", "episode-guid",
				"--content-type", "audio/mpeg",
			},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := podcast.AudioDownloadOptions{
					FeedURL:             "https://feed.test/rss.xml",
					EpisodeGUID:         "episode-guid",
					AllowedContentTypes: []string{"audio/mpeg"},
					MaxBytes:            2048,
				}
				if !reflect.DeepEqual(harness.podcastAudio.request, want) {
					t.Fatalf("podcast audio request = %#v, want %#v", harness.podcastAudio.request, want)
				}
			},
		},
		{
			name: "x search",
			args: []string{"--max-results", "9", "x", "search", "from:alnah moth", "--next-token", "next"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := xclient.SearchOptions{Query: "from:alnah moth", MaxResults: 9, NextToken: "next"}
				if !reflect.DeepEqual(harness.x.search, want) {
					t.Fatalf("x search options = %#v, want %#v", harness.x.search, want)
				}
			},
		},
		{
			name: "x post",
			args: []string{"x", "post", "187"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.x.post.ID != "187" {
					t.Fatalf("x post id = %q, want 187", harness.x.post.ID)
				}
			},
		},
		{
			name: "pdf2txt",
			args: []string{"--max-bytes", "8192", "pdf2txt", "paper.pdf", "--ocr", "--ocr-language", "fra"},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				if harness.pdf.input != "paper.pdf" {
					t.Fatalf("pdf input = %q, want paper.pdf", harness.pdf.input)
				}
				want := pdf2txt.Options{OCRAllowed: true, OCRLanguage: "fra", MaxTextBytes: 8192}
				if !reflect.DeepEqual(harness.pdf.options, want) {
					t.Fatalf("pdf options = %#v, want %#v", harness.pdf.options, want)
				}
			},
		},
		{
			name: "transcribe",
			args: []string{
				"transcribe", "voice.mp3",
				"--language", "fr",
				"--model", "whisper-1",
				"--response-format", "verbose_json",
				"--timestamp-granularity", "segment",
			},
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := transcription.Request{
					FilePath:               "voice.mp3",
					Language:               "fr",
					Model:                  "whisper-1",
					ResponseFormat:         "verbose_json",
					TimestampGranularities: []string{"segment"},
				}
				if !reflect.DeepEqual(harness.transcription.request, want) {
					t.Fatalf("transcription request = %#v, want %#v", harness.transcription.request, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()
			stdout, stderr, err := harness.execute(tt.args...)
			if err != nil {
				t.Fatalf("execute %q: %v\nstderr: %s", strings.Join(tt.args, " "), err, stderr)
			}
			assertContentPackJSON(t, stdout)
			tt.assert(t, harness)
		})
	}
}

func TestXUserLookupCommandRoutesUsernameAndRendersContentPack(t *testing.T) {
	harness := newCommandHarness()

	stdout, stderr, err := harness.execute("x", "user-lookup", "alnah")
	if err != nil {
		t.Fatalf("execute x user-lookup: %v\nstderr: %s", err, stderr)
	}
	assertContentPackJSON(t, stdout)
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !harness.x.usernameLookupCalled {
		t.Fatal("x user-lookup did not call username lookup service")
	}

	want := xclient.UsernameLookupOptions{Username: "alnah"}
	if !reflect.DeepEqual(harness.x.usernameLookup, want) {
		t.Fatalf("x user-lookup options = %#v, want %#v", harness.x.usernameLookup, want)
	}
	if harness.x.userPostsCalled {
		t.Fatal("x user-lookup called user posts service, want username profile lookup only")
	}
}

func TestXUserLookupCommandRejectsInvalidUsernameArgumentsWithStableJSON(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing username",
			args: []string{"x", "user-lookup"},
		},
		{
			name: "extra username",
			args: []string{"x", "user-lookup", "one", "two"},
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
			if !strings.Contains(document.Error.Message, "x user-lookup accepts exactly one username") {
				t.Fatalf("error.message = %q, want username argument contract", document.Error.Message)
			}
			if len(document.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want empty array", document.Warnings)
			}
		})
	}
}

func TestXUserCommandKeepsUserIDPostsRoute(t *testing.T) {
	harness := newCommandHarness()

	stdout, stderr, err := harness.execute(
		"--max-results", "8",
		"x", "user", "2244994945",
		"--next-token", "older-page",
		"--max-requests", "2",
	)
	if err != nil {
		t.Fatalf("execute x user: %v\nstderr: %s", err, stderr)
	}
	assertContentPackJSON(t, stdout)
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if !harness.x.userPostsCalled {
		t.Fatal("x user did not call user posts service")
	}
	if harness.x.usernameLookupCalled {
		t.Fatal("x user called username lookup, want user-ID posts lookup only")
	}

	want := xclient.UserPostsOptions{
		UserID:      "2244994945",
		MaxResults:  8,
		MaxRequests: 2,
		NextToken:   "older-page",
	}
	if !reflect.DeepEqual(harness.x.user, want) {
		t.Fatalf("x user options = %#v, want %#v", harness.x.user, want)
	}
}

func TestToolsDoctorUsesInjectedDependencyAndTimeout(t *testing.T) {
	harness := newCommandHarness()

	stdout, stderr, err := harness.execute("--timeout", "3s", "tools", "doctor", "--tools-dir", "/opt/moth-tools")
	if err != nil {
		t.Fatalf("execute tools doctor: %v\nstderr: %s", err, stderr)
	}
	assertJSONType(t, stdout, "tool_doctor")
	if harness.tools.options.ToolsDir != "/opt/moth-tools" {
		t.Fatalf("tools dir = %q, want /opt/moth-tools", harness.tools.options.ToolsDir)
	}
	if !harness.tools.hadDeadline {
		t.Fatal("tools doctor context had no deadline, want timeout propagated")
	}
}

func TestBrowserCommandsRouteRequestsAndRenderExpectedJSON(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantType string
		assert   func(*testing.T, *commandHarness)
	}{
		{
			name:     "open",
			args:     []string{"browser", "open", "https://example.test", "--profile", "alexis", "--session", "research"},
			wantType: "browser_page",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.OpenPageRequest{URL: "https://example.test", ProfileName: "alexis", SessionName: "research"}
				if !reflect.DeepEqual(harness.browser.open, want) {
					t.Fatalf("open request = %#v, want %#v", harness.browser.open, want)
				}
			},
		},
		{
			name:     "pages",
			args:     []string{"browser", "pages", "--profile", "alexis", "--session", "research"},
			wantType: "browser_pages",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.SessionRequest{ProfileName: "alexis", SessionName: "research"}
				if !reflect.DeepEqual(harness.browser.pages, want) {
					t.Fatalf("pages request = %#v, want %#v", harness.browser.pages, want)
				}
			},
		},
		{
			name:     "page",
			args:     []string{"browser", "page", "page-2", "--profile", "alexis", "--session", "research"},
			wantType: "browser_page",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				assertPageSelection(t, harness.browser.page, "page-2")
			},
		},
		{
			name:     "close page",
			args:     []string{"browser", "close-page", "page-2", "--profile", "alexis", "--session", "research"},
			wantType: "browser_operation",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				assertPageSelection(t, harness.browser.closePage, "page-2")
			},
		},
		{
			name: "click",
			args: []string{
				"browser", "click", "#accept",
				"--profile", "alexis",
				"--session", "research",
				"--page", "page-2",
			},
			wantType: "browser_operation",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.InteractionRequest{
					ProfileName: "alexis",
					SessionName: "research",
					PageID:      "page-2",
					Selector:    "#accept",
				}
				if !reflect.DeepEqual(harness.browser.click, want) {
					t.Fatalf("click request = %#v, want %#v", harness.browser.click, want)
				}
			},
		},
		{
			name: "input",
			args: []string{
				"browser", "input", "textarea", "hello",
				"--profile", "alexis",
				"--session", "research",
				"--page", "page-2",
			},
			wantType: "browser_operation",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.InputRequest{
					ProfileName: "alexis",
					SessionName: "research",
					PageID:      "page-2",
					Selector:    "textarea",
					Text:        "hello",
				}
				if !reflect.DeepEqual(harness.browser.input, want) {
					t.Fatalf("input request = %#v, want %#v", harness.browser.input, want)
				}
			},
		},
		{
			name: "wait",
			args: []string{
				"browser", "wait", ".ready",
				"--state", "visible",
				"--profile", "alexis",
				"--session", "research",
				"--page", "page-2",
			},
			wantType: "browser_operation",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.WaitRequest{
					ProfileName: "alexis",
					SessionName: "research",
					PageID:      "page-2",
					Selector:    ".ready",
					State:       browser.WaitVisible,
				}
				if !reflect.DeepEqual(harness.browser.wait, want) {
					t.Fatalf("wait request = %#v, want %#v", harness.browser.wait, want)
				}
			},
		},
		{
			name:     "metadata",
			args:     []string{"browser", "metadata", "https://example.test", "--max-header-bytes", "1024"},
			wantType: "browser_response_metadata",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.ResponseMetadataRequest{URL: "https://example.test", MaxHeaderBytes: 1024}
				if !reflect.DeepEqual(harness.browser.metadata, want) {
					t.Fatalf("metadata request = %#v, want %#v", harness.browser.metadata, want)
				}
			},
		},
		{
			name: "accessibility tree",
			args: []string{
				"browser", "ax-tree",
				"--max-depth", "3",
				"--profile", "alexis",
				"--session", "research",
				"--page", "page-2",
			},
			wantType: "browser_accessibility_tree",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.AccessibilityRequest{
					ProfileName: "alexis",
					SessionName: "research",
					PageID:      "page-2",
					MaxDepth:    3,
				}
				if !reflect.DeepEqual(harness.browser.ax, want) {
					t.Fatalf("ax request = %#v, want %#v", harness.browser.ax, want)
				}
			},
		},
		{
			name:     "challenge",
			args:     []string{"browser", "challenge", "--profile", "alexis", "--session", "research", "--page", "page-2"},
			wantType: "browser_challenge",
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.ManualChallengeRequest{ProfileName: "alexis", SessionName: "research", PageID: "page-2"}
				if !reflect.DeepEqual(harness.browser.challenge, want) {
					t.Fatalf("challenge request = %#v, want %#v", harness.browser.challenge, want)
				}
			},
		},
		{
			name: "screenshot",
			args: []string{
				"--max-bytes", "1000",
				"browser", "screenshot", "https://example.test", "shot.png",
				"--full-page",
			},
			wantType: content.TypeContentPack,
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.ScreenshotRequest{
					URL:      "https://example.test",
					Path:     "shot.png",
					FullPage: true,
					MaxBytes: 1000,
				}
				if !reflect.DeepEqual(harness.browser.screenshot, want) {
					t.Fatalf("screenshot request = %#v, want %#v", harness.browser.screenshot, want)
				}
			},
		},
		{
			name:     "pdf",
			args:     []string{"--max-bytes", "2000", "browser", "pdf", "https://example.test", "page.pdf"},
			wantType: content.TypeContentPack,
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.PDFRequest{URL: "https://example.test", Path: "page.pdf", MaxBytes: 2000}
				if !reflect.DeepEqual(harness.browser.pdf, want) {
					t.Fatalf("pdf request = %#v, want %#v", harness.browser.pdf, want)
				}
			},
		},
		{
			name: "download",
			args: []string{
				"browser", "download", "a.save", "file.bin",
				"--profile", "alexis",
				"--session", "research",
				"--page", "page-2",
			},
			wantType: content.TypeContentPack,
			assert: func(t *testing.T, harness *commandHarness) {
				t.Helper()
				want := browser.DownloadRequest{
					ProfileName: "alexis",
					SessionName: "research",
					PageID:      "page-2",
					Selector:    "a.save",
					Path:        "file.bin",
				}
				if !reflect.DeepEqual(harness.browser.download, want) {
					t.Fatalf("download request = %#v, want %#v", harness.browser.download, want)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			harness := newCommandHarness()
			stdout, stderr, err := harness.execute(tt.args...)
			if err != nil {
				t.Fatalf("execute %q: %v\nstderr: %s", strings.Join(tt.args, " "), err, stderr)
			}
			assertJSONType(t, stdout, tt.wantType)
			tt.assert(t, harness)
		})
	}
}

func TestResultRendererCompactsPrettyPrintsAndWritesOutput(t *testing.T) {
	harness := newCommandHarness()
	stdout, stderr, err := harness.execute("search", "web", "compact")
	if err != nil {
		t.Fatalf("execute compact command: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(stdout, "\n  \"") {
		t.Fatalf("compact JSON output = %q, want no indentation", stdout)
	}

	harness = newCommandHarness()
	stdout, stderr, err = harness.execute("--pretty", "search", "web", "pretty")
	if err != nil {
		t.Fatalf("execute pretty command: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "\n  \"type\": \"content_pack\"") {
		t.Fatalf("pretty JSON output = %q, want indented content pack", stdout)
	}
}

func TestProviderFailuresRenderSingleStableJSONError(t *testing.T) {
	harness := newCommandHarness()
	harness.webSearch.err = errors.New("provider exploded")

	stdout, stderr, err := harness.execute("search", "web", "boom")
	if err == nil {
		t.Fatal("execute failing provider error = nil, want command error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty stdout on provider error", stdout)
	}

	document := decodeSingleJSONErrorDocument(t, stderr)
	if document.Error.Code != "command_failed" {
		t.Fatalf("error.code = %q, want command_failed", document.Error.Code)
	}
	if !strings.Contains(document.Error.Message, "provider exploded") {
		t.Fatalf("error.message = %q, want provider error context", document.Error.Message)
	}
}

func defaultTestDependencies() Dependencies {
	return newCommandHarness().deps
}

type commandHarness struct {
	deps          Dependencies
	webSearch     *fakeWebSearch
	webFetch      *fakeWebFetch
	youtube       *fakeYouTube
	ytdlp         *fakeYTDLP
	podcast       *fakePodcast
	podcastAudio  *fakePodcastAudio
	x             *fakeX
	pdf           *fakePDF2Text
	transcription *fakeTranscription
	tools         *fakeToolsDoctor
	browser       *fakeBrowser
}

func newCommandHarness() *commandHarness {
	harness := &commandHarness{
		webSearch:     &fakeWebSearch{},
		webFetch:      &fakeWebFetch{},
		youtube:       &fakeYouTube{},
		ytdlp:         &fakeYTDLP{},
		podcast:       &fakePodcast{},
		podcastAudio:  &fakePodcastAudio{},
		x:             &fakeX{},
		pdf:           &fakePDF2Text{},
		transcription: &fakeTranscription{},
		tools:         &fakeToolsDoctor{},
		browser:       &fakeBrowser{},
	}
	harness.deps = Dependencies{
		WebSearch:     harness.webSearch,
		WebFetch:      harness.webFetch,
		YouTube:       harness.youtube,
		YTDLP:         harness.ytdlp,
		Podcast:       harness.podcast,
		PodcastAudio:  harness.podcastAudio,
		X:             harness.x,
		PDF2Text:      harness.pdf,
		Transcription: harness.transcription,
		Tools:         harness.tools,
		Browser:       harness.browser,
	}
	return harness
}

func (harness *commandHarness) execute(args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := NewRootCommand(harness.deps)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

type fakeWebSearch struct {
	method      string
	options     websearch.Options
	hadDeadline bool
	deadline    time.Time
	err         error
}

func (fake *fakeWebSearch) SearchWeb(ctx context.Context, options websearch.Options) (content.Pack, error) {
	fake.method = "web"
	fake.options = options
	fake.recordDeadline(ctx)
	return samplePack(content.KindPage), fake.err
}

func (fake *fakeWebSearch) SearchImages(ctx context.Context, options websearch.Options) (content.Pack, error) {
	fake.method = "images"
	fake.options = options
	fake.recordDeadline(ctx)
	return samplePack(content.KindImage), fake.err
}

func (fake *fakeWebSearch) SearchVideos(ctx context.Context, options websearch.Options) (content.Pack, error) {
	fake.method = "videos"
	fake.options = options
	fake.recordDeadline(ctx)
	return samplePack(content.KindVideo), fake.err
}

func (fake *fakeWebSearch) recordDeadline(ctx context.Context) {
	fake.deadline, fake.hadDeadline = ctx.Deadline()
}

type fakeWebFetch struct{ request webfetch.Request }

func (fake *fakeWebFetch) Fetch(_ context.Context, request webfetch.Request) (content.Pack, error) {
	fake.request = request
	return samplePack(content.KindPage), nil
}

type fakeYouTube struct {
	search  youtube.SearchOptions
	details youtube.VideoDetailsOptions
}

func (fake *fakeYouTube) SearchVideos(_ context.Context, options youtube.SearchOptions) (content.Pack, error) {
	fake.search = options
	return samplePack(content.KindVideo), nil
}

func (fake *fakeYouTube) VideoDetails(_ context.Context, options youtube.VideoDetailsOptions) (content.Pack, error) {
	fake.details = options
	return samplePack(content.KindVideo), nil
}

type fakeYTDLP struct {
	subtitles ytdlp.SubtitleRequest
	audio     ytdlp.AudioRequest
}

func (fake *fakeYTDLP) Metadata(_ context.Context, request ytdlp.MetadataRequest) (content.Item, error) {
	return content.Item{Kind: content.KindVideo, URL: request.URL, Warnings: []content.Warning{}}, nil
}

func (fake *fakeYTDLP) DownloadSubtitles(
	_ context.Context,
	request ytdlp.SubtitleRequest,
) (ytdlp.SubtitleFiles, error) {
	fake.subtitles = request
	return ytdlp.SubtitleFiles{Paths: []string{filepath.Join(request.OutputDir, "video.fr.vtt")}}, nil
}

func (fake *fakeYTDLP) ExtractAudio(_ context.Context, request ytdlp.AudioRequest) (ytdlp.AudioFile, error) {
	fake.audio = request
	return ytdlp.AudioFile{Path: filepath.Join(request.OutputDir, "video.mp3")}, nil
}

type fakePodcast struct {
	search   podcast.SearchOptions
	episodes podcast.EpisodesByFeedIDOptions
}

func (fake *fakePodcast) Search(_ context.Context, options podcast.SearchOptions) (content.Pack, error) {
	fake.search = options
	return samplePack(content.KindPodcast), nil
}

func (fake *fakePodcast) EpisodesByFeedID(
	_ context.Context,
	options podcast.EpisodesByFeedIDOptions,
) (content.Pack, error) {
	fake.episodes = options
	return samplePack(content.KindAudio), nil
}

type fakePodcastAudio struct{ request podcast.AudioDownloadOptions }

func (fake *fakePodcastAudio) DownloadEpisodeAudio(
	_ context.Context,
	options podcast.AudioDownloadOptions,
) (podcast.AudioFile, error) {
	fake.request = options
	return podcast.AudioFile{
		URL:         "https://cdn.test/episode.mp3",
		ContentType: "audio/mpeg",
		Bytes:       []byte("audio"),
	}, nil
}

type fakeX struct {
	search               xclient.SearchOptions
	post                 xclient.LookupPostOptions
	user                 xclient.UserPostsOptions
	userPostsCalled      bool
	usernameLookup       xclient.UsernameLookupOptions
	usernameLookupCalled bool
}

func (fake *fakeX) SearchRecent(_ context.Context, options xclient.SearchOptions) (content.Pack, error) {
	fake.search = options
	return samplePack(content.KindSocialPost), nil
}

func (fake *fakeX) LookupPost(_ context.Context, options xclient.LookupPostOptions) (content.Pack, error) {
	fake.post = options
	return samplePack(content.KindSocialPost), nil
}

func (fake *fakeX) UserPosts(_ context.Context, options xclient.UserPostsOptions) (content.Pack, error) {
	fake.user = options
	fake.userPostsCalled = true
	return samplePack(content.KindSocialPost), nil
}

func (fake *fakeX) LookupUserByUsername(
	_ context.Context,
	options xclient.UsernameLookupOptions,
) (content.Pack, error) {
	fake.usernameLookup = options
	fake.usernameLookupCalled = true
	return samplePack(content.KindSocialProfile), nil
}

type fakePDF2Text struct {
	input   string
	options pdf2txt.Options
}

func (fake *fakePDF2Text) Extract(_ context.Context, inputPDF string, options pdf2txt.Options) (content.Item, error) {
	fake.input = inputPDF
	fake.options = options
	return content.Item{Kind: content.KindPDF, Text: "pdf text", Warnings: []content.Warning{}}, nil
}

type fakeTranscription struct{ request transcription.Request }

func (fake *fakeTranscription) Transcribe(
	_ context.Context,
	request transcription.Request,
) (transcription.Result, error) {
	fake.request = request
	return transcription.Result{Text: "transcript"}, nil
}

type fakeToolsDoctor struct {
	options     tools.DoctorOptions
	hadDeadline bool
}

func (fake *fakeToolsDoctor) Doctor(ctx context.Context, options tools.DoctorOptions) (tools.DoctorReport, error) {
	_, fake.hadDeadline = ctx.Deadline()
	fake.options = options
	return tools.DoctorReport{
		Type: "tool_doctor",
		Tools: []tools.ToolStatus{{
			Name:     tools.ToolYTDLP,
			Status:   tools.StatusMissing,
			Warnings: []content.Warning{content.WarningToolMissing},
		}},
		Warnings: []content.Warning{},
	}, nil
}

type fakeBrowser struct {
	start          browser.StartRequest
	stop           browser.StopRequest
	status         browser.StatusRequest
	connect        browser.ConnectRequest
	open           browser.OpenPageRequest
	pages          browser.SessionRequest
	page           browser.PageSelection
	closePage      browser.PageSelection
	click          browser.InteractionRequest
	input          browser.InputRequest
	wait           browser.WaitRequest
	metadata       browser.ResponseMetadataRequest
	ax             browser.AccessibilityRequest
	challenge      browser.ManualChallengeRequest
	screenshot     browser.ScreenshotRequest
	pdf            browser.PDFRequest
	download       browser.DownloadRequest
	statusResponse browser.BrowserStatus
	err            error
}

func (fake *fakeBrowser) Start(_ context.Context, request browser.StartRequest) (browser.BrowserStatus, error) {
	fake.start = request
	return fake.browserStatus(request.Scope), fake.err
}

func (fake *fakeBrowser) Stop(_ context.Context, request browser.StopRequest) (browser.BrowserStatus, error) {
	fake.stop = request
	status := fake.browserStatus(request.Scope)
	status.Status = "stopped"
	return status, fake.err
}

func (fake *fakeBrowser) Status(_ context.Context, request browser.StatusRequest) (browser.BrowserStatus, error) {
	fake.status = request
	if fake.statusResponse.Status != "" {
		return fake.statusResponse, fake.err
	}
	return fake.browserStatus(request.Scope), fake.err
}

func (fake *fakeBrowser) Connect(_ context.Context, request browser.ConnectRequest) (browser.BrowserStatus, error) {
	fake.connect = request
	status := fake.browserStatus(request.Scope)
	status.Owned = false
	return status, fake.err
}

func (fake *fakeBrowser) OpenPage(_ context.Context, request browser.OpenPageRequest) (browser.PageInfo, error) {
	fake.open = request
	if fake.err != nil {
		return browser.PageInfo{}, fake.err
	}
	return browser.PageInfo{ID: "page-1", URL: request.URL, Active: true}, nil
}

func (fake *fakeBrowser) ListPages(_ context.Context, request browser.SessionRequest) ([]browser.PageInfo, error) {
	fake.pages = request
	if fake.err != nil {
		return nil, fake.err
	}
	return []browser.PageInfo{{ID: "page-1", Active: true}}, nil
}

func (fake *fakeBrowser) SwitchPage(_ context.Context, request browser.PageSelection) (browser.PageInfo, error) {
	fake.page = request
	if fake.err != nil {
		return browser.PageInfo{}, fake.err
	}
	return browser.PageInfo{ID: request.PageID, Active: true}, nil
}

func (fake *fakeBrowser) ClosePage(_ context.Context, request browser.PageSelection) error {
	fake.closePage = request
	return fake.err
}

func (fake *fakeBrowser) Click(_ context.Context, request browser.InteractionRequest) error {
	fake.click = request
	return fake.err
}

func (fake *fakeBrowser) Input(_ context.Context, request browser.InputRequest) error {
	fake.input = request
	return fake.err
}

func (fake *fakeBrowser) Wait(_ context.Context, request browser.WaitRequest) error {
	fake.wait = request
	return fake.err
}

func (fake *fakeBrowser) ResponseMetadata(
	_ context.Context,
	request browser.ResponseMetadataRequest,
) (browser.ResponseMetadata, error) {
	fake.metadata = request
	return browser.ResponseMetadata{URL: request.URL, Status: 200, ContentType: "text/html"}, nil
}

func (fake *fakeBrowser) AccessibilityTree(
	_ context.Context,
	request browser.AccessibilityRequest,
) (browser.AccessibilityTree, error) {
	fake.ax = request
	if fake.err != nil {
		return browser.AccessibilityTree{}, fake.err
	}
	return browser.AccessibilityTree{Nodes: []browser.AccessibilityNode{{Role: "button", Name: "Accept"}}}, nil
}

func (fake *fakeBrowser) DetectManualChallenge(
	_ context.Context,
	request browser.ManualChallengeRequest,
) (browser.ManualChallengeResult, error) {
	fake.challenge = request
	if fake.err != nil {
		return browser.ManualChallengeResult{}, fake.err
	}
	return browser.ManualChallengeResult{
		ManualRequired: true,
		Kind:           "captcha",
		Warnings:       []content.Warning{"captcha_possible"},
	}, nil
}

func (fake *fakeBrowser) Screenshot(_ context.Context, request browser.ScreenshotRequest) error {
	fake.screenshot = request
	return nil
}

func (fake *fakeBrowser) PDF(_ context.Context, request browser.PDFRequest) error {
	fake.pdf = request
	return nil
}

func (fake *fakeBrowser) Download(
	_ context.Context,
	request browser.DownloadRequest,
) (browser.CapturedDownload, error) {
	fake.download = request
	if fake.err != nil {
		return browser.CapturedDownload{}, fake.err
	}
	return browser.CapturedDownload{
		Path:        request.Path,
		Bytes:       int64(4),
		ContentType: "application/octet-stream",
	}, nil
}

func (fake *fakeBrowser) browserStatus(scope string) browser.BrowserStatus {
	if scope == "" {
		scope = "auto"
	}
	return browser.BrowserStatus{
		Status:       "running",
		Scope:        scope,
		DebugURL:     "ws://fake-browser",
		ChromePID:    4242,
		Owned:        true,
		ActivePageID: "page-1",
	}
}

func samplePack(kind content.Kind) content.Pack {
	return content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{{
			Kind:     kind,
			URL:      "https://example.test/item",
			Title:    "fake item",
			Warnings: []content.Warning{},
		}},
		Warnings: []content.Warning{},
	}
}

func assertContentPackJSON(t *testing.T, payload string) {
	t.Helper()
	assertJSONType(t, payload, content.TypeContentPack)
}

func assertJSONType(t *testing.T, payload string, want string) {
	t.Helper()
	if strings.TrimSpace(payload) == "" {
		t.Fatal("JSON payload is empty")
	}
	var document struct {
		Type string `json:"type"`
	}
	decoder := json.NewDecoder(strings.NewReader(payload))
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("decode JSON payload %q: %v", payload, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("JSON payload %q contains extra document", payload)
	}
	if document.Type != want {
		t.Fatalf("JSON type = %q, want %q; payload: %s", document.Type, want, payload)
	}
}

func assertPageSelection(t *testing.T, got browser.PageSelection, wantPageID string) {
	t.Helper()
	want := browser.PageSelection{ProfileName: "alexis", SessionName: "research", PageID: wantPageID}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("page selection = %#v, want %#v", got, want)
	}
}
