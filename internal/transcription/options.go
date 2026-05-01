package transcription

import (
	"fmt"
	"time"
)

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
