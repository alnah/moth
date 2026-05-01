package transcription

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/httpclient"
	"github.com/alnah/moth/internal/tools"
)

type openAITranscriptionRequest struct {
	model          string
	language       string
	responseFormat string
	fileName       string
	fileBytes      []byte
}

type audioRunnerOptions struct {
	Duration      string
	ChunkSize     int
	FFprobeStdout string
	FFprobeError  error
	FFmpegError   error
}

type recordedCommand struct {
	tool tools.ToolName
	path string
	args []string
}

type recordingAudioRunner struct {
	t        *testing.T
	options  audioRunnerOptions
	commands []recordedCommand
}

func newOpenAITestClient(baseURL string, runner tools.Runner) *Client {
	return NewClient(Config{
		Settings: config.Settings{OpenAIAPIKey: openAITestAPIKey},
		BaseURL:  baseURL,
		HTTPClient: httpclient.New(httpclient.Options{
			Attempts: 1,
		}),
		FFprobePath: "/fake/ffprobe",
		FFmpegPath:  "/fake/ffmpeg",
		Runner:      runner,
	})
}

func newRecordingAudioRunner(t *testing.T, options audioRunnerOptions) *recordingAudioRunner {
	t.Helper()

	return &recordingAudioRunner{t: t, options: options}
}

func (runner *recordingAudioRunner) Run(ctx context.Context, command tools.Command) (tools.Result, error) {
	runner.t.Helper()
	if err := ctx.Err(); err != nil {
		return tools.Result{ExitCode: -1}, err
	}

	runner.commands = append(runner.commands, recordedCommand{
		tool: command.Tool,
		path: command.Path,
		args: slices.Clone(command.Args),
	})

	switch command.Tool {
	case tools.ToolFFprobe:
		if runner.options.FFprobeError != nil {
			return tools.Result{ExitCode: 1, Stderr: []byte(runner.options.FFprobeError.Error())}, runner.options.FFprobeError
		}
		stdout := runner.options.FFprobeStdout
		if stdout == "" {
			stdout = `{"format":{"duration":"` + runner.options.Duration + `"}}`
		}
		return tools.Result{Stdout: []byte(stdout), ExitCode: 0}, nil
	case tools.ToolFFmpeg:
		if runner.options.FFmpegError != nil {
			return tools.Result{ExitCode: 1, Stderr: []byte(runner.options.FFmpegError.Error())}, runner.options.FFmpegError
		}
		outputPath := command.Args[len(command.Args)-1]
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
			return tools.Result{ExitCode: -1}, err
		}
		if err := os.WriteFile(outputPath, bytes.Repeat([]byte("x"), runner.options.ChunkSize), 0o600); err != nil {
			return tools.Result{ExitCode: -1}, err
		}
		return tools.Result{ExitCode: 0}, nil
	default:
		return tools.Result{ExitCode: 64, Stderr: []byte("unexpected tool")}, tools.ErrToolFailed
	}
}

func (runner *recordingAudioRunner) ffmpegCommand(index int) recordedCommand {
	runner.t.Helper()

	seen := 0
	for _, command := range runner.commands {
		if command.tool != tools.ToolFFmpeg {
			continue
		}
		if seen == index {
			return command
		}
		seen++
	}
	runner.t.Fatalf("ffmpeg command %d not recorded; commands = %#v", index, runner.commands)
	return recordedCommand{}
}

func (runner *recordingAudioRunner) ffmpegOutputPaths() []string {
	runner.t.Helper()

	paths := make([]string, 0)
	for _, command := range runner.commands {
		if command.tool == tools.ToolFFmpeg {
			paths = append(paths, command.args[len(command.args)-1])
		}
	}
	return paths
}

func writeTestAudioFile(t *testing.T, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write test audio: %v", err)
	}
	return path
}

func assertOpenAITranscriptionRequest(t *testing.T, r *http.Request, want openAITranscriptionRequest) *multipart.Form {
	t.Helper()

	if r.Method != http.MethodPost {
		t.Fatalf("method = %s, want POST", r.Method)
	}
	if r.URL.Path != "/audio/transcriptions" {
		t.Fatalf("path = %q, want /audio/transcriptions", r.URL.Path)
	}
	assertOpenAIAuthorization(t, r)

	reader, err := r.MultipartReader()
	if err != nil {
		t.Fatalf("multipart reader: %v", err)
	}
	form, err := reader.ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("read multipart form: %v", err)
	}
	t.Cleanup(func() { _ = form.RemoveAll() })

	assertOpenAIFormValue(t, form, "model", want.model)
	assertOpenAIFormValue(t, form, "language", want.language)
	assertOpenAIFormValue(t, form, "response_format", want.responseFormat)
	if want.fileName != "" || want.fileBytes != nil {
		assertOpenAIFile(t, form, want.fileName, want.fileBytes)
	}

	return form
}

