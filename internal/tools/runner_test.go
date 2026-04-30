package tools_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alnah/moth/internal/tools"
)

func TestRunCapturesOutputAndArguments(t *testing.T) {
	programPath := buildFakeToolProgram(t)

	result, err := tools.Run(context.Background(), tools.Command{
		Path: programPath,
		Args: []string{"echo-args", "alpha", "two words"},
	})
	if err != nil {
		t.Fatalf("run fake tool: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if got := string(result.Stdout); got != "alpha\ntwo words\n" {
		t.Fatalf("stdout = %q, want argument echo", got)
	}
	if got := string(result.Stderr); got != "" {
		t.Fatalf("stderr = %q, want empty stderr", got)
	}
}

func TestRunPropagatesNonZeroExitWithStderr(t *testing.T) {
	programPath := buildFakeToolProgram(t)

	result, err := tools.Run(context.Background(), tools.Command{
		Tool: tools.ToolYTDLP,
		Path: programPath,
		Args: []string{"fail", "two words"},
	})
	if err == nil {
		t.Fatal("run failing fake tool error = nil, want failure")
	}
	if !errors.Is(err, tools.ErrToolFailed) {
		t.Fatalf("run failing fake tool error = %v, want ErrToolFailed", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(string(result.Stderr), "fake tool failed") {
		t.Fatalf("stderr = %q, want fake failure diagnostic", result.Stderr)
	}
	for _, want := range []string{"yt-dlp", "\"fail\"", "\"two words\""} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run failing fake tool error = %q, want context containing %q", err, want)
		}
	}
}

func TestRunTruncatesCapturedOutputAtConfiguredLimits(t *testing.T) {
	programPath := buildFakeToolProgram(t)

	result, err := tools.Run(context.Background(), tools.Command{
		Path:             programPath,
		Args:             []string{"write-output", "stdout data", "stderr data"},
		StdoutLimitBytes: 6,
		StderrLimitBytes: 6,
	})
	if err != nil {
		t.Fatalf("run fake tool with output limits: %v", err)
	}
	if got := string(result.Stdout); got != "stdout" {
		t.Fatalf("stdout = %q, want truncated stdout", got)
	}
	if got := string(result.Stderr); got != "stderr" {
		t.Fatalf("stderr = %q, want truncated stderr", got)
	}
	if !result.StdoutTruncated || !result.StderrTruncated {
		t.Fatalf("truncated flags = stdout:%t stderr:%t, want both true", result.StdoutTruncated, result.StderrTruncated)
	}
}

func TestRunMissingToolReturnsSemanticError(t *testing.T) {
	_, err := tools.Run(context.Background(), tools.Command{
		Path: "/definitely/missing/moth/tool",
	})
	if err == nil {
		t.Fatal("run missing tool error = nil, want tool_missing error")
	}
	if !errors.Is(err, tools.ErrToolMissing) {
		t.Fatalf("run missing tool error = %v, want ErrToolMissing", err)
	}
}

func TestRunReturnsDeadlineWhenContextAlreadyTimedOut(t *testing.T) {
	programPath := buildFakeToolProgram(t)
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	_, err := tools.Run(ctx, tools.Command{Path: programPath})
	if err == nil {
		t.Fatal("run with expired context error = nil, want context deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run with expired context error = %v, want context deadline exceeded", err)
	}
}

func TestRunStopsProcessWhenContextIsCanceled(t *testing.T) {
	programPath := buildFakeToolProgram(t)
	readyPath := filepath.Join(t.TempDir(), "ready")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	started := time.Now()
	go func() {
		_, err := tools.Run(ctx, tools.Command{
			Path: programPath,
			Args: []string{"wait-for-cancel", readyPath},
		})
		runDone <- err
	}()

	waitForFile(t, readyPath)
	cancel()
	err := <-runDone
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("run canceled fake tool error = nil, want context cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run canceled fake tool error = %v, want context canceled", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("run returned after %s, want process stopped promptly after context cancellation", elapsed)
	}
}
