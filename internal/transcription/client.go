// Package transcription transcribes audio with raw OpenAI HTTP requests and ffmpeg chunking.
package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/httpclient"
	"github.com/alnah/moth/internal/tools"
)

const (
	defaultOpenAIBaseURL            = "https://api.openai.com/v1"
	defaultTranscriptionModel       = "gpt-4o-mini-transcribe"
	defaultTranscriptionFormat      = "json"
	defaultChunkDuration            = 10 * time.Minute
	defaultChunkOverlap             = 2 * time.Second
	defaultMaxUploadBytes           = 25 * 1024 * 1024
	defaultMaxParallelTranscription = 2
	openAIResponseBodyMax           = 4096
	toolOutputLimit                 = 1 << 20
)

// Config contains OpenAI transcription client dependencies and credentials.
type Config struct {
	Settings    config.Settings
	BaseURL     string
	HTTPClient  *httpclient.Client
	FFprobePath string
	FFmpegPath  string
	Runner      tools.Runner
}

// Request describes one audio transcription request.
type Request struct {
	FilePath                  string
	Language                  string
	Model                     string
	ResponseFormat            string
	TimestampGranularities    []string
	MaxUploadBytes            int64
	ChunkDuration             time.Duration
	ChunkOverlap              time.Duration
	MaxParallelTranscriptions int
	OutputDir                 string
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
	InputPath      string
	OutputDir      string
	MaxUploadBytes int64
	ChunkDuration  time.Duration
	Overlap        time.Duration
}

// Client sends raw HTTP requests to the OpenAI transcription endpoint.
type Client struct {
	settings    config.Settings
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
		settings:    cfg.Settings,
		baseURL:     baseURL,
		httpClient:  httpClient,
		ffprobePath: ffprobePath,
		ffmpegPath:  ffmpegPath,
		runner:      runner,
	}
}

// Transcribe uploads one file or bounded chunks and returns merged transcript output.
func (client *Client) Transcribe(ctx context.Context, request Request) (Result, error) {
	apiKey := strings.TrimSpace(client.settings.OpenAIAPIKey)
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

	chunks, err := client.planChunksForRequest(ctx, options)
	if err != nil {
		return Result{}, err
	}
	results, err := client.transcribeChunks(ctx, apiKey, options, chunks)
	if err != nil {
		return Result{}, err
	}

	return mergeChunkResults(options, chunks, results), nil
}

// PlanChunks inspects an audio file and splits it into upload-safe chunks.
func (client *Client) PlanChunks(ctx context.Context, options ChunkPlanOptions) ([]Chunk, error) {
	plan, err := normalizeChunkPlan(options)
	if err != nil {
		return nil, err
	}
	duration, err := client.probeDuration(ctx, plan.InputPath)
	if err != nil {
		return nil, err
	}

	chunks := make([]Chunk, 0, estimatedChunkCount(duration, plan.ChunkDuration, plan.Overlap))
	step := plan.ChunkDuration - plan.Overlap
	for offset, index := time.Duration(0), 0; offset < duration; offset, index = offset+step, index+1 {
		chunkDuration := minDuration(plan.ChunkDuration, duration-offset)
		chunkPath := filepath.Join(
			plan.OutputDir,
			fmt.Sprintf("chunk-%03d%s", index, filepath.Ext(plan.InputPath)),
		)
		if err := client.splitChunk(ctx, plan.InputPath, chunkPath, offset, chunkDuration); err != nil {
			return nil, err
		}
		info, err := os.Stat(chunkPath)
		if err != nil {
			return nil, fmt.Errorf("openai transcription: stat chunk: %w", err)
		}
		if info.Size() >= plan.MaxUploadBytes {
			return nil, fmt.Errorf(
				"openai transcription: chunk %q must be under %d bytes",
				chunkPath,
				plan.MaxUploadBytes,
			)
		}
		chunks = append(chunks, Chunk{
			Path:      chunkPath,
			Offset:    offset,
			Duration:  chunkDuration,
			SizeBytes: info.Size(),
		})
	}

	return chunks, nil
}

func (client *Client) planChunksForRequest(ctx context.Context, request normalizedRequest) ([]Chunk, error) {
	outputDir := request.OutputDir
	if outputDir == "" {
		createdDir, err := os.MkdirTemp("", "moth-transcription-*")
		if err != nil {
			return nil, fmt.Errorf("openai transcription: create chunk directory: %w", err)
		}
		outputDir = createdDir
	}

	return client.PlanChunks(ctx, ChunkPlanOptions{
		InputPath:      request.FilePath,
		OutputDir:      outputDir,
		MaxUploadBytes: request.MaxUploadBytes,
		ChunkDuration:  request.ChunkDuration,
		Overlap:        request.ChunkOverlap,
	})
}

func (client *Client) transcribeSingleFile(
	ctx context.Context,
	apiKey string,
	request normalizedRequest,
) (Result, error) {
	result, err := client.transcribeFile(ctx, apiKey, request, request.FilePath)
	if err != nil {
		return Result{}, err
	}
	result.Metadata = transcriptionMetadata(request, 1)

	return result, nil
}

