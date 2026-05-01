package transcription

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alnah/moth/internal/tools"
)

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

func TestPlanChunksValidatesOptionsBeforeRunningTools(t *testing.T) {
	inputPath := writeTestAudioFile(t, "invalid-options.mp3", []byte("audio"))
	tests := []struct {
		name    string
		options ChunkPlanOptions
		want    string
	}{
		{
			name:    "missing output directory",
			options: ChunkPlanOptions{InputPath: inputPath},
			want:    "output directory",
		},
		{
			name: "negative overlap",
			options: ChunkPlanOptions{
				InputPath: inputPath,
				OutputDir: t.TempDir(),
				Overlap:   -time.Second,
			},
			want: "negative",
		},
		{
			name: "overlap at duration",
			options: ChunkPlanOptions{
				InputPath:     inputPath,
				OutputDir:     t.TempDir(),
				ChunkDuration: time.Minute,
				Overlap:       time.Minute,
			},
			want: "less than chunk duration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "60", ChunkSize: 1})
			client := newOpenAITestClient("https://openai.example.test", runner)

			_, err := client.PlanChunks(context.Background(), test.options)
			if err == nil {
				t.Fatal("PlanChunks invalid options error = nil, want error")
			}
			assertTranscriptionErrorContains(t, err, test.want)
			if len(runner.commands) != 0 {
				t.Fatalf("commands = %#v, want validation to fail before tools", runner.commands)
			}
		})
	}
}

func TestPlanChunksUsesDefaultDurationAndOverlap(t *testing.T) {
	inputPath := writeTestAudioFile(t, "default-chunks.mp3", bytes.Repeat([]byte("a"), 20))
	outputDir := t.TempDir()
	runner := newRecordingAudioRunner(t, audioRunnerOptions{Duration: "601", ChunkSize: 9})
	client := newOpenAITestClient("https://openai.example.test", runner)

	chunks, err := client.PlanChunks(context.Background(), ChunkPlanOptions{
		InputPath:      inputPath,
		OutputDir:      outputDir,
		MaxUploadBytes: 10,
	})
	if err != nil {
		t.Fatalf("PlanChunks with defaults error = %v, want nil", err)
	}

	assertFFmpegSplitCommand(t, runner.ffmpegCommand(0), inputPath, 0, 10*time.Minute)
	assertFFmpegSplitCommand(t, runner.ffmpegCommand(1), inputPath, 598*time.Second, 3*time.Second)
	assertTranscriptChunks(t, chunks, []Chunk{
		{Path: filepath.Join(outputDir, "chunk-000.mp3"), Offset: 0, Duration: 10 * time.Minute, SizeBytes: 9},
		{Path: filepath.Join(outputDir, "chunk-001.mp3"), Offset: 598 * time.Second, Duration: 3 * time.Second, SizeBytes: 9},
	})
}

func TestPlanChunksReportsToolFailures(t *testing.T) {
	inputPath := writeTestAudioFile(t, "tool-failure.mp3", bytes.Repeat([]byte("a"), 20))
	tests := []struct {
		name          string
		runnerOptions audioRunnerOptions
		want          string
	}{
		{
			name:          "ffprobe command failure",
			runnerOptions: audioRunnerOptions{Duration: "60", ChunkSize: 1, FFprobeError: fmt.Errorf("ffprobe unavailable")},
			want:          "ffprobe duration",
		},
		{
			name:          "ffprobe malformed JSON",
			runnerOptions: audioRunnerOptions{FFprobeStdout: `{`, ChunkSize: 1},
			want:          "decode",
		},
		{
			name:          "ffprobe non-positive duration",
			runnerOptions: audioRunnerOptions{Duration: "0", ChunkSize: 1},
			want:          "positive",
		},
		{
			name:          "ffmpeg command failure",
			runnerOptions: audioRunnerOptions{Duration: "60", ChunkSize: 1, FFmpegError: fmt.Errorf("ffmpeg failed")},
			want:          "ffmpeg split",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := newRecordingAudioRunner(t, test.runnerOptions)
			client := newOpenAITestClient("https://openai.example.test", runner)

			_, err := client.PlanChunks(context.Background(), ChunkPlanOptions{
				InputPath:      inputPath,
				OutputDir:      t.TempDir(),
				MaxUploadBytes: 10,
				ChunkDuration:  time.Minute,
				Overlap:        10 * time.Second,
			})
			if err == nil {
				t.Fatal("PlanChunks tool failure error = nil, want error")
			}
			assertTranscriptionErrorContains(t, err, test.want)
		})
	}
}

func TestEstimatedChunkCountBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		duration      time.Duration
		chunkDuration time.Duration
		overlap       time.Duration
		want          int
	}{
		{name: "exact step", duration: 100 * time.Second, chunkDuration: time.Minute, overlap: 10 * time.Second, want: 2},
		{name: "partial step", duration: 101 * time.Second, chunkDuration: time.Minute, overlap: 10 * time.Second, want: 3},
		{name: "non-positive step", duration: time.Minute, chunkDuration: time.Minute, overlap: time.Minute, want: 1},
		{name: "zero duration minimum", duration: 0, chunkDuration: time.Minute, overlap: 10 * time.Second, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := estimatedChunkCount(test.duration, test.chunkDuration, test.overlap); got != test.want {
				t.Fatalf("estimatedChunkCount() = %d, want %d", got, test.want)
			}
		})
	}
}
