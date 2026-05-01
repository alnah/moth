package transcription

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/httpclient"
	"github.com/alnah/moth/internal/tools"
)

const openAITestAPIKey = "openai-test-key"

func TestTranscribeSendsRawMultipartRequestWithDocumentedDefaults(t *testing.T) {
	audioPath := writeTestAudioFile(t, "sample.mp3", []byte("fake mp3 audio"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := assertOpenAITranscriptionRequest(t, r, openAITranscriptionRequest{
			model:          "gpt-4o-mini-transcribe",
			language:       "fr",
			responseFormat: "json",
			fileName:       "sample.mp3",
			fileBytes:      []byte("fake mp3 audio"),
		})
		if granularities := form.Value["timestamp_granularities[]"]; len(granularities) != 0 {
			t.Fatalf("timestamp_granularities = %#v, want omitted for json default", granularities)
		}
		writeOpenAIJSON(t, w, `{"text":"bonjour le monde"}`)
	}))
	defer server.Close()

	client := newOpenAITestClient(server.URL, nil)

	result, err := client.Transcribe(context.Background(), Request{
		FilePath: audioPath,
		Language: "fr",
	})
	if err != nil {
		t.Fatalf("Transcribe error = %v, want nil", err)
	}

	assertTranscriptResult(t, result, Result{
		Text: "bonjour le monde",
		Metadata: map[string]any{
			"model":           "gpt-4o-mini-transcribe",
			"response_format": "json",
			"chunks":          1,
		},
	})
}

func TestTranscribeSendsWhisperVerboseJSONForTimestampGranularities(t *testing.T) {
	audioPath := writeTestAudioFile(t, "timed.wav", []byte("fake wav audio"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := assertOpenAITranscriptionRequest(t, r, openAITranscriptionRequest{
			model:          "whisper-1",
			language:       "en",
			responseFormat: "verbose_json",
			fileName:       "timed.wav",
			fileBytes:      []byte("fake wav audio"),
		})
		assertOpenAIFormValues(t, form, "timestamp_granularities[]", []string{"word", "segment"})
		writeOpenAIJSON(t, w, `{
			"text":"hello world",
			"segments":[{"start":1.25,"end":2.5,"text":"hello world"}]
		}`)
	}))
	defer server.Close()

	client := newOpenAITestClient(server.URL, nil)

	result, err := client.Transcribe(context.Background(), Request{
		FilePath:               audioPath,
		Language:               "en",
		Model:                  "whisper-1",
		ResponseFormat:         "verbose_json",
		TimestampGranularities: []string{"word", "segment"},
	})
	if err != nil {
		t.Fatalf("Transcribe timestamp error = %v, want nil", err)
	}

	assertTranscriptResult(t, result, Result{
		Text: "hello world",
		Segments: []Segment{
			{Start: 1250 * time.Millisecond, End: 2500 * time.Millisecond, Text: "hello world"},
		},
		Metadata: map[string]any{
			"model":           "whisper-1",
			"response_format": "verbose_json",
			"chunks":          1,
		},
	})
}

func TestTranscribeRejectsTimestampGranularitiesWithoutWhisperVerboseJSONBeforeRequest(t *testing.T) {
	audioPath := writeTestAudioFile(t, "timed.mp3", []byte("fake mp3 audio"))
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server received request, want invalid timestamp options to fail before HTTP")
	}))
	defer server.Close()

	client := newOpenAITestClient(server.URL, nil)

	_, err := client.Transcribe(context.Background(), Request{
		FilePath:               audioPath,
		TimestampGranularities: []string{"word"},
	})
	if err == nil {
		t.Fatal("Transcribe invalid timestamp options error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "whisper-1")
	assertTranscriptionErrorContains(t, err, "verbose_json")
}

func TestTranscribeFailsBeforeRequestWhenAPIKeyMissing(t *testing.T) {
	audioPath := writeTestAudioFile(t, "sample.mp3", []byte("fake mp3 audio"))
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server received request, want missing API key to fail before HTTP")
	}))
	defer server.Close()

	client := NewClient(Config{BaseURL: server.URL})

	_, err := client.Transcribe(context.Background(), Request{FilePath: audioPath})
	if err == nil {
		t.Fatal("Transcribe missing API key error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "api key")
}

func TestTranscribeReturnsProviderAndDecodeErrorsWithoutLeakingAPIKey(t *testing.T) {
	t.Run("non-2xx", func(t *testing.T) {
		audioPath := writeTestAudioFile(t, "quota.mp3", []byte("fake mp3 audio"))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertOpenAIAuthorization(t, r)
			http.Error(w, `{"error":{"message":"quota exceeded for openai-test-key"}}`, http.StatusTooManyRequests)
		}))
		defer server.Close()

		client := newOpenAITestClient(server.URL, nil)

		_, err := client.Transcribe(context.Background(), Request{FilePath: audioPath})
		if err == nil {
			t.Fatal("Transcribe provider error = nil, want error")
		}
		assertTranscriptionErrorContains(t, err, "openai")
		assertTranscriptionErrorContains(t, err, "429")
		assertTranscriptionErrorContains(t, err, "quota")
		assertTranscriptionErrorDoesNotContain(t, err, openAITestAPIKey)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		audioPath := writeTestAudioFile(t, "bad-json.mp3", []byte("fake mp3 audio"))
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertOpenAIAuthorization(t, r)
			writeOpenAIJSON(t, w, `{"text":`)
		}))
		defer server.Close()

		client := newOpenAITestClient(server.URL, nil)

		_, err := client.Transcribe(context.Background(), Request{FilePath: audioPath})
		if err == nil {
			t.Fatal("Transcribe malformed JSON error = nil, want decode error")
		}
		assertTranscriptionErrorContains(t, err, "openai")
		assertTranscriptionErrorContains(t, err, "decode")
	})
}