func assertOpenAIAuthorization(t *testing.T, r *http.Request) {
	t.Helper()

	if got := r.Header.Get("Authorization"); got != "Bearer "+openAITestAPIKey {
		t.Fatalf("Authorization = %q, want bearer API key", got)
	}
}

func assertOpenAIFormValue(t *testing.T, form *multipart.Form, name string, want string) {
	t.Helper()

	if want == "" {
		if got := form.Value[name]; len(got) != 0 {
			t.Fatalf("form %s = %#v, want omitted", name, got)
		}
		return
	}
	if got := form.Value[name]; len(got) != 1 || got[0] != want {
		t.Fatalf("form %s = %#v, want [%q]", name, got, want)
	}
}

func assertOpenAIFormValues(t *testing.T, form *multipart.Form, name string, want []string) {
	t.Helper()

	if diff := cmp.Diff(want, form.Value[name]); diff != "" {
		t.Fatalf("form %s mismatch (-want +got):\n%s", name, diff)
	}
}

func assertOpenAIFile(t *testing.T, form *multipart.Form, wantName string, wantBytes []byte) {
	t.Helper()

	files := form.File["file"]
	if len(files) != 1 {
		t.Fatalf("multipart files = %d, want 1", len(files))
	}
	if files[0].Filename != wantName {
		t.Fatalf("file name = %q, want %q", files[0].Filename, wantName)
	}
	file, err := files[0].Open()
	if err != nil {
		t.Fatalf("open multipart file: %v", err)
	}
	defer func() { _ = file.Close() }()
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read multipart file: %v", err)
	}
	if !bytes.Equal(got, wantBytes) {
		t.Fatalf("file bytes = %q, want %q", got, wantBytes)
	}
}

func uploadedMultipartFileName(t *testing.T, form *multipart.Form) string {
	t.Helper()

	files := form.File["file"]
	if len(files) != 1 {
		t.Fatalf("multipart files = %d, want 1", len(files))
	}
	return files[0].Filename
}

func writeOpenAIJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func assertRecordedTool(t *testing.T, runner *recordingAudioRunner, tool tools.ToolName) {
	t.Helper()

	for _, command := range runner.commands {
		if command.tool == tool {
			return
		}
	}
	t.Fatalf("commands = %#v, want tool %s", runner.commands, tool)
}

func assertRecordedFFprobeArgs(t *testing.T, command recordedCommand, inputPath string) {
	t.Helper()

	if command.tool != tools.ToolFFprobe {
		t.Fatalf("tool = %s, want ffprobe", command.tool)
	}
	assertArgsContain(t, command.args, "-v", "error", "-show_entries", "format=duration", "-of", "json", inputPath)
}

func assertFFmpegSplitCommand(
	t *testing.T,
	command recordedCommand,
	inputPath string,
	offset time.Duration,
	duration time.Duration,
) {
	t.Helper()

	if command.tool != tools.ToolFFmpeg {
		t.Fatalf("tool = %s, want ffmpeg", command.tool)
	}
	assertArgsContain(
		t,
		command.args,
		"-y",
		"-ss", ffmpegDuration(offset),
		"-i", inputPath,
		"-t", ffmpegDuration(duration),
		"-c", "copy",
	)
}

func assertArgsContain(t *testing.T, args []string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !slices.Contains(args, want) {
			t.Fatalf("args = %#v, want arg %q", args, want)
		}
	}
}

func ffmpegDuration(duration time.Duration) string {
	seconds := duration.Seconds()
	if seconds == float64(int64(seconds)) {
		return fmt.Sprintf("%.0f", seconds)
	}
	return fmt.Sprintf("%.3f", seconds)
}

func assertTranscriptChunks(t *testing.T, got []Chunk, want []Chunk) {
	t.Helper()

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("chunks mismatch (-want +got):\n%s", diff)
	}
}

func assertTranscriptResult(t *testing.T, got Result, want Result) {
	t.Helper()

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("transcript result mismatch (-want +got):\n%s", diff)
	}
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("path %q stat error = %v, want exists", path, err)
	}
}

func assertPathNotExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("path %q stat error = %v, want not exists", path, err)
	}
}

func assertTranscriptionErrorContains(t *testing.T, err error, want string) {
	t.Helper()

	if err == nil {
		t.Fatalf("error = nil, want substring %q", want)
	}
	if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
		t.Fatalf("error = %v, want substring %q", err, want)
	}
}

func assertTranscriptionErrorDoesNotContain(t *testing.T, err error, unwanted string) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want non-nil error")
	}
	if strings.Contains(err.Error(), unwanted) {
		t.Fatalf("error = %v, want no substring %q", err, unwanted)
	}
}