func (client *Client) transcribeChunks(
	ctx context.Context,
	apiKey string,
	request normalizedRequest,
	chunks []Chunk,
) ([]Result, error) {
	results := make([]Result, len(chunks))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(request.MaxParallelTranscriptions)

	for index, chunk := range chunks {
		group.Go(func() error {
			result, err := client.transcribeFile(groupCtx, apiKey, request, chunk.Path)
			if err != nil {
				return err
			}
			results[index] = result

			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}

func (client *Client) transcribeFile(
	ctx context.Context,
	apiKey string,
	request normalizedRequest,
	filePath string,
) (Result, error) {
	body, contentType, err := multipartTranscriptionBody(request, filePath)
	if err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.baseURL+"/audio/transcriptions",
		bytes.NewReader(body),
	)
	if err != nil {
		return Result{}, fmt.Errorf("openai transcription: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("openai transcription request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Result{}, openAIStatusError(resp, apiKey)
	}

	var response openAITranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return Result{}, fmt.Errorf("openai transcription decode response: %w", err)
	}

	return mapOpenAIResponse(response), nil
}

func multipartTranscriptionBody(request normalizedRequest, filePath string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("model", request.Model); err != nil {
		return nil, "", fmt.Errorf("openai transcription multipart model: %w", err)
	}
	if request.Language != "" {
		if err := writer.WriteField("language", request.Language); err != nil {
			return nil, "", fmt.Errorf("openai transcription multipart language: %w", err)
		}
	}
	if err := writer.WriteField("response_format", request.ResponseFormat); err != nil {
		return nil, "", fmt.Errorf("openai transcription multipart response format: %w", err)
	}
	for _, granularity := range request.TimestampGranularities {
		if err := writer.WriteField("timestamp_granularities[]", granularity); err != nil {
			return nil, "", fmt.Errorf("openai transcription multipart timestamp granularity: %w", err)
		}
	}
	if err := writeMultipartFile(writer, filePath); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("openai transcription close multipart: %w", err)
	}

	return body.Bytes(), writer.FormDataContentType(), nil
}

func writeMultipartFile(writer *multipart.Writer, filePath string) error {
	//nolint:gosec // The caller intentionally selects the local audio file to upload.
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("openai transcription open audio file: %w", err)
	}
	defer func() { _ = file.Close() }()

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("openai transcription multipart file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("openai transcription copy audio file: %w", err)
	}

	return nil
}

func (client *Client) probeDuration(ctx context.Context, inputPath string) (time.Duration, error) {
	result, err := client.runner.Run(ctx, tools.Command{
		Tool:             tools.ToolFFprobe,
		Path:             client.ffprobePath,
		Args:             []string{"-v", "error", "-show_entries", "format=duration", "-of", "json", inputPath},
		StdoutLimitBytes: toolOutputLimit,
		StderrLimitBytes: toolOutputLimit,
	})
	if err != nil {
		return 0, fmt.Errorf("openai transcription ffprobe duration: %w", err)
	}

	var response ffprobeResponse
	if unmarshalErr := json.Unmarshal(result.Stdout, &response); unmarshalErr != nil {
		return 0, fmt.Errorf("openai transcription ffprobe decode duration: %w", unmarshalErr)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(response.Format.Duration), 64)
	if err != nil {
		return 0, fmt.Errorf("openai transcription ffprobe parse duration: %w", err)
	}
	if seconds <= 0 {
		return 0, fmt.Errorf("openai transcription ffprobe duration must be positive")
	}

	return time.Duration(seconds * float64(time.Second)), nil
}

func (client *Client) splitChunk(
	ctx context.Context,
	inputPath string,
	outputPath string,
	offset time.Duration,
	duration time.Duration,
) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return fmt.Errorf("openai transcription create chunk directory: %w", err)
	}
	_, err := client.runner.Run(ctx, tools.Command{
		Tool: tools.ToolFFmpeg,
		Path: client.ffmpegPath,
		Args: []string{
			"-y",
			"-ss", formatToolDuration(offset),
			"-i", inputPath,
			"-t", formatToolDuration(duration),
			"-c", "copy",
			outputPath,
		},
		StdoutLimitBytes: toolOutputLimit,
		StderrLimitBytes: toolOutputLimit,
	})
	if err != nil {
		return fmt.Errorf("openai transcription ffmpeg split chunk: %w", err)
	}

	return nil
}

type normalizedRequest struct {
	FilePath                  string
	Language                  string
	Model                     string
	ResponseFormat            string
	TimestampGranularities    []string
	MaxUploadBytes            int64
	ChunkDuration             time.Duration
	ChunkOverlap              time.Duration
	MaxParallelTranscriptions int
	OutputDir                 string
}