func TestPlanChunksUsesFFprobeDurationAndFFmpegOverlap(t *testing.T) {
	inputPath := writeTestAudioFile(t, "long.mp3", bytes.Repeat([]byte("a"), 20))
	outputDir := t.TempDir()
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "125.75", ChunkSize: 9})
	client := newOpenAITestClient("https://openai.example.test", runner)

	chunks, err := client.PlanChunks(context.Background(), ChunkPlanOptions{
		InputPath:      inputPath,
		OutputDir:      outputDir,
		MaxUploadBytes: 10,
		ChunkDuration:  time.Minute,
		Overlap:        10 * time.Second,
	})
	if err != nil {
		t.Fatalf("PlanChunks error = %v, want nil", err)
	}

	assertRecordedTool(t, runner, tools.ToolFFprobe)
	assertRecordedFFprobeArgs(t, runner.commands[0], inputPath)
	assertRecordedTool(t, runner, tools.ToolFFmpeg)
	assertFFmpegSplitCommand(t, runner.ffmpegCommand(0), inputPath, 0, time.Minute)
	assertFFmpegSplitCommand(t, runner.ffmpegCommand(1), inputPath, 50*time.Second, time.Minute)
	assertFFmpegSplitCommand(t, runner.ffmpegCommand(2), inputPath, 100*time.Second, 25750*time.Millisecond)

	assertTranscriptChunks(t, chunks, []Chunk{
		{Path: filepath.Join(outputDir, "chunk-000.mp3"), Offset: 0, Duration: time.Minute, SizeBytes: 9},
		{Path: filepath.Join(outputDir, "chunk-001.mp3"), Offset: 50 * time.Second, Duration: time.Minute, SizeBytes: 9},
		{
			Path:      filepath.Join(outputDir, "chunk-002.mp3"),
			Offset:    100 * time.Second,
			Duration:  25750 * time.Millisecond,
			SizeBytes: 9,
		},
	})
}

func TestPlanChunksRejectsChunksAtOpenAIUploadLimit(t *testing.T) {
	inputPath := writeTestAudioFile(t, "too-large.mp3", bytes.Repeat([]byte("a"), 20))
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "61", ChunkSize: 10})
	client := newOpenAITestClient("https://openai.example.test", runner)

	_, err := client.PlanChunks(context.Background(), ChunkPlanOptions{
		InputPath:      inputPath,
		OutputDir:      t.TempDir(),
		MaxUploadBytes: 10,
		ChunkDuration:  time.Minute,
		Overlap:        10 * time.Second,
	})
	if err == nil {
		t.Fatal("PlanChunks max upload boundary error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "under")
	assertTranscriptionErrorContains(t, err, "10")
}

func TestTranscribeSplitsLargeAudioUploadsChunksAndMergesOffsets(t *testing.T) {
	inputPath := writeTestAudioFile(t, "interview.mp3", bytes.Repeat([]byte("a"), 20))
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "75", ChunkSize: 9})
	uploadedFiles := make([]string, 0, 2)
	var uploadedFilesMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := assertOpenAITranscriptionRequest(t, r, openAITranscriptionRequest{
			model:          "whisper-1",
			language:       "en",
			responseFormat: "verbose_json",
		})
		fileName := uploadedMultipartFileName(t, form)
		uploadedFilesMu.Lock()
		uploadedFiles = append(uploadedFiles, fileName)
		uploadedFilesMu.Unlock()
		switch fileName {
		case "chunk-000.mp3":
			writeOpenAIJSON(t, w, `{"text":"alpha","segments":[{"start":1,"end":2,"text":"alpha"}]}`)
		case "chunk-001.mp3":
			writeOpenAIJSON(t, w, `{"text":"beta","segments":[{"start":3,"end":4,"text":"beta"}]}`)
		default:
			t.Fatalf("uploaded file = %q, want generated chunk", fileName)
		}
	}))
	defer server.Close()

	client := newOpenAITestClient(server.URL, runner)

	result, err := client.Transcribe(context.Background(), Request{
		FilePath:       inputPath,
		Language:       "en",
		Model:          "whisper-1",
		ResponseFormat: "verbose_json",
		MaxUploadBytes: 10,
		ChunkDuration:  time.Minute,
		ChunkOverlap:   10 * time.Second,
		OutputDir:      t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Transcribe chunked error = %v, want nil", err)
	}

	uploadedFilesMu.Lock()
	gotUploadedFiles := slices.Clone(uploadedFiles)
	uploadedFilesMu.Unlock()
	slices.Sort(gotUploadedFiles)
	if diff := cmp.Diff([]string{"chunk-000.mp3", "chunk-001.mp3"}, gotUploadedFiles); diff != "" {
		t.Fatalf("uploaded files mismatch (-want +got):\n%s", diff)
	}
	assertTranscriptResult(t, result, Result{
		Text: "alpha beta",
		Segments: []Segment{
			{Start: time.Second, End: 2 * time.Second, Text: "alpha"},
			{Start: 53 * time.Second, End: 54 * time.Second, Text: "beta"},
		},
		Metadata: map[string]any{
			"model":           "whisper-1",
			"response_format": "verbose_json",
			"chunks":          2,
		},
	})
}

