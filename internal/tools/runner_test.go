package tools_test

import (
	"context"
	"errors"
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
		Path: programPath,
		Args: []string{"fail"},
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

func TestRunStopsProcessWhenContextTimesOut(t *testing.T) {
	programPath := buildFakeToolProgram(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := tools.Run(ctx, tools.Command{
		Path: programPath,
		Args: []string{"sleep", "10"},
	})
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("run timed out fake tool error = nil, want context deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run timed out fake tool error = %v, want context deadline exceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("run returned after %s, want process stopped promptly after context timeout", elapsed)
	}
}