func normalizeRequest(request Request) (normalizedRequest, error) {
	model := request.Model
	if model == "" {
		model = defaultTranscriptionModel
	}
	responseFormat := request.ResponseFormat
	if responseFormat == "" {
		responseFormat = defaultTranscriptionFormat
	}
	if len(request.TimestampGranularities) > 0 && (model != "whisper-1" || responseFormat != "verbose_json") {
		return normalizedRequest{}, fmt.Errorf(
			"openai transcription timestamp granularities require model whisper-1 and response_format verbose_json",
		)
	}

	maxUploadBytes := request.MaxUploadBytes
	if maxUploadBytes <= 0 {
		maxUploadBytes = defaultMaxUploadBytes
	}
	chunkDuration := request.ChunkDuration
	if chunkDuration <= 0 {
		chunkDuration = defaultChunkDuration
	}
	chunkOverlap := request.ChunkOverlap
	if chunkOverlap < 0 {
		return normalizedRequest{}, fmt.Errorf("openai transcription chunk overlap cannot be negative")
	}
	if chunkOverlap == 0 {
		chunkOverlap = defaultChunkOverlap
	}
	if chunkOverlap >= chunkDuration {
		return normalizedRequest{}, fmt.Errorf("openai transcription chunk overlap must be less than chunk duration")
	}
	maxParallel := request.MaxParallelTranscriptions
	if maxParallel <= 0 {
		maxParallel = defaultMaxParallelTranscription
	}

	return normalizedRequest{
		FilePath:                  request.FilePath,
		Language:                  request.Language,
		Model:                     model,
		ResponseFormat:            responseFormat,
		TimestampGranularities:    append([]string(nil), request.TimestampGranularities...),
		MaxUploadBytes:            maxUploadBytes,
		ChunkDuration:             chunkDuration,
		ChunkOverlap:              chunkOverlap,
		MaxParallelTranscriptions: maxParallel,
		OutputDir:                 request.OutputDir,
	}, nil
}

func normalizeChunkPlan(options ChunkPlanOptions) (ChunkPlanOptions, error) {
	if options.MaxUploadBytes <= 0 {
		options.MaxUploadBytes = defaultMaxUploadBytes
	}
	if options.ChunkDuration <= 0 {
		options.ChunkDuration = defaultChunkDuration
	}
	if options.Overlap < 0 {
		return ChunkPlanOptions{}, fmt.Errorf("openai transcription chunk overlap cannot be negative")
	}
	if options.Overlap == 0 {
		options.Overlap = defaultChunkOverlap
	}
	if options.Overlap >= options.ChunkDuration {
		return ChunkPlanOptions{}, fmt.Errorf("openai transcription chunk overlap must be less than chunk duration")
	}
	if options.OutputDir == "" {
		return ChunkPlanOptions{}, fmt.Errorf("openai transcription chunk output directory is required")
	}

	return options, nil
}

type ffprobeResponse struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

type openAITranscriptionResponse struct {
	Text     string                  `json:"text"`
	Segments []openAIResponseSegment `json:"segments"`
}

type openAIResponseSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

func openAIStatusError(resp *http.Response, apiKey string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, openAIResponseBodyMax))
	responseText := strings.ReplaceAll(strings.TrimSpace(string(body)), apiKey, "[redacted]")

	return fmt.Errorf("openai transcription failed: status %d: %s", resp.StatusCode, responseText)
}

func mapOpenAIResponse(response openAITranscriptionResponse) Result {
	segments := make([]Segment, 0, len(response.Segments))
	for _, segment := range response.Segments {
		segments = append(segments, Segment{
			Start: secondsToDuration(segment.Start),
			End:   secondsToDuration(segment.End),
			Text:  segment.Text,
		})
	}

	result := Result{Text: response.Text}
	if len(segments) > 0 {
		result.Segments = segments
	}

	return result
}

func mergeChunkResults(request normalizedRequest, chunks []Chunk, results []Result) Result {
	texts := make([]string, 0, len(results))
	segments := make([]Segment, 0)
	for index, result := range results {
		if result.Text != "" {
			texts = append(texts, result.Text)
		}
		for _, segment := range result.Segments {
			segments = append(segments, Segment{
				Start: segment.Start + chunks[index].Offset,
				End:   segment.End + chunks[index].Offset,
				Text:  segment.Text,
			})
		}
	}

	return Result{
		Text:     strings.Join(texts, " "),
		Segments: segments,
		Metadata: transcriptionMetadata(request, len(chunks)),
	}
}

func transcriptionMetadata(request normalizedRequest, chunks int) map[string]any {
	return map[string]any{
		"model":           request.Model,
		"response_format": request.ResponseFormat,
		"chunks":          chunks,
	}
}

func estimatedChunkCount(duration time.Duration, chunkDuration time.Duration, overlap time.Duration) int {
	step := chunkDuration - overlap
	if step <= 0 {
		return 1
	}
	count := int(duration / step)
	if duration%step != 0 {
		count++
	}
	if count < 1 {
		return 1
	}

	return count
}

func minDuration(first time.Duration, second time.Duration) time.Duration {
	if first < second {
		return first
	}

	return second
}

func secondsToDuration(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

func formatToolDuration(duration time.Duration) string {
	seconds := duration.Seconds()
	if seconds == float64(int64(seconds)) {
		return fmt.Sprintf("%.0f", seconds)
	}

	return fmt.Sprintf("%.3f", seconds)
}