func TestTranscribeLargeAudioUsesDefaultParallelLimit(t *testing.T) {
	inputPath := writeTestAudioFile(t, "default-parallel.mp3", bytes.Repeat([]byte("a"), 20))
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "75", ChunkSize: 9})
	server, recorder := newParallelOpenAIServer(t, parallelOpenAIOptions{
		Limit:       2,
		Chunks:      2,
		WaitForFile: "chunk-000.mp3",
	})
	defer server.Close()

	client := newOpenAITestClient(server.URL, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Transcribe(ctx, Request{
		FilePath:       inputPath,
		Language:       "en",
		Model:          "whisper-1",
		ResponseFormat: "verbose_json",
		MaxUploadBytes: 10,
		ChunkDuration:  time.Minute,
		ChunkOverlap:   10 * time.Second,
		OutputDir:      t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Transcribe default parallel chunks error = %v, want nil", err)
	}

	assertParallelUploads(t, recorder, parallelUploadWant{
		MaxInFlight: 2,
		Files:       []string{"chunk-000.mp3", "chunk-001.mp3"},
	})
	assertTranscriptResult(t, result, Result{
		Text: "alpha beta",
		Segments: []Segment{
			{Start: time.Second, End: 2 * time.Second, Text: "alpha"},
			{Start: 54 * time.Second, End: 55 * time.Second, Text: "beta"},
		},
		Metadata: map[string]any{
			"model":           "whisper-1",
			"response_format": "verbose_json",
			"chunks":          2,
		},
	})
}

func TestTranscribeLargeAudioUploadsChunksInParallelWithLimit(t *testing.T) {
	const maxParallelTranscriptions = 2

	inputPath := writeTestAudioFile(t, "parallel-interview.mp3", bytes.Repeat([]byte("a"), 20))
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "125.75", ChunkSize: 9})
	server, recorder := newParallelOpenAIServer(t, parallelOpenAIOptions{
		Limit:       maxParallelTranscriptions,
		Chunks:      3,
		WaitForFile: "chunk-000.mp3",
	})
	defer server.Close()

	client := newOpenAITestClient(server.URL, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := client.Transcribe(ctx, Request{
		FilePath:                  inputPath,
		Language:                  "en",
		Model:                     "whisper-1",
		ResponseFormat:            "verbose_json",
		MaxUploadBytes:            10,
		ChunkDuration:             time.Minute,
		ChunkOverlap:              10 * time.Second,
		MaxParallelTranscriptions: maxParallelTranscriptions,
		OutputDir:                 t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Transcribe parallel chunks error = %v, want nil", err)
	}

	assertParallelUploads(t, recorder, parallelUploadWant{
		MaxInFlight: maxParallelTranscriptions,
		Files:       []string{"chunk-000.mp3", "chunk-001.mp3", "chunk-002.mp3"},
	})
	assertTranscriptResult(t, result, Result{
		Text: "alpha beta gamma",
		Segments: []Segment{
			{Start: time.Second, End: 2 * time.Second, Text: "alpha"},
			{Start: 54 * time.Second, End: 55 * time.Second, Text: "beta"},
			{Start: 107 * time.Second, End: 108 * time.Second, Text: "gamma"},
		},
		Metadata: map[string]any{
			"model":           "whisper-1",
			"response_format": "verbose_json",
			"chunks":          3,
		},
	})
}

type parallelOpenAIOptions struct {
	Limit       int
	Chunks      int
	WaitForFile string
}

type parallelOpenAIRecorder struct {
	t                  *testing.T
	options            parallelOpenAIOptions
	mu                 sync.Mutex
	inFlight           int
	maxInFlight        int
	uploadedFiles      []string
	parallelStarted    chan struct{}
	allChunksSeen      chan struct{}
	closeParallel      sync.Once
	closeAllChunksSeen sync.Once
}

type parallelUploadWant struct {
	MaxInFlight int
	Files       []string
}

func newParallelOpenAIServer(
	t *testing.T,
	options parallelOpenAIOptions,
) (*httptest.Server, *parallelOpenAIRecorder) {
	t.Helper()

	recorder := &parallelOpenAIRecorder{
		t:               t,
		options:         options,
		uploadedFiles:   make([]string, 0, options.Chunks),
		parallelStarted: make(chan struct{}),
		allChunksSeen:   make(chan struct{}),
	}
	server := httptest.NewServer(http.HandlerFunc(recorder.handle))

	return server, recorder
}

func (recorder *parallelOpenAIRecorder) handle(w http.ResponseWriter, r *http.Request) {
	form := assertOpenAITranscriptionRequest(recorder.t, r, openAITranscriptionRequest{
		model:          "whisper-1",
		language:       "en",
		responseFormat: "verbose_json",
	})
	fileName := uploadedMultipartFileName(recorder.t, form)
	defer recorder.trackRequest(r.Context(), fileName)()

	switch fileName {
	case "chunk-000.mp3":
		writeOpenAIJSON(recorder.t, w, `{"text":"alpha","segments":[{"start":1,"end":2,"text":"alpha"}]}`)
	case "chunk-001.mp3":
		writeOpenAIJSON(recorder.t, w, `{"text":"beta","segments":[{"start":4,"end":5,"text":"beta"}]}`)
	case "chunk-002.mp3":
		writeOpenAIJSON(recorder.t, w, `{"text":"gamma","segments":[{"start":7,"end":8,"text":"gamma"}]}`)
	default:
		recorder.t.Fatalf("uploaded file = %q, want generated chunk", fileName)
	}
}

func (recorder *parallelOpenAIRecorder) trackRequest(ctx context.Context, fileName string) func() {
	recorder.mu.Lock()
	recorder.inFlight++
	if recorder.inFlight > recorder.maxInFlight {
		recorder.maxInFlight = recorder.inFlight
	}
	if recorder.inFlight == recorder.options.Limit {
		recorder.closeParallel.Do(func() { close(recorder.parallelStarted) })
	}
	recorder.uploadedFiles = append(recorder.uploadedFiles, fileName)
	if len(recorder.uploadedFiles) == recorder.options.Chunks {
		recorder.closeAllChunksSeen.Do(func() { close(recorder.allChunksSeen) })
	}
	recorder.mu.Unlock()

	waitFor(ctx, recorder.parallelStarted)
	if fileName == recorder.options.WaitForFile {
		waitFor(ctx, recorder.allChunksSeen)
	}

	return func() {
		recorder.mu.Lock()
		recorder.inFlight--
		recorder.mu.Unlock()
	}
}

func waitFor(ctx context.Context, done <-chan struct{}) {
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func assertParallelUploads(t *testing.T, recorder *parallelOpenAIRecorder, want parallelUploadWant) {
	t.Helper()

	recorder.mu.Lock()
	gotMaxInFlight := recorder.maxInFlight
	gotUploadedFiles := slices.Clone(recorder.uploadedFiles)
	recorder.mu.Unlock()

	if gotMaxInFlight <= 1 {
		t.Fatalf("max in-flight OpenAI requests = %d, want parallel chunk transcription", gotMaxInFlight)
	}
	if gotMaxInFlight > want.MaxInFlight {
		t.Fatalf("max in-flight OpenAI requests = %d, want <= configured limit %d", gotMaxInFlight, want.MaxInFlight)
	}
	slices.Sort(gotUploadedFiles)
	if diff := cmp.Diff(want.Files, gotUploadedFiles); diff != "" {
		t.Fatalf("uploaded files mismatch (-want +got):\n%s", diff)
	}
}

type openAITranscriptionRequest struct {
	model          string
	language       string
	responseFormat string
	fileName       string
	fileBytes      []byte
}

type audioRunnerOptions struct {
	Duration  string
	ChunkSize int
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
		return tools.Result{Stdout: []byte(`{"format":{"duration":"` + runner.options.Duration + `"}}`), ExitCode: 0}, nil
	case tools.ToolFFmpeg:
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
