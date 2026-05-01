package transcription

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alnah/moth/internal/tools"
)

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

func (client *Client) planChunksForRequest(
	ctx context.Context,
	request normalizedRequest,
) ([]Chunk, func(), error) {
	outputDir := request.OutputDir
	var cleanup func()
	if outputDir == "" {
		createdDir, err := os.MkdirTemp("", "moth-transcription-*")
		if err != nil {
			return nil, nil, fmt.Errorf("openai transcription: create chunk directory: %w", err)
		}
		outputDir = createdDir
		cleanup = func() { _ = os.RemoveAll(createdDir) }
	}

	chunks, err := client.PlanChunks(ctx, ChunkPlanOptions{
		InputPath:      request.FilePath,
		OutputDir:      outputDir,
		MaxUploadBytes: request.MaxUploadBytes,
		ChunkDuration:  request.ChunkDuration,
		Overlap:        request.ChunkOverlap,
	})
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, err
	}

	return chunks, cleanup, nil
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

type ffprobeResponse struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
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

func formatToolDuration(duration time.Duration) string {
	seconds := duration.Seconds()
	if seconds == float64(int64(seconds)) {
		return fmt.Sprintf("%.0f", seconds)
	}

	return fmt.Sprintf("%.3f", seconds)
}
