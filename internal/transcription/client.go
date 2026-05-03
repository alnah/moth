// Package transcription transcribes audio with raw OpenAI HTTP requests and ffmpeg chunking.
package transcription

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/httpclient"
	"github.com/alnah/moth/internal/tools"
)

// Config contains OpenAI transcription client dependencies and credentials.
type Config struct {
	Credentials config.Credentials
	BaseURL     string
	HTTPClient  *httpclient.Client
	FFprobePath string
	FFmpegPath  string
	Runner      tools.Runner
}

// Request describes one audio transcription request.
type Request struct {
	FilePath               string
	Language               string
	Model                  string
	ResponseFormat         string
	TimestampGranularities []string
	MaxUploadBytes         int64
	ChunkDuration          time.Duration
	ChunkOverlap           time.Duration

	// MaxParallelTranscriptions bounds concurrent chunk uploads.
	MaxParallelTranscriptions int

	// OutputDir stores generated chunks. If empty, Transcribe creates and removes a temporary directory.
	OutputDir string
}

// Result contains normalized transcription text and optional timed segments.
type Result struct {
	Text     string         `json:"text"`
	Segments []Segment      `json:"segments,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Segment is one timed transcript fragment.
type Segment struct {
	Start time.Duration `json:"start"`
	End   time.Duration `json:"end"`
	Text  string        `json:"text"`
}

// Chunk describes one ffmpeg-generated audio chunk.
type Chunk struct {
	Path      string
	Offset    time.Duration
	Duration  time.Duration
	SizeBytes int64
}

// ChunkPlanOptions describes ffprobe/ffmpeg split parameters.
type ChunkPlanOptions struct {
	InputPath string

	// OutputDir receives generated chunks and remains caller-owned.
	OutputDir string

	MaxUploadBytes int64
	ChunkDuration  time.Duration
	Overlap        time.Duration
}

// Client sends raw HTTP requests to the OpenAI transcription endpoint.
type Client struct {
	credentials config.Credentials
	baseURL     string
	httpClient  *httpclient.Client
	ffprobePath string
	ffmpegPath  string
	runner      tools.Runner
}

// NewClient creates a transcription client with defaults for unset dependencies.
func NewClient(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = httpclient.New(httpclient.Options{})
	}
	runner := cfg.Runner
	if runner == nil {
		runner = tools.LocalRunner{}
	}
	ffprobePath := cfg.FFprobePath
	if ffprobePath == "" {
		ffprobePath = string(tools.ToolFFprobe)
	}
	ffmpegPath := cfg.FFmpegPath
	if ffmpegPath == "" {
		ffmpegPath = string(tools.ToolFFmpeg)
	}

	return &Client{
		credentials: cfg.Credentials,
		baseURL:     baseURL,
		httpClient:  httpClient,
		ffprobePath: ffprobePath,
		ffmpegPath:  ffmpegPath,
		runner:      runner,
	}
}

// Transcribe uploads one file or bounded chunks and returns merged transcript output.
func (client *Client) Transcribe(ctx context.Context, request Request) (Result, error) {
	apiKey := strings.TrimSpace(client.credentials.OpenAIAPIKey)
	if apiKey == "" {
		return Result{}, fmt.Errorf("openai transcription: api key is required")
	}

	options, err := normalizeRequest(request)
	if err != nil {
		return Result{}, err
	}

	fileInfo, err := os.Stat(options.FilePath)
	if err != nil {
		return Result{}, fmt.Errorf("openai transcription: stat audio file: %w", err)
	}
	if fileInfo.Size() < options.MaxUploadBytes {
		return client.transcribeSingleFile(ctx, apiKey, options)
	}

	chunks, cleanupChunks, err := client.planChunksForRequest(ctx, options)
	if err != nil {
		return Result{}, err
	}
	if cleanupChunks != nil {
		defer cleanupChunks()
	}
	results, err := client.transcribeChunks(ctx, apiKey, options, chunks)
	if err != nil {
		return Result{}, err
	}

	return mergeChunkResults(options, chunks, results), nil
}
