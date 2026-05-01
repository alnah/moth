package transcription

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

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
