package ytdlp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

const (
	ytdlpTestPath = "/test/bin/yt-dlp"
	ytdlpVideoURL = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
)

func TestMetadataParsesDumpSingleJSONOutput(t *testing.T) {
	runner := &fakeYTDLPRunner{
		results: []tools.Result{
			{
				Stdout: []byte(`{
					"id": "dQw4w9WgXcQ",
					"title": "Never Gonna Give You Up",
					"description": "Official music video.",
					"duration": 213,
					"webpage_url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
					"uploader": "Rick Astley",
					"upload_date": "20091025",
					"subtitles": {"en": [{"ext": "vtt", "url": "https://subs.example/en.vtt"}]},
					"automatic_captions": {"fr": [{"ext": "vtt", "url": "https://subs.example/fr.vtt"}]}
				}`),
				ExitCode: 0,
			},
		},
	}
	client := New(Config{ToolPath: ytdlpTestPath, Runner: runner})

	item, err := client.Metadata(context.Background(), MetadataRequest{URL: ytdlpVideoURL})
	if err != nil {
		t.Fatalf("Metadata error = %v, want nil", err)
	}

	assertYTDLPCommand(t, runner.commands[0], []string{"-J", "--skip-download", ytdlpVideoURL})
	assertYTDLPContentItem(t, item, content.Item{
		Kind:  content.KindVideo,
		URL:   "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Title: "Never Gonna Give You Up",
		Text:  "Official music video.",
		Metadata: map[string]any{
			"video_id":           "dQw4w9WgXcQ",
			"duration_seconds":   213,
			"uploader":           "Rick Astley",
			"upload_date":        "20091025",
			"subtitles":          []string{"en"},
			"automatic_captions": []string{"fr"},
		},
	})
}

func TestDownloadSubtitlesConstructsStableCommand(t *testing.T) {
	runner := &fakeYTDLPRunner{
		results: []tools.Result{
			{
				Stdout:   []byte("/work/subs/dQw4w9WgXcQ.en.vtt\n/work/subs/dQw4w9WgXcQ.fr.vtt\n"),
				ExitCode: 0,
			},
		},
	}
	client := New(Config{ToolPath: ytdlpTestPath, Runner: runner})

	files, err := client.DownloadSubtitles(context.Background(), SubtitleRequest{
		URL:              ytdlpVideoURL,
		OutputDir:        "/work/subs",
		Languages:        []string{"en", "fr"},
		Format:           "vtt",
		IncludeAutomatic: true,
	})
	if err != nil {
		t.Fatalf("DownloadSubtitles error = %v, want nil", err)
	}

	assertYTDLPCommand(t, runner.commands[0], []string{
		"--skip-download",
		"--write-subs",
		"--write-auto-subs",
		"--sub-langs", "en,fr",
		"--sub-format", "vtt",
		"--paths", "/work/subs",
		"--output", "subtitle:%(id)s.%(language)s.%(ext)s",
		ytdlpVideoURL,
	})
	assertStringSlice(t, files.Paths, []string{"/work/subs/dQw4w9WgXcQ.en.vtt", "/work/subs/dQw4w9WgXcQ.fr.vtt"})
}

func TestDownloadSubtitlesReturnsMissingSubtitleError(t *testing.T) {
	runner := &fakeYTDLPRunner{
		results: []tools.Result{
			{
				Stderr:   []byte("There are no subtitles for the requested languages"),
				ExitCode: 1,
			},
		},
		errors: []error{tools.ErrToolFailed},
	}
	client := New(Config{ToolPath: ytdlpTestPath, Runner: runner})

	_, err := client.DownloadSubtitles(context.Background(), SubtitleRequest{
		URL:       ytdlpVideoURL,
		OutputDir: "/work/subs",
		Languages: []string{"de"},
		Format:    "vtt",
	})
	if err == nil {
		t.Fatal("DownloadSubtitles missing subtitles error = nil, want error")
	}
	if !errors.Is(err, ErrSubtitlesMissing) {
		t.Fatalf("DownloadSubtitles error = %v, want ErrSubtitlesMissing", err)
	}
}

