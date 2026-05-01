package ytdlp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/tools"
)

const ytdlpVideoURL = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

func TestMetadataParsesDumpSingleJSONOutput(t *testing.T) {
	client, argsPath := newFakeYTDLPClient(t, "metadata")

	item, err := client.Metadata(context.Background(), MetadataRequest{URL: ytdlpVideoURL})
	if err != nil {
		t.Fatalf("Metadata error = %v, want nil", err)
	}

	assertYTDLPArgsContain(t, readFakeYTDLPArgs(t, argsPath), "-J", "--skip-download", ytdlpVideoURL)
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

func TestDownloadSubtitlesReturnsPrintedFilesInsideOutputDir(t *testing.T) {
	client, argsPath := newFakeYTDLPClient(t, "subtitles")
	outputDir := t.TempDir()

	files, err := client.DownloadSubtitles(context.Background(), SubtitleRequest{
		URL:              ytdlpVideoURL,
		OutputDir:        outputDir,
		Languages:        []string{"en", "fr"},
		Format:           "vtt",
		IncludeAutomatic: true,
	})
	if err != nil {
		t.Fatalf("DownloadSubtitles error = %v, want nil", err)
	}

	args := readFakeYTDLPArgs(t, argsPath)
	assertYTDLPArgsContain(t, args, "--skip-download", "--write-subs", "--write-auto-subs", ytdlpVideoURL)
	assertYTDLPFlagValue(t, args, "--sub-langs", "en,fr")
	assertYTDLPFlagValue(t, args, "--sub-format", "vtt")
	assertYTDLPFlagValue(t, args, "--paths", outputDir)
	assertYTDLPFlagValue(t, args, "--output", "subtitle:%(id)s.%(language)s.%(ext)s")

	wantPaths := []string{
		filepath.Join(outputDir, "dQw4w9WgXcQ.en.vtt"),
		filepath.Join(outputDir, "dQw4w9WgXcQ.fr.vtt"),
	}
	assertStringSlice(t, files.Paths, wantPaths)
	for _, path := range wantPaths {
		assertFileExists(t, path)
	}
}

func TestDownloadSubtitlesReturnsMissingSubtitleError(t *testing.T) {
	client, _ := newFakeYTDLPClient(t, "missing-subtitles")

	_, err := client.DownloadSubtitles(context.Background(), SubtitleRequest{
		URL:       ytdlpVideoURL,
		OutputDir: t.TempDir(),
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
	client, argsPath := newFakeYTDLPClient(t, "audio")
	outputDir := t.TempDir()

	file, err := client.ExtractAudio(context.Background(), AudioRequest{
		URL:       ytdlpVideoURL,
		OutputDir: outputDir,
		Format:    "mp3",
		Section: TimeRange{
			Start: 10 * time.Second,
			End:   70 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("ExtractAudio error = %v, want nil", err)
	}

	args := readFakeYTDLPArgs(t, argsPath)
	assertYTDLPArgsContain(t, args, "--extract-audio", "--print", "after_move:filepath", ytdlpVideoURL)
	assertYTDLPFlagValue(t, args, "--audio-format", "mp3")
	assertYTDLPFlagValue(t, args, "--download-sections", "*00:00:10-00:01:10")
	assertYTDLPFlagValue(t, args, "--paths", outputDir)
	assertYTDLPFlagValue(t, args, "--output", "%(id)s.%(ext)s")

	wantPath := filepath.Join(outputDir, "dQw4w9WgXcQ.mp3")
	if file.Path != wantPath {
		t.Fatalf("ExtractAudio path = %q, want printed final path %q", file.Path, wantPath)
	}
	assertFileExists(t, wantPath)
}

func TestMetadataReturnsToolMissing(t *testing.T) {
	client := New(Config{})

	_, err := client.Metadata(context.Background(), MetadataRequest{URL: ytdlpVideoURL})
	if err == nil {
		t.Fatal("Metadata missing tool error = nil, want error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("Metadata error = %v, want tool_missing", err)
	}
}

func TestMetadataRejectsBadURLBeforeRunningTool(t *testing.T) {
	client, argsPath := newFakeYTDLPClient(t, "metadata")

	_, err := client.Metadata(context.Background(), MetadataRequest{URL: "javascript:alert(1)"})
	if err == nil {
		t.Fatal("Metadata bad URL error = nil, want error")
	}
	assertYTDLPErrorContains(t, err, "invalid url")
	assertFakeYTDLPNotRun(t, argsPath)
}

func TestExtractAudioRejectsInvalidDurationRangeBeforeRunningTool(t *testing.T) {
	client, argsPath := newFakeYTDLPClient(t, "audio")

	_, err := client.ExtractAudio(context.Background(), AudioRequest{
		URL:       ytdlpVideoURL,
		OutputDir: t.TempDir(),
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
	assertFakeYTDLPNotRun(t, argsPath)
}

func TestMetadataReturnsDecodeErrorForMalformedJSON(t *testing.T) {
	client, _ := newFakeYTDLPClient(t, "bad-metadata-json")

	_, err := client.Metadata(context.Background(), MetadataRequest{URL: ytdlpVideoURL})
	if err == nil {
		t.Fatal("Metadata malformed JSON error = nil, want decode error")
	}
	assertYTDLPErrorContains(t, err, "metadata")
	assertYTDLPErrorContains(t, err, "decode")
}

func newFakeYTDLPClient(t *testing.T, scenario string) (*Client, string) {
	t.Helper()

	argsPath := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv("MOTH_FAKE_YTDLP_ARGS_FILE", argsPath)
	t.Setenv("MOTH_FAKE_YTDLP_SCENARIO", scenario)

	return New(Config{ToolPath: buildFakeYTDLP(t)}), argsPath
}

func buildFakeYTDLP(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "main.go")
	binaryPath := filepath.Join(dir, executableName("yt-dlp"))

	const source = `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const metadataJSON = ` + "`" + `{
	"id": "dQw4w9WgXcQ",
	"title": "Never Gonna Give You Up",
	"description": "Official music video.",
	"duration": 213,
	"webpage_url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
	"uploader": "Rick Astley",
	"upload_date": "20091025",
	"subtitles": {"en": [{"ext": "vtt", "url": "https://subs.example/en.vtt"}]},
	"automatic_captions": {"fr": [{"ext": "vtt", "url": "https://subs.example/fr.vtt"}]}
}` + "`" + `

func main() {
	args := os.Args[1:]
	if argsPath := os.Getenv("MOTH_FAKE_YTDLP_ARGS_FILE"); argsPath != "" {
		_ = os.WriteFile(argsPath, []byte(strings.Join(args, "\n")+"\n"), 0o600)
	}

	scenario := os.Getenv("MOTH_FAKE_YTDLP_SCENARIO")
	switch {
	case scenario == "bad-metadata-json":
		fmt.Print(` + "`" + `{"id":` + "`" + `)
	case scenario == "missing-subtitles":
		fmt.Fprintln(os.Stderr, "There are no subtitles for the requested languages")
		os.Exit(1)
	case hasArg(args, "-J"):
		fmt.Print(metadataJSON)
	case hasArg(args, "--write-subs"):
		writeSubtitleOutput(args)
	case hasArg(args, "--extract-audio"):
		writeAudioOutput(args)
	default:
		fmt.Fprintln(os.Stderr, "unsupported fake yt-dlp args:", strings.Join(args, " "))
		os.Exit(64)
	}
}

func writeSubtitleOutput(args []string) {
	outputDir := flagValue(args, "--paths")
	if outputDir == "" {
		outputDir = "."
	}
	_ = os.MkdirAll(outputDir, 0o755)
	paths := []string{
		filepath.Join(outputDir, "dQw4w9WgXcQ.en.vtt"),
		filepath.Join(outputDir, "dQw4w9WgXcQ.fr.vtt"),
	}
	for _, path := range paths {
		_ = os.WriteFile(path, []byte("WEBVTT\n"), 0o600)
		fmt.Println(path)
	}
	fmt.Println(filepath.Join(filepath.Dir(outputDir), "outside.vtt"))
}

func writeAudioOutput(args []string) {
	outputDir := flagValue(args, "--paths")
	if outputDir == "" {
		outputDir = "."
	}
	_ = os.MkdirAll(outputDir, 0o755)
	path := filepath.Join(outputDir, "dQw4w9WgXcQ.mp3")
	_ = os.WriteFile(path, []byte("fake mp3\n"), 0o600)
	fmt.Println(filepath.Join(filepath.Dir(outputDir), "outside.mp3"))
	fmt.Println(path)
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func flagValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
`

	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		t.Fatalf("write fake yt-dlp source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	//nolint:gosec // Test builds a controlled fake yt-dlp executable.
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fake yt-dlp: %v\n%s", err, output)
	}

	return binaryPath
}

func executableName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}

	return name
}

func readFakeYTDLPArgs(t *testing.T, path string) []string {
	t.Helper()

	//nolint:gosec // Test reads controlled fake yt-dlp fixture output.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fake yt-dlp args: %v", err)
	}

	return strings.Fields(string(contents))
}

func assertYTDLPArgsContain(t *testing.T, args []string, wantArgs ...string) {
	t.Helper()

	for _, want := range wantArgs {
		if !slices.Contains(args, want) {
			t.Errorf("yt-dlp args = %#v, want arg %q", args, want)
		}
	}
}

func assertYTDLPFlagValue(t *testing.T, args []string, flag string, want string) {
	t.Helper()

	for index, arg := range args {
		if arg == flag && index+1 < len(args) {
			if got := args[index+1]; got != want {
				t.Fatalf("yt-dlp %s = %q, want %q in args %#v", flag, got, want, args)
			}
			return
		}
	}

	t.Fatalf("yt-dlp args = %#v, want flag %s=%q", args, flag, want)
}

func assertFakeYTDLPNotRun(t *testing.T, argsPath string) {
	t.Helper()

	//nolint:gosec // Test reads controlled fake yt-dlp fixture output.
	if contents, err := os.ReadFile(argsPath); err == nil {
		t.Fatalf("fake yt-dlp args = %q, want no run", string(contents))
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read fake yt-dlp args: %v", err)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file %q stat error = %v, want existing file", path, err)
	}
}

func assertYTDLPContentItem(t *testing.T, got content.Item, want content.Item) {
	t.Helper()

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("content item mismatch (-want +got):\n%s", diff)
	}
}

func assertStringSlice(t *testing.T, got []string, want []string) {
	t.Helper()

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("strings mismatch (-want +got):\n%s", diff)
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
