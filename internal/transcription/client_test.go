package transcription

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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

func TestMultipartBodyReportsMissingFile(t *testing.T) {
	_, _, err := multipartTranscriptionBody(normalizedRequest{
		Model:          defaultTranscriptionModel,
		ResponseFormat: defaultTranscriptionFormat,
	}, filepath.Join(t.TempDir(), "missing.mp3"))
	if err == nil {
		t.Fatal("multipart body missing file error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "open audio file")
}

func TestMultipartBodyReportsCopyError(t *testing.T) {
	_, _, err := multipartTranscriptionBody(normalizedRequest{
		Model:          defaultTranscriptionModel,
		ResponseFormat: defaultTranscriptionFormat,
	}, t.TempDir())
	if err == nil {
		t.Fatal("multipart body copy error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "copy audio file")
}

func TestTranscribeFileRejectsUploadAtLimitBeforeMultipart(t *testing.T) {
	audioPath := writeTestAudioFile(t, "at-limit.mp3", bytes.Repeat([]byte("a"), 10))
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("server received request, want upload size guard to fail before HTTP")
	}))
	defer server.Close()

	client := newOpenAITestClient(server.URL, nil)

	_, err := client.transcribeFile(context.Background(), openAITestAPIKey, normalizedRequest{
		FilePath:       audioPath,
		Model:          defaultTranscriptionModel,
		ResponseFormat: defaultTranscriptionFormat,
		MaxUploadBytes: 10,
	}, audioPath)
	if err == nil {
		t.Fatal("transcribeFile at upload limit error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "under 10")
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
	outputDir := t.TempDir()

	result, err := client.Transcribe(context.Background(), Request{
		FilePath:       inputPath,
		Language:       "en",
		Model:          "whisper-1",
		ResponseFormat: "verbose_json",
		MaxUploadBytes: 10,
		ChunkDuration:  time.Minute,
		ChunkOverlap:   10 * time.Second,
		OutputDir:      outputDir,
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
	assertPathExists(t, filepath.Join(outputDir, "chunk-000.mp3"))
	assertPathExists(t, filepath.Join(outputDir, "chunk-001.mp3"))
}

func TestTranscribeLargeAudioCleansTemporaryChunkDirectory(t *testing.T) {
	inputPath := writeTestAudioFile(t, "temporary-chunks.mp3", bytes.Repeat([]byte("a"), 20))
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "75", ChunkSize: 9})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := assertOpenAITranscriptionRequest(t, r, openAITranscriptionRequest{
			model:          "whisper-1",
			language:       "en",
			responseFormat: "verbose_json",
		})
		switch uploadedMultipartFileName(t, form) {
		case "chunk-000.mp3":
			writeOpenAIJSON(t, w, `{"text":"alpha"}`)
		case "chunk-001.mp3":
			writeOpenAIJSON(t, w, `{"text":"beta"}`)
		default:
			t.Fatalf("uploaded file = %q, want generated chunk", uploadedMultipartFileName(t, form))
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
	})
	if err != nil {
		t.Fatalf("Transcribe with temporary chunks error = %v, want nil", err)
	}
	assertTranscriptResult(t, result, Result{
		Text:     "alpha beta",
		Segments: []Segment{},
		Metadata: map[string]any{
			"model":           "whisper-1",
			"response_format": "verbose_json",
			"chunks":          2,
		},
	})
	for _, path := range runner.ffmpegOutputPaths() {
		assertPathNotExists(t, filepath.Dir(path))
	}
}

func TestTranscribeLargeAudioCleansTemporaryChunkDirectoryAfterPlanningError(t *testing.T) {
	inputPath := writeTestAudioFile(t, "temporary-planning-error.mp3", bytes.Repeat([]byte("a"), 20))
	runner := newRecordingAudioRunner(t, audioRunnerOptions{
		Duration:    "75",
		ChunkSize:   9,
		FFmpegError: fmt.Errorf("split failed"),
	})
	client := newOpenAITestClient("https://openai.example.test", runner)

	_, err := client.Transcribe(context.Background(), Request{
		FilePath:       inputPath,
		MaxUploadBytes: 10,
		ChunkDuration:  time.Minute,
		ChunkOverlap:   10 * time.Second,
	})
	if err == nil {
		t.Fatal("Transcribe planning error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "ffmpeg split")
	for _, path := range runner.ffmpegOutputPaths() {
		assertPathNotExists(t, filepath.Dir(path))
	}
}

func TestTranscribeReturnsFileAndRequestSetupErrors(t *testing.T) {
	t.Run("audio stat", func(t *testing.T) {
		client := newOpenAITestClient("https://openai.example.test", nil)

		_, err := client.Transcribe(context.Background(), Request{FilePath: filepath.Join(t.TempDir(), "missing.mp3")})
		if err == nil {
			t.Fatal("Transcribe missing audio error = nil, want error")
		}
		assertTranscriptionErrorContains(t, err, "stat audio file")
	})

	t.Run("build request", func(t *testing.T) {
		audioPath := writeTestAudioFile(t, "build-request.mp3", []byte("audio"))
		client := newOpenAITestClient("://bad-url", nil)

		_, err := client.transcribeFile(context.Background(), openAITestAPIKey, normalizedRequest{
			Model:          defaultTranscriptionModel,
			ResponseFormat: defaultTranscriptionFormat,
			MaxUploadBytes: 10,
		}, audioPath)
		if err == nil {
			t.Fatal("transcribeFile build request error = nil, want error")
		}
		assertTranscriptionErrorContains(t, err, "build request")
	})
}

func TestTranscribeLargeAudioReturnsChunkUploadError(t *testing.T) {
	inputPath := writeTestAudioFile(t, "chunk-error.mp3", bytes.Repeat([]byte("a"), 20))
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "75", ChunkSize: 9})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := assertOpenAITranscriptionRequest(t, r, openAITranscriptionRequest{
			model:          "whisper-1",
			language:       "en",
			responseFormat: "verbose_json",
		})
		switch uploadedMultipartFileName(t, form) {
		case "chunk-000.mp3":
			http.Error(w, "chunk failed", http.StatusBadGateway)
		case "chunk-001.mp3":
			writeOpenAIJSON(t, w, `{"text":"beta"}`)
		default:
			t.Fatalf("uploaded file = %q, want generated chunk", uploadedMultipartFileName(t, form))
		}
	}))
	defer server.Close()

	client := newOpenAITestClient(server.URL, runner)

	_, err := client.Transcribe(context.Background(), Request{
		FilePath:                  inputPath,
		Language:                  "en",
		Model:                     "whisper-1",
		ResponseFormat:            "verbose_json",
		MaxUploadBytes:            10,
		ChunkDuration:             time.Minute,
		ChunkOverlap:              10 * time.Second,
		MaxParallelTranscriptions: 2,
	})
	if err == nil {
		t.Fatal("Transcribe chunk upload error = nil, want error")
	}
	assertTranscriptionErrorContains(t, err, "502")
}