func TestExtractAudioReturnsPrintedFinalPath(t *testing.T) {
	runner := &fakeYTDLPRunner{
		results: []tools.Result{
			{Stdout: []byte("/work/audio/dQw4w9WgXcQ.mp3\n"), ExitCode: 0},
		},
	}
	client := New(Config{ToolPath: ytdlpTestPath, Runner: runner})

	file, err := client.ExtractAudio(context.Background(), AudioRequest{
		URL:       ytdlpVideoURL,
		OutputDir: "/work/audio",
		Format:    "mp3",
		Section: TimeRange{
			Start: 10 * time.Second,
			End:   70 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("ExtractAudio error = %v, want nil", err)
	}

	assertYTDLPCommand(t, runner.commands[0], []string{
		"--extract-audio",
		"--audio-format", "mp3",
		"--download-sections", "*00:00:10-00:01:10",
		"--paths", "/work/audio",
		"--output", "%(id)s.%(ext)s",
		"--print", "after_move:filepath",
		ytdlpVideoURL,
	})
	if file.Path != "/work/audio/dQw4w9WgXcQ.mp3" {
		t.Fatalf("ExtractAudio path = %q, want printed final path", file.Path)
	}
}

func TestMetadataReturnsToolMissing(t *testing.T) {
	runner := &fakeYTDLPRunner{}
	client := New(Config{Runner: runner})

	_, err := client.Metadata(context.Background(), MetadataRequest{URL: ytdlpVideoURL})
	if err == nil {
		t.Fatal("Metadata missing tool error = nil, want error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("Metadata error = %v, want tool_missing", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %#v, want no command when tool is missing", runner.commands)
	}
}

func TestMetadataRejectsBadURLBeforeRunningTool(t *testing.T) {
	runner := &fakeYTDLPRunner{}
	client := New(Config{ToolPath: ytdlpTestPath, Runner: runner})

	_, err := client.Metadata(context.Background(), MetadataRequest{URL: "javascript:alert(1)"})
	if err == nil {
		t.Fatal("Metadata bad URL error = nil, want error")
	}
	assertYTDLPErrorContains(t, err, "invalid url")
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %#v, want no command for invalid URL", runner.commands)
	}
}

func TestExtractAudioRejectsInvalidDurationRangeBeforeRunningTool(t *testing.T) {
	runner := &fakeYTDLPRunner{}
	client := New(Config{ToolPath: ytdlpTestPath, Runner: runner})

	_, err := client.ExtractAudio(context.Background(), AudioRequest{
		URL:       ytdlpVideoURL,
		OutputDir: "/work/audio",
		Format:    "mp3",
		Section: TimeRange{
			Start: 90 * time.Second,
			End:   10 * time.Second,
		},
	})
	if err == nil {
		t.Fatal("ExtractAudio invalid duration range error = nil, want error")
	}
	assertYTDLPErrorContains(t, err, "duration")
	if len(runner.commands) != 0 {
		t.Fatalf("runner commands = %#v, want no command for invalid duration range", runner.commands)
	}
}

func TestMetadataReturnsDecodeErrorForMalformedJSON(t *testing.T) {
	runner := &fakeYTDLPRunner{
		results: []tools.Result{{Stdout: []byte(`{"id":`), ExitCode: 0}},
	}
	client := New(Config{ToolPath: ytdlpTestPath, Runner: runner})

	_, err := client.Metadata(context.Background(), MetadataRequest{URL: ytdlpVideoURL})
	if err == nil {
		t.Fatal("Metadata malformed JSON error = nil, want decode error")
	}
	assertYTDLPErrorContains(t, err, "metadata")
	assertYTDLPErrorContains(t, err, "decode")
}

type fakeYTDLPRunner struct {
	commands []tools.Command
	results  []tools.Result
	errors   []error
}

func (runner *fakeYTDLPRunner) Run(ctx context.Context, command tools.Command) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{ExitCode: -1}, err
	}

	runner.commands = append(runner.commands, command)
	index := len(runner.commands) - 1
	if index >= len(runner.results) {
		return tools.Result{ExitCode: 0}, nil
	}
	var err error
	if index < len(runner.errors) {
		err = runner.errors[index]
	}

	return runner.results[index], err
}

func assertYTDLPCommand(t *testing.T, command tools.Command, wantArgs []string) {
	t.Helper()

	if command.Tool != tools.ToolYTDLP {
		t.Fatalf("command tool = %q, want yt-dlp", command.Tool)
	}
	if command.Path != ytdlpTestPath {
		t.Fatalf("command path = %q, want %q", command.Path, ytdlpTestPath)
	}
	if !reflect.DeepEqual(command.Args, wantArgs) {
		t.Fatalf("command args = %#v, want %#v", command.Args, wantArgs)
	}
}

func assertYTDLPContentItem(t *testing.T, got content.Item, want content.Item) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("content item = %#v, want %#v", got, want)
	}
}

func assertStringSlice(t *testing.T, got []string, want []string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
}

func assertYTDLPErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}
